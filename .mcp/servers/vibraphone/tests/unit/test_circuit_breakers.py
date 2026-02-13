"""Tests for circuit breaker logic across quality and review tools."""

from __future__ import annotations

import hashlib
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from utils import br_client, session
from utils.br_client import detect_cycles, detect_orphans

# ── Fixtures ──────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _isolate_session(tmp_path, monkeypatch):
    """Redirect session files to a temp dir so tests don't pollute each other."""
    monkeypatch.setattr(session, "SESSION_DIR", tmp_path)
    monkeypatch.setattr(session, "SESSION_FILE", tmp_path / "session.json")
    monkeypatch.setattr(session, "AUDIT_LOG", tmp_path / "audit.log")


@pytest.fixture
def set_active_task():
    """Helper to write a session with an active task."""
    def _inner(task_id="T-1"):
        state = session.SessionState(active_task=task_id)
        session.save_session(state)
    return _inner


@pytest.fixture
def mock_br_update():
    with patch.object(br_client, "br_update", new_callable=AsyncMock) as m:
        m.return_value = {}
        yield m


@pytest.fixture
def mock_run_shell():
    with patch("tools.quality_tools._run_shell", new_callable=AsyncMock) as m:
        yield m


# ── Session counter tests ────────────────────────────────────────────


class TestSessionCounters:
    def test_increment_test_attempts(self):
        assert session.increment_test_attempts("T-1") == 1
        assert session.increment_test_attempts("T-1") == 2
        assert session.increment_test_attempts("T-1") == 3

    def test_increment_review_attempts(self):
        assert session.increment_review_attempts("T-1") == 1
        assert session.increment_review_attempts("T-1") == 2

    def test_separate_tasks_have_separate_counters(self):
        session.increment_test_attempts("T-1")
        session.increment_test_attempts("T-1")
        assert session.increment_test_attempts("T-2") == 1


# ── run_tests circuit breaker ────────────────────────────────────────


class TestRunTestsCircuitBreaker:
    @pytest.mark.asyncio
    async def test_escalates_after_max_attempts(self, set_active_task, mock_br_update, mock_run_shell, monkeypatch):
        from config import config
        monkeypatch.setattr(config.quality_gate, "max_test_attempts", 2)

        set_active_task("T-1")
        mock_run_shell.return_value = (1, "FAILED")

        from tools.quality_tools import run_tests

        # Attempts 1 and 2 should run normally
        r1 = await run_tests()
        assert r1["status"] == "fail"
        assert r1["attempt"] == 1

        r2 = await run_tests()
        assert r2["status"] == "fail"
        assert r2["attempt"] == 2

        # Attempt 3 should trigger ESCALATED
        r3 = await run_tests()
        assert r3["status"] == "ESCALATED"
        assert r3["attempt"] == 3
        mock_br_update.assert_called_with("T-1", status="blocked")

    @pytest.mark.asyncio
    async def test_sets_task_blocked(self, set_active_task, mock_br_update, mock_run_shell, monkeypatch):
        from config import config
        monkeypatch.setattr(config.quality_gate, "max_test_attempts", 1)

        set_active_task("T-1")
        mock_run_shell.return_value = (1, "FAILED")

        from tools.quality_tools import run_tests

        await run_tests()  # attempt 1
        await run_tests()  # attempt 2 → ESCALATED

        mock_br_update.assert_called_with("T-1", status="blocked")


# ── request_code_review circuit breaker ──────────────────────────────


class TestRequestCodeReviewCircuitBreaker:
    @pytest.mark.asyncio
    async def test_escalates_after_max_review_attempts(self, set_active_task, mock_br_update, monkeypatch):
        from config import config
        monkeypatch.setattr(config.quality_gate, "max_review_attempts", 2)

        set_active_task("T-1")

        from tools.review_tools import request_code_review

        # Burn through attempts by manually incrementing
        session.increment_review_attempts("T-1")  # 1
        session.increment_review_attempts("T-1")  # 2

        # Next call should escalate (attempt 3 > max 2)
        result = await request_code_review()
        assert result["status"] == "ESCALATED"
        assert "Max review attempts" in result["reason"]
        mock_br_update.assert_called_with("T-1", status="blocked")


# ── Review issue aggregation ─────────────────────────────────────────


