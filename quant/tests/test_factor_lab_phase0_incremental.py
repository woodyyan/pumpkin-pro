from __future__ import annotations

import importlib.util
import os
import signal
import sqlite3
import subprocess
import sys
import threading
import time
from pathlib import Path

import pytest

SCRIPT_PATH = Path(__file__).resolve().parents[1] / "scripts" / "update_factor_lab_phase0_incremental.py"
spec = importlib.util.spec_from_file_location("update_factor_lab_phase0_incremental", SCRIPT_PATH)
module = importlib.util.module_from_spec(spec)
assert spec.loader is not None
sys.modules[spec.name] = module
spec.loader.exec_module(module)


def test_split_csv_trims_empty_items():
    assert module.split_csv("securities, daily-bars,,financials ") == ["securities", "daily-bars", "financials"]

def test_default_modes_exclude_dividends_but_supported_modes_keep_manual_refresh():
    assert module.DEFAULT_MODES == ("securities", "industries", "daily-bars", "index-bars", "financials")
    assert "dividends" not in module.DEFAULT_MODES
    assert "dividends" in module.SUPPORTED_MODES


def test_buffered_start_date_uses_yyyymmdd_format():
    assert module.buffered_start_date("2026-05-20", 7) == "20260513"
    assert module.buffered_start_date("", 7) == ""


def test_latest_date_returns_empty_for_missing_table(tmp_path):
    conn = sqlite3.connect(tmp_path / "factor.db")
    try:
        assert module.latest_date(conn, "missing_table", "trade_date") == ""
    finally:
        conn.close()


