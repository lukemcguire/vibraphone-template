"""GSD-to-Beads bridge tools — imports GSD plans into Beads tasks.

Tools: import_gsd_plan

Parses GSD PLAN.md files (YAML frontmatter + XML <tasks> blocks) and creates
Beads issues with the correct dependency graph for execution.
"""

from __future__ import annotations

import re
from pathlib import Path

import yaml
from defusedxml.ElementTree import fromstring as parse_xml

from config import project_root
from utils import br_client, session

# ---------------------------------------------------------------------------
# Parsing layer (pure functions, no I/O)
# ---------------------------------------------------------------------------

_FRONTMATTER_RE = re.compile(r"^---\s*\n(.*?)\n---\s*\n", re.DOTALL)
_TASKS_RE = re.compile(r"<tasks>(.*?)</tasks>", re.DOTALL)
_NEW_COMPONENT_RE = re.compile(
    r"\b(new service|create endpoint|add component|new module|scaffold)\b",
    re.IGNORECASE,
)


def _extract_frontmatter(content: str) -> dict:
    """Extract YAML frontmatter from between --- fences."""
    match = _FRONTMATTER_RE.match(content)
    if not match:
        return {}
    return yaml.safe_load(match.group(1)) or {}


_BARE_AMP_RE = re.compile(r"&(?!amp;|lt;|gt;|quot;|apos;|#)")
# Structural XML tags used in GSD plan files (opening, closing, or self-closing)
_KNOWN_TAGS = r"tasks|task|name|files|action|verify|done|title|description|labels|type"
_STRUCTURAL_TAG_RE = re.compile(rf"<(?:/?(?:{_KNOWN_TAGS})\b)")


def _sanitize_xml_content(raw: str) -> str:
    """Escape '&' and '<' that aren't part of known XML structure."""
    raw = _BARE_AMP_RE.sub("&amp;", raw)
    # Escape '<' that don't start a known structural tag
    result: list[str] = []
    i = 0
    while i < len(raw):
        if raw[i] == "<":
            if _STRUCTURAL_TAG_RE.match(raw, i):
                result.append("<")
            else:
                result.append("&lt;")
        else:
            result.append(raw[i])
        i += 1
    return "".join(result)


def _extract_tasks_from_xml(body: str) -> list[dict]:
    """Find <tasks>...</tasks> block and parse each <task> element."""
    match = _TASKS_RE.search(body)
    if not match:
        return []

    inner = _sanitize_xml_content(match.group(1))
    xml_str = f"<tasks>{inner}</tasks>"
    root = parse_xml(xml_str)

    tasks = []
    for task_el in root.findall("task"):
        task_data: dict = {}
        for child in task_el:
            task_data[child.tag] = (child.text or "").strip()
        tasks.append(task_data)
    return tasks


def _detect_new_components(body: str) -> bool:
    """Heuristic: does the plan body mention creating new components?"""
    return bool(_NEW_COMPONENT_RE.search(body))


# ---------------------------------------------------------------------------
# Beads integration (async, calls br CLI)
# ---------------------------------------------------------------------------


async def _create_beads_task(plan_id: str, task_idx: int, task_data: dict) -> str:
    """Create a single Beads issue from a parsed task element.

    Returns the Beads issue ID.
    """
    title_text = task_data.get("title", f"Task {task_idx + 1}")
    title = f"[{plan_id}] {title_text}"
    description = task_data.get("description", "")
    labels = task_data.get("labels", f"plan:{plan_id}")
    type_ = task_data.get("type", "task")

    result = await br_client.br_create(title, description=description or None, type_=type_, labels=labels)
    return str(result.get("id", result.get("issue_id", "")))