class TestReviewIssueAggregation:
    @pytest.mark.asyncio
    async def test_merges_previous_and_current_issues(self, set_active_task, monkeypatch):
        set_active_task("T-1")

        # Seed session with issues from a prior review
        state = session.load_session()
        state.last_review_issues = [
            {"rule": "no-unused-vars", "file": "foo.py", "line": 10, "severity": "warning", "message": "old"},
        ]
        session.save_session(state)

        # Bump review attempt so the next call is attempt 2
        session.increment_review_attempts("T-1")

        # Mock all the I/O inside request_code_review
        diff_text = "diff --git a/foo.py b/foo.py\n+print('hi')"
        diff_hash = hashlib.sha256(diff_text.encode()).hexdigest()

        new_issues = [
            {"rule": "no-unused-vars", "file": "foo.py", "line": 10, "severity": "warning", "message": "new"},
            {"rule": "no-print", "file": "foo.py", "line": 11, "severity": "error", "message": "new"},
        ]

        monkeypatch.setenv("REVIEWER_API_KEY", "fake-key")

        with (
            patch("tools.review_tools._git_diff_staged", new_callable=AsyncMock, return_value=diff_text),
            patch("tools.review_tools._git_diff_staged_hash", new_callable=AsyncMock, return_value=diff_hash),
            patch("tools.review_tools._get_changed_files", new_callable=AsyncMock, return_value=["foo.py"]),
            patch("tools.review_tools._read_file_contents", return_value="# foo.py content"),
            patch("tools.review_tools._perform_review", new_callable=AsyncMock, return_value={
                "status": "REJECTED",
                "issues": new_issues,
                "raw_response": "[]",
            }),
            patch("tools.review_tools.Path") as mock_path,
        ):
            # Make constitution and prompt files appear to exist
            mock_constitution = MagicMock()
            mock_constitution.exists.return_value = True
            mock_constitution.read_text.return_value = "# Constitution"

            mock_prompt = MagicMock()
            mock_prompt.exists.return_value = True
            mock_prompt.read_text.return_value = "# Prompt"

            mock_path.side_effect = lambda p: mock_constitution if "CONSTITUTION" in str(p) else mock_prompt

            from tools.review_tools import request_code_review
            result = await request_code_review()

        # Should have 2 unique issues (deduped by rule+file+line)
        assert len(result["issues"]) == 2
        rules = {i["rule"] for i in result["issues"]}
        assert rules == {"no-unused-vars", "no-print"}

    @pytest.mark.asyncio
    async def test_no_aggregation_on_first_attempt(self, set_active_task, monkeypatch):
        set_active_task("T-1")

        diff_text = "diff --git a/foo.py b/foo.py\n+print('hi')"
        diff_hash = hashlib.sha256(diff_text.encode()).hexdigest()
        current_issues = [
            {"rule": "no-print", "file": "foo.py", "line": 1, "severity": "error", "message": "msg"},
        ]

        monkeypatch.setenv("REVIEWER_API_KEY", "fake-key")

        with (
            patch("tools.review_tools._git_diff_staged", new_callable=AsyncMock, return_value=diff_text),
            patch("tools.review_tools._git_diff_staged_hash", new_callable=AsyncMock, return_value=diff_hash),
            patch("tools.review_tools._get_changed_files", new_callable=AsyncMock, return_value=["foo.py"]),
            patch("tools.review_tools._read_file_contents", return_value="# foo.py content"),
            patch("tools.review_tools._perform_review", new_callable=AsyncMock, return_value={
                "status": "REJECTED",
                "issues": current_issues,
                "raw_response": "[]",
            }),
            patch("tools.review_tools.Path") as mock_path,
        ):
            mock_file = MagicMock()
            mock_file.exists.return_value = True
            mock_file.read_text.return_value = "# content"
            mock_path.side_effect = lambda _p: mock_file

            from tools.review_tools import request_code_review
            result = await request_code_review()

        assert result["attempt"] == 1
        assert len(result["issues"]) == 1


# ── attempt_commit rejection ─────────────────────────────────────────


class TestAttemptCommit:
    @pytest.mark.asyncio
    async def test_rejects_without_approved_review(self):
        from tools.quality_tools import attempt_commit
        result = await attempt_commit("test commit")
        assert result["status"] == "rejected"
        assert "No approved review" in result["reason"]

    @pytest.mark.asyncio
    async def test_rejects_when_diff_changed(self, set_active_task):
        set_active_task("T-1")
        state = session.load_session()
        state.last_review_status = "APPROVED"
        state.last_review_diff_hash = "stale-hash"
        session.save_session(state)

        with patch("tools.quality_tools._git_diff_staged_hash", new_callable=AsyncMock, return_value="different-hash"):
            from tools.quality_tools import attempt_commit
            result = await attempt_commit("test commit")

        assert result["status"] == "rejected"
        assert "diff changed" in result["reason"]