def test_build_daily_bars_command_uses_latest_date_buffer(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_daily_bars (code TEXT, trade_date TEXT)")
        conn.execute("CREATE TABLE factor_market_metrics (code TEXT, trade_date TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,0,'MAIN')")
        conn.execute("INSERT INTO factor_daily_bars VALUES ('000001', '2026-05-20')")
        conn.execute("INSERT INTO factor_market_metrics VALUES ('000001', '2026-05-20'),('000002','2026-05-20')")
        args = module.parse_args(["--db", str(db_path), "--write", "--buffer-days", "5", "--progress-interval", "123", "--item-progress-interval", "1", "--daily-bars-source", "tencent"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "daily-bars", conn)
        assert "--write" in cmd
        assert "--start-date" in cmd
        assert cmd[cmd.index("--start-date") + 1] == "20260515"
        assert cmd[cmd.index("--progress-interval") + 1] == "1"
        assert cmd[cmd.index("--daily-bars-source") + 1] == "tencent"
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip() == "000002"
    finally:
        conn.close()


def test_build_index_bars_command_falls_back_to_lookback(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        args = module.parse_args(["--db", str(db_path), "--lookback-days", "600"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "index-bars", conn)
        assert "--lookback-days" in cmd
        assert cmd[cmd.index("--lookback-days") + 1] == "600"
    finally:
        conn.close()


def test_build_financial_and_dividend_commands_use_small_report_limits(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_financial_metrics (code TEXT, report_period TEXT)")
        conn.execute("CREATE TABLE factor_dividend_records (code TEXT, updated_at TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN')")
        args = module.parse_args(["--db", str(db_path), "--financial-report-limit", "2", "--dividend-report-limit", "3"])
        args.db_path = db_path
        args.temp_files = []
        financial_cmd = module.build_mode_command(args, "financials", conn)
        dividend_cmd = module.build_mode_command(args, "dividends", conn)
        assert financial_cmd[financial_cmd.index("--report-limit") + 1] == "2"
        assert dividend_cmd[dividend_cmd.index("--report-limit") + 1] == "3"
    finally:
        conn.close()


def test_build_industries_command_adds_mode_without_code_list(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        args = module.parse_args(["--db", str(db_path), "--industries-source", "eastmoney"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "industries", conn)
        assert "--mode" in cmd
        assert cmd[cmd.index("--mode") + 1] == "industries"
        assert "--industries-source" in cmd
        assert cmd[cmd.index("--industries-source") + 1] == "eastmoney"
        assert "--code-list-file" not in cmd
        assert "--daily-bars-source" not in cmd
    finally:
        conn.close()


def test_repair_missing_dividend_yield_selects_old_dividend_records(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_dividend_records (code TEXT, dividend_yield REAL, cash_dividend_per_share REAL, raw_plan TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,0,'MAIN')")
        conn.execute("INSERT INTO factor_dividend_records VALUES ('000001',NULL,NULL,''),('000002',0.02,NULL,'10派2元')")
        args = module.parse_args(["--db", str(db_path), "--scope", "repair_missing_dividend_yield"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "dividends", conn)
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip() == "000001"
    finally:
        conn.close()


def test_repair_missing_fcfm_inputs_selects_missing_capex_rows(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute(
            "CREATE TABLE factor_financial_metrics (code TEXT, report_period TEXT, revenue REAL, revenue_yoy REAL, net_profit_yoy REAL, operating_cash_flow REAL, capex REAL)"
        )
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,0,'MAIN')")
        conn.execute(
            "INSERT INTO factor_financial_metrics VALUES ('000001','2026-03-31',100,12,15,50,NULL),('000002','2026-03-31',100,12,15,50,20)"
        )
        args = module.parse_args(["--db", str(db_path), "--scope", "repair_missing_fcfm_inputs"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "financials", conn)
        assert "--require-fcfm-inputs" in cmd
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip() == "000001"
    finally:
        conn.close()


def test_repair_missing_fcfm_inputs_selects_missing_growth_yoy_rows(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute(
            "CREATE TABLE factor_financial_metrics (code TEXT, report_period TEXT, revenue REAL, revenue_yoy REAL, net_profit_yoy REAL, operating_cash_flow REAL, capex REAL)"
        )
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,0,'MAIN')")
        conn.execute(
            "INSERT INTO factor_financial_metrics VALUES ('000001','2026-03-31',100,NULL,15,50,20),('000002','2026-03-31',100,12,15,50,20)"
        )
        args = module.parse_args(["--db", str(db_path), "--scope", "repair_missing_fcfm_inputs"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "financials", conn)
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip() == "000001"
    finally:
        conn.close()


def test_full_refresh_dividends_selects_active_universe(tmp_path):
    db_path = tmp_path / "factor.db"
    conn = sqlite3.connect(db_path)
    try:
        conn.execute("CREATE TABLE factor_securities (code TEXT, is_active INTEGER, is_st INTEGER, board TEXT)")
        conn.execute("CREATE TABLE factor_dividend_records (code TEXT, updated_at TEXT)")
        conn.execute("INSERT INTO factor_securities VALUES ('000001',1,0,'MAIN'),('000002',1,1,'MAIN'),('300001',1,0,'CHINEXT')")
        args = module.parse_args(["--db", str(db_path), "--scope", "full_refresh_dividends"])
        args.db_path = db_path
        args.temp_files = []
        cmd = module.build_mode_command(args, "dividends", conn)
        code_list = cmd[cmd.index("--code-list-file") + 1]
        assert Path(code_list).read_text().strip().splitlines() == ["000001", "300001"]
    finally:
        conn.close()


@pytest.mark.parametrize("body", [
    "import time; time.sleep(60)",
    "import time\nwhile True:\n print('working', flush=True); time.sleep(0.01)",
    "import os\nwhile True:\n os.write(1, b'x' * 8192)",
])
def test_run_mode_timeout_covers_silent_noisy_and_unterminated_stdout(monkeypatch, tmp_path, body):
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    monkeypatch.setattr(module, "build_mode_command", lambda *unused: [sys.executable, "-c", body])
    args = module.parse_args([])
    args.step_timeout_seconds = 0.3
    conn = sqlite3.connect(tmp_path / "isolated.db")
    started = time.monotonic()
    try:
        result = module.run_mode(args, "daily-bars", conn)
    finally:
        conn.close()
    assert result.return_code == 124
    assert result.status == "failed"
    assert time.monotonic() - started < 5


def test_run_mode_keeps_final_summary_and_does_not_trust_summary_after_error(monkeypatch, tmp_path):
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    args = module.parse_args([])
    conn = sqlite3.connect(tmp_path / "isolated.db")
    try:
        for code, expected in [(0, "partial"), (1, "failed")]:
            body = f"import sys; print('summary={{}} status=partial', end='', flush=True); sys.exit({code})"
            monkeypatch.setattr(module, "build_mode_command", lambda *unused: [sys.executable, "-c", body])
            assert module.run_mode(args, "daily-bars", conn).status == expected
    finally:
        conn.close()


def test_output_is_bounded_and_long_line_does_not_prevent_next_summary(monkeypatch):
    import factor_lab_process as lifecycle
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    lines = []
    body = "import sys; sys.stdout.write('x' * 3000000 + '\\nsummary={} status=success\\n')"
    assert lifecycle.run_supervised_command([sys.executable, "-c", body], 5, lines.append) == 0
    assert len(lines) == 2
    assert len(lines[0]) <= lifecycle.MAX_LOG_LINE_BYTES + len(" [truncated]")
    assert lines[0].endswith(" [truncated]")
    assert lines[1] == "summary={} status=success"


def test_output_callback_failure_kills_child(monkeypatch):
    import factor_lab_process as lifecycle
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)

    def fail(_line):
        raise OSError("broken output sink")

    started = time.monotonic()
    with pytest.raises(RuntimeError, match="forwarding failed"):
        lifecycle.run_supervised_command(
            [sys.executable, "-c", "import time; print('ready', flush=True); time.sleep(60)"], 5, fail)
    assert time.monotonic() - started < 3


def test_reader_start_failure_still_kills_started_process(monkeypatch):
    import factor_lab_process as lifecycle
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    processes = []
    original_popen = lifecycle.subprocess.Popen

    def spawn(*args, **kwargs):
        process = original_popen(*args, **kwargs)
        processes.append(process)
        return process

    def fail_start(_self):
        raise RuntimeError("cannot start reader")

    monkeypatch.setattr(lifecycle.subprocess, "Popen", spawn)
    monkeypatch.setattr(lifecycle.threading.Thread, "start", fail_start)
    with pytest.raises(RuntimeError, match="cannot start reader"):
        lifecycle.run_supervised_command([sys.executable, "-c", "import time; time.sleep(60)"], 5, lambda unused: None)
    assert len(processes) == 1
    assert processes[0].poll() is not None


def test_blocked_output_sink_does_not_disable_timeout(monkeypatch):
    import factor_lab_process as lifecycle
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    unblock = threading.Event()
    started = time.monotonic()
    try:
        with pytest.raises(subprocess.TimeoutExpired):
            lifecycle.run_supervised_command(
                [sys.executable, "-c", "import time; print('ready', flush=True); time.sleep(60)"],
                0.3, lambda unused: unblock.wait(10))
        assert time.monotonic() - started < 4
    finally:
        unblock.set()


def test_mode_locks_conflict_across_processes_all_mode_and_symlinks(tmp_path):
    import factor_lab_process as lifecycle
    db = tmp_path / "isolated.db"
    db.touch()
    alias = tmp_path / "alias.db"
    alias.symlink_to(db)
    body = (
        f"import sys; sys.path.insert(0, {str(SCRIPT_PATH.parent)!r}); "
        "from factor_lab_process import phase0_mode_lock; from pathlib import Path; "
        f"ctx = phase0_mode_lock(Path({str(alias)!r}), 'daily-bars'); ctx.__enter__()"
    )
    with lifecycle.phase0_mode_lock(db, "all"):
        result = subprocess.run([sys.executable, "-c", body], capture_output=True, timeout=5)
        assert result.returncode != 0
        assert b"already running" in result.stderr
    with lifecycle.phase0_mode_lock(alias, "daily-bars"):
        # Different modes are independent; do not unnecessarily block Phase 0 work.
        with lifecycle.phase0_mode_lock(db, "financials"):
            pass


def test_mode_lock_releases_after_exception(tmp_path):
    import factor_lab_process as lifecycle
    db = tmp_path / "isolated.db"
    with pytest.raises(ValueError):
        with lifecycle.phase0_mode_lock(db, "daily-bars"):
            raise ValueError("cancel")
    with lifecycle.phase0_mode_lock(db, "daily-bars"):
        pass


def test_supervisor_kills_descendants_on_timeout_and_normal_exit(monkeypatch, tmp_path):
    import factor_lab_process as lifecycle
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    heartbeat = tmp_path / "heartbeat"
    grandchild = f"import time; from pathlib import Path\np=Path({str(heartbeat)!r})\nwhile True:\n p.write_text(str(time.monotonic())); time.sleep(0.02)"
    for wait in [True, False]:
        body = f"import subprocess, sys, time; p=subprocess.Popen([sys.executable, '-c', {grandchild!r}]); time.sleep(0.15); "
        body += "p.wait()" if wait else "sys.exit(0)"
        try:
            lifecycle.run_supervised_command([sys.executable, "-c", body], 0.5, lambda unused: None)
            assert not wait
        except subprocess.TimeoutExpired:
            assert wait
        assert heartbeat.exists()
        before = heartbeat.read_text()
        time.sleep(0.1)  # Observe external heartbeat, not background-task polling.
        assert heartbeat.read_text() == before


@pytest.mark.skipif(sys.platform != "linux", reason="Linux PR_SET_PDEATHSIG integration")
def test_linux_child_dies_when_supervisor_is_killed(monkeypatch, tmp_path):
    monkeypatch.delenv("FACTORLAB_MANAGED_PROCESS_GROUP", raising=False)
    heartbeat = tmp_path / "heartbeat"
    worker = (
        f"import sys; sys.path.insert(0, {str(SCRIPT_PATH.parent)!r}); "
        "from factor_lab_process import bind_to_parent; bind_to_parent(); "
        "import time; from pathlib import Path\n"
        f"p=Path({str(heartbeat)!r})\nwhile True:\n p.write_text(str(time.monotonic())); time.sleep(0.02)"
    )
    supervisor = (
        f"import sys; sys.path.insert(0, {str(SCRIPT_PATH.parent)!r}); "
        "from factor_lab_process import run_supervised_command; "
        f"run_supervised_command([sys.executable, '-c', {worker!r}], 60, lambda line: None)"
    )
    process = subprocess.Popen([sys.executable, "-c", supervisor], start_new_session=True)
    try:
        deadline = time.monotonic() + 5
        while not heartbeat.exists() and time.monotonic() < deadline:
            time.sleep(0.02)
        assert heartbeat.exists()
        process.kill()
        process.wait(timeout=3)
        time.sleep(0.05)
        before = heartbeat.read_text()
        time.sleep(0.1)
        assert heartbeat.read_text() == before
    finally:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait(timeout=3)


def test_timeout_marks_only_its_own_task_failed_and_keeps_committed_items(monkeypatch, tmp_path):
    import backfill_factor_lab_phase0 as backfill
    conn = sqlite3.connect(tmp_path / "isolated.db")
    backfill.ensure_schema(conn)
    args = module.parse_args(["--write"])
    for run_id in ["own-run", "other-run"]:
        backfill.insert_task_run(conn, run_id, "daily-bars", args)
    backfill.upsert_task_item(conn, "own-run", "daily_bar", "000001", "success")
    conn.commit()

    def timeout(command, budget, on_line, **unused):
        on_line("run_id=own-run")
        raise subprocess.TimeoutExpired(command, budget)

    monkeypatch.setattr(module, "build_mode_command", lambda *unused: ["unused"])
    monkeypatch.setattr(module, "run_supervised_command", timeout)
    try:
        assert module.run_mode(args, "daily-bars", conn).return_code == 124
        statuses = dict(conn.execute("SELECT id, status FROM factor_task_runs"))
        assert statuses == {"own-run": "failed", "other-run": "running"}
        row = conn.execute("SELECT finished_at,error_message FROM factor_task_runs WHERE id='own-run'").fetchone()
        assert row[0] and "timeout" in row[1]
        assert conn.execute("SELECT status FROM factor_task_items WHERE run_id='own-run'").fetchone()[0] == "success"
    finally:
        conn.close()
