import type { ActivityEdge, FlowNode } from '../types';
import { edge, flowNode } from '../utils/flow';

export const baseNodes: FlowNode[] = [
  flowNode('event-in', 90, 260),
  flowNode('inbox', 420, 260),
  flowNode('normalize', 750, 260),
  flowNode('match', 1080, 260),
  flowNode('fact-query', 1080, 500),
  flowNode('qualify', 1410, 260),
  flowNode('reward', 1740, 160),
  flowNode('limit', 1740, 380),
  flowNode('quota', 2070, 380),
  flowNode('participation', 2400, 380),
  flowNode('grant-order', 2730, 260),
  flowNode('outbox', 3060, 260),
  flowNode('audit', 3060, 560),
];

export const baseEdges: ActivityEdge[] = [
  edge('event-in', 'inbox'),
  edge('inbox', 'normalize', '', undefined, 'input-0'),
  edge('normalize', 'match', '', undefined, 'input-0'),
  edge('match', 'fact-query', '', undefined, 'input-0'),
  edge('fact-query', 'qualify', '', undefined, 'input-0'),
  edge('qualify', 'reward', 'PASS', 'output-0', 'input-0'),
  edge('qualify', 'audit', 'BLOCK', 'output-1', 'input-0'),
  edge('qualify', 'audit', 'RISK', 'output-2', 'input-0'),
  edge('reward', 'limit', '', undefined, 'input-0'),
  edge('limit', 'quota', '', undefined, 'input-0'),
  edge('quota', 'participation', '', undefined, 'input-0'),
  edge('participation', 'grant-order', '', undefined, 'input-0'),
  edge('grant-order', 'outbox', '', undefined, 'input-0'),
  edge('outbox', 'audit', 'audit', undefined, 'input-0'),
];

export function simpleTemplate(): { nodes: FlowNode[]; edges: ActivityEdge[] } {
  return {
    nodes: [
      flowNode('event-in', 80, 380),
      flowNode('inbox', 410, 380),
      flowNode('normalize', 740, 380),
      flowNode('match', 1070, 380),
      flowNode('fact-query', 1070, 620),
      flowNode('qualify', 1400, 380),
      flowNode('reward', 1730, 260),
      flowNode('limit', 1730, 500),
      flowNode('quota', 2060, 500),
      flowNode('participation', 2390, 500),
      flowNode('grant-order', 2720, 380),
      flowNode('outbox', 3050, 380),
      flowNode('audit', 3050, 700),
    ],
    edges: [
      edge('event-in', 'inbox'),
      edge('inbox', 'normalize'),
      edge('normalize', 'match'),
      edge('match', 'fact-query'),
      edge('fact-query', 'qualify'),
      edge('qualify', 'reward', 'PASS', 'output-0', 'input-0'),
      edge('qualify', 'audit', 'BLOCK', 'output-1', 'input-0'),
      edge('qualify', 'audit', 'RISK', 'output-2', 'input-0'),
      edge('reward', 'limit'),
      edge('limit', 'quota'),
      edge('quota', 'participation'),
      edge('participation', 'grant-order'),
      edge('grant-order', 'outbox'),
      edge('outbox', 'audit', 'audit'),
    ],
  };
}
