from capital_map.models import CapitalMapSector, CapitalMapSnapshot, CapitalMapStock
from capital_map.normalizer import normalize_capital_map_stock, normalize_capital_map_sector
from capital_map.service import build_market_payload, calculate_poc


def test_normalize_stock_prefers_pe_ttm_and_market_prefix():
    stock = normalize_capital_map_stock({
        "f12": "600000",
        "f14": "浦发银行",
        "f13": 1,
        "f6": 500_000_000,
        "f9": 8.5,
        "f115": 7.2,
        "f62": 20_000_000,
        "f20": 100_000_000_000,
    })

    assert stock.symbol == "SH600000"
    assert stock.pe == 7.2
    assert stock.pe_source == "PE TTM"
    assert stock.amount_yi == 5
    assert stock.main_net_inflow_yi == 0.2


def test_normalize_sector_calculates_inflow_intensity():
    sector = normalize_capital_map_sector({"f12": "BK1", "f14": "银行", "f6": 400_000_000, "f62": 20_000_000})

    assert sector.amount_yi == 4
    assert sector.main_net_inflow_yi == 0.2
    assert sector.net_inflow_intensity == 5


def test_calculate_poc_keeps_bin_logic():
    stocks = [
        CapitalMapStock(code="000001", symbol="SZ000001", name="平安银行", market="SZ", pe=12.3, amount=300_000_000, amount_yi=3, pct_chg=1.2),
        CapitalMapStock(code="600000", symbol="SH600000", name="浦发银行", market="SH", pe=14.8, amount=500_000_000, amount_yi=5, pct_chg=-0.5),
        CapitalMapStock(code="300001", symbol="SZ300001", name="特锐德", market="SZ", pe=42.1, amount=900_000_000, amount_yi=9, pct_chg=2.5),
        CapitalMapStock(code="688001", symbol="SH688001", name="无效高PE", market="SH", pe=125, amount=1_200_000_000),
    ]

    poc, distribution = calculate_poc(stocks)

    assert poc["key"] == "40-45"
    assert len(distribution) == 2
    assert distribution[0]["key"] == "10-15"
    assert distribution[0]["stockCount"] == 2
    assert distribution[0]["totalAmountYi"] == 8


def test_build_market_payload_summarizes_sample_and_sectors():
    snapshot = CapitalMapSnapshot(
        stocks=[
            CapitalMapStock(code="000001", symbol="SZ000001", name="平安银行", market="SZ", pe=12, amount=300_000_000, amount_yi=3, pct_chg=1),
            CapitalMapStock(code="600000", symbol="SH600000", name="浦发银行", market="SH", pe=18, amount=700_000_000, amount_yi=7, pct_chg=-2),
            CapitalMapStock(code="300001", symbol="SZ300001", name="无效估值", market="SZ", pe=None, amount=100_000_000, amount_yi=1, pct_chg=0),
        ],
        sectors=[
            CapitalMapSector(code="BK1", name="银行", amount=500_000_000, amount_yi=5, main_net_inflow=20_000_000, main_net_inflow_yi=0.2),
            CapitalMapSector(code="BK2", name="半导体", amount=250_000_000, amount_yi=2.5, main_net_inflow=80_000_000, main_net_inflow_yi=0.8),
        ],
        total_available=5000,
        sample_scope="成交额前 3 只股票",
        computed_at="2026-07-13T12:00:00+00:00",
    )

    payload = build_market_payload(snapshot)

    assert payload["market"]["stockCount"] == 5000
    assert payload["market"]["sampleCount"] == 3
    assert payload["market"]["positivePeCount"] == 2
    assert payload["market"]["totalAmountYi"] == 11
    assert payload["market"]["upCount"] == 1
    assert payload["market"]["downCount"] == 1
    assert payload["stocks"][0]["code"] == "600000"
    assert payload["sectors"][0]["amountRatio"] == 45.45
    assert payload["inflowSectors"][0]["code"] == "BK2"