# ── health_check structured return ───────────────────────────────────


class TestHealthCheck:
    @pytest.mark.asyncio
    async def test_returns_structured_format(self):
        tasks = [
            {"id": "1", "dependencies": []},
            {"id": "2", "dependencies": ["1"]},
        ]

        with (
            patch.object(br_client, "br_doctor", new_callable=AsyncMock, return_value={"status": "ok"}),
            patch.object(br_client, "br_list", new_callable=AsyncMock, return_value=tasks),
        ):
            from tools.beads_tools import health_check
            result = await health_check()

        assert "br_doctor" in result
        assert "cycles" in result
        assert "orphans" in result
        assert result["br_doctor"] == {"status": "ok"}
        assert result["cycles"] == []
        assert result["orphans"] == []

    @pytest.mark.asyncio
    async def test_detects_cycles(self):
        tasks = [
            {"id": "1", "dependencies": ["2"]},
            {"id": "2", "dependencies": ["1"]},
        ]

        with (
            patch.object(br_client, "br_doctor", new_callable=AsyncMock, return_value={}),
            patch.object(br_client, "br_list", new_callable=AsyncMock, return_value=tasks),
        ):
            from tools.beads_tools import health_check
            result = await health_check()

        assert len(result["cycles"]) > 0

    @pytest.mark.asyncio
    async def test_detects_orphans(self):
        tasks = [
            {"id": "1", "dependencies": ["999"]},
        ]

        with (
            patch.object(br_client, "br_doctor", new_callable=AsyncMock, return_value={}),
            patch.object(br_client, "br_list", new_callable=AsyncMock, return_value=tasks),
        ):
            from tools.beads_tools import health_check
            result = await health_check()

        assert len(result["orphans"]) == 1
        assert result["orphans"][0]["missing_dep"] == "999"

    @pytest.mark.asyncio
    async def test_handles_dict_with_tasks_key(self):
        tasks = [{"id": "1", "dependencies": []}]

        with (
            patch.object(br_client, "br_doctor", new_callable=AsyncMock, return_value={}),
            patch.object(br_client, "br_list", new_callable=AsyncMock, return_value={"tasks": tasks}),
        ):
            from tools.beads_tools import health_check
            result = await health_check()

        assert result["cycles"] == []
        assert result["orphans"] == []


# ── detect_cycles / detect_orphans unit tests ────────────────────────


class TestDetectCycles:
    def test_no_cycles(self):
        tasks = [
            {"id": "1", "dependencies": []},
            {"id": "2", "dependencies": ["1"]},
            {"id": "3", "dependencies": ["2"]},
        ]
        assert detect_cycles(tasks) == []

    def test_simple_cycle(self):
        tasks = [
            {"id": "A", "dependencies": ["B"]},
            {"id": "B", "dependencies": ["A"]},
        ]
        cycles = detect_cycles(tasks)
        assert len(cycles) >= 1
        # The cycle should contain both A and B
        flat = {node for cycle in cycles for node in cycle}
        assert "A" in flat
        assert "B" in flat

    def test_self_cycle(self):
        tasks = [{"id": "X", "dependencies": ["X"]}]
        cycles = detect_cycles(tasks)
        assert len(cycles) >= 1

    def test_three_node_cycle(self):
        tasks = [
            {"id": "1", "dependencies": ["3"]},
            {"id": "2", "dependencies": ["1"]},
            {"id": "3", "dependencies": ["2"]},
        ]
        cycles = detect_cycles(tasks)
        assert len(cycles) >= 1


class TestDetectOrphans:
    def test_no_orphans(self):
        tasks = [
            {"id": "1", "dependencies": []},
            {"id": "2", "dependencies": ["1"]},
        ]
        assert detect_orphans(tasks) == []

    def test_missing_dependency(self):
        tasks = [
            {"id": "1", "dependencies": ["99"]},
        ]
        orphans = detect_orphans(tasks)
        assert len(orphans) == 1
        assert orphans[0] == {"task_id": "1", "missing_dep": "99"}

    def test_multiple_orphans(self):
        tasks = [
            {"id": "1", "dependencies": ["99", "100"]},
            {"id": "2", "dependencies": ["1"]},
        ]
        orphans = detect_orphans(tasks)
        assert len(orphans) == 2
