"""Tests for session recovery logic in session_tools.py."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock, patch

import pytest

from utils import br_client, session

# ── Fixtures ──────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _isolate_session(tmp_path, monkeypatch):
    """Redirect session files to a temp dir so tests don't pollute each other."""
    monkeypatch.setattr(session, "SESSION_DIR", tmp_path)
    monkeypatch.setattr(session, "SESSION_FILE", tmp_path / "session.json")
    monkeypatch.setattr(session, "AUDIT_LOG", tmp_path / "audit.log")


def _make_session(
    task_id: str = "T-1",
    worktree: str | None = "./worktrees/T-1",
    minutes_ago: int = 5,
) -> None:
    """Write a session with an active task at a given staleness."""
    ts = (datetime.now(UTC) - timedelta(minutes=minutes_ago)).isoformat()
    state = session.SessionState(
        active_task=task_id,
        worktree=worktree,
        last_action="run_tests",
        last_action_time=ts,
        test_attempts={task_id: 2},
        review_attempts={task_id: 1},
    )
    session.save_session(state)


# ── Tests ─────────────────────────────────────────────────────────────


class TestCleanSession:
    @pytest.mark.asyncio
    async def test_clean_session_no_task(self):
        """No session file at all → clean."""
        from tools.session_tools import recover_session

        result = await recover_session()
        assert result["status"] == "clean"
        assert result["action"] == "none"

    @pytest.mark.asyncio
    async def test_clean_session_no_active_task(self):
        """Session file exists but active_task is None → clean."""
        session.save_session(session.SessionState())

        from tools.session_tools import recover_session

        result = await recover_session()
        assert result["status"] == "clean"
        assert result["action"] == "none"


class TestFreshSession:
    @pytest.mark.asyncio
    async def test_fresh_session_resumes(self, tmp_path):
        """Active task < 30 min old with valid worktree → resume."""
        worktree_path = tmp_path / "worktrees" / "T-1"
        worktree_path.mkdir(parents=True)

        _make_session(worktree=str(worktree_path), minutes_ago=5)

        from tools.session_tools import recover_session

        result = await recover_session()
        assert result["status"] == "active"
        assert result["action"] == "resume"
        assert result["task_id"] == "T-1"
        assert result["worktree"] == str(worktree_path)
        assert result["attempt_counts"]["test_attempts"] == {"T-1": 2}
        assert result["attempt_counts"]["review_attempts"] == {"T-1": 1}


class TestStaleSession:
    @pytest.mark.asyncio
    async def test_stale_session_worktree_missing(self):
        """Active task >= 30 min, worktree gone → cleans up, resets to open."""
        _make_session(worktree="/nonexistent/worktree/path", minutes_ago=60)

        with (
            patch.object(
                br_client,
                "br_show",
                new_callable=AsyncMock,
                return_value={"status": "in_progress"},
            ) as mock_show,
            patch.object(br_client, "br_update", new_callable=AsyncMock, return_value={}) as mock_update,
        ):
            from tools.session_tools import recover_session

            result = await recover_session()

        assert result["status"] == "stale"
        assert result["action"] == "cleaned_up"
        assert result["reason"] == "worktree missing"
        mock_show.assert_called_once_with("T-1")
        mock_update.assert_called_once_with("T-1", status="open")

        # Session should be cleared
        state = session.load_session()
        assert state is not None
        assert state.active_task is None

    @pytest.mark.asyncio
    async def test_stale_session_worktree_exists(self, tmp_path):
        """Active task >= 30 min, worktree present → resume with stale flag."""
        worktree_path = tmp_path / "worktrees" / "T-1"
        worktree_path.mkdir(parents=True)

        _make_session(worktree=str(worktree_path), minutes_ago=60)

        with patch.object(
            br_client,
            "br_show",
            new_callable=AsyncMock,
            return_value={"status": "in_progress"},
        ):
            from tools.session_tools import recover_session

            result = await recover_session()

        assert result["status"] == "stale"
        assert result["action"] == "resume"
        assert result["task_id"] == "T-1"
        assert result["worktree"] == str(worktree_path)

    @pytest.mark.asyncio
    async def test_stale_session_task_already_closed(self):
        """Active task >= 30 min, task no longer in_progress → cleans up."""
        _make_session(worktree="/some/path", minutes_ago=60)

        with patch.object(
            br_client,
            "br_show",
            new_callable=AsyncMock,
            return_value={"status": "closed"},
        ):
            from tools.session_tools import recover_session

            result = await recover_session()

        assert result["status"] == "stale"
        assert result["action"] == "cleaned_up"
        assert result["reason"] == "task no longer in_progress"

        # Session should be cleared
        state = session.load_session()
        assert state is not None
        assert state.active_task is None
