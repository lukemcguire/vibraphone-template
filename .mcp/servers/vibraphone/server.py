"""Vibraphone MCP Server — FastMCP entry point.

Registers all Vibraphone tools and starts the MCP server on stdio transport.
"""

from dotenv import load_dotenv

load_dotenv("../../../.env")  # Load from project root

from fastmcp import FastMCP
from fastmcp.tools import Tool

from tools.beads_tools import (
    abandon_task,
    add_task,
    complete_task,
    get_task_context,
    health_check,
    list_tasks,
    next_ready,
    plan_parallel,
    triage,
)
from tools.bridge_tools import import_gsd_plan
from tools.quality_tools import attempt_commit, run_format, run_lint, run_tests
from tools.review_tools import request_code_review
from tools.session_tools import recover_session
from tools.stack_tools import configure_stack
from tools.worktree_tools import cleanup_task, merge_task, start_task

mcp = FastMCP("vibraphone")

for fn in [
    list_tasks,
    next_ready,
    complete_task,
    abandon_task,
    health_check,
    get_task_context,
    add_task,
    triage,
    plan_parallel,
    start_task,
    merge_task,
    cleanup_task,
    run_tests,
    run_lint,
    run_format,
    request_code_review,
    attempt_commit,
    import_gsd_plan,
    recover_session,
    configure_stack,
]:
    mcp.add_tool(Tool.from_function(fn))

if __name__ == "__main__":
    mcp.run()
