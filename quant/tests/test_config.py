"""Tests for config constants and defaults."""

from config import (
    DATA_PATH,
    DATE_COL,
    PRICE_COLS,
    VOLUME_COL,
    INITIAL_CAPITAL,
    TRANSACTION_FEE,
    SLIPPAGE,
    EXECUTION_PRICE,
    MA_SHORT,
    MA_LONG,
    ATR_PERIOD,
)


class TestConfigConstants:
    """Verify config values exist and have expected types."""

    def test_initial_capital_positive(self):
        assert INITIAL_CAPITAL == 100000

    def test_transaction_fee_range(self):
        assert 0 <= TRANSACTION_FEE <= 1

    def test_execution_price_mode(self):
        assert EXECUTION_PRICE in ("next_open", "same_close")

    def test_ma_short_less_than_long(self):
        assert MA_SHORT < MA_LONG

    def test_atr_period_positive(self):
        assert ATR_PERIOD > 0

    def test_price_cols_contains_close(self):
        assert "close" in PRICE_COLS

    def test_date_col_is_string(self):
        assert isinstance(DATE_COL, str)
        assert len(DATE_COL) > 0

    def test_volume_col_is_string(self):
        assert isinstance(VOLUME_COL, str)

    def test_slippage_zero(self):
        # Currently slippage is not considered (set to 0.0)
        assert SLIPPAGE == 0.0
