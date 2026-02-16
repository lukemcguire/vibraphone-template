# ruff: noqa: S603, S607
"""Tests for review tools — _parse_porcelain and staging logic.

S603/S607: subprocess calls use partial paths, but this is test code
with controlled input.
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest

from tools.review_tools import _parse_porcelain, _stage_files, _working_dir
from utils import session


class TestParsePorcelain:
    """Tests for _parse_porcelain extracting paths from git status output."""

    def test_modified_not_staged(self):
        """Modified in worktree but not staged: ' M file'."""
        lines = [" M src/main.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/main.py"]

    def test_modified_staged(self):
        """Modified and staged: 'M  file'."""
        lines = ["M  src/main.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/main.py"]

    def test_added_file(self):
        """Added (staged for commit): 'A  file'."""
        lines = ["A  src/new_file.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/new_file.py"]

    def test_untracked_file(self):
        """Untracked file: '?? file'."""
        lines = ["?? src/untracked.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/untracked.py"]

    def test_deleted_file(self):
        """Deleted file: ' D file' or 'D  file'."""
        lines = [" D src/old_file.py", "D  src/another_old.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/old_file.py", "src/another_old.py"]

    def test_renamed_file(self):
        """Renamed file: 'R  old -> new' — returns destination."""
        lines = ["R  src/old_name.py -> src/new_name.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/new_name.py"]

    def test_copied_file(self):
        """Copied file: 'C  old -> new' — returns destination."""
        lines = ["C  src/original.py -> src/copy.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/copy.py"]

    def test_multiple_files(self):
        """Multiple files with various statuses."""
        lines = [
            " M src/modified.py",
            "M  src/staged.py",
            "A  src/added.py",
            "?? src/untracked.py",
            "D  src/deleted.py",
        ]
        result = _parse_porcelain(lines)
        assert result == [
            "src/modified.py",
            "src/staged.py",
            "src/added.py",
            "src/untracked.py",
            "src/deleted.py",
        ]

    def test_empty_lines_skipped(self):
        """Empty lines are skipped."""
        lines = [" M src/main.py", "", "M  src/other.py", "   "]
        result = _parse_porcelain(lines)
        assert result == ["src/main.py", "src/other.py"]

    def test_empty_input(self):
        """Empty input returns empty list."""
        result = _parse_porcelain([])
        assert result == []

    def test_whitespace_only_lines(self):
        """Lines with only whitespace are skipped."""
        lines = ["   ", "\t", "  \t  "]
        result = _parse_porcelain(lines)
        assert result == []

    def test_quoted_paths(self):
        """Paths with special characters may be quoted - git status uses XY format."""
        # Git status --porcelain uses XY prefix even for quoted paths
        lines = ['?? "path with spaces.py"', ' M "another space.py"']
        result = _parse_porcelain(lines)
        # The regex strips leading quotes from the captured group
        assert result == ["path with spaces.py", "another space.py"]

    def test_both_staged_and_unstaged(self):
        """File with both staged and unstaged changes: 'MM file'."""
        lines = ["MM src/partially_staged.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/partially_staged.py"]

    def test_merge_conflict_file(self):
        """Unmerged (conflict) file: 'UU file'."""
        lines = ["UU src/conflicted.py"]
        result = _parse_porcelain(lines)
        assert result == ["src/conflicted.py"]


class TestWorkingDirResolution:
    """Tests for _working_dir resolving session worktree paths."""

    def test_no_session_returns_none(self, tmp_path, monkeypatch):
        """No active session returns None (inherit process cwd)."""
        monkeypatch.setattr(session, "SESSION_FILE", tmp_path / "session.json")
        result = _working_dir()
        assert result is None

    def test_no_worktree_in_session_returns_none(self, tmp_path, monkeypatch):
        """Session exists but no worktree set returns None."""
        monkeypatch.setattr(session, "SESSION_DIR", tmp_path)
        monkeypatch.setattr(session, "SESSION_FILE", tmp_path / "session.json")
        session.save_session(session.SessionState(active_task="T-1"))
        result = _working_dir()
        assert result is None

    def test_worktree_resolved_to_absolute_path(self, tmp_path, monkeypatch):
        """Worktree path is resolved to absolute path using project_root."""
        monkeypatch.setattr(session, "SESSION_DIR", tmp_path)
        monkeypatch.setattr(session, "SESSION_FILE", tmp_path / "session.json")

        # Create a worktree path
        worktree_path = tmp_path / "worktrees" / "T-1"
        worktree_path.mkdir(parents=True)

        # Save session with relative worktree path
        session.save_session(
            session.SessionState(active_task="T-1", worktree="./worktrees/T-1")
        )

        # Mock project_root to return tmp_path
        from tools import review_tools

        with patch.object(review_tools, "project_root", return_value=tmp_path):
            result = _working_dir()

        assert result is not None
        assert Path(result) == worktree_path.resolve()


class TestStageFilesInWorktree:
    """Integration tests for staging files in a worktree."""

    @pytest.fixture
    def git_repo_with_worktree(self, tmp_path):
        """Create a git repo with a worktree for testing."""
        # Create main repo
        main_repo = tmp_path / "main"
        main_repo.mkdir()

        # Initialize git repo
        subprocess.run(["git", "init"], cwd=main_repo, check=True, capture_output=True)
        subprocess.run(
            ["git", "config", "user.email", "test@test.com"],
            cwd=main_repo,
            check=True,
            capture_output=True,
        )
        subprocess.run(
            ["git", "config", "user.name", "Test"],
            cwd=main_repo,
            check=True,
            capture_output=True,
        )

        # Create initial commit
        initial_file = main_repo / "initial.txt"
        initial_file.write_text("initial content\n")
        subprocess.run(
            ["git", "add", "initial.txt"], cwd=main_repo, check=True, capture_output=True
        )
        subprocess.run(
            ["git", "commit", "-m", "initial"],
            cwd=main_repo,
            check=True,
            capture_output=True,
        )

        # Create worktree inside main_repo to match relative path resolution
        worktree_path = main_repo / "worktrees" / "T-1"
        subprocess.run(
            ["git", "worktree", "add", str(worktree_path), "-b", "feat/T-1"],
            cwd=main_repo,
            check=True,
            capture_output=True,
        )

        return main_repo, worktree_path

    @pytest.mark.asyncio
    async def test_stage_files_in_worktree(self, git_repo_with_worktree, monkeypatch):
        """Staging files in worktree discovers and stages modified files."""
        main_repo, worktree_path = git_repo_with_worktree

        # Setup session to point to worktree
        session_dir = main_repo / ".vibraphone"
        session_dir.mkdir(exist_ok=True)
        monkeypatch.setattr(session, "SESSION_DIR", session_dir)
        monkeypatch.setattr(session, "SESSION_FILE", session_dir / "session.json")

        # Save session with relative worktree path (relative to project root)
        relative_worktree = "worktrees/T-1"
        session.save_session(
            session.SessionState(active_task="T-1", worktree=relative_worktree)
        )

        # Modify a file in the worktree
        modified_file = worktree_path / "initial.txt"
        modified_file.write_text("modified content\n")

        # Mock project_root to return main_repo
        from tools import review_tools

        with patch.object(review_tools, "project_root", return_value=main_repo):
            result = await _stage_files(None, stage_all=True)

        assert result["status"] == "staged", f"Result: {result}"
        assert "initial.txt" in result["staged"]

    @pytest.mark.asyncio
    async def test_stage_files_no_changes_in_worktree(
        self, git_repo_with_worktree, monkeypatch
    ):
        """Staging with no changes returns nothing_to_stage."""
        main_repo, _worktree_path = git_repo_with_worktree

        # Setup session to point to worktree
        session_dir = main_repo / ".vibraphone"
        session_dir.mkdir(exist_ok=True)
        monkeypatch.setattr(session, "SESSION_DIR", session_dir)
        monkeypatch.setattr(session, "SESSION_FILE", session_dir / "session.json")

        relative_worktree = "worktrees/T-1"
        session.save_session(
            session.SessionState(active_task="T-1", worktree=relative_worktree)
        )

        # No changes in worktree
        from tools import review_tools

        with patch.object(review_tools, "project_root", return_value=main_repo):
            result = await _stage_files(None, stage_all=True)

        assert result["status"] == "nothing_to_stage"
        assert result["staged"] == []

    @pytest.mark.asyncio
    async def test_stage_files_new_file_in_worktree(
        self, git_repo_with_worktree, monkeypatch
    ):
        """Staging a new untracked file in worktree."""
        main_repo, worktree_path = git_repo_with_worktree

        # Setup session to point to worktree
        session_dir = main_repo / ".vibraphone"
        session_dir.mkdir(exist_ok=True)
        monkeypatch.setattr(session, "SESSION_DIR", session_dir)
        monkeypatch.setattr(session, "SESSION_FILE", session_dir / "session.json")

        relative_worktree = "worktrees/T-1"
        session.save_session(
            session.SessionState(active_task="T-1", worktree=relative_worktree)
        )

        # Create new file in worktree
        new_file = worktree_path / "new_file.txt"
        new_file.write_text("new content\n")

        from tools import review_tools

        with patch.object(review_tools, "project_root", return_value=main_repo):
            result = await _stage_files(None, stage_all=True)

        assert result["status"] == "staged"
        assert "new_file.txt" in result["staged"]
