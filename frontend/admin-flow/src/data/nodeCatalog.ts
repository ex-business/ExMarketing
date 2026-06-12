import type { ActivityNodeData, CatalogGroup, NodeKind, WorkflowNodeConfigSchema } from '../types';

const CATEGORY_LABELS: Record<string, string> = {
  CORE: 'CORE 核心链路',
  ADVANCED: 'ADVANCED 高级集成',
  OPS: 'OPS 运维节点',
};

const CATEGORY_ORDER = ['CORE', 'ADVANCED', 'OPS'];

const NODE_KIND_BY_TYPE: Record<string, NodeKind> = {
  'event-in': 'entry',
  inbox: 'store',
  normalize: 'transform',
  match: 'logic',
  'fact-query': 'query',
  condition: 'logic',
  qualify: 'logic',
  switch: 'logic',
  code: 'transform',
  decimal: 'transform',
  reward: 'reward',
  limit: 'quota',
  quota: 'quota',
  participation: 'store',
  'grant-order': 'grant',
  outbox: 'dispatch',
  'http-call': 'dispatch',
  'mq-publish': 'dispatch',
  webhook: 'dispatch',
  callback: 'callback',
  retry: 'reliable',
  dlq: 'reliable',
  audit: 'audit',
  reconcile: 'audit',
  alert: 'reliable',
};

const NODE_CODE_BY_TYPE: Record<string, string> = {
  'event-in': 'Event In',
  inbox: 'Inbox',
  normalize: 'Normalize',
  match: 'MATCH',
  'fact-query': 'Fact Query',
  condition: 'CONDITION',
  qualify: 'QUALIFY',
  switch: 'SWITCH',
  code: 'CODE',
  decimal: 'Decimal',
  reward: 'REWARD',
  limit: 'LIMIT',
  quota: 'Quota',
  participation: 'Participation',
  'grant-order': 'GrantOrder',
  outbox: 'Outbox',
  'http-call': 'HTTP Call',
  'mq-publish': 'MQ Publish',
  webhook: 'Webhook',
  callback: 'Callback',
  retry: 'Retry',
  dlq: 'DLQ',
  audit: 'Audit',
  reconcile: 'Reconcile',
  alert: 'Alert',
};

function nodeDef(id: string, category: ActivityNodeData['category'], code: string, title: string, kind: ActivityNodeData['kind'], desc: string, inputs: string[], outputs: string[]): ActivityNodeData {
  return { id, category, code, title, kind, desc, inputs, outputs };
}

