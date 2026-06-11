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

# 统一维护“外部源行业名 -> 申万一级行业”的别名表。
# 这里同时覆盖：
# 1. 申万一级行业自身名称
# 2. 东方财富/AKShare 的常见行业别名
# 3. BaoStock 返回的证监会行业分类名称
A_SHARE_INDUSTRY_ALIASES = {
    "农林牧渔": [
        "农林牧渔",
        "农业",
        "林业",
        "畜牧业",
        "渔业",
        "农、林、牧、渔专业及辅助性活动",
        "饲料",
    ],
    "基础化工": [
        "基础化工",
        "农化制品",
        "化学制品",
        "化学原料",
        "化学原料和化学制品制造业",
        "化学纤维制造业",
        "橡胶和塑料制品业",
        "塑料",
    ],
    "钢铁": [
        "钢铁",
        "特钢",
        "黑色金属冶炼和压延加工业",
        "黑色金属矿采选业",
    ],
    "有色金属": [
        "有色金属",
        "小金属",
        "工业金属",
        "贵金属",
        "金属新材料",
        "有色金属冶炼和压延加工业",
        "有色金属矿采选业",
    ],
    "电子": [
        "电子",
        "元件",
        "光学光电子",
        "其他电子",
        "半导体",
        "消费电子",
        "电子化学品",
        "计算机、通信和其他电子设备制造业",
    ],
    "家用电器": [
        "家用电器",
        "照明设备",
    ],
    "食品饮料": [
        "食品饮料",
        "白酒",
        "调味发酵品",
        "酒、饮料和精制茶制造业",
        "食品制造业",
        "农副食品加工业",
    ],
    "纺织服饰": [
        "纺织服饰",
        "服装家纺",
        "纺织业",
        "纺织服装、服饰业",
        "皮革、毛皮、羽毛及其制品和制鞋业",
    ],
    "轻工制造": [
        "轻工制造",
        "造纸和纸制品业",
        "木材加工和木、竹、藤、棕、草制品业",
        "家具制造业",
        "印刷和记录媒介复制业",
        "文教、工美、体育和娱乐用品制造业",
        "其他制造业",
    ],
    "医药生物": [
        "医药生物",
        "中药",
        "化学制药",
        "医疗器械",
        "医疗服务",
        "医药制造业",
        "卫生",
        "研究和试验发展",
    ],
    "公用事业": [
        "公用事业",
        "燃气",
        "电力",
        "电力、热力生产和供应业",
        "燃气生产和供应业",
    ],
    "交通运输": [
        "交通运输",
        "航空运输业",
        "道路运输业",
        "水上运输业",
        "铁路运输业",
        "多式联运和运输代理业",
        "装卸搬运和仓储业",
        "邮政业",
    ],
    "房地产": [
        "房地产",
        "房地产开发",
        "房地产业",
    ],
    "商贸零售": [
        "商贸零售",
        "一般零售",
        "互联网电商",
        "批发业",
        "零售业",
    ],
    "社会服务": [
        "社会服务",
        "专业服务",
        "专业技术服务业",
        "商务服务业",
        "科技推广和应用服务业",
        "住宿业",
        "餐饮业",
        "教育",
        "体育",
        "文化艺术业",
        "公共设施管理业",
        "旅游及景区",
        "机动车、电子产品和日用产品修理业",
    ],
    "综合": [
        "综合",
    ],
    "建筑材料": [
        "建筑材料",
        "玻璃玻纤",
        "非金属材料",
        "非金属矿物制品业",
        "非金属矿采选业",
    ],
    "建筑装饰": [
        "建筑装饰",
        "专业工程",
        "装修装饰",
        "房屋建筑业",
        "土木工程建筑业",
        "建筑安装业",
        "建筑装饰、装修和其他建筑业",
    ],
    "电力设备": [
        "电力设备",
        "光伏设备",
        "其他电源设备",
        "电机",
        "电池",
        "电网设备",
        "电气机械和器材制造业",
    ],
    "国防军工": [
        "国防军工",
        "军工电子",
        "航空装备",
        "铁路、船舶、航空航天和其他运输设备制造业",
    ],
    "计算机": [
        "计算机",
        "IT服务",
        "计算机设备",
        "软件开发",
        "软件和信息技术服务业",
    ],
    "传媒": [
        "传媒",
        "出版",
        "电视广播",
        "互联网和相关服务",
        "新闻和出版业",
        "广播、电视、电影和录音制作业",
    ],
    "通信": [
        "通信",
        "通信服务",
        "通信设备",
        "电信、广播电视和卫星传输服务",
    ],
    "银行": [
        "银行",
        "货币金融服务",
    ],
    "非银金融": [
        "非银金融",
        "证券",
        "保险业",
        "资本市场服务",
        "其他金融业",
        "租赁业",
    ],
    "机械设备": [
        "机械设备",
        "专用设备",
        "工程机械",
        "自动化设备",
        "通用设备",
        "仪器仪表制造业",
        "专用设备制造业",
        "通用设备制造业",
        "金属制品业",
        "金属制品、机械和设备修理业",
    ],
    "汽车": [
        "汽车",
        "汽车服务",
        "汽车零部件",
        "汽车制造业",
    ],
    "煤炭": [
        "煤炭",
        "煤炭开采",
        "煤炭开采和洗选业",
    ],
    "石油石化": [
        "石油石化",
        "炼化及贸易",
        "石油和天然气开采业",
        "石油、煤炭及其他燃料加工业",
        "开采专业及辅助性活动",
    ],
    "环保": [
        "环保",
        "环保设备",
        "环境治理",
        "生态保护和环境治理业",
        "水的生产和供应业",
        "水利管理业",
        "废弃资源综合利用业",
    ],
}


def _build_source_to_standard_name() -> Dict[str, str]:
    mapping: Dict[str, str] = {NOT_APPLICABLE: NOT_APPLICABLE}
    for standard_name, aliases in A_SHARE_INDUSTRY_ALIASES.items():
        if standard_name not in STANDARD_NAME_TO_CODE:
            raise KeyError(f"unknown standard industry name: {standard_name}")
        for alias in aliases:
            normalized = str(alias or "").strip()
            if not normalized:
                continue
            existing = mapping.get(normalized)
            if existing and existing != standard_name:
                raise ValueError(f"duplicate alias mapping: {normalized} -> {existing}, {standard_name}")
            mapping[normalized] = standard_name
    return mapping


SOURCE_TO_STANDARD_NAME = _build_source_to_standard_name()


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
        if source_industry_name == NOT_APPLICABLE:
            continue
        standardized = standardize_a_share_industry(source_industry_name)
        rows.append({
            "source_industry_name": source_industry_name,
            "industry_code": standardized["industry_code"],
            "industry_name": standardized["industry_name"],
            "industry_level": standardized["industry_level"],
            "industry_source": standardized["industry_source"],
        })
    return rows
