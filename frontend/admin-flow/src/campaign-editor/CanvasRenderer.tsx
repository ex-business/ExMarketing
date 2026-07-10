import { useState, type CSSProperties, type DragEvent as ReactDragEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import type { Node as EditorNode, NodeId, Page as EditorPage } from './editorSchema';
import { editorNodeFromFreeElement } from './editorSchema';
import type { CampaignBlockKind, CampaignPageSchema, ComponentRegistryKey, DeviceWidth, FreeDesignElement, LayoutKind } from './types';
import { ALLOWED_BLOCK_KINDS, ALLOWED_LAYOUT_KINDS, blocksToFreeElements } from './layout';
import { renderRuntimeNode } from './componentRegistry';

type CanvasRendererMode = 'design' | 'preview' | 'runtime';
type Placement = { parentId?: NodeId; index: number };
type DropIntent = 'before' | 'inside' | 'after';

function legacyNodes(page?: CampaignPageSchema, elements?: FreeDesignElement[]): EditorNode[] {
  if (page?.editor?.nodes.length) return page.editor.nodes;
  const source = elements ?? (page?.freeElements?.length ? page.freeElements : page ? blocksToFreeElements(page.blocks, page.device.defaultWidth) : []);
  const childrenByParent = source.reduce<Record<string, string[]>>((groups, element) => {
    if (!element.parentId) return groups;
    (groups[element.parentId] ??= []).push(element.id);
    return groups;
  }, {});
  return source.map((element) => editorNodeFromFreeElement(
    elements && !element.positioning ? { ...element, positioning: 'absolute', widthMode: 'fixed', heightMode: 'fixed' } : element,
    undefined,
    childrenByParent[element.id] ?? [],
  ));
}

function nodeStyle(node: EditorNode, nested: boolean): CSSProperties {
  const absolute = node.positioning === 'absolute';
  const style: CSSProperties = {
    position: absolute ? 'absolute' : 'relative',
    left: absolute ? node.frame.x : undefined,
    top: absolute ? node.frame.y : undefined,
    zIndex: absolute ? node.frame.z : undefined,
    width: node.widthMode === 'fill' ? '100%' : node.widthMode === 'hug' ? 'fit-content' : node.frame.width,
    minWidth: 0,
    height: node.heightMode === 'auto' ? 'auto' : node.frame.height,
    minHeight: node.heightMode === 'auto' ? Math.min(node.frame.height, 40) : 0,
    overflow: node.heightMode === 'scroll' ? 'auto' : 'visible',
    borderRadius: node.radius ? `${Number(node.radius) || 0}px` : undefined,
    boxSizing: 'border-box',
  };
  if (node.type === 'container') {
    Object.assign(style, {
      display: 'flex',
      flexDirection: node.flexDirection,
      justifyContent: node.justifyContent,
      alignItems: node.alignItems,
      flexWrap: node.flexWrap,
      gap: node.gap,
      padding: nested ? 10 : 12,
    });
  }
  return style;
}

function canvasHeight(nodes: EditorNode[], embedded: boolean) {
  const absoluteBottom = nodes
    .filter((node) => !node.parentId && node.positioning === 'absolute')
    .reduce((bottom, node) => Math.max(bottom, node.frame.y + node.frame.height + 24), 0);
  return Math.max(embedded ? 520 : 760, absoluteBottom);
}

function visualKind(node: EditorNode) {
  if (node.type === 'container') return 'layout';
  if (node.component === 'Text' || node.component === 'Title') return 'text';
  if (node.component === 'Button' || node.component === 'ClaimButton') return 'button';
  if (node.component === 'Image') return 'image';
  return 'card';
}

function dropPayload(event: ReactDragEvent<HTMLElement>, fallback: {
  block?: CampaignBlockKind | null;
  layout?: LayoutKind | null;
  component?: ComponentRegistryKey | null;
}) {
  return {
    nodeId: event.dataTransfer.getData('application/x-campaign-node') || event.dataTransfer.getData('application/x-campaign-tree-free-id'),
    block: (event.dataTransfer.getData('application/x-campaign-block') || fallback.block) as CampaignBlockKind,
    layout: (event.dataTransfer.getData('application/x-campaign-layout') || fallback.layout) as LayoutKind,
    component: (event.dataTransfer.getData('application/x-campaign-component') || fallback.component) as ComponentRegistryKey,
  };
}

export function CanvasRenderer({
  page,
  editorPage,
  mode,
  width,
  elements,
  selectedNodeId,
  hoveredNodeId,
  draggingBlockKind,
  draggingLayoutKind,
  draggingComponentKey,
  onSelectNode,
  onHoverNode,
  onDropBlock,
  onDropLayout,
  onDropComponent,
  onPlaceNode,
  onDragDone,
  onLayoutDragDone,
  onPointerDownNode,
  onResizePointerDownNode,
  onPointerDownElement,
  onPointerDownNested,
  onResizePointerDown,
  overlay,
  embedded = false,
}: {
  page?: CampaignPageSchema;
  editorPage?: EditorPage;
  mode: CanvasRendererMode;
  width?: DeviceWidth;
  elements?: FreeDesignElement[];
  selectedNodeId?: string;
  hoveredNodeId?: string;
  draggingBlockKind?: CampaignBlockKind | null;
  draggingLayoutKind?: LayoutKind | null;
  draggingComponentKey?: ComponentRegistryKey | null;
  onSelectNode?: (id: string) => void;
  onHoverNode?: (id?: string) => void;
  onDropBlock?: (kind: CampaignBlockKind, x: number, y: number, parentId?: string, index?: number) => void;
  onDropLayout?: (kind: LayoutKind, x: number, y: number, parentId?: string, index?: number) => void;
  onDropComponent?: (componentKey: ComponentRegistryKey, x: number, y: number, parentId?: string, index?: number) => void;
  onPlaceNode?: (nodeId: string, parentId?: string, index?: number) => void;
  onDragDone?: () => void;
  onLayoutDragDone?: () => void;
  onPointerDownNode?: (event: ReactPointerEvent<HTMLElement>, node: EditorNode) => void;
  onResizePointerDownNode?: (event: ReactPointerEvent<HTMLSpanElement>, node: EditorNode) => void;
  /** @deprecated Used only by the legacy free-canvas adapter. */
  onPointerDownElement?: (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => void;
  /** @deprecated Used only by the legacy free-canvas adapter. */
  onPointerDownNested?: (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => void;
  /** @deprecated Used only by the legacy free-canvas adapter. */
  onResizePointerDown?: (event: ReactPointerEvent<HTMLSpanElement>, element: FreeDesignElement) => void;
  overlay?: ReactNode;
  embedded?: boolean;
}) {
  const schema = editorPage ?? page?.editor;
  const nodes = legacyNodes(page, elements).filter((node) => mode === 'design' || node.visible);
  const nodesById = new Map(nodes.map((node) => [node.id, node]));
  const rootIds = (schema?.rootNodeIds ?? nodes.filter((node) => !node.parentId).map((node) => node.id)).filter((id) => nodesById.has(id));
  const runtime = mode === 'runtime';
  const design = mode === 'design';
  const canvasWidth = width ?? page?.device.defaultWidth ?? schema?.device.defaultWidth ?? 393;
  const height = canvasHeight(nodes, embedded);
  const canvasClass = runtime
    ? `campaign-runtime-canvas${embedded ? ' embedded-preview' : ''}`
    : design
      ? 'campaign-free-canvas campaign-flow-canvas'
      : 'campaign-free-canvas campaign-mobile-canvas campaign-module-preview-canvas campaign-flow-canvas';
  const fallback = { block: draggingBlockKind, layout: draggingLayoutKind, component: draggingComponentKey };
  const [draggingNodeId, setDraggingNodeId] = useState<string>();
  const [dropTarget, setDropTarget] = useState<{ nodeId: string; intent: DropIntent }>();
  const dragActive = design && Boolean(draggingNodeId || draggingBlockKind || draggingLayoutKind || draggingComponentKey);

  const elementsById = new Map((elements ?? []).map((element) => [element.id, element]));
  const completeDrop = (event: ReactDragEvent<HTMLElement>, placement: Placement) => {
    event.preventDefault();
    event.stopPropagation();
    const payload = dropPayload(event, fallback);
    if (payload.nodeId) onPlaceNode?.(payload.nodeId, placement.parentId, placement.index);
    else if (ALLOWED_BLOCK_KINDS.has(payload.block)) onDropBlock?.(payload.block, 0, 0, placement.parentId, placement.index);
    else if (ALLOWED_LAYOUT_KINDS.has(payload.layout)) onDropLayout?.(payload.layout, 0, 0, placement.parentId, placement.index);
    else if (payload.component) onDropComponent?.(payload.component, 0, 0, placement.parentId, placement.index);
    onDragDone?.();
    onLayoutDragDone?.();
    setDraggingNodeId(undefined);
    setDropTarget(undefined);
  };

  const placementForTarget = (event: ReactDragEvent<HTMLElement>, node: EditorNode): { placement: Placement; intent: DropIntent } => {
    const parent = node.parentId ? nodesById.get(node.parentId) : undefined;
    const siblings = parent?.type === 'container' ? parent.children : rootIds;
    const index = Math.max(0, siblings.indexOf(node.id));
    const rect = event.currentTarget.getBoundingClientRect();
    const horizontal = parent?.type === 'container' && parent.flexDirection === 'row';
    const ratio = horizontal
      ? (event.clientX - rect.left) / Math.max(1, rect.width)
      : (event.clientY - rect.top) / Math.max(1, rect.height);
    if (node.type === 'container' && ratio >= 0.25 && ratio <= 0.75) return { placement: { parentId: node.id, index: node.children.length }, intent: 'inside' };
    const after = ratio > 0.5;
    return { placement: { parentId: node.parentId, index: index + (after ? 1 : 0) }, intent: after ? 'after' : 'before' };
  };

  const DropSlot = ({ placement, edge, axis = 'column' }: { placement: Placement; edge: 'before' | 'after'; axis?: 'row' | 'column' }) => dragActive ? (
    <div
      className={`campaign-canvas-drop-slot ${edge} ${axis}`}
      onDragOver={(event) => {
        event.preventDefault();
        event.stopPropagation();
        event.dataTransfer.dropEffect = event.dataTransfer.types.includes('application/x-campaign-node') ? 'move' : 'copy';
      }}
      onDrop={(event) => completeDrop(event, placement)}
      aria-hidden="true"
    />
  ) : null;

  const renderNode = (nodeId: NodeId, nested = false): ReactNode => {
    const node = nodesById.get(nodeId);
    if (!node) return null;
    const childIds = node.type === 'container' ? node.children.filter((id) => nodesById.has(id)) : [];
    const childContent = node.type === 'container' ? (
      <>
        {childIds.map((childId, index) => (
          <div className={`campaign-canvas-child-slot ${node.flexDirection} ${nodesById.get(childId)?.positioning ?? 'flow'}`} key={childId}>
            {nodesById.get(childId)?.positioning === 'flow' ? <DropSlot placement={{ parentId: node.id, index }} edge="before" axis={node.flexDirection} /> : null}
            {renderNode(childId, true)}
            {index === childIds.length - 1 && nodesById.get(childId)?.positioning === 'flow' ? <DropSlot placement={{ parentId: node.id, index: childIds.length }} edge="after" axis={node.flexDirection} /> : null}
          </div>
        ))}
        {!childIds.length && design ? <span className="campaign-container-placeholder">拖入组件</span> : null}
      </>
    ) : null;
    const targetIntent = dropTarget?.nodeId === node.id ? dropTarget.intent : undefined;
    const className = `${design ? 'campaign-free-element' : 'campaign-runtime-element'} ${visualKind(node)} ${node.type === 'container' ? node.layout : node.component} ${nested ? 'nested' : ''} ${node.positioning} ${node.type === 'component' ? node.content.variant ?? 'primary' : ''} ${dragActive && node.type === 'container' ? 'accepting-drop' : ''} ${targetIntent ? `drop-${targetIntent}` : ''} ${design && selectedNodeId === node.id ? 'selected' : ''} ${design && hoveredNodeId === node.id ? 'hovered' : ''}`;
    return (
      <div
        key={node.id}
        className={className}
        data-node-id={node.id}
        data-action={node.actionBindings[0]?.actionId ?? ''}
        style={nodeStyle(node, nested)}
        role={runtime ? undefined : 'button'}
        tabIndex={design ? 0 : undefined}
        draggable={design && !node.locked && node.positioning === 'flow'}
        onDragStart={design ? (event) => {
          event.stopPropagation();
          setDraggingNodeId(node.id);
          event.dataTransfer.setData('application/x-campaign-node', node.id);
          event.dataTransfer.effectAllowed = 'move';
        } : undefined}
        onDragEnd={design ? () => { setDraggingNodeId(undefined); setDropTarget(undefined); } : undefined}
        onPointerDown={design && node.positioning === 'absolute' ? (event) => {
          onPointerDownNode?.(event, node);
          const legacyElement = elementsById.get(node.id);
          if (legacyElement) (nested ? onPointerDownNested : onPointerDownElement)?.(event, legacyElement);
        } : undefined}
        onMouseEnter={design ? (event) => { event.stopPropagation(); onHoverNode?.(node.id); } : undefined}
        onMouseLeave={design ? () => onHoverNode?.() : undefined}
        onClick={design ? (event) => { event.stopPropagation(); onSelectNode?.(node.id); } : undefined}
        onDragOver={dragActive ? (event) => {
          event.preventDefault();
          event.stopPropagation();
          const target = placementForTarget(event, node);
          setDropTarget({ nodeId: node.id, intent: target.intent });
          event.dataTransfer.dropEffect = event.dataTransfer.types.includes('application/x-campaign-node') ? 'move' : 'copy';
        } : undefined}
        onDragLeave={dragActive ? (event) => {
          if (!event.currentTarget.contains(event.relatedTarget as globalThis.Node | null)) setDropTarget(undefined);
        } : undefined}
        onDrop={dragActive ? (event) => completeDrop(event, placementForTarget(event, node).placement) : undefined}
      >
        {renderRuntimeNode(node, childContent)}
        {design && node.positioning === 'absolute' && selectedNodeId === node.id && (onResizePointerDownNode || onResizePointerDown) ? (
          <span className="campaign-free-resize" aria-label="调整尺寸" onPointerDown={(event) => {
            onResizePointerDownNode?.(event, node);
            const legacyElement = elementsById.get(node.id);
            if (legacyElement) onResizePointerDown?.(event, legacyElement);
          }} />
        ) : null}
      </div>
    );
  };

  return (
    <section className={runtime ? undefined : 'campaign-free-workspace'}>
      <div
        className={canvasClass}
        style={{ width: canvasWidth, minHeight: height }}
        onClick={design ? () => onSelectNode?.('') : undefined}
        onDragOver={dragActive ? (event) => event.preventDefault() : undefined}
        onDrop={dragActive ? (event) => completeDrop(event, { index: rootIds.length }) : undefined}
      >
        {overlay}
        {rootIds.map((id, index) => (
          <div className={`campaign-canvas-root-slot ${nodesById.get(id)?.positioning ?? 'flow'}`} key={id}>
            {nodesById.get(id)?.positioning === 'flow' ? <DropSlot placement={{ index }} edge="before" /> : null}
            {renderNode(id)}
            {index === rootIds.length - 1 && nodesById.get(id)?.positioning === 'flow' ? <DropSlot placement={{ index: rootIds.length }} edge="after" /> : null}
          </div>
        ))}
      </div>
    </section>
  );
}
