"""Standalone code reviewer for CI use.

Runs the same review logic as the MCP tool, but as a CLI script.
Exit codes: 0 = APPROVED, 1 = REJECTED, 2 = error/ESCALATED.

Usage:
    uv run python scripts/review.py [FILE ...]
    REVIEWER_API_KEY=sk-... uv run python scripts/review.py
"""

from __future__ import annotations

import asyncio
import json
import os
import subprocess
import sys
from pathlib import Path

# Add MCP server directory to path so we can import shared modules
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / ".mcp" / "servers" / "vibraphone"))

from config import load_config
from tools.review_tools import _perform_review


def _git_diff_staged() -> str:
    """Get staged diff text (synchronous for CLI use)."""
    result = subprocess.run(
        ["git", "diff", "--staged"],
        capture_output=True,
        text=True,
    )
    return result.stdout


def _get_changed_files() -> list[str]:
    """Get list of staged file paths (synchronous for CLI use)."""
    result = subprocess.run(
        ["git", "diff", "--staged", "--name-only"],
        capture_output=True,
        text=True,
    )
    return [f for f in result.stdout.strip().splitlines() if f]


def _read_file_contents(paths: list[str]) -> str:
    """Read and format file contents for review context."""
    sections = []
    for path in paths:
        p = Path(path)
        if p.exists() and p.is_file():
            try:
                content = p.read_text()
                sections.append(f"## {path}\n```\n{content}\n```")
            except (OSError, UnicodeDecodeError):
                sections.append(f"## {path}\n(binary or unreadable)")
    return "\n\n".join(sections)


async def main() -> int:
    """Run standalone review and return exit code."""
    cfg = load_config()

    api_key = os.environ.get("REVIEWER_API_KEY")
    if not api_key:
        print(json.dumps({"error": "REVIEWER_API_KEY environment variable not set"}))
        return 2

    # Get staged diff
    diff = _git_diff_staged()
    if not diff.strip():
        print(json.dumps({"error": "No staged changes to review"}))
        return 2

    # Get changed files — use CLI args if provided, otherwise staged files
    if len(sys.argv) > 1:
        changed_files = sys.argv[1:]
    else:
        changed_files = _get_changed_files()

    files_content = _read_file_contents(changed_files)

    # Load constitution and reviewer prompt
    constitution_path = Path(cfg.review.constitution_file)
    prompt_path = Path(cfg.review.prompt_file)

    if not constitution_path.exists():
        print(json.dumps({"error": f"Constitution not found: {cfg.review.constitution_file}"}))
        return 2

    if not prompt_path.exists():
        print(json.dumps({"error": f"Reviewer prompt not found: {cfg.review.prompt_file}"}))
        return 2

    constitution = constitution_path.read_text()
    prompt = prompt_path.read_text()

    # Perform review
    result = await _perform_review(
        diff=diff,
        files_content=files_content,
        constitution=constitution,
        prompt=prompt,
        model=cfg.review.model,
        api_key=api_key,
    )

    # Output JSON
    output = {"status": result["status"], "issues": result["issues"]}
    if "parse_error" in result:
        output["parse_error"] = result["parse_error"]

    print(json.dumps(output, indent=2))

    if result["status"] == "APPROVED":
        return 0
    elif result["status"] == "REJECTED":
        return 1
    else:
        return 2


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
