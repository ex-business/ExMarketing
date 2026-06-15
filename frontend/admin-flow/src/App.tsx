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
import { ResourceManagement, WorkspaceContent, WorkspaceDetailPanel, WorkspaceHomeLayout, WorkspaceSidebar, WorkspaceTopBar } from './components/workspace-home';
import type { GlobalSearchResult, WorkspaceDetailTarget, WorkspaceItemAction, WorkspaceSortMode } from './components/workspace-home';
import { baseEdges, baseNodes, simpleTemplate } from './data/initialFlow';
import { catalogFromSchemas, getDef } from './data/nodeCatalog';
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
  WorkflowNodeSummary,
  ResourceAuditLogSummary,
  ResourceBindingSummary,
  ResourceSummary,
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
  triggerEvents?: string[];
  status?: string;
  visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  version?: number;
  createdAt?: string;
  updatedAt?: string;
};

type WorkspaceSummary = {
  id: string;
  tenantId: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
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
  expiresAt?: string;
  maxAccessCount?: number;
  accessCount?: number;
  lastAccessedAt?: string;
  disabledReason?: string;
  enabled: boolean;
  token?: string;
  createdBy?: string;
  createdAt?: string;
};

type WorkspaceBatchResult = {
  committed?: boolean;
  count?: number;
  items?: Array<{
    id: string;
    type: string;
    status: string;
    error?: string;
  }>;
};

