import type { PointerEvent as ReactPointerEvent } from 'react';
import type { Node as EditorNode } from './editorSchema';
import type { CampaignBlockKind, CampaignPageSchema, ComponentRegistryKey, LayoutKind } from './types';
import { CanvasRenderer } from './CanvasRenderer';

export function CampaignModulePreview({
  page,
  selectedId,
  onSelect,
}: {
  page: CampaignPageSchema;
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  return (
    <CanvasRenderer
      page={page}
      mode="preview"
      selectedNodeId={selectedId}
      onSelectNode={onSelect}
    />
  );
}

export function CampaignBuildCanvas({
  page,
  selectedNodeId,
  draggingBlockKind,
  draggingLayoutKind,
  draggingComponentKey,
  hoveredNodeId,
  onSelectNode,
  onDropBlock,
  onDropLayout,
  onDropComponent,
  onPlaceNode,
  onHoverNode,
  onPointerDownNode,
  onResizePointerDownNode,
  alignmentGuides,
  onDragDone,
  onLayoutDragDone,
}: {
  page: CampaignPageSchema;
  selectedNodeId: string;
  draggingBlockKind: CampaignBlockKind | null;
  draggingLayoutKind: LayoutKind | null;
  draggingComponentKey: ComponentRegistryKey | null;
  hoveredNodeId?: string;
  onSelectNode: (id: string) => void;
  onDropBlock: (kind: CampaignBlockKind, x: number, y: number, parentId?: string, index?: number) => void;
  onDropLayout: (kind: LayoutKind, x: number, y: number, parentId?: string, index?: number) => void;
  onDropComponent: (componentKey: ComponentRegistryKey, x: number, y: number, parentId?: string, index?: number) => void;
  onPlaceNode: (nodeId: string, parentId?: string, index?: number) => void;
  onHoverNode: (nodeId?: string) => void;
  onPointerDownNode: (event: ReactPointerEvent<HTMLElement>, node: EditorNode) => void;
  onResizePointerDownNode: (event: ReactPointerEvent<HTMLSpanElement>, node: EditorNode) => void;
  alignmentGuides: { x?: number; y?: number };
  onDragDone: () => void;
  onLayoutDragDone: () => void;
}) {
  return (
    <CanvasRenderer
      page={page}
      mode="design"
      selectedNodeId={selectedNodeId}
      draggingBlockKind={draggingBlockKind}
      draggingLayoutKind={draggingLayoutKind}
      draggingComponentKey={draggingComponentKey}
      hoveredNodeId={hoveredNodeId}
      onSelectNode={onSelectNode}
      onDropBlock={onDropBlock}
      onDropLayout={onDropLayout}
      onDropComponent={onDropComponent}
      onPlaceNode={onPlaceNode}
      onHoverNode={onHoverNode}
      onPointerDownNode={onPointerDownNode}
      onResizePointerDownNode={onResizePointerDownNode}
      overlay={<>{alignmentGuides.x !== undefined ? <span className="campaign-guide vertical" style={{ left: alignmentGuides.x }} /> : null}{alignmentGuides.y !== undefined ? <span className="campaign-guide horizontal" style={{ top: alignmentGuides.y }} /> : null}</>}
      onDragDone={onDragDone}
      onLayoutDragDone={onLayoutDragDone}
    />
  );
}
