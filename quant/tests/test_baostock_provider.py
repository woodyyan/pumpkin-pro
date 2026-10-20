"""BaoStock 接收与 Provider 资源边界回归，不连接网络。

测试依赖 baostock==0.9.2 的传输/分页源码；登录、连接和配额均使用 stub。
CI 必须安装该 SDK，不能因缺依赖而跳过资源安全回归。
"""

import importlib.util
import sys
import time
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import Mock

import pytest

from data_sources.health import DataSourceHealth
from data_sources.manager import DataSourceManager
from data_sources.models import Capability, DataSourceRequest, DailyBar, Market
from data_sources.providers import baostock as provider_module
from data_sources.providers.baostock import BaoStockProvider, _BoundedSocket


class FakeClock:
    def __init__(self):
        self.now = 100.0

    def monotonic(self):
        return self.now


class FakeSocket:
    def __init__(self, chunks=(), *, clock=None, recv_delay=0, timeout=None):
        self.chunks = iter(chunks)
        self.clock = clock
        self.recv_delay = recv_delay
        self.timeout = timeout
        self.timeouts = []
        self.send_timeouts = []
        self.recv_timeouts = []
        self.sent = []
        self.recv_calls = 0
        self.close_calls = 0
        self.recv_call_limit = 1100
        self.send_error = None
        self.send_count = None
        self.recv_args = []

    def send(self, data, *args, **kwargs):
        self.sent.append((data, args, kwargs))
        self.send_timeouts.append(self.timeout)
        if self.send_error is not None:
            raise self.send_error
        return len(data) if self.send_count is None else self.send_count

    def recv(self, size, *args, **kwargs):
        self.recv_calls += 1
        # 即使 EOF 防护回归，测试也只执行有限次接收。
        assert self.recv_calls <= self.recv_call_limit
        self.recv_timeouts.append(self.timeout)
        self.recv_args.append((size, args, kwargs))
        if self.clock is not None:
            self.clock.now += self.recv_delay
        chunk = next(self.chunks, b"")
        if isinstance(chunk, Exception):
            raise chunk
        return chunk

    def gettimeout(self):
        return self.timeout

    def settimeout(self, value):
        self.timeout = value
        self.timeouts.append(value)

    def close(self):
        self.close_calls += 1

    def fileno(self):
        return 42


@pytest.fixture
def clock(monkeypatch):
    clock = FakeClock()
    monkeypatch.setattr(provider_module, "time", SimpleNamespace(
        monotonic=clock.monotonic,
        sleep=Mock(side_effect=AssertionError("资源边界错误不应重试等待")),
    ))
    return clock


@pytest.fixture
def sdk(monkeypatch):
    # 不 import 包入口或登录模块：只加载传输/分页源码，其依赖全部 stub。
    modules = {}
    for name in (
        "baostock", "baostock.common", "baostock.common.context",
        "baostock.common.contants", "baostock.util", "baostock.data",
        "baostock.data.messageheader",
    ):
        module = ModuleType(name)
        module.__path__ = []
        monkeypatch.setitem(sys.modules, name, module)
        modules[name] = module
        if "." in name:
            parent, child = name.rsplit(".", 1)
            setattr(modules[parent], child, module)

    cons = modules["baostock.common.contants"]
    cons.MESSAGE_SPLIT = "\x01"
    header = cons.MESSAGE_SPLIT.join(("0", "plain", "0", ""))
    cons.MESSAGE_HEADER_LENGTH = len(header)
    cons.COMPRESSED_MESSAGE_TYPE_TUPLE = ()
    cons.BAOSTOCK_CLIENT_VERSION = "stub"
    cons.BSERR_NO_LOGIN = "1"
    cons.BSERR_SUCCESS = "0"
    cons.BAOSTOCK_PER_PAGE_COUNT = 2
    modules["baostock.data.messageheader"].to_message_header = lambda *args: header

    def load_sdk_source(name, relative_path):
        path = next((Path(root) / "baostock" / relative_path for root in sys.path
                     if (Path(root) / "baostock" / relative_path).is_file()), None)
        assert path is not None, (
            f"缺少 BaoStock SDK 测试源码 {relative_path}；"
            "请执行 python -m pip install baostock==0.9.2，并检查 CI 的依赖安装步骤"
        )
        spec = importlib.util.spec_from_file_location(name, path)
        module = importlib.util.module_from_spec(spec)
        monkeypatch.setitem(sys.modules, name, module)
        parent, child = name.rsplit(".", 1)
        setattr(modules[parent], child, module)
        spec.loader.exec_module(module)
        return module

    socketutil = load_sdk_source("baostock.util.socketutil", "util/socketutil.py")
    resultset = load_sdk_source("baostock.data.resultset", "data/resultset.py")
    bs = modules["baostock"]
    context = modules["baostock.common.context"]
    raw_socket = FakeSocket()
    context.default_socket = raw_socket

    def login():
        assert context.default_socket is raw_socket
        return SimpleNamespace(error_code="0", error_msg="")

    bs.login = Mock(side_effect=login)
    bs.logout = Mock()
    bs.query_history_k_data_plus = Mock()
    quota = Mock()
    quota.try_acquire.return_value = True
    monkeypatch.setattr(provider_module, "get_global_quota_guard", lambda: quota)
    return SimpleNamespace(
        bs=bs, context=context, raw_socket=raw_socket, socketutil=socketutil,
        resultset=resultset, cons=cons, header=header, quota=quota,
    )