type WorkspaceFileObjectSummary = {
  id: string;
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  workflowVersionId?: string;
  executionId?: string;
  kind: string;
  fileName: string;
  contentType?: string;
  sizeBytes: number;
  storageProvider: string;
  storageKey: string;
  checksumSha256: string;
  createdBy?: string;
  createdAt: string;
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

type WorkspaceActivityGrantSummary = {
  id: string;
  workspaceId: string;
  activityId: string;
  principalType: 'USER' | 'MEMBER' | 'ROLE';
  principalId: string;
  role: Exclude<WorkspaceRole, 'OWNER'>;
  createdBy?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
};

type WorkspaceHomeSearchResult = {
  id: string;
  type: 'workspace' | 'workflow' | 'folder' | 'member';
  title: string;
  subtitle: string;
  workspaceId?: string;
  folderId?: string;
  highlights?: {
    title?: string;
    subtitle?: string;
  };
  permission?: string;
  permissionReason?: string;
  permissionSources?: string[];
  score?: number;
};

type WorkspaceHomeStorageSummary = {
  usedBytes: number;
  totalBytes: number;
  workspaces: number;
  folders: number;
  workflows: number;
  members: number;
  shareLinks: number;
  auditLogs: number;
  drafts?: number;
  versions?: number;
  executions?: number;
  nodeExecutions?: number;
  activityGrants?: number;
  fileObjects?: number;
  fileBytes?: number;
};

type OpsToast = {
  id: number;
  message: string;
  tone: 'success' | 'error' | 'info';
};

function opsToastTone(message: string): OpsToast['tone'] {
  if (/(失败|错误|不可用|不能|需要|先选择|不存在|无权限)/.test(message)) return 'error';
  if (/(正在|处理中|加载)/.test(message)) return 'info';
  return 'success';
}

type WorkflowDraftTemplateSummary = {
  id: string;
  versionId: string;
  version: number;
  name: string;
  description?: string;
  category?: string;
  workspaceId?: string;
  folderId?: string;
  visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  tags?: string[];
  source: 'BUILTIN' | 'SAVED';
  status: 'ACTIVE' | 'ARCHIVED';
  definition: WorkflowDefinition;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

type WorkflowDraftTemplateCompareResult = {
  leftVersionId: string;
  rightVersionId: string;
  leftVersion: number;
  rightVersion: number;
  nodeDelta: number;
  edgeDelta: number;
  addedNodes: string[];
  removedNodes: string[];
  commonNodes: number;
  nodeChanges?: Array<{ nodeId: string; label?: string; type?: string; kind?: string; changes: WorkflowExecutionJSONDiff[] }>;
  edgeChanges?: Array<{ edgeId: string; source?: string; target?: string; label?: string; changes: WorkflowExecutionJSONDiff[] }>;
  changedMeta?: Record<string, unknown>;
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
  definition?: WorkflowDefinition;
  visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC';
};

type StudioFolderPatch = {
  parentId?: string;
  name?: string;
  visibility?: string;
  sortOrder?: number;
  status?: string;
};

type ActivitySettingsInput = {
  activityName: string;
  workspaceId: string;
  folderId: string;
};

type UnsavedDraftNavigation =
  | { kind: 'activity'; activityId: string }
  | { kind: 'workspace'; workspaceId: string; folderId: string };

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

const WORKSPACE_FILE_UPLOAD_MAX_BYTES = 64 * 1024 * 1024;

function studioActorId(): string {
  return window.localStorage.getItem('exmarketing.studio.actorId') || 'admin-flow';
}

function shouldSeedDemoWorkspace(): boolean {
  const params = new URLSearchParams(window.location.search);
  const queryValue = params.get('seedDemo');
  if (queryValue != null) {
    return ['1', 'true', 'yes'].includes(queryValue.trim().toLowerCase());
  }
  return String(import.meta.env.VITE_EXMARKETING_SEED_DEMO ?? '').trim().toLowerCase() === 'true';
}

function workspaceRoleAllows(role: WorkspaceRole, required: WorkspaceRole): boolean {
  return WORKSPACE_ROLE_RANK[role] >= WORKSPACE_ROLE_RANK[required];
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
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
  offlineTrace: boolean;
  versionId: string;
  executions: WorkflowExecution[];
  pageLimit: number;
  pageOffset: number;
  hasMore: boolean;
  nextOffset: number;
  selectedExecutionId: string;
  nodeExecutions: WorkflowNodeExecution[];
  nodeSummaries: WorkflowNodeSummary[];
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
  openApiLoading: boolean;
  template: string;
  payloadText: string;
  startNodeId: string;
  scheduleType: string;
  scheduleAt: string;
  scheduleTimezone: string;
  lastResult?: DebugRunResultSummary;
  message: string;
};

type DebugRunResultSummary = {
  mode: 'DEBUG_DRAFT' | 'OPENAPI_EVENT';
  status: string;
  title: string;
  executionId?: string;
  eventId?: string;
  idempotencyKey?: string;
  duplicate?: boolean;
  nodeTotal?: number;
  nodeSuccess?: number;
  nodeFailed?: number;
  nodeSkipped?: number;
  participations?: Array<Record<string, unknown>>;
  grantOrders?: Array<Record<string, unknown>>;
  outboxMessages?: Array<Record<string, unknown>>;
  ruleResults?: Array<Record<string, unknown>>;
  logs: string[];
};

type DebugResumePreview = {
  startNodeId: string;
  sourceExecutionId: string;
  upstreamNodes: Array<{ nodeId: string; title: string; outputKeys: string[] }>;
  expectedInputs: string[];
  missingInputs: string[];
};

type DebugPayloadTemplate = {
  id: string;
  name: string;
  payloadText: string;
  createdAt: string;
};

const DEBUG_PAYLOAD_TEMPLATE_STORAGE_KEY = 'exmarketing.studio.debugPayloadTemplates';
const RIGHT_PANEL_WIDTH_STORAGE_KEY = 'exmarketing.studio.rightPanelWidth';
const DEFAULT_RIGHT_PANEL_WIDTH = 432;
const MIN_RIGHT_PANEL_WIDTH = 340;
const MAX_RIGHT_PANEL_WIDTH = 760;

function clampRightPanelWidth(value: number): number {
  const viewportMax = typeof window === 'undefined' ? MAX_RIGHT_PANEL_WIDTH : Math.max(MIN_RIGHT_PANEL_WIDTH, Math.min(MAX_RIGHT_PANEL_WIDTH, window.innerWidth - 280));
  return Math.min(viewportMax, Math.max(MIN_RIGHT_PANEL_WIDTH, Math.round(value)));
}

function loadRightPanelWidth(): number {
  const parsed = Number(window.localStorage.getItem(RIGHT_PANEL_WIDTH_STORAGE_KEY));
  if (!Number.isFinite(parsed)) return DEFAULT_RIGHT_PANEL_WIDTH;
  return clampRightPanelWidth(parsed);
}

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

function sideEffectKindLabel(kind: string): string {
  const labels: Record<string, string> = {
    participation: '参与记录',
    grantOrders: '发奖订单',
    outboxMessages: 'Outbox',
    quotaReservations: '额度预留',
    rule: '规则结果',
    scope: '副作用范围',
  };
  return labels[kind] ?? kind;
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

type WorkflowErrorClass = {
  code: string;
  category: string;
  tone: 'neutral' | 'warning' | 'danger';
  suggestion: string;
};

const WORKFLOW_ERROR_DICTIONARY: Array<Omit<WorkflowErrorClass, 'code'> & { patterns: RegExp[] }> = [
  {
    category: '超时',
    tone: 'danger',
    suggestion: '检查节点超时配置、外部接口耗时和重试退避策略。',
    patterns: [/TIMEOUT/i, /DEADLINE/i],
  },
  {
    category: '配置错误',
    tone: 'danger',
    suggestion: '回到节点设置，检查必填配置、Schema 默认值和变量引用是否完整。',
    patterns: [/CONFIG/i, /MISSING/i, /REQUIRED/i, /SCHEMA/i],
  },
  {
    category: '路由错误',
    tone: 'danger',
    suggestion: '检查上一节点输出、连线路由表达式、默认分支和 handle 是否匹配。',
    patterns: [/ROUTE/i, /BRANCH/i, /CONDITION/i, /DEFAULT/i],
  },
  {
    category: '风控限制',
    tone: 'warning',
    suggestion: '确认资格、额度、幂等、限流或风险策略是否按预期拦截。',
    patterns: [/QUOTA/i, /LIMIT/i, /RISK/i, /QUALIFY/i],
  },
  {
    category: '外部依赖',
    tone: 'danger',
    suggestion: '检查 HTTP、Webhook、MQ、Kafka、Redis 或 Provider 资源是否可用。',
    patterns: [/HTTP/i, /WEBHOOK/i, /CALLBACK/i, /\bMQ\b/i, /KAFKA/i, /REDIS/i, /PROVIDER/i],
  },
  {
    category: '事件接入',
    tone: 'danger',
    suggestion: '检查 eventId、eventType、sourceSystem、tenantId 和幂等收件箱记录。',
    patterns: [/INBOX/i, /EVENT/i, /IDEMPOTEN/i],
  },
];

function classifyWorkflowError(message?: string): WorkflowErrorClass {
  const text = (message || '').trim();
  if (!text) return { code: 'NONE', category: '无错误', tone: 'neutral', suggestion: '当前 execution 或节点没有错误信息。' };
  const upper = text.toUpperCase();
  const codeMatch = upper.match(/\b([A-Z][A-Z0-9_]{2,})\b/);
  const code = codeMatch?.[1] ?? 'UNCLASSIFIED_ERROR';
  const matched = WORKFLOW_ERROR_DICTIONARY.find((item) => item.patterns.some((pattern) => pattern.test(text)));
  if (matched) return { code, category: matched.category, tone: matched.tone, suggestion: matched.suggestion };
  return { code, category: '业务执行错误', tone: 'danger', suggestion: '查看节点 input/output，确认业务 handler 返回值、异常和副作用写入是否符合预期。' };
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

const WORKFLOW_EXECUTION_STATUSES = new Set(['RUNNING', 'SUCCESS', 'FAILED', 'CANCELED']);
const WORKFLOW_NODE_EXECUTION_STATUSES = new Set(['RUNNING', 'SUCCESS', 'FAILED', 'SKIPPED']);

function validateWorkflowTrace(trace: WorkflowExecutionTrace): string {
  if (!trace || typeof trace !== 'object') return '文件不是 trace JSON 对象';
  if (!trace.execution || typeof trace.execution !== 'object') return '缺少 execution';
  if (!Array.isArray(trace.nodeExecutions)) return '缺少 nodeExecutions';
  const execution = trace.execution as Partial<WorkflowExecution>;
  if (!execution.id || typeof execution.id !== 'string') return 'execution 缺少 id';
  if (!execution.tenantId || typeof execution.tenantId !== 'string') return 'execution 缺少 tenantId';
  if (!execution.activityId || typeof execution.activityId !== 'string') return 'execution 缺少 activityId';
  if (!execution.workflowVersionId || typeof execution.workflowVersionId !== 'string') return 'execution 缺少 workflowVersionId';
  if (typeof execution.workflowVersion !== 'number' || !Number.isFinite(execution.workflowVersion)) return 'execution 缺少有效 workflowVersion';
  if (!execution.status || !WORKFLOW_EXECUTION_STATUSES.has(execution.status)) return 'execution 缺少有效 status';
  if (!execution.startedAt || typeof execution.startedAt !== 'string') return 'execution 缺少 startedAt';
  const invalidNode = trace.nodeExecutions.find((item) => (
    !item ||
    typeof item !== 'object' ||
    !item.id ||
    !item.executionId ||
    !item.nodeId ||
    !item.nodeType ||
    !item.status ||
    !WORKFLOW_NODE_EXECUTION_STATUSES.has(item.status) ||
    !item.startedAt
  ));
  if (invalidNode) return 'nodeExecutions 存在缺少 id/executionId/nodeId/nodeType/status/startedAt 的节点';
  return '';
}

function traceToRunState(trace: WorkflowExecutionTrace): Pick<RunHistoryState, 'executions' | 'selectedExecutionId' | 'nodeExecutions' | 'nodeSummaries' | 'selectedNodeExecutionId'> {
  const failed = trace.nodeExecutions.find((item) => item.status === 'FAILED');
  const selectedNode = failed ?? trace.nodeExecutions[0];
  return {
    executions: [trace.execution],
    selectedExecutionId: trace.execution.id,
    nodeExecutions: trace.nodeExecutions,
    nodeSummaries: trace.nodeSummaries ?? [],
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
  const eventType = activity?.triggerEvents?.[0] || 'DEPOSIT_SUCCESS';
  return JSON.stringify({
    eventId: `debug_evt_${Date.now()}`,
    eventType,
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

function uniqueFlowNodeId(base: string, nodes: FlowNode[]): string {
  const existing = new Set(nodes.map((node) => node.id));
  const safeBase = base.trim() || 'node';
  let candidate = `${safeBase}-${Date.now().toString(36)}`;
  let index = 1;
  while (existing.has(candidate)) {
    candidate = `${safeBase}-${Date.now().toString(36)}-${index}`;
    index += 1;
  }
  return candidate;
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

function workflowEventIdempotencyKey(event: unknown): string {
  if (!event || typeof event !== 'object') return '';
  const item = event as Record<string, unknown>;
  const biz = item.biz && typeof item.biz === 'object' ? item.biz as Record<string, unknown> : {};
  const parts = [item.tenantId, item.sourceSystem, item.eventType, biz.bizType, biz.bizId].map((part) => String(part ?? '').trim());
  return parts.every(Boolean) ? parts.join(':') : '';
}

function scheduleDebugPayloadTemplate(activity: ActivitySummary | undefined, input: { scheduleType: string; scheduleAt: string; timezone: string }): string {
  const now = new Date();
  const scheduledAt = input.scheduleAt.trim() || now.toISOString();
  const eventType = input.scheduleType.trim() || activity?.triggerEvents?.[0] || 'SCHEDULE_TICK';
  const tenantId = activity?.tenantId || 'site_001';
  const activityCode = activity?.activityCode || 'workflow';
  return JSON.stringify({
    eventId: `schedule_evt_${Date.now()}`,
    eventType,
    sourceSystem: 'scheduler',
    tenantId,
    subject: { type: 'SYSTEM', id: 'scheduler' },
    biz: { bizType: 'SCHEDULE', bizId: `${activityCode}:${scheduledAt}` },
    occurredAt: scheduledAt,
    attributes: {
      scheduleType: eventType,
      scheduleAt: scheduledAt,
      timezone: input.timezone.trim() || 'UTC',
      activityId: activity?.id ?? '',
      activityCode,
      simulated: true,
    },
  }, null, 2);
}

function asRecordArray(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item)) : [];
}

function compactDebugItem(item: Record<string, unknown>, keys: string[]): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  keys.forEach((key) => {
    if (item[key] !== undefined && item[key] !== null && item[key] !== '') out[key] = item[key];
  });
  return Object.keys(out).length > 0 ? out : item;
}

function objectKeys(value: unknown): string[] {
  return value && typeof value === 'object' && !Array.isArray(value) ? Object.keys(value as Record<string, unknown>).sort() : [];
}

function nodePayloadText(nodeExecution: WorkflowNodeExecution | undefined, source: 'input' | 'output'): string {
  const value = source === 'input' ? nodeExecution?.input : nodeExecution?.output;
  return value === undefined ? '' : JSON.stringify(value, null, 2);
}

function buildDebugResumePreview(input: {
  startNodeId: string;
  sourceExecutionId: string;
  nodeExecutions: WorkflowNodeExecution[];
  flowNodes: FlowNode[];
}): DebugResumePreview | undefined {
  const startNodeId = input.startNodeId.trim();
  if (!startNodeId || !input.sourceExecutionId) return undefined;
  const startIndex = input.nodeExecutions.findIndex((item) => item.nodeId === startNodeId);
  const upstream = input.nodeExecutions
    .filter((item, index) => item.status === 'SUCCESS' && item.output !== undefined && (startIndex < 0 || index < startIndex))
    .map((item) => ({
      nodeId: item.nodeId,
      title: item.nodeTitle || item.nodeId,
      outputKeys: objectKeys(item.output),
    }));
  const targetNode = input.flowNodes.find((node) => node.id === startNodeId && node.type === 'activity');
  const expectedInputs: string[] = targetNode?.type === 'activity' ? targetNode.data.inputs : [];
  const providedKeys = new Set(upstream.flatMap((item) => item.outputKeys));
  const missingInputs = expectedInputs.filter((key) => !providedKeys.has(key));
  return {
    startNodeId,
    sourceExecutionId: input.sourceExecutionId,
    upstreamNodes: upstream,
    expectedInputs,
    missingInputs,
  };
}

function debugTraceSummary(trace: WorkflowExecutionTrace): DebugRunResultSummary {
  const nodes = trace.nodeExecutions ?? [];
  const success = nodes.filter((item) => item.status === 'SUCCESS').length;
  const failed = nodes.filter((item) => item.status === 'FAILED').length;
  const skipped = nodes.filter((item) => item.status === 'SKIPPED').length;
  const output = trace.execution.output && typeof trace.execution.output === 'object' ? trace.execution.output as Record<string, unknown> : {};
  const logs = [
    `execution ${trace.execution.id}`,
    `status ${trace.execution.status}`,
    `nodes success=${success} failed=${failed} skipped=${skipped}`,
  ];
  if (trace.execution.errorMessage) logs.push(`error ${trace.execution.errorMessage}`);
  return {
    mode: 'DEBUG_DRAFT',
    status: trace.execution.status,
    title: 'Draft 调试结果',
    executionId: trace.execution.id,
    eventId: trace.execution.eventId,
    nodeTotal: nodes.length,
    nodeSuccess: success,
    nodeFailed: failed,
    nodeSkipped: skipped,
    participations: asRecordArray(output.participations).map((item) => compactDebugItem(item, ['id', 'activityId', 'status', 'idempotencyKey'])),
    grantOrders: asRecordArray(output.grantOrders).map((item) => compactDebugItem(item, ['id', 'billNo', 'rewardType', 'rewardAmount', 'grantStatus', 'grantChannel'])),
    outboxMessages: asRecordArray(output.outboxMessages).map((item) => compactDebugItem(item, ['id', 'messageType', 'targetChannel', 'targetRef', 'sendStatus', 'retryCount'])),
    ruleResults: asRecordArray(output.ruleResults).map((item) => compactDebugItem(item, ['activityId', 'activityCode', 'decision', 'failedStage', 'reasonCode', 'reason'])),
    logs,
  };
}

function openApiDebugSummary(response: unknown, idempotencyKey: string): DebugRunResultSummary {
  const body = response && typeof response === 'object' ? response as Record<string, unknown> : {};
  const inbox = body.inbox && typeof body.inbox === 'object' ? body.inbox as Record<string, unknown> : {};
  const event = inbox.event && typeof inbox.event === 'object' ? inbox.event as Record<string, unknown> : {};
  const participations = asRecordArray(body.participations).map((item) => compactDebugItem(item, ['id', 'activityId', 'status', 'idempotencyKey']));
  const grantOrders = asRecordArray(body.grantOrders).map((item) => compactDebugItem(item, ['id', 'billNo', 'rewardType', 'rewardAmount', 'grantStatus', 'grantChannel']));
  const outboxMessages = asRecordArray(body.outboxMessages).map((item) => compactDebugItem(item, ['id', 'messageType', 'targetChannel', 'targetRef', 'sendStatus', 'retryCount']));
  const ruleResults = asRecordArray(body.ruleResults).map((item) => compactDebugItem(item, ['activityId', 'activityCode', 'decision', 'failedStage', 'reasonCode', 'reason']));
  const duplicate = Boolean(body.duplicate);
  const logs = [
    `event ${String(event.eventId ?? body.eventId ?? '-')}`,
    `idempotency ${idempotencyKey}`,
    `duplicate ${duplicate}`,
    ...(duplicate ? ['rejected duplicate eventId'] : []),
    `participations=${participations.length} grants=${grantOrders.length} outbox=${outboxMessages.length}`,
  ];
  return {
    mode: 'OPENAPI_EVENT',
    status: duplicate ? 'REJECTED' : 'ACCEPTED',
    title: duplicate ? 'OpenAPI 拦截结果' : 'OpenAPI 真实调用结果',
    eventId: String(event.eventId ?? body.eventId ?? ''),
    idempotencyKey,
    duplicate,
    participations,
    grantOrders,
    outboxMessages,
    ruleResults,
    logs,
  };
}

function debugResultTone(status: string): string {
  if (['SUCCESS', 'ACCEPTED'].includes(status)) return 'success';
  if (['FAILED', 'ERROR', 'REJECTED'].includes(status)) return 'failed';
  if (status === 'DUPLICATE') return 'skipped';
  return 'running';
}

function DebugResultList({ title, items }: { title: string; items?: Array<Record<string, unknown>> }) {
  const rows = items ?? [];
  return (
    <div className="debug-result-list">
      <span>{title}</span>
      {rows.length ? rows.slice(0, 5).map((item, index) => (
        <code key={`${title}-${index}`}>{JSON.stringify(item)}</code>
      )) : <em>0 条</em>}
      {rows.length > 5 ? <em>仅显示前 5 条 / 共 {rows.length} 条</em> : null}
    </div>
  );
}

function DebugRunResult({ result }: { result?: DebugRunResultSummary }) {
  if (!result) return null;
  return (
    <div className={cx('debug-result-card', debugResultTone(result.status))}>
      <div className="debug-result-head">
        <span>
          <b>{result.title}</b>
          <em>{result.mode} · {result.status}</em>
        </span>
        <strong>{result.duplicate ? 'DUP' : result.nodeTotal !== undefined ? `${result.nodeSuccess ?? 0}/${result.nodeTotal}` : `${result.participations?.length ?? 0}P`}</strong>
      </div>
      <div className="debug-result-metrics">
        {result.executionId ? <div><span>Execution</span><b>{result.executionId}</b></div> : null}
        {result.eventId ? <div><span>Event</span><b>{result.eventId}</b></div> : null}
        {result.idempotencyKey ? <div><span>Idempotency</span><b>{result.idempotencyKey}</b></div> : null}
        {result.nodeTotal !== undefined ? <div><span>Nodes</span><b>{result.nodeSuccess ?? 0} 成功 / {result.nodeFailed ?? 0} 失败 / {result.nodeSkipped ?? 0} 跳过</b></div> : null}
      </div>
      <DebugResultList title="Participations" items={result.participations} />
      <DebugResultList title="Grant Orders" items={result.grantOrders} />
      <DebugResultList title="Outbox Messages" items={result.outboxMessages} />
      <DebugResultList title="Rule Results" items={result.ruleResults} />
      <div className="debug-log-list">
        <span>Logs</span>
        {result.logs.map((line) => <code key={line}>{line}</code>)}
      </div>
    </div>
  );
}

function DebugRunPanel({
  state,
  templates,
  selectedExecutionId,
  selectedNodeId,
  selectedNodeExecution,
  resumePreview,
  onResizeStart,
  onChange,
  onClose,
  onLoadTemplate,
  onSaveTemplate,
  onSaveNodePayloadTemplate,
  onUseNodePayload,
  onUseSelectedExecutionPayload,
  onRunDraft,
  onRunOpenApi,
  onBuildSchedulePayload,
  onRunScheduleDraft,
  onRerun,
  onContinueFromNode,
  onExportTrace,
}: {
  state: DebugPanelState;
  templates: DebugPayloadTemplate[];
  selectedExecutionId: string;
  selectedNodeId: string;
  selectedNodeExecution?: WorkflowNodeExecution;
  resumePreview?: DebugResumePreview;
  onResizeStart: (event: React.PointerEvent<HTMLButtonElement>) => void;
  onChange: (patch: Partial<DebugPanelState>) => void;
  onClose: () => void;
  onLoadTemplate: (templateId: string) => void;
  onSaveTemplate: () => void;
  onSaveNodePayloadTemplate: (source: 'input' | 'output') => void;
  onUseNodePayload: (source: 'input' | 'output') => void;
  onUseSelectedExecutionPayload: () => void;
  onRunDraft: () => void;
  onRunOpenApi: () => void;
  onBuildSchedulePayload: () => void;
  onRunScheduleDraft: () => void;
  onRerun: () => void;
  onContinueFromNode: () => void;
  onExportTrace: () => void;
}) {
  if (!state.open) return null;
  return (
    <aside className="debug-run-panel">
      <button className="right-panel-resize-handle" type="button" aria-label="调整调试面板宽度" onPointerDown={onResizeStart} />
      <header>
        <div>
          <b>调试运行</b>
          <span>默认 Draft 沙箱运行，OpenAPI 会写真实业务数据</span>
        </div>
        <button onClick={onClose}>×</button>
      </header>
      <section>
        <div className="debug-mode-cards">
          <div className="active">
            <b>Draft 沙箱</b>
            <span>写 debug execution，不写真实 inbox。</span>
          </div>
          <div>
            <b>OpenAPI 真实调用</b>
            <span>写 inbox、participation、grant order、outbox。</span>
          </div>
        </div>
        <div className="debug-payload-card">
          <div className="debug-field-grid">
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
          </div>
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
          <div className="debug-inline-actions node-payload-actions">
            <button type="button" onClick={() => onUseNodePayload('input')} disabled={selectedNodeExecution?.input === undefined}>使用节点 Input</button>
            <button type="button" onClick={() => onUseNodePayload('output')} disabled={selectedNodeExecution?.output === undefined}>使用节点 Output</button>
            <button type="button" onClick={() => onSaveNodePayloadTemplate('input')} disabled={selectedNodeExecution?.input === undefined}>Input 存模板</button>
            <button type="button" onClick={() => onSaveNodePayloadTemplate('output')} disabled={selectedNodeExecution?.output === undefined}>Output 存模板</button>
          </div>
        </div>
        <details className="debug-disclosure">
          <summary>
            <span>主动调用 OpenAPI</span>
            <em>真实写入</em>
          </summary>
          <div className="debug-callout real">
            <span>调用 `/openapi/v1/events`，会写入真实 inbox、participation、grant order 和 outbox。</span>
            <button type="button" onClick={onRunOpenApi} disabled={state.openApiLoading || state.loading || !state.payloadText.trim()}>
              {state.openApiLoading ? '调用中' : '调用 OpenAPI'}
            </button>
          </div>
        </details>
        <details className="debug-disclosure">
          <summary>
            <span>定时模拟参数</span>
            <em>生成 scheduler payload</em>
          </summary>
          <div className="debug-schedule-box">
          <label>
            <span>事件类型</span>
            <input value={state.scheduleType} placeholder="SCHEDULE_TICK" onChange={(event) => onChange({ scheduleType: event.target.value })} />
          </label>
          <label>
            <span>触发时间</span>
            <input value={state.scheduleAt} placeholder={new Date().toISOString()} onChange={(event) => onChange({ scheduleAt: event.target.value })} />
          </label>
          <label>
            <span>时区</span>
            <input value={state.scheduleTimezone} placeholder="UTC" onChange={(event) => onChange({ scheduleTimezone: event.target.value })} />
          </label>
          <div className="debug-inline-actions">
            <button type="button" onClick={onBuildSchedulePayload}>生成定时 Payload</button>
            <button type="button" onClick={onRunScheduleDraft} disabled={state.loading}>模拟定时跑 Draft</button>
          </div>
          </div>
        </details>
        <div className="debug-resume-card">
          <label>
            <span>从节点继续跑</span>
            <input value={state.startNodeId} placeholder={selectedNodeId || '可选：node id'} onChange={(event) => onChange({ startNodeId: event.target.value })} />
          </label>
          <span>复用选中 execution 的上游成功节点输出。</span>
          {resumePreview ? (
            <div className="debug-resume-preview">
              <div>
                <span>来源 execution</span>
                <b>{resumePreview.sourceExecutionId}</b>
              </div>
              <div>
                <span>起点节点</span>
                <b>{resumePreview.startNodeId}</b>
              </div>
              <div>
                <span>上游 output</span>
                <b>{resumePreview.upstreamNodes.length} 个成功节点</b>
              </div>
              {resumePreview.upstreamNodes.length ? (
                <ul>
                  {resumePreview.upstreamNodes.slice(0, 4).map((item) => (
                    <li key={item.nodeId}>
                      <b>{item.title}</b>
                      <em>{item.outputKeys.length ? item.outputKeys.join(', ') : '无 output key'}</em>
                    </li>
                  ))}
                </ul>
              ) : <em>没有可复用的上游成功 output。</em>}
              <div>
                <span>输入缺口</span>
                <b>{resumePreview.missingInputs.length ? resumePreview.missingInputs.join(', ') : '无明显缺口'}</b>
              </div>
            </div>
          ) : null}
        </div>
        <DebugRunResult result={state.lastResult} />
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
  onResizeStart,
  onClose,
  onApply,
  onSave,
  onUpdate,
  onCopy,
  onCompare,
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
  onResizeStart: (event: React.PointerEvent<HTMLButtonElement>) => void;
  onClose: () => void;
  onApply: (templateId: string, input: { versionId?: string; version?: number; parameters: Record<string, string | number> }) => void;
  onSave: (input: { name: string; workspaceId: string; folderId: string; visibility: string }) => void;
  onUpdate: (templateId: string, input: { versionId?: string; name?: string; description?: string; category?: string; workspaceId?: string; folderId?: string; visibility?: string; tags?: string[]; status?: string }) => void;
  onCopy: (templateId: string, input: { versionId?: string; name: string; category?: string; workspaceId: string; folderId?: string; visibility: string; tags?: string[] }) => Promise<WorkflowDraftTemplateSummary>;
  onCompare: (templateId: string, leftVersionId: string, rightVersionId: string) => Promise<WorkflowDraftTemplateCompareResult>;
  onLoadVersions: (templateId: string) => void;
}) {
  const [saveName, setSaveName] = useState('');
  const [saveWorkspaceId, setSaveWorkspaceId] = useState(activeWorkspaceId || workspaces[0]?.id || '');
  const [saveFolderId, setSaveFolderId] = useState(activeFolderId || '');
  const [saveVisibility, setSaveVisibility] = useState('TEAM');
  const [selectedTemplateId, setSelectedTemplateId] = useState('');
  const [selectedVersionId, setSelectedVersionId] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('ALL');
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'ARCHIVED' | 'ALL'>('ACTIVE');
  const [sourceFilter, setSourceFilter] = useState<'ALL' | 'BUILTIN' | 'SAVED'>('ALL');
  const [templateSearch, setTemplateSearch] = useState('');
  const [manageName, setManageName] = useState('');
  const [manageDescription, setManageDescription] = useState('');
  const [manageCategory, setManageCategory] = useState('');
  const [manageWorkspaceId, setManageWorkspaceId] = useState('');
  const [manageFolderId, setManageFolderId] = useState('');
  const [manageVisibility, setManageVisibility] = useState('TEAM');
  const [manageTags, setManageTags] = useState('');
  const [copyName, setCopyName] = useState('');
  const [copyWorkspaceId, setCopyWorkspaceId] = useState('');
  const [copyFolderId, setCopyFolderId] = useState('');
  const [copyVisibility, setCopyVisibility] = useState('TEAM');
  const [compareLeftId, setCompareLeftId] = useState('');
  const [compareRightId, setCompareRightId] = useState('');
  const [compareResult, setCompareResult] = useState<WorkflowDraftTemplateCompareResult | null>(null);
  const [compareBusy, setCompareBusy] = useState(false);
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
  const categories = Array.from(new Set(templates.map((template) => template.category || template.source))).sort();
  const filteredTemplates = templates.filter((template) => {
    const keyword = templateSearch.trim().toLowerCase();
    if (statusFilter !== 'ALL' && template.status !== statusFilter) return false;
    if (sourceFilter !== 'ALL' && template.source !== sourceFilter) return false;
    if (categoryFilter !== 'ALL' && (template.category || template.source) !== categoryFilter) return false;
    if (!keyword) return true;
    return `${template.name} ${template.description ?? ''} ${template.id} ${template.category ?? ''} ${template.tags?.join(' ') ?? ''}`.toLowerCase().includes(keyword);
  });
  const grouped = filteredTemplates.reduce<Record<string, WorkflowDraftTemplateSummary[]>>((acc, item) => {
    const key = item.category || item.source;
    acc[key] = [...(acc[key] ?? []), item];
    return acc;
  }, {});
  const selectedTemplate = templates.find((template) => template.id === selectedTemplateId) ?? filteredTemplates[0] ?? templates[0];
  const versions = templateVersions.filter((template) => template.id === selectedTemplate?.id);
  const selectedVersion = versions.find((template) => template.versionId === selectedVersionId) ?? selectedTemplate;
  const selectedValidation = selectedVersion ? workflowTemplateValidation(selectedVersion) : undefined;
  const selectedWorkspaceFolders = folders.filter((folder) => folder.workspaceId === saveWorkspaceId && folder.status !== 'DELETED');
  const manageWorkspaceFolders = folders.filter((folder) => folder.workspaceId === manageWorkspaceId && folder.status !== 'DELETED');
  const copyWorkspaceFolders = folders.filter((folder) => folder.workspaceId === copyWorkspaceId && folder.status !== 'DELETED');
  useEffect(() => {
    if (!selectedVersion || !open) return;
    setManageName(selectedVersion.name);
    setManageDescription(selectedVersion.description || '');
    setManageCategory(selectedVersion.category || '');
    setManageWorkspaceId(selectedVersion.workspaceId || activeWorkspaceId || workspaces[0]?.id || '');
    setManageFolderId(selectedVersion.folderId || '');
    setManageVisibility(selectedVersion.visibility || 'TEAM');
    setManageTags((selectedVersion.tags || []).join(', '));
    setCopyName(`${selectedVersion.name} Copy`);
    setCopyWorkspaceId(activeWorkspaceId || selectedVersion.workspaceId || workspaces[0]?.id || '');
    setCopyFolderId(activeFolderId || selectedVersion.folderId || '');
    setCopyVisibility(selectedVersion.visibility || 'TEAM');
    setCompareResult(null);
  }, [activeFolderId, activeWorkspaceId, open, selectedVersion?.versionId, workspaces]);
  useEffect(() => {
    if (!versions.length) return;
    setCompareLeftId(versions[versions.length - 1]?.versionId || versions[0].versionId);
    setCompareRightId(versions[0].versionId);
  }, [versions]);
  const parseTags = (value: string) => value.split(',').map((tag) => tag.trim()).filter(Boolean);
  const runCompare = async () => {
    if (!selectedTemplate || !compareLeftId || !compareRightId) return;
    setCompareBusy(true);
    try {
      setCompareResult(await onCompare(selectedTemplate.id, compareLeftId, compareRightId));
    } finally {
      setCompareBusy(false);
    }
  };
  if (!open) return null;
  return (
    <aside className="workflow-template-panel">
      <button className="right-panel-resize-handle" type="button" aria-label="调整模板面板宽度" onPointerDown={onResizeStart} />
      <header>
        <span>
          <b>Workflow Templates</b>
          <em>{templates.length} templates · {nodePresets.length} presets</em>
        </span>
        <button onClick={onClose}>×</button>
      </header>
      <div className="workflow-template-scroll">
        <section className="workflow-template-manager-bar">
          <label>
            <span>搜索</span>
            <input value={templateSearch} onChange={(event) => setTemplateSearch(event.target.value)} placeholder="模板名称、分类、标签" />
          </label>
          <label>
            <span>分类</span>
            <select value={categoryFilter} onChange={(event) => setCategoryFilter(event.target.value)}>
              <option value="ALL">全部分类</option>
              {categories.map((category) => <option key={category} value={category}>{category}</option>)}
            </select>
          </label>
          <label>
            <span>来源</span>
            <select data-testid="template-source-filter" value={sourceFilter} onChange={(event) => setSourceFilter(event.target.value as typeof sourceFilter)}>
              <option value="ALL">全部来源</option>
              <option value="BUILTIN">Builtin</option>
              <option value="SAVED">Saved</option>
            </select>
          </label>
          <label>
            <span>状态</span>
            <select data-testid="template-status-filter" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as typeof statusFilter)}>
              <option value="ACTIVE">Active</option>
              <option value="ARCHIVED">Archived</option>
              <option value="ALL">全部状态</option>
            </select>
          </label>
        </section>
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
                      data-testid="template-catalog-item"
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
            <h3>模板预览 / 管理</h3>
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
                <div className="workflow-template-manage-grid">
                  <label>
                    <span>名称</span>
                    <input data-testid="template-manage-name" value={manageName} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageName(event.target.value)} />
                  </label>
                  <label>
                    <span>分类</span>
                    <input value={manageCategory} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageCategory(event.target.value)} />
                  </label>
                  <label className="wide">
                    <span>描述</span>
                    <input value={manageDescription} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageDescription(event.target.value)} />
                  </label>
                  <label>
                    <span>权限</span>
                    <select value={manageVisibility} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageVisibility(event.target.value)}>
                      <option value="PRIVATE">Private</option>
                      <option value="TEAM">Team</option>
                      <option value="PUBLIC">Public</option>
                    </select>
                  </label>
                  <label>
                    <span>Workspace</span>
                    <select value={manageWorkspaceId} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => {
                      const workspaceId = event.target.value;
                      setManageWorkspaceId(workspaceId);
                      setManageFolderId(folders.find((folder) => folder.workspaceId === workspaceId && folder.status !== 'DELETED')?.id ?? '');
                    }}>
                      <option value="">无 workspace</option>
                      {workspaces.filter((workspace) => workspace.status !== 'DELETED').map((workspace) => (
                        <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <span>Folder</span>
                    <select value={manageFolderId} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageFolderId(event.target.value)}>
                      <option value="">Workspace Root</option>
                      {manageWorkspaceFolders.map((folder) => (
                        <option key={folder.id} value={folder.id}>{folder.name}</option>
                      ))}
                    </select>
                  </label>
                  <label className="wide">
                    <span>Tags</span>
                    <input value={manageTags} disabled={selectedVersion.source === 'BUILTIN'} onChange={(event) => setManageTags(event.target.value)} placeholder="risk, reward, audit" />
                  </label>
                  <div className="workflow-template-actions">
                    <button
                      data-testid="template-save-metadata"
                      disabled={selectedVersion.source === 'BUILTIN'}
                      onClick={() => selectedTemplate && onUpdate(selectedTemplate.id, {
                        versionId: selectedVersion.versionId,
                        name: manageName,
                        description: manageDescription,
                        category: manageCategory,
                        workspaceId: manageWorkspaceId,
                        folderId: manageFolderId,
                        visibility: manageVisibility,
                        tags: parseTags(manageTags),
                      })}
                    >
                      保存元数据
                    </button>
                    <button
                      data-testid="template-archive-toggle"
                      disabled={selectedVersion.source === 'BUILTIN'}
                      onClick={() => selectedTemplate && onUpdate(selectedTemplate.id, {
                        versionId: selectedVersion.versionId,
                        status: selectedVersion.status === 'ARCHIVED' ? 'ACTIVE' : 'ARCHIVED',
                      })}
                    >
                      {selectedVersion.status === 'ARCHIVED' ? '恢复模板' : '归档模板'}
                    </button>
                  </div>
                </div>
                <div className="workflow-template-copy">
                  <h3>复制到 Workspace</h3>
                  <label>
                    <span>新名称</span>
                    <input data-testid="template-copy-name" value={copyName} onChange={(event) => setCopyName(event.target.value)} />
                  </label>
                  <label>
                    <span>Workspace</span>
                    <select value={copyWorkspaceId} onChange={(event) => {
                      const workspaceId = event.target.value;
                      setCopyWorkspaceId(workspaceId);
                      setCopyFolderId(folders.find((folder) => folder.workspaceId === workspaceId && folder.status !== 'DELETED')?.id ?? '');
                    }}>
                      {workspaces.filter((workspace) => workspace.status !== 'DELETED').map((workspace) => (
                        <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <span>Folder</span>
                    <select value={copyFolderId} onChange={(event) => setCopyFolderId(event.target.value)}>
                      <option value="">Workspace Root</option>
                      {copyWorkspaceFolders.map((folder) => (
                        <option key={folder.id} value={folder.id}>{folder.name}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <span>权限</span>
                    <select value={copyVisibility} onChange={(event) => setCopyVisibility(event.target.value)}>
                      <option value="PRIVATE">Private</option>
                      <option value="TEAM">Team</option>
                      <option value="PUBLIC">Public</option>
                    </select>
                  </label>
                  <button data-testid="template-copy-submit" disabled={!copyWorkspaceId || !copyName.trim()} onClick={() => selectedTemplate && void onCopy(selectedTemplate.id, {
                    versionId: selectedVersion.versionId,
                    name: copyName,
                    category: manageCategory || selectedVersion.category,
                    workspaceId: copyWorkspaceId,
                    folderId: copyFolderId,
                    visibility: copyVisibility,
                    tags: parseTags(manageTags),
                  })}>复制模板</button>
                </div>
                <div className="workflow-template-compare">
                  <h3>版本对比</h3>
                  <label>
                    <span>左版本</span>
                    <select value={compareLeftId} onChange={(event) => setCompareLeftId(event.target.value)}>
                      {(versions.length ? versions : [selectedVersion]).map((version) => (
                        <option key={version.versionId} value={version.versionId}>v{version.version}</option>
                      ))}
                    </select>
                  </label>
                  <label>
                    <span>右版本</span>
                    <select value={compareRightId} onChange={(event) => setCompareRightId(event.target.value)}>
                      {(versions.length ? versions : [selectedVersion]).map((version) => (
                        <option key={version.versionId} value={version.versionId}>v{version.version}</option>
                      ))}
                    </select>
                  </label>
                  <button data-testid="template-compare-submit" disabled={compareBusy || !compareLeftId || !compareRightId} onClick={() => void runCompare()}>{compareBusy ? '对比中...' : '对比版本'}</button>
                  {compareResult ? (
                    <div className="workflow-template-diff" data-testid="template-compare-result">
                      <b>v{compareResult.leftVersion} → v{compareResult.rightVersion}</b>
                      <span>Nodes {compareResult.nodeDelta >= 0 ? '+' : ''}{compareResult.nodeDelta} · Edges {compareResult.edgeDelta >= 0 ? '+' : ''}{compareResult.edgeDelta} · Common {compareResult.commonNodes}</span>
                      <em>Added: {(compareResult.addedNodes ?? []).join(', ') || '-'}</em>
                      <em>Removed: {(compareResult.removedNodes ?? []).join(', ') || '-'}</em>
                      <em>Changed nodes: {(compareResult.nodeChanges ?? []).map((item) => item.label || item.nodeId).slice(0, 4).join(', ') || '-'}</em>
                      <em>Changed edges: {(compareResult.edgeChanges ?? []).map((item) => item.label || item.edgeId).slice(0, 4).join(', ') || '-'}</em>
                    </div>
                  ) : null}
                </div>
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
          <input data-testid="template-save-name" value={saveName} placeholder="模板名称" onChange={(event) => setSaveName(event.target.value)} />
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
          <button data-testid="template-save-submit" disabled={!canEdit || saving} onClick={() => onSave({ name: saveName, workspaceId: saveWorkspaceId, folderId: saveFolderId, visibility: saveVisibility })}>
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
  const nodeStatusChanges = compareResult?.nodeStatusChanges ?? [];
  const sideEffectChanges = compareResult?.sideEffectChanges ?? [];
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
        <div className={cx(nodeStatusChanges.length > 0 && 'changed')}>
          <b>节点状态</b>
          <span>{nodeStatusChanges.length ? `${nodeStatusChanges.length} 个节点变化` : '一致'}</span>
          <em>{nodeStatusChanges.slice(0, 3).map((item) => item.label).join(', ') || 'status/error/retry 未变化'}</em>
        </div>
        <div className={cx(sideEffectChanges.length > 0 && 'changed')}>
          <b>业务副作用</b>
          <span>{sideEffectChanges.length ? `${sideEffectChanges.length} 类变化` : '一致'}</span>
          <em>{sideEffectChanges.slice(0, 3).map((item) => sideEffectKindLabel(item.kind)).join(', ') || 'participation/grant/outbox 未变化'}</em>
        </div>
      </div>
      {nodeStatusChanges.length ? (
        <div className="run-compare-payload-diff">
          <h4>Node Status Diff</h4>
          {nodeStatusChanges.slice(0, 12).map((item) => (
            <section key={item.nodeId} className="expanded">
              <button type="button">
                <span>{item.label}</span>
                <em>{item.baselineStatus || '-'} → {item.currentStatus || '-'}</em>
              </button>
              <div className="run-json-diff-rows">
                <div className="changed">
                  <b>Status</b>
                  <code>{item.nodeId}</code>
                  <span>{item.baselineAttempt !== item.currentAttempt ? `attempt ${item.baselineAttempt || 0} → ${item.currentAttempt || 0}` : 'changed'}</span>
                  <em>current: {item.currentErrorCode || item.currentError || compactDuration(item.currentDurationMs ?? 0)}</em>
                  <em>baseline: {item.baselineErrorCode || item.baselineError || compactDuration(item.baselineDurationMs ?? 0)}</em>
                </div>
              </div>
            </section>
          ))}
        </div>
      ) : null}
      {sideEffectChanges.length ? (
        <div className="run-compare-payload-diff">
          <h4>Side Effect Diff</h4>
          {sideEffectChanges.map((item) => (
            <section key={item.kind} className="expanded">
              <button type="button">
                <span>{sideEffectKindLabel(item.kind)}</span>
                <em>{item.changes.length} changes</em>
              </button>
              <div className="run-json-diff-rows">
                {item.changes.slice(0, 12).map((row, index) => (
                  <div key={`${item.kind}-${row.path}-${index}`} className={row.type}>
                    <b>{row.type}</b>
                    <code>{row.path}</code>
                    <span>{item.kind}</span>
                    <em>current: {jsonValueLabel(row.current)}</em>
                    <em>baseline: {jsonValueLabel(row.baseline)}</em>
                  </div>
                ))}
                {item.changes.length > 12 ? <p>仅显示前 12 条差异</p> : null}
              </div>
            </section>
          ))}
        </div>
      ) : null}
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
  onResizeStart,
  onClose,
  onVersionChange,
  onSelectExecution,
  onCompareExecution,
  onSelectNode,
  onPageChange,
  onExportTrace,
  onImportTrace,
  onFocusEdge,
  onReplayStepChange,
}: {
  state: RunHistoryState;
  workflowVersions: WorkflowVersion[];
  onResizeStart: (event: React.PointerEvent<HTMLButtonElement>) => void;
  onClose: () => void;
  onVersionChange: (versionId: string) => void;
  onSelectExecution: (executionId: string) => void;
  onCompareExecution: (executionId: string) => void;
  onSelectNode: (nodeExecution: WorkflowNodeExecution) => void;
  onPageChange: (offset: number) => void;
  onExportTrace: () => void;
  onImportTrace: (file: File) => void;
  onFocusEdge: (edgeId: string) => void;
  onReplayStepChange: (step: number) => void;
}) {
  const [payloadQuery, setPayloadQuery] = useState('');
  const [collapsedPayloads, setCollapsedPayloads] = useState({ input: false, output: false });
  const [shareLinkStatus, setShareLinkStatus] = useState('');
  const traceInputRef = useRef<HTMLInputElement | null>(null);
  if (!state.open) return null;
  const selectedExecution = state.executions.find((item) => item.id === state.selectedExecutionId);
  const compareExecution = state.executions.find((item) => item.id === state.compareExecutionId);
  const selectedNode = state.nodeExecutions.find((item) => item.id === state.selectedNodeExecutionId) ?? state.nodeExecutions[0];
  const summaryByNodeId = new Map(state.nodeSummaries.map((item) => [item.nodeId, item]));
  const selectedNodeSummary = selectedNode ? summaryByNodeId.get(selectedNode.nodeId) : undefined;
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
      <button className="right-panel-resize-handle" type="button" aria-label="调整运行历史面板宽度" onPointerDown={onResizeStart} />
      <header>
        <div>
          <b>运行历史</b>
          <span>{shareLinkStatus || (state.offlineTrace ? '离线 Trace 查看' : selectedExecution ? `execution ${selectedExecution.id.slice(0, 12)}` : '选择一次执行')}</span>
        </div>
        <div className="run-history-header-actions">
          <input
            ref={traceInputRef}
            type="file"
            accept="application/json,.json"
            hidden
            onChange={(event) => {
              const file = event.currentTarget.files?.[0];
              if (file) onImportTrace(file);
              event.currentTarget.value = '';
            }}
          />
          <button type="button" className="share" onClick={() => traceInputRef.current?.click()}>导入 Trace</button>
          <button type="button" className="share" onClick={onExportTrace} disabled={!selectedExecution}>导出 Trace</button>
          <button type="button" className="share" onClick={() => void copyCurrentRunLink()} disabled={!selectedExecution}>复制定位链接</button>
          <button type="button" onClick={onClose}>×</button>
        </div>
      </header>
      <div className="run-history-filters">
        <select value={state.versionId} onChange={(event) => onVersionChange(event.target.value)} disabled={state.offlineTrace}>
          {state.offlineTrace ? (
            <option value="">离线 Trace</option>
          ) : (
            <>
              <option value="">全部版本</option>
              {workflowVersions.map((version) => (
                <option key={version.id} value={version.id}>workflow v{version.version} · {version.status}</option>
              ))}
            </>
          )}
        </select>
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
                <em>{summaryByNodeId.get(nodeExecution.nodeId)?.decision || (duration === undefined ? '运行中' : `${duration}ms`)}{attempts && attempts > 1 ? ` · retry ${attempts - 1}` : ''}</em>
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
                <small>{executionError.suggestion}</small>
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
                <em>{selectedNodeSummary?.explanation || selectedNode.errorMessage || `耗时 ${selectedDuration ?? 0}ms · retry ${Math.max((selectedAttempts ?? 1) - 1, 0)}`}</em>
                {selectedNodeSummary?.decision ? <i>decision · {selectedNodeSummary.decision}</i> : null}
                {selectedNodeSummary?.errorCode || selectedNode.errorMessage ? <i title={nodeError.suggestion}>{nodeError.category} · {selectedNodeSummary?.errorCode || nodeError.code}</i> : null}
              </div>
              {selectedNodeSummary ? (
                <div className="run-node-explain">
                  <div>
                    <span>输入</span>
                    <b>{selectedNodeSummary.inputKeys?.join(', ') || '-'}</b>
                  </div>
                  <div>
                    <span>输出</span>
                    <b>{selectedNodeSummary.outputKeys?.join(', ') || '-'}</b>
                  </div>
                  <div>
                    <span>耗时</span>
                    <b>{compactDuration(selectedNodeSummary.durationMs)}</b>
                  </div>
                </div>
              ) : null}
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
    visibility: input.visibility || 'TEAM',
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
  body?: Record<string, unknown>;

  constructor(message: string, status: number, statusText: string, body?: Record<string, unknown>) {
    super(message);
    this.name = 'APIError';
    this.status = status;
    this.statusText = statusText;
    this.body = body;
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

function workflowValidationFailureResult(message: string, code = 'WORKFLOW_VALIDATION_FAILED'): WorkflowValidationResult {
  return {
    valid: false,
    items: [{
      severity: 'BLOCK',
      code,
      message,
      path: 'workflow.validation',
    }],
    summary: { blockCount: 1, warningCount: 0 },
  };
}

function normalizeWorkflowValidationResult(result: WorkflowValidationResult | null | undefined): WorkflowValidationResult | null {
  if (!result || typeof result !== 'object') return null;
  const items = Array.isArray(result.items) ? result.items : [];
  const blockCount = Number.isFinite(result.summary?.blockCount) ? result.summary.blockCount : items.filter((item) => item.severity === 'BLOCK').length;
  const warningCount = Number.isFinite(result.summary?.warningCount) ? result.summary.warningCount : items.filter((item) => item.severity === 'WARNING').length;
  return {
    ...result,
    valid: Boolean(result.valid) && blockCount === 0,
    items,
    summary: { blockCount, warningCount },
  };
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
    throw new APIError(body.error ?? body.message ?? `${response.status} ${response.statusText}`, response.status, response.statusText, isRecord(body) ? body : undefined);
  }
  return response.json() as Promise<T>;
}

async function uploadWorkspaceFile(input: {
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  kind: string;
  file: File;
  onProgress?: (percent: number) => void;
}): Promise<WorkspaceFileObjectSummary> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    form.set('workspaceId', input.workspaceId);
    if (input.folderId) form.set('folderId', input.folderId);
    if (input.activityId) form.set('activityId', input.activityId);
    form.set('kind', input.kind);
    form.set('file', input.file);
    const request = new XMLHttpRequest();
    request.open('POST', '/admin/v1/workspace-files');
    request.setRequestHeader('X-Actor-Id', studioActorId());
    request.upload.onprogress = (event) => {
      if (event.lengthComputable) input.onProgress?.(Math.min(99, Math.round((event.loaded / event.total) * 100)));
    };
    request.onerror = () => reject(new Error('上传失败，请检查网络后重试'));
    request.onload = () => {
      const body = JSON.parse(request.responseText || '{}');
      if (request.status < 200 || request.status >= 300) {
        reject(new APIError(body.error ?? body.message ?? `${request.status} ${request.statusText}`, request.status, request.statusText));
        return;
      }
      input.onProgress?.(100);
      resolve(body as WorkspaceFileObjectSummary);
    };
    request.send(form);
  });
}

function StudioActivityStart({
  activities,
  workspaces,
  folders,
  membersByWorkspace,
  shareLinksByWorkspace,
  auditLogsByWorkspace,
  activityGrantsByActivity,
  templates,
  currentActorId,
  placements,
  hiddenTestActivityCount,
  workflowStatus,
  workspaceDataReady,
  initialWorkspaceId,
  initialFolderId,
  onSelectActivity,
  onCreateWorkspace,
  onCreateActivity,
  onCreateFolder,
  onUpdateFolder,
  onUpdateActivity,
  onDeleteFolder,
  onRestoreFolder,
  onUpdateWorkspace,
  onDeleteWorkspace,
  onCreateMember,
  onUpdateMemberRole,
  onRemoveMember,
  onCreateShareLink,
  onDisableShareLink,
  onRefreshActivityACL,
  onCreateActivityACL,
  onDeleteActivityACL,
  onRefreshWorkspaceOps,
}: {
  activities: ActivitySummary[];
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  membersByWorkspace: Record<string, WorkspaceMemberSummary[]>;
  shareLinksByWorkspace: Record<string, WorkspaceShareLinkSummary[]>;
  auditLogsByWorkspace: Record<string, WorkspaceAuditLogSummary[]>;
  activityGrantsByActivity: Record<string, WorkspaceActivityGrantSummary[]>;
  templates: WorkflowDraftTemplateSummary[];
  currentActorId: string;
  placements: Record<string, ActivityPlacement>;
  hiddenTestActivityCount: number;
  workflowStatus: string;
  workspaceDataReady: boolean;
  initialWorkspaceId: string;
  initialFolderId: string;
  onSelectActivity: (activityId: string) => void;
  onCreateWorkspace: (input: CreateStudioWorkspaceInput) => Promise<WorkspaceSummary>;
  onCreateActivity: (input: CreateStudioActivityInput) => Promise<ActivitySummary>;
  onCreateFolder: (workspaceId: string, name: string) => Promise<WorkspaceFolderSummary>;
  onUpdateFolder: (workspaceId: string, folderId: string, patch: StudioFolderPatch) => Promise<WorkspaceFolderSummary>;
  onUpdateActivity: (activityId: string, patch: { activityName?: string; workspaceId?: string; folderId?: string; status?: string; visibility?: string }) => Promise<ActivitySummary>;
  onDeleteFolder: (workspaceId: string, folderId: string) => Promise<WorkspaceFolderSummary>;
  onRestoreFolder: (workspaceId: string, folderId: string) => Promise<WorkspaceFolderSummary>;
  onUpdateWorkspace: (workspaceId: string, patch: { name?: string; slug?: string; description?: string; icon?: string; color?: string; visibility?: string; defaultRole?: WorkspaceRole; ownerId?: string; status?: string }) => Promise<WorkspaceSummary>;
  onDeleteWorkspace: (workspaceId: string) => Promise<WorkspaceSummary>;
  onCreateMember: (workspaceId: string, input: { userId: string; email: string; displayName: string; role: WorkspaceRole }) => Promise<void>;
  onUpdateMemberRole: (workspaceId: string, memberId: string, role: WorkspaceRole) => Promise<void>;
  onRemoveMember: (workspaceId: string, memberId: string) => Promise<void>;
  onCreateShareLink: (workspaceId: string, input: { scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY'; folderId?: string; activityId?: string; permission: 'VIEW' | 'EDIT' }) => Promise<WorkspaceShareLinkSummary>;
  onDisableShareLink: (workspaceId: string, linkId: string) => Promise<void>;
  onRefreshActivityACL: (activityId: string) => Promise<WorkspaceActivityGrantSummary[]>;
  onCreateActivityACL: (activityId: string, input: { principalType: 'USER' | 'MEMBER' | 'ROLE'; principalId: string; role: Exclude<WorkspaceRole, 'OWNER'> }) => Promise<WorkspaceActivityGrantSummary>;
  onDeleteActivityACL: (activityId: string, grantId: string) => Promise<void>;
  onRefreshWorkspaceOps: (workspaceId: string) => Promise<void>;
}) {
  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<'ACTIVE' | 'DRAFT' | 'ARCHIVED' | 'ALL'>('ALL');
  const [sortMode, setSortMode] = useState<WorkspaceSortMode>('recent');
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
  const [opsToasts, setOpsToasts] = useState<OpsToast[]>([]);
  const opsToastIdRef = useRef(0);
  const opsToastTimersRef = useRef<number[]>([]);
  const [serverSearchResults, setServerSearchResults] = useState<WorkspaceHomeSearchResult[]>([]);
  const [storageSummary, setStorageSummary] = useState<WorkspaceHomeStorageSummary | null>(null);
  const [workspaceFiles, setWorkspaceFiles] = useState<WorkspaceFileObjectSummary[]>([]);
  const [workspaceFilesLoading, setWorkspaceFilesLoading] = useState(false);
  const [workspaceFileUpload, setWorkspaceFileUpload] = useState<{ file?: File; progress: number; status: 'idle' | 'uploading' | 'failed' | 'done'; error: string }>({ progress: 0, status: 'idle', error: '' });
  const [selectedDetail, setSelectedDetail] = useState<{ id: string; type: 'Folder' | 'Workflow'; name: string; visibility: string; status?: string; version?: string; owner?: string } | null>(null);
  const [detailVisibilityOverride, setDetailVisibilityOverride] = useState<WorkspaceDetailTarget['visibility'] | ''>('');
  const [detailAction, setDetailAction] = useState<'rename' | 'move' | 'archive' | ''>('');
  const [detailActionName, setDetailActionName] = useState('');
  const [detailActionFolderId, setDetailActionFolderId] = useState('');
  const [detailActionBusy, setDetailActionBusy] = useState(false);
  const [detailActionError, setDetailActionError] = useState('');
  const [batchAction, setBatchAction] = useState<'move' | 'permission' | ''>('');
  const [batchActionItems, setBatchActionItems] = useState<Array<{ id: string; type: 'Folder' | 'Workflow'; name: string; visibility: string; status?: string; version?: string; owner?: string }>>([]);
  const [batchActionFolderId, setBatchActionFolderId] = useState('');
  const [batchActionVisibility, setBatchActionVisibility] = useState<WorkspaceDetailTarget['visibility']>('Workspace');
  const [batchActionBusy, setBatchActionBusy] = useState(false);
  const [batchActionError, setBatchActionError] = useState('');
  const [batchActionFailures, setBatchActionFailures] = useState<WorkspaceBatchResult['items']>([]);
  const [homeUtilityDialog, setHomeUtilityDialog] = useState<'settings' | 'storage' | 'members' | 'share' | 'audit' | 'resources' | ''>('');
  const [resources, setResources] = useState<ResourceSummary[]>([]);
  const [resourcesLoading, setResourcesLoading] = useState(false);
  const [selectedResourceCode, setSelectedResourceCode] = useState('');
  const [resourceBindings, setResourceBindings] = useState<ResourceBindingSummary[]>([]);
  const [resourceAudits, setResourceAudits] = useState<ResourceAuditLogSummary[]>([]);
  const [resourceMessage, setResourceMessage] = useState('');
  const [workspaceSettingsDraft, setWorkspaceSettingsDraft] = useState({
    name: '',
    slug: '',
    description: '',
    icon: '',
    color: '',
    visibility: 'TEAM',
    defaultRole: 'VIEWER' as WorkspaceRole,
    ownerId: '',
  });
  const demoSeedStartedRef = useRef(false);
  const importWorkflowInputRef = useRef<HTMLInputElement | null>(null);
  const storageFileInputRef = useRef<HTMLInputElement | null>(null);
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
    if (!shouldSeedDemoWorkspace() || !workspaceDataReady || demoSeedStartedRef.current || activeWorkspaces.length) return;
    demoSeedStartedRef.current = true;
    void (async () => {
      try {
        const workspace = await onCreateWorkspace({ name: 'hello', tenantId: 'site_001' });
        const root = await onCreateFolder(workspace.id, '风控活动');
        const childNames = ['高风险策略', '额度保护', '回调审计'];
        for (const name of childNames) {
          const folder = await onCreateFolder(workspace.id, name);
          await onUpdateFolder(workspace.id, folder.id, { parentId: root.id });
        }
        const workflowNames = [
          '资格校验链路',
          'Quota 保护奖励链路',
          'Fact Query 数据增强',
          '新用户奖励链路',
          '高风险策略',
          '活动匹配链路',
          'Reward 计算',
          'Limit 次数限制',
        ];
        for (const [index, name] of workflowNames.entries()) {
          await onCreateActivity({
            tenantId: workspace.tenantId,
            activityName: name,
            activityCode: `${normalizeActivityCode(name)}_${index + 1}`,
            workspaceId: workspace.id,
            folderId: root.id,
            visibility: index % 3 === 0 ? 'PRIVATE' : 'TEAM',
          });
        }
        setSelectedWorkspaceId(workspace.id);
        setSelectedFolderId(root.id);
        await onRefreshWorkspaceOps(workspace.id);
        setOpsMessage('已初始化本地 demo 工作台');
      } catch (error) {
        setOpsMessage(`初始化 demo 数据失败：${(error as Error).message}`);
      }
    })();
  }, [activeWorkspaces.length, activities.length, folders.length, onCreateActivity, onCreateFolder, onCreateWorkspace, onRefreshWorkspaceOps, onUpdateFolder, workspaceDataReady]);
  useEffect(() => {
    if (initialWorkspaceId) setSelectedWorkspaceId(initialWorkspaceId);
    if (initialFolderId) setSelectedFolderId(initialFolderId);
  }, [initialFolderId, initialWorkspaceId]);
  useEffect(() => {
    const closeWorkspaceDialogs = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setDetailAction('');
      setBatchAction('');
      setHomeUtilityDialog('');
      setShowWorkspaceDialog(false);
      setShowFolderDialog(false);
      setShowCreateDialog(false);
    };
    document.addEventListener('keydown', closeWorkspaceDialogs);
    return () => document.removeEventListener('keydown', closeWorkspaceDialogs);
  }, []);
  useEffect(() => {
    let cancelled = false;
    fetchJSON<WorkspaceHomeStorageSummary>('/admin/v1/workspace-home/storage')
      .then((summary) => {
        if (!cancelled) setStorageSummary(summary);
      })
      .catch(() => {
        if (!cancelled) setStorageSummary(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  useEffect(() => {
    const keyword = search.trim();
    if (!keyword) {
      setServerSearchResults([]);
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      fetchJSON<{ items: WorkspaceHomeSearchResult[] }>(`/admin/v1/workspace-home/search?q=${encodeURIComponent(keyword)}&limit=20`)
        .then((body) => {
          if (!cancelled) setServerSearchResults(body.items ?? []);
        })
        .catch(() => {
          if (!cancelled) setServerSearchResults([]);
        });
    }, 180);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [search]);
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
  const shareUrlForLink = (link?: WorkspaceShareLinkSummary) => {
    const token = link?.token || link?.id;
    return token ? `${window.location.origin}/share/${token}` : '';
  };
  const shareLinkDisplayId = (link: WorkspaceShareLinkSummary) => {
    const value = link.token || link.id;
    if (!value) return 'share link';
    const prefix = link.token ? 'tok' : 'id';
    if (value.length <= 10) return `${prefix}_${value}`;
    return `${prefix}_${value.slice(0, 4)}…${value.slice(-4)}`;
  };
  const shareLinkForTarget = (target: { id: string; type: 'Workspace' | 'Folder' | 'Workflow' }) => selectedShareLinks.find((link) => {
    if (!link.enabled) return false;
    if (target.type === 'Workspace') return link.scope === 'WORKSPACE' && link.workspaceId === target.id;
    if (target.type === 'Folder') return link.scope === 'FOLDER' && link.folderId === target.id;
    return link.scope === 'ACTIVITY' && link.activityId === target.id;
  });
  const selectedRole = workspaceRoleForActor(selectedWorkspace, selectedMembers, currentActorId);
  const canEditWorkspace = workspaceRoleAllows(selectedRole, 'EDITOR');
  const canManageFolders = workspaceRoleAllows(selectedRole, 'ADMIN');
  const canManageMembers = workspaceRoleAllows(selectedRole, 'OWNER');
  const permissionCapabilities = {
    role: selectedRole,
    canEditWorkspace,
    canManageFolders,
    canManageMembers,
    reason: `当前角色 ${selectedRole} 权限不足`,
  };
  const workspaceStatusError = workflowStatus.includes('失败') || workflowStatus.includes('不可用') || workflowStatus.includes('异常')
    ? workflowStatus
    : '';
  const selectedResource = resources.find((resource) => resource.code === selectedResourceCode) ?? resources[0];
  const dismissOpsToast = useCallback((id: number) => {
    setOpsToasts((items) => items.filter((item) => item.id !== id));
  }, []);
  useEffect(() => () => {
    opsToastTimersRef.current.forEach((timer) => window.clearTimeout(timer));
    opsToastTimersRef.current = [];
  }, []);
  useEffect(() => {
    const message = opsMessage.trim();
    if (!message) return;
    const id = Date.now() + opsToastIdRef.current++;
    const timer = window.setTimeout(() => {
      dismissOpsToast(id);
      opsToastTimersRef.current = opsToastTimersRef.current.filter((item) => item !== timer);
    }, 4000);
    opsToastTimersRef.current.push(timer);
    setOpsToasts((items) => [...items.slice(-3), { id, message, tone: opsToastTone(message) }]);
  }, [dismissOpsToast, opsMessage]);
  useEffect(() => {
    if (!selectedWorkspace) return;
    setWorkspaceSettingsDraft({
      name: selectedWorkspace.name || '',
      slug: selectedWorkspace.slug || '',
      description: selectedWorkspace.description || '',
      icon: selectedWorkspace.icon || '',
      color: selectedWorkspace.color || '',
      visibility: selectedWorkspace.visibility || 'TEAM',
      defaultRole: selectedWorkspace.defaultRole || 'VIEWER',
      ownerId: selectedWorkspace.ownerId || '',
    });
  }, [selectedWorkspace?.id, selectedWorkspace?.name, selectedWorkspace?.slug, selectedWorkspace?.description, selectedWorkspace?.icon, selectedWorkspace?.color, selectedWorkspace?.visibility, selectedWorkspace?.defaultRole, selectedWorkspace?.ownerId]);
  useEffect(() => {
    setDetailVisibilityOverride('');
  }, [selectedDetail?.id, selectedFolder?.id, selectedWorkspace?.id]);
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
  useEffect(() => {
    if (!selectedWorkspace || selectedFolderId) return;
    const defaultFolder = selectedWorkspaceFolders.find((folder) => folder.name === '风控活动') ?? selectedWorkspaceFolders[0];
    if (defaultFolder) setSelectedFolderId(defaultFolder.id);
  }, [selectedFolderId, selectedWorkspace, selectedWorkspaceFolders]);
  const selectedChildFolders = useMemo(() => {
    if (!selectedWorkspace) return [];
    const parentId = selectedFolder?.id ?? '';
    return selectedWorkspaceFolders.filter((folder) => (folder.parentId ?? '') === parentId);
  }, [selectedFolder?.id, selectedWorkspace, selectedWorkspaceFolders]);
  const selectedWorkflows = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return activities
      .filter((activity) => {
        const archived = archivedActivityIds.has(activity.id) || activity.status === 'ARCHIVED';
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
        return !archivedActivityIds.has(activity.id) && activity.status !== 'ARCHIVED';
      })
      .sort((a, b) => activityTimestamp(b) - activityTimestamp(a))
      .slice(0, 5)
  ), [activities, archivedActivityIds, placements, selectedWorkspace]);
  const sharedWorkflowCount = useMemo(() => (
    selectedWorkspace ? selectedShareLinks.filter((link) => link.enabled && link.scope === 'ACTIVITY').length : 0
  ), [selectedShareLinks, selectedWorkspace]);
  const archivedWorkflowCount = useMemo(() => (
    activities.filter((activity) => archivedActivityIds.has(activity.id) || activity.status === 'ARCHIVED').length
  ), [activities, archivedActivityIds]);
  const storageUsedGb = storageSummary ? storageSummary.usedBytes / 1024 / 1024 / 1024 : 32.4;
  const storageTotalGb = storageSummary ? storageSummary.totalBytes / 1024 / 1024 / 1024 : 100;
  const globalSearchResults = useMemo<GlobalSearchResult[]>(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return [];
    const workflowResults = activities
      .filter((activity) => `${activity.activityName} ${activity.activityCode} ${activity.tenantId}`.toLowerCase().includes(keyword))
      .slice(0, 6)
      .map((activity) => ({
        id: activity.id,
        type: 'workflow' as const,
        title: activity.activityName || activity.activityCode,
        subtitle: `${activity.activityCode} · ${activity.status || 'ACTIVE'}`,
        workspaceId: placements[activity.id]?.workspaceId,
        folderId: placements[activity.id]?.folderId,
      }));
    const folderResults = folders
      .filter((folder) => folder.name.toLowerCase().includes(keyword))
      .slice(0, 6)
      .map((folder) => ({
        id: folder.id,
        type: 'folder' as const,
        title: folder.name,
        subtitle: `${folder.visibility} · ${workflowCountByFolder[folder.id] ?? 0} workflows`,
        workspaceId: folder.workspaceId,
        folderId: folder.id,
      }));
    const memberResults = selectedMembers
      .filter((member) => `${member.displayName ?? ''} ${member.userId ?? ''} ${member.email ?? ''}`.toLowerCase().includes(keyword))
      .slice(0, 5)
      .map((member) => ({
        id: member.id,
        type: 'member' as const,
        title: member.displayName || member.userId || member.email || 'Member',
        subtitle: `${member.role} · ${member.status}`,
        workspaceId: member.workspaceId,
      }));
    return serverSearchResults.length ? serverSearchResults : [...workflowResults, ...folderResults, ...memberResults].slice(0, 10);
  }, [activities, folders, placements, search, selectedMembers, serverSearchResults, workflowCountByFolder]);
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
      const link = await onCreateShareLink(selectedWorkspace.id, {
        scope: selectedFolder ? 'FOLDER' : 'WORKSPACE',
        folderId: selectedFolder?.id,
        permission: 'VIEW',
      });
      void navigator.clipboard?.writeText(shareUrlForLink(link));
      setOpsMessage('分享链接已创建并复制');
    } catch (error) {
      setOpsMessage((error as Error).message);
    }
  };
  const updateStudioActivityFromHome = async (workflowId: string, patch: { activityName?: string; workspaceId?: string; folderId?: string; status?: string; visibility?: string }) => {
    const updated = await onUpdateActivity(workflowId, patch);
    if (selectedDetail?.id === workflowId) {
      setSelectedDetail((item) => item ? {
        ...item,
        name: updated.activityName || updated.activityCode,
        status: updated.status || item.status,
        visibility: updated.visibility || item.visibility,
      } : item);
    }
    return updated;
  };
  const deleteWorkspaceFolderFromHome = async (workspaceId: string, folderId: string) => {
    const deleted = await onDeleteFolder(workspaceId, folderId);
    if (selectedFolderId === folderId) setSelectedFolderId('');
    if (selectedDetail?.id === folderId) setSelectedDetail(null);
    return deleted;
  };
  const updateWorkspaceFromHome = async (workspaceId: string, patch: { name?: string; status?: string }) => {
    return onUpdateWorkspace(workspaceId, patch);
  };
  const deleteWorkspaceFromHome = async (workspaceId: string) => {
    const deleted = await onDeleteWorkspace(workspaceId);
    if (selectedWorkspaceId === workspaceId) {
      setSelectedWorkspaceId('');
      setSelectedFolderId('');
    }
    setSelectedDetail(null);
    return deleted;
  };
  const importWorkflowFromHome = () => {
    importWorkflowInputRef.current?.click();
  };
  const importWorkflowJSONToHome = async (file: File | null | undefined) => {
    if (!file) return;
    if (!selectedWorkspace?.id) {
      setOpsMessage('先选择 workspace 后再导入 workflow。');
      return;
    }
    const targetFolderId = selectedFolder?.id || selectedWorkspaceFolders[0]?.id || '';
    if (!targetFolderId) {
      setOpsMessage('先创建 folder 后再导入 workflow。');
      return;
    }
    try {
      const raw = await file.text();
      const parsed = JSON.parse(raw) as WorkflowDefinition | { definition?: WorkflowDefinition; name?: string };
      const definition = 'definition' in parsed && parsed.definition ? parsed.definition : parsed as WorkflowDefinition;
      const metaName = typeof definition.meta?.name === 'string' ? definition.meta.name : '';
      const baseName = ('name' in parsed && typeof parsed.name === 'string' && parsed.name) || metaName || file.name.replace(/\.json$/i, '') || '导入 Workflow';
      const created = await onCreateActivity({
        tenantId: selectedWorkspace.tenantId,
        activityName: baseName,
        activityCode: normalizeActivityCode(baseName),
        workspaceId: selectedWorkspace.id,
        folderId: targetFolderId,
        definition,
      });
      await uploadWorkspaceFile({
        workspaceId: selectedWorkspace.id,
        folderId: targetFolderId,
        activityId: created.id,
        kind: 'IMPORT_WORKFLOW',
        file,
      });
      setOpsMessage(`已导入 workflow 并保存原文件：${baseName}`);
    } catch (error) {
      setOpsMessage(`导入 Workflow 失败：${(error as Error).message}`);
    } finally {
      if (importWorkflowInputRef.current) importWorkflowInputRef.current.value = '';
    }
  };
  const openWorkspaceSettings = () => {
    setHomeUtilityDialog('settings');
  };
  const refreshWorkspaceFiles = async (workspaceId = selectedWorkspace?.id ?? '') => {
    if (!workspaceId) return;
    setWorkspaceFilesLoading(true);
    try {
      const body = await fetchJSON<{ items: WorkspaceFileObjectSummary[] }>(`/admin/v1/workspace-files?workspaceId=${encodeURIComponent(workspaceId)}&limit=50`);
      setWorkspaceFiles(body.items ?? []);
    } catch (error) {
      setOpsMessage((error as Error).message);
    } finally {
      setWorkspaceFilesLoading(false);
    }
  };
  const uploadStorageFile = async (file: File | null | undefined) => {
    if (!file) return;
    if (!selectedWorkspace?.id) {
      setOpsMessage('先选择 workspace 后再上传文件。');
      return;
    }
    if (file.size > WORKSPACE_FILE_UPLOAD_MAX_BYTES) {
      setWorkspaceFileUpload({ file, progress: 0, status: 'failed', error: `文件超过 ${formatBytes(WORKSPACE_FILE_UPLOAD_MAX_BYTES)} 限制` });
      return;
    }
    const targetFolderId = selectedFolder?.id || selectedWorkspaceFolders[0]?.id || '';
    setWorkspaceFileUpload({ file, progress: 0, status: 'uploading', error: '' });
    try {
      await uploadWorkspaceFile({
        workspaceId: selectedWorkspace.id,
        folderId: targetFolderId,
        kind: 'EXECUTION_ATTACHMENT',
        file,
        onProgress: (progress) => setWorkspaceFileUpload((state) => ({ ...state, progress })),
      });
      setWorkspaceFileUpload({ file, progress: 100, status: 'done', error: '' });
      await Promise.all([
        refreshWorkspaceFiles(selectedWorkspace.id),
        fetchJSON<WorkspaceHomeStorageSummary>('/admin/v1/workspace-home/storage').then(setStorageSummary),
      ]);
      setOpsMessage(`文件已上传：${file.name}`);
    } catch (error) {
      setWorkspaceFileUpload({ file, progress: 0, status: 'failed', error: (error as Error).message });
    } finally {
      if (storageFileInputRef.current) storageFileInputRef.current.value = '';
    }
  };
  const deleteStorageFile = async (file: WorkspaceFileObjectSummary) => {
    if (!selectedWorkspace?.id) return;
    try {
      await fetchJSON<WorkspaceFileObjectSummary>(`/admin/v1/workspace-files/${file.id}?workspaceId=${encodeURIComponent(selectedWorkspace.id)}`, { method: 'DELETE' });
      setWorkspaceFiles((items) => items.filter((item) => item.id !== file.id));
      await fetchJSON<WorkspaceHomeStorageSummary>('/admin/v1/workspace-home/storage').then(setStorageSummary);
      setOpsMessage(`文件已删除：${file.fileName}`);
    } catch (error) {
      setOpsMessage(`删除文件失败：${(error as Error).message}`);
    }
  };
  const openStorageManager = () => {
    setHomeUtilityDialog('storage');
    if (selectedWorkspace?.id) void refreshWorkspaceFiles(selectedWorkspace.id);
  };
  const refreshResources = useCallback(async (selectCode = selectedResourceCode) => {
    setResourcesLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('tenantId', selectedWorkspace?.tenantId || 'site_001');
      params.set('limit', '500');
      const body = await fetchJSON<{ items: ResourceSummary[] }>(`/admin/v1/resources?${params.toString()}`);
      const items = body.items ?? [];
      setResources(items);
      const nextCode = selectCode || items[0]?.code || '';
      setSelectedResourceCode(nextCode);
      if (nextCode) {
        const [bindingsBody, auditsBody] = await Promise.all([
          fetchJSON<{ items: ResourceBindingSummary[] }>(`/admin/v1/resources/${encodeURIComponent(nextCode)}/bindings?tenantId=${encodeURIComponent(selectedWorkspace?.tenantId || 'site_001')}&limit=100`),
          fetchJSON<{ items: ResourceAuditLogSummary[] }>(`/admin/v1/resources/${encodeURIComponent(nextCode)}/audit-logs?tenantId=${encodeURIComponent(selectedWorkspace?.tenantId || 'site_001')}&limit=100`),
        ]);
        setResourceBindings(bindingsBody.items ?? []);
        setResourceAudits(auditsBody.items ?? []);
      } else {
        setResourceBindings([]);
        setResourceAudits([]);
      }
      setResourceMessage('资源已刷新');
    } catch (error) {
      setResourceMessage((error as Error).message);
    } finally {
      setResourcesLoading(false);
    }
  }, [selectedResourceCode, selectedWorkspace?.tenantId]);
  const openResourceManager = () => {
    setHomeUtilityDialog('resources');
    void refreshResources();
  };
  const selectResource = useCallback((code: string) => {
    setSelectedResourceCode(code);
    if (!code) {
      setResourceBindings([]);
      setResourceAudits([]);
      return;
    }
    const tenantId = selectedWorkspace?.tenantId || 'site_001';
    void Promise.all([
      fetchJSON<{ items: ResourceBindingSummary[] }>(`/admin/v1/resources/${encodeURIComponent(code)}/bindings?tenantId=${encodeURIComponent(tenantId)}&limit=100`),
      fetchJSON<{ items: ResourceAuditLogSummary[] }>(`/admin/v1/resources/${encodeURIComponent(code)}/audit-logs?tenantId=${encodeURIComponent(tenantId)}&limit=100`),
    ]).then(([bindingsBody, auditsBody]) => {
      setResourceBindings(bindingsBody.items ?? []);
      setResourceAudits(auditsBody.items ?? []);
    }).catch((error) => setResourceMessage((error as Error).message));
  }, [selectedWorkspace?.tenantId]);
  const saveResource = async (resource: Partial<ResourceSummary> & { code: string }) => {
    if (!resource.code.trim()) throw new Error('资源 Code 必填');
    const tenantId = resource.tenantId || selectedWorkspace?.tenantId || 'site_001';
    await fetchJSON<ResourceSummary>(`/admin/v1/resources/${encodeURIComponent(resource.code)}?tenantId=${encodeURIComponent(tenantId)}`, {
      method: 'PUT',
      body: JSON.stringify({ ...resource, tenantId }),
    });
    setResourceMessage(`资源已保存：${resource.code}`);
    await refreshResources(resource.code);
  };
  const saveResourceBinding = async (resourceCode: string, binding: Partial<ResourceBindingSummary>) => {
    const tenantId = binding.tenantId || selectedWorkspace?.tenantId || 'site_001';
    await fetchJSON<ResourceBindingSummary>(`/admin/v1/resources/${encodeURIComponent(resourceCode)}/bindings?tenantId=${encodeURIComponent(tenantId)}`, {
      method: 'POST',
      body: JSON.stringify({ ...binding, tenantId }),
    });
    setResourceMessage(`资源权限已保存：${resourceCode}`);
    selectResource(resourceCode);
  };
  const testResource = async (resourceCode: string) => {
    const tenantId = selectedWorkspace?.tenantId || 'site_001';
    const result = await fetchJSON<{ ok?: boolean; errorCode?: string }>(`/admin/v1/resources/${encodeURIComponent(resourceCode)}/test?tenantId=${encodeURIComponent(tenantId)}`, { method: 'POST' });
    setResourceMessage(result.ok ? `资源测试通过：${resourceCode}` : `资源测试失败：${result.errorCode || resourceCode}`);
    selectResource(resourceCode);
  };
  const normalizeDetailVisibility = (value?: string): WorkspaceDetailTarget['visibility'] => {
    const normalized = (value || '').toLowerCase();
    if (normalized.includes('public')) return 'Public';
    if (normalized.includes('team') || normalized.includes('workspace')) return 'Workspace';
    return 'Private';
  };
  const detailTarget: WorkspaceDetailTarget = useMemo(() => {
    if (selectedDetail) {
      return {
        id: selectedDetail.id,
        type: selectedDetail.type,
        name: selectedDetail.name,
        visibility: detailVisibilityOverride || normalizeDetailVisibility(selectedDetail.visibility),
        workflowCount: selectedDetail.type === 'Folder' ? workflowCountByFolder[selectedDetail.id] ?? 0 : 1,
        folderCount: selectedDetail.type === 'Folder' ? selectedWorkspaceFolders.filter((folder) => (folder.parentId ?? '') === selectedDetail.id).length : 0,
        memberCount: selectedMembers.filter((member) => member.status !== 'REMOVED').length,
        updatedLabel: '2 小时前更新',
        owner: selectedDetail.owner,
        version: selectedDetail.type === 'Workflow' ? selectedDetail.version : undefined,
        status: selectedDetail.type === 'Workflow' ? selectedDetail.status : undefined,
      };
    }
    return {
      id: selectedFolder?.id || selectedWorkspace?.id || 'workspace-root',
      type: selectedFolder ? 'Folder' : 'Workspace',
      name: selectedFolder?.name || selectedWorkspace?.name || '风控活动',
      visibility: detailVisibilityOverride || normalizeDetailVisibility(selectedFolder?.visibility || selectedWorkspace?.visibility),
      workflowCount: selectedFolder ? workflowCountByFolder[selectedFolder.id] ?? selectedWorkflows.length : selectedWorkflows.length,
      folderCount: selectedChildFolders.length,
      memberCount: selectedMembers.filter((member) => member.status !== 'REMOVED').length,
      updatedLabel: workflowStatus.includes('失败') || workflowStatus.includes('不可用') ? workflowStatus : '2 小时前更新',
    };
  }, [detailVisibilityOverride, selectedChildFolders.length, selectedDetail, selectedFolder, selectedMembers, selectedWorkspace, selectedWorkflows.length, selectedWorkspaceFolders, workflowCountByFolder, workflowStatus]);
  const detailActivityGrants = detailTarget.type === 'Workflow' ? activityGrantsByActivity[detailTarget.id] ?? [] : [];
  useEffect(() => {
    if (detailTarget.type !== 'Workflow' || activityGrantsByActivity[detailTarget.id]) return;
    void onRefreshActivityACL(detailTarget.id).catch((error) => setOpsMessage((error as Error).message));
  }, [activityGrantsByActivity, detailTarget.id, detailTarget.type, onRefreshActivityACL]);
  const detailShareLink = shareLinkForTarget(detailTarget);
  const detailShareUrl = shareUrlForLink(detailShareLink) || `${window.location.origin}/share/pending-${detailTarget.type.toLowerCase()}-${detailTarget.id}`;
  const detailDataSources = useMemo(() => {
    const storageSource = storageSummary
      ? storageSummary.fileObjects !== undefined
        ? `文件存储已接入 · ${storageSummary.fileObjects} objects`
        : 'PostgreSQL 对象统计'
      : '默认容量估算';
    const storageTone = storageSummary ? 'good' : 'warn';
    const shareValue = detailShareLink
      ? `Active · used ${detailShareLink.accessCount ?? 0}${detailShareLink.expiresAt ? ` · expires ${new Date(detailShareLink.expiresAt).toLocaleDateString()}` : ''}`
      : '未创建真实 share link';
    return [
      {
        label: `${detailTarget.type} 数据源`,
        value: 'Admin API -> PostgreSQL',
        tone: 'good' as const,
      },
      {
        label: '权限来源',
        value: detailTarget.type === 'Workflow'
          ? `Visibility + Workflow ACL (${detailActivityGrants.length})`
          : 'Workspace role + Visibility',
        tone: 'info' as const,
      },
      {
        label: '存储统计',
        value: storageSource,
        tone: storageTone as 'good' | 'warn',
      },
      {
        label: 'Share link',
        value: shareValue,
        tone: detailShareLink ? 'good' as const : 'warn' as const,
      },
    ];
  }, [detailActivityGrants.length, detailShareLink, detailTarget.type, storageSummary]);
  const updateDetailVisibility = (visibility: WorkspaceDetailTarget['visibility']) => {
    setDetailVisibilityOverride(visibility);
    if (detailTarget.type === 'Workspace') {
      void (async () => {
        const nextVisibility = visibility === 'Workspace' ? 'TEAM' : visibility.toUpperCase();
        const updated = await onUpdateWorkspace(detailTarget.id, { visibility: nextVisibility });
        setOpsMessage(`${updated.name} 可见性已切换为 ${visibility}`);
      })().catch((error) => setOpsMessage((error as Error).message));
    } else if (selectedFolder && detailTarget.type === 'Folder') {
      void updateSelectedFolderVisibility(visibility === 'Workspace' ? 'TEAM' : visibility.toUpperCase());
    } else {
      void updateStudioActivityFromHome(detailTarget.id, { visibility: visibility === 'Workspace' ? 'TEAM' : visibility.toUpperCase() })
        .then((updated) => setOpsMessage(`${updated.activityName || updated.activityCode} 可见性已切换为 ${visibility}`))
        .catch((error) => setOpsMessage((error as Error).message));
    }
  };
  const openDetailAction = (action: 'rename' | 'move' | 'archive') => {
    setDetailAction(action);
    setDetailActionName(detailTarget.name);
    setDetailActionFolderId(selectedFolder?.id || selectedWorkspaceFolders[0]?.id || '');
    setDetailActionError('');
  };
  const openDetailActionFor = (action: WorkspaceItemAction, item: { id: string; type: 'Folder' | 'Workflow'; name: string; visibility: string; status?: string; version?: string; owner?: string }) => {
    setSelectedDetail(item);
    if (action === 'copyLink') {
      const url = shareUrlForLink(shareLinkForTarget(item)) || `${window.location.origin}/share/pending-${item.type.toLowerCase()}-${item.id}`;
      void navigator.clipboard?.writeText(url);
      setOpsMessage(`已复制链接：${item.name}`);
      return;
    }
    if (action === 'permission') {
      setDetailVisibilityOverride(item.visibility === 'Workspace' ? 'Workspace' : normalizeDetailVisibility(item.visibility));
      setOpsMessage(`已打开权限：${item.name}`);
      return;
    }
    if (action === 'share') {
      if (selectedWorkspace) {
        let created: Promise<WorkspaceShareLinkSummary>;
        if (item.type === 'Folder') {
          created = onCreateShareLink(selectedWorkspace.id, { scope: 'FOLDER', folderId: item.id, permission: 'VIEW' });
        } else {
          created = onCreateShareLink(selectedWorkspace.id, { scope: 'ACTIVITY', activityId: item.id, folderId: selectedFolder?.id, permission: 'VIEW' });
        }
        void created.then((link) => {
          void navigator.clipboard?.writeText(shareUrlForLink(link));
          setOpsMessage(`已创建分享并复制链接：${item.name}`);
        }).catch((error) => setOpsMessage((error as Error).message));
      }
      return;
    }
    setDetailAction(action);
    setDetailActionName(item.name);
    setDetailActionFolderId(selectedFolder?.id || selectedWorkspaceFolders.find((folder) => folder.id !== item.id)?.id || '');
    setDetailActionError('');
  };
  const handleCurrentAction = (action: WorkspaceItemAction) => {
    if (action === 'copyLink') {
      void navigator.clipboard?.writeText(detailShareUrl);
      setOpsMessage(`已复制链接：${detailTarget.name}`);
      return;
    }
    if (action === 'permission') {
      setOpsMessage(`已打开权限：${detailTarget.name}`);
      return;
    }
    if (action === 'share') {
      if (selectedWorkspace) void createFolderShare();
      else setOpsMessage('先选择 workspace。');
      return;
    }
    openDetailAction(action);
  };
  const archiveItems = async (items: Array<{ id: string; type: 'Folder' | 'Workflow'; name: string; visibility: string; status?: string; version?: string; owner?: string }>) => {
    if (!selectedWorkspace) {
      setOpsMessage('先选择 workspace。');
      return;
    }
    setOpsMessage(`正在归档 ${items.length} 项...`);
    try {
      await fetchJSON<{ count: number }>(`/admin/v1/workspaces/${selectedWorkspace.id}/batch`, {
        method: 'POST',
        body: JSON.stringify({ action: 'ARCHIVE', items: items.map((item) => ({ id: item.id, type: item.type })) }),
      });
      setOpsMessage(`已归档 ${items.length} 项`);
      await onRefreshWorkspaceOps(selectedWorkspace.id);
      window.location.reload();
    } catch (error) {
      setOpsMessage(`批量归档失败：${(error as Error).message}`);
    }
  };
  const openBatchAction = (action: 'move' | 'permission' | 'copyLink', items: Array<{ id: string; type: 'Folder' | 'Workflow'; name: string; visibility: string; status?: string; version?: string; owner?: string }>) => {
    if (action === 'copyLink') {
      const links = items.map((item) => shareUrlForLink(shareLinkForTarget(item)) || `${window.location.origin}/share/pending-${item.type.toLowerCase()}-${item.id}`).join('\n');
      void navigator.clipboard?.writeText(links);
      setOpsMessage(`已复制 ${items.length} 个链接`);
      return;
    }
    setBatchAction(action);
    setBatchActionItems(items);
    setBatchActionFolderId(selectedFolder?.id || selectedWorkspaceFolders[0]?.id || '');
    setBatchActionVisibility('Workspace');
    setBatchActionError('');
    setBatchActionFailures([]);
  };
  const runWorkspaceBatch = async (workspaceId: string, body: Record<string, unknown>) => {
    const response = await fetch(`/admin/v1/workspaces/${workspaceId}/batch`, {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        'X-Actor-Id': studioActorId(),
      },
      body: JSON.stringify(body),
    });
    const payload = await response.json().catch(() => ({})) as WorkspaceBatchResult & { error?: string; message?: string };
    if (!response.ok) {
      const failures = payload.items?.filter((item) => item.status === 'FAILED' || item.error) ?? [];
      if (failures.length) {
        setBatchActionFailures(failures);
        throw new Error(payload.error || payload.message || `${failures.length} 项处理失败`);
      }
      throw new APIError(payload.error ?? payload.message ?? `${response.status} ${response.statusText}`, response.status, response.statusText);
    }
    return payload;
  };
  const submitBatchAction = async () => {
    if (!selectedWorkspace) {
      setBatchActionError('先选择 workspace。');
      return;
    }
    setBatchActionBusy(true);
    setBatchActionError('');
    setBatchActionFailures([]);
    try {
      if (batchAction === 'move') {
        if (!batchActionFolderId) throw new Error('请选择目标 folder');
        await runWorkspaceBatch(selectedWorkspace.id, { action: 'MOVE', folderId: batchActionFolderId, items: batchActionItems.map((item) => ({ id: item.id, type: item.type })) });
        setOpsMessage(`已移动 ${batchActionItems.length} 项`);
      }
      if (batchAction === 'permission') {
        const visibility = batchActionVisibility === 'Workspace' ? 'TEAM' : batchActionVisibility.toUpperCase();
        await runWorkspaceBatch(selectedWorkspace.id, { action: 'PERMISSION', visibility, items: batchActionItems.map((item) => ({ id: item.id, type: item.type })) });
        setOpsMessage(`已更新 ${batchActionItems.length} 项权限`);
      }
      await onRefreshWorkspaceOps(selectedWorkspace.id);
      window.location.reload();
      setBatchAction('');
      setBatchActionItems([]);
    } catch (error) {
      setBatchActionError((error as Error).message);
    } finally {
      setBatchActionBusy(false);
    }
  };
  const submitDetailAction = async () => {
    if (!selectedWorkspace) {
      setDetailActionError('先选择 workspace。');
      return;
    }
    setDetailActionBusy(true);
    setDetailActionError('');
    try {
      if (detailAction === 'rename') {
        const nextName = detailActionName.trim();
        if (!nextName) throw new Error('名称不能为空');
        if (detailTarget.type === 'Workspace') {
          const updated = await updateWorkspaceFromHome(detailTarget.id, { name: nextName });
          setOpsMessage(`已重命名 workspace：${updated.name}`);
        } else if (detailTarget.type === 'Folder') {
          const updated = await onUpdateFolder(selectedWorkspace.id, detailTarget.id, { name: nextName });
          setSelectedDetail((item) => item && item.id === updated.id ? { ...item, name: updated.name } : item);
          setOpsMessage(`已重命名 folder：${updated.name}`);
        } else if (detailTarget.type === 'Workflow') {
          const updated = await updateStudioActivityFromHome(detailTarget.id, { activityName: nextName });
          setOpsMessage(`已重命名 workflow：${updated.activityName}`);
        }
      }
      if (detailAction === 'move') {
        if (detailTarget.type === 'Workspace') throw new Error('Workspace 不支持移动');
        if (!detailActionFolderId) throw new Error('请选择目标 folder');
        if (detailTarget.type === 'Folder') {
          const siblings = selectedWorkspaceFolders.filter((folder) => (folder.parentId ?? '') === detailActionFolderId);
          const updated = await onUpdateFolder(selectedWorkspace.id, detailTarget.id, { parentId: detailActionFolderId, sortOrder: (siblings.length + 1) * 10 });
          setSelectedDetail((item) => item && item.id === updated.id ? { ...item, name: updated.name } : item);
          setOpsMessage(`已移动 folder：${updated.name}`);
        } else if (detailTarget.type === 'Workflow') {
          const updated = await updateStudioActivityFromHome(detailTarget.id, { workspaceId: selectedWorkspace.id, folderId: detailActionFolderId });
          setOpsMessage(`已移动 workflow：${updated.activityName}`);
        }
      }
      if (detailAction === 'archive') {
        if (detailTarget.type === 'Workspace') {
          await deleteWorkspaceFromHome(detailTarget.id);
          setOpsMessage(`已归档 workspace：${detailTarget.name}`);
        } else if (detailTarget.type === 'Folder') {
          await deleteWorkspaceFolderFromHome(selectedWorkspace.id, detailTarget.id);
          setOpsMessage(`已归档 folder：${detailTarget.name}`);
        } else if (detailTarget.type === 'Workflow') {
          const updated = await updateStudioActivityFromHome(detailTarget.id, { status: 'ARCHIVED' });
          setOpsMessage(`已归档 workflow：${updated.activityName}`);
        }
      }
      setDetailAction('');
    } catch (error) {
      setDetailActionError((error as Error).message);
    } finally {
      setDetailActionBusy(false);
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
    const activity = activities.find((item) => item.id === activityId);
    const nextStatus = activity?.status === 'ARCHIVED' || archivedActivityIds.has(activityId) ? 'ACTIVE' : 'ARCHIVED';
    void updateStudioActivityFromHome(activityId, { status: nextStatus })
      .then(() => {
        setArchivedActivityIds((items) => {
          const next = new Set(items);
          next.delete(activityId);
          saveStudioStringSet('exmarketing.studio.archivedActivities', next);
          return next;
        });
        setOpsMessage(nextStatus === 'ARCHIVED' ? 'Workflow 已归档' : 'Workflow 已恢复');
      })
      .catch((error) => setOpsMessage((error as Error).message));
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
        <WorkspaceTopBar
          workspaceName={selectedWorkspace?.name ?? 'hello'}
          folderName={selectedFolder?.name ?? '风控活动'}
          searchValue={search}
          workflowCount={selectedWorkflows.length}
          folderCount={selectedChildFolders.length}
          memberCount={selectedMembers.filter((member) => member.status !== 'REMOVED').length}
          searchResults={globalSearchResults}
          actorInitial={currentActorId || 'D'}
          isLoading={!workspaceDataReady}
          onSearchChange={setSearch}
          onSearchResultSelect={(result) => {
            if (result.type === 'workspace') {
              setSelectedWorkspaceId(result.workspaceId || result.id);
              setSelectedFolderId('');
              return;
            }
            if (result.type === 'workflow') {
              const placement = result.workspaceId || result.folderId ? { workspaceId: result.workspaceId, folderId: result.folderId } : placements[result.id];
              if (placement?.workspaceId) setSelectedWorkspaceId(placement.workspaceId);
              if (placement?.folderId) setSelectedFolderId(placement.folderId);
              onSelectActivity(result.id);
              return;
            }
            if (result.type === 'folder') {
              const folder = folders.find((item) => item.id === (result.folderId || result.id));
              if (folder?.workspaceId) setSelectedWorkspaceId(folder.workspaceId);
              else if (result.workspaceId) setSelectedWorkspaceId(result.workspaceId);
              setSelectedFolderId(result.folderId || result.id);
              setSelectedDetail({ id: result.folderId || result.id, type: 'Folder', name: result.title, visibility: folder?.visibility || 'TEAM' });
              return;
            }
            setOpsMessage(`成员已定位：${result.title}`);
          }}
          onNewWorkflow={() => {
            if (!canEditWorkspace) {
              setOpsMessage(`${permissionCapabilities.reason}，不能新建 workflow。`);
              return;
            }
            openCreateDialog('新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id });
          }}
          onImport={() => {
            if (!canEditWorkspace) {
              setOpsMessage(`${permissionCapabilities.reason}，不能导入 workflow。`);
              return;
            }
            importWorkflowFromHome();
          }}
          onSettings={openWorkspaceSettings}
        />
        <WorkspaceHomeLayout
          sidebar={(
            <WorkspaceSidebar
              workspaces={activeWorkspaces}
              folders={selectedWorkspaceFolders}
              selectedWorkspaceId={selectedWorkspace?.id}
              selectedFolderId={selectedFolder?.id}
              workflowCountByFolder={workflowCountByFolder}
              recentCount={recentWorkflows.length}
              sharedCount={sharedWorkflowCount}
              favoriteCount={favoriteActivityIds.size}
              archivedCount={archivedWorkflowCount}
              memberCount={selectedMembers.filter((member) => member.status !== 'REMOVED').length}
              auditCount={selectedAuditLogs.length}
              storageUsedGb={storageUsedGb}
              storageTotalGb={storageTotalGb}
              isLoading={!workspaceDataReady}
              onWorkspaceChange={(workspaceId) => {
                setSelectedWorkspaceId(workspaceId);
                setSelectedFolderId('');
              }}
              onFolderSelect={setSelectedFolderId}
              onCreateWorkspace={() => {
                setWorkspaceError('');
                setShowWorkspaceDialog(true);
              }}
              onNavRecent={() => {
                setSortMode('recent');
                setStatusFilter('ALL');
                setShowFavoritesOnly(false);
              }}
              onNavWorkspace={() => {
                setStatusFilter('ACTIVE');
                setShowFavoritesOnly(false);
              }}
              onNavShared={() => setHomeUtilityDialog('share')}
              onNavFavorites={() => {
                setShowFavoritesOnly(true);
                setStatusFilter('ALL');
              }}
              onNavArchived={() => {
                setStatusFilter('ARCHIVED');
                setShowFavoritesOnly(false);
              }}
              onNavMembers={() => setHomeUtilityDialog('members')}
              onNavResources={openResourceManager}
              onNavAudit={() => setHomeUtilityDialog('audit')}
              onManageStorage={openStorageManager}
            />
          )}

          content={(
            <WorkspaceContent
              workspaceName={selectedWorkspace?.name ?? 'hello'}
              folderName={selectedFolder?.name ?? '风控活动'}
              folderVisibility={selectedFolder?.visibility ?? selectedWorkspace?.visibility ?? 'Private'}
              folderUpdatedLabel={workflowStatus.includes('失败') || workflowStatus.includes('不可用') ? workflowStatus : '2 小时前更新'}
              folders={selectedChildFolders}
              workflows={selectedWorkflows}
              templates={templates}
              workflowCountByFolder={workflowCountByFolder}
              searchValue={search}
              statusFilter={statusFilter}
              sortMode={sortMode}
              favoriteWorkflowIds={favoriteActivityIds}
              archivedWorkflowIds={archivedActivityIds}
              isLoading={!workspaceDataReady}
              errorMessage={workspaceStatusError}
              capabilities={permissionCapabilities}
              onRetryLoad={() => {
                if (selectedWorkspace?.id) void onRefreshWorkspaceOps(selectedWorkspace.id);
                fetchJSON<WorkspaceHomeStorageSummary>('/admin/v1/workspace-home/storage')
                  .then(setStorageSummary)
                  .catch((error) => setOpsMessage((error as Error).message));
              }}
              onSearchChange={setSearch}
              onStatusFilterChange={setStatusFilter}
              onSortChange={setSortMode}
              onCreateFolder={() => {
                setFolderError('');
                setShowFolderDialog(true);
              }}
              onCreateWorkflow={() => openCreateDialog(search || '新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id })}
              onImportWorkflow={importWorkflowFromHome}
              onCurrentAction={handleCurrentAction}
              onItemSelect={(item) => setSelectedDetail(item)}
              onFolderOpen={setSelectedFolderId}
              onWorkflowOpen={onSelectActivity}
              onTemplateOpen={(templateId) => {
                const template = templates.find((item) => item.id === templateId);
                if (!template) {
                  setOpsMessage('模板不存在或尚未加载');
                  return;
                }
                if (!selectedWorkspace?.id || !selectedFolder?.id) {
                  setOpsMessage('先选择目标 workspace 和 folder 后再从模板创建');
                  return;
                }
                void onCreateActivity({
                  tenantId: selectedWorkspace.tenantId,
                  activityName: template.name,
                  activityCode: normalizeActivityCode(template.name),
                  workspaceId: selectedWorkspace.id,
                  folderId: selectedFolder.id,
                  definition: template.definition,
                });
              }}
              onToggleFavorite={toggleFavorite}
              onToggleArchived={toggleArchived}
              onItemAction={openDetailActionFor}
              onBulkAction={openBatchAction}
              onBulkArchive={archiveItems}
            />
          )}

          detail={(
            <WorkspaceDetailPanel
              target={detailTarget}
              members={selectedMembers}
              audits={selectedAuditLogs}
              activityGrants={detailActivityGrants}
              shareUrl={detailShareUrl}
              dataSources={detailDataSources}
              isLoading={!workspaceDataReady}
              errorMessage={workspaceStatusError}
              capabilities={permissionCapabilities}
              onVisibilityChange={updateDetailVisibility}
              onCreateActivityGrant={(input) => (
                onCreateActivityACL(detailTarget.id, input)
                  .then(() => setOpsMessage('Workflow 单独授权已添加'))
                  .catch((error) => {
                    setOpsMessage((error as Error).message);
                    throw error;
                  })
              )}
              onDeleteActivityGrant={(grantId) => (
                onDeleteActivityACL(detailTarget.id, grantId)
                  .then(() => setOpsMessage('Workflow 单独授权已删除'))
                  .catch((error) => {
                    setOpsMessage((error as Error).message);
                    throw error;
                  })
              )}
              onCopyShare={() => {
                const existing = shareLinkForTarget(detailTarget);
                if (existing) {
                  void navigator.clipboard?.writeText(shareUrlForLink(existing));
                  setOpsMessage('分享链接已复制');
                  return;
                }
                if (!selectedWorkspace) {
                  setOpsMessage('先选择 workspace。');
                  return;
                }
                void onCreateShareLink(selectedWorkspace.id, {
                  scope: detailTarget.type === 'Workspace' ? 'WORKSPACE' : detailTarget.type === 'Folder' ? 'FOLDER' : 'ACTIVITY',
                  folderId: detailTarget.type === 'Folder' ? detailTarget.id : selectedFolder?.id,
                  activityId: detailTarget.type === 'Workflow' ? detailTarget.id : undefined,
                  permission: 'VIEW',
                }).then((link) => {
                  void navigator.clipboard?.writeText(shareUrlForLink(link));
                  setOpsMessage('分享链接已创建并复制');
                }).catch((error) => setOpsMessage((error as Error).message));
              }}
              onManageMembers={() => setHomeUtilityDialog('members')}
              onManageShare={() => {
                setHomeUtilityDialog('share');
              }}
              onViewAllAudit={() => setHomeUtilityDialog('audit')}
              onRename={() => openDetailAction('rename')}
              onMove={() => openDetailAction('move')}
              onArchive={() => openDetailAction('archive')}
              onClose={() => {
                setSelectedDetail(null);
                setOpsMessage('详情面板已回到当前文件夹');
              }}
            />
          )}
        />
        <input
          ref={importWorkflowInputRef}
          className="workflow-import-input"
          type="file"
          accept="application/json,.json"
          onChange={(event) => {
            void importWorkflowJSONToHome(event.target.files?.[0]);
          }}
        />
        {opsToasts.length ? (
          <div className="exm-toast-stack" role="status" aria-live="polite">
            {opsToasts.map((toast) => (
              <div key={toast.id} className={`exm-toast ${toast.tone}`}>
                <span className="exm-toast-dot" />
                <p>{toast.message}</p>
                <button type="button" aria-label="关闭提示" onClick={() => dismissOpsToast(toast.id)}>×</button>
              </div>
            ))}
          </div>
        ) : null}
      </section>
      {detailAction ? (
        <div className="studio-create-dialog-backdrop" onClick={() => setDetailAction('')}>
          <section className="studio-create-dialog" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>{detailAction === 'rename' ? '重命名' : detailAction === 'move' ? '移动' : '归档'} {detailTarget.type}</b>
                <span>{detailTarget.name}</span>
              </div>
              <button onClick={() => setDetailAction('')}>×</button>
            </div>
            <div className="studio-create-grid">
              {detailAction === 'rename' ? (
                <label>
                  <span>名称</span>
                  <input value={detailActionName} onChange={(event) => setDetailActionName(event.target.value)} />
                </label>
              ) : null}
              {detailAction === 'move' ? (
                <label>
                  <span>目标 Folder</span>
                  <select data-testid="detail-action-target-folder" value={detailActionFolderId} onChange={(event) => setDetailActionFolderId(event.target.value)}>
                    <option value="">选择 folder</option>
                    {selectedWorkspaceFolders.filter((folder) => folder.id !== detailTarget.id).map((folder) => (
                      <option key={folder.id} value={folder.id}>{folder.name}</option>
                    ))}
                  </select>
                </label>
              ) : null}
              {detailAction === 'archive' ? (
                <div className="studio-confirm-copy">
                  <b>确认归档？</b>
                  <span>{detailTarget.type === 'Folder' ? 'Folder 会从当前目录树移除。' : 'Workflow 状态会更新为 ARCHIVED。'}</span>
                </div>
              ) : null}
              <div className="studio-dialog-actions">
                <button className="dialog-secondary" type="button" onClick={() => setDetailAction('')}>取消</button>
                <button className={detailAction === 'archive' ? 'dialog-danger' : 'dialog-primary'} data-testid="detail-action-confirm" type="button" onClick={submitDetailAction} disabled={detailActionBusy}>
                  {detailActionBusy ? '处理中...' : detailAction === 'archive' ? '确认归档' : '确认'}
                </button>
              </div>
            </div>
            {detailActionError ? <p>{detailActionError}</p> : null}
          </section>
        </div>
      ) : null}
      {homeUtilityDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" role="dialog" aria-modal="true" aria-labelledby="home-utility-dialog-title" data-testid="home-utility-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b id="home-utility-dialog-title">{homeUtilityDialog === 'settings' ? 'Workspace 设置' : homeUtilityDialog === 'storage' ? '存储空间' : homeUtilityDialog === 'members' ? '成员管理' : homeUtilityDialog === 'share' ? '分享管理' : homeUtilityDialog === 'resources' ? '资源管理' : '审计记录'}</b>
                <span>{selectedWorkspace?.name ?? 'hello'}</span>
              </div>
              <button onClick={() => setHomeUtilityDialog('')}>×</button>
            </div>
            {homeUtilityDialog === 'settings' ? (
              <div className="studio-create-grid">
                <div className="studio-connection-status">
                  <b>产品连接状态</b>
                  {detailDataSources.map((source) => (
                    <span key={source.label}>
                      <i>{source.label}</i>
                      <strong className={source.tone ?? 'info'}>{source.value}</strong>
                    </span>
                  ))}
                  <span>
                    <i>搜索索引</i>
                    <strong className={serverSearchResults.length || !search.trim() ? 'good' : 'warn'}>
                      PostgreSQL fuzzy/index + fallback contains
                    </strong>
                  </span>
                  <span>
                    <i>邮件邀请</i>
                    <strong className="info">SMTP/SendGrid/SES 配置驱动</strong>
                  </span>
                </div>
                <label>
                  <span>Workspace</span>
                  <input value={workspaceSettingsDraft.name} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, name: event.target.value }))} />
                </label>
                <label>
                  <span>Slug</span>
                  <input value={workspaceSettingsDraft.slug} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, slug: event.target.value }))} />
                </label>
                <label>
                  <span>Icon</span>
                  <input value={workspaceSettingsDraft.icon} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, icon: event.target.value }))} />
                </label>
                <label>
                  <span>Color</span>
                  <input type="color" value={workspaceSettingsDraft.color || '#2563eb'} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, color: event.target.value }))} />
                </label>
                <label>
                  <span>描述</span>
                  <input value={workspaceSettingsDraft.description} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, description: event.target.value }))} />
                </label>
                <label>
                  <span>Visibility</span>
                  <select value={workspaceSettingsDraft.visibility} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, visibility: event.target.value }))}>
                    <option value="PRIVATE">Private</option>
                    <option value="TEAM">Workspace</option>
                    <option value="PUBLIC">Public</option>
                  </select>
                </label>
                <label>
                  <span>Default Role</span>
                  <select value={workspaceSettingsDraft.defaultRole} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, defaultRole: event.target.value as WorkspaceRole }))}>
                    <option value="VIEWER">Viewer</option>
                    <option value="EDITOR">Editor</option>
                    <option value="ADMIN">Admin</option>
                  </select>
                </label>
                <label>
                  <span>Owner</span>
                  <input value={workspaceSettingsDraft.ownerId} onChange={(event) => setWorkspaceSettingsDraft((draft) => ({ ...draft, ownerId: event.target.value }))} />
                </label>
                <label>
                  <span>当前角色</span>
                  <input value={`${selectedRole} · ${currentActorId}`} readOnly />
                </label>
                <button disabled={!selectedWorkspace || !canManageFolders} onClick={() => {
                  if (!selectedWorkspace) return;
                  void onUpdateWorkspace(selectedWorkspace.id, {
                    name: workspaceSettingsDraft.name,
                    slug: workspaceSettingsDraft.slug,
                    description: workspaceSettingsDraft.description,
                    icon: workspaceSettingsDraft.icon,
                    color: workspaceSettingsDraft.color,
                    visibility: workspaceSettingsDraft.visibility,
                    defaultRole: workspaceSettingsDraft.defaultRole,
                    ownerId: workspaceSettingsDraft.ownerId,
                  }).then((updated) => setOpsMessage(`Workspace 设置已保存：${updated.name}`)).catch((error) => setOpsMessage((error as Error).message));
                }}>保存设置</button>
                <button onClick={() => selectedWorkspace && void onRefreshWorkspaceOps(selectedWorkspace.id)}>刷新成员 / 分享 / 审计</button>
              </div>
            ) : homeUtilityDialog === 'storage' ? (
              <div className="studio-create-grid">
                <div className="studio-confirm-copy">
                  <b>{storageUsedGb.toFixed(2)} GB / {storageTotalGb.toFixed(0)} GB</b>
                  <span>{storageSummary ? `${storageSummary.workflows} workflows · ${storageSummary.folders} folders · ${storageSummary.members} members · ${storageSummary.auditLogs} audit logs · ${storageSummary.versions ?? 0} versions · ${storageSummary.executions ?? 0} runs · ${storageSummary.fileObjects ?? 0} files` : '后端存储统计不可用，正在显示默认容量。'}</span>
                </div>
                <div className="studio-file-manager">
                  <div className="studio-file-manager-head">
                    <span>
                      <b>文件对象</b>
                      <em>单文件上限 {formatBytes(WORKSPACE_FILE_UPLOAD_MAX_BYTES)}，支持导入文件、附件、执行日志附件。</em>
                    </span>
                    <button type="button" data-testid="storage-upload-trigger" disabled={!selectedWorkspace || workspaceFileUpload.status === 'uploading'} onClick={() => storageFileInputRef.current?.click()}>上传文件</button>
                  </div>
                  <input
                    ref={storageFileInputRef}
                    className="workflow-import-input"
                    type="file"
                    onChange={(event) => {
                      void uploadStorageFile(event.target.files?.[0]);
                    }}
                  />
                  {workspaceFileUpload.status !== 'idle' ? (
                    <div className={`studio-file-upload ${workspaceFileUpload.status}`} data-testid="storage-upload-status">
                      <span>
                        <b>{workspaceFileUpload.file?.name ?? '文件上传'}</b>
                        <em>{workspaceFileUpload.file ? formatBytes(workspaceFileUpload.file.size) : ''}</em>
                      </span>
                      <div className="studio-file-upload-track"><i style={{ width: `${workspaceFileUpload.status === 'failed' ? 100 : workspaceFileUpload.progress}%` }} /></div>
                      <strong>{workspaceFileUpload.status === 'uploading' ? `${workspaceFileUpload.progress}%` : workspaceFileUpload.status === 'done' ? '上传完成' : workspaceFileUpload.error}</strong>
                      {workspaceFileUpload.status === 'failed' && workspaceFileUpload.file ? (
                        <button type="button" data-testid="storage-upload-retry" onClick={() => void uploadStorageFile(workspaceFileUpload.file)}>重试</button>
                      ) : null}
                    </div>
                  ) : null}
                  <div className="studio-file-list" data-testid="storage-file-list">
                    {workspaceFilesLoading ? <span className="studio-file-empty">文件列表加载中...</span> : workspaceFiles.length ? workspaceFiles.map((file) => (
                      <span key={file.id}>
                        <i>{file.kind}</i>
                        <strong>{file.fileName}</strong>
                        <em>{formatBytes(file.sizeBytes)} · {file.storageProvider} · {new Date(file.createdAt).toLocaleString()}</em>
                        <button type="button" disabled={!canManageMembers} title={!canManageMembers ? '当前角色不能删除文件' : '删除文件'} onClick={() => void deleteStorageFile(file)}>删除</button>
                      </span>
                    )) : <span className="studio-file-empty">暂无文件对象。</span>}
                  </div>
                </div>
                <button onClick={() => {
                  fetchJSON<WorkspaceHomeStorageSummary>('/admin/v1/workspace-home/storage')
                    .then(setStorageSummary)
                    .catch((error) => setOpsMessage((error as Error).message));
                  if (selectedWorkspace?.id) void refreshWorkspaceFiles(selectedWorkspace.id);
                }}>刷新存储统计 / 文件列表</button>
                <button onClick={() => {
                  setArchivedActivityIds(new Set());
                  saveStudioStringSet('exmarketing.studio.archivedActivities', new Set());
                  setOpsMessage('已清理本地归档标记');
                  setHomeUtilityDialog('');
                }}>清理本地归档标记</button>
              </div>
            ) : homeUtilityDialog === 'resources' ? (
              <ResourceManagement
                tenantId={selectedWorkspace?.tenantId || 'site_001'}
                workspaceId={selectedWorkspace?.id}
                folderId={selectedFolder?.id}
                resources={resources}
                selectedResourceCode={selectedResource?.code || selectedResourceCode}
                selectedResource={selectedResource}
                bindings={resourceBindings}
                audits={resourceAudits}
                loading={resourcesLoading}
                message={resourceMessage}
                onRefresh={() => void refreshResources()}
                onSelectResource={selectResource}
                onSaveResource={saveResource}
                onSaveBinding={saveResourceBinding}
                onTestResource={testResource}
              />
            ) : homeUtilityDialog === 'members' ? (
              <div className="studio-member-manager">
                {canManageMembers ? (
                  <section className="studio-member-invite">
                    <div>
                      <b>邀请成员</b>
                      <span>添加 User ID 或 Email，并分配 workspace 角色。</span>
                    </div>
                    <label>
                      <span>User ID</span>
                      <input value={memberUserId} onChange={(event) => setMemberUserId(event.target.value)} placeholder="user_123" />
                    </label>
                    <label>
                      <span>Email</span>
                      <input value={memberEmail} onChange={(event) => setMemberEmail(event.target.value)} placeholder="member@example.com" />
                    </label>
                    <label>
                      <span>Role</span>
                      <select value={memberRole} onChange={(event) => setMemberRole(event.target.value as WorkspaceRole)}>
                        <option value="EDITOR">Editor</option>
                        <option value="ADMIN">Admin</option>
                        <option value="VIEWER">Viewer</option>
                      </select>
                    </label>
                    <button onClick={() => void submitMember()}>添加成员</button>
                  </section>
                ) : (
                  <p className="studio-member-readonly-note">当前角色只能查看成员，邀请和角色调整由 Owner 管理。</p>
                )}
                <section className={canManageMembers ? 'studio-member-list' : 'studio-member-list readonly'}>
                  <header>
                    <b>{canManageMembers ? '当前成员' : '只读成员列表'}</b>
                    <span>{selectedMembers.filter((member) => member.status !== 'REMOVED').length} 位成员</span>
                  </header>
                  {selectedMembers.filter((member) => member.status !== 'REMOVED').map((member) => {
                    const memberName = member.displayName || member.userId || member.email || member.id;
                    const memberMeta = member.email && member.email !== memberName ? member.email : (member.userId && member.userId !== memberName ? member.userId : member.status);
                    return (
                      <div key={member.id} className="studio-member-row">
                        <i>{memberName.slice(0, 1).toUpperCase()}</i>
                        <div>
                          <b>{memberName}</b>
                          <span>{memberMeta}</span>
                        </div>
                        {canManageMembers ? (
                          <>
                            <select value={member.role} disabled={member.role === 'OWNER'} onChange={(event) => selectedWorkspace && void onUpdateMemberRole(selectedWorkspace.id, member.id, event.target.value as WorkspaceRole)}>
                              <option value="OWNER">Owner</option>
                              <option value="ADMIN">Admin</option>
                              <option value="EDITOR">Editor</option>
                              <option value="VIEWER">Viewer</option>
                            </select>
                            <button disabled={member.role === 'OWNER'} onClick={() => selectedWorkspace && void onRemoveMember(selectedWorkspace.id, member.id)}>移除</button>
                          </>
                        ) : (
                          <span className="studio-member-role-pill">{member.role}</span>
                        )}
                      </div>
                    );
                  })}
                </section>
              </div>
            ) : homeUtilityDialog === 'share' ? (
              <div className="studio-create-grid">
                <button disabled={!selectedWorkspace || !canManageMembers} onClick={() => selectedWorkspace && void onCreateShareLink(selectedWorkspace.id, {
                  scope: detailTarget.type === 'Workspace' ? 'WORKSPACE' : detailTarget.type === 'Folder' ? 'FOLDER' : 'ACTIVITY',
                  folderId: detailTarget.type === 'Folder' ? detailTarget.id : selectedFolder?.id,
                  activityId: detailTarget.type === 'Workflow' ? detailTarget.id : undefined,
                  permission: 'VIEW',
                }).then(() => setOpsMessage('分享链接已创建')).catch((error) => setOpsMessage((error as Error).message))}>创建 View 链接</button>
                <button disabled={!selectedWorkspace || !canManageMembers} onClick={() => selectedWorkspace && void onCreateShareLink(selectedWorkspace.id, {
                  scope: detailTarget.type === 'Workspace' ? 'WORKSPACE' : detailTarget.type === 'Folder' ? 'FOLDER' : 'ACTIVITY',
                  folderId: detailTarget.type === 'Folder' ? detailTarget.id : selectedFolder?.id,
                  activityId: detailTarget.type === 'Workflow' ? detailTarget.id : undefined,
                  permission: 'EDIT',
                }).then(() => setOpsMessage('编辑分享链接已创建')).catch((error) => setOpsMessage((error as Error).message))}>创建 Edit 链接</button>
                <div className="studio-confirm-copy">
                  {selectedShareLinks.map((link) => (
                    <span key={link.id}>
                      {link.scope} · {link.permission || 'VIEW'} · {link.enabled ? 'Enabled' : `Disabled${link.disabledReason ? `:${link.disabledReason}` : ''}`} · used {link.accessCount ?? 0}{link.maxAccessCount ? `/${link.maxAccessCount}` : ''} · {shareLinkDisplayId(link)}
                      <button onClick={() => void navigator.clipboard?.writeText(shareUrlForLink(link))}>复制</button>
                      <button disabled={!canManageMembers || !link.enabled} onClick={() => selectedWorkspace && void onDisableShareLink(selectedWorkspace.id, link.id)}>停用</button>
                    </span>
                  ))}
                </div>
              </div>
            ) : (
              <div className="studio-create-grid">
                <button onClick={() => selectedWorkspace && void onRefreshWorkspaceOps(selectedWorkspace.id)}>刷新审计</button>
                <div className="studio-confirm-copy">
                  {selectedAuditLogs.length ? selectedAuditLogs.map((audit) => (
                    <span key={audit.id}>{audit.createdAt} · {audit.actorId || 'system'} · {audit.action} · {audit.targetType}</span>
                  )) : <span>暂无审计记录</span>}
                </div>
              </div>
            )}
          </section>
        </div>
      ) : null}
      {batchAction ? (
        <div className="studio-create-dialog-backdrop" onClick={() => setBatchAction('')}>
          <section className="studio-create-dialog" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>{batchAction === 'move' ? '批量移动' : '批量权限'}</b>
                <span>已选择 {batchActionItems.length} 项</span>
              </div>
              <button onClick={() => setBatchAction('')}>×</button>
            </div>
            <div className="studio-create-grid">
              <div className="studio-selected-preview">
                <b>选中项</b>
                {batchActionItems.slice(0, 6).map((item) => (
                  <span key={`${item.type}-${item.id}`}>
                    <i>{item.type}</i>
                    <strong>{item.name}</strong>
                    <em>{item.visibility}{item.status ? ` · ${item.status}` : ''}</em>
                  </span>
                ))}
                {batchActionItems.length > 6 ? <small>还有 {batchActionItems.length - 6} 项未显示</small> : null}
              </div>
              {batchAction === 'move' ? (
                <label>
                  <span>目标 Folder</span>
                  <select data-testid="batch-target-folder" value={batchActionFolderId} onChange={(event) => setBatchActionFolderId(event.target.value)}>
                    <option value="">选择 folder</option>
                    {selectedWorkspaceFolders.map((folder) => (
                      <option key={folder.id} value={folder.id}>{folder.name}</option>
                    ))}
                  </select>
                </label>
              ) : null}
              {batchAction === 'permission' ? (
                <label>
                  <span>可见性</span>
                  <select data-testid="batch-visibility" value={batchActionVisibility} onChange={(event) => setBatchActionVisibility(event.target.value as WorkspaceDetailTarget['visibility'])}>
                    <option value="Private">Private</option>
                    <option value="Workspace">Workspace</option>
                    <option value="Public">Public</option>
                  </select>
                </label>
              ) : null}
              {batchActionFailures?.length ? (
                <div className="studio-batch-failures" data-testid="batch-failures">
                  <b>失败明细</b>
                  {batchActionFailures.map((failure) => {
                    const source = batchActionItems.find((item) => item.id === failure.id && item.type.toUpperCase() === failure.type.toUpperCase());
                    return (
                      <span key={`${failure.type}-${failure.id}`}>
                        <i>{source?.type ?? failure.type}</i>
                        <strong>{source?.name ?? failure.id}</strong>
                        <em>{failure.error || '处理失败'}</em>
                      </span>
                    );
                  })}
                </div>
              ) : null}
              <div className="studio-dialog-actions">
                <button className="dialog-secondary" type="button" onClick={() => setBatchAction('')}>取消</button>
                <button className="dialog-primary" data-testid="batch-confirm" type="button" onClick={submitBatchAction} disabled={batchActionBusy}>{batchActionBusy ? '处理中...' : '确认'}</button>
              </div>
            </div>
            {batchActionError ? <p>{batchActionError}</p> : null}
          </section>
        </div>
      ) : null}
      {showWorkspaceDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建工作区</b>
                <span>用于组织文件夹和流程。</span>
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
              <button onClick={submitWorkspace} disabled={creatingWorkspace}>{creatingWorkspace ? '创建中...' : '创建工作区'}</button>
            </div>
            {workspaceError ? <p>{workspaceError}</p> : null}
          </section>
        </div>
      ) : null}
      {showFolderDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建文件夹</b>
                <span>{selectedWorkspace ? `创建到 ${selectedWorkspace.name}` : '先选择工作区。'}</span>
              </div>
              <button onClick={() => setShowFolderDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input data-testid="folder-name-input" value={folderName} placeholder="增长活动" onChange={(event) => setFolderName(event.target.value)} />
              </label>
              <button data-testid="folder-create-confirm" onClick={submitFolder} disabled={creatingFolder || !selectedWorkspace || !canManageFolders}>{creatingFolder ? '创建中...' : '创建文件夹'}</button>
            </div>
            {folderError ? <p>{folderError}</p> : null}
          </section>
        </div>
      ) : null}
      {showCreateDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()}>
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
                <input data-testid="workflow-name-input" value={createName} placeholder="增长活动" onChange={(event) => setCreateName(event.target.value)} />
              </label>
              <label>
                <span>流程编码</span>
                <input data-testid="workflow-code-input" value={createCode} placeholder={suggestedCode} onChange={(event) => setCreateCode(event.target.value)} />
              </label>
              <label>
                <span>租户</span>
                <input value={createTenant} placeholder="site_001" onChange={(event) => setCreateTenant(event.target.value)} />
              </label>
              <label>
                <span>工作区</span>
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
                <span>文件夹</span>
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
              <button data-testid="workflow-create-confirm" onClick={submitCreate} disabled={creating || !canEditWorkspace}>{creating ? '创建中...' : '创建并编排'}</button>
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
  const [rightPanelWidth, setRightPanelWidth] = useState(loadRightPanelWidth);
  const [validatingWorkflow, setValidatingWorkflow] = useState(false);
  const [workflowValidation, setWorkflowValidation] = useState<WorkflowValidationResult | null>(null);
  const [nodeConfigSchemas, setNodeConfigSchemas] = useState<WorkflowNodeConfigSchema[]>([]);
  const [workflowTemplates, setWorkflowTemplates] = useState<WorkflowDraftTemplateSummary[]>([]);
  const [workflowNodePresets, setWorkflowNodePresets] = useState<WorkflowNodePresetSummary[]>([]);
  const [canvasResources, setCanvasResources] = useState<ResourceSummary[]>([]);
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
  const [workspaceActivityGrants, setWorkspaceActivityGrants] = useState<Record<string, WorkspaceActivityGrantSummary[]>>({});
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
  const [unsavedDraftNavigation, setUnsavedDraftNavigation] = useState<UnsavedDraftNavigation | null>(null);
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
    offlineTrace: false,
    versionId: '',
    executions: [],
    pageLimit: 20,
    pageOffset: 0,
    hasMore: false,
    nextOffset: 0,
    selectedExecutionId: '',
    nodeExecutions: [],
    nodeSummaries: [],
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
    openApiLoading: false,
    template: 'deposit_success',
    payloadText: '',
    startNodeId: '',
    scheduleType: 'SCHEDULE_TICK',
    scheduleAt: '',
    scheduleTimezone: 'UTC',
    lastResult: undefined,
    message: '',
  });
  const [debugPayloadTemplates, setDebugPayloadTemplates] = useState<DebugPayloadTemplate[]>(() => loadDebugPayloadTemplates());
  const [homeWorkspaceId, setHomeWorkspaceId] = useState('');
  const [homeFolderId, setHomeFolderId] = useState('');
  const [workspaceDataReady, setWorkspaceDataReady] = useState(false);

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
  useEffect(() => {
    const tenantId = activities.find((activity) => activity.id === activityId)?.tenantId || activeWorkspace?.tenantId || 'site_001';
    fetchJSON<{ items: ResourceSummary[] }>(`/admin/v1/resources?tenantId=${encodeURIComponent(tenantId)}&limit=500`)
      .then((body) => setCanvasResources(body.items ?? []))
      .catch(() => setCanvasResources([]));
  }, [activityId, activeWorkspace?.tenantId, activities]);
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
        setWorkspaceDataReady(true);
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
        setWorkspaceDataReady(true);
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
        const shouldUseTemplateLayout = /资格校验|风控/.test(activity?.activityName ?? '') && next.nodes.some((node) => node.type === 'activity' && node.data.id === 'qualify');
        setNodes(shouldUseTemplateLayout ? layoutFlowNodes(next.nodes) : next.nodes);
        setEdges(next.edges);
        setWorkflowStatus(next.nodes.length === 0 && next.edges.length === 0 ? '空 draft 已保存，等待添加节点' : '已加载草稿');
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
          setWorkflowStatus('未找到 draft，已进入待保存空画布');
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

  const schemaCatalog = useMemo(() => catalogFromSchemas(nodeConfigSchemas), [nodeConfigSchemas]);
  const filteredCatalog = useMemo(() => {
    const keyword = nodeSearch.trim().toLowerCase();
    if (!keyword) return schemaCatalog;
    return schemaCatalog
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => (
          item.title.toLowerCase().includes(keyword)
          || item.code.toLowerCase().includes(keyword)
          || item.desc.toLowerCase().includes(keyword)
        )),
      }))
      .filter((group) => group.items.length > 0);
  }, [nodeSearch, schemaCatalog]);

  const closeFloatingUi = useCallback(() => {
    setContextMenu(null);
    setNodePicker(null);
  }, []);

  const clearCanvasSelection = useCallback(() => {
    closeFloatingUi();
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setNodes((items) => items.map((node) => (node.selected ? { ...node, selected: false } : node)));
    setEdges((items) => items.map((edgeItem) => (edgeItem.selected ? { ...edgeItem, selected: false } : edgeItem)));
  }, [closeFloatingUi, setEdges, setNodes]);

  const prepareGlobalPanelOpen = useCallback(() => {
    clearCanvasSelection();
    setDebugPanel((state) => (state.open ? { ...state, open: false } : state));
  }, [clearCanvasSelection]);

  const closeGlobalPanelsForNodeInspect = useCallback(() => {
    setShowChecklist(false);
    setRunHistory((state) => (state.open ? { ...state, open: false } : state));
    setDebugPanel((state) => (state.open ? { ...state, open: false } : state));
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
    const id = uniqueFlowNodeId(def.id, nodes);
    setNodes((items) => items.concat(flowNode(def.id, x, y, { id })));
    if (nodePicker?.sourceNodeId) {
      const newEdge = edge(nodePicker.sourceNodeId, id, nodePicker.sourceLabel ?? '', nodePicker.sourceHandle);
      setEdges((items) => items.concat(newEdge));
      setSelectedEdgeIds([newEdge.id]);
      setSelectedNodeIds([]);
    }
    setLastAdded(def.title);
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, nodePicker, nodes, setEdges, setNodes]);

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
    const id = uniqueFlowNodeId('note', nodes);
    setNodes((items) => items.concat({
      id,
      type: 'note',
      position: { x, y },
      data: { text: '补充活动流程说明、配置约束或异常处理备注。' },
    } satisfies NoteNode));
    setLastAdded('备注');
    closeFloatingUi();
  }, [closeFloatingUi, invalidateWorkflowValidation, nodes, setNodes]);

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
    const duplicateBaseId = current.type === 'activity' ? (current.data.id || current.id) : (current.type || current.id || 'node');
    const duplicate = {
      ...current,
      id: uniqueFlowNodeId(duplicateBaseId, nodes),
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
    const groupId = uniqueFlowNodeId('group', nodes);
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
    const validationItems = Array.isArray(workflowValidation?.items) ? workflowValidation.items : [];
    validationItems.forEach((item) => {
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

  const handleFlowInit = useCallback((instance: ReactFlowInstance<FlowNode, ActivityEdge>) => {
    setReactFlowInstance(instance);
    window.setTimeout(() => {
      instance.setViewport({ x: 24, y: 36, zoom: 1 }, { duration: 0 });
      setZoomPercent(100);
    }, 120);
  }, []);

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
        const failed = workflowValidationFailureResult('保存草稿失败，无法执行发布检查。请确认当前账号有编辑权限。', 'WORKFLOW_DRAFT_SAVE_FAILED');
        setWorkflowValidation(failed);
        setWorkflowStatus('检查失败：保存草稿失败');
        return failed;
      }
      const response = await fetchJSON<WorkflowValidationResult | null>(`/admin/v1/activities/${activityId}/workflow/draft/validate`, {
        method: 'POST',
      });
      const result = normalizeWorkflowValidationResult(response);
      if (!result) {
        const failed = workflowValidationFailureResult('检查接口没有返回 validation 结果。', 'WORKFLOW_VALIDATION_EMPTY');
        setWorkflowValidation(failed);
        setWorkflowStatus('检查失败：接口返回为空');
        return failed;
      }
      setWorkflowValidation(result);
      if (result.valid) {
        setWorkflowStatus('发布检查通过');
      } else {
        setWorkflowStatus(`发布检查未通过：${result.summary.blockCount} 个阻塞项`);
      }
      return result;
    } catch (error) {
      const message = isAPIError(error) && error.status === 403
        ? '无权执行发布检查。请确认当前账号拥有该 workspace / workflow 的编辑或发布权限。'
        : `检查接口失败：${(error as Error).message}`;
      const failed = workflowValidationFailureResult(message, isAPIError(error) ? `WORKFLOW_VALIDATION_HTTP_${error.status}` : 'WORKFLOW_VALIDATION_REQUEST_FAILED');
      setWorkflowValidation(failed);
      setWorkflowStatus(`检查失败：${message}`);
      return failed;
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

  const draftHasUnsavedChanges = draftDirty || draftSaveState === 'dirty' || draftSaveState === 'saving' || draftSaveState === 'failed';

  const executeActivityNavigation = useCallback((nextActivityId: string) => {
    setActivityId(nextActivityId);
    setHomeWorkspaceId('');
    setHomeFolderId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setWorkflowValidation(null);
    setContextMenu(null);
    setNodePicker(null);
    setRunHistory((state) => ({ ...state, open: false, selectedExecutionId: '', nodeExecutions: [], nodeSummaries: [], selectedNodeExecutionId: '', replayStep: 0, compareExecutionId: '', compareNodeExecutions: [], compareResult: undefined, compareLoading: false }));
    replaceUrlParams({ activityId: nextActivityId, executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
    setWorkflowStatus('切换活动中...');
  }, []);

  const executeWorkspaceNavigation = useCallback((workspaceId: string, folderId = '') => {
    setHomeWorkspaceId(workspaceId);
    setHomeFolderId(folderId);
    setActivityId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setContextMenu(null);
    setNodePicker(null);
    window.history.replaceState(null, '', window.location.pathname);
    setWorkflowStatus(folderId ? '已返回 folder 视图' : '已返回 workspace 视图');
  }, []);

  const changeActivity = useCallback((nextActivityId: string) => {
    if (draftHasUnsavedChanges) {
      setUnsavedDraftNavigation({ kind: 'activity', activityId: nextActivityId });
      return;
    }
    executeActivityNavigation(nextActivityId);
  }, [draftHasUnsavedChanges, executeActivityNavigation]);

  const navigateWorkspaceHome = useCallback((workspaceId: string, folderId = '') => {
    if (draftHasUnsavedChanges) {
      setUnsavedDraftNavigation({ kind: 'workspace', workspaceId, folderId });
      return;
    }
    executeWorkspaceNavigation(workspaceId, folderId);
  }, [draftHasUnsavedChanges, executeWorkspaceNavigation]);

  const confirmUnsavedDraftNavigation = useCallback(() => {
    if (!unsavedDraftNavigation) return;
    const navigation = unsavedDraftNavigation;
    setUnsavedDraftNavigation(null);
    if (navigation.kind === 'activity') {
      executeActivityNavigation(navigation.activityId);
      return;
    }
    executeWorkspaceNavigation(navigation.workspaceId, navigation.folderId);
  }, [executeActivityNavigation, executeWorkspaceNavigation, unsavedDraftNavigation]);

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

  const deleteWorkspaceFolder = useCallback(async (workspaceId: string, folderId: string) => {
    const deleted = await fetchJSON<WorkspaceFolderSummary>(`/admin/v1/workspaces/${workspaceId}/folders/${folderId}`, { method: 'DELETE' });
    setWorkspaceFolders((items) => items.map((item) => (item.id === deleted.id ? deleted : item)).filter((item) => item.status !== 'DELETED'));
    await refreshWorkspaceOps(workspaceId);
    return deleted;
  }, [refreshWorkspaceOps]);

  const restoreWorkspaceFolder = useCallback(async (workspaceId: string, folderId: string) => {
    const restored = await fetchJSON<WorkspaceFolderSummary>(`/admin/v1/workspaces/${workspaceId}/folders/${folderId}/restore`, { method: 'POST' });
    setWorkspaceFolders((items) => [restored, ...items.filter((item) => item.id !== restored.id)]);
    await refreshWorkspaceOps(workspaceId);
    return restored;
  }, [refreshWorkspaceOps]);

  const updateStudioActivity = useCallback(async (nextActivityId: string, patch: { activityName?: string; workspaceId?: string; folderId?: string; status?: string; visibility?: string }) => {
    const current = activities.find((activity) => activity.id === nextActivityId);
    if (!current) throw new Error('workflow 不存在');
    const updated = await fetchJSON<ActivitySummary>(`/admin/v1/activities/${nextActivityId}`, {
      method: 'PUT',
      body: JSON.stringify({
        operator: currentActorId,
        reason: 'workspace-home-action',
        ...patch,
      }),
    });
    setActivities((items) => items.map((item) => (item.id === updated.id ? updated : item)));
    setActivityPlacements((items) => ({
      ...items,
      [updated.id]: {
        workspaceId: updated.workspaceId || patch.workspaceId || current.workspaceId || items[updated.id]?.workspaceId || '',
        folderId: updated.folderId || patch.folderId || current.folderId || items[updated.id]?.folderId || '',
      },
    }));
    return updated;
  }, [activities, currentActorId]);

  const updateWorkspace = useCallback(async (workspaceId: string, patch: { name?: string; slug?: string; description?: string; icon?: string; color?: string; visibility?: string; defaultRole?: WorkspaceRole; ownerId?: string; status?: string }) => {
    const updated = await fetchJSON<WorkspaceSummary>(`/admin/v1/workspaces/${workspaceId}`, {
      method: 'PUT',
      body: JSON.stringify({ ...patch, updatedBy: currentActorId }),
    });
    setWorkspaces((items) => items.map((item) => (item.id === updated.id ? updated : item)).filter((item) => item.status !== 'DELETED'));
    await refreshWorkspaceOps(workspaceId);
    return updated;
  }, [currentActorId, refreshWorkspaceOps]);

  const deleteWorkspace = useCallback(async (workspaceId: string) => {
    const deleted = await fetchJSON<WorkspaceSummary>(`/admin/v1/workspaces/${workspaceId}`, { method: 'DELETE' });
    setWorkspaces((items) => items.map((item) => (item.id === deleted.id ? deleted : item)).filter((item) => item.status !== 'DELETED'));
    return deleted;
  }, []);

  const createWorkspaceMember = useCallback(async (workspaceId: string, input: { userId: string; email: string; displayName: string; role: WorkspaceRole }) => {
    await fetchJSON<WorkspaceMemberSummary>(`/admin/v1/workspaces/${workspaceId}/members`, {
      method: 'POST',
      body: JSON.stringify({
        userId: input.userId,
        email: input.email,
        displayName: input.displayName,
        role: input.role,
        status: 'INVITED',
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
    const created = await fetchJSON<WorkspaceShareLinkSummary>(`/admin/v1/workspaces/${workspaceId}/share-links`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
    await refreshWorkspaceOps(workspaceId);
    return created;
  }, [refreshWorkspaceOps]);

  const disableWorkspaceShareLink = useCallback(async (workspaceId: string, linkId: string) => {
    await fetchJSON<WorkspaceShareLinkSummary>(`/admin/v1/workspaces/${workspaceId}/share-links/${linkId}`, { method: 'DELETE' });
    await refreshWorkspaceOps(workspaceId);
  }, [refreshWorkspaceOps]);

  const refreshActivityACL = useCallback(async (activityId: string) => {
    const grants = await fetchJSON<{ items: WorkspaceActivityGrantSummary[] }>(`/admin/v1/activities/${activityId}/acl`).then((body) => body.items ?? []);
    setWorkspaceActivityGrants((items) => ({ ...items, [activityId]: grants }));
    return grants;
  }, []);

  const createActivityACL = useCallback(async (activityId: string, input: { principalType: 'USER' | 'MEMBER' | 'ROLE'; principalId: string; role: Exclude<WorkspaceRole, 'OWNER'> }) => {
    const created = await fetchJSON<WorkspaceActivityGrantSummary>(`/admin/v1/activities/${activityId}/acl`, {
      method: 'POST',
      body: JSON.stringify({
        principalType: input.principalType,
        principalId: input.principalId,
        role: input.role,
        createdBy: currentActorId,
        updatedBy: currentActorId,
      }),
    });
    await refreshActivityACL(activityId);
    if (created.workspaceId) await refreshWorkspaceOps(created.workspaceId);
    return created;
  }, [currentActorId, refreshActivityACL, refreshWorkspaceOps]);

  const deleteActivityACL = useCallback(async (activityId: string, grantId: string) => {
    await fetchJSON<{ deleted: string }>(`/admin/v1/activities/${activityId}/acl/${grantId}`, { method: 'DELETE' });
    await refreshActivityACL(activityId);
    const activity = activities.find((item) => item.id === activityId);
    if (activity?.workspaceId) await refreshWorkspaceOps(activity.workspaceId);
  }, [activities, refreshActivityACL, refreshWorkspaceOps]);

  const createStudioActivity = useCallback(async (input: CreateStudioActivityInput) => {
    const payload = buildStudioActivityPayload(input);
    const created = await fetchJSON<ActivitySummary>('/admin/v1/activities', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    if (input.definition) {
      await fetchJSON(`/admin/v1/activities/${created.id}/workflow/draft`, {
        method: 'PUT',
        body: JSON.stringify({
          operator: currentActorId,
          definition: {
            ...input.definition,
            meta: {
              ...(input.definition.meta ?? {}),
              activityId: created.id,
              workspaceId: created.workspaceId || input.workspaceId,
              folderId: created.folderId || input.folderId,
              importedAt: new Date().toISOString(),
            },
          },
        }),
      });
    }
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
    setWorkflowStatus(input.definition ? '活动已创建，导入 draft 已保存' : '活动已创建，空 draft 已保存');
    setWorkflowVersionLabel('workflow 未发布');
    setDraftDirty(false);
    setDraftSaveState('saved');
    setDraftSaveError('');
    setLastSavedAt(new Date().toISOString());
    draftRevisionRef.current = 0;
    savedDraftRevisionRef.current = 0;
    setIsPublished(false);
    return created;
  }, [currentActorId, setEdges, setNodes]);

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
      setRunHistory((state) => ({ ...state, offlineTrace: false, selectedExecutionId: '', nodeExecutions: [], nodeSummaries: [], selectedNodeExecutionId: '', replayStep: 0, compareExecutionId: '', compareNodeExecutions: [], compareResult: undefined, compareLoading: false }));
      replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
      applyRunOverlay(undefined, []);
      return;
    }
    const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/workflow/executions/${execution.id}/trace`);
    replaceUrlParams({ executionId: execution.id });
    const mergedNodes = trace.nodeExecutions.length
      ? trace.nodeExecutions
      : (await fetchJSON<{ items: WorkflowNodeExecution[] }>(`/admin/v1/workflow/executions/${execution.id}/nodes`)).items;
    const failed = mergedNodes.find((item) => item.status === 'FAILED');
    setRunHistory((state) => ({
      ...state,
      selectedExecutionId: trace.execution.id,
      nodeExecutions: mergedNodes,
      nodeSummaries: trace.nodeSummaries ?? [],
      selectedNodeExecutionId: (failed ?? mergedNodes[0])?.id ?? '',
      compareExecutionId: state.compareExecutionId === trace.execution.id ? '' : state.compareExecutionId,
      compareNodeExecutions: state.compareExecutionId === trace.execution.id ? [] : state.compareNodeExecutions,
      compareResult: state.compareExecutionId === trace.execution.id ? undefined : state.compareResult,
    }));
    if (new URLSearchParams(window.location.search).get('compareExecutionId') === trace.execution.id) {
      replaceUrlParams({ compareExecutionId: undefined });
    }
    applyRunOverlay(trace.execution, mergedNodes);
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
    setRunHistory((state) => ({ ...state, open: true, loading: true, offlineTrace: false, versionId }));
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
        offlineTrace: false,
        versionId,
        executions: executions.items,
        pageLimit: executions.limit ?? pageLimit,
        pageOffset: executions.offset ?? Math.max(0, pageOffset),
        hasMore: Boolean(executions.hasMore),
        nextOffset: executions.nextOffset ?? Math.max(0, pageOffset) + executions.items.length,
        selectedExecutionId: selected?.id ?? '',
        nodeExecutions: selected ? state.nodeExecutions : [],
        nodeSummaries: selected ? state.nodeSummaries : [],
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
    setShowChecklist(false);
    await loadRunHistory(runHistory.versionId, runHistory.selectedExecutionId, runHistory.pageOffset);
  }, [loadRunHistory, runHistory.pageOffset, runHistory.selectedExecutionId, runHistory.versionId]);

  const changeRunHistoryPage = useCallback(async (offset: number) => {
    await loadRunHistory(runHistory.versionId, '', Math.max(0, offset));
  }, [loadRunHistory, runHistory.versionId]);

  const selectRunExecution = useCallback(async (executionId: string) => {
    const execution = runHistory.executions.find((item) => item.id === executionId);
    if (runHistory.offlineTrace) {
      setRunHistory((state) => ({ ...state, selectedExecutionId: executionId, replayStep: 0 }));
      replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
      if (execution) setWorkflowStatus(`离线 Trace：${execution.status} / v${execution.workflowVersion}`);
      return;
    }
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
  }, [loadExecutionNodes, runHistory.executions, runHistory.offlineTrace]);

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
    if (!runHistory.offlineTrace && nodeExecution.input === undefined && nodeExecution.output === undefined && nodeExecution.executionId) {
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
  }, [nodes, reactFlowInstance, runHistory.offlineTrace]);

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
      offlineTrace: false,
      versionId: '',
      executions: [trace.execution, ...state.executions.filter((item) => item.id !== trace.execution.id)],
      selectedExecutionId: next.selectedExecutionId,
      nodeExecutions: next.nodeExecutions,
      nodeSummaries: next.nodeSummaries,
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
    setDebugPanel((state) => ({ ...state, lastResult: debugTraceSummary(trace) }));
    setWorkflowStatus(`Debug run：${trace.execution.status} · DEBUG_DRAFT`);
  }, [applyRunOverlay]);

  const importTraceFile = useCallback(async (file: File) => {
    try {
      const text = await file.text();
      const trace = JSON.parse(text) as WorkflowExecutionTrace;
      const validationError = validateWorkflowTrace(trace);
      if (validationError) throw new Error(validationError);
      const next = traceToRunState(trace);
      setRunHistory((state) => ({
        ...state,
        open: true,
        loading: false,
        offlineTrace: true,
        versionId: '',
        executions: next.executions,
        pageLimit: 1,
        pageOffset: 0,
        hasMore: false,
        nextOffset: 0,
        selectedExecutionId: next.selectedExecutionId,
        nodeExecutions: next.nodeExecutions,
        nodeSummaries: next.nodeSummaries,
        selectedNodeExecutionId: next.selectedNodeExecutionId,
        replayStep: 0,
        compareExecutionId: '',
        compareNodeExecutions: [],
        compareResult: undefined,
        compareLoading: false,
      }));
      replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
      applyRunOverlay(trace.execution, trace.nodeExecutions);
      const failed = trace.nodeExecutions.find((item) => item.status === 'FAILED');
      if (failed) {
        setSelectedNodeIds([failed.nodeId]);
        setSelectedEdgeIds([]);
      }
      setWorkflowStatus(`已导入离线 Trace：${trace.execution.id.slice(0, 12)}`);
    } catch (error) {
      setWorkflowStatus(`导入 Trace 失败：${(error as Error).message}`);
    }
  }, [applyRunOverlay]);

  const saveDebugPayloadTemplate = useCallback((payloadTextInput: string, defaultName: string, successLabel: string) => {
    const payloadText = payloadTextInput.trim();
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
    const name = window.prompt('模板名称', defaultName)?.trim();
    if (!name) return;
    const next = [{
      id: `payload_${Date.now().toString(36)}`,
      name,
      payloadText,
      createdAt: new Date().toISOString(),
    }, ...debugPayloadTemplates.filter((item) => item.name !== name)].slice(0, 30);
    setDebugPayloadTemplates(next);
    saveDebugPayloadTemplates(next);
    setDebugPanel((state) => ({ ...state, payloadText, message: `${successLabel}：${name}` }));
  }, [debugPayloadTemplates]);

  const saveCurrentDebugPayloadTemplate = useCallback(() => {
    saveDebugPayloadTemplate(debugPanel.payloadText, `Debug Payload ${debugPayloadTemplates.length + 1}`, '已保存 payload 模板');
  }, [debugPanel.payloadText, debugPayloadTemplates.length, saveDebugPayloadTemplate]);

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

  const selectedRunNodeExecution = useMemo(() => (
    runHistory.nodeExecutions.find((item) => item.id === runHistory.selectedNodeExecutionId) ?? runHistory.nodeExecutions[0]
  ), [runHistory.nodeExecutions, runHistory.selectedNodeExecutionId]);

  const useSelectedNodePayload = useCallback((source: 'input' | 'output') => {
    const payloadText = nodePayloadText(selectedRunNodeExecution, source);
    if (!payloadText) {
      setDebugPanel((state) => ({ ...state, message: `当前节点没有 ${source.toUpperCase()} payload` }));
      return;
    }
    setDebugPanel((state) => ({
      ...state,
      payloadText,
      message: `已复制节点 ${source.toUpperCase()}：${selectedRunNodeExecution?.nodeTitle || selectedRunNodeExecution?.nodeId || '-'}`,
    }));
  }, [selectedRunNodeExecution]);

  const saveSelectedNodePayloadTemplate = useCallback((source: 'input' | 'output') => {
    const payloadText = nodePayloadText(selectedRunNodeExecution, source);
    if (!payloadText) {
      setDebugPanel((state) => ({ ...state, message: `当前节点没有 ${source.toUpperCase()} payload` }));
      return;
    }
    const nodeLabel = selectedRunNodeExecution?.nodeTitle || selectedRunNodeExecution?.nodeId || 'Node';
    saveDebugPayloadTemplate(payloadText, `${nodeLabel} ${source.toUpperCase()} 模板`, `已保存节点 ${source.toUpperCase()} 模板`);
  }, [saveDebugPayloadTemplate, selectedRunNodeExecution]);

  const debugResumePreview = useMemo(() => buildDebugResumePreview({
    startNodeId: debugPanel.startNodeId.trim() || selectedNodeIds[0] || '',
    sourceExecutionId: runHistory.selectedExecutionId,
    nodeExecutions: runHistory.nodeExecutions,
    flowNodes: nodes,
  }), [debugPanel.startNodeId, nodes, runHistory.nodeExecutions, runHistory.selectedExecutionId, selectedNodeIds]);

  const debugPayloadBody = useCallback((startNodeId = '', payloadTextOverride?: string) => {
    const body: Record<string, unknown> = {
      operator: 'admin-flow',
      template: debugPanel.template,
      startNodeId,
    };
    if (startNodeId && runHistory.selectedExecutionId) {
      body.sourceExecutionId = runHistory.selectedExecutionId;
    }
    const payload = (payloadTextOverride ?? debugPanel.payloadText).trim();
    if (payload) {
      body.payload = JSON.parse(payload);
    }
    return body;
  }, [debugPanel.payloadText, debugPanel.template, runHistory.selectedExecutionId]);

  const runDraftDebug = useCallback(async (startNodeId = '', payloadTextOverride?: string) => {
    if (!activityId) return;
    if (!canEditActiveWorkflow) {
      setDebugPanel((state) => ({ ...state, message: `当前角色 ${activeWorkspaceRole} 不能调试 draft` }));
      return;
    }
    setDebugPanel((state) => ({ ...state, loading: true, message: '', lastResult: undefined }));
    try {
      const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/activities/${activityId}/workflow/debug-run`, {
        method: 'POST',
        body: JSON.stringify(debugPayloadBody(startNodeId, payloadTextOverride)),
      });
      applyDebugTrace(trace);
      const validation = await validateCurrentDraft();
      setDebugPanel((state) => ({ ...state, loading: false, message: `已生成 DEBUG_DRAFT ${trace.execution.id.slice(0, 12)}${validation ? ` · 检查 ${validation.valid ? '通过' : `${validation.summary.blockCount} 个阻塞`}` : ''}` }));
    } catch (error) {
      setDebugPanel((state) => ({ ...state, loading: false, message: `调试失败：${(error as Error).message}`, lastResult: {
        mode: 'DEBUG_DRAFT',
        status: 'FAILED',
        title: 'Draft 调试失败',
        logs: [(error as Error).message],
      } }));
      setWorkflowStatus(`调试失败：${(error as Error).message}`);
    }
  }, [activityId, activeWorkspaceRole, applyDebugTrace, canEditActiveWorkflow, debugPayloadBody, validateCurrentDraft]);

  const buildScheduleDebugPayload = useCallback(() => {
    const activity = activities.find((item) => item.id === activityId);
    const payloadText = scheduleDebugPayloadTemplate(activity, {
      scheduleType: debugPanel.scheduleType,
      scheduleAt: debugPanel.scheduleAt,
      timezone: debugPanel.scheduleTimezone,
    });
    setDebugPanel((state) => ({ ...state, payloadText, message: '已生成定时模拟 payload' }));
    return payloadText;
  }, [activities, activityId, debugPanel.scheduleAt, debugPanel.scheduleTimezone, debugPanel.scheduleType]);

  const runScheduleDraftDebug = useCallback(() => {
    const payloadText = buildScheduleDebugPayload();
    void runDraftDebug('', payloadText);
  }, [buildScheduleDebugPayload, runDraftDebug]);

  const runOpenAPIEventDebug = useCallback(async () => {
    const payloadText = debugPanel.payloadText.trim();
    if (!payloadText) {
      setDebugPanel((state) => ({ ...state, message: '请先填写事件 payload' }));
      return;
    }
    let payload: unknown;
    try {
      payload = JSON.parse(payloadText);
    } catch (error) {
      setDebugPanel((state) => ({ ...state, message: `Payload JSON 无效：${(error as Error).message}` }));
      return;
    }
    const idempotencyKey = workflowEventIdempotencyKey(payload);
    if (!idempotencyKey) {
      setDebugPanel((state) => ({ ...state, message: 'OpenAPI 事件缺少 tenantId/sourceSystem/eventType/biz.bizType/biz.bizId，无法生成 Idempotency-Key' }));
      return;
    }
    setDebugPanel((state) => ({ ...state, openApiLoading: true, message: '', lastResult: undefined }));
    try {
      const response = await fetchJSON<unknown>('/openapi/v1/events', {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body: JSON.stringify(payload),
      });
      setDebugPanel((state) => ({ ...state, openApiLoading: false, message: `OpenAPI 已接收：${idempotencyKey}`, lastResult: openApiDebugSummary(response, idempotencyKey) }));
      await loadRunHistory('', '', 0);
    } catch (error) {
      if (isAPIError(error) && error.body && error.body.duplicate === true) {
        setDebugPanel((state) => ({
          ...state,
          openApiLoading: false,
          message: `OpenAPI 已拦截重复事件：${idempotencyKey}`,
          lastResult: openApiDebugSummary(error.body, idempotencyKey),
        }));
        setWorkflowStatus('OpenAPI 已拦截重复事件');
        return;
      }
      setDebugPanel((state) => ({ ...state, openApiLoading: false, message: `OpenAPI 调用失败：${(error as Error).message}`, lastResult: {
        mode: 'OPENAPI_EVENT',
        status: 'FAILED',
        title: 'OpenAPI 调用失败',
        idempotencyKey,
        logs: [(error as Error).message],
      } }));
      setWorkflowStatus(`OpenAPI 调用失败：${(error as Error).message}`);
    }
  }, [debugPanel.payloadText, loadRunHistory]);

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
    if (runHistory.offlineTrace) {
      const execution = runHistory.executions.find((item) => item.id === runHistory.selectedExecutionId);
      if (!execution) return;
      downloadJSON(`workflow-trace-${runHistory.selectedExecutionId}.json`, {
        schemaVersion: 'workflow.execution.trace.v1',
        exportedAt: new Date().toISOString(),
        execution,
        nodeExecutions: runHistory.nodeExecutions,
        nodeSummaries: runHistory.nodeSummaries,
        routeTrace: workflowRouteTrace(execution),
      } satisfies WorkflowExecutionTrace);
      return;
    }
    try {
      const trace = await fetchJSON<WorkflowExecutionTrace>(`/admin/v1/workflow/executions/${runHistory.selectedExecutionId}/trace`);
      downloadJSON(`workflow-trace-${runHistory.selectedExecutionId}.json`, trace);
    } catch (error) {
      setDebugPanel((state) => ({ ...state, message: runHistoryErrorMessage(error, '导出失败') }));
    }
  }, [runHistory.executions, runHistory.nodeExecutions, runHistory.nodeSummaries, runHistory.offlineTrace, runHistory.selectedExecutionId]);

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

  const updateWorkflowTemplate = useCallback(async (templateId: string, input: {
    versionId?: string;
    name?: string;
    description?: string;
    category?: string;
    workspaceId?: string;
    folderId?: string;
    visibility?: string;
    tags?: string[];
    status?: string;
  }) => {
    setTemplateMessage('正在更新模板...');
    try {
      const query = input.versionId ? `?versionId=${encodeURIComponent(input.versionId)}` : '';
      const updated = await fetchJSON<WorkflowDraftTemplateSummary>(`/admin/v1/workflow/draft-templates/${templateId}${query}`, {
        method: 'PUT',
        body: JSON.stringify({
          ...input,
          description: input.description ?? '',
          category: input.category ?? '',
          workspaceId: input.workspaceId ?? '',
          folderId: input.folderId ?? '',
          visibility: input.visibility ?? 'TEAM',
          operator: currentActorId,
        }),
      });
      await refreshWorkflowTemplates();
      await loadWorkflowTemplateVersions(templateId);
      setTemplateMessage(`模板已更新：${updated.name} v${updated.version}`);
      return updated;
    } catch (error) {
      setTemplateMessage(`更新模板失败：${(error as Error).message}`);
      throw error;
    }
  }, [currentActorId, loadWorkflowTemplateVersions, refreshWorkflowTemplates]);

  const copyWorkflowTemplate = useCallback(async (templateId: string, input: {
    versionId?: string;
    name: string;
    category?: string;
    workspaceId: string;
    folderId?: string;
    visibility: string;
    tags?: string[];
  }) => {
    setTemplateMessage('正在复制模板...');
    try {
      const query = input.versionId ? `?versionId=${encodeURIComponent(input.versionId)}` : '';
      const copied = await fetchJSON<WorkflowDraftTemplateSummary>(`/admin/v1/workflow/draft-templates/${templateId}/copy${query}`, {
        method: 'POST',
        body: JSON.stringify({ ...input, operator: currentActorId }),
      });
      await refreshWorkflowTemplates();
      setTemplateMessage(`模板已复制到 workspace：${copied.name}`);
      return copied;
    } catch (error) {
      setTemplateMessage(`复制模板失败：${(error as Error).message}`);
      throw error;
    }
  }, [currentActorId, refreshWorkflowTemplates]);

  const compareWorkflowTemplateVersions = useCallback(async (templateId: string, leftVersionId: string, rightVersionId: string) => {
    const query = new URLSearchParams({ leftVersionId, rightVersionId });
    return fetchJSON<WorkflowDraftTemplateCompareResult>(`/admin/v1/workflow/draft-templates/${templateId}/compare?${query.toString()}`);
  }, []);
  const beginRightPanelResize = useCallback((event: React.PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = rightPanelWidth;
    let latestWidth = startWidth;
    const updateWidth = (moveEvent: PointerEvent) => {
      latestWidth = clampRightPanelWidth(startWidth + startX - moveEvent.clientX);
      setRightPanelWidth(latestWidth);
    };
    const stopResize = () => {
      document.body.classList.remove('property-panel-resizing');
      document.body.classList.remove('right-panel-resizing');
      window.localStorage.setItem(RIGHT_PANEL_WIDTH_STORAGE_KEY, String(latestWidth));
      window.removeEventListener('pointermove', updateWidth);
      window.removeEventListener('pointerup', stopResize);
    };
    document.body.classList.add('property-panel-resizing');
    document.body.classList.add('right-panel-resizing');
    window.addEventListener('pointermove', updateWidth);
    window.addEventListener('pointerup', stopResize, { once: true });
  }, [rightPanelWidth]);
  const rightPanelStyle = useMemo(() => ({ '--diagnostic-panel-width': `${rightPanelWidth}px` } as React.CSSProperties), [rightPanelWidth]);
  const propertyPanelVisible = Boolean(activityId && selectedCount > 0 && !runHistory.open && !showChecklist && !debugPanel.open);

  return (
    <div className={cx('flow-shell', propertyPanelVisible && 'property-panel-visible')} style={rightPanelStyle}>
      <NodePalette open={activityId ? showPalette : false} catalog={schemaCatalog} selectedTemplate={selectedTemplate} onAddNode={addCatalogNode} onDefault={resetDefault} onSimple={loadSimpleTemplate} />
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
            setShowChecklist(false);
            setRunHistory((state) => (state.open ? { ...state, open: false } : state));
            setDebugPanel((state) => ({
              ...state,
              open: true,
              payloadText: state.payloadText || builtinDebugPayloadTemplate(activity),
              message: state.message || 'DEBUG_DRAFT 和生产 execution 分开记录',
            }));
          }}
          onRunHistory={refreshRunStatus}
          runHistoryOpen={runHistory.open}
          onCloseRunHistory={() => setRunHistory((state) => ({ ...state, open: false }))}
          onGlobalPanelOpen={prepareGlobalPanelOpen}
          onToggleChecklist={() => {
            if (showChecklist) {
              setShowChecklist(false);
              return;
            }
            setRunHistory((state) => ({ ...state, open: false }));
            setShowChecklist(true);
            void validateCurrentDraft();
          }}
          onValidationIssueFocus={focusValidationIssue}
          onWorkflowDiffFocus={focusWorkflowDiffItem}
          onPublish={publishWorkflow}
          onRollbackVersion={rollbackWorkflowVersion}
          onArchiveVersion={archiveWorkflowVersion}
          onRightPanelResizeStart={beginRightPanelResize}
        />
        <section className={cx('canvas-card', !activityId && 'studio-home-card', interactionMode === 'hand' && 'hand-mode')}>
          <ReactFlowProvider>
            {activityId ? (
              <>
            <ReactFlow
              nodes={visibleNodes}
              edges={visibleEdges}
              nodeTypes={nodeTypes}
              onInit={handleFlowInit}
              onMoveEnd={syncZoom}
              onNodesChange={handleNodesChange}
              onEdgesChange={handleEdgesChange}
              onConnect={canEditActiveWorkflow ? onConnect : undefined}
              onPaneClick={() => {
                clearCanvasSelection();
              }}
              onPaneContextMenu={canEditActiveWorkflow ? openPaneMenu : undefined}
              onNodeClick={(event, node) => {
                closeFloatingUi();
                closeGlobalPanelsForNodeInspect();
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
              nodesDraggable={canEditActiveWorkflow && interactionMode === 'pointer'}
              nodesConnectable={canEditActiveWorkflow && interactionMode === 'pointer'}
              edgesReconnectable={canEditActiveWorkflow && interactionMode === 'pointer'}
              elementsSelectable
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
                <span>从标准事件入口开始，或选择一个成熟模板生成 workflow draft。</span>
                <div>
                  <button type="button" onClick={() => addCatalogNode(getDef('event-in'))}>添加标准事件入口</button>
                  <button type="button" onClick={() => {
                    setShowTemplatePanel(true);
                    void refreshWorkflowTemplates();
                  }}>从模板开始</button>
                  <button type="button" onClick={() => importFileInputRef.current?.click()}>导入 JSON</button>
                </div>
                <em>
                  {draftSaveState === 'saved'
                    ? '空 draft 已保存，可以直接开始编排。'
                    : draftSaveState === 'saving'
                      ? '正在保存空 draft。'
                      : draftSaveState === 'failed'
                        ? '空 draft 保存失败，请先重试保存。'
                        : '空画布尚未保存；保存草稿后会记录一个空 draft。'}
                </em>
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
	            {contextMenu && !nodePicker ? (
              <ContextMenu
                menu={contextMenu}
	                onOpenPicker={() => {
	                  if (contextMenu.type !== 'pane') return;
	                  setNodeSearch('');
	                  setContextMenu(null);
	                  setNodePicker({
	                    x: Math.max(12, Math.min(contextMenu.x + 220, window.innerWidth - 340)),
	                    y: Math.max(12, Math.min(contextMenu.y - 8, window.innerHeight - 430)),
	                    flowX: contextMenu.flowX,
	                    flowY: contextMenu.flowY,
	                  });
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
                activityGrantsByActivity={workspaceActivityGrants}
                templates={workflowTemplates}
                currentActorId={currentActorId}
                placements={activityPlacements}
                hiddenTestActivityCount={hiddenTestActivityCount}
                workflowStatus={workflowStatus}
                workspaceDataReady={workspaceDataReady}
                initialWorkspaceId={homeWorkspaceId}
                initialFolderId={homeFolderId}
                onSelectActivity={changeActivity}
                onCreateWorkspace={createStudioWorkspace}
                onCreateActivity={createStudioActivity}
                onCreateFolder={createWorkspaceFolder}
                onUpdateFolder={updateWorkspaceFolder}
                onUpdateActivity={updateStudioActivity}
                onDeleteFolder={deleteWorkspaceFolder}
                onRestoreFolder={restoreWorkspaceFolder}
                onUpdateWorkspace={updateWorkspace}
                onDeleteWorkspace={deleteWorkspace}
                onCreateMember={createWorkspaceMember}
                onUpdateMemberRole={updateWorkspaceMemberRole}
                onRemoveMember={removeWorkspaceMember}
                onCreateShareLink={createWorkspaceShareLink}
                onDisableShareLink={disableWorkspaceShareLink}
                onRefreshActivityACL={refreshActivityACL}
                onCreateActivityACL={createActivityACL}
                onDeleteActivityACL={deleteActivityACL}
                onRefreshWorkspaceOps={refreshWorkspaceOps}
              />
            )}
          </ReactFlowProvider>
        </section>
      </main>
      <RunHistoryPanel
        state={runHistory}
        workflowVersions={workflowVersions}
        onResizeStart={beginRightPanelResize}
        onClose={() => {
          setRunHistory((state) => ({ ...state, open: false }));
          replaceUrlParams({ executionId: undefined, replayStep: undefined, compareExecutionId: undefined });
        }}
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
        onImportTrace={(file) => {
          void importTraceFile(file);
        }}
        onFocusEdge={focusRunEdge}
        onReplayStepChange={updateRunReplayStep}
      />
      <DebugRunPanel
        state={debugPanel}
        templates={debugPayloadTemplates}
        selectedExecutionId={runHistory.selectedExecutionId}
        selectedNodeId={selectedNodeIds[0] ?? ''}
        selectedNodeExecution={selectedRunNodeExecution}
        resumePreview={debugResumePreview}
        onResizeStart={beginRightPanelResize}
        onChange={(patch) => setDebugPanel((state) => ({ ...state, ...patch }))}
        onClose={() => setDebugPanel((state) => ({ ...state, open: false }))}
        onLoadTemplate={loadDebugPayloadTemplate}
        onSaveTemplate={saveCurrentDebugPayloadTemplate}
        onSaveNodePayloadTemplate={saveSelectedNodePayloadTemplate}
        onUseNodePayload={useSelectedNodePayload}
        onUseSelectedExecutionPayload={useSelectedExecutionPayload}
        onRunDraft={() => {
          void runDraftDebug('');
        }}
        onRunOpenApi={() => {
          void runOpenAPIEventDebug();
        }}
        onBuildSchedulePayload={buildScheduleDebugPayload}
        onRunScheduleDraft={runScheduleDraftDebug}
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
        onResizeStart={beginRightPanelResize}
        onClose={() => setShowTemplatePanel(false)}
        onApply={(templateId, input) => {
          void applyWorkflowTemplate(templateId, input);
        }}
        onSave={(input) => {
          void saveWorkflowTemplate(input);
        }}
        onUpdate={(templateId, input) => {
          void updateWorkflowTemplate(templateId, input);
        }}
        onCopy={(templateId, input) => copyWorkflowTemplate(templateId, input)}
        onCompare={compareWorkflowTemplateVersions}
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
      {unsavedDraftNavigation ? (
        <div className="studio-create-dialog-backdrop" data-testid="unsaved-draft-dialog" onClick={() => setUnsavedDraftNavigation(null)}>
          <section className="studio-create-dialog" role="dialog" aria-modal="true" aria-labelledby="unsaved-draft-title" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b id="unsaved-draft-title">未保存的 Workflow Draft</b>
                <span>{unsavedDraftNavigation.kind === 'activity' ? '切换 workflow 前需要确认。' : '离开画布前需要确认。'}</span>
              </div>
              <button type="button" aria-label="关闭未保存提示" onClick={() => setUnsavedDraftNavigation(null)}>×</button>
            </div>
            <div className="studio-create-grid">
              <div className="studio-confirm-copy">
                <b>当前 draft 还有未保存的改动</b>
                <span>{draftSaveState === 'saving' ? '草稿正在保存中，继续操作可能丢失最新修改。' : draftSaveState === 'failed' ? '上次保存失败，继续操作会放弃未保存内容。' : '继续操作会放弃当前未保存内容。'}</span>
              </div>
              <div className="studio-dialog-actions">
                <button className="dialog-secondary" type="button" onClick={() => setUnsavedDraftNavigation(null)}>继续编辑</button>
                <button className="dialog-danger" data-testid="unsaved-draft-confirm" type="button" onClick={confirmUnsavedDraftNavigation}>放弃并继续</button>
              </div>
            </div>
          </section>
        </div>
      ) : null}
      {propertyPanelVisible ? <PropertyPanel nodes={nodes} edges={edges} selectedNodeIds={selectedNodeIds} selectedEdgeIds={selectedEdgeIds} nodeConfigSchemas={nodeConfigSchemas} validationIssues={validationHighlights.nodeIssues.get(selectedNodeIds[0] ?? '') ?? []} edgeValidationIssues={validationHighlights.edgeIssues.get(selectedEdgeIds[0] ?? '') ?? []} resourceOptions={canvasResources} onNodeConfigChange={updateNodeConfig} onEdgeRouteChange={updateEdgeRoute} onLocateNode={(nodeId) => focusNodeById(nodeId, '已定位变量来源')} onDeleteNodes={deleteNodes} onDeleteEdges={deleteEdges} panelWidth={rightPanelWidth} onResizeStart={beginRightPanelResize} /> : null}
    </div>
  );
}
