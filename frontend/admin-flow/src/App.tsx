import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
} from '@xyflow/react';
import type { Connection, ReactFlowInstance } from '@xyflow/react';
import type { EdgeChange, NodeChange } from '@xyflow/react';
import { ActivityNodeView } from './components/ActivityNodeView';
import { ContextMenu } from './components/ContextMenu';
import { GroupNodeView, NoteNodeView } from './components/FlowExtraNodes';
import { NodePalette } from './components/NodePalette';
import { NodePicker } from './components/NodePicker';
import { PropertyPanel } from './components/PropertyPanel';
import { StudioChrome } from './components/StudioChrome';
import { baseEdges, baseNodes, simpleTemplate } from './data/initialFlow';
import { getDef, nodeCatalog } from './data/nodeCatalog';
import type {
  ActivityEdge,
  ActivityNode,
  ActivityNodeData,
  AlignmentGuide,
  FlowContextMenu,
  FlowNode,
  GroupNode,
  NodePickerState,
  NoteNode,
  OutputAddRequest,
  WorkflowDefinition,
  WorkflowDiffDetail,
  WorkflowDiffItem,
  WorkflowEdgeRoute,
  WorkflowExecution,
  WorkflowExecutionCompareResult,
  WorkflowExecutionJSONDiff,
  WorkflowExecutionTrace,
  WorkflowNodeConfigSchema,
  WorkflowNodeExecution,
  WorkflowValidationIssue,
  WorkflowValidationResult,
  WorkflowVersion,
} from './types';
import { ACTIVITY_NODE_NORMAL_HEIGHT, ACTIVITY_NODE_WIDTH, activityNodeHeight, cx, edge, flowNode, layoutFlowNodes } from './utils/flow';

type ActivitySummary = {
  id: string;
  workspaceId?: string;
  folderId?: string;
  activityCode: string;
  activityName: string;
  tenantId: string;
  status?: string;
  version?: number;
  createdAt?: string;
  updatedAt?: string;
};

type WorkspaceSummary = {
  id: string;
  tenantId: string;
  name: string;
  slug: string;
  color?: string;
  visibility: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  defaultRole?: WorkspaceRole;
  ownerId?: string;
  status: string;
};

type WorkspaceRole = 'OWNER' | 'ADMIN' | 'EDITOR' | 'VIEWER' | 'GUEST';

type WorkspaceFolderSummary = {
  id: string;
  workspaceId: string;
  parentId?: string;
  name: string;
  color?: string;
  visibility: string;
  sortOrder: number;
  status: string;
};

type WorkspaceShareLinkSummary = {
  id: string;
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY';
  permission?: 'VIEW' | 'EDIT';
  enabled: boolean;
  token?: string;
  createdBy?: string;
  createdAt?: string;
};

type WorkspaceMemberSummary = {
  id: string;
  workspaceId: string;
  userId?: string;
  email?: string;
  displayName?: string;
  role: WorkspaceRole;
  status: 'ACTIVE' | 'INVITED' | 'SUSPENDED' | 'REMOVED';
};

type WorkspaceAuditLogSummary = {
  id: string;
  workspaceId: string;
  actorId?: string;
  action: string;
  targetType: string;
  targetId: string;
  createdAt: string;
};

type WorkflowDraftTemplateSummary = {
  id: string;
  versionId: string;
  version: number;
  name: string;
  description?: string;
  category?: string;
  source: 'BUILTIN' | 'SAVED';
  status: 'ACTIVE' | 'ARCHIVED';
  definition: WorkflowDefinition;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

type WorkflowNodePresetSummary = {
  id: string;
  nodeType: string;
  name: string;
  description?: string;
  config?: Record<string, unknown>;
  tags?: string[];
};

type DraftSaveState = 'idle' | 'dirty' | 'saving' | 'saved' | 'failed';
type PublishState = 'idle' | 'publishing' | 'published' | 'failed';

type WorkflowDraftResponse = {
  activityId: string;
  definition: WorkflowDefinition;
};

type ActivityPlacement = {
  workspaceId: string;
  folderId: string;
  shareLinkId?: string;
};

type CreateStudioWorkspaceInput = {
  name: string;
  tenantId: string;
};

type CreateStudioActivityInput = {
  tenantId: string;
  activityName: string;
  activityCode: string;
  workspaceId: string;
  folderId: string;
};

type StudioFolderPatch = {
  parentId?: string;
  visibility?: string;
  sortOrder?: number;
};

type ActivitySettingsInput = {
  activityName: string;
  workspaceId: string;
  folderId: string;
};

type EdgeRouteVisual = {
  className: string;
  color: string;
};

const WORKSPACE_ROLE_RANK: Record<WorkspaceRole, number> = {
  GUEST: 0,
  VIEWER: 1,
  EDITOR: 2,
  ADMIN: 3,
  OWNER: 4,
};

function studioActorId(): string {
  return window.localStorage.getItem('exmarketing.studio.actorId') || 'admin-flow';
}

function workspaceRoleAllows(role: WorkspaceRole, required: WorkspaceRole): boolean {
  return WORKSPACE_ROLE_RANK[role] >= WORKSPACE_ROLE_RANK[required];
}

function replaceUrlParams(patch: Record<string, string | number | undefined | null>) {
  const params = new URLSearchParams(window.location.search);
  Object.entries(patch).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') {
      params.delete(key);
    } else {
      params.set(key, String(value));
    }
  });
  const query = params.toString();
  window.history.replaceState(null, '', query ? `${window.location.pathname}?${query}` : window.location.pathname);
}

type WorkflowRouteTraceItem = {
  edgeId?: string;
  source?: string;
  target?: string;
  label?: string;
  routeType?: string;
  routeKey?: string;
  routeLabel?: string;
  expression?: string;
  conditionRef?: string;
  default?: boolean;
  decision?: string;
  route?: string;
};

type RunHistoryState = {
  open: boolean;
  loading: boolean;
  versionId: string;
  executions: WorkflowExecution[];
  pageLimit: number;
  pageOffset: number;
  hasMore: boolean;
  nextOffset: number;
  selectedExecutionId: string;
  nodeExecutions: WorkflowNodeExecution[];
  selectedNodeExecutionId: string;
  replayStep: number;
  compareExecutionId: string;
  compareNodeExecutions: WorkflowNodeExecution[];
  compareResult?: WorkflowExecutionCompareResult;
  compareLoading: boolean;
};

type JsonDiffRow = WorkflowExecutionJSONDiff;

type DebugPanelState = {
  open: boolean;
  loading: boolean;
  template: string;
  payloadText: string;
  startNodeId: string;
  message: string;
};

type DebugPayloadTemplate = {
  id: string;
  name: string;
  payloadText: string;
  createdAt: string;
};

const DEBUG_PAYLOAD_TEMPLATE_STORAGE_KEY = 'exmarketing.studio.debugPayloadTemplates';

function edgeRouteVisual(edgeItem: ActivityEdge): EdgeRouteVisual {
  const route = edgeItem.data?.route;
  const routeType = route?.type ?? 'DEFAULT';
  const routeKey = (route?.key ?? route?.label ?? (typeof edgeItem.label === 'string' ? edgeItem.label : '')).toUpperCase();
  if (route?.default || routeType === 'DEFAULT' || routeType === 'ELSE') {
    return { className: 'edge-route-default', color: '#aeb8c6' };
  }
  if (routeType === 'BLOCK' || routeType === 'SKIP' || ['BLOCK', 'SKIP', 'FAILED', 'FAIL', 'EXHAUSTED', 'DLQ'].includes(routeKey)) {
    return { className: 'edge-route-terminal', color: '#f97316' };
  }
  if (route?.expression || route?.conditionRef || ['IF', 'ELIF', 'PASS', 'RISK'].includes(routeType)) {
    return { className: 'edge-route-condition', color: '#74a8ff' };
  }
  return { className: 'edge-route-output', color: '#c4ccd8' };
}

