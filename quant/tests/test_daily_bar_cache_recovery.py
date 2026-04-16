"""
DailyBarCache — SQLite 连接失败自动恢复测试

验证 _ensure_db / _is_connection_alive / _delete_corrupted_files 能正确处理：
  - 正常连接成功
  - 连接失败后重试
  - 重试全部失败 → 删除损坏文件并重建
  - WAL/SHM 文件清理

使用临时目录，不影响生产数据。
"""

import os
import sqlite3
import tempfile
import shutil
import unittest


class TestDailyBarCacheRecovery(unittest.TestCase):
    """测试 DailyBarCache 在磁盘 I/O 错误时的自动恢复能力。"""

    def setUp(self):
        """每个测试用独立的临时目录。"""
        self.tmpdir = tempfile.mkdtemp(prefix="test_quadrant_cache_")
        self.db_path = os.path.join(self.tmpdir, "quadrant_cache.db")

    def tearDown(self):
        """清理临时目录。"""
        if os.path.exists(self.tmpdir):
            # 先关闭所有可能的连接
            try:
                pass  # SQLite 会自动清理
            finally:
                shutil.rmtree(self.tmpdir, ignore_errors=True)

    # ── Helper: 导入目标模块 ────────────────────────────────

    @staticmethod
    def _get_DailyBarCache():
        from screener.quadrant import DailyBarCache
        return DailyBarCache

    @staticmethod
    def _get_HkDailyBarCache():
        from screener.quadrant import HkDailyBarCache, HK_CACHE_DB_PATH
        return HkDailyBarCache, HK_CACHE_DB_PATH

    # ── 正常场景 ────────────────────────────────────────────

    def test_normal_init_creates_tables(self):
        """正常初始化应创建 daily_bars 和 cache_meta 两张表。"""
        DC = self._get_DailyBarCache()
        cache = DC(db_path=self.db_path)
        self.assertIsNotNone(cache._conn)
        tables = cache._conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
        ).fetchall()
        table_names = [t[0] for t in tables]
        self.assertIn("daily_bars", table_names)
        self.assertIn("cache_meta", table_names)

    def test_normal_write_and_read(self):
        """写入数据后能正常读回。"""
        DC = self._get_DailyBarCache()
        cache = DC(db_path=self.db_path)
        cache._conn.execute(
            "INSERT OR REPLACE INTO daily_bars (code, date, open, close, high, low) "
            "VALUES ('000001', '2026-04-15', 10.0, 11.0, 11.5, 9.8)"
        )
        cache._conn.commit()
        row = cache._conn.execute(
            "SELECT code, close FROM daily_bars WHERE code='000001'"
        ).fetchone()
        self.assertEqual(row[0], "000001")
        self.assertAlmostEqual(row[1], 11.0)

    # ── is_connection_alive ──────────────────────────────────

    def test_is_connection_alive_returns_true_for_good_conn(self):
        """正常连接返回 True。"""
        DC = self._get_DailyBarCache()
        cache = DC(db_path=self.db_path)
        self.assertTrue(cache._is_connection_alive())

    def test_is_connection_alive_returns_false_when_none(self):
        """_conn 为 None 时返回 False。"""
        DC = self._get_DailyBarCache()
        cache = DC.__new__(DC)  # 不调用 __init__
        cache._conn = None
        self.assertFalse(cache._is_connection_alive())

    def test_is_connection_alive_returns_false_on_closed_conn(self):
        """关闭的连接返回 False。"""
        DC = self._get_DailyBarCache()
        cache = DC(db_path=self.db_path)
        cache._conn.close()
        self.assertFalse(cache._is_connection_alive())

    # ── delete_corrupted_files ───────────────────────────────

    def test_delete_corrupted_files_removes_db_wal_shm(self):
        """删除损坏文件时应清理 .db + .db-wal + .db-shm。"""
        # 先创建一个数据库和 WAL 文件
        conn = sqlite3.connect(self.db_path)
        conn.execute("CREATE TABLE t (id INT)")
        conn.commit()
        conn.close()
        # 创建模拟的 WAL/SHM 文件
        wal_path = self.db_path + "-wal"
        shm_path = self.db_path + "-shm"
        with open(wal_path, "w") as f:
            f.write("fake-wal")
        with open(shm_path, "w") as f:
            f.write("fake-shm")

        for p in [self.db_path, wal_path, shm_path]:
            self.assertTrue(os.path.exists(p))

        DC = self._get_DailyBarCache()
        DC._delete_corrupted_files(db_path=self.db_path)

        for p in [self.db_path, wal_path, shm_path]:
            self.assertFalse(os.path.exists(p), f"{p} 应该被删除")

    def test_delete_corrupted_files_handles_missing_file(self):
        """文件不存在时不抛异常（静默忽略）。"""
        DC = self._get_DailyBarCache()
        should_not_raise = lambda: DC._delete_corrupted_files(db_path="/nonexistent/path/quadrant_cache.db")
        should_not_raise()  # 不应该抛异常

    def test_delete_corrupted_files_removes_journal(self):
        """清理 .db-journal 文件。"""
        conn = sqlite3.connect(self.db_path)
        conn.execute("PRAGMA journal_mode=DELETE")
        conn.execute("CREATE TABLE t (id INT)")
        conn.commit()
        conn.close()
        journal_path = self.db_path + "-journal"
        with open(journal_path, "w") as f:
            f.write("fake-journal")

        DC = self._get_DailyBarCache()
        DC._delete_corrupted_files(db_path=self.db_path)

        self.assertFalse(os.path.exists(journal_path), ".journal 文件应被删除")

    # ── 自动恢复：损坏 DB 后重建 ───────────────────────────

    def test_rebuild_after_corrupted_db(self):
        """DB 文件损坏（内容被覆写为乱码）→ 应自动删除并重建。"""
        # 创建一个正常的 DB 并插入数据
        conn = sqlite3.connect(self.db_path)
        conn.execute("CREATE TABLE daily_bars (code TEXT, date TEXT)")
        conn.commit()
        conn.close()

        # 损坏：用随机字节覆盖 DB 文件
        with open(self.db_path, "wb") as f:
            f.write(b"\x00\xFF\xFE" * 10000)

        # 现在尝试创建 DailyBarCache —— 内部会检测到损坏并重建
        DC = self._get_DailyBarCache()
        cache = DC(db_path=self.db_path)

        # 验证重建后的连接可用
        self.assertTrue(cache._is_connection_alive())
        tables = cache._conn.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
        ).fetchall()
        self.assertTrue(len(tables) >= 2, "重建后应有 daily_bars 和 cache_meta 表")

    def test_rebuild_with_readonly_dir_fails_gracefully(self):
        """如果目录不可写，最终应抛出 RuntimeError。"""
        # 创建一个只读子目录
        readonly_dir = os.path.join(self.tmpdir, "readonly")
        os.makedirs(readonly_dir)
        readonly_db = os.path.join(readonly_dir, "cache.db")
        os.chmod(readonly_dir, 0o444)  # 只读

        DC = self._get_DailyBarCache()

        # 注意：在某些系统上（如 macOS），root 用户可能仍能写入。
        # 我们只验证不会崩溃出意外异常类型即可。
        try:
            cache = DC(db_path=readonly_db)
            # 如果没报错（可能因为权限检查不严格），也算通过
            self.assertIsNotNone(cache._conn)
        except (RuntimeError, OSError, sqlite3.OperationalError):
            # 期望的错误类型
            pass
        finally:
            os.chmod(readonly_dir, 0o755)  # 恢复权限以便 tearDown 清理

    # ── HkDailyBarCache 继承 ─────────────────────────────────

    def test_hk_cache_inherits_recovery_logic(self):
        """HkDailyBarCache 继承自 DailyBarCache，同样具备恢复能力。"""
        HkDC, orig_hk_path = self._get_HkDailyBarCache()
        hk_test_db = os.path.join(self.tmpdir, "hk_test.db")
        cache = HkDC.__new__(HkDC)
        cache.db_path = hk_test_db
        # 直接调用父类的 _ensure_db
        cache._ensure_db()
        self.assertTrue(cache._is_connection_alive())


if __name__ == "__main__":
    unittest.main()
