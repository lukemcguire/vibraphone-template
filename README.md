# Vibraphone Template

Project template for the Vibraphone framework — a tool-enforced development workflow for AI coding agents.

## Quick Start

```bash
./scripts/bootstrap.sh
```

Or manually:

```bash
just bootstrap
```

## Structure

- `vibraphone.yaml` — project configuration
- `.mcp/` — MCP server (tool definitions for AI agents)
- `docs/` — constitution, architecture, specs
- `scripts/` — bootstrap and CI helpers
- `Justfile` — task runner

See `AGENTS.md` for the agent behavioral contract.
