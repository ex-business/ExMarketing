import type { ContainerNode, Node as EditorNode, NodeFrame, NodeId, Page } from './editorSchema';
import { LAYOUT_CHILD_HORIZONTAL_INSET } from './layout';

export type EditorEngineState = {
  page: Page;
  selectedNodeId?: NodeId;
  past: Page[];
  future: Page[];
};

export type ReorderPosition = 'forward' | 'backward' | 'front' | 'back' | number;

type SizePatch = Partial<Pick<NodeFrame, 'width' | 'height'>>;
type PositionPatch = Partial<Pick<NodeFrame, 'x' | 'y'>>;

export type EditorEngineAction =
  | { type: 'selectNode'; nodeId?: NodeId }
  | { type: 'moveNode'; nodeId: NodeId; position: PositionPatch }
  | { type: 'resizeNode'; nodeId: NodeId; size: SizePatch }
  | { type: 'moveIntoContainer'; nodeId: NodeId; containerId: NodeId; index?: number }
  | { type: 'moveOutOfContainer'; nodeId: NodeId; frame?: PositionPatch }
  | { type: 'deleteNode'; nodeId: NodeId }
  | { type: 'duplicateNode'; nodeId: NodeId }
  | { type: 'reorderNode'; nodeId: NodeId; position: ReorderPosition }
  | { type: 'undo' }
  | { type: 'redo' };

const HISTORY_LIMIT = 80;

export function createEditorEngineState(page: Page, selectedNodeId?: NodeId): EditorEngineState {
  return {
    page: normalizePage(page),
    selectedNodeId: selectedNodeId && page.nodes.some((node) => node.id === selectedNodeId) ? selectedNodeId : page.rootNodeIds[0],
    past: [],
    future: [],
  };
}

function pushHistory(state: EditorEngineState, page: Page): EditorEngineState {
  return {
    ...state,
    page,
    past: [...state.past.slice(-(HISTORY_LIMIT - 1)), state.page],
    future: [],
  };
}

function updateNode(page: Page, nodeId: NodeId, updater: (node: EditorNode) => EditorNode): Page {
  return normalizePage({
    ...page,
    nodes: page.nodes.map((node) => node.id === nodeId ? updater(node) : node),
  });
}

function nodeById(page: Page, nodeId: NodeId) {
  return page.nodes.find((node) => node.id === nodeId);
}

function containerById(page: Page, nodeId: NodeId): ContainerNode | undefined {
  const node = nodeById(page, nodeId);
  return node?.type === 'container' ? node : undefined;
}

function descendantsOf(page: Page, nodeId: NodeId): Set<NodeId> {
  const directChildren = page.nodes.filter((node) => node.parentId === nodeId).map((node) => node.id);
  const descendants = new Set<NodeId>(directChildren);
  for (const childId of directChildren) {
    for (const descendantId of descendantsOf(page, childId)) descendants.add(descendantId);
  }
  return descendants;
}

function isDescendantOf(page: Page, nodeId: NodeId, ancestorId: NodeId): boolean {
  let current = nodeById(page, nodeId);
  while (current?.parentId) {
    if (current.parentId === ancestorId) return true;
    current = nodeById(page, current.parentId);
  }
  return false;
}

function nextNodeId(baseId: NodeId, existingIds: Set<NodeId>) {
  let index = 1;
  let id = `${baseId}_copy`;
  while (existingIds.has(id)) {
    index += 1;
    id = `${baseId}_copy_${index}`;
  }
  existingIds.add(id);
  return id;
}

function reorderIds(ids: NodeId[], nodeId: NodeId, position: ReorderPosition) {
  const currentIndex = ids.indexOf(nodeId);
  if (currentIndex < 0) return ids;
  const nextIds = [...ids];
  const [item] = nextIds.splice(currentIndex, 1);
  let nextIndex = currentIndex;
  if (position === 'forward') nextIndex = Math.min(nextIds.length, currentIndex + 1);
  else if (position === 'backward') nextIndex = Math.max(0, currentIndex - 1);
  else if (position === 'front') nextIndex = nextIds.length;
  else if (position === 'back') nextIndex = 0;
  else nextIndex = Math.max(0, Math.min(nextIds.length, position));
  nextIds.splice(nextIndex, 0, item);
  return nextIds;
}

function normalizeRootIds(page: Page) {
  const nodeIds = new Set(page.nodes.map((node) => node.id));
  const rootsFromPage = page.rootNodeIds.filter((id) => nodeIds.has(id) && !nodeById(page, id)?.parentId);
  const rootsFromNodes = page.nodes.filter((node) => !node.parentId && !rootsFromPage.includes(node.id)).sort((a, b) => a.frame.z - b.frame.z).map((node) => node.id);
  return [...rootsFromPage, ...rootsFromNodes];
}

