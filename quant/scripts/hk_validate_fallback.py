"""
补充验证：用腾讯财经接口获取港股快照 + 恒生指数
（绕过东财快照的连接限制）
"""
import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import time
import json
import requests


_QQ_HEADERS = {
    "User-Agent": (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "
        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
    ),
    "Referer": "https://stockapp.finance.qq.com/",
}


def test_tencent_hk_snapshot():
    """用腾讯财经接口拉取港股列表（分批）。
    
    港股代码格式: hk00xxx (5位数字)
    主板代码范围: 00001-99999，实际活跃约 2000+ 只
    
    策略：先取已知热门港股验证格式正确性，
    再确认可以批量拉取。
    """
    print("=" * 60)
    print("  腾讯财经 — 港股行情接口验证")
    print("=" * 60)

    # 用一批已知港股代码测试
    test_codes = [
        "00700",  # 腾讯控股
        "09988",  # 阿里巴巴-W
        "03690",  # 美团-W
        "06618",  # 京东健康
        "09888",  # 银河娱乐
        "00005",  # 汇丰控股
        "13980",  # 美图公司
        "01751",  # 小米-W
        "02411",  # 比亚迪股份
        "00941",  # 中国移动
    ]

    qq_codes = [f"hk{c}" for c in test_codes]
    url = f"http://qt.gtimg.cn/q={','.join(qq_codes)}"
    
    print(f"\n  测试单批请求 ({len(test_codes)} 只)...")
    try:
        resp = requests.get(url, headers=_QQ_HEADERS, timeout=15)
        resp.encoding = "gbk"
        
        success = 0
        fail = 0
        results = []
        
        for line in resp.text.strip().split("\n"):
            if "~" not in line or "=" not in line:
                continue
            body = line.split("=", 1)[1].strip().strip('"').strip(";")
            parts = body.split("~")
            if len(parts) < 40:
                fail += 1
                continue
            
            code_raw = str(parts[2]).zfill(5) if len(parts[2]) <= 5 else parts[2]
            name = parts[1]
            price_str = parts[3]
            
            try:
                price = float(price_str) if price_str else None
            except (ValueError, TypeError):
                price = None
            
            status = "✅" if price and price > 0 else "⚠️"
            if price and price > 0:
                success += 1
            else:
                fail += 1
            
            pe_str = parts[39] if len(parts) > 39 else ""
            mv_str = parts[39] if len(parts) > 44 else ""  # 总市值在 45
            
            results.append({
                "code": f"{code_raw}.HK",
                "name": name,
                "price": f"{price:.2f}" if price else "N/A",
                "pe": pe_str or "N/A",
            })
            print(f"  {status} {code_raw}  {name:<10s} 价格={price:>8s}  PE={pe_str or 'N/A':>8s}")

        print(f"\n  批量结果: 成功 {success}/{len(test_codes)}")
        
        # 验证关键字段可用性
        print(f"  关键字段检查:")
        if results and any(r["pe"] != "N/A" for r in results):
            pe_valid = sum(1 for r in results if r["pe"] != "N/A")
            print(f"    PE 可用: {pe_valid}/{len(results)}")
        else:
            print(f"    PE 字段可能为空（腾讯接口对部分股票不返回PE）")

        return success >= len(test_codes) * 0.7  # 70% 成功率即可

    except Exception as exc:
        print(f"  ❌ 请求失败: {exc}")
        return False


