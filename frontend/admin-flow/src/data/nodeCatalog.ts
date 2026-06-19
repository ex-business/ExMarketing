import type { ActivityNodeData, CatalogGroup, NodeKind, WorkflowNodeConfigSchema } from '../types';

const CATEGORY_LABELS: Record<string, string> = {
  EVENT_INPUT: 'EVENT_INPUT 事件输入',
  CORE: 'CORE 核心链路',
  STORAGE: 'STORAGE 存储节点',
  FACT_QUERY: 'FACT_QUERY 事实查询',
  ADVANCED: 'ADVANCED 高级集成',
  OPS: 'OPS 运维节点',
};

const CATEGORY_ORDER = ['EVENT_INPUT', 'CORE', 'STORAGE', 'FACT_QUERY', 'ADVANCED', 'OPS'];

const NODE_ORDER = [
  'event-in',
  'http-event',
  'webhook-event',
  'mq-event',
  'schedule-event',
  'state-event',
  'manual-event',
  'inbox',
  'normalize',
  'match',
  'fact-query',
  'context-query',
  'local-cache-query',
  'http-query',
  'redis-query',
  'grpc-query',
  'plugin-query',
  'custom-query',
  'lua-query',
  'facts-compose',
  'facts-normalize',
  'activity-state',
  'state-store',
  'quota-check',
  'qualify',
  'candidate-reward',
  'combine',
  'split',
  'reward',
  'limit',
  'quota',
  'budget-reserve',
  'participation',
  'grant-order',
  'outbox',
  'condition',
  'switch',
  'code',
  'decimal',
  'approval-gate',
  'random-pick',
  'plugin-execute',
  'ab-bucket',
  'risk-score',
  'settlement-window',
  'manual-adjust',
  'http-call',
  'mq-publish',
  'webhook',
  'callback',
  'compensation',
  'audit',
  'retry',
  'dlq',
  'reward-reconcile',
  'reconcile',
  'alert',
];

const NODE_KIND_BY_TYPE: Record<string, NodeKind> = {
  'event-in': 'entry',
  'http-event': 'query',
  'webhook-event': 'query',
  'mq-event': 'query',
  'schedule-event': 'query',
  'state-event': 'query',
  'manual-event': 'query',
  inbox: 'store',
  normalize: 'transform',
  match: 'logic',
  'fact-query': 'query',
  'context-query': 'query',
  'local-cache-query': 'query',
  'http-query': 'query',
  'redis-query': 'query',
  'grpc-query': 'query',
  'plugin-query': 'query',
  'custom-query': 'query',
  'lua-query': 'query',
  'facts-compose': 'transform',
  'facts-normalize': 'transform',
  'activity-state': 'store',
  'state-store': 'store',
  'quota-check': 'quota',
  condition: 'logic',
  qualify: 'logic',
  switch: 'logic',
  code: 'transform',
  combine: 'transform',
  split: 'transform',
  'candidate-reward': 'reward',
  decimal: 'transform',
  reward: 'reward',
  limit: 'quota',
  quota: 'quota',
  'budget-reserve': 'quota',
  participation: 'store',
  'grant-order': 'grant',
  outbox: 'dispatch',
  'http-call': 'dispatch',
  'mq-publish': 'dispatch',
  webhook: 'dispatch',
  callback: 'callback',
  'approval-gate': 'logic',
  compensation: 'reliable',
  'random-pick': 'logic',
  'plugin-execute': 'dispatch',
  'ab-bucket': 'logic',
  'risk-score': 'logic',
  'settlement-window': 'logic',
  'manual-adjust': 'callback',
  retry: 'reliable',
  dlq: 'reliable',
  audit: 'audit',
  'reward-reconcile': 'audit',
  reconcile: 'audit',
  alert: 'reliable',
};

