"""Tests for Stitch integration: MCP config sync, .env management, and configure_stack stitch support."""

from __future__ import annotations

import json
from unittest.mock import patch

import pytest
import yaml

from tools.stack_tools import STITCH_MCP_ENTRY, _sync_mcp_config, _update_env_var

# ── Fixtures ──────────────────────────────────────────────────────────


@pytest.fixture
def project(tmp_path):
    """Create a minimal project directory with .mcp/config.json and vibraphone.yaml."""
    mcp_dir = tmp_path / ".mcp"
    mcp_dir.mkdir()

    config = {
        "mcpServers": {
            "vibraphone": {
                "command": "uv",
                "args": ["run", "python", ".mcp/servers/vibraphone/server.py"],
                "env": {"VIBRAPHONE_CONFIG": "./vibraphone.yaml"},
            }
        }
    }
    (mcp_dir / "config.json").write_text(json.dumps(config, indent=2) + "\n")

    yaml_content = {
        "project": {"name": "test-app", "version": "0.1.0"},
        "components": {
            "backend": {
                "language": "python",
                "root": "./backend",
                "test_command": "pytest --tb=short",
                "lint_command": "ruff check .",
                "format_command": "ruff format .",
                "coverage_threshold": 80,
            }
        },
        "stitch": {"enabled": False, "project_id": "${STITCH_PROJECT_ID}"},
    }
    (tmp_path / "vibraphone.yaml").write_text(yaml.dump(yaml_content, default_flow_style=False, sort_keys=False))

    (tmp_path / ".env.example").write_text("REVIEWER_API_KEY=\nSTITCH_PROJECT_ID=\nSTITCH_API_KEY=\n")

    return tmp_path


# ── _sync_mcp_config tests ────────────────────────────────────────────


class TestSyncMcpConfig:
    def test_adds_stitch_when_enabled(self, project):
        result = _sync_mcp_config(stitch_enabled=True, project_root=project)

        assert result["stitch_config_changed"] is True
        assert result["stitch_enabled"] is True

        config = json.loads((project / ".mcp" / "config.json").read_text())
        assert "stitch" in config["mcpServers"]
        assert config["mcpServers"]["stitch"] == STITCH_MCP_ENTRY

    def test_removes_stitch_when_disabled(self, project):
        # First add stitch
        _sync_mcp_config(stitch_enabled=True, project_root=project)

        # Then remove it
        result = _sync_mcp_config(stitch_enabled=False, project_root=project)

        assert result["stitch_config_changed"] is True
        assert result["stitch_enabled"] is False

        config = json.loads((project / ".mcp" / "config.json").read_text())
        assert "stitch" not in config["mcpServers"]

    def test_no_change_when_already_synced(self, project):
        # Stitch not present, disabled → no change
        result = _sync_mcp_config(stitch_enabled=False, project_root=project)
        assert result["stitch_config_changed"] is False

        # Add stitch, then call again with enabled → no change
        _sync_mcp_config(stitch_enabled=True, project_root=project)
        result = _sync_mcp_config(stitch_enabled=True, project_root=project)
        assert result["stitch_config_changed"] is False

    def test_preserves_vibraphone_entry(self, project):
        _sync_mcp_config(stitch_enabled=True, project_root=project)

        config = json.loads((project / ".mcp" / "config.json").read_text())
        assert "vibraphone" in config["mcpServers"]
        assert config["mcpServers"]["vibraphone"]["command"] == "uv"

        _sync_mcp_config(stitch_enabled=False, project_root=project)
        config = json.loads((project / ".mcp" / "config.json").read_text())
        assert "vibraphone" in config["mcpServers"]


# ── _update_env_var tests ─────────────────────────────────────────────


