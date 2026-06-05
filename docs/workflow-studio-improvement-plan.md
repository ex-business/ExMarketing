# ExMarketing Workflow Studio Improvement Plan

This document defines the remaining work needed to move Workflow Studio from a visual workflow editor to a production-ready workflow design, publish, execution, and debugging system.

## Current Scope

Workflow Studio already has the following foundation:

- Visual canvas with nodes and edges.
- Workspace, folder, and workflow entry points.
- Activity workflow draft save.
- Workflow version publish foundation.
- Workflow executor foundation.
- Execution and node execution log foundation.
- Frontend node running state display foundation.

The next work should be completed one stage at a time. Each stage should have backend API coverage, frontend integration, and tests before moving to the next stage.

## Stage 1: Workflow Base Validation

Goal: prevent invalid workflows from being saved, published, or executed.

Required capabilities:

- Validate that exactly one entry node exists, or explicitly support multiple entry nodes.
- Validate node IDs are unique.
- Validate every edge references existing source and target nodes.
- Validate all required nodes are reachable from the entry node.
- Detect unsupported cycles before publish.
- Detect isolated nodes.
- Detect duplicate edge handles where a node output should only connect once.
- Validate workflow metadata contains `activityId`, `workspaceId`, and `folderId` when saved from Studio.

Backend work:

- Add workflow validation service.
- Add validation result model with severity, code, message, nodeId, edgeId, and path.
- Add API endpoint for validating current draft.
- Run validation before publish.

Frontend work:

- Show validation errors in the publish checklist.
- Highlight invalid nodes and edges.
- Clicking a validation item should focus the related node or edge.

Acceptance criteria:

- Invalid draft can still be saved.
- Invalid draft cannot be published.
- Validation errors are deterministic and actionable.

## Stage 2: Node Configuration Schema

Goal: every node type must have structured configuration instead of ad hoc fields.

Required capabilities:

- Each node type defines a schema.
- Schema supports field type, label, description, default value, required flag, enum options, and validation rules.
- Node configuration panel is rendered from schema.
- Node config is saved into workflow draft JSON.
- Node config is validated during publish.

Node types to cover:

- Event In
- Inbox
- Normalize
- Match
- Condition
- Qualify
- Switch
- Fact Query
- Code
- Reward
- Limit
- Quota
- Callback
- Audit

Acceptance criteria:

- A user can configure every node from the property panel.
- Missing required config is visible before publish.
- Config shape is stable for executor handlers.

## Stage 3: Variable System

Goal: users should bind node inputs from upstream outputs through a variable picker.

Required capabilities:

- Every node type declares output variables.
- Every node type declares accepted input variables.
- Variable picker shows available upstream outputs.
- Variable references are saved as structured references.
- Publish validation checks variable references.
- Removed or renamed outputs invalidate dependent nodes.

Recommended variable reference shape:

```json
{
  "sourceNodeId": "node_inbox",
  "outputKey": "event_context",
  "path": "$.user.id"
}
```

Acceptance criteria:

- Users do not need to manually type most variable names.
- Invalid variable references block publish.
- Executor can resolve variables from node execution outputs.

## Stage 4: Edge Routing Rules

Goal: edge labels such as `IF`, `ELIF`, `ELSE`, `PASS`, and `RISK` must become executable routing rules.

Required capabilities:

- Edge stores structured route type.
- Conditional edges store expression or condition reference.
- Branch nodes validate route coverage.
- Default route is explicit.
- Executor evaluates route rules deterministically.

Backend work:

- Define route model.
- Validate route model during publish.
- Add executor route evaluation.

Frontend work:

- Edge editor for condition, route label, and default route.
- Visual distinction between default, conditional, and terminal edges.

Acceptance criteria:

- Branch output is not just a visual label.
- Executor follows the same branch displayed in Studio.

## Stage 5: Publish Checklist

Goal: publish should be a controlled process, not just a button.

Required checks:

- Entry node exists.
- All node IDs and edges are valid.
- All required node config is present.
- All variables resolve.
- All branch routes are valid.
- Reward, quota, and limit nodes have risk-related config.
- Workflow has no unsupported cycles.
- Workflow can be compiled into executor plan.

Frontend work:

- Replace static checklist with real validation results.
- Show pass, warning, and block states.
- Disable publish when block errors exist.

Acceptance criteria:

- Publish checklist reflects real backend validation.
- User can fix every blocking issue from the checklist.

## Stage 6: Version Management

Goal: workflow versions should be inspectable, comparable, and recoverable.

Required capabilities:

- Show current draft version.
- Show active published version.
- Show historical versions.
- Compare draft with active version.
- Compare two historical versions.
- Roll back active version.
- Archive old versions.

Acceptance criteria:

