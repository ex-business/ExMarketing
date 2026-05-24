from __future__ import annotations

import json
import os
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Literal, TypedDict

from langgraph.graph import END, START, StateGraph


AgentName = Literal["analysis", "review", "test-plan", "dev", "test", "commit"]


class WorkflowState(TypedDict, total=False):
    requirement_id: str
    root: str
    task_dir: str
    workflow_status: str
    next_agent: str
    stopped: bool
    error: str
    log: str
    score: int


AGENT_ORDER: list[AgentName] = ["analysis", "review", "test-plan", "dev", "test", "commit"]
STATUS_TO_AGENT: dict[str, AgentName] = {
    "ANALYSIS_READY": "analysis",
    "REVIEW_READY": "review",
    "TEST_PLAN_READY": "test-plan",
    "TEST_PLAN_DONE": "dev",
    "DEV_READY": "dev",
    "TEST_EXEC_READY": "test",
    "TEST_READY": "test",
    "COMMIT_READY": "commit",
}
AGENT_TO_ARTIFACT: dict[AgentName, str] = {
    "analysis": "analysis.md",
    "review": "review.md",
    "test-plan": "test-plan.md",
    "dev": "implementation.md",
    "test": "test.md",
    "commit": "commit.md",
}
AGENT_TO_PROMPT_HEADING: dict[AgentName, str] = {
    "analysis": "Analysis Agent",
    "review": "Review Agent",
    "test-plan": "Test Plan Agent",
    "dev": "Dev Agent",
    "test": "Test Agent",
    "commit": "Commit Agent",
}


def select_requirement(state: WorkflowState) -> WorkflowState:
    root = Path(state.get("root") or os.getenv("EX_MARKETING_ROOT") or Path.cwd().parent).resolve()
    requirement_id = state["requirement_id"]
    backlog = _read_json(root / "requirements" / "backlog.json")
    for item in backlog.get("items", []):
        if item.get("id") != requirement_id:
            continue
        task_dir = root / item.get("taskDir", f"tasks/{requirement_id}")
        return {
            **state,
            "root": str(root),
            "task_dir": str(task_dir),
            "workflow_status": item.get("status", ""),
            "next_agent": STATUS_TO_AGENT.get(item.get("status", ""), ""),
            "stopped": False,
        }
    return {
        **state,
        "root": str(root),
        "stopped": True,
        "error": f"requirement {requirement_id} not found",
    }


def analysis_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "analysis")


def review_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "review")


def test_plan_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "test-plan")


def dev_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "dev")


def test_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "test")


def commit_node(state: WorkflowState) -> WorkflowState:
    return _run_agent_if_current(state, "commit")


def route_after_select(state: WorkflowState) -> str:
    if state.get("stopped"):
        return END
    return state.get("next_agent") or END


def route_after_analysis(state: WorkflowState) -> str:
    if state.get("workflow_status") == "REVIEW_READY":
        return "review"
    return END


def route_after_review(state: WorkflowState) -> str:
    if state.get("workflow_status") == "TEST_PLAN_READY":
        return "test-plan"
    return END


def route_after_test_plan(state: WorkflowState) -> str:
    if state.get("workflow_status") == "DEV_READY":
        return "dev"
    return END


def route_after_dev(state: WorkflowState) -> str:
    if state.get("workflow_status") in {"TEST_EXEC_READY", "TEST_READY"}:
        return "test"
    return END


def route_after_test(state: WorkflowState) -> str:
    if state.get("workflow_status") == "COMMIT_READY":
        return "commit"
    return END


