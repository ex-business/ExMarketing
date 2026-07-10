import type { ContainerNode, HeightMode, Node as EditorNode, NodeFrame, NodeId, Page, PositioningMode, WidthMode } from './editorSchema';

export type EditorEngineState = {
  page: Page;
  selectedNodeId?: NodeId;
  past: Page[];
  future: Page[];
  transactionPage?: Page;
};

export type ReorderPosition = 'forward' | 'backward' | 'front' | 'back' | number;

type SizePatch = Partial<Pick<NodeFrame, 'width' | 'height'>>;
type PositionPatch = Partial<Pick<NodeFrame, 'x' | 'y'>>;

export type EditorNodePatch = {
  name?: string;
  visible?: boolean;
  visibilityExpression?: string;
  frame?: Partial<NodeFrame>;
  radius?: string;
  positioning?: PositioningMode;
  widthMode?: WidthMode;
  heightMode?: HeightMode;
  locked?: boolean;
  content?: { text?: string; variant?: 'primary' | 'secondary' };
  resourceBindings?: EditorNode['resourceBindings'];
  actionBindings?: EditorNode['actionBindings'];
  styleTokens?: EditorNode['styleTokens'];
  layout?: ContainerNode['layout'];
  flexDirection?: ContainerNode['flexDirection'];
  justifyContent?: ContainerNode['justifyContent'];
  alignItems?: ContainerNode['alignItems'];
  flexWrap?: ContainerNode['flexWrap'];
  gap?: number;
};

export type EditorEngineAction =
  | { type: 'replacePage'; page: Page; selectedNodeId?: NodeId }
  | { type: 'selectNode'; nodeId?: NodeId }
  | { type: 'insertNode'; node: EditorNode; parentId?: NodeId; index?: number }
  | { type: 'updateNode'; nodeId: NodeId; patch: EditorNodePatch }
  | { type: 'beginTransaction' }
  | { type: 'commitTransaction' }
  | { type: 'moveNode'; nodeId: NodeId; position: PositionPatch; transient?: boolean }
  | { type: 'resizeNode'; nodeId: NodeId; size: SizePatch; transient?: boolean }
  | { type: 'placeNode'; nodeId: NodeId; parentId?: NodeId; index?: number }
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
  const historyPage = state.transactionPage ?? state.page;
  return {
    ...state,
    page,
    past: [...state.past.slice(-(HISTORY_LIMIT - 1)), historyPage],
    future: [],
    transactionPage: undefined,
  };
}

