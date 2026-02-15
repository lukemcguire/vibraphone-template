set shell := ["bash", "-c"]
set dotenv-load := true

# --- Quality Gate ---
check: lint test
    @echo "Quality gate passed."

# --- Testing ---
test *ARGS: test-app
    @echo "All tests passed."

# --- Linting ---
lint: lint-app
    @echo "All linting passed."

# --- Formatting ---
format: format-app
    @echo "All formatting done."

# --- Per-Component Recipes ---
test-app *ARGS:
    cd ./src && go test ./... {{ARGS}}

lint-app:
    cd ./src && golangci-lint run ./...

format-app:
    cd ./src && gofumpt -w .

# --- Git Worktrees ---
start-task id:
    @echo "Creating worktree for {{id}}..."
    git worktree add -b feat/{{id}} ./worktrees/{{id}} main
    @echo "Worktree ready at ./worktrees/{{id}}"

merge-task id:
    @echo "Merging task {{id}}..."
    git rebase main feat/{{id}}
    git merge --no-ff feat/{{id}} -m "Merge feat/{{id}} into main"
    @echo "Merged feat/{{id}} into main"

cleanup-task id:
    @echo "Cleaning up task {{id}}..."
    git worktree remove ./worktrees/{{id}} --force
    git branch -D feat/{{id}} 2>/dev/null || true
    @echo "Cleaned up feat/{{id}}"

list-worktrees:
    git worktree list

# --- Beads ---
beads-init:
    br init
    @echo "Beads initialized."

beads-status:
    br list --json

beads-ready:
    br ready --json

beads-sync:
    br sync --flush-only

add-task:
    uv run python scripts/add_task.py

# --- Bootstrap ---
bootstrap:
    @echo "Bootstrapping Vibraphone project..."
    @echo "Checking prerequisites..."
    @which git >/dev/null 2>&1 || (echo "git not found" && exit 1)
    @which just >/dev/null 2>&1 || (echo "just not found" && exit 1)
    @which br >/dev/null 2>&1 || (echo "br (beads_rust) not found. Install: cargo install beads_rust" && exit 1)
    @echo "Prerequisites OK."
    cp -n .env.example .env 2>/dev/null || true
    just beads-init
    mkdir -p .vibraphone worktrees
    @echo "Ready. Run /gsd:new-project to start planning."

# --- Review (standalone, for CI) ---
review *FILES:
    uv run python scripts/review.py {{FILES}}
