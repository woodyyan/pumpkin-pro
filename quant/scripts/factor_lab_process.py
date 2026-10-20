"""Bounded process supervision for Factor Lab CLI jobs (Unix production hosts)."""
from __future__ import annotations

import ctypes
import fcntl
import math
import os
import signal
import subprocess
import sys
import threading
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Callable, Iterator

MAX_LOG_LINE_BYTES = 64 * 1024
PROCESS_CLEANUP_SECONDS = 2.0
PHASE0_MODES = ("securities", "industries", "daily-bars", "index-bars", "financials", "dividends")


class JobAlreadyRunning(RuntimeError):
    pass


def bind_to_parent() -> None:
    """Linux kills managed Python children if their supervisor/backend disappears.

    The expected PID closes the race where the parent dies before prctl runs.
    Direct shell invocations (without the internal environment) are not bound.
    """
    expected = os.environ.get("FACTORLAB_PARENT_PID")
    if not expected:
        return
    parent_pid = int(expected)
    if sys.platform == "linux":
        libc = ctypes.CDLL(None, use_errno=True)
        if libc.prctl(1, signal.SIGKILL, 0, 0, 0) != 0:  # PR_SET_PDEATHSIG
            raise OSError(ctypes.get_errno(), "cannot bind Factor Lab child to parent")
    if os.getppid() != parent_pid:
        raise SystemExit(125)


@contextmanager
def termination_signals() -> Iterator[None]:
    """Unwind supervision on TERM/INT instead of abandoning its child."""
    def terminate(signum, _frame):
        raise SystemExit(128 + signum)

    previous = {sig: signal.signal(sig, terminate) for sig in (signal.SIGTERM, signal.SIGINT)}
    try:
        yield
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)


@contextmanager
def phase0_mode_lock(db_path: Path, mode: str) -> Iterator[None]:
    """Same DB + mode is exclusive, including dry-runs and direct CLI calls.

    'all' acquires every mode in a stable order. Keep lock files: unlinking a
    locked inode would allow another process to create a second, unlocked inode.
    """
    modes = PHASE0_MODES if mode == "all" else (mode,)
    handles = []
    try:
        for name in sorted(modes):
            path = Path(str(db_path.resolve()) + f".factorlab-{name}.lock")
            handle = path.open("a+")
            handles.append(handle)
            try:
                fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError as exc:
                raise JobAlreadyRunning(f"Factor Lab {name} is already running for {db_path}") from exc
            handle.seek(0)
            handle.truncate()
            handle.write(f"pid={os.getpid()} mode={name}\n")
            handle.flush()
        yield
    finally:
        for handle in reversed(handles):
            handle.close()  # releases flock even on exceptions; crash releases it in the kernel


def _kill(process: subprocess.Popen, own_group: bool) -> None:
    try:
        if own_group:
            os.killpg(process.pid, signal.SIGKILL)
        elif process.poll() is None:
            process.kill()  # Go owns the shared group; Linux parent-death protection cascades
    except ProcessLookupError:
        pass


def run_supervised_command(
    command: list[str], timeout_seconds: float, on_line: Callable[[str], None],
    *, cwd: str | None = None, child_env: dict[str, str] | None = None,
) -> int:
    """Start deadline before spawn; stdout never owns the timeout control loop.

    A daemon reader streams bounded lines, never communicate()'s unbounded log
    buffer. The supervising thread remains able to kill a hung/noisy child even
    if stdout has no newline or forwarding itself stalls.
    """
    if not math.isfinite(timeout_seconds) or timeout_seconds <= 0:
        raise ValueError("process timeout must be finite and positive")
    deadline = time.monotonic() + timeout_seconds
    own_group = os.environ.get("FACTORLAB_MANAGED_PROCESS_GROUP") != "1"
    env = dict(os.environ)
    env.update(child_env or {})
    env["FACTORLAB_MANAGED_PROCESS_GROUP"] = "1"
    env["FACTORLAB_PARENT_PID"] = str(os.getpid())
    process = subprocess.Popen(command, cwd=cwd, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.STDOUT, bufsize=0, start_new_session=own_group)
    errors: list[BaseException] = []

    def read_output() -> None:
        pending = bytearray()
        truncated = False

        def emit() -> None:
            nonlocal truncated
            line = pending.decode("utf-8", errors="replace").rstrip("\r")
            if truncated:
                line += " [truncated]"
            pending.clear()
            truncated = False
            if line:
                on_line(line)

        try:
            assert process.stdout is not None
            while True:
                chunk = process.stdout.read(8192)
                if not chunk:
                    if pending or truncated:
                        emit()
                    return
                parts = chunk.split(b"\n")
                for index, part in enumerate(parts):
                    room = MAX_LOG_LINE_BYTES - len(pending)
                    pending.extend(part[:room])
                    truncated = truncated or len(part) > room
                    if index < len(parts) - 1:
                        emit()
        except BaseException as exc:
            errors.append(exc)

    reader = threading.Thread(target=read_output, name="factorlab-output", daemon=True)
    reader_started = False
    try:
        reader.start()
        reader_started = True
        while True:
            if errors:
                raise RuntimeError("Factor Lab output forwarding failed") from errors[0]
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise subprocess.TimeoutExpired(command, timeout_seconds)
            try:
                result = process.wait(timeout=min(remaining, 0.1))
                break
            except subprocess.TimeoutExpired:
                continue
        # Reap remaining descendants before waiting for EOF on inherited pipes.
        _kill(process, own_group)
        reader.join(PROCESS_CLEANUP_SECONDS)
        if reader.is_alive():
            raise RuntimeError("Factor Lab output did not close after process exit")
        if errors:
            raise RuntimeError("Factor Lab output forwarding failed") from errors[0]
        return result
    finally:
        _kill(process, own_group)
        process.wait(timeout=PROCESS_CLEANUP_SECONDS)
        if reader_started:
            reader.join(PROCESS_CLEANUP_SECONDS)
        if process.stdout is not None:
            process.stdout.close()
