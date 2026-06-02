import type { ActivityNodeData, CatalogGroup } from '../types';

function nodeDef(id: string, code: string, title: string, kind: ActivityNodeData['kind'], desc: string, inputs: string[], outputs: string[]): ActivityNodeData {
  return { id, code, title, kind, desc, inputs, outputs };
}

export const nodeCatalog: CatalogGroup[] = [
  {
    category: '入口',
    items: [
      nodeDef('event-in', 'Event In', '事件入口', 'entry', 'HTTP / MQ / Webhook 标准事件接入', ['eventId', 'eventType', 'tenantId', 'subject', 'attributes'], ['raw_event']),
      nodeDef('inbox', 'Inbox', '幂等收件箱', 'store', '写入 marketing_event_inbox，重复事件直接返回已有结果', ['raw_event', 'Idempotency-Key'], ['event_context']),
      nodeDef('normalize', 'Normalize', '事件标准化', 'transform', '字段映射、默认值、时间格式、租户和业务主体规范化', ['raw_event'], ['normalized_event']),
    ],
  },
  {
    category: '判断与规则',
    items: [
      nodeDef('match', 'MATCH', '活动匹配', 'logic', '按活动状态、时间窗、事件类型、来源系统匹配活动', ['event_context', 'activity_config'], ['matched_activity']),
      nodeDef('condition', 'CONDITION', '条件判断', 'logic', '字段路径读取、类型转换、缺失字段处理和错误原因输出', ['fact', 'condition_expr'], ['IF', 'ELSE']),
      nodeDef('qualify', 'QUALIFY', '资格校验', 'logic', '用户资格、活动门槛、参与范围、黑白名单和风险规则校验', ['event_context', 'matched_activity'], ['PASS', 'BLOCK', 'RISK']),
      nodeDef('switch', 'SWITCH', '条件分支', 'logic', '基于阶段结果选择后续路径，支持 PASS / BLOCK / SKIP', ['stage_result'], ['PASS', 'BLOCK', 'SKIP']),
      nodeDef('fact-query', 'Fact Query', '事实查询', 'query', '查询用户、订单、交易、历史参与和外部事实数据', ['subject', 'fact_keys'], ['facts']),
    ],
  },
  {
    category: '奖励与限制',
    items: [
      nodeDef('reward', 'REWARD', '奖励计算', 'reward', '支持 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT', ['qualified_context', 'reward_config'], ['reward_items']),
      nodeDef('decimal', 'Decimal', '精度处理', 'transform', '金额、比例、阶梯、概率和奖池拆分使用 Decimal 精度', ['amount_expr'], ['decimal_amount']),
      nodeDef('limit', 'LIMIT', '次数限制', 'quota', '用户、活动、周期、事件维度参与次数限制', ['subject', 'activity_id'], ['limit_hold']),
      nodeDef('quota', 'Quota', '预算库存扣减', 'quota', '预算、库存、次数限制冻结/确认/释放，保证并发原子性', ['reward_items', 'limit_hold'], ['quota_hold']),
      nodeDef('participation', 'Participation', '参与记录', 'store', '写入参与流水，关联事件、活动、奖励、quota 和阶段结果', ['qualified_context', 'quota_hold'], ['participation_id']),
    ],
  },
  {
    category: '发奖与回调',
    items: [
      nodeDef('grant-order', 'GrantOrder', '发奖订单', 'grant', '生成发奖订单，状态流转 CREATED / SENT / SUCCESS / FAILED', ['participation_id', 'reward_items'], ['grant_order']),
      nodeDef('outbox', 'Outbox', '消息发件箱', 'dispatch', '写入 outbox，后续 worker 发送 HTTP / MQ / Webhook', ['grant_order'], ['HTTP', 'MQ', 'Webhook', 'Failed']),
      nodeDef('http-call', 'HTTP Call', 'HTTP 发奖', 'dispatch', '调用外部发奖接口，记录响应、耗时和错误码', ['outbox_message'], ['http_result']),
      nodeDef('mq-publish', 'MQ Publish', 'MQ 发奖', 'dispatch', '发布发奖指令到消息队列，支持 key、headers 和 traceId', ['outbox_message'], ['mq_result']),
      nodeDef('webhook', 'Webhook', 'Webhook 通知', 'dispatch', '向业务系统推送发奖、失败、参与和风险结果', ['stage_result'], ['webhook_result']),
      nodeDef('callback', 'Callback', '结果回调', 'callback', '处理成功/失败回调，保证 grant、quota、participation 状态一致', ['provider_callback'], ['callback_result']),
    ],
  },
  {
    category: '可靠性',
    items: [
      nodeDef('retry', 'Retry', '失败重试', 'reliable', '指数退避、最大次数、可重入执行和最后错误保存', ['failed_message'], ['retry_message']),
      nodeDef('dlq', 'DLQ', '死信队列', 'reliable', '超过重试次数进入 DLQ，支持人工修复和重新投递', ['failed_message'], ['dlq_record']),
      nodeDef('audit', 'Audit', '审计记录', 'audit', '记录事件、规则、奖励、quota、outbox、callback 全链路证据', ['stage_result'], ['audit_log']),
      nodeDef('reconcile', 'Reconcile', '对账校验', 'audit', '对 grant、quota、callback、outbox 做一致性扫描和修复建议', ['date_range'], ['reconcile_report']),
      nodeDef('alert', 'Alert', '告警通知', 'reliable', '关键失败、DLQ 增长、回调失败和预算异常告警', ['metric_event'], ['alert_event']),
    ],
  },
];

export function flatCatalog(): ActivityNodeData[] {
  return nodeCatalog.flatMap((group) => group.items);
}

export function getDef(id: string): ActivityNodeData {
  return flatCatalog().find((item) => item.id === id) ?? flatCatalog()[0];
}