const NODE_CODE_BY_TYPE: Record<string, string> = {
  'event-in': 'Event In',
  'http-event': 'HTTP Event',
  'webhook-event': 'Webhook Event',
  'mq-event': 'MQ Event',
  'schedule-event': 'Schedule Event',
  'state-event': 'State Event',
  'manual-event': 'Manual Event',
  inbox: 'Inbox',
  normalize: 'Normalize',
  match: 'MATCH',
  'fact-query': 'Fact Query',
  'context-query': 'Context Query',
  'local-cache-query': 'Local Cache',
  'http-query': 'HTTP Query',
  'redis-query': 'Redis Query',
  'grpc-query': 'gRPC Query',
  'plugin-query': 'Plugin Query',
  'custom-query': 'Custom Query',
  'lua-query': 'Lua Query',
  'facts-compose': 'Facts Compose',
  'facts-normalize': 'Facts Normalize',
  'activity-state': 'Activity State',
  'state-store': 'State Store',
  'quota-check': 'Quota Check',
  condition: 'CONDITION',
  qualify: 'QUALIFY',
  switch: 'SWITCH',
  code: 'CODE',
  combine: 'Combine',
  split: 'Split',
  'candidate-reward': 'Candidate Reward',
  decimal: 'Decimal',
  reward: 'REWARD',
  limit: 'LIMIT',
  quota: 'Quota',
  'budget-reserve': 'Budget Reserve',
  participation: 'Participation',
  'grant-order': 'GrantOrder',
  outbox: 'Outbox',
  'http-call': 'HTTP Call',
  'mq-publish': 'MQ Publish',
  webhook: 'Webhook',
  callback: 'Callback',
  'approval-gate': 'Approval Gate',
  compensation: 'Compensation',
  'random-pick': 'Random Pick',
  'plugin-execute': 'Plugin Execute',
  'ab-bucket': 'AB Bucket',
  'risk-score': 'Risk Score',
  'settlement-window': 'Settlement Window',
  'manual-adjust': 'Manual Adjust',
  retry: 'Retry',
  dlq: 'DLQ',
  audit: 'Audit',
  'reward-reconcile': 'Reward Reconcile',
  reconcile: 'Reconcile',
  alert: 'Alert',
};

function nodeDef(id: string, category: ActivityNodeData['category'], code: string, title: string, kind: ActivityNodeData['kind'], desc: string, inputs: string[], outputs: string[]): ActivityNodeData {
  return { id, category, code, title, kind, desc, inputs, outputs };
}

