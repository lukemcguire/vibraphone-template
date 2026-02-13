"""Structured code review tools — LLM-powered review with JSON output.

Tools: request_code_review
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
from pathlib import Path

from config import config
from openai import AsyncOpenAI
from utils import br_client, session


async def _git_diff_staged() -> str:
    """Return the text of the current staged diff."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, _ = await proc.communicate()
    return stdout.decode()


async def _git_diff_staged_hash() -> str:
    """Return a SHA-256 hash of the current staged diff."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, _ = await proc.communicate()
    return hashlib.sha256(stdout).hexdigest()


async def _get_changed_files() -> list[str]:
    """Return list of staged file paths."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        "--name-only",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, _ = await proc.communicate()
    return [f for f in stdout.decode().strip().splitlines() if f]


def _read_file_contents(paths: list[str]) -> str:
    """Read and format the contents of the given files for review context."""
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


async def _perform_review(
    diff: str,
    files_content: str,
    constitution: str,
    prompt: str,
    model: str,
    api_key: str,
) -> dict:
    """Core review logic shared by MCP tool and standalone CLI.

    Returns dict with keys: status, issues, raw_response.
    """
    client = AsyncOpenAI(
        api_key=api_key,
        base_url="https://openrouter.ai/api/v1",
    )

    user_message = f"# RULES\n{constitution}\n\n# DIFF\n{diff}\n\n# FILES\n{files_content}"

    response = await client.chat.completions.create(
        model=model,
        max_tokens=4096,
        messages=[
            {"role": "system", "content": prompt},
            {"role": "user", "content": user_message},
        ],
    )

    raw_text = (response.choices[0].message.content or "").strip()

    # Parse JSON response — handle markdown code fences
    json_text = raw_text
    if json_text.startswith("```"):
        lines = json_text.splitlines()
        # Remove opening fence (```json or ```)
        lines = lines[1:]
        # Remove closing fence
        if lines and lines[-1].strip() == "```":
            lines = lines[:-1]
        json_text = "\n".join(lines)

    try:
        issues = json.loads(json_text)
        if not isinstance(issues, list):
            msg = "Response is not a JSON array"
            raise TypeError(msg)  # noqa: TRY301
    except (json.JSONDecodeError, TypeError):
        return {
            "status": "ESCALATED",
            "issues": [],
            "raw_response": raw_text,
            "parse_error": "Failed to parse reviewer response as JSON array",
        }

    # Determine status based on severity threshold
    threshold = config.quality_gate.review_severity_threshold
    dominated_severities = {"error"}
    if threshold == "warning":
        dominated_severities.add("warning")

    has_blocking = any(issue.get("severity") in dominated_severities for issue in issues)
    status = "REJECTED" if has_blocking else "APPROVED"

    return {"status": status, "issues": issues, "raw_response": raw_text}


async def request_code_review() -> dict:
    """Review staged git changes against the project constitution.

    Uses an LLM to check the staged diff against CONSTITUTION.md rules.
    Returns structured JSON with status (APPROVED/REJECTED/ESCALATED) and issues.
    """
    state = session.load_session() or session.SessionState()
    task_id = state.active_task or "unknown"

    # Increment review attempts
    attempt = session.increment_review_attempts(task_id)
    max_attempts = config.quality_gate.max_review_attempts

    # Circuit breaker
    if attempt > max_attempts:
        await br_client.br_update(task_id, status="blocked")
        result = {
            "status": "ESCALATED",
            "issues": [],
            "attempt": attempt,
            "reason": f"Max review attempts ({max_attempts}) exceeded. Task blocked.",
        }
        session.audit_log("request_code_review", {}, "escalated", result)
        return result

    # Validate API key
    api_key = os.environ.get("REVIEWER_API_KEY")
    if not api_key:
        result = {
            "status": "ESCALATED",
            "issues": [],
            "attempt": attempt,
            "reason": "REVIEWER_API_KEY environment variable not set",
        }
        session.audit_log("request_code_review", {}, "escalated", result)
        return result

    # Get staged diff
    diff = await _git_diff_staged()
    if not diff.strip():
        result = {
            "status": "REJECTED",
            "issues": [],
            "attempt": attempt,
            "reason": "No staged changes to review",
        }
        session.audit_log("request_code_review", {}, "rejected", result)
        return result

    # Get changed file contents
    changed_files = await _get_changed_files()
    files_content = _read_file_contents(changed_files)

    # Load constitution and reviewer prompt
    constitution_path = Path(config.review.constitution_file)
    prompt_path = Path(config.review.prompt_file)

    if not constitution_path.exists():
        result = {
            "status": "ESCALATED",
            "issues": [],
            "attempt": attempt,
            "reason": f"Constitution file not found: {config.review.constitution_file}",
        }
        session.audit_log("request_code_review", {}, "escalated", result)
        return result

    if not prompt_path.exists():
        result = {
            "status": "ESCALATED",
            "issues": [],
            "attempt": attempt,
            "reason": f"Reviewer prompt not found: {config.review.prompt_file}",
        }
        session.audit_log("request_code_review", {}, "escalated", result)
        return result

    constitution = constitution_path.read_text()
    prompt = prompt_path.read_text()

    # Perform the review
    review_result = await _perform_review(
        diff=diff,
        files_content=files_content,
        constitution=constitution,
        prompt=prompt,
        model=config.review.model,
        api_key=api_key,
    )

    status = review_result["status"]
    issues = review_result["issues"]

    # Update session state
    state = session.load_session() or session.SessionState()
    state.last_review_status = status
    state.last_review_diff_hash = await _git_diff_staged_hash()
    state.last_review_issues = issues
    session.save_session(state)

    result = {"status": status, "issues": issues, "attempt": attempt}
    if "parse_error" in review_result:
        result["parse_error"] = review_result["parse_error"]

    session.audit_log("request_code_review", {}, status.lower(), result)
    return result
