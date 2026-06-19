import { MarkerType } from '@xyflow/react';
import type { ActivityEdge, ActivityNode, FlowNode, WorkflowEdgeRoute, WorkflowEdgeRouteType } from '../types';
import { getDef } from '../data/nodeCatalog';

export const ACTIVITY_NODE_WIDTH = 242;
export const ACTIVITY_NODE_NORMAL_HEIGHT = 116;
export const ACTIVITY_NODE_BRANCH_HEIGHT = 166;

export const layoutPositionByDefId: Record<string, { x: number; y: number }> = {
  'event-in': { x: 90, y: 300 },
  inbox: { x: 410, y: 300 },
  normalize: { x: 410, y: 540 },
  match: { x: 730, y: 300 },
  'fact-query': { x: 730, y: 300 },
  'facts-compose': { x: 780, y: 420 },
  'facts-normalize': { x: 1040, y: 420 },
  code: { x: 720, y: 690 },
  condition: { x: 720, y: 80 },
  qualify: { x: 780, y: 520 },
  reward: { x: 1230, y: 220 },
  decimal: { x: 1230, y: 20 },
  limit: { x: 1220, y: 520 },
  quota: { x: 1560, y: 360 },
  participation: { x: 1880, y: 360 },
  audit: { x: 1230, y: 720 },
  'grant-order': { x: 2200, y: 300 },
  outbox: { x: 2520, y: 300 },
  retry: { x: 2520, y: 620 },
  'http-call': { x: 2840, y: 80 },
  'mq-publish': { x: 2840, y: 300 },
  webhook: { x: 2840, y: 520 },
  dlq: { x: 2840, y: 780 },
  callback: { x: 3160, y: 300 },
  reconcile: { x: 3160, y: 620 },
  alert: { x: 3160, y: 820 },
};

export function cx(...names: Array<string | false | null | undefined>): string {
  return names.filter(Boolean).join(' ');
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

const STRUCTURED_ROUTE_LABELS = new Set<WorkflowEdgeRouteType>(['IF', 'ELIF', 'ELSE', 'PASS', 'BLOCK', 'RISK', 'SKIP']);

const DEFAULT_ROUTE_EXPRESSIONS: Partial<Record<WorkflowEdgeRouteType, string>> = {
  IF: 'condition == true',
  ELIF: 'condition == true',
  PASS: 'decision == "PASS"',
  BLOCK: 'decision == "BLOCK"',
  RISK: 'decision == "RISK"',
  SKIP: 'decision == "SKIP"',
};

const DEFAULT_BRANCH_ROUTE_LABELS = new Set<WorkflowEdgeRouteType>(['ELSE', 'RISK', 'SKIP', 'DEFAULT']);

export function routeFromEdgeLabel(label = '', sourceHandle = ''): WorkflowEdgeRoute {
  const normalizedLabel = label.trim();
  const upperLabel = normalizedLabel.toUpperCase() as WorkflowEdgeRouteType;
  if (STRUCTURED_ROUTE_LABELS.has(upperLabel)) {
    return { type: upperLabel, key: upperLabel, label: upperLabel, expression: DEFAULT_ROUTE_EXPRESSIONS[upperLabel], default: DEFAULT_BRANCH_ROUTE_LABELS.has(upperLabel) };
  }
  if (normalizedLabel) {
    return { type: 'OUTPUT', key: normalizedLabel, label: normalizedLabel };
  }
  if (sourceHandle) {
    return { type: 'OUTPUT', key: sourceHandle, label: sourceHandle };
  }
  return { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true };
}

export function edge(source: string, target: string, label = '', sourceHandle?: string, targetHandle?: string, route?: WorkflowEdgeRoute): ActivityEdge {
  const edgeRoute = route ?? routeFromEdgeLabel(label, sourceHandle);
  return {
    id: `${source}-${target}-${label || 'default'}`,
    source,
    target,
    label: edgeRoute.label ?? label,
    sourceHandle,
    targetHandle,
    data: { route: edgeRoute },
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
