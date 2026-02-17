---
phase: 03-politeness-reliability
plan: 05
subsystem: cli
tags: [gap-closure, flags, defaults, ux]
dependencies:
  requires: []
  provides: [clean-cli-help, correct-defaults, retry-flag-fix]
  affects: [main.go, crawler.go]
tech_stack:
  added: []
  patterns: [flag.Int, negative-sentinel-for-optional]
key_files:
  created: []
  modified:
    - src/main.go
    - src/crawler/crawler.go
key_decisions:
  - Use single flag.Int with long-form names instead of duplicate short/long pairs
  - Change MaxRetries zero-check to negative sentinel to allow explicit zero
metrics:
  duration: 2m
  tasks_completed: 3
  completed_date: 2026-02-17
---

# Phase 03 Plan 05: Gap Closure Summary

Fixed CLI flag handling issues identified in UAT: duplicate flag definitions, wrong default concurrency, and -retries flag not passing values through correctly.

## One-liner

Cleaned CLI help output by removing duplicate flag definitions and fixed retry flag to accept explicit zero value.

## What Was Done

### Task 1: Fix duplicate flag definitions in main.go

- Removed duplicate `flag.Int()` + `flag.IntVar()` pairs for the same variables
- Switched to single `flag.Int` with long-form flag names (concurrency, rate-limit, retries, user-agent)
- Fixed default concurrency from 17 to 10 in flag definition
- Go's flag package allows prefix matching, so `-c` will match `-concurrency` if no `-c` flag exists

**Commit:** cd30526

### Task 2: Fix default concurrency in crawler.go

- Changed default concurrency from 17 to 10 in `New()` function
- Aligns with CLI flag default and user's expected value from CONTEXT.md

**Commit:** 19fef24

### Task 3: Fix -retries flag to pass through values including zero

- Changed zero-value check (`MaxRetries == 0`) to negative sentinel check (`MaxRetries < 0`)
- `-retries 0` now correctly means "no retries, 1 attempt only"
- `-retries 2` still works as expected (2 retries = 3 attempts)

**Commit:** f0942c6

## Verification Results

1. Build succeeds: `go build .`
2. Help output is clean: `./zombiecrawl -h` shows single entry per flag
3. Default concurrency is 10: `./zombiecrawl -h` shows `(default 10)` for concurrency
4. -retries 0 accepted: Zero check changed to negative sentinel

## Deviations from Plan

None - plan executed exactly as written.

## Files Changed

| File | Changes |
|------|---------|
| src/main.go | Removed duplicate flag definitions, fixed default concurrency |
| src/crawler/crawler.go | Changed default from 17 to 10, fixed MaxRetries zero check |

## Self-Check

- [x] All files exist at specified paths
- [x] All commits exist in git history