- A published workflow can be rolled back.
- Users can understand what changed before publishing.
- Draft state and active version are clearly distinct.

## Stage 7: Executor Enhancement

Goal: executor must support reliable production execution.

Required capabilities:

- Compile workflow definition into executable DAG.
- Execute nodes in topological order.
- Support branch routing.
- Support skipped node state.
- Support node timeout.
- Support retry policy.
- Support idempotency key.
- Support execution cancellation.
- Support resumed execution when possible.
- Persist execution and node execution logs.

Acceptance criteria:

- The executor can run a published version from an incoming event.
- Failed nodes record clear failure reason.
- Branches and skipped nodes are visible in logs.

## Stage 8: Node Handler Completion

Goal: every node type must have a production handler.

Handler requirements:

- Input validation.
- Config validation.
- Business execution.
- Output payload.
- Error code.
- Timeout behavior.
- Retry behavior.
- Audit fields.

Priority order:

1. Event In
2. Inbox
3. Normalize
4. Match
5. Condition
6. Qualify
7. Reward
8. Limit
9. Quota
10. Fact Query
11. Switch
12. Code
13. Callback
14. Audit

Acceptance criteria:

- Every visual node can execute.
- Every handler has unit tests.
- Handler output is available to downstream variables.

## Stage 9: Run History Detail

Goal: users can inspect exactly what happened during a workflow execution.

Required capabilities:

- Execution list by activity and workflow version.
- Execution detail page or panel.
- Node status overlay on canvas.
- Node input payload viewer.
- Node output payload viewer.
- Error reason and stack/context display.
- Duration and retry count display.
- Branch route trace.

Acceptance criteria:

- A failed execution can be diagnosed from Studio without checking logs manually.
- Users can see which branch was taken.

## Stage 10: Debugging Tools

Goal: users can test workflow logic before production traffic hits it.

Required capabilities:

- Manual test run with payload.
- Payload templates.
- Run draft workflow without publishing.
- Re-run an execution.
- Re-run from a selected node when supported.
- Export execution trace.

Acceptance criteria:

- A user can validate a draft workflow with sample event data.
- Debug runs are clearly separated from production executions.

## Stage 11: Workspace Permission Integration

Goal: workspace, folder, member, share, and audit models must control Studio behavior.

Required capabilities:

- Workspace member management UI.
- Folder visibility controls.
- Share link management UI.
- Audit log view.
- Role-based button visibility.
- Role-based API authorization.

Acceptance criteria:

- Viewers cannot edit or publish.
- Editors can edit drafts.
- Admins can manage workspace and folder settings.
- Owners can manage members and sharing.

## Stage 12: Templates and Reuse

Goal: users should create workflows from proven patterns.

Required capabilities:

- Workflow template catalog.
- Node preset catalog.
- Create workflow from template.
- Save current workflow as template.
- Template versioning.

Recommended templates:

- Complete marketing activity flow.
- Simple reward flow.
- Risk and qualification flow.
- Quota protected reward flow.
- Fact query enrichment flow.

Acceptance criteria:

- New workflow creation does not require starting from a blank canvas.
- Templates produce valid drafts.

## Stage 13: Frontend Experience Details

Goal: Studio should feel reliable and polished during real work.

Required capabilities:

- Dirty state.
- Saving state.
- Saved state.
- Save failure state.
- Publish progress.
- Publish failure details.
- Autosave.
- Keyboard shortcuts.
- Node config error badges.
- Edge error badges.
- Focus invalid node or edge.
- Better empty states.
- Workspace and folder breadcrumb navigation.

Acceptance criteria:

- Users always know whether their changes are saved.
- Errors are visible at the place where they need to be fixed.

## Stage 14: Test Coverage

Goal: protect workflow behavior as the system grows.

Required tests:

- Workflow validation unit tests.
- Workflow HTTP API tests.
- Executor unit tests.
- Branch execution tests.
- Variable resolution tests.
- Publish and rollback tests.
- Node handler tests.
- Frontend workflow picker tests.
- Frontend publish checklist tests.
- End-to-end draft, publish, execute, inspect flow.

Acceptance criteria:

- `go test ./...` passes.
- `npm run typecheck:admin-flow` passes.
- `npm run build:admin-flow` passes.
- Critical workflow flows are covered by automated tests.

## Recommended Implementation Order

The recommended next sequence is:

1. Workflow base validation.
2. Node configuration schema.
3. Variable system.
4. Edge routing rules.
5. Real publish checklist.
6. Version diff and rollback.
7. Executor enhancement.
8. Node handler completion.
9. Run history detail.
10. Debug test run.
11. Workspace permission UI.
12. Templates.
13. Frontend polish.
14. Test coverage expansion.

This order keeps the foundation stable before adding deeper execution and debugging features.