function updateTransaction(state: EditorEngineState, page: Page): EditorEngineState {
  return {
    ...state,
    page,
    transactionPage: state.transactionPage ?? state.page,
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

function fitContainerToChildren(page: Page, containerId: NodeId): Page {
  const container = containerById(page, containerId);
  if (!container?.children.length) return page;
  const children = container.children.map((id) => nodeById(page, id)).filter((node): node is EditorNode => Boolean(node));
  if (!children.length) return page;
  const gap = container.gap ?? 8;
  const padding = 24;
  const isColumn = container.flexDirection === 'column';
  const contentWidth = isColumn
    ? Math.max(...children.map((child) => child.frame.width))
    : children.reduce((sum, child) => sum + child.frame.width, 0) + gap * Math.max(0, children.length - 1);
  const contentHeight = isColumn
    ? children.reduce((sum, child) => sum + child.frame.height, 0) + gap * Math.max(0, children.length - 1)
    : Math.max(...children.map((child) => child.frame.height));
  return updateNode(page, containerId, (current) => ({
    ...current,
    frame: {
      ...current.frame,
      width: Math.max(current.frame.width, Math.ceil(contentWidth + (isColumn ? 0 : padding))),
      height: Math.max(current.frame.height, Math.ceil(contentHeight + padding)),
    },
  }));
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

function nextNodeName(baseName: string, existingNames: Set<string>) {
  const normalizedBase = baseName.replace(/\s+(?:副本|\d+)$/u, '').trim() || '节点';
  let index = 2;
  let name = normalizedBase;
  while (existingNames.has(name)) {
    name = `${normalizedBase} ${index}`;
    index += 1;
  }
  existingNames.add(name);
  return name;
}

function nextEntityId(baseId: string, existingIds: Set<string>) {
  let index = 2;
  let id = baseId;
  while (existingIds.has(id)) {
    id = `${baseId}_${index}`;
    index += 1;
  }
  existingIds.add(id);
  return id;
}

function cloneEntityIds<T extends { id: string }>(items: T[], baseId: string, existingIds: Set<string>): T[] {
  return items.map((item, index) => ({
    ...item,
    id: nextEntityId(`${baseId}${items.length > 1 ? `_${index + 1}` : ''}`, existingIds),
  }));
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
    const base = {
      ...node,
      parentId,
      positioning: node.positioning ?? 'flow',
      widthMode: node.widthMode ?? 'fill',
      heightMode: node.heightMode ?? 'auto',
      locked: node.locked ?? false,
    };
    if (base.type !== 'container') return base;
    return {
      ...base,
      children: base.children.filter((childId) => nodeIds.has(childId) && childId !== node.id),
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

export function replacePage(page: Page, selectedNodeId?: NodeId): EditorEngineState {
  return createEditorEngineState(page, selectedNodeId);
}

export function insertNode(state: EditorEngineState, node: EditorNode, parentId?: NodeId, index?: number): EditorEngineState {
  if (state.page.nodes.some((current) => current.id === node.id)) return state;
  const parent = parentId ? containerById(state.page, parentId) : undefined;
  const nextNode = {
    ...node,
    parentId: parent?.id,
    positioning: parent ? 'flow' : node.positioning,
    widthMode: parent ? 'fill' : node.widthMode,
  } as EditorNode;
  const rootNodeIds = parent
    ? state.page.rootNodeIds
    : (() => {
      const ids = [...state.page.rootNodeIds];
      ids.splice(index === undefined ? ids.length : Math.max(0, Math.min(ids.length, index)), 0, node.id);
      return ids;
    })();
  const nodes = state.page.nodes.map((current) => {
    if (!parent || current.id !== parent.id || current.type !== 'container') return current;
    const children = [...current.children];
    const insertionIndex = index === undefined ? children.length : Math.max(0, Math.min(children.length, index));
    children.splice(insertionIndex, 0, node.id);
    return { ...current, children };
  });
  const normalizedPage = normalizePage({ ...state.page, rootNodeIds, nodes: [...nodes, nextNode] });
  const page = parent ? fitContainerToChildren(normalizedPage, parent.id) : normalizedPage;
  return { ...pushHistory(state, page), selectedNodeId: node.id };
}

export function patchNode(state: EditorEngineState, nodeId: NodeId, patch: EditorNodePatch): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node) return state;
  const page = updateNode(state.page, nodeId, (current) => {
    const base = {
      ...current,
      ...(patch.name !== undefined ? { name: patch.name.trim() || current.name } : {}),
      ...(patch.visible !== undefined ? { visible: patch.visible } : {}),
      ...(patch.visibilityExpression !== undefined ? { visibilityExpression: patch.visibilityExpression } : {}),
      ...(patch.radius !== undefined ? { radius: patch.radius } : {}),
      ...(patch.positioning !== undefined ? { positioning: patch.positioning } : {}),
      ...(patch.widthMode !== undefined ? { widthMode: patch.widthMode } : {}),
      ...(patch.heightMode !== undefined ? { heightMode: patch.heightMode } : {}),
      ...(patch.locked !== undefined ? { locked: patch.locked } : {}),
      ...(patch.resourceBindings ? { resourceBindings: patch.resourceBindings } : {}),
      ...(patch.actionBindings ? { actionBindings: patch.actionBindings } : {}),
      ...(patch.styleTokens ? { styleTokens: patch.styleTokens } : {}),
      frame: patch.frame ? {
        ...current.frame,
        ...(patch.frame.x !== undefined ? { x: patch.frame.x } : {}),
        ...(patch.frame.y !== undefined ? { y: patch.frame.y } : {}),
        ...(patch.frame.width !== undefined ? { width: Math.max(0, patch.frame.width) } : {}),
        ...(patch.frame.height !== undefined ? { height: Math.max(0, patch.frame.height) } : {}),
        ...(patch.frame.z !== undefined ? { z: patch.frame.z } : {}),
      } : current.frame,
      content: patch.content ? {
        ...current.content,
        ...(patch.content.text !== undefined ? { text: patch.content.text } : {}),
        ...(patch.content.variant !== undefined ? { variant: patch.content.variant } : {}),
      } : current.content,
    };
    if (base.type !== 'container') return base;
    return {
      ...base,
      ...(patch.layout !== undefined ? { layout: patch.layout } : {}),
      ...(patch.flexDirection !== undefined ? { flexDirection: patch.flexDirection } : {}),
      ...(patch.justifyContent !== undefined ? { justifyContent: patch.justifyContent } : {}),
      ...(patch.alignItems !== undefined ? { alignItems: patch.alignItems } : {}),
      ...(patch.flexWrap !== undefined ? { flexWrap: patch.flexWrap } : {}),
      ...(patch.gap !== undefined ? { gap: Math.max(0, patch.gap) } : {}),
    };
  });
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function beginTransaction(state: EditorEngineState): EditorEngineState {
  return state.transactionPage ? state : { ...state, transactionPage: state.page };
}

export function commitTransaction(state: EditorEngineState): EditorEngineState {
  if (!state.transactionPage) return state;
  return {
    ...state,
    past: [...state.past.slice(-(HISTORY_LIMIT - 1)), state.transactionPage],
    future: [],
    transactionPage: undefined,
  };
}

export function moveNode(state: EditorEngineState, nodeId: NodeId, position: PositionPatch, transient = false): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node || node.positioning !== 'absolute' || node.locked) return state;
  const updatedPage = updateNode(state.page, nodeId, (current) => ({
    ...current,
    frame: {
      ...current.frame,
      x: position.x ?? current.frame.x,
      y: position.y ?? current.frame.y,
    },
  }));
  const page = node.parentId ? fitContainerToChildren(updatedPage, node.parentId) : updatedPage;
  return { ...(transient ? updateTransaction(state, page) : pushHistory(state, page)), selectedNodeId: nodeId };
}

export function resizeNode(state: EditorEngineState, nodeId: NodeId, size: SizePatch, transient = false): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  if (!node || node.locked) return state;
  const updatedPage = updateNode(state.page, nodeId, (current) => ({
    ...current,
    frame: {
      ...current.frame,
      width: Math.max(0, size.width ?? current.frame.width),
      height: Math.max(0, size.height ?? current.frame.height),
    },
  }));
  const page = node.parentId ? fitContainerToChildren(updatedPage, node.parentId) : updatedPage;
  return { ...(transient ? updateTransaction(state, page) : pushHistory(state, page)), selectedNodeId: nodeId };
}

export function moveIntoContainer(state: EditorEngineState, nodeId: NodeId, containerId: NodeId, index?: number): EditorEngineState {
  return placeNode(state, nodeId, containerId, index);
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
    rootNodeIds: [...state.page.rootNodeIds.filter((id) => id !== nodeId), nodeId],
    nodes: state.page.nodes.map((current) => {
      if (current.id === nodeId) return { ...current, parentId: undefined, frame: nextFrame, positioning: 'flow', widthMode: 'fill' } as EditorNode;
      if (current.id === node.parentId && current.type === 'container') return { ...current, children: current.children.filter((id) => id !== nodeId) };
      return current;
    }),
  });
  return { ...pushHistory(state, page), selectedNodeId: nodeId };
}

