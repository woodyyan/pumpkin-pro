import json

import data.news_kline as nk


def test_normalize_symbol_supports_a_share_and_hk_suffixes():
    sh = nk.normalize_symbol("600519.SH")
    assert sh.symbol == "600519.SH"
    assert sh.quote_code == "sh600519"
    assert sh.exchange == "SSE"

    hk = nk.normalize_symbol("00700.HK")
    assert hk.symbol == "00700.HK"
    assert hk.quote_code == "hk00700"
    assert hk.exchange == "HKEX"


def test_build_report_fetches_tencent_events_and_hides_empty_research_category(monkeypatch):
    nk._REPORT_CACHE.clear()

    def fake_http_get(url, **kwargs):
        if "fqkline" in url:
            return json.dumps({
                "data": {
                    "sh600519": {
                        "qfqday": [
                            ["2026-07-10", "100", "101", "102", "99", "1000"],
                            ["2026-07-13", "101", "103", "104", "100", "1100"],
                            ["2026-07-14", "103", "106", "107", "102", "1200"],
                            ["2026-07-15", "106", "105", "108", "104", "1300"],
                            ["2026-07-16", "105", "109", "110", "104", "1400"],
                        ]
                    }
                }
            })
        if "type=0" in url and "page=1" in url:
            return json.dumps({
                "data": {
                    "data": [
                        {
                            "id": "notice-1",
                            "title": "贵州茅台2026年半年度业绩预增公告",
                            "time": "2026-07-13 18:00:00",
                            "url": "https://example.com/notice",
                            "src": "交易所公告",
                            "typeStr": "公告",
                        }
                    ]
                }
            })
        if "type=2" in url and "page=1" in url:
            return json.dumps({
                "data": {
                    "data": [
                        {
                            "id": "news-1",
                            "title": "白酒行业市场需求回暖",
                            "time": "2026-07-12 09:00:00",
                            "url": "https://example.com/news",
                            "src": "财经媒体",
                            "typeStr": "新闻",
                        }
                    ]
                }
            })
        return json.dumps({"data": {"data": []}})

    monkeypatch.setattr(nk, "_http_get", fake_http_get)

    report = nk.get_news_kline_report("600519.SH", days=5, pages=2, force=True)

    assert report["META"]["symbol"] == "600519.SH"
    assert report["META"]["adjustment"] == "qfq"
    assert report["META"]["cache_ttl_seconds"] == 1800
    assert len(report["KLINE"]) == 5
    assert len(report["EVENTS"]) == 2
    assert "研报评级" not in report["CATS"]
    assert {event["category"] for event in report["EVENTS"]} == {"财报业绩", "行业市场"}

    weekend_event = next(event for event in report["EVENTS"] if event["id"] == "news-1")
    assert weekend_event["date"] == "2026-07-12"
    assert weekend_event["trade_date"] == "2026-07-13"
    assert weekend_event["date_note"] == "non_trading_day_mapped_to_next_trade_date"

    top_categories = [row["category"] for row in report["STATS"]]
    assert "财报业绩" in top_categories
    assert "行业市场" in top_categories


def test_get_news_kline_report_returns_stale_cache_on_refresh_failure(monkeypatch):
    nk._REPORT_CACHE.clear()

    def ok_http_get(url, **kwargs):
        if "fqkline" in url:
            return json.dumps({
                "data": {
                    "hk00700": {
                        "qfqday": [
                            ["2026-07-14", "500", "510", "512", "498", "1000"],
                            ["2026-07-15", "510", "508", "515", "505", "1100"],
                            ["2026-07-16", "508", "520", "522", "507", "1200"],
                        ]
                    }
                }
            })
        return json.dumps({"data": {"data": []}})

    monkeypatch.setattr(nk, "_http_get", ok_http_get)
    first = nk.get_news_kline_report("00700.HK", days=3, pages=1, force=True)
    assert first["META"]["cache_status"] == "fresh"

    def bad_http_get(url, **kwargs):
        raise RuntimeError("source down")

    monkeypatch.setattr(nk, "_http_get", bad_http_get)
    stale = nk.get_news_kline_report("00700.HK", days=3, pages=1, force=True)
    assert stale["META"]["cache_status"] == "stale"
    assert "source down" in stale["META"]["last_error"]
