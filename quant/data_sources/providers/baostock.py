"""
BaoStock 数据源 Provider — A 股日线数据第一优先级。

设计要点：
1. 长连接单例：进程启动时 bs.login() 一次，全程复用 session。
2. 串行锁：baostock 不是线程安全的，所有请求串行执行。
3. 全局配额守卫：每次请求前经过 GlobalBaostockQuotaGuard.try_acquire()。
4. 字段映射：baostock 返回的 DataFrame 归一化为 DailyBar 列表。
5. 仅支持 A 股 DAILY_BARS，不支持港股/指数/其他能力。
"""

from __future__ import annotations

import logging
import threading
import time
from datetime import datetime, timedelta
from typing import Any, List, Optional

from ..models import Capability, DataSourceRequest, DailyBar, Market
from ..normalizers.daily_bars import safe_float, parse_date
from ..quota.baostock_quota import get_global_quota_guard

logger = logging.getLogger(__name__)

# baostock adjustflag 映射
# "qfq" (前复权) → "2", "hfq" (后复权) → "1", 其他 → "3" (不复权)
_ADJUST_MAP = {"qfq": "2", "hfq": "1"}

# baostock 请求字段（与 output/baostock_quadrant_calc.py 保持一致）
_KLINE_FIELDS = "date,code,open,high,low,close,preclose,volume,amount,turn,pctChg,isST,peTTM"

# 仅完整响应后的普通错误可重试；传输及资源边界错误直接交给 Gateway。
_MAX_RETRIES = 2
_RETRY_DELAY_S = 1.0
_RESPONSE_TIMEOUT_S = 30.0
_MAX_RESPONSE_BYTES = 8 * 1024 * 1024


class _ResponseLimitError(RuntimeError):
    """响应大小或日线行数超过资源上限。"""


class _BoundedSocket:
    """仅包装登录成功后的连接，不修改 socket 或 SDK 方法。"""

    def __init__(self, sock: Any):
        self._socket = sock
        self._deadline: Optional[float] = None
        self._response_bytes = 0
        self.error: Optional[Exception] = None

    def __getattr__(self, name: str) -> Any:
        return getattr(self._socket, name)

    def fail(self, exc: Exception) -> None:
        # SDK 会吞掉接收异常，保留首个错误供 Provider 检查，禁止复用坏连接。
        if self.error is None:
            self.error = exc
            try:
                self._socket.close()
            except Exception:
                pass
        raise self.error

    def _remaining(self) -> float:
        if self.error is not None:
            raise self.error
        if self._deadline is None:
            self.fail(ConnectionError("BaoStock 接收前尚未发送请求"))
        remaining = self._deadline - time.monotonic()
        if remaining <= 0:
            self.fail(TimeoutError("BaoStock 整帧响应超过总时限"))
        return remaining

    def send(self, data: bytes, *args: Any, **kwargs: Any) -> int:
        if self.error is not None:
            raise self.error
        self._deadline = time.monotonic() + _RESPONSE_TIMEOUT_S
        self._response_bytes = 0
        try:
            original_timeout = self._socket.gettimeout()
            self._socket.settimeout(self._remaining())
            try:
                sent = self._socket.send(data, *args, **kwargs)
            finally:
                self._socket.settimeout(original_timeout)
            self._remaining()
            if sent != len(data):
                self.fail(ConnectionError("BaoStock 请求未完整发送"))
            return sent
        except Exception as exc:
            self.fail(exc)

    def recv(self, size: int, *args: Any, **kwargs: Any) -> bytes:
        try:
            remaining = self._remaining()
            original_timeout = self._socket.gettimeout()
            self._socket.settimeout(remaining)
            try:
                chunk = self._socket.recv(size, *args, **kwargs)
            finally:
                self._socket.settimeout(original_timeout)
            if not chunk:
                self.fail(ConnectionError("BaoStock 响应未完成时连接已关闭（EOF）"))
            self._response_bytes += len(chunk)
            if self._response_bytes > _MAX_RESPONSE_BYTES:
                self.fail(_ResponseLimitError("BaoStock 整帧响应超过 8 MiB 上限"))
            self._remaining()
            return chunk
        except Exception as exc:
            self.fail(exc)


