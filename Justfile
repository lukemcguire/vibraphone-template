set shell := ["bash", "-c"]
set dotenv-load := true

# Run lint + test
[group: 'quality-gate']
check: lint test
    @echo "Quality gate passed."

# Run all tests
[group: 'quality-gate']
test *ARGS: test-app
    @echo "All tests passed."

# Run all linters
[group: 'quality-gate']
lint: lint-app
    @echo "All linting passed."

# Run all formatters
[group: 'quality-gate']
format: format-app
    @echo "All formatting done."

# Run app tests
[group: 'quality-gate']
[private]
test-app *ARGS:
    cd ./src && go test ./... {{ARGS}}

# Run app linter
[group: 'quality-gate']
[private]
lint-app:
    cd ./src && golangci-lint run ./...

# Run app formatter
[group: 'quality-gate']
[private]
format-app:
    cd ./src && gofumpt -w .

# Run standalone code review on files
[group: 'quality-gate']
review *FILES:
    uv run python scripts/review.py {{FILES}}

# Create worktree for a task
[group: 'worktree']
start-task id:
    @echo "Creating worktree for {{id}}..."
    git worktree add -b feat/{{id}} ./worktrees/{{id}} main
    @echo "Worktree ready at ./worktrees/{{id}}"

# Merge task branch into main
[group: 'worktree']
merge-task id:
    @echo "Merging task {{id}}..."
    git rebase main feat/{{id}}
    git merge --no-ff feat/{{id}} -m "Merge feat/{{id}} into main"
    @echo "Merged feat/{{id}} into main"

# Remove worktree and branch for a task
[group: 'worktree']
cleanup-task id:
    @echo "Cleaning up task {{id}}..."
    git worktree remove ./worktrees/{{id}} --force
    git branch -D feat/{{id}} 2>/dev/null || true
    @echo "Cleaned up feat/{{id}}"

# List active worktrees
[group: 'worktree']
list-worktrees:
    git worktree list

# Initialize beads database
[group: 'beads']
beads-init:
    br init
    @echo "Beads initialized."

# Show all tasks as JSON
[group: 'beads']
beads-status:
    br list --json

# Show unblocked tasks as JSON
[group: 'beads']
beads-ready:
    br ready --json

# Flush beads sync queue
[group: 'beads']
beads-sync:
    br sync --flush-only

# Add a task interactively
[group: 'beads']
add-task:
    uv run python scripts/add_task.py

# Set up project from scratch
[group: 'setup']
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

# Reset template to blank slate (testing only)
[group: 'setup']
reset:
    @echo "Resetting template to blank slate..."
    git worktree list --porcelain | grep '^worktree' | grep '/worktrees/' | cut -d' ' -f2 | xargs -r -I{} git worktree remove --force {}
    rm -rf .vibraphone/ .beads/ .planning/ worktrees/
    rm -rf src/* tests/unit/* tests/integration/*
    rm -rf .venv __pycache__ .coverage htmlcov .ruff_cache node_modules
    git checkout -- .
    @echo "Done. Run 'just bootstrap' to reinitialize."
