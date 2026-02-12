#!/usr/bin/env bash
set -euo pipefail

# Vibraphone Bootstrap — one-command project setup
# Checks prerequisites, copies .env, initializes beads, creates runtime dirs.

echo "Bootstrapping Vibraphone project..."

# Check prerequisites
missing=0
for cmd in git just br uv; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "Missing: $cmd"
        missing=1
    fi
done

if [ "$missing" -eq 1 ]; then
    echo ""
    echo "Install missing prerequisites:"
    echo "  git     — https://git-scm.com"
    echo "  just    — https://github.com/casey/just"
    echo "  br      — cargo install beads_rust"
    echo "  uv      — curl -LsSf https://astral.sh/uv/install.sh | sh"
    exit 1
fi

echo "Prerequisites OK."

# Copy secrets template if .env doesn't exist
if [ ! -f .env ]; then
    cp .env.example .env
    echo "Created .env from .env.example"
fi

# Initialize beads
just beads-init

# Create runtime directories
mkdir -p .vibraphone worktrees

echo "Ready. Run /gsd:new-project to start planning."