def _to_baostock_code(symbol: str) -> str:
    """
    将 6 位 A 股代码映射为 baostock 格式（sh.XXXXXX / sz.XXXXXX）。

    规则：
    - 6 开头（沪市主板/科创板）→ sh.XXXXXX
    - 0/3 开头（深市主板/创业板）→ sz.XXXXXX
    - 其他前缀（8/4/920 等，北交所）→ baostock 不支持，返回空字符串
    """
    code = str(symbol or "").strip().zfill(6)
    if len(code) != 6:
        return ""
    if code.startswith(("6",)):
        return f"sh.{code}"
    if code.startswith(("0", "3")):
        return f"sz.{code}"
    # 北交所（8/4/920）baostock 不支持
    return ""


def _adjust_flag(adjust: str) -> str:
    return _ADJUST_MAP.get(adjust, "3")


class BaoStockProvider:
    """
    BaoStock 数据源 Provider。

    特性：
    - 进程启动时登录一次，全程复用 session（长连接单例）。
    - 线程安全通过 threading.Lock 保证串行（baostock 非线程安全）。
    - 每次请求前经过全局配额守卫。
    - 仅支持 A 股 DAILY_BARS。
    """

    name = "baostock"

    def __init__(self):
        self._login_lock = threading.Lock()
        self._fetch_lock = threading.Lock()
        self._logged_in = False
        self._login_attempted = False
        self._socket: Optional[_BoundedSocket] = None

    def _check_socket(self) -> None:
        if self._socket is not None and self._socket.error is not None:
            self._logged_in = False
            raise self._socket.error

    def _ensure_login(self) -> bool:
        """
        确保 baostock 已登录（线程安全，进程内只登录一次）。

        Returns:
            True 如果已登录或登录成功；False 如果登录失败。
        """
        self._check_socket()
        if self._logged_in:
            return True

        with self._login_lock:
            if self._logged_in:
                return True
            if self._login_attempted:
                # 已经尝试过且失败了，不重复尝试
                return False

            self._login_attempted = True
            try:
                import baostock as bs
                # 登录尚未有可包装的连接，仍须由外层真实任务 timeout 兜底。
                lg = bs.login()
                if lg.error_code != "0":
                    logger.error(
                        "[baostock] 登录失败: code=%s, msg=%s",
                        lg.error_code, lg.error_msg,
                    )
                    return False
                import baostock.common.context as context
                sock = context.default_socket
                if sock is None:
                    raise ConnectionError("BaoStock 登录成功但未建立连接")
                self._socket = _BoundedSocket(sock)
                context.default_socket = self._socket
                self._logged_in = True
                logger.info("[baostock] 登录成功，session 已建立")
                return True
            except ImportError:
                logger.error("[baostock] baostock 包未安装，provider 不可用")
                return False
            except Exception as exc:
                logger.error("[baostock] 登录异常: %s", exc)
                return False

    def fetch(self, request: DataSourceRequest) -> List[DailyBar]:
        """
        通过 baostock 获取日线数据。

        仅支持 A 股 DAILY_BARS，其他能力直接抛 ValueError。
        """
        if request.capability != Capability.DAILY_BARS:
            raise ValueError(f"BaoStock 不支持能力 {request.capability}")
        if request.market != Market.ASHARE:
            raise ValueError(f"BaoStock 不支持市场 {request.market}")

        # 代码映射
        bs_code = _to_baostock_code(request.symbol)
        if not bs_code:
            # 北交所或其他不支持的代码，直接返回空（让 manager 走 fallback）
            raise ValueError(f"BaoStock 不支持代码 {request.symbol}（可能是北交所）")

        # 确保登录
        if not self._ensure_login():
            raise RuntimeError("BaoStock 登录失败")

        # 配额检查（caller 从 extras 透传，便于按消费方归因；缺省回退 quadrant 保持向后兼容）
        caller = (request.extras or {}).get("caller") or "quadrant"
        quota_guard = get_global_quota_guard()
        if not quota_guard.try_acquire(cost=1, caller=caller):
            raise RuntimeError("BaoStock 配额不足或已黑名单")

        # 计算日期范围
        start, end = self._date_range(request)

        # 串行请求（baostock 非线程安全）
        with self._fetch_lock:
            return self._fetch_klines(bs_code, request.symbol, start, end, request.adjust)

    def _fetch_klines(
        self,
        bs_code: str,
        symbol: str,
        start: str,
        end: str,
        adjust: str,
    ) -> List[DailyBar]:
        """实际调用 baostock API 获取 K 线数据。"""
        import baostock as bs

        start_dt = datetime.strptime(parse_date(start), "%Y-%m-%d")
        end_dt = datetime.strptime(parse_date(end), "%Y-%m-%d")
        start, end = start_dt.strftime("%Y-%m-%d"), end_dt.strftime("%Y-%m-%d")
        max_rows = (end_dt - start_dt).days + 1
        if max_rows <= 0:
            raise ValueError("BaoStock 开始日期不能晚于结束日期")
        self._check_socket()
        adj_flag = _adjust_flag(adjust)
        last_exc: Optional[Exception] = None

        for attempt in range(_MAX_RETRIES + 1):
            try:
                rs = bs.query_history_k_data_plus(
                    bs_code,
                    _KLINE_FIELDS,
                    start_date=start,
                    end_date=end,
                    frequency="d",
                    adjustflag=adj_flag,
                )
                self._check_socket()
                if rs.error_code != "0":
                    raise RuntimeError(
                        f"baostock query failed: code={rs.error_code}, msg={rs.error_msg}"
                    )

                # 逐行转换，计数包括无效日期行，避免 next() 不推进而无限增长。
                bars: List[DailyBar] = []
                row_count = 0
                while True:
                    has_next = rs.next()
                    self._check_socket()
                    if rs.error_code != "0":
                        raise RuntimeError(
                            f"baostock page failed: code={rs.error_code}, msg={rs.error_msg}"
                        )
                    if not has_next:
                        break
                    if row_count >= max_rows:
                        raise _ResponseLimitError(
                            f"BaoStock 日线行数超过日期自然日上限 {max_rows}"
                        )
                    row_count += 1
                    row = rs.get_row_data()
                    self._check_socket()
                    trade_date = parse_date(row[0]) if len(row) > 0 else ""
                    if not trade_date:
                        continue
                    bars.append(DailyBar(
                        symbol=symbol,
                        market=Market.ASHARE,
                        trade_date=trade_date,
                        open=safe_float(row[2]) or 0.0 if len(row) > 2 else 0.0,
                        high=safe_float(row[3]) or 0.0 if len(row) > 3 else 0.0,
                        low=safe_float(row[4]) or 0.0 if len(row) > 4 else 0.0,
                        close=safe_float(row[5]) or 0.0 if len(row) > 5 else 0.0,
                        volume=safe_float(row[7]) or 0.0 if len(row) > 7 else 0.0,
                        amount=safe_float(row[8]) if len(row) > 8 else None,
                        turnover_rate=safe_float(row[9]) if len(row) > 9 else None,
                        provider=self.name,
                    ))

                if not bars:
                    raise RuntimeError(f"baostock 归一化后无有效数据: {bs_code}")

                return bars

            except Exception as exc:
                self._check_socket()
                # 未读完的帧不能在原连接重试，否则后续响应会错位。
                if isinstance(exc, (OSError, _ResponseLimitError)):
                    self._logged_in = False
                    if self._socket is not None:
                        self._socket.fail(exc)
                    raise
                last_exc = exc
                if attempt < _MAX_RETRIES:
                    logger.warning(
                        "[baostock] 拉取 %s 第 %d 次失败: %s，重试中...",
                        bs_code, attempt + 1, exc,
                    )
                    time.sleep(_RETRY_DELAY_S)
                else:
                    raise

        # 不应该到达这里
        raise last_exc or RuntimeError(f"baostock 拉取 {bs_code} 未知失败")

    def _date_range(self, request: DataSourceRequest) -> tuple[str, str]:
        """计算 baostock 请求的日期范围。"""
        end = request.end_date or request.target_trade_date
        if not end:
            end = datetime.now().strftime("%Y-%m-%d")
        end = datetime.strptime(parse_date(end), "%Y-%m-%d").strftime("%Y-%m-%d")

        if request.start_date:
            start = datetime.strptime(parse_date(request.start_date), "%Y-%m-%d").strftime("%Y-%m-%d")
        else:
            # 按 lookback_days 往前推
            days = max(request.lookback_days, 90)
            start_dt = datetime.strptime(end, "%Y-%m-%d") - timedelta(days=days + 30)
            start = start_dt.strftime("%Y-%m-%d")

        if start > end:
            raise ValueError("BaoStock 开始日期不能晚于结束日期")
        return start, end

    def logout(self) -> None:
        """登出 baostock（仅在进程退出时调用）。"""
        if self._logged_in:
            try:
                import baostock as bs
                bs.logout()
                self._logged_in = False
                logger.info("[baostock] 已登出")
            except Exception as exc:
                logger.warning("[baostock] 登出异常: %s", exc)
