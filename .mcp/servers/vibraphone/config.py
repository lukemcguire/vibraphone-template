"""Configuration loader — reads vibraphone.yaml and provides typed access."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path

import yaml


@dataclass
class ComponentConfig:
    """Per-component build and quality settings (language, test/lint commands)."""

    language: str = "python"
    root: str = "./src"
    test_command: str = "pytest --tb=short"
    lint_command: str = "ruff check ."
    format_command: str = "ruff check --fix . && ruff format ."
    coverage_threshold: int = 80


@dataclass
class QualityGateConfig:
    """Thresholds and circuit-breaker limits for the quality gate."""

    require_tests: bool = True
    require_lint: bool = True
    require_review: bool = True
    review_severity_threshold: str = "error"
    max_test_attempts: int = 5
    max_review_attempts: int = 3


@dataclass
class WorktreeConfig:
    """Git worktree lifecycle settings (base branch, naming prefix)."""

    base_branch: str = "main"
    prefix: str = "feat/"
    auto_cleanup: bool = False


@dataclass
class ReviewConfig:
    """LLM code-review settings (model, prompt file, constitution file)."""

    model: str = "anthropic/claude-sonnet-4-5-20250929"
    prompt_file: str = "./docs/prompts/reviewer.md"
    constitution_file: str = "./docs/CONSTITUTION.md"


@dataclass
class BeadsConfig:
    """Beads task tracker integration toggles."""

    use_bv: bool = False
    auto_sync: bool = True


@dataclass
class StitchConfig:
    """Google Stitch MCP integration settings."""

    enabled: bool = False
    project_id: str = ""


@dataclass
class ProjectConfig:
    """Top-level project identity (name and version)."""

    name: str = "my-app"
    version: str = "0.1.0"


@dataclass
class VibraphoneConfig:
    """Root configuration parsed from vibraphone.yaml."""

    project: ProjectConfig = field(default_factory=ProjectConfig)
    components: dict[str, ComponentConfig] = field(default_factory=dict)
    quality_gate: QualityGateConfig = field(default_factory=QualityGateConfig)
    worktree: WorktreeConfig = field(default_factory=WorktreeConfig)
    review: ReviewConfig = field(default_factory=ReviewConfig)
    beads: BeadsConfig = field(default_factory=BeadsConfig)
    stitch: StitchConfig = field(default_factory=StitchConfig)


def _build_config(raw: dict) -> VibraphoneConfig:
    """Build a VibraphoneConfig from parsed YAML dict."""
    project = ProjectConfig(**raw.get("project", {}))

    components: dict[str, ComponentConfig] = {}
    for name, comp in raw.get("components", {}).items():
        components[name] = ComponentConfig(**comp)

    quality_gate = QualityGateConfig(**raw.get("quality_gate", {}))
    worktree = WorktreeConfig(**raw.get("worktree", {}))
    review = ReviewConfig(**raw.get("review", {}))
    beads = BeadsConfig(**raw.get("beads", {}))
    stitch = StitchConfig(**raw.get("stitch", {}))

    return VibraphoneConfig(
        project=project,
        components=components,
        quality_gate=quality_gate,
        worktree=worktree,
        review=review,
        beads=beads,
        stitch=stitch,
    )


def project_root() -> Path:
    """Return the project root directory.

    Resolution: VIBRAPHONE_CONFIG env var parent > walk-up search for
    vibraphone.yaml > fall back to CWD.
    """
    env_path = os.environ.get("VIBRAPHONE_CONFIG")
    if env_path:
        resolved = Path(env_path).resolve()
        if resolved.exists():
            return resolved.parent
    found = _find_config_file()
    if found:
        return found.parent
    return Path.cwd().resolve()


def _find_config_file(filename: str = "vibraphone.yaml") -> Path | None:
    """Walk up from CWD to find vibraphone.yaml."""
    current = Path.cwd().resolve()
    for directory in [current, *current.parents]:
        candidate = directory / filename
        if candidate.exists():
            return candidate
    return None


def load_config(path: str | Path | None = None) -> VibraphoneConfig:
    """Load configuration from vibraphone.yaml.

    Resolution order: explicit path > VIBRAPHONE_CONFIG env var > walk-up search.
    """
    if path is None:
        env_path = os.environ.get("VIBRAPHONE_CONFIG")
        if env_path:
            path = Path(env_path)
        else:
            found = _find_config_file()
            if found is None:
                return VibraphoneConfig()
            path = found
    else:
        path = Path(path)

    if not path.exists():
        return VibraphoneConfig()

    with Path(path).open() as f:
        raw = yaml.safe_load(f) or {}

    return _build_config(raw)


# Module-level singleton — loaded once, imported by tools
config = load_config()


def reload_config() -> VibraphoneConfig:
    """Re-read vibraphone.yaml and update the module-level singleton."""
    global config
    config = load_config()
    return config
