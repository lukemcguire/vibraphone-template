set shell := ["bash", "-c"]
set dotenv-load := true

# ─── Quality Gate ───────────────────────────────────
check: lint test
    @echo "Quality gate passed."

# ─── Testing ──────────────────────────────────────
test *ARGS:
    @echo "Running tests..."
    cd src && uv run pytest --tb=short {{ARGS}}

# ─── Linting ──────────────────────────────────────
lint:
    @echo "Linting..."
    cd src && uv run ruff check .

# ─── Git Worktrees ─────────────────────────────────
start-task id:
    @echo "Creating worktree for {{id}}..."
    git fetch origin main
    git worktree add -b feat/{{id}} ./worktrees/{{id}} origin/main
    @echo "Worktree ready at ./worktrees/{{id}}"

finish-task id:
    @echo "Finishing task {{id}}..."
    just check
    cd ./worktrees/{{id}} && git push origin feat/{{id}}
    @echo "Pushed feat/{{id}}"

cleanup-task id:
    @echo "Removing worktree for {{id}}..."
    git worktree remove ./worktrees/{{id}} --force
    git branch -D feat/{{id}} 2>/dev/null || true

list-worktrees:
    git worktree list

# ─── Beads ─────────────────────────────────────────
beads-init:
    br init
    @echo "Beads initialized."

beads-status:
    br list --json

beads-ready:
    br ready --json

beads-sync:
    br sync --flush-only

# ─── Bootstrap ─────────────────────────────────────
bootstrap:
    @echo "Bootstrapping Vibraphone project..."
    @echo "Checking prerequisites..."
    @which git >/dev/null 2>&1 || (echo "git not found" && exit 1)
    @which just >/dev/null 2>&1 || (echo "just not found" && exit 1)
    @which br >/dev/null 2>&1 || (echo "br (beads_rust) not found. Install: cargo install beads_rust" && exit 1)
    @which uv >/dev/null 2>&1 || (echo "uv not found. Install: curl -LsSf https://astral.sh/uv/install.sh | sh" && exit 1)
    @echo "Prerequisites OK."
    cp -n .env.example .env 2>/dev/null || true
    just beads-init
    mkdir -p .vibraphone worktrees
    @echo "Ready. Run /gsd:new-project to start planning."

# ─── Review (standalone, for CI) ───────────────────
review *FILES:
    uv run python scripts/review.py {{FILES}}