def test_hsi_benchmark():
    """测试多种恒生指数数据源。"""
    print("\n" + "=" * 60)
    print("  基准指数 — 多源验证")
    print("=" * 60)

    sources_ok = []

    # 方法1：腾讯财经 K线接口（尝试不同代码格式）
    hsi_variants = [
        ("hkHSI", "恒生指数(原格式)"),
        ("hk00001.HKI", "恒生指数(HKI后缀)"),
        ("hkHSI.HKI", "恒生指数(HSI.HKI)"),
    ]
    
    print("\n  [尝试] 腾讯财经 K线 接口:")
    import pandas as pd
    end_date = pd.Timestamp.now().strftime("%Y-%m-%d")
    start_date = (pd.Timestamp.now() - pd.Timedelta(days=180)).strftime("%Y-%m-%d")

    for code, name in hsi_variants:
        try:
            url = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
            params = {"param": f"{code},day,{start_date},{end_date},180"}
            resp = requests.get(url, headers=_QQ_HEADERS, timeout=15)
            resp.raise_for_status()
            data = resp.json()
            idx_data = data.get("data", {}).get(code, {})
            klines = idx_data.get("day") or idx_data.get("qfqday") or []
            
            if klines:
                closes = [float(k[2]) for k in klines if len(k) >= 3]
                closes = [c for c in closes if c > 0]
                if len(closes) >= 2:
                    lb = min(60, len(closes))
                    ret = (closes[-1] / closes[-lb] - 1) * 100
                    print(f"    ✅ {name}: K线={len(klines)}, 最新={closes[-1]:.2f}, 60d收益={ret:+.2f}%")
                    sources_ok.append(name)
                else:
                    print(f"    ⚠️ {name}: 有K线但无有效价格")
            else:
                print(f"    ❌ {name}: 空 klines (keys={list(data.get('data', {}).keys())[:5]})")
        except Exception as e:
            print(f"    ❌ {name}: {e}")

    # 方法2：直接用腾讯实时行情接口获取恒生指数点位
    print("\n  [尝试] 腾讯实时行情接口:")
    hsi_realtime_codes = ["hkHSI", "hkHSCEI"]
    for code in hsi_realtime_codes:
        try:
            url = f"http://qt.gtimg.cn/q={code}"
            resp = requests.get(url, headers=_QQ_HEADERS, timeout=10)
            resp.encoding = "gbk"
            body = resp.text.strip()
            if "=" in body:
                content = body.split("=", 1)[1].strip().strip('"').strip(";")
                parts = content.split("~")
                if len(parts) > 3:
                    name = parts[1]
                    last_price = parts[3]
                    change_pct = parts[32] if len(parts) > 32 else "?"
                    print(f"    ✅ {code}: {name} 现价={last_price} 涨跌幅={change_pct}")
                    sources_ok.append(f"{code}(realtime)")
                else:
                    print(f"    ⚠️ {code}: 格式异常 (parts={len(parts)})")
            else:
                print(f"    ❌ {code}: 无响应内容")
        except Exception as e:
            print(f"    ❌ {code}: {e}")

    # 方法3：Yahoo Finance 作为兜底
    print("\n  [尝试] Yahoo Finance 兜底:")
    try:
        yfinance_available = True
        import yfinance as yf
        hsi = yf.Ticker("^HSI")
        hist = hsi.history(period="3mo")
        if hist is not None and len(hist) >= 20:
            closes = hist["Close"].dropna()
            lb = min(60, len(closes))
            ret = (closes.iloc[-1] / closes.iloc[-lb] - 1) * 100
            print(f"    ✅ ^HSI (Yahoo): 数据={len(hist)}天, 最新={closes.iloc[-1]:.2f}, 60d收益={ret:+.2f}%")
            sources_ok.append("Yahoo Finance ^HSI")
        else:
            print(f"    ⚠️ Yahoo: 数据不足")
            yfinance_available = False
    except ImportError:
        print(f"    ⏭️ yfinance 未安装，跳过")
        yfinance_available = False
    except Exception as e:
        print(f"    ⚠️ Yahoo: {e}")
        yfinance_available = False

    print(f"\n  可用数据源: {len(sources_ok)} 个 → {sources_ok}")
    return len(sources_ok) >= 1


if __name__ == "__main__":
    ok1 = test_tencent_hk_snapshot()
    ok2 = test_hsi_benchmark()
    
    print("\n" + "=" * 60)
    if ok1 and ok2:
        print("  ✅ 补充验证全部通过！")
    elif ok2:
        print("  ⚠️ 快照需优化，但基准指数有可用替代方案")
    elif ok1:
        print("  ⚠️ 基准指数需进一步排查，但快照可用")
    else:
        print("  ❌ 补充验证未通过，需要网络调试")
    print("=" * 60)