class TestUpdateEnvVar:
    def test_updates_existing_var(self, project):
        (project / ".env").write_text("FOO=old\nBAR=baz\n")

        changed = _update_env_var("FOO", "new", project)

        assert changed is True
        content = (project / ".env").read_text()
        assert "FOO=new" in content
        assert "BAR=baz" in content

    def test_appends_new_var(self, project):
        (project / ".env").write_text("FOO=val\n")

        changed = _update_env_var("NEW_KEY", "new_val", project)

        assert changed is True
        content = (project / ".env").read_text()
        assert "NEW_KEY=new_val" in content
        assert "FOO=val" in content

    def test_creates_env_from_example(self, project):
        # .env doesn't exist yet, .env.example does
        assert not (project / ".env").exists()

        changed = _update_env_var("STITCH_PROJECT_ID", "proj-123", project)

        assert changed is True
        content = (project / ".env").read_text()
        assert "STITCH_PROJECT_ID=proj-123" in content


# ── configure_stack integration tests ─────────────────────────────────


class TestConfigureStackStitch:
    @pytest.fixture(autouse=True)
    def _patch_config(self, project):
        """Patch _find_config_file to point at the tmp project's vibraphone.yaml."""
        with (
            patch("tools.stack_tools._find_config_file", return_value=project / "vibraphone.yaml"),
            patch("tools.stack_tools.reload_config"),
        ):
            self.project = project
            yield

    @pytest.mark.asyncio
    async def test_stitch_project_id_enables_stitch(self):
        from tools.stack_tools import configure_stack

        components = {
            "backend": {"language": "python", "root": "./backend"},
        }
        result = await configure_stack(components, preview=False, stitch_project_id="test-proj-123")

        assert result["status"] == "configured"
        assert result["stitch_config"]["stitch_enabled"] is True
        assert result["stitch_config"]["stitch_config_changed"] is True

        # Check vibraphone.yaml has stitch.enabled: true
        yaml_content = yaml.safe_load((self.project / "vibraphone.yaml").read_text())
        assert yaml_content["stitch"]["enabled"] is True

        # Check .env has the project ID
        env_content = (self.project / ".env").read_text()
        assert "STITCH_PROJECT_ID=test-proj-123" in env_content

        # Check .mcp/config.json has stitch entry
        mcp_config = json.loads((self.project / ".mcp" / "config.json").read_text())
        assert "stitch" in mcp_config["mcpServers"]

    @pytest.mark.asyncio
    async def test_configure_stack_preview_no_sync(self):
        from tools.stack_tools import configure_stack

        components = {
            "backend": {"language": "python", "root": "./backend"},
        }
        result = await configure_stack(components, preview=True, stitch_project_id="test-proj-123")

        assert result["status"] == "preview"
        assert "stitch_note" in result

        # .env should not exist (not created)
        assert not (self.project / ".env").exists()

        # .mcp/config.json should not have stitch (still only vibraphone)
        mcp_config = json.loads((self.project / ".mcp" / "config.json").read_text())
        assert "stitch" not in mcp_config["mcpServers"]

    @pytest.mark.asyncio
    async def test_enable_disable_roundtrip(self):
        from tools.stack_tools import configure_stack

        components = {
            "backend": {"language": "python", "root": "./backend"},
        }

        # Enable stitch
        result = await configure_stack(components, preview=False, stitch_project_id="proj-abc")
        assert result["stitch_config"]["stitch_enabled"] is True
        mcp_config = json.loads((self.project / ".mcp" / "config.json").read_text())
        assert "stitch" in mcp_config["mcpServers"]

        # Now reconfigure without stitch_project_id — stitch.enabled was set to true
        # in the yaml, so it should still be enabled (config is re-read from file)
        yaml_data = yaml.safe_load((self.project / "vibraphone.yaml").read_text())
        assert yaml_data["stitch"]["enabled"] is True

        # Manually disable stitch in yaml to test removal
        yaml_data["stitch"]["enabled"] = False
        (self.project / "vibraphone.yaml").write_text(yaml.dump(yaml_data, default_flow_style=False, sort_keys=False))

        result = await configure_stack(components, preview=False)
        assert result["stitch_config"]["stitch_enabled"] is False
        assert result["stitch_config"]["stitch_config_changed"] is True

        mcp_config = json.loads((self.project / ".mcp" / "config.json").read_text())
        assert "stitch" not in mcp_config["mcpServers"]
