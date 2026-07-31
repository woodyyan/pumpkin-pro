"""
组合策略模块：网格(主体) + RSI/布林带择时(增强) + 放量突破熔断(机动)
================================================================
来源：用户自研 combo_strategy.py（网格 60% + RSI/布林择时 30% + 突破熔断 10%）。

本类适配 pumpkin 回测/信号框架的"单列 signal + signal_size"约定（方案 A）：
- 内部仍逐 K 线跑原始三腿状态机（保留执行优先级：向下熔断 > 硬止损 > 向上突破 > 网格 > 择时）；
- 每根 K 线把三腿净动作**投影**成一个 signal(buy/sell/hold) + signal_size(占当前现金/持股的比例)；
- 成交交由 BacktestEngine 按 next_open 统一执行、按引擎 fee 统一收费（策略内不重复扣费）；
- 额外输出 combo_leg 列（本根实际触发的腿，用于信号推送文案）。

指标列由 registry 的 attach_indicators 预先挂好，本类只消费不计算。需要的列：
    combo_box_up, combo_box_low, combo_grid_step, combo_stop_line,
    combo_break_up_line, combo_break_down_line, combo_boll_up, combo_boll_low,
    combo_rsi, combo_vol_ma
"""
import numpy as np
import pandas as pd


class ComboGridStrategy:
    """网格 + RSI/布林择时 + 突破熔断 组合策略（投影为单列 signal/signal_size）。

    signal_size 语义（与 BacktestEngine 一致）：
      - buy  时表示"动用当前现金的比例"；
      - sell 时表示"卖出当前持股的比例"。
    这是三腿独立账本在单持仓引擎上的近似投影，故 size 用"腿资金占比"折算。
    """

    REQUIRED_COLUMNS = [
        "combo_box_up",
        "combo_box_low",
        "combo_grid_step",
        "combo_stop_line",
        "combo_break_up_line",
        "combo_break_down_line",
        "combo_boll_up",
        "combo_boll_low",
        "combo_rsi",
        "combo_vol_ma",
    ]

    def __init__(
        self,
        data: pd.DataFrame,
        grid_num: int = 8,
        grid_capital_ratio: float = 0.60,
        grid_stop_pct: float = 0.025,
        rsi_oversold: float = 30.0,
        rsi_overbought: float = 70.0,
        timing_capital_ratio: float = 0.30,
        break_buffer: float = 0.00,
        vol_multiple: float = 1.5,
        trail_stop_pct: float = 0.043,
        break_capital_ratio: float = 0.10,
    ):
        self.data = data.copy()
        self.grid_num = int(grid_num)
        self.grid_capital_ratio = float(grid_capital_ratio)
        self.grid_stop_pct = float(grid_stop_pct)
        self.rsi_oversold = float(rsi_oversold)
        self.rsi_overbought = float(rsi_overbought)
        self.timing_capital_ratio = float(timing_capital_ratio)
        self.break_buffer = float(break_buffer)
        self.vol_multiple = float(vol_multiple)
        self.trail_stop_pct = float(trail_stop_pct)
        self.break_capital_ratio = float(break_capital_ratio)

    def generate_signals(self) -> pd.DataFrame:
        print(
            f"📡 生成组合策略信号 (网格{self.grid_num}格/{self.grid_capital_ratio:.0%} + "
            f"择时{self.timing_capital_ratio:.0%} + 突破{self.break_capital_ratio:.0%}, "
            f"止损{self.grid_stop_pct:.1%})..."
        )
        self.data["signal"] = "hold"
        self.data["signal_size"] = 0.0
        self.data["combo_leg"] = ""

        n = len(self.data)
        if n == 0:
            return self.data

        for col in self.REQUIRED_COLUMNS:
            if col not in self.data.columns:
                raise ValueError(f"缺失指标 ({col})，请先调用 attach_indicators")

        # ── 内部状态（跨 K 线保持，用于近似三腿账本以决定投影 size 与买卖时机）──
        # 网格：记录当前已"持有买单"的格档集合（不追踪具体股数，只用于避免重复买入同档）
        grid_levels_held = set()
        grid_active = True
        timing_held = False       # 择时腿是否持仓
        break_mode = ""           # "" / "long"
        break_entry = 0.0

        # 每格网格投影 size：占总资金的 grid_capital_ratio / grid_num
        grid_size = self.grid_capital_ratio / self.grid_num if self.grid_num > 0 else 0.0

        for i in range(n):
            row = self.data.iloc[i]
            px = float(row["close"])
            hi = float(row["high"])
            lo = float(row["low"])
            box_up = row["combo_box_up"]
            box_low = row["combo_box_low"]
            step = row["combo_grid_step"]
            vol_ma = row["combo_vol_ma"]
            volume = float(row["volume"])

            # 指标未预热：跳过（保持 hold）
            if pd.isna(box_up) or pd.isna(box_low) or pd.isna(step) or step <= 0:
                continue

            side = "hold"
            size = 0.0
            leg = ""

            volume_confirmed = (not pd.isna(vol_ma)) and volume > float(vol_ma) * self.vol_multiple

            # ── 优先级1: 向下破位熔断（放量跌破下沿）── 清空全部 → 卖出 100%
            if px < float(row["combo_break_down_line"]) and volume_confirmed:
                if grid_levels_held or timing_held or break_mode == "long":
                    side, size, leg = "sell", 1.0, "break_down"
                grid_levels_held.clear()
                timing_held = False
                break_mode = ""
                grid_active = False
                self._write(i, side, size, leg)
                continue

            # ── 硬止损: 收盘跌破止损线 ── 清空网格 → 卖出全部
            if grid_active and px < float(row["combo_stop_line"]):
                if grid_levels_held or timing_held:
                    side, size, leg = "sell", 1.0, "hard_stop"
                grid_levels_held.clear()
                timing_held = False
                grid_active = False
                self._write(i, side, size, leg)
                continue

            # ── 优先级2: 向上突破追多（放量突破上沿）── 先清网格再全仓追多
            if px > float(row["combo_break_up_line"]) and volume_confirmed and break_mode != "long":
                grid_levels_held.clear()
                grid_active = False
                # 追多动用 突破 + 择时 两部分资金 → 投影为"当前现金全投"
                side, size, leg = "buy", 1.0, "break_up"
                break_mode = "long"
                break_entry = px
                self._write(i, side, size, leg)
                continue

            # ── 追多移动止损: 回落超过 trail_stop_pct 离场 ──
            if break_mode == "long" and px < break_entry * (1 - self.trail_stop_pct):
                side, size, leg = "sell", 1.0, "break_trail_stop"
                break_mode = ""
                grid_active = True
                self._write(i, side, size, leg)
                continue

            # ── 优先级3: 网格（箱体内，逐档买/卖）──
            # 单持仓引擎无法同时表达"买某档 + 卖另一档"，故取本根网格净动作：
            #   优先处理卖出（落袋），否则处理买入（越跌越买）
            grid_buy_hits = 0
            grid_sell_hits = 0
            if grid_active and box_low <= px <= box_up:
                # 卖：持仓档的上一格被触及
                for held in list(grid_levels_held):
                    sell_level = box_low + (held + 1) * step
                    if hi >= sell_level:
                        grid_levels_held.discard(held)
                        grid_sell_hits += 1
                # 买：当日最低触及某档且该档未持仓
                for level_idx in range(self.grid_num + 1):
                    level = box_low + level_idx * step
                    if lo <= level < px and level_idx not in grid_levels_held:
                        grid_levels_held.add(level_idx)
                        grid_buy_hits += 1

            # ── 择时: RSI + 布林带（增强开仓质量）──
            timing_buy = False
            timing_sell = False
            rsi = row["combo_rsi"]
            boll_low = row["combo_boll_low"]
            boll_up = row["combo_boll_up"]
            if grid_active and not pd.isna(rsi) and not pd.isna(boll_low):
                if rsi < self.rsi_oversold and lo <= float(boll_low) and not timing_held:
                    timing_buy = True
                    timing_held = True
                elif rsi > self.rsi_overbought and hi >= float(boll_up) and timing_held:
                    timing_sell = True
                    timing_held = False

            # ── 投影：把本根网格 + 择时的净动作折叠成单个 side/size ──
            # 净化优先级：卖出优先（落袋/避险），其次买入
            if grid_sell_hits > 0 or timing_sell:
                # 卖出比例 = 命中卖出的仓位近似（网格每档一格资金 + 择时全部）
                sell_frac = min(1.0, grid_sell_hits * grid_size + (self.timing_capital_ratio if timing_sell else 0.0))
                side, size = "sell", max(sell_frac, 0.0)
                leg = "+".join(([f"grid_sell x{grid_sell_hits}"] if grid_sell_hits else []) + (["timing_sell"] if timing_sell else []))
                # sell 的 size 语义是"占当前持股比例"，网格档卖出难以精确映射，
                # 保守用命中比例；择时卖出叠加。上限 1.0。
            elif grid_buy_hits > 0 or timing_buy:
                buy_frac = grid_buy_hits * grid_size + (self.timing_capital_ratio if timing_buy else 0.0)
                side, size = "buy", min(1.0, max(buy_frac, 0.0))
                leg = "+".join(([f"grid_buy x{grid_buy_hits}"] if grid_buy_hits else []) + (["timing_buy"] if timing_buy else []))

            self._write(i, side, size, leg)

        return self.data

    def _write(self, i: int, side: str, size: float, leg: str) -> None:
        idx = self.data.index[i]
        self.data.loc[idx, "signal"] = side
        self.data.loc[idx, "signal_size"] = float(size) if side in ("buy", "sell") else 0.0
        self.data.loc[idx, "combo_leg"] = leg