export const nodeCatalog: CatalogGroup[] = [
  {
    category: 'CORE 核心链路',
    items: [
      nodeDef('event-in', 'CORE', 'Event In', '事件入口', 'entry', 'HTTP / MQ / Webhook 标准事件接入', ['eventId', 'eventType', 'tenantId', 'subject', 'attributes'], ['raw_event']),
      nodeDef('inbox', 'CORE', 'Inbox', '幂等收件箱', 'store', '写入 marketing_event_inbox，重复事件直接返回已有结果', ['raw_event', 'Idempotency-Key'], ['event_context']),
      nodeDef('normalize', 'CORE', 'Normalize', '事件标准化', 'transform', '字段映射、默认值、时间格式、租户和业务主体规范化', ['raw_event'], ['normalized_event']),
      nodeDef('match', 'CORE', 'MATCH', '活动匹配', 'logic', '按活动状态、时间窗、事件类型、来源系统匹配活动', ['event_context', 'activity_config'], ['matched_activity']),
      nodeDef('fact-query', 'CORE', 'Fact Query', '事实查询', 'query', '查询用户、订单、交易、历史参与和外部事实数据', ['subject', 'fact_keys'], ['facts']),
      nodeDef('qualify', 'CORE', 'QUALIFY', '资格校验', 'logic', '用户资格、活动门槛、参与范围、黑白名单和风险规则校验', ['event_context', 'matched_activity'], ['PASS', 'BLOCK', 'RISK']),
      nodeDef('reward', 'CORE', 'REWARD', '奖励计算', 'reward', '支持 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT', ['qualified_context', 'reward_config'], ['reward_items']),
      nodeDef('limit', 'CORE', 'LIMIT', '次数限制', 'quota', '用户、活动、周期、事件维度参与次数限制', ['subject', 'activity_id'], ['limit_hold']),
      nodeDef('quota', 'CORE', 'Quota', '预算库存扣减', 'quota', '预算、库存、次数限制冻结/确认/释放，保证并发原子性', ['reward_items', 'limit_hold'], ['quota_hold']),
      nodeDef('participation', 'CORE', 'Participation', '参与记录', 'store', '写入参与流水，关联事件、活动、奖励、quota 和阶段结果', ['qualified_context', 'quota_hold'], ['participation_id']),
      nodeDef('grant-order', 'CORE', 'GrantOrder', '发奖订单', 'grant', '生成发奖订单，状态流转 CREATED / SENT / SUCCESS / FAILED', ['participation_id', 'reward_items'], ['grant_order']),
      nodeDef('outbox', 'CORE', 'Outbox', '消息发件箱', 'dispatch', '写入 outbox，后续 worker 发送 HTTP / MQ / Webhook', ['grant_order'], ['HTTP', 'MQ', 'Webhook', 'Failed']),
    ],
  },
  {
    category: 'ADVANCED 高级集成',
    items: [
      nodeDef('condition', 'ADVANCED', 'CONDITION', '条件判断', 'logic', '字段路径读取、类型转换、缺失字段处理和错误原因输出', ['fact', 'condition_expr'], ['IF', 'ELSE']),
      nodeDef('switch', 'ADVANCED', 'SWITCH', '条件分支', 'logic', '基于阶段结果选择后续路径，支持 PASS / BLOCK / SKIP', ['stage_result'], ['PASS', 'BLOCK', 'SKIP']),
      nodeDef('code', 'ADVANCED', 'CODE', '代码节点', 'transform', '用受控 LUA 或表达式处理上下文，输出中间变量给下游节点', ['context', 'input'], ['code_result']),
      nodeDef('decimal', 'ADVANCED', 'Decimal', '精度处理', 'transform', '把金额解析为固定 4 位小数字符串，供金额链路显式复用', ['amount_expr'], ['decimal_amount']),
      nodeDef('http-call', 'ADVANCED', 'HTTP Call', 'HTTP 发奖', 'dispatch', '调用外部发奖接口，记录响应、耗时和错误码', ['outbox_message'], ['http_result']),
      nodeDef('mq-publish', 'ADVANCED', 'MQ Publish', 'MQ 发奖', 'dispatch', '发布发奖指令到消息队列，支持 key、headers 和 traceId', ['outbox_message'], ['mq_result']),
      nodeDef('webhook', 'ADVANCED', 'Webhook', 'Webhook 通知', 'dispatch', '向业务系统推送发奖、失败、参与和风险结果', ['stage_result'], ['webhook_result']),
      nodeDef('callback', 'ADVANCED', 'Callback', '结果回调', 'callback', '处理成功/失败回调，保证 grant、quota、participation 状态一致', ['provider_callback'], ['callback_result']),
    ],
  },
  {
    category: 'OPS 运维节点',
    items: [
      nodeDef('retry', 'OPS', 'Retry', '失败重试', 'reliable', '指数退避、最大次数、可重入执行和最后错误保存', ['failed_message'], ['retry_message']),
      nodeDef('dlq', 'OPS', 'DLQ', '死信队列', 'reliable', '超过重试次数进入 DLQ，支持人工修复和重新投递', ['failed_message'], ['dlq_record']),
      nodeDef('audit', 'OPS', 'Audit', '审计记录', 'audit', '记录事件、规则、奖励、quota、outbox、callback 全链路证据', ['stage_result'], ['audit_log']),
      nodeDef('reconcile', 'OPS', 'Reconcile', '对账校验', 'audit', '对 grant、quota、callback、outbox 做一致性扫描和修复建议', ['date_range'], ['reconcile_report']),
      nodeDef('alert', 'OPS', 'Alert', '告警通知', 'reliable', '关键失败、DLQ 增长、回调失败和预算异常告警', ['metric_event'], ['alert_event']),
    ],
  },
];

export function catalogFromSchemas(schemas: WorkflowNodeConfigSchema[]): CatalogGroup[] {
  if (schemas.length === 0) return nodeCatalog;
  const grouped = new Map<string, ActivityNodeData[]>();
  schemas.forEach((schema) => {
    const category = (schema.category || 'CORE').toUpperCase();
    const label = CATEGORY_LABELS[category] ?? category;
    const item: ActivityNodeData = {
      id: schema.nodeType,
      category,
      code: NODE_CODE_BY_TYPE[schema.nodeType] ?? schema.nodeType,
      title: schema.title || schema.nodeType,
      kind: NODE_KIND_BY_TYPE[schema.nodeType] ?? 'logic',
      desc: schema.description ?? '',
      inputs: schema.inputs?.map((input) => input.key) ?? [],
      outputs: schema.outputs?.map((output) => output.key) ?? [],
    };
    grouped.set(label, [...(grouped.get(label) ?? []), item]);
  });
  const labels = [
    ...CATEGORY_ORDER.map((category) => CATEGORY_LABELS[category]),
    ...Array.from(grouped.keys()).filter((label) => !CATEGORY_ORDER.map((category) => CATEGORY_LABELS[category]).includes(label)).sort(),
  ];
  return labels
    .map((category) => ({ category, items: grouped.get(category) ?? [] }))
    .filter((group) => group.items.length > 0);
}

export function flatCatalog(): ActivityNodeData[] {
  return nodeCatalog.flatMap((group) => group.items);
}

export function getDef(id: string): ActivityNodeData {
  return flatCatalog().find((item) => item.id === id) ?? flatCatalog()[0];
}
