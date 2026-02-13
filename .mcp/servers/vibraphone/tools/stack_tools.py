"""Stack configuration tool — generates per-component Justfile recipes and vibraphone.yaml config."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import yaml

from config import _find_config_file, reload_config

STACK_DEFAULTS: dict[str, dict[str, str]] = {
    "python": {
        "test_command": "uv run pytest --tb=short",
        "lint_command": "uv run ruff check . && uv run ruff format --check .",
        "format_command": "uv run ruff check --fix . && uv run ruff format .",
    },
    "typescript": {
        "test_command": "npx vitest run",
        "lint_command": "npx eslint .",
        "format_command": "npx eslint --fix . && npx prettier --write .",
    },
    "go": {
        "test_command": "go test ./...",
        "lint_command": "golangci-lint run",
        "format_command": "gofmt -w .",
    },
}


def _render_component_recipes(name: str, root: str, commands: dict[str, str]) -> str:
    """Render per-component Justfile recipes for a single component."""
    lines: list[str] = []

    lines.append(f"test-{name} *ARGS:")
    lines.append(f"    cd {root} && {commands['test_command']} {{{{ARGS}}}}")
    lines.append("")

    lines.append(f"lint-{name}:")
    lines.append(f"    cd {root} && {commands['lint_command']}")
    lines.append("")

    lines.append(f"format-{name}:")
    lines.append(f"    cd {root} && {commands['format_command']}")

    return "\n".join(lines)


def _render_justfile(components: dict[str, dict[str, Any]]) -> str:
    """Render a complete Justfile from component definitions."""
    names = list(components.keys())

    sections: list[str] = []

    # Header
    sections.append('set shell := ["bash", "-c"]')
    sections.append("set dotenv-load := true")
    sections.append("")

    # Quality gate
    sections.append("# --- Quality Gate ---")
    sections.append("check: lint test")
    sections.append('    @echo "Quality gate passed."')
    sections.append("")

    # Aggregate test
    sections.append("# --- Testing ---")
    test_deps = " ".join(f"test-{n}" for n in names)
    sections.append(f"test *ARGS: {test_deps}")
    sections.append('    @echo "All tests passed."')
    sections.append("")

    # Aggregate lint
    sections.append("# --- Linting ---")
    lint_deps = " ".join(f"lint-{n}" for n in names)
    sections.append(f"lint: {lint_deps}")
    sections.append('    @echo "All linting passed."')
    sections.append("")

    # Aggregate format
    sections.append("# --- Formatting ---")
    format_deps = " ".join(f"format-{n}" for n in names)
    sections.append(f"format: {format_deps}")
    sections.append('    @echo "All formatting done."')
    sections.append("")

    # Per-component recipes
    sections.append("# --- Per-Component Recipes ---")
    for name, comp in components.items():
        root = comp.get("root", f"./{name}")
        defaults = STACK_DEFAULTS.get(comp.get("language", ""), {})
        commands = {
            "test_command": comp.get("test_command", defaults.get("test_command", "echo 'no test command'")),
            "lint_command": comp.get("lint_command", defaults.get("lint_command", "echo 'no lint command'")),
            "format_command": comp.get("format_command", defaults.get("format_command", "echo 'no format command'")),
        }
        sections.append(_render_component_recipes(name, root, commands))
        sections.append("")

    # Universal recipes
    sections.append("# --- Git Worktrees ---")
    sections.append("start-task id:")
    sections.append('    @echo "Creating worktree for {{id}}..."')
    sections.append("    git fetch origin main")
    sections.append("    git worktree add -b feat/{{id}} ./worktrees/{{id}} origin/main")
    sections.append('    @echo "Worktree ready at ./worktrees/{{id}}"')
    sections.append("")
    sections.append("finish-task id:")
    sections.append('    @echo "Finishing task {{id}}..."')
    sections.append("    just check")
    sections.append("    cd ./worktrees/{{id}} && git push origin feat/{{id}}")
    sections.append('    @echo "Pushed feat/{{id}}"')
    sections.append("")
    sections.append("cleanup-task id:")
    sections.append('    @echo "Removing worktree for {{id}}..."')
    sections.append("    git worktree remove ./worktrees/{{id}} --force")
    sections.append("    git branch -D feat/{{id}} 2>/dev/null || true")
    sections.append("")
    sections.append("list-worktrees:")
    sections.append("    git worktree list")
    sections.append("")

    # Beads
    sections.append("# --- Beads ---")
    sections.append("beads-init:")
    sections.append("    br init")
    sections.append('    @echo "Beads initialized."')
    sections.append("")
    sections.append("beads-status:")
    sections.append("    br list --json")
    sections.append("")
    sections.append("beads-ready:")
    sections.append("    br ready --json")
    sections.append("")
    sections.append("beads-sync:")
    sections.append("    br sync --flush-only")
    sections.append("")

    # Bootstrap
    sections.append("# --- Bootstrap ---")
    sections.append("bootstrap:")
    sections.append('    @echo "Bootstrapping Vibraphone project..."')
    sections.append('    @echo "Checking prerequisites..."')
    sections.append('    @which git >/dev/null 2>&1 || (echo "git not found" && exit 1)')
    sections.append('    @which just >/dev/null 2>&1 || (echo "just not found" && exit 1)')
    sections.append(
        '    @which br >/dev/null 2>&1 || (echo "br (beads_rust) not found.'
        ' Install: cargo install beads_rust" && exit 1)'
    )
    sections.append('    @echo "Prerequisites OK."')
    sections.append("    cp -n .env.example .env 2>/dev/null || true")
    sections.append("    just beads-init")
    sections.append("    mkdir -p .vibraphone worktrees")
    sections.append('    @echo "Ready. Run /gsd:new-project to start planning."')
    sections.append("")

    # Review
    sections.append("# --- Review (standalone, for CI) ---")
    sections.append("review *FILES:")
    sections.append("    uv run python scripts/review.py {{FILES}}")
    sections.append("")

    return "\n".join(sections)


def _render_vibraphone_yaml(components: dict[str, dict[str, Any]], existing_config: dict) -> str:
    """Render vibraphone.yaml with updated components section, preserving other sections."""
    # Build new components section
    new_components: dict[str, dict[str, Any]] = {}
    for name, comp in components.items():
        defaults = STACK_DEFAULTS.get(comp.get("language", ""), {})
        new_components[name] = {
            "language": comp.get("language", "python"),
            "root": comp.get("root", f"./{name}"),
            "test_command": comp.get("test_command", defaults.get("test_command", "")),
            "lint_command": comp.get("lint_command", defaults.get("lint_command", "")),
            "format_command": comp.get("format_command", defaults.get("format_command", "")),
            "coverage_threshold": comp.get("coverage_threshold", 80),
        }

    # Replace components in existing config
    existing_config["components"] = new_components
    return yaml.dump(existing_config, default_flow_style=False, sort_keys=False)


async def configure_stack(
    components: dict[str, dict[str, Any]],
    *,
    preview: bool = True,
) -> dict:
    """Generate per-component Justfile recipes and vibraphone.yaml config.

    Two-phase flow:
    - preview=True: returns proposed file contents for agent to present to user.
    - preview=False: writes files, reloads config, returns confirmation.

    Args:
        components: mapping of component name to config (language, root, test_command, etc.)
        preview: if True, return proposals without writing; if False, write and reload.
    """
    # Read existing vibraphone.yaml
    config_path = _find_config_file()
    existing_config: dict = {}
    if config_path and config_path.exists():
        with config_path.open() as f:
            existing_config = yaml.safe_load(f) or {}

    justfile_content = _render_justfile(components)
    yaml_content = _render_vibraphone_yaml(components, existing_config)

    if preview:
        return {
            "status": "preview",
            "justfile": justfile_content,
            "vibraphone_yaml": yaml_content,
        }

    # Write files
    project_root = config_path.parent if config_path else Path.cwd()

    justfile_path = project_root / "Justfile"
    justfile_path.write_text(justfile_content)

    yaml_path = project_root / "vibraphone.yaml"
    yaml_path.write_text(yaml_content)

    # Reload config singleton so quality tools pick up new components
    reload_config()

    return {
        "status": "configured",
        "justfile_path": str(justfile_path),
        "vibraphone_yaml_path": str(yaml_path),
        "components": list(components.keys()),
    }
