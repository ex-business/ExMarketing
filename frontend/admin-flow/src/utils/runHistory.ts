import type {
  WorkflowExecution,
  WorkflowExecutionJSONDiff,
  WorkflowExecutionTrace,
} from '../types';

export type WorkflowRouteTraceItem = {
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

export type JsonDiffRow = WorkflowExecutionJSONDiff;

export type WorkflowErrorClass = {
  code: string;
  category: string;
  tone: 'neutral' | 'warning' | 'danger';
  suggestion: string;
};

export function jsonPreview(value: unknown): string {
  if (value === undefined || value === null || value === '') return '-';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function fallbackCopyText(text: string): boolean {
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

export function jsonValueLabel(value: unknown): string {
  if (value === undefined) return 'undefined';
  if (typeof value === 'string') return JSON.stringify(value);
  if (value === null || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return jsonPreview(value).replace(/\s+/g, ' ').slice(0, 180);
}

export function sideEffectKindLabel(kind: string): string {
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

export function diffJsonValues(current: unknown, baseline: unknown, path = '$'): JsonDiffRow[] {
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

export function elapsedMs(startedAt?: string, finishedAt?: string): number | undefined {
  if (!startedAt || !finishedAt) return undefined;
  const started = new Date(startedAt).getTime();
  const finished = new Date(finishedAt).getTime();
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) return undefined;
  return finished - started;
}

export function timestampMs(value?: string): number | undefined {
  if (!value) return undefined;
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function compactDuration(ms: number | undefined): string {
  if (ms === undefined) return 'running';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`;
}

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

export function classifyWorkflowError(message?: string): WorkflowErrorClass {
  const text = (message || '').trim();
  if (!text) return { code: 'NONE', category: '无错误', tone: 'neutral', suggestion: '当前 execution 或节点没有错误信息。' };
  const upper = text.toUpperCase();
  const codeMatch = upper.match(/\b([A-Z][A-Z0-9_]{2,})\b/);
  const code = codeMatch?.[1] ?? 'UNCLASSIFIED_ERROR';
  const matched = WORKFLOW_ERROR_DICTIONARY.find((item) => item.patterns.some((pattern) => pattern.test(text)));
  if (matched) return { code, category: matched.category, tone: matched.tone, suggestion: matched.suggestion };
  return { code, category: '业务执行错误', tone: 'danger', suggestion: '查看节点 input/output，确认业务 handler 返回值、异常和副作用写入是否符合预期。' };
}

export function workflowRouteTrace(execution?: WorkflowExecution): WorkflowRouteTraceItem[] {
  const output = execution?.output;
  if (!output || typeof output !== 'object') return [];
  const routeTrace = (output as Record<string, unknown>).routeTrace;
  return Array.isArray(routeTrace) ? routeTrace.filter((item): item is WorkflowRouteTraceItem => Boolean(item && typeof item === 'object')) : [];
}

export function workflowAttemptCount(output: unknown): number | undefined {
  if (!output || typeof output !== 'object') return undefined;
  const attempt = (output as Record<string, unknown>).attempt;
  if (typeof attempt === 'number') return attempt;
  if (typeof attempt === 'string') {
    const parsed = Number(attempt);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

export function workflowExecutionModeLabel(execution?: WorkflowExecution): 'DEBUG_DRAFT' | 'PRODUCTION' {
  if (!execution) return 'PRODUCTION';
  if (execution.workflowVersionId?.startsWith('debug-draft')) return 'DEBUG_DRAFT';
  const output = execution.output;
  if (output && typeof output === 'object' && (output as Record<string, unknown>).debug === true) return 'DEBUG_DRAFT';
  return 'PRODUCTION';
}

const WORKFLOW_EXECUTION_STATUSES = new Set(['RUNNING', 'SUCCESS', 'FAILED', 'CANCELED']);
const WORKFLOW_NODE_EXECUTION_STATUSES = new Set(['RUNNING', 'SUCCESS', 'FAILED', 'SKIPPED']);

export function validateWorkflowTrace(trace: WorkflowExecutionTrace): string {
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

export function downloadJSON(filename: string, value: unknown) {
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
