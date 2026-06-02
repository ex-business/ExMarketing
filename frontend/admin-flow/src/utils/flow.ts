import { MarkerType } from '@xyflow/react';
import type { ActivityEdge, ActivityNode, FlowNode, NodeKind } from '../types';
import { getDef } from '../data/nodeCatalog';

export const ACTIVITY_NODE_WIDTH = 242;
export const ACTIVITY_NODE_NORMAL_HEIGHT = 116;
export const ACTIVITY_NODE_BRANCH_HEIGHT = 166;

export const layoutPositionByDefId: Record<string, { x: number; y: number }> = {
  'event-in': { x: 80, y: 380 },
  inbox: { x: 410, y: 380 },
  normalize: { x: 410, y: 620 },
  match: { x: 740, y: 380 },
  'fact-query': { x: 740, y: 90 },
  condition: { x: 1070, y: 90 },
  qualify: { x: 1070, y: 380 },
  reward: { x: 1400, y: 220 },
  decimal: { x: 1400, y: 20 },
  limit: { x: 1400, y: 540 },
  quota: { x: 1730, y: 380 },
  participation: { x: 2060, y: 380 },
  audit: { x: 2060, y: 720 },
  'grant-order': { x: 2390, y: 380 },
  outbox: { x: 2720, y: 380 },
  retry: { x: 2720, y: 720 },
  'http-call': { x: 3050, y: 120 },
  'mq-publish': { x: 3050, y: 380 },
  webhook: { x: 3050, y: 640 },
  dlq: { x: 3050, y: 840 },
  callback: { x: 3380, y: 380 },
  reconcile: { x: 3380, y: 720 },
  alert: { x: 3380, y: 920 },
};

export function cx(...names: Array<string | false | null | undefined>): string {
  return names.filter(Boolean).join(' ');
}

export function iconFor(kind: NodeKind): string {
  return {
    entry: '⌂',
    store: '▣',
    transform: '⇄',
    logic: '⌁',
    query: '⌕',
    reward: '◇',
    quota: '↯',
    grant: '✓',
    dispatch: '↗',
    callback: '↩',
    reliable: '∞',
    audit: '◷',
  }[kind];
}

export function flowNode(defId: string, x: number, y: number, extra: Partial<ActivityNode> = {}): ActivityNode {
  const def = getDef(defId);
  return {
    id: extra.id ?? def.id,
    type: 'activity',
    position: { x, y },
    data: { ...def, ...(extra.data ?? {}) },
  };
}

export function edge(source: string, target: string, label = '', sourceHandle?: string, targetHandle?: string): ActivityEdge {
  return {
    id: `${source}-${target}-${label || 'default'}`,
    source,
    target,
    label,
    sourceHandle,
    targetHandle,
    markerEnd: { type: MarkerType.ArrowClosed, width: 5, height: 5, color: '#c4ccd8' },
  };
}

export function activityNodeHeight(node: ActivityNode): number {
  return ['condition', 'qualify', 'switch'].includes(node.data.id) ? ACTIVITY_NODE_BRANCH_HEIGHT : ACTIVITY_NODE_NORMAL_HEIGHT;
}

export function layoutFlowNodes(items: FlowNode[]): FlowNode[] {
  let unknownIndex = 0;
  return items.map((node) => {
    if (node.type !== 'activity') return node;
    const position = layoutPositionByDefId[node.data.id] ?? {
      x: 80 + (unknownIndex % 6) * 330,
      y: 1060 + Math.floor(unknownIndex / 6) * 230,
    };
    unknownIndex += layoutPositionByDefId[node.data.id] ? 0 : 1;
    return { ...node, parentId: undefined, extent: undefined, position };
  });
}