export function placeNode(state: EditorEngineState, nodeId: NodeId, parentId?: NodeId, index?: number): EditorEngineState {
  const node = nodeById(state.page, nodeId);
  const parent = parentId ? containerById(state.page, parentId) : undefined;
  if (!node || node.locked || (parentId && !parent) || nodeId === parentId || (parentId && isDescendantOf(state.page, parentId, nodeId))) return state;

  const currentSiblings = node.parentId ? containerById(state.page, node.parentId)?.children ?? [] : state.page.rootNodeIds;
  const targetSiblings = parent ? parent.children : state.page.rootNodeIds;
  const targetIndex = index === undefined ? targetSiblings.length : Math.max(0, Math.min(targetSiblings.length, index));
  const adjustedIndex = node.parentId === parentId && currentSiblings.indexOf(nodeId) >= 0 && currentSiblings.indexOf(nodeId) < targetIndex
    ? targetIndex - 1
    : targetIndex;
  const nextTargetSiblings = targetSiblings.filter((id) => id !== nodeId);
  nextTargetSiblings.splice(Math.max(0, Math.min(nextTargetSiblings.length, adjustedIndex)), 0, nodeId);

  const page = normalizePage({
    ...state.page,
    rootNodeIds: parent ? state.page.rootNodeIds.filter((id) => id !== nodeId) : nextTargetSiblings,
    nodes: state.page.nodes.map((current) => {
      if (current.id === nodeId) return {
        ...current,
        parentId: parent?.id,
        positioning: 'flow',
        widthMode: 'fill',
      } as EditorNode;
      if (current.type !== 'container') return current;
      if (current.id === parent?.id) return { ...current, children: nextTargetSiblings };
      if (current.id === node.parentId && current.id !== parent?.id) return { ...current, children: current.children.filter((id) => id !== nodeId) };
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
  const existingNames = new Set(state.page.nodes.map((item) => item.name));
  const existingSourceBlockIds = new Set(state.page.nodes.flatMap((item) => item.type === 'component' && item.sourceBlockId ? [item.sourceBlockId] : []));
  const existingResourceBindingIds = new Set(state.page.nodes.flatMap((item) => item.resourceBindings.map((binding) => binding.id)));
  const existingActionBindingIds = new Set(state.page.nodes.flatMap((item) => item.actionBindings.map((binding) => binding.id)));
  const existingStyleTokenIds = new Set(state.page.nodes.flatMap((item) => item.styleTokens.map((token) => token.id)));
  const idMap = new Map<NodeId, NodeId>();
  for (const id of idsToClone) idMap.set(id, nextNodeId(id, existingIds));
  const clonedNodes = state.page.nodes.flatMap((current): EditorNode[] => {
    if (!idMap.has(current.id)) return [];
    const clonedId = idMap.get(current.id)!;
    const clonedParentId = current.parentId ? idMap.get(current.parentId) ?? current.parentId : undefined;
    const frame = current.id === nodeId
      ? { ...current.frame, x: current.frame.x + 18, y: current.frame.y + 18, z: current.frame.z + 1 }
      : { ...current.frame };
    const clonedBase = {
      id: clonedId,
      name: nextNodeName(current.name, existingNames),
      parentId: clonedParentId,
      frame,
      resourceBindings: cloneEntityIds(current.resourceBindings, `${clonedId}_resource`, existingResourceBindingIds),
      actionBindings: cloneEntityIds(current.actionBindings, `${clonedId}_action`, existingActionBindingIds),
      styleTokens: cloneEntityIds(current.styleTokens, `${clonedId}_style`, existingStyleTokenIds),
    };
    if (current.type === 'container') {
      return [{
        ...current,
        ...clonedBase,
        children: current.children.map((childId) => idMap.get(childId)).filter((id): id is NodeId => Boolean(id)),
      }];
    }
    return [{
      ...current,
      ...clonedBase,
      sourceBlockId: current.sourceBlockId ? nextEntityId(`${current.sourceBlockId}_copy`, existingSourceBlockIds) : undefined,
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
  const siblingIds = node.parentId
    ? containerById(state.page, node.parentId)?.children ?? []
    : state.page.rootNodeIds;
  const orderedSiblingIds = reorderIds(siblingIds, nodeId, position);
  const siblingZValues = siblingIds.map((id) => nodeById(state.page, id)?.frame.z ?? 0);
  const firstZ = siblingZValues.length ? Math.min(...siblingZValues) : 0;
  const zByNodeId = new Map(orderedSiblingIds.map((id, index) => [id, firstZ + index]));
  const page = normalizePage({
    ...state.page,
    rootNodeIds: node.parentId ? state.page.rootNodeIds : orderedSiblingIds,
    nodes: state.page.nodes.map((current) => {
      if (current.id === node.parentId && current.type === 'container') {
        return { ...current, children: orderedSiblingIds };
      }
      return current;
    }).map((current) => zByNodeId.has(current.id) ? {
      ...current,
      frame: {
        ...current.frame,
        z: zByNodeId.get(current.id)!,
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
    transactionPage: undefined,
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
    transactionPage: undefined,
  };
}

export function editorEngineReducer(state: EditorEngineState, action: EditorEngineAction): EditorEngineState {
  if (action.type === 'replacePage') return replacePage(action.page, action.selectedNodeId);
  if (action.type === 'selectNode') return selectNode(state, action.nodeId);
  if (action.type === 'insertNode') return insertNode(state, action.node, action.parentId, action.index);
  if (action.type === 'updateNode') return patchNode(state, action.nodeId, action.patch);
  if (action.type === 'beginTransaction') return beginTransaction(state);
  if (action.type === 'commitTransaction') return commitTransaction(state);
  if (action.type === 'moveNode') return moveNode(state, action.nodeId, action.position, action.transient);
  if (action.type === 'resizeNode') return resizeNode(state, action.nodeId, action.size, action.transient);
  if (action.type === 'placeNode') return placeNode(state, action.nodeId, action.parentId, action.index);
  if (action.type === 'moveIntoContainer') return moveIntoContainer(state, action.nodeId, action.containerId, action.index);
  if (action.type === 'moveOutOfContainer') return moveOutOfContainer(state, action.nodeId, action.frame);
  if (action.type === 'deleteNode') return deleteNode(state, action.nodeId);
  if (action.type === 'duplicateNode') return duplicateNode(state, action.nodeId);
  if (action.type === 'reorderNode') return reorderNode(state, action.nodeId, action.position);
  if (action.type === 'undo') return undo(state);
  return redo(state);
}