export function normalizePage(page: Page): Page {
  const nodeIds = new Set(page.nodes.map((node) => node.id));
  const nodes = page.nodes.map((node) => {
    const parentId = node.parentId && nodeIds.has(node.parentId) ? node.parentId : undefined;
    if (node.type !== 'container') return { ...node, parentId };
    return {
      ...node,
      parentId,
      children: node.children.filter((childId) => nodeIds.has(childId) && childId !== node.id),
    };
  });
  const childrenByParent = nodes.reduce<Record<string, NodeId[]>>((groups, node) => {
    if (!node.parentId) return groups;
    return { ...groups, [node.parentId]: [...(groups[node.parentId] ?? []), node.id] };
  }, {});
  const normalizedNodes = nodes.map((node) => {
    if (node.type !== 'container') return node;
    const existingChildren = node.children.filter((childId) => childrenByParent[node.id]?.includes(childId));
    const missingChildren = (childrenByParent[node.id] ?? []).filter((childId) => !existingChildren.includes(childId));
    return { ...node, children: [...existingChildren, ...missingChildren] };
  });
  const normalizedPage = {
    ...page,
    nodes: normalizedNodes,
    rootNodeIds: normalizeRootIds({ ...page, nodes: normalizedNodes }),
  };
  return {
    ...normalizedPage,
    resourceBindings: normalizedPage.nodes.flatMap((node) => node.resourceBindings),
    actionBindings: normalizedPage.nodes.flatMap((node) => node.actionBindings),
    styleTokens: normalizedPage.nodes.flatMap((node) => node.styleTokens),
  };
}

export function selectNode(state: EditorEngineState, nodeId?: NodeId): EditorEngineState {
  return {
    ...state,
    selectedNodeId: nodeId && state.page.nodes.some((node) => node.id === nodeId) ? nodeId : undefined,
  };
}

export function moveNode(state: EditorEngineState, nodeId: NodeId, position: PositionPatch): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const page = updateNode(state.page, nodeId, (current) => ({
    ...current,
    frame: {
      ...current.frame,
      x: position.x ?? current.frame.x,
      y: position.y ?? current.frame.y,
    },
  }));
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function resizeNode(state: EditorEngineState, nodeId: NodeId, size: SizePatch): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const page = updateNode(state.page, nodeId, (current) => ({
    ...current,
    frame: {
      ...current.frame,
      width: Math.max(0, size.width ?? current.frame.width),
      height: Math.max(0, size.height ?? current.frame.height),
    },
  }));
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function moveIntoContainer(state: EditorEngineState, nodeId: NodeId, containerId: NodeId, index?: number): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  const container = containerById(state.page, containerId);
  if (!node || !container || node.id === container.id || isDescendantOf(state.page, container.id, node.id)) return state;
  const children = container.children.filter((id) => id !== nodeId);
  const childIndex = index === undefined ? children.length : Math.max(0, Math.min(children.length, index));
  children.splice(childIndex, 0, nodeId);
  const page = normalizePage({
    ...state.page,
    rootNodeIds: state.page.rootNodeIds.filter((id) => id !== nodeId),
    nodes: state.page.nodes.map((current) => {
      if (current.id === nodeId) {
        return {
          ...current,
          parentId: containerId,
          frame: {
            ...current.frame,
            x: Math.max(0, current.frame.x - container.frame.x),
            y: Math.max(0, current.frame.y - container.frame.y),
            width: current.type === 'container' ? current.frame.width : Math.max(0, container.frame.width - LAYOUT_CHILD_HORIZONTAL_INSET),
          },
        };
      }
      if (current.id === containerId && current.type === 'container') return { ...current, children };
      return current;
    }),
  });
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function moveOutOfContainer(state: EditorEngineState, nodeId: NodeId, frame?: PositionPatch): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node?.parentId) return state;
  const parent = nodeById(state.page, node.parentId);
  const nextFrame = {
    ...node.frame,
    x: frame?.x ?? Math.max(0, (parent?.frame.x ?? 0) + node.frame.x),
    y: frame?.y ?? Math.max(0, (parent?.frame.y ?? 0) + (parent?.frame.height ?? 0) + 18),
  };
  const page = normalizePage({
    ...state.page,
    rootNodeIds: [...state.page.rootNodeIds, nodeId],
    nodes: state.page.nodes.map((current) => {
      if (current.id === nodeId) return { ...current, parentId: undefined, frame: nextFrame };
      if (current.id === node.parentId && current.type === 'container') {
        return { ...current, children: current.children.filter((childId) => childId !== nodeId) };
      }
      return current;
    }),
  });
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function deleteNode(state: EditorEngineState, nodeId: NodeId): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const idsToDelete = descendantsOf(state.page, nodeId);
  idsToDelete.add(nodeId);
  const page = normalizePage({
    ...state.page,
    rootNodeIds: state.page.rootNodeIds.filter((id) => !idsToDelete.has(id)),
    nodes: state.page.nodes.flatMap((current): EditorNode[] => {
      if (idsToDelete.has(current.id)) return [];
      if (current.type === 'container') return [{ ...current, children: current.children.filter((childId) => !idsToDelete.has(childId)) }];
      return [current];
    }),
  });
  const selectedNodeId = state.selectedNodeId && !idsToDelete.has(state.selectedNodeId) ? state.selectedNodeId : page.rootNodeIds[0];
  return { ...pushHistory(state, page), selectedNodeId };
}

