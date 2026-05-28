from __future__ import annotations

from typing import Any, Dict

NOT_APPLICABLE = "not_applicable"
STANDARD_LEVEL_L1 = "l1"
STANDARD_SOURCE_SW_L1 = "sw_l1"

STANDARD_NAME_TO_CODE = {
    "农林牧渔": "agriculture",
    "基础化工": "basic_chemicals",
    "钢铁": "steel",
    "有色金属": "nonferrous_metals",
    "电子": "electronics",
    "家用电器": "home_appliances",
    "食品饮料": "food_beverage",
    "纺织服饰": "textile_apparel",
    "轻工制造": "light_manufacturing",
    "医药生物": "pharmaceuticals_biotech",
    "公用事业": "utilities",
    "交通运输": "transportation",
    "房地产": "real_estate",
    "商贸零售": "retail_trade",
    "社会服务": "social_services",
    "综合": "conglomerates",
    "建筑材料": "building_materials",
    "建筑装饰": "building_decoration",
    "电力设备": "power_equipment",
    "国防军工": "defense_military",
    "计算机": "computer",
    "传媒": "media",
    "通信": "telecom",
    "银行": "banks",
    "非银金融": "non_bank_finance",
    "机械设备": "machinery",
    "汽车": "automobiles",
    "煤炭": "coal",
    "石油石化": "petrochemicals",
    "环保": "environmental_protection",
    NOT_APPLICABLE: NOT_APPLICABLE,
}

SOURCE_TO_STANDARD_NAME = {
    "IT服务": "计算机",
    "一般零售": "商贸零售",
    "专业工程": "建筑装饰",
    "专业服务": "社会服务",
    "专用设备": "机械设备",
    "中药": "医药生物",
    "互联网电商": "商贸零售",
    "元件": "电子",
    "光伏设备": "电力设备",
    "光学光电子": "电子",
    "其他电子": "电子",
    "其他电源设备": "电力设备",
    "军工电子": "国防军工",
    "农化制品": "基础化工",
    "出版": "传媒",
    "化学制品": "基础化工",
    "化学制药": "医药生物",
    "化学原料": "基础化工",
    "医疗器械": "医药生物",
    "医疗服务": "医药生物",
    "半导体": "电子",
    "塑料": "基础化工",
    "小金属": "有色金属",
    "工业金属": "有色金属",
    "工程机械": "机械设备",
    "房地产开发": "房地产",
    "旅游及景区": "社会服务",
    "服装家纺": "纺织服饰",
    "汽车服务": "汽车",
    "汽车零部件": "汽车",
    "消费电子": "电子",
    "炼化及贸易": "石油石化",
    "煤炭开采": "煤炭",
    "照明设备": "家用电器",
    "燃气": "公用事业",
    "特钢": "钢铁",
    "环保设备": "环保",
    "环境治理": "环保",
    "玻璃玻纤": "建筑材料",
    "电力": "公用事业",
    "电子化学品": "电子",
    "电机": "电力设备",
    "电池": "电力设备",
    "电网设备": "电力设备",
    "电视广播": "传媒",
    "白酒": "食品饮料",
    "自动化设备": "机械设备",
    "航空装备": "国防军工",
    "装修建材": "建筑材料",
    "装修装饰": "建筑装饰",
    "计算机设备": "计算机",
    "证券": "非银金融",
    "调味发酵品": "食品饮料",
    "贵金属": "有色金属",
    "软件开发": "计算机",
    "通信服务": "通信",
    "通信设备": "通信",
    "通用设备": "机械设备",
    "金属新材料": "有色金属",
    "非金属材料": "建筑材料",
    "饲料": "农林牧渔",
}


def standardize_a_share_industry(source_industry_name: Any) -> Dict[str, str]:
    source_name = str(source_industry_name or "").strip()
    standard_name = SOURCE_TO_STANDARD_NAME.get(source_name, "")
    if not standard_name:
        return {
            "industry_code": "",
            "industry_name": "",
            "industry_level": "",
            "industry_source": STANDARD_SOURCE_SW_L1,
        }
    return {
        "industry_code": STANDARD_NAME_TO_CODE[standard_name],
        "industry_name": standard_name,
        "industry_level": STANDARD_LEVEL_L1,
        "industry_source": STANDARD_SOURCE_SW_L1,
    }


def standardize_not_applicable_industry() -> Dict[str, str]:
    return {
        "industry_code": NOT_APPLICABLE,
        "industry_name": NOT_APPLICABLE,
        "industry_level": NOT_APPLICABLE,
        "industry_source": NOT_APPLICABLE,
    }


def build_a_share_mapping_rows() -> list[dict[str, str]]:
    rows: list[dict[str, str]] = []
    for source_industry_name in sorted(SOURCE_TO_STANDARD_NAME.keys()):
        standardized = standardize_a_share_industry(source_industry_name)
        rows.append({
            "source_industry_name": source_industry_name,
            "industry_code": standardized["industry_code"],
            "industry_name": standardized["industry_name"],
            "industry_level": standardized["industry_level"],
            "industry_source": standardized["industry_source"],
        })
    return rows
