"""Beads Rust CLI client — shells out to br with --json flag."""

from __future__ import annotations

import asyncio
import json


class BrError(Exception):
    """Raised when br CLI returns a non-zero exit code."""

    def __init__(self, returncode: int, stderr: str, args: tuple[str, ...]):
        self.returncode = returncode
        self.stderr = stderr
        self.args_used = args
        super().__init__(f"br {' '.join(args)} failed (rc={returncode}): {stderr}")


async def br_run(*args: str) -> dict:
    """Run ``br <args> --json`` and return parsed JSON output."""
    cmd = ["br", *args, "--json"]
    proc = await asyncio.create_subprocess_exec(
        *cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, stderr = await proc.communicate()

    if proc.returncode != 0:
        raise BrError(proc.returncode, stderr.decode().strip(), args)

    text = stdout.decode().strip()
    if not text:
        return {}
    return json.loads(text)


async def br_list(filter_str: str | None = None) -> dict:
    """List tasks, optionally filtered."""
    args = ["list"]
    if filter_str:
        args.extend(["--filter", filter_str])
    return await br_run(*args)


async def br_ready() -> dict:
    """Get next ready task(s)."""
    return await br_run("ready")


async def br_close(task_id: str) -> dict:
    """Close (complete) a task."""
    return await br_run("close", task_id)


async def br_update(task_id: str, **kwargs: str) -> dict:
    """Update task fields. Keyword args become --key value flags."""
    args = ["update", task_id]
    for key, value in kwargs.items():
        args.extend([f"--{key}", value])
    return await br_run(*args)


async def br_doctor() -> dict:
    """Run br doctor health check."""
    return await br_run("doctor")


async def br_sync() -> dict:
    """Run br sync --flush-only."""
    return await br_run("sync", "--flush-only")
