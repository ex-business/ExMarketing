# ExMarketing LangGraph Agent Runtime

这个目录把 ExMarketing 的文件驱动 Agent 流程接到 LangGraph / LangSmith Studio。

它不会替代 Go 服务：

```text
Go ExMarketing /admin/      看需求状态、Agent 状态、产物文件
LangSmith Studio            看 LangGraph 节点、调用链、每步输入输出和 trace
```

Agent 流程是：

```text
Analysis -> Review -> Test Plan -> Dev -> Test -> Commit
```

## 准备

建议使用 Python 3.11 或 3.12。

```bash
cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing/agent-runtime
python3.12 -m venv .venv
source .venv/bin/activate
pip install -e .
pip install -U "langgraph-cli[inmem]"
cp .env.example .env
```

编辑 `.env`：

```bash
LANGSMITH_API_KEY=你的 LangSmith Key
LANGSMITH_TRACING=true
EX_MARKETING_ROOT=/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing
EX_MARKETING_AGENT_CMD=/Applications/Codex.app/Contents/Resources/codex exec --cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing --sandbox danger-full-access --ask-for-approval never -
```

如果只想本地看图，不把 trace 发到 LangSmith：

```bash
LANGSMITH_TRACING=false
```

## 启动 Studio

```bash
cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing/agent-runtime
source .venv/bin/activate
langgraph dev
```

命令会输出：

```text
API: http://127.0.0.1:2024
Studio UI: https://smith.langchain.com/studio/?baseUrl=http://127.0.0.1:2024
```

打开 Studio UI，选择 `exmarketing_workflow`。

## 输入

在 Studio 里运行 graph 时传：

```json
{
  "requirement_id": "2026-05-24-example-automated-requirement"
}
```

## 共享文件

LangGraph 节点会读写 ExMarketing 同一套文件：

```text
requirements/backlog.json
tasks/<id>/agent-prompts.md
tasks/<id>/agent-status.json
tasks/<id>/analysis.md
tasks/<id>/review.md
tasks/<id>/test-plan.md
tasks/<id>/implementation.md
tasks/<id>/test.md
tasks/<id>/commit.md
tasks/<id>/runs/*.log
```
