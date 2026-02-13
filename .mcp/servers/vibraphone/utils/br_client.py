"""Beads Rust CLI client — shells out to br with --json flag."""

from __future__ import annotations

import asyncio
import json


class BrError(Exception):
    """Raised when br CLI returns a non-zero exit code."""

    def __init__(self, returncode: int, stderr: str, args: tuple[str, ...]) -> None:
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

    if proc.returncode:
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


async def br_create(
    title: str,
    description: str | None = None,
    type_: str | None = None,
    labels: str | None = None,
) -> dict:
    """Create a new Beads issue."""
    args = ["create", title]
    if description:
        args.extend(["--description", description])
    if type_:
        args.extend(["--type", type_])
    if labels:
        args.extend(["--labels", labels])
    return await br_run(*args)


async def br_dep_add(issue: str, depends_on: str, dep_type: str = "blocks") -> dict:
    """Add dependency: depends_on must complete before issue can start."""
    return await br_run("dep", "add", issue, depends_on, "--type", dep_type)


async def br_doctor() -> dict:
    """Run br doctor health check."""
    return await br_run("doctor")


def detect_cycles(tasks: list[dict]) -> list[list[str]]:
    """Detect dependency cycles in a task list via DFS.

    Each task dict should have an 'id' field and optionally a 'dependencies'
    field (list of task IDs this task depends on).  Returns a list of cycles,
    where each cycle is a list of task IDs forming the loop.
    """
    adj: dict[str, list[str]] = {}
    for t in tasks:
        tid = str(t.get("id", ""))
        deps = [str(d) for d in (t.get("dependencies") or [])]
        adj[tid] = deps

    WHITE, GRAY, BLACK = 0, 1, 2
    color: dict[str, int] = {tid: WHITE for tid in adj}
    cycles: list[list[str]] = []
    path: list[str] = []

    def dfs(node: str) -> None:
        color[node] = GRAY
        path.append(node)
        for neighbour in adj.get(node, []):
            if neighbour not in color:
                continue
            if color[neighbour] == GRAY:
                idx = path.index(neighbour)
                cycles.append(path[idx:] + [neighbour])
            elif color[neighbour] == WHITE:
                dfs(neighbour)
        path.pop()
        color[node] = BLACK

    for node in list(adj):
        if color[node] == WHITE:
            dfs(node)

    return cycles


def detect_orphans(tasks: list[dict]) -> list[dict]:
    """Find tasks whose dependencies reference non-existent task IDs.

    Returns a list of ``{"task_id": ..., "missing_dep": ...}`` dicts.
    """
    known_ids = {str(t.get("id", "")) for t in tasks}
    orphans: list[dict] = []
    for t in tasks:
        tid = str(t.get("id", ""))
        for dep in t.get("dependencies") or []:
            if str(dep) not in known_ids:
                orphans.append({"task_id": tid, "missing_dep": str(dep)})
    return orphans


async def br_sync() -> dict:
    """Run br sync --flush-only."""
    return await br_run("sync", "--flush-only")