export function duplicateNode(state: EditorEngineState, nodeId: NodeId): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const idsToClone = [nodeId, ...descendantsOf(state.page, nodeId)];
  const existingIds = new Set(state.page.nodes.map((item) => item.id));
  const idMap = new Map<NodeId, NodeId>();
  for (const id of idsToClone) idMap.set(id, nextNodeId(id, existingIds));
  const clonedNodes = state.page.nodes.flatMap((current): EditorNode[] => {
    if (!idMap.has(current.id)) return [];
    const clonedId = idMap.get(current.id)!;
    const clonedParentId = current.parentId ? idMap.get(current.parentId) ?? current.parentId : undefined;
    const frame = current.id === nodeId
      ? { ...current.frame, x: current.frame.x + 18, y: current.frame.y + 18, z: current.frame.z + 1 }
      : { ...current.frame };
    if (current.type === 'container') {
      return [{
        ...current,
        id: clonedId,
        name: `${current.name} 副本`,
        parentId: clonedParentId,
        frame,
        children: current.children.map((childId) => idMap.get(childId)).filter((id): id is NodeId => Boolean(id)),
      }];
    }
    return [{
      ...current,
      id: clonedId,
      name: `${current.name} 副本`,
      parentId: clonedParentId,
      frame,
      sourceBlockId: current.sourceBlockId ? `${current.sourceBlockId}_copy` : undefined,
    }];
  });
  const clonedRootId = idMap.get(nodeId)!;
  const page = normalizePage({
    ...state.page,
    nodes: [...state.page.nodes, ...clonedNodes],
    rootNodeIds: node.parentId ? state.page.rootNodeIds : reorderIds([...state.page.rootNodeIds, clonedRootId], clonedRootId, state.page.rootNodeIds.indexOf(nodeId) + 1),
  });
  return { ...pushHistory(state, page), selectedNodeId: clonedRootId };
}

export function reorderNode(state: EditorEngineState, nodeId: NodeId, position: ReorderPosition): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const page = normalizePage({
    ...state.page,
    rootNodeIds: node.parentId ? state.page.rootNodeIds : reorderIds(state.page.rootNodeIds, nodeId, position),
    nodes: state.page.nodes.map((current) => {
      if (current.id === node.parentId && current.type === 'container') {
        return { ...current, children: reorderIds(current.children, nodeId, position) };
      }
      return current;
    }).map((current) => current.id === nodeId ? {
      ...current,
      frame: {
        ...current.frame,
        z: position === 'front'
          ? Math.max(...state.page.nodes.map((item) => item.frame.z)) + 1
          : position === 'back'
            ? 1
            : current.frame.z,
      },
    } : current),
  });
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function undo(state: EditorEngineState): EditorEngineState {
  const previous = state.past[state.past.length - 1];
  if (!previous) return state;
  return {
    ...state,
    page: previous,
    selectedNodeId: state.selectedNodeId && previous.nodes.some((node) => node.id === state.selectedNodeId) ? state.selectedNodeId : previous.rootNodeIds[0],
    past: state.past.slice(0, -1),
    future: [state.page, ...state.future],
  };
}

export function redo(state: EditorEngineState): EditorEngineState {
  const next = state.future[0];
  if (!next) return state;
  return {
    ...state,
    page: next,
    selectedNodeId: state.selectedNodeId && next.nodes.some((node) => node.id === state.selectedNodeId) ? state.selectedNodeId : next.rootNodeIds[0],
    past: [...state.past.slice(-(HISTORY_LIMIT - 1)), state.page],
    future: state.future.slice(1),
  };
}

export function editorEngineReducer(state: EditorEngineState, action: EditorEngineAction): EditorEngineState {
  if (action.type === 'selectNode') return selectNode(state, action.nodeId);
  if (action.type === 'moveNode') return moveNode(state, action.nodeId, action.position);
  if (action.type === 'resizeNode') return resizeNode(state, action.nodeId, action.size);
  if (action.type === 'moveIntoContainer') return moveIntoContainer(state, action.nodeId, action.containerId, action.index);
  if (action.type === 'moveOutOfContainer') return moveOutOfContainer(state, action.nodeId, action.frame);
  if (action.type === 'deleteNode') return deleteNode(state, action.nodeId);
  if (action.type === 'duplicateNode') return duplicateNode(state, action.nodeId);
  if (action.type === 'reorderNode') return reorderNode(state, action.nodeId, action.position);
  if (action.type === 'undo') return undo(state);
  return redo(state);
}