async def _setup_plan_dependencies(
    plan_tasks: dict[str, list[str]],
    plan_deps: dict[str, list[str]],
    all_plans_map: dict[str, list[str]],
) -> list[dict]:
    """Wire up intra-plan and inter-plan dependencies.

    Args:
        plan_tasks: mapping of plan_id -> list of beads issue IDs (ordered)
        plan_deps: mapping of plan_id -> list of plan_ids it depends on
        all_plans_map: same as plan_tasks (used for cross-plan lookups)

    Returns:
        List of dependency records {blocked, blocker, type}.
    """
    deps_created: list[dict] = []

    for plan_id, task_ids in plan_tasks.items():
        # Intra-plan: sequential execution (task N+1 blocked by task N)
        for i in range(1, len(task_ids)):
            blocked = task_ids[i]
            blocker = task_ids[i - 1]
            await br_client.br_dep_add(blocked, blocker)
            deps_created.append({"blocked": blocked, "blocker": blocker, "type": "intra-plan"})

        # Inter-plan: first task of this plan blocked by last task of each dep
        if plan_id in plan_deps and task_ids:
            first_task = task_ids[0]
            for dep_plan_id in plan_deps[plan_id]:
                dep_task_ids = all_plans_map.get(dep_plan_id, [])
                if dep_task_ids:
                    last_dep_task = dep_task_ids[-1]
                    await br_client.br_dep_add(first_task, last_dep_task)
                    deps_created.append(
                        {
                            "blocked": first_task,
                            "blocker": last_dep_task,
                            "type": "inter-plan",
                        }
                    )

    return deps_created


# ---------------------------------------------------------------------------
# Main tool
# ---------------------------------------------------------------------------

_PLAN_FILE_RE = re.compile(r"^(\d+-\d+)-PLAN\.md$")


async def import_gsd_plan(phase_number: int) -> dict:
    """Import all GSD PLAN.md files for a phase into Beads tasks.

    Reads .planning/phases/{phase_number}/ for files matching {N}-{M}-PLAN.md,
    parses frontmatter and XML task blocks, creates Beads issues, and wires up
    dependency edges.

    Args:
        phase_number: The GSD phase number to import (e.g. 1).

    Returns:
        dict with tasks_created, dependencies, and diagram_update_needed.
    """
    # GSD uses zero-padded directory names like "01-core-crawler-foundation"
    root = project_root()
    padded = str(phase_number).zfill(2)
    phases_dir = root / ".planning" / "phases"
    candidates = sorted(phases_dir.glob(f"{padded}-*"))
    if not candidates:
        # Fallback to exact numeric match
        candidates = [phases_dir / str(phase_number)]
    phase_dir = candidates[0]
    if not phase_dir.is_dir():
        return {"error": f"Phase directory not found: {phase_dir}"}

    # Discover plan files, sorted for deterministic ordering
    plan_files: list[tuple[str, Path]] = []
    for path in sorted(phase_dir.iterdir()):
        match = _PLAN_FILE_RE.match(path.name)
        if match:
            plan_files.append((match.group(1), path))

    if not plan_files:
        return {"error": f"No PLAN.md files found in {phase_dir}"}

    # --- Pass 1: Parse all plans, create Beads tasks ---
    plan_tasks: dict[str, list[str]] = {}  # plan_id -> [beads_issue_id, ...]
    plan_deps: dict[str, list[str]] = {}  # plan_id -> [dep_plan_id, ...]
    tasks_created: list[dict] = []
    has_new_components = False

    for plan_id, plan_path in plan_files:
        content = plan_path.read_text()
        frontmatter = _extract_frontmatter(content)
        parsed_tasks = _extract_tasks_from_xml(content)

        if not parsed_tasks:
            continue

        # Record inter-plan dependencies from frontmatter
        raw_deps = frontmatter.get("depends_on", [])
        if isinstance(raw_deps, str):
            raw_deps = [raw_deps]
        plan_deps[plan_id] = [str(d) for d in raw_deps]

        # Check for new component creation
        if _detect_new_components(content):
            has_new_components = True

        # Create Beads issues for each task
        issue_ids: list[str] = []
        for idx, task_data in enumerate(parsed_tasks):
            issue_id = await _create_beads_task(plan_id, idx, task_data)
            issue_ids.append(issue_id)
            tasks_created.append({"plan_id": plan_id, "task_index": idx, "issue_id": issue_id})

        plan_tasks[plan_id] = issue_ids

    # --- Pass 2: Wire up all dependencies ---
    dependencies = await _setup_plan_dependencies(plan_tasks, plan_deps, plan_tasks)

    result = {
        "tasks_created": tasks_created,
        "dependencies": dependencies,
        "diagram_update_needed": has_new_components,
    }

    # Flush to JSONL so bv sees the imported tasks
    await br_client.br_sync()

    session.audit_log(
        "import_gsd_plan",
        {"phase_number": phase_number},
        "ok",
        result,
    )

    return result
