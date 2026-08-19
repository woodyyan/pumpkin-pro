from data.industry_standardization import (
    build_a_share_mapping_rows,
    standardize_a_share_industry,
    standardize_not_applicable_industry,
)


def test_industry_standardization_maps_a_share_and_hk():
    assert standardize_a_share_industry("白酒") == {
        "industry_code": "food_beverage",
        "industry_name": "食品饮料",
        "industry_level": "l1",
        "industry_source": "sw_l1",
    }
    assert standardize_a_share_industry("货币金融服务") == {
        "industry_code": "banks",
        "industry_name": "银行",
        "industry_level": "l1",
        "industry_source": "sw_l1",
    }
    assert standardize_a_share_industry("电力、热力生产和供应业")["industry_name"] == "公用事业"
    assert standardize_a_share_industry("未知行业")["industry_name"] == ""
    assert standardize_not_applicable_industry() == {
        "industry_code": "not_applicable",
        "industry_name": "not_applicable",
        "industry_level": "not_applicable",
        "industry_source": "not_applicable",
    }


def test_build_a_share_mapping_rows_contains_source_industry_name():
    rows = build_a_share_mapping_rows()
    assert any(item["source_industry_name"] == "白酒" and item["industry_name"] == "食品饮料" for item in rows)
    assert any(item["source_industry_name"] == "货币金融服务" and item["industry_name"] == "银行" for item in rows)