def _run_agent_if_current(state: WorkflowState, agent: AgentName) -> WorkflowState:
    if state.get("stopped"):
        return state
    expected = STATUS_TO_AGENT.get(state.get("workflow_status", ""))
    if expected != agent:
        return state

    root = Path(state["root"])
    requirement_id = state["requirement_id"]
    task_dir = Path(state["task_dir"])
    command = os.getenv("EX_MARKETING_AGENT_CMD", "").strip()
    log_path = task_dir / "runs" / f"{_stamp()}-{agent}.log"
    log_path.parent.mkdir(parents=True, exist_ok=True)
    _mark_agent(task_dir, requirement_id, agent, "RUNNING", log=str(log_path))

    if not command:
        error = "EX_MARKETING_AGENT_CMD is empty"
        log_path.write_text(error + "\n", encoding="utf-8")
        _mark_agent(task_dir, requirement_id, agent, "FAILED", log=str(log_path), error=error, exit_code=1)
        return {**state, "stopped": True, "error": error, "log": str(log_path)}

    prompt = _prompt_for_agent(task_dir, agent)
    result = subprocess.run(
        command,
        input=prompt,
        cwd=root,
        shell=True,
        text=True,
        capture_output=True,
        check=False,
        env={
            **os.environ,
            "EX_MARKETING_AGENT": agent,
            "EX_MARKETING_REQUIREMENT_ID": requirement_id,
            "EX_MARKETING_TASK_DIR": str(task_dir.relative_to(root)),
            "EX_MARKETING_AGENT_PROMPT": prompt,
        },
    )
    log_path.write_text(result.stdout + result.stderr, encoding="utf-8")
    if result.returncode != 0:
        error = f"agent command exited {result.returncode}"
        _mark_agent(task_dir, requirement_id, agent, "FAILED", log=str(log_path), error=error, exit_code=result.returncode)
        return {**state, "stopped": True, "error": error, "log": str(log_path)}

    next_status, score = _evaluate_agent(task_dir, agent)
    _mark_agent(
        task_dir,
        requirement_id,
        agent,
        "DONE",
        log=str(log_path),
        exit_code=0,
        next_status=next_status,
    )
    _update_backlog(root, requirement_id, next_status, score)
    next_agent = STATUS_TO_AGENT.get(next_status, "")
    out: WorkflowState = {
        **state,
        "workflow_status": next_status,
        "next_agent": next_agent,
        "log": str(log_path),
    }
    if score >= 0:
        out["score"] = score
    return out


def _prompt_for_agent(task_dir: Path, agent: AgentName) -> str:
    content = (task_dir / "agent-prompts.md").read_text(encoding="utf-8")
    heading = AGENT_TO_PROMPT_HEADING[agent]
    match = re.search(rf"(?ms)^## {re.escape(heading)}\s+~~~text\s+(.*?)\s+~~~", content)
    if not match:
        raise ValueError(f"prompt for {heading} not found")
    return match.group(1).strip() + "\n"


def _evaluate_agent(task_dir: Path, agent: AgentName) -> tuple[str, int]:
    if agent == "analysis":
        content = _artifact(task_dir, agent)
        score = _parse_score(content)
        if score >= int(os.getenv("EX_MARKETING_ANALYSIS_PASS_SCORE", "80")) and _parse_token(content, "Decision", ["PASS", "BLOCK"]) == "PASS":
            return "REVIEW_READY", score
        return "REVIEW_BLOCK", score
    if agent == "review":
        content = _artifact(task_dir, agent)
        if _parse_token(content, "Decision", ["PASS", "BLOCK"]) == "PASS":
            return "TEST_PLAN_READY", -1
        return "REVIEW_BLOCK", -1
    if agent == "test-plan":
        content = _artifact(task_dir, agent)
        if _parse_token(content, "Decision", ["PASS", "BLOCK"]) == "PASS":
            return "DEV_READY", -1
        return "TEST_FAIL", -1
    if agent == "dev":
        return "TEST_EXEC_READY", -1
    if agent == "test":
        content = _artifact(task_dir, agent)
        if _parse_token(content, "Result", ["PASS", "FAIL"]) == "PASS":
            return "COMMIT_READY", -1
        return "TEST_FAIL", -1
    if agent == "commit":
        return "COMMITTED", -1
    raise ValueError(f"unknown agent {agent}")