def request(**kwargs):
    values = dict(capability=Capability.DAILY_BARS, market=Market.ASHARE,
                  symbol="000001", start_date="2026-07-10", end_date="2026-07-11")
    values.update(kwargs)
    return DataSourceRequest(**values)


def row(date="20260710"):
    return [date, "sz.000001", "10", "12", "9", "11", "10", "100", "1100", "1.5"]


def result(sdk, rows):
    rs = sdk.resultset.ResultData()
    rs.error_code = "0"
    rs.data = rows
    rs.cur_page_num = "1"
    rs.msg_body = sdk.cons.MESSAGE_SPLIT.join(("stub", "user", "1", "payload"))
    rs.msg_type = "plain"
    return rs


def test_sdk_send_msg_eof_stops_after_one_recv(sdk, clock):
    wrapper = _BoundedSocket(sdk.raw_socket)
    sdk.context.default_socket = wrapper
    started = time.monotonic()
    assert sdk.socketutil.send_msg("stub") is None
    assert time.monotonic() - started < 1.0
    assert sdk.raw_socket.recv_calls == 1
    assert isinstance(wrapper.error, ConnectionError)
    assert sdk.raw_socket.close_calls == 1
    assert sdk.raw_socket.timeout is None
    with pytest.raises(ConnectionError) as caught:
        wrapper.send(b"retry")
    assert caught.value is wrapper.error
    assert len(sdk.raw_socket.sent) == 1


@pytest.mark.parametrize("original_timeout", [None, 2.5])
def test_socket_delegates_and_resets_each_frame(clock, original_timeout):
    raw = FakeSocket([b"abc", b"def", b"next"], timeout=original_timeout)
    wrapper = _BoundedSocket(raw)
    assert wrapper.fileno() == 42
    assert wrapper.send(b"one", 1) == 3
    assert raw.sent == [(b"one", (1,), {})]
    assert raw.send_timeouts == [30.0]
    clock.now += 5
    assert wrapper.recv(8192, 2) == b"abc"
    clock.now += 4
    assert wrapper.recv(8192) == b"def"
    assert raw.recv_timeouts == [25.0, 21.0]
    assert raw.recv_args[0] == (8192, (2,), {})
    assert wrapper._response_bytes == 6
    clock.now += 10
    assert wrapper.send(b"two") == 3
    assert wrapper._response_bytes == 0
    assert wrapper.recv(8192) == b"next"
    assert raw.recv_timeouts[-1] == 30.0
    assert raw.timeout == original_timeout
    assert wrapper.error is None
    assert raw.close_calls == 0


def test_sdk_normal_response_keeps_sdk_method_unchanged(sdk, clock):
    response = sdk.header.encode() + b"payload<![CDATA[]]>\n"
    sdk.raw_socket.chunks = iter([response[:5], response[5:]])
    original_send_msg = sdk.socketutil.send_msg
    provider = BaoStockProvider()
    assert provider._ensure_login()
    assert sdk.context.default_socket is provider._socket
    assert provider._socket._socket is sdk.raw_socket
    assert sdk.socketutil.send_msg("stub") == response.decode()
    assert sdk.socketutil.send_msg is original_send_msg
    assert provider._ensure_login()
    assert sdk.bs.login.call_count == 1
    assert sdk.raw_socket.close_calls == 0


