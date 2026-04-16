"""Import 完整性测试 — 防止 main.py 调用未导入的名称（NameError 生产事故）。

触发条件：2026-04-12 港股四象限调用报错
  name 'compute_hk_quadrant_scores' is not defined
根因：main.py:30 的 import 遗漏了 compute_hk_quadrant_scores

策略：静态扫描 — 提取 screener.quadrant 模块中所有被调用的函数名，
      与 import 语句做集合差，发现遗漏即 fail。
"""

import ast
import importlib
import inspect
import re

import pytest


def _extract_imported_from_quadrant(main_source):
    """提取 main.py 中 'from screener.quadrant import ...' 导入的所有名称。"""
    tree = ast.parse(main_source)
    names = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom):
            if node.module == "screener.quadrant":
                for alias in node.names:
                    names.add(alias.name)
    return names


def _extract_quadrant_calls_in_main(main_source):
    """提取 main.py 中所有来自 quadrant 模块的函数调用名。"""
    # 已知的 quadrant 公开 API 名称列表（来自 quadrant.py 顶层 def）
    known_apis = {
        "compute_all_quadrant_scores",
        "compute_hk_quadrant_scores",
        "get_cached_scores",
        "get_hk_cached_scores",   # 如果存在
    }
    tree = ast.parse(main_source)
    called = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Name):
            name = node.func.id
            if name in known_apis:
                called.add(name)
    return called


def _get_quadrant_module_public_names():
    """返回 screener.quadrant 模块中所有顶层可调用对象名（不含 _ 前缀）。
    
    需要 requests 等运行时依赖，缺失时跳过相关测试。
    """
    pytest.importorskip("requests", reason="screener.quadrant 依赖 requests")
    from screener import quadrant as qm
    public = set()
    for name, obj in inspect.getmembers(qm):
        if not name.startswith("_") and callable(obj):
            public.add(name)
    return public


class TestQuadrantImportCompleteness:
    """确保 main.py 对 screener.quadrant 的导入完整无遗漏。"""

    def test_all_called_functions_are_imported(self):
        """main.py 中调用的每个 quadrant 函数都必须在 import 中出现。"""
        _MAIN_PATH = "main.py"
        with open(_MAIN_PATH, encoding="utf-8") as f:
            main_source = f.read()

        imported = _extract_imported_from_quadrant(main_source)
        called = _extract_quadrant_calls_in_main(main_source)

        missing = called - imported
        assert not missing, (
            f"以下 quadrant 函数在 main.py 中被调用但未导入（将导致 NameError）：\n"
            f"  {sorted(missing)}\n"
            f"请修改 main.py 第 ~30 行的导入语句，补充这些名称。"
        )

    def test_imported_names_actually_exist_in_quadrant_module(self):
        """import 中的每个名称必须确实存在于 screener.quadrant 模块中。
        
        注意：允许导入以 _ 开头的私有 API（动态调用场景）。
        """
        _MAIN_PATH = "main.py"
        with open(_MAIN_PATH, encoding="utf-8") as f:
            main_source = f.read()

        imported = _extract_imported_from_quadrant(main_source)
        actual_public = _get_quadrant_module_public_names()

        # 只检查公开名称；私有名称（_ 前缀）允许动态导入
        public_imported = {n for n in imported if not n.startswith("_")}
        nonexistent = public_imported - actual_public
        assert not nonexistent, (
            f"以下名称在 main.py 中从 screener.quadrant 导入，但模块中不存在：\n"
            f"  {sorted(nonexistent)}\n"
            f"可能是拼写错误或该函数已被重命名/删除。"
        )

    def test_no_stale_imports(self):
        """import 中没有未被使用的 quadrant 名称（避免死导入堆积）。
        
        注意：以 _ 开头的私有 API 在异常处理等处动态调用，
        不属于「stale」范围。
        """
        _MAIN_PATH = "main.py"
        with open(_MAIN_PATH, encoding="utf-8") as f:
            main_source = f.read()

        imported = _extract_imported_from_quadrant(main_source)
        called = _extract_quadrant_calls_in_main(main_source)

        # 排除私有 API（_ 前缀）
        public_imported = {n for n in imported if not n.startswith("_")}
        stale = public_imported - called
        assert not stale, (
            f"以下 quadrant 名称已导入但从未使用（建议清理）：\n"
            f"  {sorted(stale)}"
        )
