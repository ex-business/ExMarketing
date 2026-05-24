# LangGraph + LangSmith Studio 集成

## 定位

ExMarketing 里保留两套视图：

```text
ExMarketing /admin/      看需求、状态、产物、日志路径
LangSmith Studio         看 LangGraph 节点、每步输入输出、trace、工具调用
```

两边共享同一套文件：

```text
requirements/backlog.json
tasks/<id>/agent-status.json
tasks/<id>/analysis.md
tasks/<id>/review.md
tasks/<id>/implementation.md
tasks/<id>/test.md
tasks/<id>/commit.md
tasks/<id>/runs/*.log
```

## 安装

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
LANGSMITH_TRACING=true
LANGSMITH_API_KEY=你的 LangSmith Key
EX_MARKETING_ROOT=/Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing
EX_MARKETING_AGENT_CMD=/Applications/Codex.app/Contents/Resources/codex exec --cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing --sandbox danger-full-access --ask-for-approval never -
```

只想本地看图，不上传 trace：

```bash
LANGSMITH_TRACING=false
```

## 启动

```bash
cd /Users/tong/Documents/GitHub/OracleBetX-Projects/ExMarketing/agent-runtime
source .venv/bin/activate
langgraph dev
```

启动后打开：

```text
https://smith.langchain.com/studio/?baseUrl=http://127.0.0.1:2024
```

ExMarketing 管理后台也有入口：

```text
http://localhost:8080/admin/
```

## Studio 输入

运行 `exmarketing_workflow` graph 时输入：

```json
{
  "requirement_id": "2026-05-24-example-automated-requirement"
}
```

## 执行逻辑

LangGraph 节点顺序：

```text
select_requirement
  -> analysis
  -> review
  -> test-plan
  -> dev
  -> test
  -> commit
```

每个节点会：

```text
读取 tasks/<id>/agent-prompts.md 中对应 Agent prompt
调用 EX_MARKETING_AGENT_CMD
写 tasks/<id>/runs/*.log
更新 tasks/<id>/agent-status.json
更新 requirements/backlog.json 状态
```

如果当前状态不是该节点负责的状态，节点会跳过。

状态映射：

```text
ANALYSIS_READY -> analysis
REVIEW_READY   -> review
DEV_READY      -> dev
TEST_READY     -> test
COMMIT_READY   -> commit
```
