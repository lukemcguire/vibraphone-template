"""Interactive script for adding Beads tasks from the command line."""

from __future__ import annotations

import subprocess
import sys

VALID_TYPES = ("task", "bug", "feature", "epic", "question", "docs")


def prompt(label: str, default: str = "") -> str:
    suffix = f" (default: {default})" if default else ""
    value = input(f"{label}{suffix}: ").strip()
    return value or default


def main() -> None:
    print("\n--- Add Beads Task ---\n")

    title = prompt("Title")
    if not title:
        print("Error: title is required.")
        sys.exit(1)

    type_ = prompt(f"Type [{'/'.join(VALID_TYPES)}]", "task")
    if type_ not in VALID_TYPES:
        print(f"Error: invalid type '{type_}'. Must be one of: {', '.join(VALID_TYPES)}")
        sys.exit(1)

    priority_str = prompt("Priority [0-4]", "2")
    try:
        priority = int(priority_str)
        if not 0 <= priority <= 4:
            raise ValueError
    except ValueError:
        print(f"Error: invalid priority '{priority_str}'. Must be 0-4.")
        sys.exit(1)

    labels = prompt("Labels (comma-separated, optional)")
    description = prompt("Description (optional, press Enter to skip)")
    depends_on_str = prompt("Depends on (task IDs, comma-separated, optional)")
    blocks_str = prompt("Blocks (task IDs, comma-separated, optional)")

    # Build br create command
    cmd = ["br", "create", title, "--type", type_, "--priority", str(priority), "--json"]
    if labels:
        cmd.extend(["--labels", labels])
    if description:
        cmd.extend(["--description", description])

    print("\nCreating task...")
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)

    # Parse the new task ID from JSON output
    import json

    data = json.loads(result.stdout.strip()) if result.stdout.strip() else {}
    new_id = str(data.get("id", data.get("issue_id", "")))

    if not new_id:
        print("Error: could not determine new task ID from br output.")
        print(f"stdout: {result.stdout}")
        sys.exit(1)

    # Wire dependencies
    depends_on = [s.strip() for s in depends_on_str.split(",") if s.strip()] if depends_on_str else []
    blocks = [s.strip() for s in blocks_str.split(",") if s.strip()] if blocks_str else []

    for dep_id in depends_on:
        subprocess.run(
            ["br", "dep", "add", new_id, dep_id, "--type", "blocks"],
            check=True,
        )

    for blocked_id in blocks:
        subprocess.run(
            ["br", "dep", "add", blocked_id, new_id, "--type", "blocks"],
            check=True,
        )

    print(f'\nCreated: {new_id} "{title}"')
    if depends_on:
        print(f"  Depends on: {', '.join(depends_on)}")
    if blocks:
        print(f"  Blocks: {', '.join(blocks)}")


if __name__ == "__main__":
    main()
