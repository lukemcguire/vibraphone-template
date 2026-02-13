set shell := ["bash", "-c"]
set dotenv-load := true

# Run lint + test
[group: 'quality-gate']
check:
    @echo "Error: Run configure_stack to set up quality recipes." && exit 1

# Run all tests
[group: 'quality-gate']
test *ARGS:
    @echo "Error: Run configure_stack to set up quality recipes." && exit 1

# Run all linters
[group: 'quality-gate']
lint:
    @echo "Error: Run configure_stack to set up quality recipes." && exit 1

# Run all formatters
[group: 'quality-gate']
format:
    @echo "Error: Run configure_stack to set up quality recipes." && exit 1

# Run standalone code review on files
[group: 'quality-gate']
review *FILES:
    uv run python scripts/review.py {{FILES}}

# Create worktree for a task
[group: 'worktree']
start-task id:
    @echo "Creating worktree for {{id}}..."
    git fetch origin main
    git worktree add -b feat/{{id}} ./worktrees/{{id}} origin/main
    @echo "Worktree ready at ./worktrees/{{id}}"

# Run quality gate and push task branch
[group: 'worktree']
finish-task id:
    @echo "Finishing task {{id}}..."
    just check
    cd ./worktrees/{{id}} && git push origin feat/{{id}}
    @echo "Pushed feat/{{id}}"

# Remove worktree and branch for a task
[group: 'worktree']
cleanup-task id:
    @echo "Removing worktree for {{id}}..."
    git worktree remove ./worktrees/{{id}} --force
    git branch -D feat/{{id}} 2>/dev/null || true

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

# Reset template to blank slate (testing only — remove before shipping)
[group: 'setup']
reset:
    @echo "Resetting template to blank slate..."
    git worktree list --porcelain | grep '^worktree' | grep '/worktrees/' | cut -d' ' -f2 | xargs -r -I{} git worktree remove --force {}
    rm -rf .vibraphone/ .beads/ .planning/ worktrees/
    rm -rf src/* tests/unit/* tests/integration/*
    rm -rf .venv __pycache__ .coverage htmlcov .ruff_cache node_modules
    git checkout -- .
    @echo "Done. Run 'just bootstrap' to reinitialize."
