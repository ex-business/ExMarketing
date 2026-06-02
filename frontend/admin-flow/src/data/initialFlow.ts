import type { ActivityEdge, FlowNode } from '../types';
import { edge, flowNode } from '../utils/flow';

export const baseNodes: FlowNode[] = [
  flowNode('event-in', 80, 380),
  flowNode('inbox', 410, 380),
  flowNode('match', 740, 380),
  flowNode('fact-query', 740, 90),
  flowNode('condition', 1070, 90),
  flowNode('qualify', 1070, 380),
  flowNode('reward', 1400, 220),
  flowNode('limit', 1400, 540),
  flowNode('quota', 1730, 380),
  flowNode('participation', 2060, 380),
  flowNode('grant-order', 2390, 380),
  flowNode('outbox', 2720, 380),
  flowNode('http-call', 3050, 120),
  flowNode('mq-publish', 3050, 380),
  flowNode('webhook', 3050, 640),
  flowNode('callback', 3380, 380),
  flowNode('audit', 2060, 720),
  flowNode('retry', 2720, 720),
  flowNode('dlq', 3050, 840),
];

export const baseEdges: ActivityEdge[] = [
  edge('event-in', 'inbox'),
  edge('inbox', 'match', '', undefined, 'input-0'),
  edge('match', 'qualify', '', undefined, 'input-0'),
  edge('fact-query', 'qualify', 'fact', undefined, 'input-0'),
  edge('condition', 'qualify', 'condition', 'output-0', 'input-0'),
  edge('qualify', 'reward', 'PASS', 'output-0', 'input-0'),
  edge('qualify', 'limit', 'RISK', 'output-2', 'input-0'),
  edge('reward', 'quota', '', undefined, 'input-0'),
  edge('limit', 'quota', '', undefined, 'input-0'),
  edge('quota', 'participation'),
  edge('participation', 'grant-order'),
  edge('grant-order', 'outbox'),
  edge('outbox', 'http-call', 'HTTP', 'output-0'),
  edge('outbox', 'mq-publish', 'MQ', 'output-1'),
  edge('outbox', 'webhook', 'Webhook', 'output-2'),
  edge('http-call', 'callback', '', undefined, 'input-0'),
  edge('mq-publish', 'callback', '', undefined, 'input-0'),
  edge('webhook', 'callback', '', undefined, 'input-0'),
  edge('participation', 'audit', 'audit'),
  edge('outbox', 'retry', 'Failed', 'output-3'),
  edge('retry', 'dlq', 'exhausted'),
];

export function simpleTemplate(): { nodes: FlowNode[]; edges: ActivityEdge[] } {
  return {
    nodes: [
      flowNode('event-in', 80, 380),
      flowNode('inbox', 410, 380),
      flowNode('match', 740, 380),
      flowNode('reward', 1070, 380),
      flowNode('quota', 1400, 380),
      flowNode('grant-order', 1730, 380),
      flowNode('outbox', 2060, 380),
      flowNode('callback', 2390, 380),
      flowNode('audit', 1400, 700),
    ],
    edges: [
      edge('event-in', 'inbox'),
      edge('inbox', 'match'),
      edge('match', 'reward'),
      edge('reward', 'quota'),
      edge('quota', 'grant-order'),
      edge('grant-order', 'outbox'),
      edge('outbox', 'callback'),
      edge('quota', 'audit', 'audit'),
    ],
  };
}