def test_wrapper_keeps_owned_socket_when_context_changes(sdk, clock):
    provider = BaoStockProvider()
    assert provider._ensure_login()
    sdk.raw_socket.chunks = iter([b"owned"])
    other_socket = FakeSocket([b"other"])
    sdk.context.default_socket = other_socket
    assert provider._socket.send(b"stub") == 4
    assert provider._socket.recv(8192) == b"owned"
    assert other_socket.sent == []
    assert other_socket.recv_calls == 0


def test_response_size_is_cumulative_and_limited_to_8_mib(clock):
    assert provider_module._MAX_RESPONSE_BYTES == 8 * 1024 * 1024
    chunk = b"x" * 8192
    raw = FakeSocket([chunk] * 1025)
    wrapper = _BoundedSocket(raw)
    wrapper.send(b"stub")
    for _ in range(1024):
        assert wrapper.recv(8192) == chunk
    with pytest.raises(RuntimeError, match="8 MiB") as caught:
        wrapper.recv(8192)
    assert caught.value is wrapper.error
    assert raw.recv_calls == 1025
    assert raw.close_calls == 1


def test_sdk_rejects_oversize_before_concatenation(sdk, clock, monkeypatch):
    monkeypatch.setattr(provider_module, "_MAX_RESPONSE_BYTES", 12)
    sdk.raw_socket.chunks = iter([b"x" * 8, b"x" * 8])
    wrapper = _BoundedSocket(sdk.raw_socket)
    sdk.context.default_socket = wrapper
    assert sdk.socketutil.send_msg("stub") is None
    assert isinstance(wrapper.error, RuntimeError)
    assert sdk.raw_socket.recv_calls == 2
    assert sdk.raw_socket.close_calls == 1


def test_sdk_slow_trickle_uses_total_frame_deadline(sdk, clock):
    sdk.raw_socket.clock = clock
    sdk.raw_socket.recv_delay = 11
    sdk.raw_socket.chunks = iter([b"x"] * 10)
    wrapper = _BoundedSocket(sdk.raw_socket)
    sdk.context.default_socket = wrapper
    assert sdk.socketutil.send_msg("stub") is None
    assert isinstance(wrapper.error, TimeoutError)
    assert sdk.raw_socket.recv_timeouts == [30.0, 19.0, 8.0]
    assert sdk.raw_socket.recv_calls == 3
    assert sdk.raw_socket.close_calls == 1


def test_expired_deadline_does_not_call_recv(clock):
    raw = FakeSocket([b"late"])
    wrapper = _BoundedSocket(raw)
    wrapper.send(b"stub")
    clock.now += 30
    with pytest.raises(TimeoutError):
        wrapper.recv(8192)
    assert raw.recv_calls == 0
    assert raw.close_calls == 1


def test_late_terminator_is_not_success(sdk, clock):
    sdk.raw_socket.clock = clock
    sdk.raw_socket.recv_delay = 30
    sdk.raw_socket.chunks = iter([sdk.header.encode() + b"<![CDATA[]]>\n"])
    wrapper = _BoundedSocket(sdk.raw_socket)
    sdk.context.default_socket = wrapper
    assert sdk.socketutil.send_msg("stub") is None
    assert isinstance(wrapper.error, TimeoutError)
    assert sdk.raw_socket.close_calls == 1


@pytest.mark.parametrize("error", [TimeoutError("stub timeout"), OSError("stub reset")])
def test_recv_exception_closes_and_restores_timeout(clock, error):
    raw = FakeSocket([error], timeout=2.5)
    wrapper = _BoundedSocket(raw)
    wrapper.send(b"stub")
    with pytest.raises(type(error)) as caught:
        wrapper.recv(8192)
    assert caught.value is error is wrapper.error
    assert raw.timeout == 2.5
    assert raw.close_calls == 1
    with pytest.raises(type(error)):
        wrapper.recv(8192)
    assert raw.recv_calls == 1


