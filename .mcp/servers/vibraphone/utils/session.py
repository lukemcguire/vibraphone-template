"""Session state management — reads/writes .vibraphone/session.json."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path

SESSION_DIR = Path(".vibraphone")
SESSION_FILE = SESSION_DIR / "session.json"
AUDIT_LOG = SESSION_DIR / "audit.log"


@dataclass
class SessionState:
    active_task: str | None = None
    phase: str | None = None
    worktree: str | None = None
    last_action: str | None = None
    last_action_result: str | None = None
    last_action_time: str | None = None
    test_attempts: dict[str, int] = field(default_factory=dict)
    review_attempts: dict[str, int] = field(default_factory=dict)
    last_review_status: str | None = None
    last_review_diff_hash: str | None = None
    last_review_issues: list[dict] = field(default_factory=list)


def _ensure_dir() -> None:
    SESSION_DIR.mkdir(parents=True, exist_ok=True)


def load_session() -> SessionState | None:
    """Load session state from disk. Returns None if no session file exists."""
    if not SESSION_FILE.exists():
        return None
    raw = json.loads(SESSION_FILE.read_text())
    return SessionState(**raw)


def save_session(state: SessionState) -> None:
    """Write session state to disk."""
    _ensure_dir()
    SESSION_FILE.write_text(json.dumps(asdict(state), indent=2) + "\n")


def increment_test_attempts(task_id: str) -> int:
    """Increment and return the new test attempt count for a task."""
    state = load_session() or SessionState()
    count = state.test_attempts.get(task_id, 0) + 1
    state.test_attempts[task_id] = count
    save_session(state)
    return count


def increment_review_attempts(task_id: str) -> int:
    """Increment and return the new review attempt count for a task."""
    state = load_session() or SessionState()
    count = state.review_attempts.get(task_id, 0) + 1
    state.review_attempts[task_id] = count
    save_session(state)
    return count


def clear_task() -> None:
    """Reset active task state while preserving session file."""
    state = load_session() or SessionState()
    state.active_task = None
    state.phase = None
    state.worktree = None
    state.last_review_status = None
    state.last_review_diff_hash = None
    state.last_review_issues = []
    save_session(state)


def audit_log(tool: str, inputs: dict, status: str, output: dict) -> None:
    """Append a structured entry to the audit log and update session state.

    Combines audit logging with session bookkeeping so every tool invocation
    automatically keeps last_action / last_action_result / last_action_time
    current for crash recovery.
    """
    now = datetime.now(timezone.utc).isoformat()

    # Update session state
    state = load_session() or SessionState()
    state.last_action = tool
    state.last_action_result = status
    state.last_action_time = now
    save_session(state)

    # Append audit entry
    _ensure_dir()
    entry = {
        "timestamp": now,
        "tool": tool,
        "inputs": inputs,
        "status": status,
        "output": output,
    }
    with open(AUDIT_LOG, "a") as f:
        f.write(json.dumps(entry) + "\n")
