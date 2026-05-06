import json

from data.company_profile import (
    build_business_summary,
    extract_core_business_phrases,
    normalize_date,
    normalize_industry_name,
    normalize_symbol,
    normalize_website,
)
from scripts.sync_company_profiles import parse_symbols, sync_symbols


def test_normalize_industry_name_removes_source_noise_and_suffix():
    assert normalize_industry_name("食品饮料Ⅰ") == "食品饮料"
    assert normalize_industry_name("半导体Ⅱ") == "半导体"
    assert normalize_industry_name("软件开发(东财行业)") == "软件开发"
    assert normalize_industry_name("银行（申万）") == "银行"
    assert normalize_industry_name("信息技术服务-软件开发") == "信息技术服务"
    assert normalize_industry_name("--") == ""


def test_business_summary_extracts_without_creating_facts():
    summary, source, flags = build_business_summary({
        "name": "贵州茅台",
        "main_business": "主要从事茅台酒及系列酒的生产与销售。",
        "industry_name": "食品饮料",
    })
    assert summary == "贵州茅台主要从事茅台酒及系列酒的生产与销售，所属行业为食品饮料。"
    assert source == "source_extract"
    assert flags == []


def test_business_summary_from_scope_filters_legal_noise():
    summary, source, flags = build_business_summary({
        "name": "中芯国际",
        "business_scope": "一般项目：集成电路制造；集成电路设计；半导体器件专用设备制造；依法须经批准的项目，经相关部门批准后方可开展经营活动。",
    })
    assert "依法须经批准" not in summary
    assert "集成电路制造" in summary
    assert "集成电路设计" in summary
    assert source == "rule_template"
    assert flags == []


def test_business_summary_fallback_does_not_guess():
    summary, source, flags = build_business_summary({"name": "招商银行", "industry_name": "银行", "board_name": "主板"})
    assert summary == "招商银行属于银行行业，在主板上市交易，具体主营业务资料暂待补全。"
    assert source == "fallback"
    assert flags == ["summary_fallback"]


def test_business_summary_removes_promotional_terms():
    summary, _, _ = build_business_summary({
        "name": "示例公司",
        "main_business": "主要从事行业龙头产品、最强平台、有望受益业务。",
    })
    assert "龙头" not in summary
    assert "最强" not in summary
    assert "有望" not in summary


def test_extract_core_business_phrases_filters_generic_terms():
    phrases = extract_core_business_phrases("主要业务包括服务、咨询、集成电路制造、集成电路设计。")
    assert phrases == ["集成电路制造", "集成电路设计"]


def test_normalize_symbol_website_and_date():
    assert normalize_symbol("700") == ("00700.HK", "HKEX", "00700")
    assert normalize_symbol("600519") == ("600519.SH", "SSE", "600519")
    assert normalize_website("www.example.com") == "https://www.example.com"
    assert normalize_website("javascript:alert(1)") == ""
    assert normalize_date("1999") == ("1999-01-01", "year")
    assert normalize_date("19991120") == ("1999-11-20", "day")


def test_sync_symbols_uses_fetch_profile(monkeypatch):
    import scripts.sync_company_profiles as sync_mod

    monkeypatch.setattr(sync_mod, "fetch_profile", lambda symbol: {"symbol": symbol})
    assert parse_symbols("600519.SH, 00700.HK") == ["600519.SH", "00700.HK"]
    assert sync_symbols(["600519.SH", "00700.HK"], limit=1) == [{"symbol": "600519.SH"}]