@pytest.mark.parametrize("partial", [False, True])
def test_send_failure_closes_and_is_sticky(clock, partial):
    raw = FakeSocket(timeout=2.5)
    if partial:
        raw.send_count = 1
    else:
        raw.send_error = OSError("stub send failure")
    wrapper = _BoundedSocket(raw)
    with pytest.raises(OSError):
        wrapper.send(b"stub")
    assert raw.close_calls == 1
    assert raw.timeout == 2.5
    assert wrapper.error is not None
    with pytest.raises(OSError):
        wrapper.send(b"retry")
    assert len(raw.sent) == 1


@pytest.mark.parametrize("start,end", [
    ("20260710", "20260711"), ("2026-07-10", "2026-07-11"),
    ("20260710", "2026-07-11"),
])
def test_provider_normalizes_dates_and_rows(sdk, clock, start, end):
    sdk.bs.query_history_k_data_plus.return_value = result(sdk, [row()])
    provider = BaoStockProvider()
    bars = provider.fetch(request(start_date=start, end_date=end, extras={"caller": "stub"}))
    assert len(bars) == 1
    assert bars[0].trade_date == "2026-07-10"
    assert (bars[0].open, bars[0].close, bars[0].high, bars[0].low) == (10, 11, 12, 9)
    assert (bars[0].volume, bars[0].amount, bars[0].turnover_rate) == (100, 1100, 1.5)
    assert bars[0].provider == "baostock"
    kwargs = sdk.bs.query_history_k_data_plus.call_args.kwargs
    assert kwargs["start_date"] == "2026-07-10"
    assert kwargs["end_date"] == "2026-07-11"
    assert kwargs["frequency"] == "d"
    assert kwargs["adjustflag"] == "2"
    sdk.quota.try_acquire.assert_called_once_with(cost=1, caller="stub")


@pytest.mark.parametrize("end_field", ["end_date", "target_trade_date"])
def test_date_range_normalizes_before_default_lookback(end_field):
    provider = BaoStockProvider()
    dates = {"start_date": "", "end_date": "", end_field: "20260710"}
    req = request(lookback_days=1, **dates)
    assert provider._date_range(req) == ("2026-03-12", "2026-07-10")


@pytest.mark.parametrize("start,end", [
    ("20260230", "2026-07-11"), ("2026-07-10", "bad"),
    ("20260712", "20260711"),
])
def test_invalid_or_reversed_dates_rejected(start, end):
    with pytest.raises(ValueError):
        BaoStockProvider()._date_range(request(start_date=start, end_date=end))


@pytest.mark.parametrize("bad_date", [False, True])
def test_infinite_next_is_limited_including_invalid_rows(sdk, clock, bad_date):
    rs = SimpleNamespace(error_code="0", error_msg="",
                         next=Mock(return_value=True),
                         get_row_data=Mock(return_value=row("bad" if bad_date else "20260710")))
    sdk.bs.query_history_k_data_plus.return_value = rs
    provider = BaoStockProvider()
    with pytest.raises(RuntimeError, match="自然日上限 2"):
        provider.fetch(request())
    assert rs.next.call_count == 3
    assert rs.get_row_data.call_count == 2
    assert sdk.bs.query_history_k_data_plus.call_count == 1
    assert sdk.raw_socket.close_calls == 1
    assert provider._logged_in is False
    with pytest.raises(RuntimeError, match="自然日上限"):
        provider.fetch(request())
    assert sdk.bs.login.call_count == 1
    assert sdk.bs.query_history_k_data_plus.call_count == 1


def test_exact_calendar_span_allows_final_next_check(sdk, clock):
    # 两个自然日允许两行，最后一次 next() 可正常返回 False。
    rs = SimpleNamespace(error_code="0", error_msg="",
                         next=Mock(side_effect=[True, True, False]),
                         get_row_data=Mock(side_effect=[row(), row("20260711")]))
    sdk.bs.query_history_k_data_plus.return_value = rs
    bars = BaoStockProvider().fetch(request())
    assert [bar.trade_date for bar in bars] == ["2026-07-10", "2026-07-11"]
    assert rs.next.call_count == 3
    assert sdk.raw_socket.close_calls == 0