function jsonPreview(value: unknown): string {
  if (value === undefined || value === null || value === '') return '-';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function highlightedJsonText(text: string, query: string): React.ReactNode {
  const normalizedQuery = query.trim();
  if (!normalizedQuery) return text;
  const lowerText = text.toLowerCase();
  const lowerQuery = normalizedQuery.toLowerCase();
  const parts: React.ReactNode[] = [];
  let cursor = 0;
  let matchIndex = lowerText.indexOf(lowerQuery);
  while (matchIndex >= 0) {
    if (matchIndex > cursor) parts.push(text.slice(cursor, matchIndex));
    const end = matchIndex + normalizedQuery.length;
    parts.push(<mark key={`${matchIndex}-${end}`}>{text.slice(matchIndex, end)}</mark>);
    cursor = end;
    matchIndex = lowerText.indexOf(lowerQuery, cursor);
  }
  if (cursor < text.length) parts.push(text.slice(cursor));
  return parts;
}

function fallbackCopyText(text: string): boolean {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  const copied = document.execCommand('copy');
  textarea.remove();
  return copied;
}

function jsonValueLabel(value: unknown): string {
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') return JSON.stringify(value);
  if (value === null || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return jsonPreview(value).replace(/\s+/g, ' ').slice(0, 180);
}

function jsonPathChild(path: string, key: string | number): string {
  if (typeof key === 'number') return `${path}[${key}]`;
  return path === '$' ? `$.${key}` : `${path}.${key}`;
}

function diffJsonValues(current: unknown, baseline: unknown, path = '$'): JsonDiffRow[] {
  if (jsonPreview(current) === jsonPreview(baseline)) return [];
  if (current === undefined) return [{ path, type: 'removed', baseline }];
  if (baseline === undefined) return [{ path, type: 'added', current }];
  if (Array.isArray(current) && Array.isArray(baseline)) {
    const length = Math.max(current.length, baseline.length);
    return Array.from({ length }).flatMap((_, index) => diffJsonValues(current[index], baseline[index], jsonPathChild(path, index)));
  }
  if (isRecord(current) && isRecord(baseline)) {
    const keys = new Set([...Object.keys(current), ...Object.keys(baseline)]);
    return [...keys].sort().flatMap((key) => diffJsonValues(current[key], baseline[key], jsonPathChild(path, key)));
  }
  return [{ path, type: 'changed', current, baseline }];
}

function elapsedMs(startedAt?: string, finishedAt?: string): number | undefined {
  if (!startedAt || !finishedAt) return undefined;
  const started = new Date(startedAt).getTime();
  const finished = new Date(finishedAt).getTime();
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) return undefined;
  return finished - started;
}

function timestampMs(value?: string): number | undefined {
  if (!value) return undefined;
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : undefined;
}

function compactDuration(ms: number | undefined): string {
  if (ms === undefined) return 'running';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

function classifyWorkflowError(message?: string): { code: string; category: string; tone: 'neutral' | 'warning' | 'danger' } {
  const text = (message || '').trim();
  if (!text) return { code: 'NONE', category: '无错误', tone: 'neutral' };
  const upper = text.toUpperCase();
  const codeMatch = upper.match(/\b([A-Z][A-Z0-9_]{2,})\b/);
  const code = codeMatch?.[1] ?? 'UNCLASSIFIED_ERROR';
  if (upper.includes('TIMEOUT') || upper.includes('DEADLINE')) return { code, category: '超时', tone: 'danger' };
  if (upper.includes('CONFIG') || upper.includes('MISSING') || upper.includes('REQUIRED') || upper.includes('SCHEMA')) return { code, category: '配置错误', tone: 'danger' };
  if (upper.includes('ROUTE') || upper.includes('BRANCH') || upper.includes('CONDITION') || upper.includes('DEFAULT')) return { code, category: '路由错误', tone: 'danger' };
  if (upper.includes('QUOTA') || upper.includes('LIMIT') || upper.includes('RISK') || upper.includes('QUALIFY')) return { code, category: '风控限制', tone: 'warning' };
  if (upper.includes('HTTP') || upper.includes('WEBHOOK') || upper.includes('CALLBACK') || upper.includes('MQ') || upper.includes('KAFKA')) return { code, category: '外部依赖', tone: 'danger' };
  if (upper.includes('INBOX') || upper.includes('EVENT')) return { code, category: '事件接入', tone: 'danger' };
  return { code, category: '业务执行错误', tone: 'danger' };
}

function workflowRouteTrace(execution?: WorkflowExecution): WorkflowRouteTraceItem[] {
  const output = execution?.output;
  if (!output || typeof output !== 'object') return [];
  const routeTrace = (output as Record<string, unknown>).routeTrace;
  return Array.isArray(routeTrace) ? routeTrace.filter((item): item is WorkflowRouteTraceItem => Boolean(item && typeof item === 'object')) : [];
}

function workflowAttemptCount(output: unknown): number | undefined {
  if (!output || typeof output !== 'object') return undefined;
  const attempt = (output as Record<string, unknown>).attempt;
  if (typeof attempt === 'number') return attempt;
  if (typeof attempt === 'string') {
    const parsed = Number(attempt);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function workflowExecutionModeLabel(execution?: WorkflowExecution): 'DEBUG_DRAFT' | 'PRODUCTION' {
  if (!execution) return 'PRODUCTION';
  if (execution.workflowVersionId?.startsWith('debug-draft')) return 'DEBUG_DRAFT';
  const output = execution.output;
  if (output && typeof output === 'object' && (output as Record<string, unknown>).debug === true) return 'DEBUG_DRAFT';
  return 'PRODUCTION';
}

function traceToRunState(trace: WorkflowExecutionTrace): Pick<RunHistoryState, 'executions' | 'selectedExecutionId' | 'nodeExecutions' | 'selectedNodeExecutionId'> {
  const failed = trace.nodeExecutions.find((item) => item.status === 'FAILED');
  const selectedNode = failed ?? trace.nodeExecutions[0];
  return {
    executions: [trace.execution],
    selectedExecutionId: trace.execution.id,
    nodeExecutions: trace.nodeExecutions,
    selectedNodeExecutionId: selectedNode?.id ?? '',
  };
}

function downloadJSON(filename: string, value: unknown) {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function builtinDebugPayloadTemplate(activity?: ActivitySummary): string {
  return JSON.stringify({
    eventId: `debug_evt_${Date.now()}`,
    eventType: activity?.activityCode ? `${activity.activityCode}_DEBUG` : 'DEPOSIT_SUCCESS',
    sourceSystem: 'studio-debug',
    tenantId: activity?.tenantId || 'site_001',
    subject: { type: 'USER', id: 'debug_user' },
    biz: { bizType: 'DEBUG', bizId: `debug_biz_${Date.now()}` },
    occurredAt: new Date().toISOString(),
    attributes: { amount: '200.00', currency: 'USD' },
  }, null, 2);
}

function loadDebugPayloadTemplates(): DebugPayloadTemplate[] {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(DEBUG_PAYLOAD_TEMPLATE_STORAGE_KEY) || '[]');
    return Array.isArray(parsed) ? parsed.filter((item): item is DebugPayloadTemplate => Boolean(item?.id && item?.name && item?.payloadText)) : [];
  } catch {
    return [];
  }
}

function saveDebugPayloadTemplates(items: DebugPayloadTemplate[]) {
  window.localStorage.setItem(DEBUG_PAYLOAD_TEMPLATE_STORAGE_KEY, JSON.stringify(items.slice(0, 30)));
}

function loadStudioStringSet(key: string): Set<string> {
  try {
    const parsed = JSON.parse(window.localStorage.getItem(key) || '[]');
    return new Set(Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []);
  } catch {
    return new Set();
  }
}

function saveStudioStringSet(key: string, value: Set<string>) {
  window.localStorage.setItem(key, JSON.stringify([...value].slice(0, 200)));
}

function activityTimestamp(activity: ActivitySummary): number {
  const value = activity.updatedAt || activity.createdAt || '';
  const time = value ? Date.parse(value) : 0;
  return Number.isFinite(time) ? time : 0;
}

function formatStudioDate(value?: string): string {
  if (!value) return '暂无时间';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '暂无时间';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function templateMeta(template?: WorkflowDraftTemplateSummary): Record<string, unknown> {
  return (template?.definition?.meta ?? {}) as Record<string, unknown>;
}

function templateMetaString(template: WorkflowDraftTemplateSummary | undefined, key: string): string {
  const value = templateMeta(template)[key];
  return typeof value === 'string' ? value : '';
}

function workflowTemplateValidation(template: WorkflowDraftTemplateSummary): { valid: boolean; message: string } {
  const nodes = template.definition?.nodes ?? [];
  const edges = template.definition?.edges ?? [];
  if (nodes.length === 0) return { valid: false, message: '模板没有节点' };
  if (!nodes.some((node) => node.type === 'event-in' || node.kind === 'entry')) return { valid: false, message: '缺少入口节点' };
  const nodeIds = new Set(nodes.map((node) => node.id));
  const brokenEdge = edges.find((edgeItem) => !nodeIds.has(edgeItem.source) || !nodeIds.has(edgeItem.target));
  if (brokenEdge) return { valid: false, message: `连线 ${brokenEdge.id} 指向不存在节点` };
  return { valid: true, message: '可应用' };
}

function workflowTemplateParametersPayload(parameters: Record<string, string>): Record<string, string | number> {
  const out: Record<string, string | number> = {};
  Object.entries(parameters).forEach(([key, value]) => {
    const trimmed = value.trim();
    if (!trimmed) return;
    if (key === 'rewardAmount' || key === 'limitMaxCount') {
      const number = Number(trimmed);
      if (Number.isFinite(number)) out[key] = number;
      return;
    }
    out[key] = trimmed;
  });
  return out;
}

function workflowExecutionEventPayload(execution?: WorkflowExecution): unknown {
  if (!execution?.input || typeof execution.input !== 'object') return undefined;
  const event = (execution.input as Record<string, unknown>).event;
  return event ?? execution.input;
}

function DebugRunPanel({
  state,
  templates,
  selectedExecutionId,
  selectedNodeId,
  onChange,
  onClose,
  onLoadTemplate,
  onSaveTemplate,
  onUseSelectedExecutionPayload,
  onRunDraft,
  onRerun,
  onContinueFromNode,
  onExportTrace,
}: {
  state: DebugPanelState;
  templates: DebugPayloadTemplate[];
  selectedExecutionId: string;
  selectedNodeId: string;
  onChange: (patch: Partial<DebugPanelState>) => void;
  onClose: () => void;
  onLoadTemplate: (templateId: string) => void;
  onSaveTemplate: () => void;
  onUseSelectedExecutionPayload: () => void;
  onRunDraft: () => void;
  onRerun: () => void;
  onContinueFromNode: () => void;
  onExportTrace: () => void;
}) {
  if (!state.open) return null;
  return (
    <aside className="debug-run-panel">
      <header>
        <div>
          <b>调试运行</b>
          <span>DEBUG_DRAFT · 不影响生产 execution</span>
        </div>
        <button onClick={onClose}>×</button>
      </header>
      <section>
        <label>
          <span>Payload 模板</span>
          <select value={state.template} onChange={(event) => onChange({ template: event.target.value })}>
            <option value="deposit_success">Deposit Success</option>
            <option value="callback_success">Callback Success</option>
            <option value="risk_event">Risk Event</option>
          </select>
        </label>
        <label>
          <span>已保存模板</span>
          <select value="" onChange={(event) => event.target.value && onLoadTemplate(event.target.value)}>
            <option value="">选择本地模板</option>
            {templates.map((template) => (
              <option key={template.id} value={template.id}>{template.name}</option>
            ))}
          </select>
        </label>
        <label>
          <span>测试 Payload JSON</span>
          <textarea
            value={state.payloadText}
            spellCheck={false}
            placeholder="为空时使用模板 payload"
            onChange={(event) => onChange({ payloadText: event.target.value })}
          />
        </label>
        <div className="debug-inline-actions">
          <button type="button" onClick={onSaveTemplate} disabled={!state.payloadText.trim()}>保存为模板</button>
          <button type="button" onClick={onUseSelectedExecutionPayload} disabled={!selectedExecutionId}>使用选中 execution payload</button>
        </div>
        <label>
          <span>从节点继续跑</span>
          <input value={state.startNodeId} placeholder={selectedNodeId || '可选：node id'} onChange={(event) => onChange({ startNodeId: event.target.value })} />
        </label>
        <div className="debug-mode-note">
          <b>DEBUG_DRAFT</b>
          <span>写入独立 debug execution；从节点继续跑时会复用选中 execution 的上游成功节点输出。</span>
        </div>
      </section>
      <div className="debug-actions">
        <button onClick={onRunDraft} disabled={state.loading}>{state.loading ? '运行中' : '运行 Draft'}</button>
        <button onClick={onContinueFromNode} disabled={state.loading || !(state.startNodeId || selectedNodeId)}>从节点继续</button>
        <button onClick={onRerun} disabled={state.loading || !selectedExecutionId}>重跑 Execution</button>
        <button onClick={onExportTrace} disabled={!selectedExecutionId}>导出 Trace</button>
      </div>
      {state.message ? <p>{state.message}</p> : null}
    </aside>
  );
}

function ActivitySettingsModal({
  activity,
  workspaces,
  folders,
  placement,
  saving,
  error,
  onClose,
  onSubmit,
}: {
  activity: ActivitySummary;
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  placement?: { workspaceId: string; folderId: string };
  saving: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (input: ActivitySettingsInput) => void;
}) {
  const [activityName, setActivityName] = useState(activity.activityName);
  const [workspaceId, setWorkspaceId] = useState(activity.workspaceId || placement?.workspaceId || workspaces[0]?.id || '');
  const availableFolders = useMemo(() => folders.filter((folder) => folder.workspaceId === workspaceId && folder.status !== 'DELETED'), [folders, workspaceId]);
  const [folderId, setFolderId] = useState(activity.folderId || placement?.folderId || availableFolders[0]?.id || '');

  useEffect(() => {
    const nextWorkspaceId = activity.workspaceId || placement?.workspaceId || workspaces[0]?.id || '';
    const nextFolders = folders.filter((folder) => folder.workspaceId === nextWorkspaceId && folder.status !== 'DELETED');
    setActivityName(activity.activityName);
    setWorkspaceId(nextWorkspaceId);
    setFolderId(activity.folderId || placement?.folderId || nextFolders[0]?.id || '');
  }, [activity.activityName, activity.folderId, activity.workspaceId, folders, placement?.folderId, placement?.workspaceId, workspaces]);

  useEffect(() => {
    if (!folderId || availableFolders.some((folder) => folder.id === folderId)) return;
    setFolderId(availableFolders[0]?.id || '');
  }, [availableFolders, folderId]);

  return (
    <div className="publish-confirm-backdrop" role="presentation">
      <section className="activity-settings-modal" role="dialog" aria-modal="true" aria-labelledby="activity-settings-title">
        <header>
          <span>
            <b id="activity-settings-title">活动基础信息</b>
            <em>用于 Studio 工作区、文件夹和画布上下文。</em>
          </span>
          <button type="button" onClick={onClose}>x</button>
        </header>
        <div className="activity-settings-grid">
          <label>
            <span>名称</span>
            <input value={activityName} onChange={(event) => setActivityName(event.target.value)} />
          </label>
          <label>
            <span>Code</span>
            <input value={activity.activityCode} disabled />
          </label>
          <label>
            <span>Tenant</span>
            <input value={activity.tenantId} disabled />
          </label>
          <label>
            <span>Workspace</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.filter((workspace) => workspace.status !== 'DELETED').map((workspace) => (
                <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
              ))}
            </select>
          </label>
          <label className="wide">
            <span>Folder</span>
            <select value={folderId} onChange={(event) => setFolderId(event.target.value)}>
              {availableFolders.map((folder) => (
                <option key={folder.id} value={folder.id}>{folder.name} · {folder.visibility}</option>
              ))}
            </select>
          </label>
        </div>
        {error ? <p>{error}</p> : <p>Code 和 tenant 属于活动身份字段，当前只展示不修改。</p>}
        <footer>
          <button type="button" onClick={onClose}>取消</button>
          <button
            className="publish-confirm-primary"
            type="button"
            disabled={saving || !activityName.trim() || !workspaceId || !folderId}
            onClick={() => onSubmit({ activityName, workspaceId, folderId })}
          >
            {saving ? '保存中...' : '保存信息'}
          </button>
        </footer>
      </section>
    </div>
  );
}

function WorkflowTemplatePanel({
  open,
  templates,
  nodePresets,
  workspaces,
  folders,
  activeWorkspaceId,
  activeFolderId,
  canEdit,
  saving,
  message,
  templateVersions,
  loadingVersions,
  onClose,
  onApply,
  onSave,
  onLoadVersions,
}: {
  open: boolean;
  templates: WorkflowDraftTemplateSummary[];
  nodePresets: WorkflowNodePresetSummary[];
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  activeWorkspaceId?: string;
  activeFolderId?: string;
  canEdit: boolean;
  saving: boolean;
  message: string;
  templateVersions: WorkflowDraftTemplateSummary[];
  loadingVersions: boolean;
  onClose: () => void;
  onApply: (templateId: string, input: { versionId?: string; version?: number; parameters: Record<string, string | number> }) => void;
  onSave: (input: { name: string; workspaceId: string; folderId: string; visibility: string }) => void;
  onLoadVersions: (templateId: string) => void;
}) {
  const [saveName, setSaveName] = useState('');
  const [saveWorkspaceId, setSaveWorkspaceId] = useState(activeWorkspaceId || workspaces[0]?.id || '');
  const [saveFolderId, setSaveFolderId] = useState(activeFolderId || '');
  const [saveVisibility, setSaveVisibility] = useState('TEAM');
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [selectedVersionId, setSelectedVersionId] = useState('');
  const [parameters, setParameters] = useState<Record<string, string>>({
    eventType: 'DEPOSIT_SUCCESS',
    rewardAmount: '10',
    currency: 'USD',
    quotaCode: 'DEFAULT_REWARD_BUDGET',
    limitMaxCount: '1',
    callbackProvider: 'default',
  });
  useEffect(() => {
    if (!open) return;
    setSaveWorkspaceId(activeWorkspaceId || workspaces[0]?.id || '');
    setSaveFolderId(activeFolderId || '');
  }, [activeFolderId, activeWorkspaceId, open, workspaces]);
  useEffect(() => {
    if (!open || selectedTemplateId || templates.length === 0) return;
    setSelectedTemplateId(templates[0].id);
    setSelectedVersionId(templates[0].versionId);
    onLoadVersions(templates[0].id);
  }, [onLoadVersions, open, selectedTemplateId, templates]);
  if (!open) return null;
  const grouped = templates.reduce<Record<string, WorkflowDraftTemplateSummary[]>>((acc, item) => {
    const key = item.category || item.source;
    acc[key] = [...(acc[key] ?? []), item];
    return acc;
  }, {});
  const selectedTemplate = templates.find((template) => template.id === selectedTemplateId) ?? templates[0];
  const versions = templateVersions.filter((template) => template.id === selectedTemplate?.id);
  const selectedVersion = versions.find((template) => template.versionId === selectedVersionId) ?? selectedTemplate;
  const selectedValidation = selectedVersion ? workflowTemplateValidation(selectedVersion) : undefined;
  const selectedWorkspaceFolders = folders.filter((folder) => folder.workspaceId === saveWorkspaceId && folder.status !== 'DELETED');
  return (
    <aside className="workflow-template-panel">
      <header>
        <span>
          <b>Workflow Templates</b>
          <em>{templates.length} templates · {nodePresets.length} presets</em>
        </span>
        <button onClick={onClose}>×</button>
      </header>
      <div className="workflow-template-scroll">
        <div className="workflow-template-layout">
          <section className="workflow-template-catalog">
            {Object.entries(grouped).map(([category, items]) => (
              <div key={category}>
                <h3>{category}</h3>
                {items.map((template) => {
                  const validation = workflowTemplateValidation(template);
                  return (
                    <button
                      key={`${template.id}-${template.version}`}
                      className={cx(template.id === selectedTemplate?.id && 'active')}
                      onClick={() => {
                        setSelectedTemplateId(template.id);
                        setSelectedVersionId(template.versionId);
                        onLoadVersions(template.id);
                      }}
                    >
                      <span>
                        <b>{template.name}</b>
                        <em>{template.description || template.id}</em>
                      </span>
                      <strong className={cx(validation.valid ? 'valid' : 'invalid')}>{validation.valid ? '合法' : '阻塞'} · v{template.version}</strong>
                    </button>
                  );
                })}
              </div>
            ))}
          </section>
          <section className="workflow-template-preview">
            <h3>模板预览</h3>
            {selectedVersion ? (
              <>
                <div className="workflow-template-preview-card">
                  <b>{selectedVersion.name}</b>
                  <span>{selectedVersion.description || selectedVersion.id}</span>
                  <div>
                    <strong>{selectedVersion.definition.nodes.length} nodes</strong>
                    <strong>{selectedVersion.definition.edges.length} edges</strong>
                    <strong className={cx(selectedValidation?.valid ? 'valid' : 'invalid')}>{selectedValidation?.message}</strong>
                  </div>
                  <em>{selectedVersion.source} · {templateMetaString(selectedVersion, 'visibility') || 'BUILTIN'} · {formatStudioDate(selectedVersion.updatedAt || selectedVersion.createdAt)}</em>
                </div>
                <div className="workflow-template-node-preview">
                  {selectedVersion.definition.nodes.slice(0, 10).map((node) => (
                    <span key={node.id}>
                      <i>{node.type.slice(0, 1).toUpperCase()}</i>
                      <b>{node.title || node.type}</b>
                      <em>{node.outputs?.join(', ') || 'no output'}</em>
                    </span>
                  ))}
                </div>
                <label>
                  <span>版本</span>
                  <select value={selectedVersionId} onChange={(event) => setSelectedVersionId(event.target.value)}>
                    {(versions.length ? versions : [selectedVersion]).map((version) => (
                      <option key={version.versionId} value={version.versionId}>v{version.version} · {version.status} · {version.source}</option>
                    ))}
                  </select>
                </label>
                {loadingVersions ? <p>版本加载中...</p> : null}
                <div className="workflow-template-params">
                  <h3>业务参数</h3>
                  {[
                    ['eventType', '事件类型'],
                    ['rewardAmount', '奖励金额'],
                    ['currency', '币种'],
                    ['quotaCode', 'Quota Code'],
                    ['limitMaxCount', '次数限制'],
                    ['callbackProvider', '回调 Provider'],
                  ].map(([key, label]) => (
                    <label key={key}>
                      <span>{label}</span>
                      <input value={parameters[key] ?? ''} onChange={(event) => setParameters((state) => ({ ...state, [key]: event.target.value }))} />
                    </label>
                  ))}
                </div>
                <button
                  className="workflow-template-primary"
                  disabled={!canEdit || !selectedValidation?.valid}
                  onClick={() => onApply(selectedVersion.id, {
                    versionId: selectedVersion.versionId,
                    version: selectedVersion.version,
                    parameters: workflowTemplateParametersPayload(parameters),
                  })}
                >
                  从模板创建 Draft
                </button>
              </>
            ) : <p>暂无模板可预览</p>}
          </section>
        </div>
        <section>
          <h3>Node Presets</h3>
          <div className="workflow-preset-grid">
            {nodePresets.slice(0, 12).map((preset) => (
              <div key={preset.id}>
                <b>{preset.name}</b>
                <em>{preset.nodeType}</em>
              </div>
            ))}
          </div>
        </section>
        <section className="workflow-template-save">
          <h3>保存当前 Workflow 为模板</h3>
          <input value={saveName} placeholder="模板名称" onChange={(event) => setSaveName(event.target.value)} />
          <div className="workflow-template-save-grid">
            <label>
              <span>Workspace</span>
              <select value={saveWorkspaceId} onChange={(event) => {
                const workspaceId = event.target.value;
                setSaveWorkspaceId(workspaceId);
                setSaveFolderId(folders.find((folder) => folder.workspaceId === workspaceId && folder.status !== 'DELETED')?.id ?? '');
              }}>
                {workspaces.filter((workspace) => workspace.status !== 'DELETED').map((workspace) => (
                  <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
                ))}
              </select>
            </label>
            <label>
              <span>Folder</span>
              <select value={saveFolderId} onChange={(event) => setSaveFolderId(event.target.value)}>
                <option value="">Workspace Root</option>
                {selectedWorkspaceFolders.map((folder) => (
                  <option key={folder.id} value={folder.id}>{folder.name}</option>
                ))}
              </select>
            </label>
            <label>
              <span>可见性</span>
              <select value={saveVisibility} onChange={(event) => setSaveVisibility(event.target.value)}>
                <option value="PRIVATE">Private</option>
                <option value="TEAM">Team</option>
                <option value="PUBLIC">Public</option>
              </select>
            </label>
          </div>
          <button disabled={!canEdit || saving} onClick={() => onSave({ name: saveName, workspaceId: saveWorkspaceId, folderId: saveFolderId, visibility: saveVisibility })}>
            {saving ? '保存中...' : '保存模板版本'}
          </button>
        </section>
      </div>
      {message ? <p>{message}</p> : null}
    </aside>
  );
}

function PayloadViewer({
  title,
  value,
  query,
  collapsed,
  filename,
  onToggle,
}: {
  title: string;
  value: unknown;
  query: string;
  collapsed: boolean;
  filename: string;
  onToggle: () => void;
}) {
  const text = useMemo(() => jsonPreview(value), [value]);
  const hasQuery = Boolean(query.trim());
  const matched = hasQuery && text.toLowerCase().includes(query.trim().toLowerCase());
  const copyPayload = useCallback(() => {
    void navigator.clipboard?.writeText(text);
  }, [text]);
  return (
    <section className={cx('run-payload-card', collapsed && 'collapsed', hasQuery && (matched ? 'match' : 'miss'))}>
      <div className="run-payload-head">
        <div>
          <h4>{title}</h4>
          {hasQuery ? <span>{matched ? '命中' : '未命中'}</span> : null}
        </div>
        <div>
          <button type="button" onClick={copyPayload}>复制</button>
          <button type="button" onClick={() => downloadJSON(filename, value)}>下载</button>
          <button type="button" onClick={onToggle}>{collapsed ? '展开' : '折叠'}</button>
        </div>
      </div>
      {!collapsed ? <pre>{highlightedJsonText(text, query)}</pre> : null}
    </section>
  );
}

function RunTimeline({
  execution,
  nodeExecutions,
  routeTrace,
  selectedNodeId,
  replayStep,
  onSelectNode,
  onFocusEdge,
  onReplayStepChange,
}: {
  execution?: WorkflowExecution;
  nodeExecutions: WorkflowNodeExecution[];
  routeTrace: WorkflowRouteTraceItem[];
  selectedNodeId?: string;
  replayStep: number;
  onSelectNode: (nodeExecution: WorkflowNodeExecution) => void;
  onFocusEdge: (edgeId: string) => void;
  onReplayStepChange: (step: number) => void;
}) {
  const [playing, setPlaying] = useState(false);
  const appliedInitialReplayRef = useRef('');
  const orderedNodes = useMemo(() => [...nodeExecutions].sort((a, b) => (timestampMs(a.startedAt) ?? 0) - (timestampMs(b.startedAt) ?? 0)), [nodeExecutions]);
  const routeByPair = useMemo(() => {
    const map = new Map<string, WorkflowRouteTraceItem>();
    routeTrace.forEach((item) => {
      if (item.source && item.target) map.set(`${item.source}->${item.target}`, item);
    });
    return map;
  }, [routeTrace]);
  const replaySteps = useMemo(() => {
    const steps: Array<{ type: 'node'; node: WorkflowNodeExecution } | { type: 'route'; route: WorkflowRouteTraceItem }> = [];
    orderedNodes.forEach((nodeExecution, index) => {
      steps.push({ type: 'node', node: nodeExecution });
      const nextNode = orderedNodes[index + 1];
      const route = nextNode ? routeByPair.get(`${nodeExecution.nodeId}->${nextNode.nodeId}`) : undefined;
      if (route) steps.push({ type: 'route', route });
    });
    return steps;
  }, [orderedNodes, routeByPair]);
  const startMs = timestampMs(execution?.startedAt) ?? timestampMs(orderedNodes[0]?.startedAt);
  const totalMs = elapsedMs(execution?.startedAt, execution?.finishedAt);
  const activeStepIndex = Math.max(0, Math.min(replayStep, Math.max(replaySteps.length - 1, 0)));
  const activeStep = replaySteps[activeStepIndex];
  const applyReplayStep = useCallback((index: number) => {
    if (!replaySteps.length) return;
    const nextIndex = Math.max(0, Math.min(index, replaySteps.length - 1));
    onReplayStepChange(nextIndex);
    const step = replaySteps[nextIndex];
    if (step.type === 'node') {
      onSelectNode(step.node);
    } else if (step.route.edgeId) {
      onFocusEdge(step.route.edgeId);
    }
  }, [onFocusEdge, onReplayStepChange, onSelectNode, replaySteps]);
  useEffect(() => {
    setPlaying(false);
  }, [execution?.id]);
  useEffect(() => {
    const key = `${execution?.id ?? ''}:${replaySteps.length}`;
    if (!execution?.id || !replaySteps.length || replayStep <= 0 || appliedInitialReplayRef.current === key) return;
    appliedInitialReplayRef.current = key;
    applyReplayStep(activeStepIndex);
  }, [activeStepIndex, applyReplayStep, execution?.id, replayStep, replaySteps.length]);
  useEffect(() => {
    if (!playing || !replaySteps.length) return undefined;
    const timer = window.setInterval(() => {
      const next = activeStepIndex + 1;
      if (next >= replaySteps.length) {
        window.clearInterval(timer);
        setPlaying(false);
        return;
      }
      applyReplayStep(next);
    }, 900);
    return () => window.clearInterval(timer);
  }, [activeStepIndex, applyReplayStep, playing, replaySteps.length]);
  if (!orderedNodes.length) return null;
  return (
    <section className="run-timeline">
      <div className="run-timeline-head">
        <div>
          <h3>Execution Timeline</h3>
          <span>{orderedNodes.length} nodes · {replaySteps.length} steps · {compactDuration(totalMs)}</span>
        </div>
        <em>{execution?.status ?? 'UNKNOWN'}</em>
      </div>
      <div className="run-timeline-controls">
        <button type="button" onClick={() => applyReplayStep(0)} disabled={!replaySteps.length}>重置</button>
        <button type="button" onClick={() => applyReplayStep(activeStepIndex - 1)} disabled={activeStepIndex <= 0}>上一步</button>
        <button type="button" className="primary" onClick={() => setPlaying((value) => !value)} disabled={!replaySteps.length || activeStepIndex >= replaySteps.length - 1}>
          {playing ? '暂停' : '播放'}
        </button>
        <button type="button" onClick={() => applyReplayStep(activeStepIndex + 1)} disabled={activeStepIndex >= replaySteps.length - 1}>下一步</button>
        <span>{replaySteps.length ? `${activeStepIndex + 1}/${replaySteps.length}` : '0/0'} · {activeStep?.type === 'node' ? (activeStep.node.nodeTitle || activeStep.node.nodeId) : (activeStep?.route.routeLabel || activeStep?.route.routeKey || activeStep?.route.routeType || 'route')}</span>
      </div>
      <div className="run-timeline-list">
        {orderedNodes.map((nodeExecution, index) => {
          const started = timestampMs(nodeExecution.startedAt);
          const offset = startMs !== undefined && started !== undefined ? started - startMs : undefined;
          const duration = elapsedMs(nodeExecution.startedAt, nodeExecution.finishedAt);
          const attempts = workflowAttemptCount(nodeExecution.output);
          const nextNode = orderedNodes[index + 1];
          const route = nextNode ? routeByPair.get(`${nodeExecution.nodeId}->${nextNode.nodeId}`) : undefined;
          const nodeStepIndex = replaySteps.findIndex((step) => step.type === 'node' && step.node.id === nodeExecution.id);
          const routeStepIndex = route ? replaySteps.findIndex((step) => step.type === 'route' && step.route === route) : -1;
          return (
            <React.Fragment key={nodeExecution.id}>
              <button
                type="button"
                className={cx('run-timeline-node', `run-${nodeExecution.status.toLowerCase()}`, nodeExecution.id === selectedNodeId && 'active', nodeStepIndex === activeStepIndex && 'replay-active', nodeStepIndex < activeStepIndex && 'replay-done')}
                onClick={() => {
                  setPlaying(false);
                  applyReplayStep(nodeStepIndex);
                }}
              >
                <i />
                <span>{offset === undefined ? '+--' : `+${compactDuration(offset)}`}</span>
                <b>{nodeExecution.nodeTitle || nodeExecution.nodeId}</b>
                <em>{nodeExecution.status} · {compactDuration(duration)}{attempts && attempts > 1 ? ` · retry ${attempts - 1}` : ''}</em>
                {nodeExecution.errorMessage ? <strong>{nodeExecution.errorMessage}</strong> : null}
              </button>
              {route ? (
                <button
                  type="button"
                  className={cx('run-timeline-route', routeStepIndex === activeStepIndex && 'replay-active', routeStepIndex < activeStepIndex && 'replay-done')}
                  onClick={() => {
                    setPlaying(false);
                    applyReplayStep(routeStepIndex);
                  }}
                >
                  <i />
                  <b>{route.routeLabel || route.routeKey || route.routeType || route.label || 'route'}</b>
                  <span>{route.source} → {route.target}</span>
                  <em>{route.default ? 'DEFAULT' : route.expression || route.conditionRef || route.decision || route.route || 'selected'}</em>
                </button>
              ) : null}
            </React.Fragment>
          );
        })}
      </div>
    </section>
  );
}

function nodeExecutionTotalMs(items: WorkflowNodeExecution[]): number {
  return items.reduce((sum, item) => sum + (elapsedMs(item.startedAt, item.finishedAt) ?? 0), 0);
}

function executionPath(items: WorkflowNodeExecution[]): string[] {
  return [...items]
    .sort((a, b) => (timestampMs(a.startedAt) ?? 0) - (timestampMs(b.startedAt) ?? 0))
    .map((item) => item.nodeId);
}

function failedNodeLabels(items: WorkflowNodeExecution[]): string[] {
  return items.filter((item) => item.status === 'FAILED').map((item) => item.nodeTitle || item.nodeId);
}

function payloadChangeDetails(current: WorkflowNodeExecution[], baseline: WorkflowNodeExecution[]): Array<{ nodeId: string; label: string; input: JsonDiffRow[]; output: JsonDiffRow[] }> {
  const baselineByNode = new Map(baseline.map((item) => [item.nodeId, item]));
  return current
    .map((item) => {
      const target = baselineByNode.get(item.nodeId);
      return {
        nodeId: item.nodeId,
        label: item.nodeTitle || item.nodeId,
        input: diffJsonValues(item.input, target?.input),
        output: diffJsonValues(item.output, target?.output),
      };
    })
    .filter((item) => item.input.length > 0 || item.output.length > 0);
}

function RunExecutionCompare({
  currentExecution,
  baselineExecution,
  currentNodes,
  baselineNodes,
  compareResult,
  loading,
}: {
  currentExecution?: WorkflowExecution;
  baselineExecution?: WorkflowExecution;
  currentNodes: WorkflowNodeExecution[];
  baselineNodes: WorkflowNodeExecution[];
  compareResult?: WorkflowExecutionCompareResult;
  loading: boolean;
}) {
  const [expandedPayloadNodeId, setExpandedPayloadNodeId] = useState('');
  if (!baselineExecution) return null;
  const currentPath = compareResult?.currentPath ?? executionPath(currentNodes);
  const baselinePath = compareResult?.baselinePath ?? executionPath(baselineNodes);
  const pathChanged = compareResult?.pathChanged ?? currentPath.join(' -> ') !== baselinePath.join(' -> ');
  const currentFailures = compareResult?.currentFailures ?? failedNodeLabels(currentNodes);
  const baselineFailures = compareResult?.baselineFailures ?? failedNodeLabels(baselineNodes);
  const currentDuration = compareResult?.currentDurationMs ?? nodeExecutionTotalMs(currentNodes);
  const baselineDuration = compareResult?.baselineDurationMs ?? nodeExecutionTotalMs(baselineNodes);
  const durationDelta = compareResult?.durationDeltaMs ?? currentDuration - baselineDuration;
  const payloadChanges = compareResult?.payloadChanges ?? payloadChangeDetails(currentNodes, baselineNodes);
  return (
    <section className="run-compare-panel">
      <div className="run-compare-head">
        <h3>Execution Compare</h3>
        <span>{loading ? '加载对比中' : `${currentExecution?.id.slice(0, 10) ?? 'current'} vs ${baselineExecution.id.slice(0, 10)}`}</span>
      </div>
      <div className="run-compare-grid">
        <div className={cx(pathChanged && 'changed')}>
          <b>路径</b>
          <span>{pathChanged ? '不同' : '一致'}</span>
          <em>{currentPath.join(' -> ') || '-'}</em>
        </div>
        <div className={cx(currentFailures.join('|') !== baselineFailures.join('|') && 'changed')}>
          <b>失败节点</b>
          <span>{currentFailures.length ? currentFailures.join(', ') : '无失败'}</span>
          <em>baseline: {baselineFailures.length ? baselineFailures.join(', ') : '无失败'}</em>
        </div>
        <div className={cx(durationDelta !== 0 && 'changed')}>
          <b>耗时差异</b>
          <span>{durationDelta >= 0 ? '+' : ''}{compactDuration(durationDelta)}</span>
          <em>current {compactDuration(currentDuration)} / baseline {compactDuration(baselineDuration)}</em>
        </div>
        <div className={cx(payloadChanges.length > 0 && 'changed')}>
          <b>Payload</b>
          <span>{payloadChanges.length ? `${payloadChanges.length} 个节点变化` : '一致'}</span>
          <em>{payloadChanges.slice(0, 3).map((item) => item.label).join(', ') || 'input/output 未变化'}</em>
        </div>
      </div>
      {payloadChanges.length ? (
        <div className="run-compare-payload-diff">
          <h4>Payload Diff</h4>
          {compareResult?.truncated ? (
            <p>结果已按服务端上限裁剪：最多 {compareResult.payloadChangeLimit ?? payloadChanges.length} 个变化节点，每组最多 {compareResult.jsonDiffLimit ?? 24} 条 JSON diff。</p>
          ) : null}
          {payloadChanges.map((item) => {
            const expanded = expandedPayloadNodeId === item.nodeId;
            const rows = [...item.input.map((row) => ({ ...row, scope: 'Input' })), ...item.output.map((row) => ({ ...row, scope: 'Output' }))].slice(0, 24);
            return (
              <section key={item.nodeId} className={cx(expanded && 'expanded')}>
                <button type="button" onClick={() => setExpandedPayloadNodeId(expanded ? '' : item.nodeId)}>
                  <span>{item.label}</span>
                  <em>{item.input.length} input · {item.output.length} output</em>
                </button>
                {expanded ? (
                  <div className="run-json-diff-rows">
                    {rows.map((row, index) => (
                      <div key={`${row.scope}-${row.path}-${index}`} className={row.type}>
                        <b>{row.scope}</b>
                        <code>{row.path}</code>
                        <span>{row.type}</span>
                        <em>current: {jsonValueLabel(row.current)}</em>
                        <em>baseline: {jsonValueLabel(row.baseline)}</em>
                      </div>
                    ))}
                    {item.input.length + item.output.length > rows.length ? <p>仅显示前 {rows.length} 条差异</p> : null}
                  </div>
                ) : null}
              </section>
            );
          })}
        </div>
      ) : null}
    </section>
  );
}

function RunHistoryPanel({
  state,
  workflowVersions,
  onClose,
  onRefresh,
  onVersionChange,
  onSelectExecution,
  onCompareExecution,
  onSelectNode,
  onPageChange,
  onExportTrace,
  onFocusEdge,
  onReplayStepChange,
}: {
  state: RunHistoryState;
  workflowVersions: WorkflowVersion[];
  onClose: () => void;
  onRefresh: () => void;
  onVersionChange: (versionId: string) => void;
  onSelectExecution: (executionId: string) => void;
  onCompareExecution: (executionId: string) => void;
  onSelectNode: (nodeExecution: WorkflowNodeExecution) => void;
  onPageChange: (offset: number) => void;
  onExportTrace: () => void;
  onFocusEdge: (edgeId: string) => void;
  onReplayStepChange: (step: number) => void;
}) {
  const [payloadQuery, setPayloadQuery] = useState('');
  const [collapsedPayloads, setCollapsedPayloads] = useState({ input: false, output: false });
  const [shareLinkStatus, setShareLinkStatus] = useState('');
  if (!state.open) return null;
  const selectedExecution = state.executions.find((item) => item.id === state.selectedExecutionId);
  const compareExecution = state.executions.find((item) => item.id === state.compareExecutionId);
  const selectedNode = state.nodeExecutions.find((item) => item.id === state.selectedNodeExecutionId) ?? state.nodeExecutions[0];
  const trace = workflowRouteTrace(selectedExecution);
  const selectedDuration = elapsedMs(selectedNode?.startedAt, selectedNode?.finishedAt);
  const selectedAttempts = workflowAttemptCount(selectedNode?.output);
  const executionError = classifyWorkflowError(selectedExecution?.errorMessage);
  const nodeError = classifyWorkflowError(selectedNode?.errorMessage);
  const copyCurrentRunLink = async () => {
    if (!selectedExecution) return;
    const url = new URL(window.location.href);
    url.searchParams.set('executionId', selectedExecution.id);
    url.searchParams.set('replayStep', String(Math.max(0, state.replayStep)));
    if (state.compareExecutionId) {
      url.searchParams.set('compareExecutionId', state.compareExecutionId);
    } else {
      url.searchParams.delete('compareExecutionId');
    }
    try {
      const link = url.toString();
      try {
        if (navigator.clipboard?.writeText) {
          await navigator.clipboard.writeText(link);
        } else if (!fallbackCopyText(link)) {
          throw new Error('copy command failed');
        }
      } catch {
        if (!fallbackCopyText(link)) throw new Error('copy command failed');
      }
      setShareLinkStatus('已复制');
    } catch {
      setShareLinkStatus('复制失败');
    }
    window.setTimeout(() => setShareLinkStatus(''), 1800);
  };
  return (
    <aside className="run-history-panel">
      <header>
        <div>
          <b>运行历史</b>
          <span>{shareLinkStatus || (selectedExecution ? `execution ${selectedExecution.id.slice(0, 12)}` : '选择一次执行')}</span>
        </div>
        <div className="run-history-header-actions">
          <button type="button" className="share" onClick={onExportTrace} disabled={!selectedExecution}>导出 Trace</button>
          <button type="button" className="share" onClick={() => void copyCurrentRunLink()} disabled={!selectedExecution}>复制定位链接</button>
          <button type="button" onClick={onClose}>×</button>
        </div>
      </header>
      <div className="run-history-filters">
        <select value={state.versionId} onChange={(event) => onVersionChange(event.target.value)}>
          <option value="">全部版本</option>
          {workflowVersions.map((version) => (
            <option key={version.id} value={version.id}>workflow v{version.version} · {version.status}</option>
          ))}
        </select>
        <button onClick={onRefresh} disabled={state.loading}>{state.loading ? '加载中' : '刷新'}</button>
      </div>
      <div className="run-compare-selector">
        <span>对比</span>
        <select value={state.compareExecutionId} onChange={(event) => onCompareExecution(event.target.value)} disabled={!state.executions.length}>
          <option value="">不对比</option>
          {state.executions.filter((execution) => execution.id !== state.selectedExecutionId).map((execution) => (
            <option key={execution.id} value={execution.id}>{execution.status} · {execution.eventId || execution.id.slice(0, 10)}</option>
          ))}
        </select>
      </div>
      <div className="run-history-grid">
        <section className="run-history-list">
          <h3>Executions</h3>
          {state.executions.length ? state.executions.map((execution) => (
            <button
              key={execution.id}
              className={cx('run-execution-item', execution.id === state.selectedExecutionId && 'active', `run-${execution.status.toLowerCase()}`, workflowExecutionModeLabel(execution) === 'DEBUG_DRAFT' && 'debug-execution')}
              onClick={() => onSelectExecution(execution.id)}
            >
              <b>{execution.status}</b>
              <span>{workflowExecutionModeLabel(execution)} · v{execution.workflowVersion} · {execution.eventId || 'no event'}</span>
              <em>{new Date(execution.startedAt).toLocaleString()}</em>
            </button>
          )) : <div className="run-empty">暂无执行记录</div>}
          <div className="run-history-pagination">
            <button type="button" disabled={state.loading || state.pageOffset <= 0} onClick={() => onPageChange(Math.max(0, state.pageOffset - state.pageLimit))}>上一页</button>
            <span>{state.executions.length ? `${state.pageOffset + 1}-${state.pageOffset + state.executions.length}` : '0-0'}</span>
            <button type="button" disabled={state.loading || !state.hasMore} onClick={() => onPageChange(state.nextOffset)}>下一页</button>
          </div>
        </section>
        <section className="run-node-list">
          <h3>节点路径</h3>
          {state.nodeExecutions.length ? state.nodeExecutions.map((nodeExecution) => {
            const duration = elapsedMs(nodeExecution.startedAt, nodeExecution.finishedAt);
            const attempts = workflowAttemptCount(nodeExecution.output);
            return (
              <button
                key={nodeExecution.id}
                className={cx('run-node-item', nodeExecution.id === selectedNode?.id && 'active', `run-${nodeExecution.status.toLowerCase()}`)}
                onClick={() => onSelectNode(nodeExecution)}
              >
                <i />
                <b>{nodeExecution.nodeTitle || nodeExecution.nodeId}</b>
                <span>{nodeExecution.status}</span>
                <em>{duration === undefined ? '运行中' : `${duration}ms`}{attempts && attempts > 1 ? ` · retry ${attempts - 1}` : ''}</em>
              </button>
            );
          }) : <div className="run-empty">选择 execution 后显示节点日志</div>}
        </section>
        <section className="run-detail">
          <div className="run-detail-title">
            <h3>详情</h3>
            <input value={payloadQuery} onChange={(event) => setPayloadQuery(event.target.value)} placeholder="搜索 JSON" />
          </div>
          {selectedExecution ? (
            <div className="run-detail-summary">
              <span>{selectedExecution.status}</span>
              <b>{workflowExecutionModeLabel(selectedExecution)} · workflow v{selectedExecution.workflowVersion}</b>
              <em>{selectedExecution.errorMessage || '无 execution 级错误'}</em>
            </div>
          ) : null}
          {selectedExecution ? (
            <div className="run-execution-detail">
              <div><span>Execution ID</span><b>{selectedExecution.id}</b></div>
              <div><span>Event</span><b>{selectedExecution.eventId || '-'}</b></div>
              <div><span>Version ID</span><b>{selectedExecution.workflowVersionId || '-'}</b></div>
              <div><span>Started</span><b>{new Date(selectedExecution.startedAt).toLocaleString()}</b></div>
              <div><span>Duration</span><b>{compactDuration(elapsedMs(selectedExecution.startedAt, selectedExecution.finishedAt))}</b></div>
              <div className={cx('run-error-class', executionError.tone)}>
                <span>错误分类</span>
                <b>{executionError.category}</b>
                <em>{executionError.code}</em>
              </div>
            </div>
          ) : null}
          <RunExecutionCompare
            currentExecution={selectedExecution}
            baselineExecution={compareExecution}
            currentNodes={state.nodeExecutions}
            baselineNodes={state.compareNodeExecutions}
            compareResult={state.compareResult}
            loading={state.compareLoading}
          />
          <RunTimeline
            execution={selectedExecution}
            nodeExecutions={state.nodeExecutions}
            routeTrace={trace}
            selectedNodeId={selectedNode?.id}
            replayStep={state.replayStep}
            onSelectNode={onSelectNode}
            onFocusEdge={onFocusEdge}
            onReplayStepChange={onReplayStepChange}
          />
          {selectedNode ? (
            <>
              <div className="run-detail-summary node">
                <span>{selectedNode.status}</span>
                <b>{selectedNode.nodeTitle || selectedNode.nodeId}</b>
                <em>{selectedNode.errorMessage || `耗时 ${selectedDuration ?? 0}ms · retry ${Math.max((selectedAttempts ?? 1) - 1, 0)}`}</em>
                {selectedNode.errorMessage ? <i>{nodeError.category} · {nodeError.code}</i> : null}
              </div>
              <div className="run-payload-tabs">
                <PayloadViewer
                  title="Input Payload"
                  value={selectedNode.input}
                  query={payloadQuery}
                  collapsed={collapsedPayloads.input}
                  filename={`workflow-node-input-${selectedNode.id}.json`}
                  onToggle={() => setCollapsedPayloads((value) => ({ ...value, input: !value.input }))}
                />
                <PayloadViewer
                  title="Output Payload"
                  value={selectedNode.output}
                  query={payloadQuery}
                  collapsed={collapsedPayloads.output}
                  filename={`workflow-node-output-${selectedNode.id}.json`}
                  onToggle={() => setCollapsedPayloads((value) => ({ ...value, output: !value.output }))}
                />
              </div>
            </>
          ) : null}
          <div className="run-route-trace">
            <h3>Route Trace</h3>
            {trace.length ? trace.map((item, index) => (
              <button key={`${item.edgeId}-${index}`} onClick={() => item.edgeId && onFocusEdge(item.edgeId)}>
                <b>{item.routeLabel || item.routeKey || item.routeType || item.label || 'route'}</b>
                <span>{item.source} → {item.target}</span>
                <em>{item.default ? 'DEFAULT' : item.expression || item.conditionRef || item.decision || item.route || 'selected'}</em>
              </button>
            )) : <div className="run-empty">暂无分支 trace</div>}
          </div>
        </section>
      </div>
    </aside>
  );
}

function comparableWorkflowDefinition(definition: WorkflowDefinition | undefined): WorkflowDefinition | undefined {
  if (!definition) return undefined;
  return {
    ...definition,
    viewport: undefined,
    meta: definition.meta ? { ...definition.meta, exportedAt: undefined } : undefined,
    nodes: [...(definition.nodes ?? [])].sort((a, b) => a.id.localeCompare(b.id)),
    edges: [...(definition.edges ?? [])].sort((a, b) => a.id.localeCompare(b.id)),
  };
}

function stableWorkflowString(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableWorkflowString).join(',')}]`;
  if (value && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([key, item]) => `${JSON.stringify(key)}:${stableWorkflowString(item)}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

function workflowDiffDetail(left: WorkflowDefinition | undefined, right: WorkflowDefinition | undefined, empty = '缺少对比版本'): WorkflowDiffDetail {
  const a = comparableWorkflowDefinition(left);
  const b = comparableWorkflowDefinition(right);
  if (!a || !b) return { changed: Boolean(a || b), summary: empty, detail: [], items: [] };
  if (stableWorkflowString(a) === stableWorkflowString(b)) {
    return { changed: false, summary: '无差异', detail: ['节点、连线、配置和 route 均一致。'], items: [] };
  }
  const detail: string[] = [];
  const items: WorkflowDiffItem[] = [];
  const leftNodes = new Map(a.nodes.map((node) => [node.id, node]));
  const rightNodes = new Map(b.nodes.map((node) => [node.id, node]));
  const leftEdges = new Map(a.edges.map((edgeItem) => [edgeItem.id, edgeItem]));
  const rightEdges = new Map(b.edges.map((edgeItem) => [edgeItem.id, edgeItem]));
  const nodeAdded = [...leftNodes.keys()].filter((id) => !rightNodes.has(id));
  const nodeRemoved = [...rightNodes.keys()].filter((id) => !leftNodes.has(id));
  const nodeChanged = [...leftNodes.keys()].filter((id) => rightNodes.has(id) && stableWorkflowString(leftNodes.get(id)) !== stableWorkflowString(rightNodes.get(id)));
  const edgeAdded = [...leftEdges.keys()].filter((id) => !rightEdges.has(id));
  const edgeRemoved = [...rightEdges.keys()].filter((id) => !leftEdges.has(id));
  const edgeChanged = [...leftEdges.keys()].filter((id) => rightEdges.has(id) && stableWorkflowString(leftEdges.get(id)) !== stableWorkflowString(rightEdges.get(id)));
  if (nodeAdded.length) detail.push(`新增节点：${nodeAdded.join(', ')}`);
  if (nodeRemoved.length) detail.push(`删除节点：${nodeRemoved.join(', ')}`);
  if (nodeChanged.length) detail.push(`变更节点：${nodeChanged.join(', ')}`);
  if (edgeAdded.length) detail.push(`新增连线：${edgeAdded.join(', ')}`);
  if (edgeRemoved.length) detail.push(`删除连线：${edgeRemoved.join(', ')}`);
  if (edgeChanged.length) detail.push(`变更连线：${edgeChanged.join(', ')}`);
  nodeAdded.forEach((id) => {
    const node = leftNodes.get(id);
    items.push({ id: `node-added-${id}`, kind: 'NODE_ADDED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: 'Draft 新增节点' });
  });
  nodeRemoved.forEach((id) => {
    const node = rightNodes.get(id);
    items.push({ id: `node-removed-${id}`, kind: 'NODE_REMOVED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: 'Active 中存在，Draft 已删除' });
  });
  nodeChanged.forEach((id) => {
    const node = leftNodes.get(id);
    items.push({ id: `node-changed-${id}`, kind: 'NODE_CHANGED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: '节点位置、配置、输入或输出发生变化' });
  });
  edgeAdded.forEach((id) => {
    const edgeItem = leftEdges.get(id);
    items.push({ id: `edge-added-${id}`, kind: 'EDGE_ADDED', label: edgeItem?.label || id, edgeId: id, source: edgeItem?.source, target: edgeItem?.target, detail: 'Draft 新增连线' });
  });
  edgeRemoved.forEach((id) => {
    const edgeItem = rightEdges.get(id);
    items.push({ id: `edge-removed-${id}`, kind: 'EDGE_REMOVED', label: edgeItem?.label || id, edgeId: id, source: edgeItem?.source, target: edgeItem?.target, detail: 'Active 中存在，Draft 已删除' });
  });
  edgeChanged.forEach((id) => {
    const edgeItem = leftEdges.get(id);
    items.push({ id: `edge-changed-${id}`, kind: 'EDGE_CHANGED', label: edgeItem?.label || id, edgeId: id, source: edgeItem?.source, target: edgeItem?.target, detail: '连线端点、handle、label 或 route 发生变化' });
  });
  return {
    changed: true,
    summary: `节点 +${nodeAdded.length} / -${nodeRemoved.length} / ~${nodeChanged.length}，连线 +${edgeAdded.length} / -${edgeRemoved.length} / ~${edgeChanged.length}`,
    detail,
    items,
  };
}

function isTestActivity(activity: ActivitySummary): boolean {
  const text = `${activity.activityName} ${activity.activityCode} ${activity.id}`.toLowerCase();
  return [
    'postgresql e2e',
    'load fault',
    'smoke',
    'e2e_',
    'load_fault',
    'pg_smoke',
  ].some((marker) => text.includes(marker));
}

function workspaceRoleForActor(workspace: WorkspaceSummary | undefined, members: WorkspaceMemberSummary[], actorId: string): WorkspaceRole {
  if (!workspace || !actorId) return 'GUEST';
  if (workspace.ownerId && workspace.ownerId === actorId) return 'OWNER';
  const member = members.find((item) => (
    item.status === 'ACTIVE' &&
    ((item.userId && item.userId === actorId) || (item.email && item.email.toLowerCase() === actorId.toLowerCase()))
  ));
  if (member) return member.role;
  if (workspace.visibility === 'PUBLIC') return 'VIEWER';
  if (workspace.visibility === 'TEAM' && workspace.defaultRole === 'VIEWER') return 'VIEWER';
  return 'GUEST';
}

function normalizeActivityCode(value: string): string {
  const code = value
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[^A-Za-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toUpperCase();
  return code || `ACTIVITY_${Date.now().toString(36).toUpperCase()}`;
}

function buildStudioActivityPayload(input: CreateStudioActivityInput) {
  const now = new Date();
  const start = new Date(now.getTime() - 60_000).toISOString();
  const end = new Date(now.getTime() + 365 * 24 * 60 * 60 * 1000).toISOString();
  const activityName = input.activityName.trim();
  const activityCode = normalizeActivityCode(input.activityCode || activityName);
  return {
    operator: 'studio',
    tenantId: input.tenantId.trim() || 'site_001',
    workspaceId: input.workspaceId,
    folderId: input.folderId,
    activityCode,
    activityName: activityName || activityCode,
    activityType: 'STUDIO_WORKFLOW',
    status: 'DRAFT',
    triggerEvents: ['STUDIO_EVENT'],
    startTime: start,
    endTime: end,
    timezone: 'UTC',
    priority: 100,
    exclusiveGroup: '',
    stackable: true,
    version: 1,
    config: {
      entry: {
        channels: ['APP', 'WEB'],
        entranceCodes: ['studio'],
        landingUrl: 'https://example.com/studio',
      },
      display: {
        title: activityName || activityCode,
        visibleFrom: start,
        visibleTo: end,
        sort: 100,
      },
      audience: {
        segmentCodes: ['all_users'],
      },
      metadata: {
        owner: 'studio',
        workspaceId: input.workspaceId,
        folderId: input.folderId,
      },
    },
    rules: [
      {
        ruleCode: 'STUDIO_DEFAULT_RULE',
        rulePhase: 'QUALIFY',
        ruleType: 'CONDITION',
        ruleConfig: {
          logic: 'AND',
          conditions: [
            { field: 'attributes.eventMatched', op: '=', value: true },
          ],
        },
        sort: 1,
        enabled: true,
        version: 1,
      },
    ],
    rewards: [
      {
        rewardCode: 'STUDIO_DEFAULT_REWARD',
        rewardType: 'POINT',
        calculateType: 'FIXED',
        rewardConfig: {
          points: 1,
          unit: 'POINT',
          pointType: 'DEFAULT',
          targetRef: 'https://wallet.example.com/reward',
        },
        grantMode: 'AUTO',
        grantChannel: 'HTTP',
        enabled: true,
        version: 1,
      },
    ],
  };
}

function emptyWorkflowDefinition(context: { activityId: string; workspaceId: string; folderId: string }): WorkflowDefinition {
  return {
    meta: {
      activityId: context.activityId,
      workspaceId: context.workspaceId,
      folderId: context.folderId,
      exportedAt: new Date().toISOString(),
    },
    nodes: [],
    edges: [],
    viewport: { x: 24, y: 36, zoom: 1 },
  };
}

function serializeWorkflowDefinition(
  nodes: FlowNode[],
  edges: ActivityEdge[],
  nodeConfigSchemas: WorkflowNodeConfigSchema[],
  viewport?: { x: number; y: number; zoom: number },
  context?: { workspaceId?: string; folderId?: string; activityId?: string },
): WorkflowDefinition {
  const schemaByNodeType = new Map(nodeConfigSchemas.map((schema) => [schema.nodeType, schema]));
  return {
    nodes: nodes
      .filter((node): node is ActivityNode => node.type === 'activity')
      .map((node) => {
        const schema = schemaByNodeType.get(node.data.id);
        return {
          id: node.id,
          type: node.data.id,
          kind: node.data.kind,
          code: node.data.code,
          title: node.data.title,
          position: { x: node.position.x, y: node.position.y },
          inputs: node.data.inputs,
          outputs: node.data.outputs,
          config: materializeNodeConfig(node.data.config, schema),
        };
      }),
    edges: edges.map((item) => ({
      id: item.id,
      source: item.source,
      target: item.target,
      sourceHandle: item.sourceHandle ?? undefined,
      targetHandle: item.targetHandle ?? undefined,
      label: typeof item.label === 'string' ? item.label : undefined,
      route: item.data?.route,
    })),
    viewport,
    meta: {
      app: 'ExMarketing',
      schema: 'activity-workflow/v1',
      exportedAt: new Date().toISOString(),
      ...(context ?? {}),
    },
  };
}

function materializeNodeConfig(config: Record<string, unknown> | undefined, schema: WorkflowNodeConfigSchema | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (!schema) {
    return { ...(config ?? {}) };
  }
  schema.fields.forEach((field) => {
    if (config && Object.prototype.hasOwnProperty.call(config, field.key)) {
      out[field.key] = config[field.key];
      return;
    }
    if (Object.prototype.hasOwnProperty.call(field, 'default')) {
      out[field.key] = field.default;
      return;
    }
    switch (field.type) {
      case 'BOOLEAN':
        out[field.key] = false;
        break;
      case 'NUMBER':
        out[field.key] = 0;
        break;
      case 'ARRAY':
        out[field.key] = [];
        break;
      case 'OBJECT':
        out[field.key] = {};
        break;
      default:
        out[field.key] = '';
        break;
    }
  });
  Object.entries(config ?? {}).forEach(([key, value]) => {
    if (!Object.prototype.hasOwnProperty.call(out, key)) {
      out[key] = value;
    }
  });
  return out;
}

function hydrateWorkflowDefinition(definition: WorkflowDefinition): { nodes: FlowNode[]; edges: ActivityEdge[] } {
  return {
    nodes: definition.nodes.map((node) => {
      const base = getDef(node.type);
      return flowNode(node.type, node.position?.x ?? 80, node.position?.y ?? 120, {
        id: node.id,
        data: {
          ...base,
          id: node.type,
          code: node.code ?? base.code,
          title: node.title ?? base.title,
          kind: (node.kind as ActivityNode['data']['kind']) ?? base.kind,
          inputs: node.inputs ?? base.inputs,
          outputs: node.outputs ?? base.outputs,
          config: isRecord(node.config) ? node.config : undefined,
        },
      });
    }),
    edges: definition.edges.map((item) => ({
      ...edge(item.source, item.target, item.label ?? '', item.sourceHandle, item.targetHandle, item.route),
      id: item.id,
    })),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

class APIError extends Error {
  status: number;
  statusText: string;

  constructor(message: string, status: number, statusText: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.statusText = statusText;
  }
}

function isAPIError(error: unknown): error is APIError {
  return error instanceof APIError;
}

function runHistoryErrorMessage(error: unknown, fallback: string): string {
  if (isAPIError(error) && error.status === 403) {
    return '无权访问该 workspace / execution，可能不在当前工作区、文件夹不可见，或分享链接权限不足。';
  }
  return `${fallback}：${(error as Error).message}`;
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'content-type': 'application/json',
      'X-Actor-Id': studioActorId(),
      ...(init?.headers ?? {}),
    },
  });
  const contentType = response.headers.get('content-type') ?? '';
  if (!contentType.includes('application/json')) {
    throw new Error('接口不可用');
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new APIError(body.error ?? body.message ?? `${response.status} ${response.statusText}`, response.status, response.statusText);
  }
  return response.json() as Promise<T>;
}

function StudioActivityStart({
  activities,
  workspaces,
  folders,
  membersByWorkspace,
  shareLinksByWorkspace,
  auditLogsByWorkspace,
  currentActorId,
  placements,
  hiddenTestActivityCount,
  workflowStatus,
  initialWorkspaceId,
  initialFolderId,
  onSelectActivity,
  onCreateWorkspace,
  onCreateActivity,
  onCreateFolder,
  onUpdateFolder,
  onCreateMember,
  onUpdateMemberRole,
  onRemoveMember,
  onCreateShareLink,
  onDisableShareLink,
  onRefreshWorkspaceOps,
}: {
  activities: ActivitySummary[];
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  membersByWorkspace: Record<string, WorkspaceMemberSummary[]>;
  shareLinksByWorkspace: Record<string, WorkspaceShareLinkSummary[]>;
  auditLogsByWorkspace: Record<string, WorkspaceAuditLogSummary[]>;
  currentActorId: string;
  placements: Record<string, ActivityPlacement>;
  hiddenTestActivityCount: number;
  workflowStatus: string;
  initialWorkspaceId: string;
  initialFolderId: string;
  onSelectActivity: (activityId: string) => void;
  onCreateWorkspace: (input: CreateStudioWorkspaceInput) => Promise<WorkspaceSummary>;
  onCreateActivity: (input: CreateStudioActivityInput) => Promise<void>;
  onCreateFolder: (workspaceId: string, name: string) => Promise<WorkspaceFolderSummary>;
  onUpdateFolder: (workspaceId: string, folderId: string, patch: StudioFolderPatch) => Promise<WorkspaceFolderSummary>;
  onCreateMember: (workspaceId: string, input: { userId: string; email: string; displayName: string; role: WorkspaceRole }) => Promise<void>;
  onUpdateMemberRole: (workspaceId: string, memberId: string, role: WorkspaceRole) => Promise<void>;
  onRemoveMember: (workspaceId: string, memberId: string) => Promise<void>;
  onCreateShareLink: (workspaceId: string, input: { scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY'; folderId?: string; activityId?: string; permission: 'VIEW' | 'EDIT' }) => Promise<void>;
  onDisableShareLink: (workspaceId: string, linkId: string) => Promise<void>;
  onRefreshWorkspaceOps: (workspaceId: string) => Promise<void>;
}) {
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'DRAFT' | 'ARCHIVED' | 'ALL'>('ACTIVE');
  const [sortMode, setSortMode] = useState<'recent' | 'name' | 'code' | 'version'>('recent');
  const [showFavoritesOnly, setShowFavoritesOnly] = useState(false);
  const [favoriteActivityIds, setFavoriteActivityIds] = useState<Set<string>>(() => loadStudioStringSet('exmarketing.studio.favoriteActivities'));
  const [archivedActivityIds, setArchivedActivityIds] = useState<Set<string>>(() => loadStudioStringSet('exmarketing.studio.archivedActivities'));
  const [draggingFolderId, setDraggingFolderId] = useState('');
  const [movingFolderId, setMovingFolderId] = useState('');
  const [workspaceName, setWorkspaceName] = useState('');
  const [workspaceTenant, setWorkspaceTenant] = useState('site_001');
  const [workspaceError, setWorkspaceError] = useState('');
  const [creatingWorkspace, setCreatingWorkspace] = useState(false);
  const [showWorkspaceDialog, setShowWorkspaceDialog] = useState(false);
  const [folderName, setFolderName] = useState('');
  const [folderError, setFolderError] = useState('');
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [showFolderDialog, setShowFolderDialog] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createCode, setCreateCode] = useState('');
  const [createTenant, setCreateTenant] = useState('site_001');
  const [createWorkspaceId, setCreateWorkspaceId] = useState('');
  const [createFolderId, setCreateFolderId] = useState('');
  const [createFolderName, setCreateFolderName] = useState('');
  const [createError, setCreateError] = useState('');
  const [creating, setCreating] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState('');
  const [selectedFolderId, setSelectedFolderId] = useState('');
  const [memberUserId, setMemberUserId] = useState('');
  const [memberEmail, setMemberEmail] = useState('');
  const [memberRole, setMemberRole] = useState<WorkspaceRole>('EDITOR');
  const [opsMessage, setOpsMessage] = useState('');
  const foldersByWorkspace = useMemo(() => {
    const next: Record<string, WorkspaceFolderSummary[]> = {};
    folders
      .filter((folder) => folder.status !== 'DELETED')
      .forEach((folder) => {
        next[folder.workspaceId] = [...(next[folder.workspaceId] ?? []), folder];
      });
    Object.values(next).forEach((items) => items.sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name)));
    return next;
  }, [folders]);
  const activeWorkspaces = useMemo(() => workspaces.filter((workspace) => workspace.status !== 'DELETED'), [workspaces]);
  useEffect(() => {
    if (initialWorkspaceId) setSelectedWorkspaceId(initialWorkspaceId);
    if (initialFolderId) setSelectedFolderId(initialFolderId);
  }, [initialFolderId, initialWorkspaceId]);
  const workflowCountByWorkspace = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      const placement = placements[activity.id];
      if (!placement?.workspaceId) return;
      next[placement.workspaceId] = (next[placement.workspaceId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements]);
  const workflowCountByFolder = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      const placement = placements[activity.id];
      if (!placement?.folderId) return;
      next[placement.folderId] = (next[placement.folderId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements]);
  const selectedWorkspace = activeWorkspaces.find((workspace) => workspace.id === selectedWorkspaceId) ?? activeWorkspaces[0];
  const selectedWorkspaceFolders = selectedWorkspace ? foldersByWorkspace[selectedWorkspace.id] ?? [] : [];
  const selectedFolder = selectedWorkspaceFolders.find((folder) => folder.id === selectedFolderId);
  const selectedMembers = selectedWorkspace ? membersByWorkspace[selectedWorkspace.id] ?? [] : [];
  const selectedShareLinks = selectedWorkspace ? shareLinksByWorkspace[selectedWorkspace.id] ?? [] : [];
  const selectedAuditLogs = selectedWorkspace ? auditLogsByWorkspace[selectedWorkspace.id] ?? [] : [];
  const selectedRole = workspaceRoleForActor(selectedWorkspace, selectedMembers, currentActorId);
  const canEditWorkspace = workspaceRoleAllows(selectedRole, 'EDITOR');
  const canManageFolders = workspaceRoleAllows(selectedRole, 'ADMIN');
  const canManageMembers = workspaceRoleAllows(selectedRole, 'OWNER');
  useEffect(() => {
    if (!selectedWorkspace && selectedWorkspaceId) {
      setSelectedWorkspaceId('');
      setSelectedFolderId('');
      return;
    }
    if (selectedWorkspace && selectedWorkspace.id !== selectedWorkspaceId) {
      setSelectedWorkspaceId(selectedWorkspace.id);
    }
    if (!selectedFolder && selectedFolderId) {
      setSelectedFolderId('');
    }
  }, [selectedFolder, selectedFolderId, selectedWorkspace, selectedWorkspaceId]);
  const selectedChildFolders = useMemo(() => {
    if (!selectedWorkspace) return [];
    const parentId = selectedFolder?.id ?? '';
    return selectedWorkspaceFolders.filter((folder) => (folder.parentId ?? '') === parentId);
  }, [selectedFolder?.id, selectedWorkspace, selectedWorkspaceFolders]);
  const selectedWorkflows = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return activities
      .filter((activity) => {
        const archived = archivedActivityIds.has(activity.id);
        if (statusFilter === 'ARCHIVED') {
          if (!archived) return false;
        } else if (statusFilter !== 'ALL' && archived) {
          return false;
        }
        if (statusFilter === 'ACTIVE' && activity.status && activity.status !== 'ACTIVE') return false;
        if (statusFilter === 'DRAFT' && activity.status !== 'DRAFT') return false;
        if (showFavoritesOnly && !favoriteActivityIds.has(activity.id)) return false;
        if (selectedWorkspace) {
          const placement = placements[activity.id];
          if (!placement || placement.workspaceId !== selectedWorkspace.id) return false;
          if (selectedFolder && placement.folderId !== selectedFolder.id) return false;
        }
        if (!keyword) return true;
        const folder = placements[activity.id]?.folderId ? folders.find((item) => item.id === placements[activity.id]?.folderId) : undefined;
        return `${activity.activityName} ${activity.activityCode} ${activity.tenantId} ${selectedWorkspace?.name ?? ''} ${folder?.name ?? ''}`.toLowerCase().includes(keyword);
      })
      .sort((a, b) => {
        if (sortMode === 'name') return (a.activityName || a.activityCode).localeCompare(b.activityName || b.activityCode);
        if (sortMode === 'code') return a.activityCode.localeCompare(b.activityCode);
        if (sortMode === 'version') return (b.version ?? 0) - (a.version ?? 0);
        return activityTimestamp(b) - activityTimestamp(a);
      });
  }, [activities, archivedActivityIds, favoriteActivityIds, folders, placements, search, selectedFolder, selectedWorkspace, showFavoritesOnly, sortMode, statusFilter]);
  const recentWorkflows = useMemo(() => (
    activities
      .filter((activity) => {
        const placement = placements[activity.id];
        if (!selectedWorkspace || placement?.workspaceId !== selectedWorkspace.id) return false;
        return !archivedActivityIds.has(activity.id);
      })
      .sort((a, b) => activityTimestamp(b) - activityTimestamp(a))
      .slice(0, 5)
  ), [activities, archivedActivityIds, placements, selectedWorkspace]);
  const suggestedCode = normalizeActivityCode(createCode || createName || search);
  const openCreateDialog = (name?: string, context?: { workspaceId?: string; folderId?: string }) => {
    if (name) setCreateName(name);
    const fallbackWorkspace = context?.workspaceId || selectedWorkspace?.id || createWorkspaceId || workspaces[0]?.id || '';
    const fallbackFolder = context?.folderId || selectedFolder?.id || createFolderId || foldersByWorkspace[fallbackWorkspace]?.[0]?.id || '';
    setCreateWorkspaceId(fallbackWorkspace);
    setCreateFolderId(fallbackFolder);
    const fallbackWorkspaceItem = workspaces.find((workspace) => workspace.id === fallbackWorkspace);
    if (fallbackWorkspaceItem?.tenantId) setCreateTenant(fallbackWorkspaceItem.tenantId);
    setCreateError('');
    setShowCreateDialog(true);
  };
  const workspaceFolders = foldersByWorkspace[createWorkspaceId] ?? [];
  const submitWorkspace = async () => {
    const nextName = workspaceName.trim();
    if (!nextName) {
      setWorkspaceError('先输入 workspace 名称。');
      return;
    }
    setCreatingWorkspace(true);
    setWorkspaceError('');
    try {
      const workspace = await onCreateWorkspace({ name: nextName, tenantId: workspaceTenant.trim() || 'site_001' });
      setSelectedWorkspaceId(workspace.id);
      setSelectedFolderId('');
      setCreateWorkspaceId(workspace.id);
      setCreateTenant(workspace.tenantId);
      setWorkspaceName('');
      setShowWorkspaceDialog(false);
    } catch (error) {
      setWorkspaceError((error as Error).message);
    } finally {
      setCreatingWorkspace(false);
    }
  };
  const submitFolder = async () => {
    if (!selectedWorkspace) {
      setFolderError('先选择 workspace。');
      return;
    }
    const nextName = folderName.trim();
    if (!nextName) {
      setFolderError('先输入 folder 名称。');
      return;
    }
    setCreatingFolder(true);
    setFolderError('');
    try {
      const folder = await onCreateFolder(selectedWorkspace.id, nextName);
      setSelectedFolderId(folder.id);
      setFolderName('');
      setShowFolderDialog(false);
    } catch (error) {
      setFolderError((error as Error).message);
    } finally {
      setCreatingFolder(false);
    }
  };
  const submitCreate = async () => {
    const nextName = createName.trim() || search.trim();
    if (!nextName) {
      setCreateError('先输入活动名称。');
      return;
    }
    if (!createWorkspaceId) {
      setCreateError('先选择工作区。');
      return;
    }
    const selectedWorkspace = workspaces.find((workspace) => workspace.id === createWorkspaceId);
    let nextFolderId = createFolderId;
    if (!nextFolderId && createFolderName.trim()) {
      try {
        const folder = await onCreateFolder(createWorkspaceId, createFolderName.trim());
        nextFolderId = folder.id;
        setCreateFolderId(folder.id);
      } catch (error) {
        setCreateError((error as Error).message);
        return;
      }
    }
    if (!nextFolderId) {
      setCreateError('先选择文件夹，或输入新文件夹名称。');
      return;
    }
    setCreating(true);
    setCreateError('');
    try {
      await onCreateActivity({
        tenantId: selectedWorkspace?.tenantId || createTenant,
        activityName: nextName,
        activityCode: createCode || suggestedCode,
        workspaceId: createWorkspaceId,
        folderId: nextFolderId,
      });
      setShowCreateDialog(false);
      setCreateFolderName('');
    } catch (error) {
      setCreateError((error as Error).message);
    } finally {
      setCreating(false);
    }
  };
  const submitMember = async () => {
    if (!selectedWorkspace) return;
    if (!memberUserId.trim() && !memberEmail.trim()) {
      setOpsMessage('成员需要 userId 或 email。');
      return;
    }
    setOpsMessage('');
    try {
      await onCreateMember(selectedWorkspace.id, {
        userId: memberUserId.trim(),
        email: memberEmail.trim(),
        displayName: memberUserId.trim() || memberEmail.trim(),
        role: memberRole,
      });
      setMemberUserId('');
      setMemberEmail('');
      setOpsMessage('成员已更新');
    } catch (error) {
      setOpsMessage((error as Error).message);
    }
  };
  const updateSelectedFolderVisibility = async (visibility: string) => {
    if (!selectedWorkspace || !selectedFolder) return;
    setOpsMessage('');
    try {
      await onUpdateFolder(selectedWorkspace.id, selectedFolder.id, { visibility });
      setOpsMessage('Folder 权限已更新');
    } catch (error) {
      setOpsMessage((error as Error).message);
    }
  };
  const createFolderShare = async () => {
    if (!selectedWorkspace) return;
    setOpsMessage('');
    try {
      await onCreateShareLink(selectedWorkspace.id, {
        scope: selectedFolder ? 'FOLDER' : 'WORKSPACE',
        folderId: selectedFolder?.id,
        permission: 'VIEW',
      });
      setOpsMessage('分享链接已创建');
    } catch (error) {
      setOpsMessage((error as Error).message);
    }
  };
  const toggleFavorite = (activityId: string) => {
    setFavoriteActivityIds((items) => {
      const next = new Set(items);
      if (next.has(activityId)) next.delete(activityId);
      else next.add(activityId);
      saveStudioStringSet('exmarketing.studio.favoriteActivities', next);
      return next;
    });
  };
  const toggleArchived = (activityId: string) => {
    setArchivedActivityIds((items) => {
      const next = new Set(items);
      if (next.has(activityId)) next.delete(activityId);
      else next.add(activityId);
      saveStudioStringSet('exmarketing.studio.archivedActivities', next);
      return next;
    });
  };
  const isFolderDescendant = (folderId: string, ancestorId: string) => {
    let cursor = selectedWorkspaceFolders.find((folder) => folder.id === folderId);
    while (cursor?.parentId) {
      if (cursor.parentId === ancestorId) return true;
      cursor = selectedWorkspaceFolders.find((folder) => folder.id === cursor?.parentId);
    }
    return false;
  };
  const moveFolder = async (folder: WorkspaceFolderSummary, parentId: string) => {
    if (!selectedWorkspace || folder.workspaceId !== selectedWorkspace.id || folder.id === parentId || movingFolderId) return;
    if (parentId && isFolderDescendant(parentId, folder.id)) {
      setDraggingFolderId('');
      setOpsMessage('不能把 folder 移动到自己的子级下面。');
      return;
    }
    const nextSiblings = selectedWorkspaceFolders.filter((item) => (item.parentId ?? '') === parentId && item.id !== folder.id);
    setMovingFolderId(folder.id);
    setOpsMessage('');
    try {
      await onUpdateFolder(folder.workspaceId, folder.id, { parentId, sortOrder: (nextSiblings.length + 1) * 10 });
      setSelectedFolderId(parentId);
      setOpsMessage(parentId ? `已移动 ${folder.name}` : `已移动 ${folder.name} 到 workspace 根目录`);
    } catch (error) {
      setOpsMessage((error as Error).message);
    } finally {
      setMovingFolderId('');
      setDraggingFolderId('');
    }
  };

  return (
    <div className="studio-start-overlay">
      <section className="studio-start-shell">
        <header className="studio-home-top">
          <div>
            <b>ExMarketing</b>
            <span>Activity Studio</span>
          </div>
          <a href="/admin/">活动后台</a>
        </header>
        <div className="studio-start-body studio-file-manager">
          <aside className="studio-workspace-panel">
            <div className="studio-section-head">
              <div>
                <b>Workspace</b>
                <p>{activeWorkspaces.length} 个工作区</p>
              </div>
              <button aria-label="新建 Workspace" onClick={() => {
                setWorkspaceError('');
                setShowWorkspaceDialog(true);
              }}>+</button>
            </div>
            <div className="studio-workspace-list">
              {activeWorkspaces.length ? activeWorkspaces.map((workspace) => (
                <button
                  key={workspace.id}
                  className={cx(workspace.id === selectedWorkspace?.id && 'active')}
                  onClick={() => {
                    setSelectedWorkspaceId(workspace.id);
                    setSelectedFolderId('');
                  }}
                >
                  <span style={{ background: workspace.color || '#155eef' }}>{(workspace.name || workspace.slug || 'W').slice(0, 1).toUpperCase()}</span>
                  <div>
                    <b>{workspace.name}</b>
                    <em>{workspace.slug} · {workspace.visibility}</em>
                  </div>
                  <strong>{workflowCountByWorkspace[workspace.id] ?? 0}</strong>
                </button>
              )) : <div className="studio-start-empty">暂无 workspace</div>}
            </div>
            <div className="studio-home-side-note">
              <b>{selectedRole}</b>
              <span>{currentActorId}</span>
            </div>
            {hiddenTestActivityCount > 0 ? (
              <p className="studio-start-note">已隐藏 {hiddenTestActivityCount} 个测试活动</p>
            ) : null}
          </aside>

          <section className="studio-browser-panel">
            <div className="studio-browser-toolbar">
              <div>
                <span>Activity Studio</span>
                <b>{selectedWorkspace?.name ?? '选择 Workspace'}</b>
                <p>{selectedFolder ? `/${selectedFolder.name}` : '/ Workspace Root'} · {workflowStatus}</p>
              </div>
              <div className="studio-browser-actions">
                <button onClick={() => {
                  setFolderError('');
                  setShowFolderDialog(true);
                }} disabled={!selectedWorkspace || !canManageFolders}>新建 Folder</button>
                <button className="primary" onClick={() => openCreateDialog('新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id })} disabled={!selectedWorkspace || !canEditWorkspace}>新建 Workflow</button>
              </div>
            </div>

            <div className="studio-browser-filters">
              <input value={search} placeholder="搜索 activity 名称、code、tenant、folder" onChange={(event) => setSearch(event.target.value)} />
              <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as typeof statusFilter)}>
                <option value="ACTIVE">Active</option>
                <option value="DRAFT">Draft</option>
                <option value="ARCHIVED">Archived</option>
                <option value="ALL">All</option>
              </select>
              <select value={sortMode} onChange={(event) => setSortMode(event.target.value as typeof sortMode)}>
                <option value="recent">最近编辑</option>
                <option value="name">名称</option>
                <option value="code">Code</option>
                <option value="version">版本</option>
              </select>
              <button className={cx(showFavoritesOnly && 'active')} onClick={() => setShowFavoritesOnly((value) => !value)}>收藏</button>
            </div>

            <nav className="studio-browser-breadcrumb">
              <button onClick={() => setSelectedFolderId('')}>{selectedWorkspace?.name ?? 'Workspace'}</button>
              {selectedFolder ? (
                <>
                  <span>/</span>
                  <button>{selectedFolder.name}</button>
                </>
              ) : null}
            </nav>

            <div
              className={cx('studio-folder-drop-root', draggingFolderId && 'dragging')}
              onDragOver={(event) => {
                if (!draggingFolderId) return;
                event.preventDefault();
              }}
              onDrop={(event) => {
                event.preventDefault();
                const folder = selectedWorkspaceFolders.find((item) => item.id === draggingFolderId);
                if (folder) void moveFolder(folder, '');
              }}
            >
              <span>拖到这里移回 Workspace 根目录</span>
            </div>

            <div className="studio-file-section">
              <div className="studio-file-section-title">
                <b>Folders</b>
                <span>{selectedChildFolders.length}</span>
              </div>
              {selectedChildFolders.length ? (
                <div className="studio-folder-grid">
                  {selectedChildFolders.map((folder) => (
                    <button
                      key={folder.id}
                      draggable
                      className={cx(folder.id === selectedFolder?.id && 'active', draggingFolderId === folder.id && 'dragging', movingFolderId === folder.id && 'moving')}
                      onClick={() => setSelectedFolderId(folder.id)}
                      onDragStart={(event) => {
                        event.dataTransfer.effectAllowed = 'move';
                        setDraggingFolderId(folder.id);
                      }}
                      onDragEnd={() => setDraggingFolderId('')}
                      onDragOver={(event) => {
                        if (!draggingFolderId || draggingFolderId === folder.id) return;
                        event.preventDefault();
                      }}
                      onDrop={(event) => {
                        event.preventDefault();
                        const dragged = selectedWorkspaceFolders.find((item) => item.id === draggingFolderId);
                        if (dragged) void moveFolder(dragged, folder.id);
                      }}
                    >
                      <i style={{ background: folder.color || '#e8f1ff' }}>▰</i>
                      <span>
                        <b>{folder.name}</b>
                        <em>{folder.visibility} · {workflowCountByFolder[folder.id] ?? 0} workflows</em>
                      </span>
                    </button>
                  ))}
                </div>
              ) : <div className="studio-folder-empty-line">当前层级没有子文件夹</div>}
            </div>

            <div className="studio-file-section">
              <div className="studio-file-section-title">
                <b>Workflows</b>
                <span>{selectedWorkflows.length}</span>
              </div>
              {selectedWorkflows.length ? (
                <div className="studio-workflow-table">
                  <div className="studio-workflow-row head">
                    <span>Name</span>
                    <span>Code</span>
                    <span>Status</span>
                    <span>Version</span>
                    <span>Updated</span>
                    <span>Actions</span>
                  </div>
                  {selectedWorkflows.map((activity) => (
                    <button key={activity.id} className="studio-workflow-row" onClick={() => onSelectActivity(activity.id)}>
                      <span className="workflow-name">
                        <i>{(activity.activityName || activity.activityCode || 'W').slice(0, 1).toUpperCase()}</i>
                        <b>{activity.activityName || activity.activityCode}</b>
                      </span>
                      <span>{activity.activityCode}</span>
                      <span><strong>{archivedActivityIds.has(activity.id) ? 'ARCHIVED' : activity.status || 'ACTIVE'}</strong></span>
                      <span>v{activity.version ?? 1}</span>
                      <span>{formatStudioDate(activity.updatedAt || activity.createdAt)}</span>
                      <span className="workflow-actions">
                        <em
                          role="button"
                          tabIndex={0}
                          className={cx(favoriteActivityIds.has(activity.id) && 'active')}
                          onClick={(event) => {
                            event.stopPropagation();
                            toggleFavorite(activity.id);
                          }}
                        >
                          ★
                        </em>
                        <em
                          role="button"
                          tabIndex={0}
                          onClick={(event) => {
                            event.stopPropagation();
                            toggleArchived(activity.id);
                          }}
                        >
                          {archivedActivityIds.has(activity.id) ? '恢复' : '归档'}
                        </em>
                      </span>
                    </button>
                  ))}
                </div>
              ) : (
                <div className="studio-empty-state compact">
                  <div className="studio-empty-icon">＋</div>
                  <b>{statusFilter === 'ARCHIVED' ? '没有归档 workflow' : '当前视图没有 workflow'}</b>
                  <span>可以新建空白活动、切换 folder，或调整搜索和筛选条件。</span>
                  <div className="studio-empty-actions">
                    <button onClick={() => openCreateDialog(search || '新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id })} disabled={!canEditWorkspace}>创建 Workflow</button>
                  </div>
                </div>
              )}
            </div>
          </section>

          <aside className="studio-ops-panel">
            <section className="studio-recent-card">
              <h3>最近编辑</h3>
              <div className="studio-recent-list">
                {recentWorkflows.length ? recentWorkflows.map((activity) => (
                  <button key={activity.id} onClick={() => onSelectActivity(activity.id)}>
                    <span>
                      <b>{activity.activityName || activity.activityCode}</b>
                      <em>{formatStudioDate(activity.updatedAt || activity.createdAt)}</em>
                    </span>
                    <strong>v{activity.version ?? 1}</strong>
                  </button>
                )) : <div className="studio-folder-empty-line">暂无最近编辑</div>}
              </div>
            </section>
            <div className="studio-section-head">
              <div>
                <b>Workspace Ops</b>
                <p>{selectedRole} · {currentActorId}</p>
              </div>
              <button disabled={!selectedWorkspace} onClick={() => selectedWorkspace && void onRefreshWorkspaceOps(selectedWorkspace.id)}>↻</button>
            </div>
            <section>
              <h3>Folder 权限</h3>
              <select value={selectedFolder?.visibility ?? 'TEAM'} disabled={!selectedFolder || !canManageFolders} onChange={(event) => void updateSelectedFolderVisibility(event.target.value)}>
                <option value="INHERIT">Inherit</option>
                <option value="PRIVATE">Private</option>
                <option value="TEAM">Team</option>
                <option value="PUBLIC">Public</option>
              </select>
              <span>{canManageFolders ? 'Admin 可修改' : '当前角色只读'}</span>
            </section>
            <section>
              <h3>成员</h3>
              <div className="studio-ops-list">
                {selectedMembers.filter((member) => member.status !== 'REMOVED').slice(0, 6).map((member) => (
                  <div key={member.id} className="studio-ops-row">
                    <span>
                      <b>{member.displayName || member.userId || member.email}</b>
                      <em>{member.userId || member.email}</em>
                    </span>
                    <select value={member.role} disabled={!canManageMembers} onChange={(event) => void onUpdateMemberRole(member.workspaceId, member.id, event.target.value as WorkspaceRole)}>
                      {(['VIEWER', 'EDITOR', 'ADMIN', 'OWNER'] as WorkspaceRole[]).map((role) => <option key={role} value={role}>{role}</option>)}
                    </select>
                    {canManageMembers ? <button onClick={() => void onRemoveMember(member.workspaceId, member.id)}>×</button> : null}
                  </div>
                ))}
              </div>
              {canManageMembers ? (
                <div className="studio-ops-inline-form">
                  <input value={memberUserId} placeholder="userId" onChange={(event) => setMemberUserId(event.target.value)} />
                  <input value={memberEmail} placeholder="email" onChange={(event) => setMemberEmail(event.target.value)} />
                  <select value={memberRole} onChange={(event) => setMemberRole(event.target.value as WorkspaceRole)}>
                    <option value="VIEWER">VIEWER</option>
                    <option value="EDITOR">EDITOR</option>
                    <option value="ADMIN">ADMIN</option>
                    <option value="OWNER">OWNER</option>
                  </select>
                  <button onClick={submitMember}>添加</button>
                </div>
              ) : null}
            </section>
            <section>
              <h3>分享</h3>
              <div className="studio-ops-list">
                {selectedShareLinks.filter((link) => link.enabled).slice(0, 5).map((link) => (
                  <div key={link.id} className="studio-ops-row">
                    <span>
                      <b>{link.scope} · {link.permission ?? 'VIEW'}</b>
                      <em>{link.token || link.id}</em>
                    </span>
                    {canManageMembers ? <button onClick={() => void onDisableShareLink(link.workspaceId, link.id)}>停用</button> : null}
                  </div>
                ))}
              </div>
              {canManageMembers ? <button className="studio-ops-primary" onClick={createFolderShare}>创建当前分享</button> : null}
            </section>
            <section>
              <h3>Audit Log</h3>
              <div className="studio-ops-list audit">
                {selectedAuditLogs.slice(0, 8).map((log) => (
                  <div key={log.id} className="studio-ops-row">
                    <span>
                      <b>{log.action}</b>
                      <em>{log.actorId || 'system'} · {log.targetType}</em>
                    </span>
                  </div>
                ))}
              </div>
            </section>
            {opsMessage ? <p>{opsMessage}</p> : null}
          </aside>
        </div>
      </section>
      {showWorkspaceDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建 Workspace</b>
                <span>用于组织 folder 和 workflow。</span>
              </div>
              <button onClick={() => setShowWorkspaceDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={workspaceName} placeholder="Marketing Studio" onChange={(event) => setWorkspaceName(event.target.value)} />
              </label>
              <label>
                <span>Tenant</span>
                <input value={workspaceTenant} placeholder="site_001" onChange={(event) => setWorkspaceTenant(event.target.value)} />
              </label>
              <button onClick={submitWorkspace} disabled={creatingWorkspace}>{creatingWorkspace ? '创建中...' : '创建 Workspace'}</button>
            </div>
            {workspaceError ? <p>{workspaceError}</p> : null}
          </section>
        </div>
      ) : null}
      {showFolderDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建 Folder</b>
                <span>{selectedWorkspace ? `创建到 ${selectedWorkspace.name}` : '先选择 workspace。'}</span>
              </div>
              <button onClick={() => setShowFolderDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={folderName} placeholder="增长活动" onChange={(event) => setFolderName(event.target.value)} />
              </label>
              <button onClick={submitFolder} disabled={creatingFolder || !selectedWorkspace || !canManageFolders}>{creatingFolder ? '创建中...' : '创建 Folder'}</button>
            </div>
            {folderError ? <p>{folderError}</p> : null}
          </section>
        </div>
      ) : null}
      {showCreateDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建活动</b>
                <span>创建后直接进入编排画布。</span>
              </div>
              <button onClick={() => setShowCreateDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={createName} placeholder="618 拉新活动" onChange={(event) => setCreateName(event.target.value)} />
              </label>
              <label>
                <span>Code</span>
                <input value={createCode} placeholder={suggestedCode} onChange={(event) => setCreateCode(event.target.value)} />
              </label>
              <label>
                <span>Tenant</span>
                <input value={createTenant} placeholder="site_001" onChange={(event) => setCreateTenant(event.target.value)} />
              </label>
              <label>
                <span>Workspace</span>
                <select value={createWorkspaceId} onChange={(event) => {
                  const nextWorkspace = event.target.value;
                  setCreateWorkspaceId(nextWorkspace);
                  setCreateFolderId(foldersByWorkspace[nextWorkspace]?.[0]?.id ?? '');
                  const workspace = workspaces.find((item) => item.id === nextWorkspace);
                  if (workspace?.tenantId) setCreateTenant(workspace.tenantId);
                }}>
                  <option value="">选择工作区</option>
                  {workspaces.map((workspace) => (
                    <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>Folder</span>
                <select value={createFolderId} onChange={(event) => setCreateFolderId(event.target.value)}>
                  <option value="">选择文件夹</option>
                  {workspaceFolders.map((folder) => (
                    <option key={folder.id} value={folder.id}>{folder.name}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>新文件夹</span>
                <input value={createFolderName} placeholder="没有文件夹时填写" onChange={(event) => setCreateFolderName(event.target.value)} />
              </label>
              <button onClick={submitCreate} disabled={creating || !canEditWorkspace}>{creating ? '创建中...' : '创建并编排'}</button>
            </div>
            {createError ? <p>{createError}</p> : null}
          </section>
        </div>
      ) : null}
    </div>
  );
}

export function App() {
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<ActivityEdge>([]);
  const [selectedTemplate, setSelectedTemplate] = useState<'default' | 'simple'>('default');
  const [lastAdded, setLastAdded] = useState('');
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [selectedEdgeIds, setSelectedEdgeIds] = useState<string[]>([]);
  const [contextMenu, setContextMenu] = useState<FlowContextMenu | null>(null);
  const [nodePicker, setNodePicker] = useState<NodePickerState | null>(null);
  const [nodeSearch, setNodeSearch] = useState('');
  const [reactFlowInstance, setReactFlowInstance] = useState<ReactFlowInstance<FlowNode, ActivityEdge> | null>(null);
  const [alignmentGuide, setAlignmentGuide] = useState<AlignmentGuide>({});
  const [hoveredEdgeId, setHoveredEdgeId] = useState('');
  const [hoveredNodeId, setHoveredNodeId] = useState('');
  const [showPalette, setShowPalette] = useState(false);
  const [interactionMode, setInteractionMode] = useState<'pointer' | 'hand'>('pointer');
  const [zoomPercent, setZoomPercent] = useState(100);
  const [showMiniMap, setShowMiniMap] = useState(true);
  const [showChecklist, setShowChecklist] = useState(false);
  const [validatingWorkflow, setValidatingWorkflow] = useState(false);
  const [workflowValidation, setWorkflowValidation] = useState<WorkflowValidationResult | null>(null);
  const [nodeConfigSchemas, setNodeConfigSchemas] = useState<WorkflowNodeConfigSchema[]>([]);
  const [workflowTemplates, setWorkflowTemplates] = useState<WorkflowDraftTemplateSummary[]>([]);
  const [workflowNodePresets, setWorkflowNodePresets] = useState<WorkflowNodePresetSummary[]>([]);
  const [workflowTemplateVersions, setWorkflowTemplateVersions] = useState<WorkflowDraftTemplateSummary[]>([]);
  const [loadingTemplateVersions, setLoadingTemplateVersions] = useState(false);
  const [showTemplatePanel, setShowTemplatePanel] = useState(false);
  const [templateMessage, setTemplateMessage] = useState('');
  const [savingTemplate, setSavingTemplate] = useState(false);
  const [isPublished, setIsPublished] = useState(false);
  const [activities, setActivities] = useState<ActivitySummary[]>([]);
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceFolders, setWorkspaceFolders] = useState<WorkspaceFolderSummary[]>([]);
  const [workspaceMembers, setWorkspaceMembers] = useState<Record<string, WorkspaceMemberSummary[]>>({});
  const [workspaceShareLinks, setWorkspaceShareLinks] = useState<Record<string, WorkspaceShareLinkSummary[]>>({});
  const [workspaceAuditLogs, setWorkspaceAuditLogs] = useState<Record<string, WorkspaceAuditLogSummary[]>>({});
  const [activityPlacements, setActivityPlacements] = useState<Record<string, ActivityPlacement>>({});
  const [activityId, setActivityId] = useState('');
  const [activityLabel, setActivityLabel] = useState('选择活动');
  const [showActivitySettings, setShowActivitySettings] = useState(false);
  const [savingActivitySettings, setSavingActivitySettings] = useState(false);
  const [activitySettingsError, setActivitySettingsError] = useState('');
  const [workflowStatus, setWorkflowStatus] = useState('未连接后端');
  const [workflowVersionLabel, setWorkflowVersionLabel] = useState('workflow 未发布');
  const [workflowVersions, setWorkflowVersions] = useState<WorkflowVersion[]>([]);
  const [draftDirty, setDraftDirty] = useState(false);
  const [draftSaveState, setDraftSaveState] = useState<DraftSaveState>('idle');
  const [draftSaveError, setDraftSaveError] = useState('');
  const [lastSavedAt, setLastSavedAt] = useState('');
  const [publishState, setPublishState] = useState<PublishState>('idle');
  const [publishError, setPublishError] = useState('');
  const draftRevisionRef = useRef(0);
  const savedDraftRevisionRef = useRef(0);
  const initialRunQueryRef = useRef((() => {
    const params = new URLSearchParams(window.location.search);
    const replayStep = Number(params.get('replayStep') ?? '0');
    return {
      executionId: params.get('executionId') ?? '',
      replayStep: Number.isFinite(replayStep) && replayStep > 0 ? Math.floor(replayStep) : 0,
      compareExecutionId: params.get('compareExecutionId') ?? '',
    };
  })());
  const restoredRunQueryRef = useRef(false);
  const importFileInputRef = useRef<HTMLInputElement | null>(null);
  const [runHistory, setRunHistory] = useState<RunHistoryState>({
    open: false,
    loading: false,
    versionId: '',
    executions: [],
    pageLimit: 20,
    pageOffset: 0,
    hasMore: false,
    nextOffset: 0,
    selectedExecutionId: '',
    nodeExecutions: [],
    selectedNodeExecutionId: '',
    replayStep: initialRunQueryRef.current.replayStep,
    compareExecutionId: initialRunQueryRef.current.compareExecutionId,
    compareNodeExecutions: [],
    compareResult: undefined,
    compareLoading: false,
  });
  const [debugPanel, setDebugPanel] = useState<DebugPanelState>({
    open: false,
    loading: false,
    template: 'deposit_success',
    payloadText: '',
    startNodeId: '',
    message: '',
  });
  const [debugPayloadTemplates, setDebugPayloadTemplates] = useState<DebugPayloadTemplate[]>(() => loadDebugPayloadTemplates());
  const [homeWorkspaceId, setHomeWorkspaceId] = useState('');
  const [homeFolderId, setHomeFolderId] = useState('');

  const nodeTypes = useMemo(() => ({ activity: ActivityNodeView, group: GroupNodeView, note: NoteNodeView }), []);
  const selectedCount = selectedNodeIds.length + selectedEdgeIds.length;
  const visibleStudioActivities = useMemo(() => activities.filter((activity) => !isTestActivity(activity)), [activities]);
  const hiddenTestActivityCount = activities.length - visibleStudioActivities.length;
  const activePlacement = activityId ? activityPlacements[activityId] : undefined;
  const activeWorkspace = activePlacement ? workspaces.find((workspace) => workspace.id === activePlacement.workspaceId) : undefined;
  const activeFolder = activePlacement ? workspaceFolders.find((folder) => folder.id === activePlacement.folderId) : undefined;
  const currentActorId = studioActorId();
  const activeWorkspaceRole = workspaceRoleForActor(activeWorkspace, activeWorkspace ? workspaceMembers[activeWorkspace.id] ?? [] : [], currentActorId);
  const canEditActiveWorkflow = !activeWorkspace || workspaceRoleAllows(activeWorkspaceRole, 'EDITOR');
  const canPublishActiveWorkflow = !activeWorkspace || workspaceRoleAllows(activeWorkspaceRole, 'ADMIN');
  const currentWorkflowDefinition = useMemo(() => (
    activityId && nodeConfigSchemas.length > 0
      ? serializeWorkflowDefinition(nodes, edges, nodeConfigSchemas, reactFlowInstance?.getViewport(), {
        workspaceId: activePlacement?.workspaceId,
        folderId: activePlacement?.folderId,
        activityId,
      })
      : undefined
  ), [activityId, activePlacement?.folderId, activePlacement?.workspaceId, edges, nodeConfigSchemas, nodes, reactFlowInstance]);
  const activeWorkflowVersion = useMemo(() => workflowVersions.find((item) => item.status === 'ACTIVE'), [workflowVersions]);
  const draftActiveDiff = useMemo(() => workflowDiffDetail(currentWorkflowDefinition, activeWorkflowVersion?.definition, '暂无线上 active version'), [activeWorkflowVersion?.definition, currentWorkflowDefinition]);
  const diffHighlights = useMemo(() => {
    const addedNodeIds = new Set<string>();
    const removedNodeIds = new Set<string>();
    const changedNodeIds = new Set<string>();
    const addedEdgeIds = new Set<string>();
    const removedEdgeIds = new Set<string>();
    const changedEdgeIds = new Set<string>();
    draftActiveDiff.items.forEach((item) => {
      if (item.kind === 'NODE_ADDED' && item.nodeId) addedNodeIds.add(item.nodeId);
      if (item.kind === 'NODE_REMOVED' && item.nodeId) removedNodeIds.add(item.nodeId);
      if (item.kind === 'NODE_CHANGED' && item.nodeId) changedNodeIds.add(item.nodeId);
      if (item.kind === 'EDGE_ADDED' && item.edgeId) addedEdgeIds.add(item.edgeId);
      if (item.kind === 'EDGE_REMOVED' && item.edgeId) removedEdgeIds.add(item.edgeId);
      if (item.kind === 'EDGE_CHANGED' && item.edgeId) changedEdgeIds.add(item.edgeId);
    });
    return { addedNodeIds, removedNodeIds, changedNodeIds, addedEdgeIds, removedEdgeIds, changedEdgeIds };
  }, [draftActiveDiff.items]);

  useEffect(() => {
    let cancelled = false;
    const params = new URLSearchParams(window.location.search);
    const requestedActivityId = params.get('activityId') ?? '';
    Promise.all([
      fetchJSON<{ items: ActivitySummary[] }>('/admin/v1/activities'),
      fetchJSON<{ items: WorkspaceSummary[] }>('/admin/v1/workspaces?status=ACTIVE&limit=100'),
    ])
      .then(async ([activityBody, workspaceBody]) => {
        if (cancelled) return;
        const activityItems = activityBody.items ?? [];
        const workspaceItems = workspaceBody.items ?? [];
        const activeWorkspaces = workspaceItems.filter((workspace) => workspace.status !== 'DELETED');
        const folderResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceFolderSummary[] }>(`/admin/v1/workspaces/${workspace.id}/folders?status=ACTIVE&limit=200`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        const shareResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceShareLinkSummary[] }>(`/admin/v1/workspaces/${workspace.id}/share-links?limit=500`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        const memberResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceMemberSummary[] }>(`/admin/v1/workspaces/${workspace.id}/members`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        const auditResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceAuditLogSummary[] }>(`/admin/v1/workspaces/${workspace.id}/audit-logs?limit=40`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        if (cancelled) return;
        const nextFolders = folderResults.flat();
        const nextPlacements: Record<string, ActivityPlacement> = {};
        activityItems.forEach((activity) => {
          if (!activity.workspaceId || !activity.folderId) return;
          nextPlacements[activity.id] = {
            workspaceId: activity.workspaceId,
            folderId: activity.folderId,
          };
        });
        const nextShareLinks: Record<string, WorkspaceShareLinkSummary[]> = {};
        const nextMembers: Record<string, WorkspaceMemberSummary[]> = {};
        const nextAudits: Record<string, WorkspaceAuditLogSummary[]> = {};
        activeWorkspaces.forEach((workspace, index) => {
          nextShareLinks[workspace.id] = shareResults[index] ?? [];
          nextMembers[workspace.id] = memberResults[index] ?? [];
          nextAudits[workspace.id] = auditResults[index] ?? [];
        });
        shareResults.flat().forEach((share) => {
          if (!share.activityId || !share.folderId || !share.enabled) return;
          if (nextPlacements[share.activityId]) return;
          nextPlacements[share.activityId] = {
            workspaceId: share.workspaceId,
            folderId: share.folderId,
            shareLinkId: share.id,
          };
        });
        setActivities(activityItems);
        setWorkspaces(activeWorkspaces);
        setWorkspaceFolders(nextFolders);
        setWorkspaceShareLinks(nextShareLinks);
        setWorkspaceMembers(nextMembers);
        setWorkspaceAuditLogs(nextAudits);
        setActivityPlacements(nextPlacements);
        if (!activityItems.length) {
          setWorkflowStatus('后端已连接 · 暂无 workflow');
          return;
        }
        if (!requestedActivityId) {
          setWorkflowStatus('请选择活动开始编排');
          return;
        }
        const activity = activityItems.find((item) => item.id === requestedActivityId);
        if (!activity) {
          setWorkflowStatus('URL 中的活动不存在，请重新选择');
          return;
        }
        setActivityId(activity.id);
        setActivityLabel(`${activity.activityName} / ${activity.activityCode}`);
        setWorkflowStatus('已绑定活动');
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setWorkflowStatus(`后端未连接：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      fetchJSON<{ items: WorkflowNodeConfigSchema[] }>('/admin/v1/workflow/node-config-schemas'),
      fetchJSON<{ items: WorkflowDraftTemplateSummary[] }>('/admin/v1/workflow/draft-templates'),
      fetchJSON<{ items: WorkflowNodePresetSummary[] }>('/admin/v1/workflow/node-presets'),
    ])
      .then(([schemaBody, templateBody, presetBody]) => {
        if (cancelled) return;
        setNodeConfigSchemas(schemaBody.items ?? []);
        setWorkflowTemplates(templateBody.items ?? []);
        setWorkflowNodePresets(presetBody.items ?? []);
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setWorkflowStatus(`节点 schema / 模板加载失败：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!activityId) return;
    let cancelled = false;
    const activity = activities.find((item) => item.id === activityId);
    if (activity) {
      setActivityLabel(`${activity.activityName} / ${activity.activityCode}`);
      replaceUrlParams({ activityId: activity.id });
    }
    fetchJSON<{ definition: WorkflowDefinition }>(`/admin/v1/activities/${activityId}/workflow/draft`)
      .then((draft) => {
        if (cancelled || !draft.definition) return;
        const next = hydrateWorkflowDefinition(draft.definition);
        setNodes(next.nodes);
        setEdges(next.edges);
        setWorkflowStatus('已加载草稿');
        setDraftDirty(false);
        setDraftSaveState('saved');
        setDraftSaveError('');
        setLastSavedAt('');
        setPublishError('');
        draftRevisionRef.current = 0;
        savedDraftRevisionRef.current = 0;
      })
      .catch((error: Error) => {
        if (cancelled) return;
        if (error.message.includes('not found')) {
          setNodes([]);
          setEdges([]);
          setSelectedTemplate('default');
          setWorkflowStatus('当前活动暂无草稿，空画布开始编排');
          setDraftDirty(false);
          setDraftSaveState('idle');
          setDraftSaveError('');
          setLastSavedAt('');
          setPublishError('');
          draftRevisionRef.current = 0;
          savedDraftRevisionRef.current = 0;
          return;
        }
        setWorkflowStatus(`草稿加载失败：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, [activities, activityId, setEdges, setNodes]);

  const refreshWorkflowVersions = useCallback(async (nextActivityId = activityId) => {
    if (!nextActivityId) {
      setWorkflowVersionLabel('workflow 未发布');
      setIsPublished(false);
      setWorkflowVersions([]);
      return;
    }
    try {
      const versions = await fetchJSON<{ items: WorkflowVersion[] }>(`/admin/v1/activities/${nextActivityId}/workflow/versions`);
      const versionItems = versions.items ?? [];
      setWorkflowVersions(versionItems);
      const active = versionItems.find((item) => item.status === 'ACTIVE');
      if (!active) {
        setWorkflowVersionLabel('workflow 未发布');
        setIsPublished(false);
        return;
      }
      setWorkflowVersionLabel(`workflow v${active.version} · ${new Date(active.publishedAt).toLocaleString()}`);
      setIsPublished(true);
    } catch (error) {
      setWorkflowVersionLabel(`版本加载失败：${(error as Error).message}`);
      setIsPublished(false);
      setWorkflowVersions([]);
    }
  }, [activityId]);

  useEffect(() => {
    void refreshWorkflowVersions(activityId);
  }, [activityId, refreshWorkflowVersions]);

  const filteredCatalog = useMemo(() => {
    const keyword = nodeSearch.trim().toLowerCase();
    if (!keyword) return nodeCatalog;
    return nodeCatalog
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => (
          item.title.toLowerCase().includes(keyword)
          || item.code.toLowerCase().includes(keyword)
          || item.desc.toLowerCase().includes(keyword)
        )),
      }))
      .filter((group) => group.items.length > 0);
  }, [nodeSearch]);

  const closeFloatingUi = useCallback(() => {
    setContextMenu(null);
    setNodePicker(null);
  }, []);

  const invalidateWorkflowValidation = useCallback(() => {
    draftRevisionRef.current += 1;
    setWorkflowValidation(null);
    setDraftDirty(true);
    setDraftSaveState('dirty');
    setDraftSaveError('');
  }, []);

  const handleNodesChange = useCallback((changes: NodeChange<FlowNode>[]) => {
    const hasUserChange = changes.some((change) => (
      change.type === 'remove'
      || change.type === 'add'
    ));
    if (hasUserChange) invalidateWorkflowValidation();
    onNodesChange(changes);
  }, [invalidateWorkflowValidation, onNodesChange]);

  const handleEdgesChange = useCallback((changes: EdgeChange<ActivityEdge>[]) => {
    const hasUserChange = changes.some((change) => change.type === 'remove' || change.type === 'add');
    if (hasUserChange) invalidateWorkflowValidation();
    onEdgesChange(changes);
  }, [invalidateWorkflowValidation, onEdgesChange]);

  const updateNodeConfig = useCallback((nodeId: string, key: string, value: unknown) => {
    invalidateWorkflowValidation();
    setNodes((items) => items.map((node) => {
      if (node.type !== 'activity' || node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          config: {
            ...(node.data.config ?? {}),
            [key]: value,
          },
        },
      } as FlowNode;
    }));
  }, [invalidateWorkflowValidation, setNodes]);

  const updateEdgeRoute = useCallback((edgeId: string, route: WorkflowEdgeRoute) => {
    invalidateWorkflowValidation();
    setEdges((items) => items.map((item) => {
      if (item.id !== edgeId) return item;
      return {
        ...item,
        label: route.label ?? route.key ?? route.type,
        data: {
          ...(item.data ?? {}),
          route,
        },
      };
    }));
  }, [invalidateWorkflowValidation, setEdges]);

  const addNodeAt = useCallback((def: ActivityNodeData, x: number, y: number) => {
    invalidateWorkflowValidation();
    const id = `${def.id}-${Date.now().toString(36)}`;
    setNodes((items) => items.concat(flowNode(def.id, x, y, { id })));
    if (nodePicker?.sourceNodeId) {
      const newEdge = edge(nodePicker.sourceNodeId, id, nodePicker.sourceLabel ?? '', nodePicker.sourceHandle);
      setEdges((items) => items.concat(newEdge));
      setSelectedEdgeIds([newEdge.id]);
      setSelectedNodeIds([]);
    }
    setLastAdded(def.title);
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, nodePicker, setEdges, setNodes]);

  const addCatalogNode = useCallback((def: ActivityNodeData) => {
    const offset = nodes.length * 18;
    addNodeAt(def, 180 + (offset % 760), 120 + (offset % 420));
  }, [addNodeAt, nodes.length]);

  const openOutputNodePicker = useCallback((request: OutputAddRequest) => {
    const flowPosition = reactFlowInstance?.screenToFlowPosition({
      x: request.clientX + 220,
      y: request.clientY - 60,
    }) ?? { x: request.clientX + 180, y: request.clientY };
    setNodeSearch('');
    setContextMenu(null);
    setSelectedNodeIds([request.nodeId]);
    setSelectedEdgeIds([]);
    setNodePicker({
      x: Math.min(request.clientX + 18, window.innerWidth - 286),
      y: Math.max(14, Math.min(request.clientY - 64, window.innerHeight - 590)),
      flowX: flowPosition.x,
      flowY: flowPosition.y,
      sourceNodeId: request.nodeId,
      sourceHandle: `output-${request.outputIndex}`,
      sourceLabel: request.outputLabel,
    });
  }, [reactFlowInstance]);

  const addNoteAt = useCallback((x: number, y: number) => {
    invalidateWorkflowValidation();
    const id = `note-${Date.now().toString(36)}`;
    setNodes((items) => items.concat({
      id,
      type: 'note',
      position: { x, y },
      data: { text: '补充活动流程说明、配置约束或异常处理备注。' },
    } satisfies NoteNode));
    setLastAdded('备注');
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, setNodes]);

  const deleteNodes = useCallback((nodeIds: string[]) => {
    if (nodeIds.length === 0) return;
    invalidateWorkflowValidation();
    const nodeIdSet = new Set(nodeIds);
    setNodes((items) => {
      const deletedGroups = items.filter((item) => nodeIdSet.has(item.id) && item.type === 'group');
      const groupPositionById = new Map(deletedGroups.map((item) => [item.id, item.position]));
      return items
        .filter((item) => !nodeIdSet.has(item.id))
        .map((item) => {
          if (!item.parentId || !groupPositionById.has(item.parentId)) return item;
          const parentPosition = groupPositionById.get(item.parentId) ?? { x: 0, y: 0 };
          const { parentId: _parentId, extent: _extent, ...rest } = item;
          return { ...rest, position: { x: parentPosition.x + item.position.x, y: parentPosition.y + item.position.y } } as FlowNode;
        });
    });
    setEdges((items) => items.filter((item) => !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target)));
    setSelectedNodeIds((items) => items.filter((item) => !nodeIdSet.has(item)));
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, setEdges, setNodes]);

  const deleteEdges = useCallback((edgeIds: string[]) => {
    if (edgeIds.length === 0) return;
    invalidateWorkflowValidation();
    const edgeIdSet = new Set(edgeIds);
    setEdges((items) => items.filter((item) => !edgeIdSet.has(item.id)));
    setSelectedEdgeIds((items) => items.filter((item) => !edgeIdSet.has(item)));
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, setEdges]);

  const duplicateNode = useCallback((nodeId: string) => {
    const current = nodes.find((item) => item.id === nodeId);
    if (!current) return;
    invalidateWorkflowValidation();
    const duplicate = {
      ...current,
      id: `${current.type}-${Date.now().toString(36)}`,
      selected: false,
      parentId: undefined,
      extent: undefined,
      position: { x: current.position.x + 42, y: current.position.y + 42 },
      data: { ...current.data },
    } as FlowNode;
    setNodes((items) => items.concat(duplicate));
    setLastAdded(`${'title' in current.data ? current.data.title : '备注'} 副本`);
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, nodes, setNodes]);

  const openPaneMenu = useCallback((event: MouseEvent | React.MouseEvent) => {
    event.preventDefault();
    const flowPosition = reactFlowInstance?.screenToFlowPosition({ x: event.clientX, y: event.clientY }) ?? { x: 120, y: 120 };
    setContextMenu({ type: 'pane', x: event.clientX, y: event.clientY, flowX: flowPosition.x, flowY: flowPosition.y });
  }, [reactFlowInstance]);

  const openNodeMenu = useCallback((event: React.MouseEvent, node: FlowNode) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedNodeIds([node.id]);
    setSelectedEdgeIds([]);
    setContextMenu({ type: 'node', x: event.clientX, y: event.clientY, nodeId: node.id });
  }, []);

  const openEdgeMenu = useCallback((event: React.MouseEvent, edgeItem: ActivityEdge) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedEdgeIds([edgeItem.id]);
    setContextMenu({ type: 'edge', x: event.clientX, y: event.clientY, edgeId: edgeItem.id });
  }, []);

  const deleteSelected = useCallback(() => {
    const nodeIdSet = new Set(selectedNodeIds);
    const edgeIdSet = new Set(selectedEdgeIds);
    if (nodeIdSet.size === 0 && edgeIdSet.size === 0) return;
    invalidateWorkflowValidation();
    setNodes((items) => items.filter((item) => !nodeIdSet.has(item.id)));
    setEdges((items) => items.filter((item) => !edgeIdSet.has(item.id) && !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target)));
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, selectedEdgeIds, selectedNodeIds, setEdges, setNodes]);

  const clearCanvas = useCallback(() => {
    invalidateWorkflowValidation();
    setSelectedTemplate('default');
    setNodes([]);
    setEdges([]);
    setLastAdded('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, setEdges, setNodes]);

  const importWorkflowJSON = useCallback(async (file: File | null | undefined) => {
    if (!file) return;
    try {
      const raw = await file.text();
      const parsed = JSON.parse(raw) as WorkflowDefinition | { definition?: WorkflowDefinition };
      const definition = 'definition' in parsed && parsed.definition ? parsed.definition : parsed as WorkflowDefinition;
      const hydrated = hydrateWorkflowDefinition(definition);
      invalidateWorkflowValidation();
      setNodes(hydrated.nodes);
      setEdges(hydrated.edges);
      setSelectedNodeIds([]);
      setSelectedEdgeIds([]);
      setSelectedTemplate('default');
      setDraftDirty(true);
      setDraftSaveState('dirty');
      setDraftSaveError('');
      draftRevisionRef.current += 1;
      setWorkflowStatus('已导入 JSON，保存后写入当前活动 draft');
      setLastAdded('导入 JSON');
      window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.16, duration: 320 }));
    } catch (error) {
      setWorkflowStatus(`导入 JSON 失败：${(error as Error).message}`);
    } finally {
      if (importFileInputRef.current) {
        importFileInputRef.current.value = '';
      }
    }
  }, [invalidateWorkflowValidation, reactFlowInstance, setEdges, setNodes]);

  const resetDefault = useCallback(() => {
    invalidateWorkflowValidation();
    setSelectedTemplate('default');
    setNodes(baseNodes);
    setEdges(baseEdges);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.14, duration: 360 }));
  }, [closeFloatingUi, invalidateWorkflowValidation, reactFlowInstance, setEdges, setNodes]);

  const loadSimpleTemplate = useCallback(() => {
    invalidateWorkflowValidation();
    const template = simpleTemplate();
    setSelectedTemplate('simple');
    setNodes(template.nodes);
    setEdges(template.edges);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.16, duration: 360 }));
  }, [closeFloatingUi, invalidateWorkflowValidation, reactFlowInstance, setEdges, setNodes]);

  const autoLayout = useCallback(() => {
    invalidateWorkflowValidation();
    setNodes((items) => layoutFlowNodes(items));
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => {
      reactFlowInstance?.setViewport({ x: 24, y: 36, zoom: 1 }, { duration: 360 });
      setZoomPercent(100);
    });
  }, [closeFloatingUi, invalidateWorkflowValidation, reactFlowInstance, setNodes]);

  const groupSelectedNodes = useCallback(() => {
    const selected = nodes.filter((node): node is ActivityNode => selectedNodeIds.includes(node.id) && node.type === 'activity');
    if (selected.length < 2) return;
    invalidateWorkflowValidation();
    const minX = Math.min(...selected.map((node) => node.position.x));
    const minY = Math.min(...selected.map((node) => node.position.y));
    const maxX = Math.max(...selected.map((node) => node.position.x + ACTIVITY_NODE_WIDTH));
    const maxY = Math.max(...selected.map((node) => node.position.y + activityNodeHeight(node)));
    const groupId = `group-${Date.now().toString(36)}`;
    const groupX = minX - 28;
    const groupY = minY - 58;
    const groupNode: GroupNode = {
      id: groupId,
      type: 'group',
      position: { x: groupX, y: groupY },
      data: { title: `组合 ${selected.length} 个节点`, width: maxX - minX + 56, height: maxY - minY + 92 },
      selectable: true,
    };
    const selectedSet = new Set(selected.map((node) => node.id));
    setNodes((items) => [
      groupNode,
      ...items.map((node) => {
        if (!selectedSet.has(node.id)) return node;
        return { ...node, parentId: groupId, extent: 'parent', position: { x: node.position.x - groupX, y: node.position.y - groupY } } as FlowNode;
      }),
    ]);
    setSelectedNodeIds([groupId]);
    setSelectedEdgeIds([]);
    setLastAdded(groupNode.data.title);
  }, [invalidateWorkflowValidation, nodes, selectedNodeIds, setNodes]);

  const updateAlignmentGuide = useCallback((dragged: FlowNode) => {
    if (dragged.type !== 'activity') {
      setAlignmentGuide({});
      return;
    }
    const tolerance = 8;
    const nextGuide: AlignmentGuide = {};
    const draggedHeight = activityNodeHeight(dragged);
    const draggedX = { left: dragged.position.x, center: dragged.position.x + ACTIVITY_NODE_WIDTH / 2, right: dragged.position.x + ACTIVITY_NODE_WIDTH };
    const draggedY = { top: dragged.position.y, center: dragged.position.y + draggedHeight / 2, bottom: dragged.position.y + draggedHeight };
    nodes.forEach((node) => {
      if (node.id === dragged.id || node.type !== 'activity' || node.parentId || dragged.parentId) return;
      const referenceHeight = activityNodeHeight(node);
      const referenceX = { left: node.position.x, center: node.position.x + ACTIVITY_NODE_WIDTH / 2, right: node.position.x + ACTIVITY_NODE_WIDTH };
      const referenceY = { top: node.position.y, center: node.position.y + referenceHeight / 2, bottom: node.position.y + referenceHeight };
      (Object.keys(referenceX) as Array<keyof typeof referenceX>).forEach((refKind) => {
        (Object.keys(draggedX) as Array<keyof typeof draggedX>).forEach((dragKind) => {
          if (nextGuide.vertical || Math.abs(referenceX[refKind] - draggedX[dragKind]) > tolerance) return;
          const flowStart = Math.min(node.position.y, dragged.position.y) - 28;
          const flowEnd = Math.max(node.position.y + referenceHeight, dragged.position.y + draggedHeight) + 28;
          nextGuide.vertical = {
            kind: refKind,
            start: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: flowStart }).y ?? flowStart,
            end: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: flowEnd }).y ?? flowEnd,
            screen: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: node.position.y }).x ?? referenceX[refKind],
          };
        });
      });
      (Object.keys(referenceY) as Array<keyof typeof referenceY>).forEach((refKind) => {
        (Object.keys(draggedY) as Array<keyof typeof draggedY>).forEach((dragKind) => {
          if (nextGuide.horizontal || Math.abs(referenceY[refKind] - draggedY[dragKind]) > tolerance) return;
          const flowStart = Math.min(node.position.x, dragged.position.x) - 28;
          const flowEnd = Math.max(node.position.x + ACTIVITY_NODE_WIDTH, dragged.position.x + ACTIVITY_NODE_WIDTH) + 28;
          nextGuide.horizontal = {
            kind: refKind,
            start: reactFlowInstance?.flowToScreenPosition({ x: flowStart, y: referenceY[refKind] }).x ?? flowStart,
            end: reactFlowInstance?.flowToScreenPosition({ x: flowEnd, y: referenceY[refKind] }).x ?? flowEnd,
            screen: reactFlowInstance?.flowToScreenPosition({ x: node.position.x, y: referenceY[refKind] }).y ?? referenceY[refKind],
          };
        });
      });
    });
    setAlignmentGuide(nextGuide);
  }, [nodes, reactFlowInstance]);

  const validationHighlights = useMemo(() => {
    const blockNodeIds = new Set<string>();
    const warningNodeIds = new Set<string>();
    const blockEdgeIds = new Set<string>();
    const warningEdgeIds = new Set<string>();
    const nodeIssues = new Map<string, WorkflowValidationIssue[]>();
    const edgeIssues = new Map<string, WorkflowValidationIssue[]>();
    workflowValidation?.items.forEach((item) => {
      const nodeSet = item.severity === 'BLOCK' ? blockNodeIds : warningNodeIds;
      const edgeSet = item.severity === 'BLOCK' ? blockEdgeIds : warningEdgeIds;
      if (item.nodeId) {
        nodeSet.add(item.nodeId);
        nodeIssues.set(item.nodeId, [...(nodeIssues.get(item.nodeId) ?? []), item]);
      }
      if (item.edgeId) {
        edgeSet.add(item.edgeId);
        edgeIssues.set(item.edgeId, [...(edgeIssues.get(item.edgeId) ?? []), item]);
      }
    });
    return { blockNodeIds, warningNodeIds, blockEdgeIds, warningEdgeIds, nodeIssues, edgeIssues };
  }, [workflowValidation]);

  const removedDiffNodes = useMemo(() => {
    if (!activeWorkflowVersion?.definition) return [];
    const currentNodeIds = new Set(nodes.map((node) => node.id));
    return activeWorkflowVersion.definition.nodes
      .filter((node) => diffHighlights.removedNodeIds.has(node.id) && !currentNodeIds.has(node.id))
      .map((node) => {
        const def = getDef(node.type);
        return {
          id: `diff-removed-node-${node.id}`,
          type: 'activity',
          position: node.position ?? { x: 120, y: 120 },
          selectable: false,
          draggable: false,
          className: 'diff-node-removed',
          data: {
            ...def,
            id: node.type,
            code: node.code ?? def.code,
            title: node.title ?? def.title,
            kind: (node.kind as ActivityNodeData['kind']) ?? def.kind,
            inputs: node.inputs ?? def.inputs,
            outputs: node.outputs ?? def.outputs,
            config: isRecord(node.config) ? node.config : undefined,
          },
        } as ActivityNode;
      });
  }, [activeWorkflowVersion?.definition, diffHighlights.removedNodeIds, nodes]);

  const visibleNodes = useMemo(() => nodes.map((node) => {
    const validationClass = validationHighlights.blockNodeIds.has(node.id)
      ? 'validation-block'
      : validationHighlights.warningNodeIds.has(node.id)
        ? 'validation-warning'
        : '';
    const diffClass = diffHighlights.addedNodeIds.has(node.id)
      ? 'diff-node-added'
      : diffHighlights.changedNodeIds.has(node.id)
        ? 'diff-node-changed'
        : '';
    if (node.type === 'activity') {
      const issues = validationHighlights.nodeIssues.get(node.id) ?? [];
      const validationState = issues.some((item) => item.severity === 'BLOCK')
        ? 'BLOCK'
        : issues.some((item) => item.severity === 'WARNING')
          ? 'WARNING'
          : undefined;
      const routeIssues = workflowValidation
        ? issues
          .filter((item) => item.code.startsWith('WORKFLOW_BRANCH_') || item.code.startsWith('WORKFLOW_EDGE_ROUTE_') || item.code === 'WORKFLOW_OUTPUT_DUPLICATED_EDGE')
          .map((item) => ({ code: item.code, message: item.message, path: item.path }))
        : undefined;
      return {
        ...node,
        className: cx(node.className, validationClass, diffClass),
        data: {
          ...node.data,
          validationState,
          validationCount: issues.length,
          validationMessage: issues[0]?.message,
          routeIssues,
          onDelete: (nodeId: string) => deleteNodes([nodeId]),
          onAddFromOutput: openOutputNodePicker,
          onHoverChange: (nodeId: string, hovering: boolean) => setHoveredNodeId(hovering ? nodeId : ''),
        },
      } as ActivityNode;
    }
    return { ...node, className: cx(node.className, validationClass, diffClass), data: { ...node.data, onDelete: (nodeId: string) => deleteNodes([nodeId]) } } as FlowNode;
  }).concat(removedDiffNodes), [deleteNodes, diffHighlights.addedNodeIds, diffHighlights.changedNodeIds, nodes, openOutputNodePicker, removedDiffNodes, validationHighlights, workflowValidation]);

  const removedDiffEdges = useMemo(() => {
    if (!activeWorkflowVersion?.definition) return [];
    const currentEdgeIds = new Set(edges.map((item) => item.id));
    const currentNodeIds = new Set(nodes.map((node) => node.id));
    const mapNodeId = (nodeId: string) => (currentNodeIds.has(nodeId) ? nodeId : diffHighlights.removedNodeIds.has(nodeId) ? `diff-removed-node-${nodeId}` : nodeId);
    return activeWorkflowVersion.definition.edges
      .filter((item) => diffHighlights.removedEdgeIds.has(item.id) && !currentEdgeIds.has(item.id))
      .map((item): ActivityEdge => ({
        id: `diff-removed-edge-${item.id}`,
        source: mapNodeId(item.source),
        target: mapNodeId(item.target),
        sourceHandle: item.sourceHandle,
        targetHandle: item.targetHandle,
        label: item.label ? `${item.label} deleted` : 'deleted',
        data: { route: item.route },
        markerEnd: { type: MarkerType.ArrowClosed, width: 5, height: 5, color: '#98a2b3' },
        className: 'diff-edge-removed',
      }));
  }, [activeWorkflowVersion?.definition, diffHighlights.removedEdgeIds, diffHighlights.removedNodeIds, edges, nodes]);

  const runTraceEdgeIds = useMemo(() => {
    const selectedExecution = runHistory.executions.find((item) => item.id === runHistory.selectedExecutionId);
    return new Set(workflowRouteTrace(selectedExecution).map((item) => item.edgeId).filter((item): item is string => Boolean(item)));
  }, [runHistory.executions, runHistory.selectedExecutionId]);

  const visibleEdges = useMemo(() => edges.map((item): ActivityEdge => {
    const routeVisual = edgeRouteVisual(item);
    const issues = validationHighlights.edgeIssues.get(item.id) ?? [];
    const edgeHasBlock = validationHighlights.blockEdgeIds.has(item.id);
    const edgeHasWarning = !edgeHasBlock && validationHighlights.warningEdgeIds.has(item.id);
    const diffClass = diffHighlights.addedEdgeIds.has(item.id)
      ? 'diff-edge-added'
      : diffHighlights.changedEdgeIds.has(item.id)
        ? 'diff-edge-changed'
        : '';
    return {
      ...item,
      markerEnd: { type: MarkerType.ArrowClosed, width: 5, height: 5, color: routeVisual.color },
      label: issues.length ? `${item.label ?? item.data?.route?.label ?? ''} !${issues.length}`.trim() : item.label,
      labelStyle: issues.length ? {
        fill: edgeHasBlock ? '#b42318' : '#175cd3',
        fontSize: 11,
        fontWeight: 900,
      } : item.labelStyle,
      labelBgStyle: issues.length ? {
        fill: edgeHasBlock ? '#fff1f3' : '#eff6ff',
        fillOpacity: 0.98,
      } : item.labelBgStyle,
      className: cx(
        item.className,
        routeVisual.className,
        diffClass,
        (item.data?.runSelected || runTraceEdgeIds.has(item.id)) && 'edge-run-selected',
        edgeHasBlock && 'validation-block',
        edgeHasWarning && 'validation-warning',
        item.id === hoveredEdgeId && 'edge-hover',
        hoveredNodeId !== '' && (item.source === hoveredNodeId || item.target === hoveredNodeId) && 'edge-node-hover',
        selectedNodeIds.some((nodeId) => item.source === nodeId || item.target === nodeId) && 'edge-related',
      ),
    } as ActivityEdge;
  }).concat(removedDiffEdges), [diffHighlights.addedEdgeIds, diffHighlights.changedEdgeIds, edges, hoveredEdgeId, hoveredNodeId, removedDiffEdges, runTraceEdgeIds, selectedNodeIds, validationHighlights]);

  const onConnect = useCallback((params: Connection) => {
    if (!params.source || !params.target) return;
    invalidateWorkflowValidation();
    const sourceNode = nodes.find((item): item is ActivityNode => item.id === params.source && item.type === 'activity');
    const outputIndex = Number(String(params.sourceHandle ?? '').replace('output-', ''));
    const sourceLabel = Number.isFinite(outputIndex) && outputIndex >= 0 ? sourceNode?.data.outputs[outputIndex] : undefined;
    const newEdge = edge(params.source, params.target, sourceLabel ?? '', params.sourceHandle ?? undefined, params.targetHandle ?? undefined);
    setEdges((items) => items.concat(newEdge));
  }, [invalidateWorkflowValidation, nodes, setEdges]);

  const syncZoom = useCallback(() => {
    const zoom = reactFlowInstance?.getZoom() ?? 1;
    setZoomPercent(Math.round(zoom * 100));
  }, [reactFlowInstance]);

  const zoomTo = useCallback((zoom: number) => {
    reactFlowInstance?.setViewport({ x: 24, y: 36, zoom }, { duration: 180 });
    setZoomPercent(Math.round(zoom * 100));
  }, [reactFlowInstance]);

  const focusNodeById = useCallback((nodeId: string, statusPrefix = '已定位节点') => {
    const node = nodes.find((candidate) => candidate.id === nodeId);
    if (!node) {
      setWorkflowStatus(`未找到节点：${nodeId}`);
      return false;
    }
    setSelectedNodeIds([node.id]);
    setSelectedEdgeIds([]);
    const nodeWidth = node.type === 'activity' ? ACTIVITY_NODE_WIDTH : (node.width ?? 180);
    const nodeHeight = node.type === 'activity' ? activityNodeHeight(node) : (node.height ?? 120);
    reactFlowInstance?.setCenter(node.position.x + nodeWidth / 2, node.position.y + nodeHeight / 2, {
      zoom: Math.max(reactFlowInstance.getZoom(), 1),
      duration: 360,
    });
    setWorkflowStatus(`${statusPrefix}：${node.id}`);
    return true;
  }, [nodes, reactFlowInstance]);

  const focusValidationIssue = useCallback((item: WorkflowValidationIssue) => {
    if (item.nodeId) {
      if (focusNodeById(item.nodeId)) {
        return;
      }
    }
    if (item.edgeId) {
      const targetEdge = edges.find((candidate) => candidate.id === item.edgeId);
      if (targetEdge) {
        setSelectedNodeIds([]);
        setSelectedEdgeIds([targetEdge.id]);
        const relatedNodes = nodes.filter((node) => node.id === targetEdge.source || node.id === targetEdge.target);
        if (relatedNodes.length > 0) {
          reactFlowInstance?.fitView({ nodes: relatedNodes.map((node) => ({ id: node.id })), padding: 0.28, duration: 360 });
        }
        setWorkflowStatus(`已定位连线：${targetEdge.id}`);
        return;
      }
    }
    setWorkflowStatus(`未找到检查项定位目标：${item.code}`);
  }, [edges, focusNodeById, nodes, reactFlowInstance]);

  const focusWorkflowDiffItem = useCallback((item: WorkflowDiffItem) => {
    if (item.nodeId) {
      const currentNode = nodes.find((candidate) => candidate.id === item.nodeId);
      const activeNode = activeWorkflowVersion?.definition.nodes.find((candidate) => candidate.id === item.nodeId);
      const targetNode = currentNode ?? (activeNode ? {
        id: `diff-removed-node-${activeNode.id}`,
        type: 'activity',
        position: activeNode.position ?? { x: 120, y: 120 },
        data: { id: activeNode.type },
      } as ActivityNode : undefined);
      if (targetNode) {
        setSelectedNodeIds(currentNode ? [currentNode.id] : []);
        setSelectedEdgeIds([]);
        const nodeWidth = targetNode.type === 'activity' ? ACTIVITY_NODE_WIDTH : (targetNode.width ?? 180);
        const nodeHeight = targetNode.type === 'activity' ? activityNodeHeight(targetNode) : (targetNode.height ?? 120);
        reactFlowInstance?.setCenter(targetNode.position.x + nodeWidth / 2, targetNode.position.y + nodeHeight / 2, {
          zoom: Math.max(reactFlowInstance.getZoom(), 1),
          duration: 360,
        });
        setWorkflowStatus(`已定位 diff 节点：${item.label}`);
        return;
      }
    }
    if (item.edgeId) {
      const currentEdge = edges.find((candidate) => candidate.id === item.edgeId);
      const activeEdge = activeWorkflowVersion?.definition.edges.find((candidate) => candidate.id === item.edgeId);
      const relatedNodeIds = [currentEdge?.source ?? activeEdge?.source, currentEdge?.target ?? activeEdge?.target].filter((id): id is string => Boolean(id));
      const relatedNodes = nodes.filter((node) => relatedNodeIds.includes(node.id));
      const activeRelatedNodes = activeWorkflowVersion?.definition.nodes.filter((node) => relatedNodeIds.includes(node.id)) ?? [];
      setSelectedNodeIds([]);
      setSelectedEdgeIds(currentEdge ? [currentEdge.id] : []);
      if (relatedNodes.length > 0) {
        reactFlowInstance?.fitView({ nodes: relatedNodes.map((node) => ({ id: node.id })), padding: 0.28, duration: 360 });
      } else if (activeRelatedNodes.length > 0) {
        const minX = Math.min(...activeRelatedNodes.map((node) => node.position?.x ?? 120));
        const maxX = Math.max(...activeRelatedNodes.map((node) => (node.position?.x ?? 120) + ACTIVITY_NODE_WIDTH));
        const minY = Math.min(...activeRelatedNodes.map((node) => node.position?.y ?? 120));
        const maxY = Math.max(...activeRelatedNodes.map((node) => (node.position?.y ?? 120) + ACTIVITY_NODE_NORMAL_HEIGHT));
        reactFlowInstance?.setCenter((minX + maxX) / 2, (minY + maxY) / 2, {
          zoom: Math.max(reactFlowInstance.getZoom(), 1),
          duration: 360,
        });
      }
      setWorkflowStatus(`已定位 diff 连线：${item.label}`);
      return;
    }
    setWorkflowStatus(`未找到 diff 定位目标：${item.label}`);
  }, [activeWorkflowVersion?.definition, edges, nodes, reactFlowInstance]);

  const saveDraft = useCallback(async (options: { silent?: boolean } = {}): Promise<boolean> => {
    const failSave = (message: string) => {
      setDraftSaveState('failed');
      setDraftSaveError(message);
      if (!options.silent) setWorkflowStatus(message);
      return false;
    };
    if (!activityId) {
      return failSave('需要先绑定活动');
    }
    if (!canEditActiveWorkflow) {
      return failSave(`当前角色 ${activeWorkspaceRole} 不能编辑 draft`);
    }
    const placement = activityPlacements[activityId];
    if (!placement?.workspaceId || !placement.folderId) {
      return failSave('保存前需要先把活动绑定到 workspace / folder');
    }
    if (nodeConfigSchemas.length === 0) {
      return failSave('节点配置 schema 尚未加载，稍后再保存');
    }
    const definition = serializeWorkflowDefinition(nodes, edges, nodeConfigSchemas, reactFlowInstance?.getViewport(), {
      workspaceId: placement.workspaceId,
      folderId: placement.folderId,
      activityId,
    });
    const revisionAtStart = draftRevisionRef.current;
    try {
      setDraftSaveState('saving');
      setDraftSaveError('');
      await fetchJSON(`/admin/v1/activities/${activityId}/workflow/draft`, {
        method: 'PUT',
        body: JSON.stringify({ operator: 'admin-flow', definition }),
      });
      if (!options.silent) setWorkflowStatus('草稿已保存');
      savedDraftRevisionRef.current = revisionAtStart;
      if (draftRevisionRef.current === revisionAtStart) {
        setDraftDirty(false);
        setDraftSaveState('saved');
        setLastSavedAt(new Date().toISOString());
      } else {
        setDraftDirty(true);
        setDraftSaveState('dirty');
      }
      setLastAdded('保存草稿');
      return true;
    } catch (error) {
      const message = `保存失败：${(error as Error).message}`;
      setDraftSaveState('failed');
      setDraftSaveError(message);
      if (!options.silent) setWorkflowStatus(message);
      return false;
    }
  }, [activityId, activityPlacements, activeWorkspaceRole, canEditActiveWorkflow, edges, nodeConfigSchemas, nodes, reactFlowInstance]);

  useEffect(() => {
    if (!activityId || !canEditActiveWorkflow || !draftDirty || draftSaveState === 'saving') return;
    const timer = window.setTimeout(() => {
      void saveDraft({ silent: true });
    }, 1800);
    return () => window.clearTimeout(timer);
  }, [activityId, canEditActiveWorkflow, draftDirty, draftSaveState, saveDraft]);

  useEffect(() => {
    const shouldWarn = draftDirty || draftSaveState === 'dirty' || draftSaveState === 'saving' || draftSaveState === 'failed';
    if (!shouldWarn) return undefined;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '当前 workflow draft 还有未保存的改动。';
      return event.returnValue;
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [draftDirty, draftSaveState]);

  const validateCurrentDraft = useCallback(async (): Promise<WorkflowValidationResult | null> => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return null;
    }
    setShowChecklist(true);
    setValidatingWorkflow(true);
    try {
      const draftSaved = await saveDraft({ silent: true });
      if (!draftSaved) {
        return null;
      }
      const result = await fetchJSON<WorkflowValidationResult>(`/admin/v1/activities/${activityId}/workflow/draft/validate`, {
        method: 'POST',
      });
      setWorkflowValidation(result);
      if (result.valid) {
        setWorkflowStatus('发布检查通过');
      } else {
        setWorkflowStatus(`发布检查未通过：${result.summary.blockCount} 个阻塞项`);
      }
      return result;
    } catch (error) {
      setWorkflowStatus(`检查失败：${(error as Error).message}`);
      return null;
    } finally {
      setValidatingWorkflow(false);
    }
  }, [activityId, saveDraft]);

  const publishWorkflow = useCallback(async (releaseNote = '') => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      setPublishState('failed');
      setPublishError('需要先绑定活动');
      return;
    }
    if (!canPublishActiveWorkflow) {
      setWorkflowStatus(`当前角色 ${activeWorkspaceRole} 不能发布 workflow`);
      setPublishState('failed');
      setPublishError(`当前角色 ${activeWorkspaceRole} 不能发布 workflow`);
      return;
    }
    try {
      setPublishState('publishing');
      setPublishError('');
      const validation = await validateCurrentDraft();
      if (!validation?.valid) {
        const message = validation ? `发布被阻止：${validation.summary.blockCount} 个阻塞项需要修复` : '发布被阻止：校验未完成';
        setPublishState('failed');
        setPublishError(message);
        return;
      }
      await fetchJSON(`/admin/v1/activities/${activityId}/workflow/publish`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow', releaseNote }),
      });
      await refreshWorkflowVersions(activityId);
      setIsPublished(true);
      setShowChecklist(true);
      setWorkflowStatus(releaseNote ? `已发布 workflow version：${releaseNote}` : '已发布 workflow version');
      setLastAdded('发布版本');
      setPublishState('published');
      setPublishError('');
    } catch (error) {
      const message = `发布失败：${(error as Error).message}`;
      setWorkflowStatus(message);
      setPublishState('failed');
      setPublishError(message);
    }
  }, [activityId, activeWorkspaceRole, canPublishActiveWorkflow, refreshWorkflowVersions, validateCurrentDraft]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const mod = event.metaKey || event.ctrlKey;
      if (event.key === 'Escape') {
        closeFloatingUi();
        setShowChecklist(false);
        setShowTemplatePanel(false);
        setDebugPanel((state) => ({ ...state, open: false }));
        return;
      }
      if (!mod) return;
      if (event.key.toLowerCase() === 's') {
        event.preventDefault();
        void saveDraft();
        return;
      }
      if (event.key === 'Enter') {
        event.preventDefault();
        void validateCurrentDraft();
        return;
      }
      if (event.shiftKey && event.key.toLowerCase() === 'p') {
        event.preventDefault();
        document.querySelector<HTMLButtonElement>('.publish-action')?.click();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [closeFloatingUi, publishWorkflow, saveDraft, validateCurrentDraft]);

  const rollbackWorkflowVersion = useCallback(async (versionId: string, note = '') => {
    if (!activityId) return;
    if (!canPublishActiveWorkflow) {
      setWorkflowStatus(`当前角色 ${activeWorkspaceRole} 不能回滚版本`);
      return;
    }
    try {
      const created = await fetchJSON<WorkflowVersion>(`/admin/v1/activities/${activityId}/workflow/versions/${versionId}/rollback`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow', note }),
      });
      await refreshWorkflowVersions(activityId);
      setWorkflowStatus(note ? `已回滚并发布 workflow v${created.version}：${note}` : `已回滚并发布 workflow v${created.version}`);
      setIsPublished(true);
    } catch (error) {
      setWorkflowStatus(`回滚失败：${(error as Error).message}`);
    }
  }, [activityId, activeWorkspaceRole, canPublishActiveWorkflow, refreshWorkflowVersions]);

  const archiveWorkflowVersion = useCallback(async (versionId: string, note = '') => {
    if (!activityId) return;
    if (!canPublishActiveWorkflow) {
      setWorkflowStatus(`当前角色 ${activeWorkspaceRole} 不能归档版本`);
      return;
    }
    try {
      await fetchJSON<WorkflowVersion>(`/admin/v1/activities/${activityId}/workflow/versions/${versionId}/archive`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow', note }),
      });
      await refreshWorkflowVersions(activityId);
      setWorkflowStatus(note ? `版本已归档：${note}` : '版本已归档');
    } catch (error) {
      setWorkflowStatus(`归档失败：${(error as Error).message}`);
    }
  }, [activityId, activeWorkspaceRole, canPublishActiveWorkflow, refreshWorkflowVersions]);

  const changeActivity = useCallback((nextActivityId: string) => {
    if ((draftDirty || draftSaveState === 'dirty' || draftSaveState === 'saving' || draftSaveState === 'failed') && !window.confirm('当前 workflow draft 还有未保存的改动，确定切换吗？')) return;
    setActivityId(nextActivityId);
    setHomeWorkspaceId('');
    setHomeFolderId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setWorkflowValidation(null);
    setContextMenu(null);
    setNodePicker(null);
    setRunHistory((state) => ({ ...state, open: false, selectedExecutionId: '', nodeExecutions: [], selectedNodeExecutionId: '', replayStep: 0, compareExecutionId: '', compareNodeExecutions: [], compareResult: undefined, compareLoading: false }));
    replaceUrlParams({ activityId: nextActivityId, executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
    setWorkflowStatus('切换活动中...');
  }, [draftDirty, draftSaveState]);

  const navigateWorkspaceHome = useCallback((workspaceId: string, folderId = '') => {
    if ((draftDirty || draftSaveState === 'dirty' || draftSaveState === 'saving' || draftSaveState === 'failed') && !window.confirm('当前 workflow draft 还有未保存的改动，确定离开画布吗？')) return;
    setHomeWorkspaceId(workspaceId);
    setHomeFolderId(folderId);
    setActivityId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setContextMenu(null);
    setNodePicker(null);
    window.history.replaceState(null, '', window.location.pathname);
    setWorkflowStatus(folderId ? '已返回 folder 视图' : '已返回 workspace 视图');
  }, [draftDirty, draftSaveState]);

  const createStudioWorkspace = useCallback(async (input: CreateStudioWorkspaceInput) => {
    const slug = normalizeActivityCode(input.name).toLowerCase().replace(/_/g, '-');
    const created = await fetchJSON<WorkspaceSummary>('/admin/v1/workspaces', {
      method: 'POST',
      body: JSON.stringify({
        tenantId: input.tenantId,
        name: input.name,
        slug,
        visibility: 'TEAM',
        defaultRole: 'VIEWER',
        ownerId: currentActorId,
        createdBy: currentActorId,
      }),
    });
    setWorkspaces((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    setWorkflowStatus(`已创建 workspace：${created.name}`);
    return created;
  }, [currentActorId]);

  const createWorkspaceFolder = useCallback(async (workspaceId: string, name: string) => {
    const workspace = workspaces.find((item) => item.id === workspaceId);
    const created = await fetchJSON<WorkspaceFolderSummary>(`/admin/v1/workspaces/${workspaceId}/folders`, {
      method: 'POST',
      body: JSON.stringify({
        name,
        visibility: 'TEAM',
        sortOrder: (workspaceFolders.filter((folder) => folder.workspaceId === workspaceId).length + 1) * 10,
        createdBy: currentActorId,
      }),
    });
    setWorkspaceFolders((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    if (workspace?.tenantId) {
      setWorkflowStatus(`已创建文件夹：${workspace.name} / ${created.name}`);
    }
    return created;
  }, [currentActorId, workspaceFolders, workspaces]);

  const refreshWorkspaceOps = useCallback(async (workspaceId: string) => {
    const [members, shares, audits] = await Promise.all([
      fetchJSON<{ items: WorkspaceMemberSummary[] }>(`/admin/v1/workspaces/${workspaceId}/members`).then((body) => body.items ?? []),
      fetchJSON<{ items: WorkspaceShareLinkSummary[] }>(`/admin/v1/workspaces/${workspaceId}/share-links?limit=500`).then((body) => body.items ?? []),
      fetchJSON<{ items: WorkspaceAuditLogSummary[] }>(`/admin/v1/workspaces/${workspaceId}/audit-logs?limit=40`).then((body) => body.items ?? []),
    ]);
    setWorkspaceMembers((items) => ({ ...items, [workspaceId]: members }));
    setWorkspaceShareLinks((items) => ({ ...items, [workspaceId]: shares }));
    setWorkspaceAuditLogs((items) => ({ ...items, [workspaceId]: audits }));
  }, []);

  const updateWorkspaceFolder = useCallback(async (workspaceId: string, folderId: string, patch: StudioFolderPatch) => {
    const updated = await fetchJSON<WorkspaceFolderSummary>(`/admin/v1/workspaces/${workspaceId}/folders/${folderId}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    });
    setWorkspaceFolders((items) => items.map((item) => (item.id === updated.id ? updated : item)));
    await refreshWorkspaceOps(workspaceId);
    return updated;
  }, [refreshWorkspaceOps]);

  const createWorkspaceMember = useCallback(async (workspaceId: string, input: { userId: string; email: string; displayName: string; role: WorkspaceRole }) => {
    await fetchJSON<WorkspaceMemberSummary>(`/admin/v1/workspaces/${workspaceId}/members`, {
      method: 'POST',
      body: JSON.stringify({
        userId: input.userId,
        email: input.email,
        displayName: input.displayName,
        role: input.role,
        status: 'ACTIVE',
      }),
    });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const updateWorkspaceMemberRole = useCallback(async (workspaceId: string, memberId: string, role: WorkspaceRole) => {
    await fetchJSON<WorkspaceMemberSummary>(`/admin/v1/workspaces/${workspaceId}/members/${memberId}`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const removeWorkspaceMember = useCallback(async (workspaceId: string, memberId: string) => {
    await fetchJSON<WorkspaceMemberSummary>(`/admin/v1/workspaces/${workspaceId}/members/${memberId}`, { method: 'DELETE' });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const createWorkspaceShareLink = useCallback(async (workspaceId: string, input: { scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY'; folderId?: string; activityId?: string; permission: 'VIEW' | 'EDIT' }) => {
    await fetchJSON<WorkspaceShareLinkSummary>(`/admin/v1/workspaces/${workspaceId}/share-links`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const disableWorkspaceShareLink = useCallback(async (workspaceId: string, linkId: string) => {
    await fetchJSON<WorkspaceShareLinkSummary>(`/admin/v1/workspaces/${workspaceId}/share-links/${linkId}`, { method: 'DELETE' });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const saveEmptyWorkflowDraft = useCallback(async (created: ActivitySummary, input: { workspaceId: string; folderId: string }) => {
    const definition = emptyWorkflowDefinition({
      activityId: created.id,
      workspaceId: created.workspaceId || input.workspaceId,
      folderId: created.folderId || input.folderId,
    });
    await fetchJSON(`/admin/v1/activities/${created.id}/workflow/draft`, {
      method: 'PUT',
      body: JSON.stringify({ operator: currentActorId, definition }),
    });
  }, [currentActorId]);

  const createStudioActivity = useCallback(async (input: CreateStudioActivityInput) => {
    const payload = buildStudioActivityPayload(input);
    const created = await fetchJSON<ActivitySummary>('/admin/v1/activities', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    await saveEmptyWorkflowDraft(created, input);
    setActivities((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    setActivityPlacements((items) => ({
      ...items,
      [created.id]: {
        workspaceId: created.workspaceId || input.workspaceId,
        folderId: created.folderId || input.folderId,
      },
    }));
    setActivityId(created.id);
    setActivityLabel(`${created.activityName} / ${created.activityCode}`);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setContextMenu(null);
    setNodePicker(null);
    setNodes([]);
    setEdges([]);
    setSelectedTemplate('default');
    setWorkflowStatus('活动已创建，空 draft 已保存');
    setWorkflowVersionLabel('workflow 未发布');
    setDraftDirty(false);
    setDraftSaveState('saved');
    setDraftSaveError('');
    setLastSavedAt(new Date().toISOString());
    draftRevisionRef.current = 0;
    savedDraftRevisionRef.current = 0;
    setIsPublished(false);
  }, [saveEmptyWorkflowDraft, setEdges, setNodes]);

  const updateActivitySettings = useCallback(async (input: ActivitySettingsInput) => {
    if (!activityId) return;
    setSavingActivitySettings(true);
    setActivitySettingsError('');
    try {
      const updated = await fetchJSON<ActivitySummary>(`/admin/v1/activities/${activityId}`, {
        method: 'PUT',
        body: JSON.stringify({
          operator: currentActorId,
          activityName: input.activityName.trim(),
          workspaceId: input.workspaceId,
          folderId: input.folderId,
        }),
      });
      setActivities((items) => items.map((item) => (item.id === updated.id ? updated : item)));
      setActivityPlacements((items) => ({
        ...items,
        [updated.id]: {
          workspaceId: updated.workspaceId || input.workspaceId,
          folderId: updated.folderId || input.folderId,
        },
      }));
      setActivityLabel(`${updated.activityName} / ${updated.activityCode}`);
      setWorkflowStatus('活动基础信息已保存');
      setShowActivitySettings(false);
    } catch (error) {
      setActivitySettingsError((error as Error).message);
    } finally {
      setSavingActivitySettings(false);
    }
  }, [activityId, currentActorId]);

  const applyRunOverlay = useCallback((execution: WorkflowExecution | undefined, nodeExecutions: WorkflowNodeExecution[]) => {
    const nodeRunById = new Map(nodeExecutions.map((item) => [item.nodeId, item]));
    const selectedEdgeIds = new Set(workflowRouteTrace(execution).map((item) => item.edgeId).filter((item): item is string => Boolean(item)));
    setNodes((items) => items.map((node) => {
      if (node.type !== 'activity') return node;
      const run = nodeRunById.get(node.id);
      const duration = elapsedMs(run?.startedAt, run?.finishedAt);
      const attempts = workflowAttemptCount(run?.output);
      return {
        ...node,
        data: {
          ...node.data,
          runStatus: run?.status,
          lastRun: run ? {
            status: run.status,
            errorMessage: run.errorMessage,
            input: run.input,
            output: run.output,
            startedAt: run.startedAt,
            finishedAt: run.finishedAt,
            durationMs: duration,
            retryCount: Math.max((attempts ?? 1) - 1, 0),
          } : undefined,
        },
      } as FlowNode;
    }));
    setEdges((items) => items.map((item) => ({
      ...item,
      data: {
        ...(item.data ?? {}),
        runSelected: selectedEdgeIds.has(item.id),
      },
    })));
  }, [setEdges, setNodes]);

  const loadExecutionNodes = useCallback(async (execution: WorkflowExecution | undefined) => {
    if (!execution) {
      setRunHistory((state) => ({ ...state, selectedExecutionId: '', nodeExecutions: [], selectedNodeExecutionId: '', replayStep: 0, compareExecutionId: '', compareNodeExecutions: [], compareResult: undefined, compareLoading: false }));
      replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
      applyRunOverlay(undefined, []);
      return;
    }
    const nodesBody = await fetchJSON<{ items: WorkflowNodeExecution[] }>(`/admin/v1/workflow/executions/${execution.id}/nodes?payload=summary`);
    replaceUrlParams({ executionId: execution.id });
    const selectedSummary = nodesBody.items.find((item) => item.status === 'FAILED') ?? nodesBody.items[0];
    const selectedDetail = selectedSummary
      ? await fetchJSON<WorkflowNodeExecution>(`/admin/v1/workflow/executions/${execution.id}/nodes/${selectedSummary.id}`)
      : undefined;
    const mergedNodes = selectedDetail ? nodesBody.items.map((item) => (item.id === selectedDetail.id ? selectedDetail : item)) : nodesBody.items;
    const failed = mergedNodes.find((item) => item.status === 'FAILED');
    setRunHistory((state) => ({
      ...state,
      selectedExecutionId: execution.id,
      nodeExecutions: mergedNodes,
      selectedNodeExecutionId: (failed ?? mergedNodes[0])?.id ?? '',
      compareExecutionId: state.compareExecutionId === execution.id ? '' : state.compareExecutionId,
      compareNodeExecutions: state.compareExecutionId === execution.id ? [] : state.compareNodeExecutions,
      compareResult: state.compareExecutionId === execution.id ? undefined : state.compareResult,
    }));
    if (new URLSearchParams(window.location.search).get('compareExecutionId') === execution.id) {
      replaceUrlParams({ compareExecutionId: undefined });
    }
    applyRunOverlay(execution, mergedNodes);
    if (failed) {
      setSelectedNodeIds([failed.nodeId]);
      setSelectedEdgeIds([]);
    }
  }, [applyRunOverlay]);

  const loadRunHistory = useCallback(async (versionId: string, keepSelectedExecutionId = '', pageOffset = 0) => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return;
    }
    setRunHistory((state) => ({ ...state, open: true, loading: true, versionId }));
    try {
      const pageLimit = 20;
      const query = new URLSearchParams({ limit: String(pageLimit), offset: String(Math.max(0, pageOffset)) });
      if (versionId) query.set('workflowVersionId', versionId);
      const executions = await fetchJSON<{ items: WorkflowExecution[]; limit?: number; offset?: number; nextOffset?: number; hasMore?: boolean }>(`/admin/v1/activities/${activityId}/workflow/executions?${query.toString()}`);
      const selected = executions.items.find((item) => item.id === keepSelectedExecutionId) ?? executions.items[0];
      setRunHistory((state) => ({
        ...state,
        open: true,
        loading: false,
        versionId,
        executions: executions.items,
        pageLimit: executions.limit ?? pageLimit,
        pageOffset: executions.offset ?? Math.max(0, pageOffset),
        hasMore: Boolean(executions.hasMore),
        nextOffset: executions.nextOffset ?? Math.max(0, pageOffset) + executions.items.length,
        selectedExecutionId: selected?.id ?? '',
        nodeExecutions: selected ? state.nodeExecutions : [],
        selectedNodeExecutionId: selected ? state.selectedNodeExecutionId : '',
        replayStep: selected && (selected.id === state.selectedExecutionId || selected.id === initialRunQueryRef.current.executionId) ? state.replayStep : 0,
      }));
      if (!selected) {
        setWorkflowStatus('暂无运行日志');
        replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
        applyRunOverlay(undefined, []);
        return;
      }
      await loadExecutionNodes(selected);
      setWorkflowStatus(`运行态：${selected.status} / v${selected.workflowVersion}`);
      setLastAdded('运行历史');
    } catch (error) {
      setRunHistory((state) => ({ ...state, loading: false }));
      setWorkflowStatus(runHistoryErrorMessage(error, '运行态失败'));
    }
  }, [activityId, applyRunOverlay, loadExecutionNodes]);

  const refreshRunStatus = useCallback(async () => {
    await loadRunHistory(runHistory.versionId, runHistory.selectedExecutionId, runHistory.pageOffset);
  }, [loadRunHistory, runHistory.pageOffset, runHistory.selectedExecutionId, runHistory.versionId]);

  const changeRunHistoryPage = useCallback(async (offset: number) => {
    await loadRunHistory(runHistory.versionId, '', Math.max(0, offset));
  }, [loadRunHistory, runHistory.versionId]);

  const selectRunExecution = useCallback(async (executionId: string) => {
    const execution = runHistory.executions.find((item) => item.id === executionId);
    try {
      setRunHistory((state) => ({ ...state, replayStep: 0 }));
      replaceUrlParams({ executionId, replayStep: 0 });
      await loadExecutionNodes(execution);
      if (execution) {
        setWorkflowStatus(`运行态：${execution.status} / v${execution.workflowVersion}`);
      }
    } catch (error) {
      setWorkflowStatus(runHistoryErrorMessage(error, '运行详情失败'));
    }
  }, [loadExecutionNodes, runHistory.executions]);

  const selectCompareExecution = useCallback(async (executionId: string, options: { silent?: boolean } = {}) => {
    if (!executionId) {
      setRunHistory((state) => ({ ...state, compareExecutionId: '', compareNodeExecutions: [], compareResult: undefined, compareLoading: false }));
      replaceUrlParams({ compareExecutionId: undefined });
      return;
    }
    const currentExecutionId = runHistory.selectedExecutionId;
    setRunHistory((state) => ({ ...state, compareExecutionId: executionId, compareNodeExecutions: [], compareResult: undefined, compareLoading: true }));
    replaceUrlParams({ compareExecutionId: executionId });
    try {
      const compare = currentExecutionId
        ? await fetchJSON<WorkflowExecutionCompareResult>(`/admin/v1/workflow/executions/${currentExecutionId}/compare?baselineExecutionId=${encodeURIComponent(executionId)}`)
        : undefined;
      setRunHistory((state) => ({
        ...state,
        compareExecutionId: executionId,
        compareNodeExecutions: [],
        compareResult: compare,
        compareLoading: false,
      }));
      if (!options.silent) setWorkflowStatus(`已加载对比 execution：${executionId.slice(0, 12)}`);
    } catch (error) {
      if (isAPIError(error) && error.status === 403) {
        setRunHistory((state) => ({ ...state, compareLoading: false }));
        setWorkflowStatus(runHistoryErrorMessage(error, '对比 execution 加载失败'));
        return;
      }
      try {
        const nodesBody = await fetchJSON<{ items: WorkflowNodeExecution[] }>(`/admin/v1/workflow/executions/${executionId}/nodes`);
        setRunHistory((state) => ({
          ...state,
          compareExecutionId: executionId,
          compareNodeExecutions: nodesBody.items,
          compareResult: undefined,
          compareLoading: false,
        }));
        if (!options.silent) setWorkflowStatus(`已加载对比 execution：${executionId.slice(0, 12)}`);
      } catch {
        setRunHistory((state) => ({ ...state, compareLoading: false }));
        setWorkflowStatus(runHistoryErrorMessage(error, '对比 execution 加载失败'));
      }
    }
  }, [runHistory.selectedExecutionId]);

  useEffect(() => {
    const initialRun = initialRunQueryRef.current;
    if (!activityId || restoredRunQueryRef.current || !initialRun.executionId) return;
    restoredRunQueryRef.current = true;
    setRunHistory((state) => ({
      ...state,
      open: true,
      replayStep: initialRun.replayStep,
      compareExecutionId: initialRun.compareExecutionId,
    }));
    void (async () => {
      await loadRunHistory('', initialRun.executionId);
      if (initialRun.compareExecutionId) {
        await selectCompareExecution(initialRun.compareExecutionId, { silent: true });
      }
    })();
  }, [activityId, loadRunHistory, selectCompareExecution]);

  const selectRunNode = useCallback(async (nodeExecution: WorkflowNodeExecution) => {
    let selected = nodeExecution;
    if (nodeExecution.input === undefined && nodeExecution.output === undefined && nodeExecution.executionId) {
      try {
        selected = await fetchJSON<WorkflowNodeExecution>(`/admin/v1/workflow/executions/${nodeExecution.executionId}/nodes/${nodeExecution.id}`);
      } catch (error) {
        setWorkflowStatus(runHistoryErrorMessage(error, '节点详情失败'));
      }
    }
    setRunHistory((state) => ({
      ...state,
      selectedNodeExecutionId: selected.id,
      nodeExecutions: state.nodeExecutions.map((item) => (item.id === selected.id ? selected : item)),
    }));
    setSelectedNodeIds([selected.nodeId]);
    setSelectedEdgeIds([]);
    const node = nodes.find((candidate) => candidate.id === selected.nodeId);
    if (node) {
      const nodeWidth = node.type === 'activity' ? ACTIVITY_NODE_WIDTH : (node.width ?? 180);
      const nodeHeight = node.type === 'activity' ? activityNodeHeight(node) : (node.height ?? 120);
      reactFlowInstance?.setCenter(node.position.x + nodeWidth / 2, node.position.y + nodeHeight / 2, {
        zoom: Math.max(reactFlowInstance.getZoom(), 1),
        duration: 360,
      });
      setWorkflowStatus(`已定位运行节点：${selected.nodeTitle || selected.nodeId}`);
      return;
    }
    setWorkflowStatus(`未找到运行节点：${selected.nodeId}`);
  }, [nodes, reactFlowInstance]);

  const focusRunEdge = useCallback((edgeId: string) => {
    setSelectedEdgeIds([edgeId]);
    setSelectedNodeIds([]);
    const targetEdge = edges.find((candidate) => candidate.id === edgeId);
    if (targetEdge) {
      const relatedNodes = nodes.filter((node) => node.id === targetEdge.source || node.id === targetEdge.target);
      if (relatedNodes.length > 0) {
        reactFlowInstance?.fitView({ nodes: relatedNodes.map((node) => ({ id: node.id })), padding: 0.28, duration: 360 });
      }
      setWorkflowStatus(`已定位运行路径：${targetEdge.label || targetEdge.id}`);
      return;
    }
    setWorkflowStatus(`未找到运行路径：${edgeId}`);
  }, [edges, nodes, reactFlowInstance]);

  const updateRunReplayStep = useCallback((step: number) => {
    const nextStep = Math.max(0, Math.floor(step));
    setRunHistory((state) => ({ ...state, replayStep: nextStep }));
    replaceUrlParams({ replayStep: nextStep });
  }, []);

  const applyDebugTrace = useCallback((trace: WorkflowExecutionTrace) => {
    const next = traceToRunState(trace);
    setRunHistory((state) => ({
      ...state,
      open: true,
      loading: false,
      versionId: '',
      executions: [trace.execution, ...state.executions.filter((item) => item.id !== trace.execution.id)],
      selectedExecutionId: next.selectedExecutionId,
      nodeExecutions: next.nodeExecutions,
      selectedNodeExecutionId: next.selectedNodeExecutionId,
      replayStep: 0,
      compareExecutionId: '',
      compareNodeExecutions: [],
      compareResult: undefined,
      compareLoading: false,
    }));
    replaceUrlParams({ executionId: trace.execution.id, replayStep: 0, compareExecutionId: undefined });
    applyRunOverlay(trace.execution, trace.nodeExecutions);
    const failed = trace.nodeExecutions.find((item) => item.status === 'FAILED');
    if (failed) {
      setSelectedNodeIds([failed.nodeId]);
      setSelectedEdgeIds([]);
    }
    setWorkflowStatus(`Debug run：${trace.execution.status} · DEBUG_DRAFT`);
  }, [applyRunOverlay]);

  const saveCurrentDebugPayloadTemplate = useCallback(() => {
    const payloadText = debugPanel.payloadText.trim();
    if (!payloadText) {
      setDebugPanel((state) => ({ ...state, message: 'Payload 为空，不能保存模板' }));
      return;
    }
    try {
      JSON.parse(payloadText);
    } catch (error) {
      setDebugPanel((state) => ({ ...state, message: `Payload JSON 无效：${(error as Error).message}` }));
      return;
    }
    const name = window.prompt('模板名称', `Debug Payload ${debugPayloadTemplates.length + 1}`)?.trim();
    if (!name) return;
    const next = [{
      id: `payload_${Date.now().toString(36)}`,
      name,
      payloadText,
      createdAt: new Date().toISOString(),
    }, ...debugPayloadTemplates.filter((item) => item.name !== name)].slice(0, 30);
    setDebugPayloadTemplates(next);
    saveDebugPayloadTemplates(next);
    setDebugPanel((state) => ({ ...state, message: `已保存 payload 模板：${name}` }));
  }, [debugPanel.payloadText, debugPayloadTemplates]);

  const loadDebugPayloadTemplate = useCallback((templateId: string) => {
    const template = debugPayloadTemplates.find((item) => item.id === templateId);
    if (!template) return;
    setDebugPanel((state) => ({ ...state, payloadText: template.payloadText, message: `已载入模板：${template.name}` }));
  }, [debugPayloadTemplates]);

  const useSelectedExecutionPayload = useCallback(() => {
    const execution = runHistory.executions.find((item) => item.id === runHistory.selectedExecutionId);
    const event = workflowExecutionEventPayload(execution);
    if (!event) {
      setDebugPanel((state) => ({ ...state, message: '当前 execution 没有可复用 payload' }));
      return;
    }
    setDebugPanel((state) => ({
      ...state,
      payloadText: JSON.stringify(event, null, 2),
      message: `已复制 execution payload：${execution?.id.slice(0, 12)}`,
    }));
  }, [runHistory.executions, runHistory.selectedExecutionId]);

  const debugPayloadBody = useCallback((startNodeId = '') => {
    const body: Record<string, unknown> = {
      operator: 'admin-flow',
      template: debugPanel.template,
      startNodeId,
    };
    if (startNodeId && runHistory.selectedExecutionId) {
      body.sourceExecutionId = runHistory.selectedExecutionId;
    }
    const payload = debugPanel.payloadText.trim();
    if (payload) {
      body.payload = JSON.parse(payload);
    }
    return body;
  }, [debugPanel.payloadText, debugPanel.template, runHistory.selectedExecutionId]);

  const runDraftDebug = useCallback(async (startNodeId = '') => {
    if (!activityId) return;
    if (!canEditActiveWorkflow) {
      setDebugPanel((state) => ({ ...state, message: `当前角色 ${activeWorkspaceRole} 不能调试 draft` }));
      return;
    }
    setDebugPanel((state) => ({ ...state, loading: true, message: '' }));
    try {
      const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/activities/${activityId}/workflow/debug-run`, {
        method: 'POST',
        body: JSON.stringify(debugPayloadBody(startNodeId)),
      });
      applyDebugTrace(trace);
      const validation = await validateCurrentDraft();
      setDebugPanel((state) => ({ ...state, loading: false, message: `已生成 DEBUG_DRAFT ${trace.execution.id.slice(0, 12)}${validation ? ` · 检查 ${validation.valid ? '通过' : `${validation.summary.blockCount} 个阻塞`}` : ''}` }));
    } catch (error) {
      setDebugPanel((state) => ({ ...state, loading: false, message: `调试失败：${(error as Error).message}` }));
      setWorkflowStatus(`调试失败：${(error as Error).message}`);
    }
  }, [activityId, activeWorkspaceRole, applyDebugTrace, canEditActiveWorkflow, debugPayloadBody, validateCurrentDraft]);

  const rerunSelectedExecution = useCallback(async () => {
    if (!activityId || !runHistory.selectedExecutionId) return;
    if (!canEditActiveWorkflow) {
      setDebugPanel((state) => ({ ...state, message: `当前角色 ${activeWorkspaceRole} 不能重跑 execution` }));
      return;
    }
    setDebugPanel((state) => ({ ...state, loading: true, message: '' }));
    try {
      const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/activities/${activityId}/workflow/executions/${runHistory.selectedExecutionId}/rerun`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow' }),
      });
      applyDebugTrace(trace);
      const validation = await validateCurrentDraft();
      setDebugPanel((state) => ({ ...state, loading: false, message: `已重跑 DEBUG_DRAFT ${trace.execution.id.slice(0, 12)}${validation ? ` · 检查 ${validation.valid ? '通过' : `${validation.summary.blockCount} 个阻塞`}` : ''}` }));
    } catch (error) {
      setDebugPanel((state) => ({ ...state, loading: false, message: `重跑失败：${(error as Error).message}` }));
    }
  }, [activityId, activeWorkspaceRole, applyDebugTrace, canEditActiveWorkflow, runHistory.selectedExecutionId, validateCurrentDraft]);

  const exportSelectedTrace = useCallback(async () => {
    if (!runHistory.selectedExecutionId) return;
    try {
      const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/workflow/executions/${runHistory.selectedExecutionId}/trace`);
      downloadJSON(`workflow-trace-${runHistory.selectedExecutionId}.json`, trace);
    } catch (error) {
      setDebugPanel((state) => ({ ...state, message: runHistoryErrorMessage(error, '导出失败') }));
    }
  }, [runHistory.selectedExecutionId]);

  const refreshWorkflowTemplates = useCallback(async () => {
    const [templateBody, presetBody] = await Promise.all([
      fetchJSON<{ items: WorkflowDraftTemplateSummary[] }>('/admin/v1/workflow/draft-templates'),
      fetchJSON<{ items: WorkflowNodePresetSummary[] }>('/admin/v1/workflow/node-presets'),
    ]);
    setWorkflowTemplates(templateBody.items ?? []);
    setWorkflowNodePresets(presetBody.items ?? []);
  }, []);

  const loadWorkflowTemplateVersions = useCallback(async (templateId: string) => {
    if (!templateId) return;
    setLoadingTemplateVersions(true);
    try {
      const body = await fetchJSON<{ items: WorkflowDraftTemplateSummary[] }>(`/admin/v1/workflow/draft-templates/${templateId}/versions`);
      setWorkflowTemplateVersions((items) => [
        ...items.filter((item) => item.id !== templateId),
        ...(body.items ?? []),
      ]);
    } catch (error) {
      setTemplateMessage(`模板版本加载失败：${(error as Error).message}`);
    } finally {
      setLoadingTemplateVersions(false);
    }
  }, []);

  const applyWorkflowTemplate = useCallback(async (templateId: string, input?: { versionId?: string; version?: number; parameters?: Record<string, string | number> }) => {
    if (!activityId || !canEditActiveWorkflow) {
      setTemplateMessage(`当前角色 ${activeWorkspaceRole} 不能应用模板`);
      return;
    }
    setTemplateMessage('正在应用模板...');
    try {
      const draft = await fetchJSON<WorkflowDraftResponse>(`/admin/v1/activities/${activityId}/workflow/draft-templates/${templateId}/apply`, {
        method: 'POST',
        body: JSON.stringify({
          operator: currentActorId,
          versionId: input?.versionId,
          version: input?.version,
          parameters: input?.parameters ?? {},
          workspaceId: activePlacement?.workspaceId,
          folderId: activePlacement?.folderId,
        }),
      });
      const hydrated = hydrateWorkflowDefinition(draft.definition);
      setNodes(hydrated.nodes);
      setEdges(hydrated.edges);
      setSelectedNodeIds([]);
      setSelectedEdgeIds([]);
      setDraftDirty(false);
      setTemplateMessage('模板已应用，生成的 draft 已保存');
      setWorkflowStatus('已从模板创建 workflow draft');
      window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.16, duration: 360 }));
    } catch (error) {
      setTemplateMessage(`应用模板失败：${(error as Error).message}`);
    }
  }, [activityId, activePlacement?.folderId, activePlacement?.workspaceId, activeWorkspaceRole, canEditActiveWorkflow, currentActorId, reactFlowInstance, setEdges, setNodes]);

  const saveWorkflowTemplate = useCallback(async (input: { name: string; workspaceId: string; folderId: string; visibility: string }) => {
    if (!activityId || !canEditActiveWorkflow) {
      setTemplateMessage(`当前角色 ${activeWorkspaceRole} 不能保存模板`);
      return;
    }
    const templateName = input.name.trim() || `${activityLabel} 模板`;
    setSavingTemplate(true);
    setTemplateMessage('');
    try {
      const savedDraft = await saveDraft();
      if (!savedDraft) return;
      const created = await fetchJSON<WorkflowDraftTemplateSummary>(`/admin/v1/activities/${activityId}/workflow/draft-template`, {
        method: 'POST',
        body: JSON.stringify({
          name: templateName,
          category: '团队模板',
          operator: currentActorId,
          workspaceId: input.workspaceId || activePlacement?.workspaceId,
          folderId: input.folderId || activePlacement?.folderId,
          visibility: input.visibility || 'TEAM',
        }),
      });
      await refreshWorkflowTemplates();
      setTemplateMessage(`已保存模板 ${created.name} v${created.version}`);
    } catch (error) {
      setTemplateMessage(`保存模板失败：${(error as Error).message}`);
    } finally {
      setSavingTemplate(false);
    }
  }, [activityId, activityLabel, activePlacement?.folderId, activePlacement?.workspaceId, activeWorkspaceRole, canEditActiveWorkflow, currentActorId, refreshWorkflowTemplates, saveDraft]);

  return (
    <div className="flow-shell">
      <NodePalette open={activityId ? showPalette : false} selectedTemplate={selectedTemplate} onAddNode={addCatalogNode} onDefault={resetDefault} onSimple={loadSimpleTemplate} />
      <main className={cx('studio', !activityId && 'studio-home')}>
        <StudioChrome
          homeMode={!activityId}
          showPalette={showPalette}
          interactionMode={interactionMode}
          nodeCount={nodes.length}
          edgeCount={edges.length}
          selectedCount={selectedCount}
          selectedNodeCount={selectedNodeIds.length}
          lastAdded={lastAdded}
          activities={activities}
          workspaces={workspaces}
          folders={workspaceFolders}
          placements={activityPlacements}
          activityId={activityId}
          activityLabel={activityLabel}
          workflowStatus={workflowStatus}
          workflowVersionLabel={workflowVersionLabel}
          workflowVersions={workflowVersions}
          draftDirty={draftDirty}
          draftSaveState={draftSaveState}
          draftSaveError={draftSaveError}
          lastSavedAt={lastSavedAt}
          publishState={publishState}
          publishError={publishError}
          draftActiveDiff={draftActiveDiff}
          zoomPercent={zoomPercent}
          showChecklist={showChecklist}
          validatingWorkflow={validatingWorkflow}
          workflowValidation={workflowValidation}
          isPublished={isPublished}
          publishDisabled={validatingWorkflow || (workflowValidation?.summary.blockCount ?? 0) > 0}
          canEditWorkflow={canEditActiveWorkflow}
          canPublishWorkflow={canPublishActiveWorkflow}
          workspaceRole={activeWorkspaceRole}
          onTogglePalette={() => setShowPalette((value) => !value)}
          onActivityChange={changeActivity}
          onWorkspaceNavigate={navigateWorkspaceHome}
          onActivitySettings={() => {
            setActivitySettingsError('');
            setShowActivitySettings(true);
          }}
          onModeChange={setInteractionMode}
          onDefault={resetDefault}
          onSimple={loadSimpleTemplate}
          onTemplates={() => {
            setShowTemplatePanel(true);
            void refreshWorkflowTemplates();
          }}
          onAutoLayout={autoLayout}
          onZoomIn={() => {
            reactFlowInstance?.zoomIn({ duration: 160 });
            window.setTimeout(syncZoom, 180);
          }}
          onZoomOut={() => {
            reactFlowInstance?.zoomOut({ duration: 160 });
            window.setTimeout(syncZoom, 180);
          }}
          onZoomTo={zoomTo}
          onFitView={() => {
            reactFlowInstance?.fitView({ padding: 0.14, duration: 260 });
            window.setTimeout(syncZoom, 280);
          }}
          showMiniMap={showMiniMap}
          onToggleMiniMap={() => setShowMiniMap((value) => !value)}
          onGroup={groupSelectedNodes}
          onDelete={deleteSelected}
          onClear={clearCanvas}
          onSaveDraft={saveDraft}
          onPreview={() => setLastAdded('Preview')}
          onDebugRun={() => {
            const activity = activities.find((item) => item.id === activityId);
            setDebugPanel((state) => ({
              ...state,
              open: true,
              payloadText: state.payloadText || builtinDebugPayloadTemplate(activity),
              message: state.message || 'DEBUG_DRAFT 和生产 execution 分开记录',
            }));
          }}
          onRunHistory={refreshRunStatus}
          onToggleChecklist={() => {
            if (showChecklist) {
              setShowChecklist(false);
              return;
            }
            void validateCurrentDraft();
          }}
          onValidationIssueFocus={focusValidationIssue}
          onWorkflowDiffFocus={focusWorkflowDiffItem}
          onPublish={publishWorkflow}
          onRollbackVersion={rollbackWorkflowVersion}
          onArchiveVersion={archiveWorkflowVersion}
        />
        <section className={cx('canvas-card', !activityId && 'studio-home-card')}>
          <ReactFlowProvider>
            {activityId ? (
              <>
            <ReactFlow
              nodes={visibleNodes}
              edges={visibleEdges}
              nodeTypes={nodeTypes}
              onInit={setReactFlowInstance}
              onMoveEnd={syncZoom}
              onNodesChange={handleNodesChange}
              onEdgesChange={handleEdgesChange}
              onConnect={canEditActiveWorkflow ? onConnect : undefined}
              onPaneClick={() => {
                closeFloatingUi();
                setSelectedNodeIds([]);
                setSelectedEdgeIds([]);
              }}
              onPaneContextMenu={canEditActiveWorkflow ? openPaneMenu : undefined}
              onNodeClick={(event, node) => {
                closeFloatingUi();
                if (event.shiftKey || event.metaKey || event.ctrlKey) {
                  setSelectedNodeIds((items) => (items.includes(node.id) ? items.filter((item) => item !== node.id) : items.concat(node.id)));
                } else {
                  setSelectedNodeIds([node.id]);
                }
                setSelectedEdgeIds([]);
              }}
              onNodeContextMenu={canEditActiveWorkflow ? openNodeMenu : undefined}
              onNodeMouseEnter={(_, node) => setHoveredNodeId(node.id)}
              onNodeMouseLeave={() => setHoveredNodeId('')}
              onEdgeClick={(event, edgeItem) => {
                event.stopPropagation();
                setSelectedEdgeIds([edgeItem.id]);
                setContextMenu({ type: 'edge', x: event.clientX, y: event.clientY, edgeId: edgeItem.id });
              }}
              onEdgeContextMenu={canEditActiveWorkflow ? openEdgeMenu : undefined}
              onEdgeMouseEnter={(_, item) => setHoveredEdgeId(item.id)}
              onEdgeMouseLeave={() => setHoveredEdgeId('')}
              onNodeDrag={canEditActiveWorkflow ? (_, node) => updateAlignmentGuide(node) : undefined}
              onNodeDragStop={() => {
                invalidateWorkflowValidation();
                setAlignmentGuide({});
              }}
              onSelectionChange={({ nodes: selectedNodes, edges: selectedEdges }) => {
                setSelectedNodeIds(selectedNodes.map((node) => node.id));
                setSelectedEdgeIds(selectedEdges.map((item) => item.id));
              }}
              defaultViewport={{ x: 24, y: 36, zoom: 1 }}
              minZoom={0.25}
              maxZoom={1.6}
              panOnScroll
              panOnDrag={interactionMode === 'hand'}
              nodesDraggable={canEditActiveWorkflow}
              nodesConnectable={canEditActiveWorkflow}
              edgesReconnectable={canEditActiveWorkflow}
              selectionOnDrag={canEditActiveWorkflow && interactionMode === 'pointer'}
            >
              <Background color="#e7edf5" gap={16} size={1} />
              <Controls position="bottom-left" showInteractive={false} />
              {showMiniMap ? <MiniMap position="bottom-right" pannable zoomable nodeStrokeWidth={2} /> : null}
            </ReactFlow>
            {canEditActiveWorkflow && nodes.length === 0 && edges.length === 0 ? (
              <div className="canvas-empty-guide">
                <i>+</i>
                <b>空画布</b>
                <span>从入口节点开始，或选择一个成熟模板生成 workflow draft。</span>
                <div>
                  <button type="button" onClick={() => addCatalogNode(getDef('event-in'))}>添加入口节点</button>
                  <button type="button" onClick={() => {
                    setShowTemplatePanel(true);
                    void refreshWorkflowTemplates();
                  }}>从模板开始</button>
                  <button type="button" onClick={() => importFileInputRef.current?.click()}>导入 JSON</button>
                </div>
                <em>{draftSaveState === 'saved' ? '空 draft 已保存，可以直接开始编排。' : '添加节点前也可以先保存空 draft。'}</em>
              </div>
            ) : null}
            <input
              ref={importFileInputRef}
              className="workflow-import-input"
              type="file"
              accept="application/json,.json"
              onChange={(event) => {
                void importWorkflowJSON(event.target.files?.[0]);
              }}
            />
            {alignmentGuide.vertical ? (
              <div className={cx('align-guide vertical', alignmentGuide.vertical.kind)} style={{ left: alignmentGuide.vertical.screen, top: alignmentGuide.vertical.start, height: alignmentGuide.vertical.end - alignmentGuide.vertical.start }} />
            ) : null}
            {alignmentGuide.horizontal ? (
              <div className={cx('align-guide horizontal', alignmentGuide.horizontal.kind)} style={{ top: alignmentGuide.horizontal.screen, left: alignmentGuide.horizontal.start, width: alignmentGuide.horizontal.end - alignmentGuide.horizontal.start }} />
            ) : null}
            {contextMenu ? (
              <ContextMenu
                menu={contextMenu}
                onOpenPicker={() => {
                  if (contextMenu.type !== 'pane') return;
                  setNodeSearch('');
                  setNodePicker({ x: contextMenu.x + 160, y: contextMenu.y - 8, flowX: contextMenu.flowX, flowY: contextMenu.flowY });
                }}
                onAddNote={addNoteAt}
                onPreview={() => setLastAdded('Preview')}
                onExport={() => setLastAdded('Export DSL')}
                onImport={() => setLastAdded('Import App')}
                onDuplicateNode={duplicateNode}
                onDeleteNodes={deleteNodes}
                onDeleteEdges={deleteEdges}
              />
            ) : null}
            {nodePicker ? <NodePicker picker={nodePicker} search={nodeSearch} filteredCatalog={filteredCatalog} onSearch={setNodeSearch} onAdd={addNodeAt} /> : null}
              </>
            ) : (
              <StudioActivityStart
                activities={visibleStudioActivities}
                workspaces={workspaces}
                folders={workspaceFolders}
                membersByWorkspace={workspaceMembers}
                shareLinksByWorkspace={workspaceShareLinks}
                auditLogsByWorkspace={workspaceAuditLogs}
                currentActorId={currentActorId}
                placements={activityPlacements}
                hiddenTestActivityCount={hiddenTestActivityCount}
                workflowStatus={workflowStatus}
                initialWorkspaceId={homeWorkspaceId}
                initialFolderId={homeFolderId}
                onSelectActivity={changeActivity}
                onCreateWorkspace={createStudioWorkspace}
                onCreateActivity={createStudioActivity}
                onCreateFolder={createWorkspaceFolder}
                onUpdateFolder={updateWorkspaceFolder}
                onCreateMember={createWorkspaceMember}
                onUpdateMemberRole={updateWorkspaceMemberRole}
                onRemoveMember={removeWorkspaceMember}
                onCreateShareLink={createWorkspaceShareLink}
                onDisableShareLink={disableWorkspaceShareLink}
                onRefreshWorkspaceOps={refreshWorkspaceOps}
              />
            )}
          </ReactFlowProvider>
        </section>
      </main>
      <RunHistoryPanel
        state={runHistory}
        workflowVersions={workflowVersions}
        onClose={() => {
          setRunHistory((state) => ({ ...state, open: false }));
          replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
        }}
        onRefresh={refreshRunStatus}
        onVersionChange={(versionId) => {
          void loadRunHistory(versionId);
        }}
        onSelectExecution={(executionId) => {
          void selectRunExecution(executionId);
        }}
        onCompareExecution={(executionId) => {
          void selectCompareExecution(executionId);
        }}
        onSelectNode={selectRunNode}
        onPageChange={(offset) => {
          void changeRunHistoryPage(offset);
        }}
        onExportTrace={exportSelectedTrace}
        onFocusEdge={focusRunEdge}
        onReplayStepChange={updateRunReplayStep}
      />
      <DebugRunPanel
        state={debugPanel}
        templates={debugPayloadTemplates}
        selectedExecutionId={runHistory.selectedExecutionId}
        selectedNodeId={selectedNodeIds[0] ?? ''}
        onChange={(patch) => setDebugPanel((state) => ({ ...state, ...patch }))}
        onClose={() => setDebugPanel((state) => ({ ...state, open: false }))}
        onLoadTemplate={loadDebugPayloadTemplate}
        onSaveTemplate={saveCurrentDebugPayloadTemplate}
        onUseSelectedExecutionPayload={useSelectedExecutionPayload}
        onRunDraft={() => {
          void runDraftDebug('');
        }}
        onRerun={() => {
          void rerunSelectedExecution();
        }}
        onContinueFromNode={() => {
          const startNodeId = debugPanel.startNodeId.trim() || selectedNodeIds[0] || '';
          void runDraftDebug(startNodeId);
        }}
        onExportTrace={() => {
          void exportSelectedTrace();
        }}
      />
      <WorkflowTemplatePanel
        open={showTemplatePanel}
        templates={workflowTemplates}
        nodePresets={workflowNodePresets}
        workspaces={workspaces}
        folders={workspaceFolders}
        activeWorkspaceId={activePlacement?.workspaceId}
        activeFolderId={activePlacement?.folderId}
        canEdit={canEditActiveWorkflow}
        saving={savingTemplate}
        message={templateMessage}
        templateVersions={workflowTemplateVersions}
        loadingVersions={loadingTemplateVersions}
        onClose={() => setShowTemplatePanel(false)}
        onApply={(templateId, input) => {
          void applyWorkflowTemplate(templateId, input);
        }}
        onSave={(input) => {
          void saveWorkflowTemplate(input);
        }}
        onLoadVersions={loadWorkflowTemplateVersions}
      />
      {showActivitySettings && activityId ? (() => {
        const activity = activities.find((item) => item.id === activityId);
        if (!activity) return null;
        return (
          <ActivitySettingsModal
            activity={activity}
            workspaces={workspaces}
            folders={workspaceFolders}
            placement={activityPlacements[activity.id]}
            saving={savingActivitySettings}
            error={activitySettingsError}
            onClose={() => setShowActivitySettings(false)}
            onSubmit={updateActivitySettings}
          />
        );
      })() : null}
      {activityId ? <PropertyPanel nodes={nodes} edges={edges} selectedNodeIds={selectedNodeIds} selectedEdgeIds={selectedEdgeIds} nodeConfigSchemas={nodeConfigSchemas} onNodeConfigChange={updateNodeConfig} onEdgeRouteChange={updateEdgeRoute} onLocateNode={(nodeId) => focusNodeById(nodeId, '已定位变量来源')} onDeleteNodes={deleteNodes} onDeleteEdges={deleteEdges} /> : null}
    </div>
  );
}
