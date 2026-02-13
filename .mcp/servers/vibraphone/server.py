"""Vibraphone MCP Server — FastMCP entry point.

Registers all Vibraphone tools and starts the MCP server on stdio transport.
"""

from fastmcp import FastMCP
from fastmcp.tools import Tool

from tools.beads_tools import (
    abandon_task,
    complete_task,
    health_check,
    list_tasks,
    next_ready,
)
from tools.quality_tools import attempt_commit, run_lint, run_tests
from tools.review_tools import request_code_review
from tools.bridge_tools import import_gsd_plan
from tools.worktree_tools import finish_task, start_task

mcp = FastMCP("vibraphone")

for fn in [
    list_tasks,
    next_ready,
    complete_task,
    abandon_task,
    health_check,
    start_task,
    finish_task,
    run_tests,
    run_lint,
    request_code_review,
    attempt_commit,
    import_gsd_plan,
]:
    mcp.add_tool(Tool.from_function(fn))

if __name__ == "__main__":
    mcp.run()