def _artifact(task_dir: Path, agent: AgentName) -> str:
    return (task_dir / AGENT_TO_ARTIFACT[agent]).read_text(encoding="utf-8")


def _parse_score(content: str) -> int:
    section = _section(content, "Score")
    match = re.search(r"\b([0-9]{1,3})\b", section)
    if not match:
        return 0
    return min(100, max(0, int(match.group(1))))


def _parse_token(content: str, heading: str, allowed: list[str]) -> str:
    section = _section(content, heading).upper()
    for token in allowed:
        if re.search(rf"\b{re.escape(token)}\b", section):
            return token
    return ""


def _section(content: str, heading: str) -> str:
    match = re.search(rf"(?im)^##\s+{re.escape(heading)}\s*$", content)
    if not match:
        return content
    rest = content[match.end() :]
    next_heading = re.search(r"(?m)^##\s+", rest)
    if not next_heading:
        return rest
    return rest[: next_heading.start()]


def _mark_agent(
    task_dir: Path,
    requirement_id: str,
    agent: AgentName,
    status: str,
    *,
    log: str = "",
    error: str = "",
    exit_code: int | None = None,
    next_status: str = "",
) -> None:
    path = task_dir / "agent-status.json"
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    status_doc = _read_json(path) if path.exists() else {"requirementId": requirement_id, "agents": {}}
    status_doc.setdefault("requirementId", requirement_id)
    status_doc.setdefault("agents", {})
    current = status_doc["agents"].get(agent, {})
    if status == "RUNNING":
        current = {
            "status": status,
            "startedAt": now,
            "output": str(task_dir / AGENT_TO_ARTIFACT[agent]),
            "log": log,
        }
    else:
        current["status"] = status
        current["finishedAt"] = now
        current["output"] = str(task_dir / AGENT_TO_ARTIFACT[agent])
        if log:
            current["log"] = log
        if exit_code is not None:
            current["exitCode"] = exit_code
        if next_status:
            current["nextStatus"] = next_status
        if error:
            current["error"] = error
        else:
            current.pop("error", None)
    status_doc["agents"][agent] = current
    status_doc["updatedAt"] = now
    _write_json(path, status_doc)


def _update_backlog(root: Path, requirement_id: str, next_status: str, score: int) -> None:
    path = root / "requirements" / "backlog.json"
    backlog = _read_json(path)
    now = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
    for item in backlog.get("items", []):
        if item.get("id") == requirement_id:
            item["status"] = next_status
            item["updatedAt"] = now
            if score >= 0:
                item["score"] = score
            break
    _write_json(path, backlog)


def _read_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def _write_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _stamp() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")


builder = StateGraph(WorkflowState)
builder.add_node("select_requirement", select_requirement)
builder.add_node("analysis", analysis_node)
builder.add_node("review", review_node)
builder.add_node("test-plan", test_plan_node)
builder.add_node("dev", dev_node)
builder.add_node("test", test_node)
builder.add_node("commit", commit_node)

builder.add_edge(START, "select_requirement")
builder.add_conditional_edges("select_requirement", route_after_select, {
    "analysis": "analysis",
    "review": "review",
    "test-plan": "test-plan",
    "dev": "dev",
    "test": "test",
    "commit": "commit",
    END: END,
})
builder.add_conditional_edges("analysis", route_after_analysis, {"review": "review", END: END})
builder.add_conditional_edges("review", route_after_review, {"test-plan": "test-plan", END: END})
builder.add_conditional_edges("test-plan", route_after_test_plan, {"dev": "dev", END: END})
builder.add_conditional_edges("dev", route_after_dev, {"test": "test", END: END})
builder.add_conditional_edges("test", route_after_test, {"commit": "commit", END: END})
builder.add_edge("commit", END)

graph = builder.compile()