export const nodeCatalog: CatalogGroup[] = [
  {
    category: 'EVENT_INPUT 事件输入',
    items: [
      nodeDef('event-in', 'EVENT_INPUT', 'Event In', '标准事件入口', 'entry', '接收已标准化事件，校验 eventType、sourceSystem、tenant 后进入流程', [], ['raw_event', 'eventId', 'eventType', 'tenantId']),
      nodeDef('http-event', 'EVENT_INPUT', 'HTTP Event', 'HTTP 接口事件', 'query', '外部系统通过 OpenAPI 主动上报标准事件；真实接收由 HTTP handler 完成', [], ['standard_event']),
      nodeDef('webhook-event', 'EVENT_INPUT', 'Webhook Event', 'Webhook 映射事件', 'query', '接收第三方 payload，签名校验后映射为标准事件', [], ['standard_event']),
      nodeDef('mq-event', 'EVENT_INPUT', 'MQ Event', 'MQ Topic 事件', 'query', '监听 topic/queue 并映射消息为标准事件；入站 consumer 待实现', [], ['standard_event']),
      nodeDef('schedule-event', 'EVENT_INPUT', 'Schedule Event', '定时事件', 'query', '按 cron 或延迟任务生成标准事件；调度入口待实现', [], ['standard_event']),
      nodeDef('state-event', 'EVENT_INPUT', 'State Event', '状态监听事件', 'query', '监听 Redis、配置、指标或状态条件并生成标准事件；watcher 待实现', [], ['standard_event']),
      nodeDef('manual-event', 'EVENT_INPUT', 'Manual Event', '人工触发事件', 'query', '管理台人工补偿、重放或手动执行标准事件', [], ['standard_event']),
    ],
  },
  {
    category: 'CORE 核心链路',
    items: [
      nodeDef('inbox', 'CORE', 'Inbox', '幂等收件箱', 'store', '写入 marketing_event_inbox，重复事件直接返回已有结果', ['raw_event', 'Idempotency-Key'], ['event_context']),
      nodeDef('normalize', 'CORE', 'Normalize', '事件标准化', 'transform', '字段映射、默认值、时间格式、租户和业务主体规范化', ['raw_event'], ['normalized_event']),
      nodeDef('match', 'CORE', 'MATCH', '活动匹配', 'logic', '按活动状态、时间窗、事件类型、来源系统匹配活动', ['event_context', 'activity_config'], ['matched_activity']),
      nodeDef('quota-check', 'CORE', 'Quota Check', '次数频控校验', 'quota', '用户、活动、租户、IP、设备和业务维度次数/频率限制', ['subject', 'activity_id'], ['quota_check', 'limit_hold']),
      nodeDef('qualify', 'CORE', 'QUALIFY', '资格校验', 'logic', '用户资格、活动门槛、参与范围、黑白名单和风险规则校验', ['event_context', 'matched_activity'], ['PASS', 'BLOCK', 'RISK']),
      nodeDef('candidate-reward', 'CORE', 'Candidate Reward', '候选奖励', 'reward', '生成待发奖励候选，不直接产生资金副作用', ['qualified_context', 'plugin_result'], ['candidate_rewards', 'reward_items']),
      nodeDef('reward', 'CORE', 'REWARD', '奖励计算', 'reward', '支持 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT', ['qualified_context', 'reward_config'], ['reward_items']),
      nodeDef('limit', 'CORE', 'LIMIT', '次数限制', 'quota', '用户、活动、周期、事件维度参与次数限制', ['subject', 'activity_id'], ['limit_hold']),
      nodeDef('quota', 'CORE', 'Quota', '预算库存扣减', 'quota', '预算、库存、次数限制冻结/确认/释放，保证并发原子性', ['reward_items', 'limit_hold'], ['quota_hold']),
      nodeDef('budget-reserve', 'CORE', 'Budget Reserve', '预算库存锁定', 'quota', '发奖前锁定预算、库存或用户额度，避免并发超发', ['candidate_rewards', 'reward_items'], ['budget_reservation', 'quota_hold']),
      nodeDef('participation', 'CORE', 'Participation', '参与记录', 'store', '写入参与流水，关联事件、活动、奖励、quota 和阶段结果', ['qualified_context', 'quota_hold'], ['participation_id']),
      nodeDef('grant-order', 'CORE', 'GrantOrder', '发奖订单', 'grant', '生成发奖订单，状态流转 CREATED / SENT / SUCCESS / FAILED', ['participation_id', 'reward_items'], ['grant_order']),
      nodeDef('outbox', 'CORE', 'Outbox', '消息发件箱', 'dispatch', '写入 outbox，后续 worker 发送 HTTP / MQ / Webhook', ['grant_order'], ['HTTP', 'MQ', 'Webhook', 'Failed']),
    ],
  },
  {
    category: 'STORAGE 存储节点',
    items: [
      nodeDef('activity-state', 'STORAGE', 'Activity State', '活动状态', 'store', '读取或更新用户活动状态、任务进度、签到状态和插件状态', ['event_context', 'state_patch'], ['activity_state', 'state_version']),
      nodeDef('state-store', 'STORAGE', 'State Store', '通用存储', 'store', '统一读写内存、工作流上下文、数据库和外部 Provider 存储', ['value', 'state_patch'], ['stored_value', 'storage_entry', 'storage_version']),
    ],
  },
  {
    category: 'FACT_QUERY 事实查询',
    items: [
      nodeDef('fact-query', 'FACT_QUERY', 'Fact Query', '事实查询', 'query', '组合大节点：按 providerChain 查询上下文、缓存、HTTP 等来源', ['subject', 'fact_keys'], ['facts']),
      nodeDef('context-query', 'FACT_QUERY', 'Context Query', '上下文查询', 'query', '从事件 payload、输入、上游输出或流程变量读取 facts', ['subject'], ['facts']),
      nodeDef('local-cache-query', 'FACT_QUERY', 'Local Cache', '本地缓存查询', 'query', '读取 ExMarketing 工作流变量或营销事实缓存', ['subject'], ['facts']),
      nodeDef('http-query', 'FACT_QUERY', 'HTTP Query', 'HTTP 查询', 'query', '通过 HTTP Fact Provider 查询用户中心、钱包或风控事实', ['subject'], ['facts']),
      nodeDef('redis-query', 'FACT_QUERY', 'Redis Query', 'Redis 查询', 'query', '读取稳定契约的营销 Redis 事实缓存，需要 adapter', ['subject'], ['facts']),
      nodeDef('grpc-query', 'FACT_QUERY', 'gRPC Query', 'gRPC 查询', 'query', '通过 gRPC Fact Provider 查询内部服务事实，需要 adapter', ['subject'], ['facts']),
      nodeDef('plugin-query', 'FACT_QUERY', 'Plugin Query', '插件查询', 'query', '通过业务插件生成 facts，需要插件 adapter', ['subject'], ['facts']),
      nodeDef('custom-query', 'FACT_QUERY', 'Custom Query', '自定义查询', 'query', '自定义协议或专有 SDK 的事实查询入口，需要 adapter', ['subject'], ['facts']),
      nodeDef('lua-query', 'FACT_QUERY', 'Lua Query', 'Lua 查询', 'query', '用受控 Lua 从当前上下文生成 facts，不允许网络或文件访问', ['subject'], ['facts']),
      nodeDef('facts-compose', 'FACT_QUERY', 'Facts Compose', '事实映射合并', 'transform', '把多个事实查询节点输出合并为统一 facts，供资格校验节点消费', ['facts', 'USER_PROFILE', 'KYC_PROFILE', 'RISK_PROFILE', 'DEPOSIT_SUMMARY'], ['facts']),
      nodeDef('facts-normalize', 'FACT_QUERY', 'Facts Normalize', '事实标准化', 'transform', '把统一 facts 转成 standard_context.user/kyc/risk/checkin，后续节点只读标准字段', ['facts', 'event_context'], ['standard_context', 'user', 'kyc', 'risk', 'checkin', 'facts']),
    ],
  },
  {
    category: 'ADVANCED 高级集成',
    items: [
      nodeDef('condition', 'ADVANCED', 'CONDITION', '条件判断', 'logic', '字段路径读取、类型转换、缺失字段处理和错误原因输出', ['fact', 'condition_expr'], ['IF', 'ELSE']),
      nodeDef('switch', 'ADVANCED', 'SWITCH', '条件分支', 'logic', '基于阶段结果选择后续路径，支持 PASS / BLOCK / SKIP', ['stage_result'], ['PASS', 'BLOCK', 'SKIP']),
      nodeDef('code', 'ADVANCED', 'CODE', '代码节点', 'transform', '用受控 LUA 或表达式处理上下文，输出中间变量给下游节点', ['context', 'input'], ['code_result']),
      nodeDef('combine', 'ADVANCED', 'Combine', '组合节点', 'transform', '用 Lua 把多个上游输入组合成一个结构化结果', ['items', 'context'], ['combined_result']),
      nodeDef('split', 'ADVANCED', 'Split', '拆分节点', 'transform', '用 Lua 把结构化输入拆分成数组或多个输出分片', ['payload', 'items'], ['split_items', 'split_result']),
      nodeDef('decimal', 'ADVANCED', 'Decimal', '精度处理', 'transform', '把金额解析为固定 4 位小数字符串，供金额链路显式复用', ['amount_expr'], ['decimal_amount']),
      nodeDef('approval-gate', 'ADVANCED', 'Approval Gate', '审批网关', 'logic', '按金额、风险或策略决定自动通过、阻断或进入人工审批', ['candidate_rewards', 'risk_result'], ['approval_decision']),
      nodeDef('random-pick', 'ADVANCED', 'Random Pick', '随机抽取', 'logic', '可审计的权重随机、奖池抽取和分桶随机', ['candidate_rewards'], ['random_pick', 'selected_rewards']),
      nodeDef('plugin-execute', 'ADVANCED', 'Plugin Execute', '插件执行', 'dispatch', '调用签到、抽奖、红包等活动插件动作', ['event_context', 'activity_state'], ['plugin_result', 'candidate_rewards']),
      nodeDef('ab-bucket', 'ADVANCED', 'AB Bucket', 'AB 分桶', 'logic', '按稳定主体分配实验桶或运营策略桶', ['subject'], ['ab_bucket']),
      nodeDef('risk-score', 'ADVANCED', 'Risk Score', '风险评分', 'logic', '根据事实、规则或外部风控结果输出风险评分和标签', ['facts'], ['risk_result']),
      nodeDef('settlement-window', 'ADVANCED', 'Settlement Window', '结算窗口', 'logic', '限制发奖、对账或结算只能在指定窗口内执行', ['event_context'], ['settlement_window']),
      nodeDef('manual-adjust', 'ADVANCED', 'Manual Adjust', '人工调整', 'callback', '记录人工补偿、修正、加减奖励或状态修复意图', ['operator'], ['manual_adjustment']),
      nodeDef('http-call', 'ADVANCED', 'HTTP Call', 'HTTP 发奖', 'dispatch', '调用外部发奖接口，记录响应、耗时和错误码', ['outbox_message'], ['http_result']),
      nodeDef('mq-publish', 'ADVANCED', 'MQ Publish', 'MQ 发奖', 'dispatch', '发布发奖指令到消息队列，支持 key、headers 和 traceId', ['outbox_message'], ['mq_result']),
      nodeDef('webhook', 'ADVANCED', 'Webhook', 'Webhook 通知', 'dispatch', '向业务系统推送发奖、失败、参与和风险结果', ['stage_result'], ['webhook_result']),
      nodeDef('callback', 'ADVANCED', 'Callback', '结果回调', 'callback', '处理成功/失败回调，保证 grant、quota、participation 状态一致', ['provider_callback'], ['callback_result']),
      nodeDef('compensation', 'ADVANCED', 'Compensation', '业务补偿', 'reliable', '生成发奖、预算、回调、通知失败后的补偿计划', ['stage_result'], ['compensation_plan']),
    ],
  },
  {
    category: 'OPS 运维节点',
    items: [
      nodeDef('retry', 'OPS', 'Retry', '失败重试', 'reliable', '指数退避、最大次数、可重入执行和最后错误保存', ['failed_message'], ['retry_message']),
      nodeDef('dlq', 'OPS', 'DLQ', '死信队列', 'reliable', '超过重试次数进入 DLQ，支持人工修复和重新投递', ['failed_message'], ['dlq_record']),
      nodeDef('audit', 'OPS', 'Audit', '审计记录', 'audit', '记录事件、规则、奖励、quota、outbox、callback 全链路证据', ['stage_result'], ['audit_log']),
      nodeDef('reward-reconcile', 'OPS', 'Reward Reconcile', '奖励对账', 'audit', '对发奖订单、预算锁定、回调和 outbox 做奖励链路一致性校验', ['date_range'], ['reward_reconcile_report']),
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
    .map((category) => ({
      category,
      items: (grouped.get(category) ?? []).sort((a, b) => {
        const left = NODE_ORDER.indexOf(a.id);
        const right = NODE_ORDER.indexOf(b.id);
        if (left !== -1 && right !== -1) return left - right;
        if (left !== -1) return -1;
        if (right !== -1) return 1;
        return a.title.localeCompare(b.title);
      }),
    }))
    .filter((group) => group.items.length > 0);
}

export function flatCatalog(): ActivityNodeData[] {
  return nodeCatalog.flatMap((group) => group.items);
}

export function getDef(id: string): ActivityNodeData {
  return flatCatalog().find((item) => item.id === id) ?? flatCatalog()[0];
}