def test_row_is_converted_before_sdk_reuses_same_buffer(sdk, clock):
    shared_row = row()
    call_count = 0

    def next_row():
        nonlocal call_count
        call_count += 1
        if call_count == 2:
            shared_row[0] = "20260711"
            shared_row[5] = "22"
        return call_count <= 2

    rs = SimpleNamespace(error_code="0", error_msg="", next=next_row,
                         get_row_data=lambda: shared_row)
    sdk.bs.query_history_k_data_plus.return_value = rs
    bars = BaoStockProvider().fetch(request())
    assert [(bar.trade_date, bar.close) for bar in bars] == [
        ("2026-07-10", 11), ("2026-07-11", 22),
    ]


@pytest.mark.parametrize("failure", ["eof", "size", "deadline"])
def test_resultdata_next_swallowed_failure_is_not_partial_success(sdk, clock, monkeypatch, failure):
    rs = result(sdk, [row(), row("20260711")])
    sdk.bs.query_history_k_data_plus.return_value = rs
    if failure == "size":
        monkeypatch.setattr(provider_module, "_MAX_RESPONSE_BYTES", 12)
        sdk.raw_socket.chunks = iter([b"x" * 8, b"x" * 8])
        error_type = RuntimeError
    elif failure == "deadline":
        sdk.raw_socket.clock = clock
        sdk.raw_socket.recv_delay = 11
        sdk.raw_socket.chunks = iter([b"x"] * 10)
        error_type = TimeoutError
    else:
        error_type = ConnectionError
    provider = BaoStockProvider()
    with pytest.raises(error_type) as caught:
        provider.fetch(request())
    assert caught.value is provider._socket.error
    assert rs.cur_row_num == 2
    assert rs.error_code == "0"
    assert sdk.bs.query_history_k_data_plus.call_count == 1
    assert sdk.raw_socket.close_calls == 1
    assert provider._logged_in is False
    with pytest.raises(error_type):
        provider.fetch(request())
    assert sdk.bs.query_history_k_data_plus.call_count == 1
    assert sdk.bs.login.call_count == 1


def test_initial_query_swallowed_eof_is_raised_before_result_access(sdk, clock):
    def query(*args, **kwargs):
        assert sdk.socketutil.send_msg("stub") is None
        return None

    sdk.bs.query_history_k_data_plus.side_effect = query
    provider = BaoStockProvider()
    with pytest.raises(ConnectionError) as caught:
        provider.fetch(request())
    assert caught.value is provider._socket.error
    assert sdk.bs.query_history_k_data_plus.call_count == 1
    assert sdk.raw_socket.close_calls == 1


@pytest.mark.parametrize("failure", ["eof", "rows"])
def test_gateway_fallback_after_bounded_provider_failure(sdk, clock, failure):
    if failure == "eof":
        sdk.bs.query_history_k_data_plus.return_value = result(sdk, [row(), row("20260711")])
    else:
        sdk.bs.query_history_k_data_plus.return_value = SimpleNamespace(
            error_code="0", error_msg="", next=lambda: True, get_row_data=row,
        )
    fallback_bar = DailyBar(symbol="000001", market=Market.ASHARE,
                            trade_date="2026-07-11", open=10, close=11, high=12, low=9,
                            provider="tencent")
    fallback = Mock()
    fallback.fetch.return_value = [fallback_bar]
    manager = DataSourceManager(
        providers={"baostock": BaoStockProvider(), "tencent": fallback},
        health=DataSourceHealth(),
    )
    response = manager.fetch(request(target_trade_date="2026-07-11"))
    assert response.ok is True
    assert response.used_sources == ["tencent"]
    assert response.data == [fallback_bar]
    assert [trace.status for trace in response.trace] == ["failed", "success"]
    assert sdk.bs.query_history_k_data_plus.call_count == 1
    assert sdk.raw_socket.close_calls == 1
    fallback.fetch.assert_called_once()


def test_failed_login_never_wraps_connection(sdk, clock):
    sdk.bs.login.side_effect = None
    sdk.bs.login.return_value = SimpleNamespace(error_code="1", error_msg="stub failure")
    provider = BaoStockProvider()
    assert provider._ensure_login() is False
    assert provider._ensure_login() is False
    assert provider._socket is None
    assert sdk.context.default_socket is sdk.raw_socket
    assert sdk.bs.login.call_count == 1
