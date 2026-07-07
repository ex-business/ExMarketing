import type { PointerEvent as ReactPointerEvent, ReactNode } from 'react';
import type { CampaignBlockKind, CampaignPageSchema, ComponentRegistryKey, DeviceWidth, FreeDesignElement, LayoutKind } from './types';
import { ALLOWED_BLOCK_KINDS, ALLOWED_LAYOUT_KINDS, blockIdForFreeElementId, blocksToFreeElements, freeCanvasHeight, freeElementStyle } from './layout';
import { editorPageToFreeElements } from './editorSchema';
import { renderRuntimeComponent } from './componentRegistry';

type CanvasRendererMode = 'design' | 'preview' | 'runtime';

export function pageElementsForRenderer(page: CampaignPageSchema) {
  if (page.editor?.nodes.length) return editorPageToFreeElements(page.editor);
  if (page.freeElements?.length) return page.freeElements;
  return blocksToFreeElements(page.blocks, page.device.defaultWidth);
}

function groupsByParent(elements: FreeDesignElement[]) {
  return elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
}

function selectedMatches(elementId: string, selectedNodeId?: string) {
  if (!selectedNodeId) return false;
  return selectedNodeId === elementId || blockIdForFreeElementId(elementId) === selectedNodeId;
}

function selectionIdForElement(elementId: string) {
  return blockIdForFreeElementId(elementId) || elementId;
}

export function CanvasRenderer({
  page,
  mode,
  width,
  elements: explicitElements,
  selectedNodeId,
  draggingBlockKind,
  draggingLayoutKind,
  draggingComponentKey,
  onSelectNode,
  onDropBlock,
  onDropLayout,
  onDropComponent,
  onDragDone,
  onLayoutDragDone,
  onPointerDownElement,
  onPointerDownNested,
  onResizePointerDown,
  overlay,
  embedded = false,
}: {
  page?: CampaignPageSchema;
  mode: CanvasRendererMode;
  width?: DeviceWidth;
  elements?: FreeDesignElement[];
  selectedNodeId?: string;
  draggingBlockKind?: CampaignBlockKind | null;
  draggingLayoutKind?: LayoutKind | null;
  draggingComponentKey?: ComponentRegistryKey | null;
  onSelectNode?: (id: string) => void;
  onDropBlock?: (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => void;
  onDropLayout?: (kind: LayoutKind, x: number, y: number) => void;
  onDropComponent?: (componentKey: ComponentRegistryKey, x: number, y: number) => void;
  onDragDone?: () => void;
  onLayoutDragDone?: () => void;
  onPointerDownElement?: (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => void;
  onPointerDownNested?: (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => void;
  onResizePointerDown?: (event: ReactPointerEvent<HTMLSpanElement>, element: FreeDesignElement) => void;
  overlay?: ReactNode;
  embedded?: boolean;
}) {
  const elements = [...(explicitElements ?? (page ? pageElementsForRenderer(page) : []))].sort((a, b) => a.z - b.z);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = groupsByParent(elements);
  const isRuntime = mode === 'runtime';
  const isDesign = mode === 'design';
  const isInteractive = mode === 'design';
  const canvasWidth = width ?? page?.device.defaultWidth ?? 393;
  const canvasHeight = embedded || !isRuntime ? freeCanvasHeight(elements, embedded ? 0 : 760) : undefined;
  const canvasClass = isRuntime
    ? `campaign-runtime-canvas${embedded ? ' embedded-preview' : ''}`
    : isDesign
      ? 'campaign-free-canvas'
      : 'campaign-free-canvas campaign-mobile-canvas campaign-module-preview-canvas';
  const elementClass = isDesign ? 'campaign-free-element' : 'campaign-runtime-element';

  const selectElement = (id: string) => {
    if (isInteractive) onSelectNode?.(isDesign ? id : selectionIdForElement(id));
  };

  return (
    <section className={isRuntime ? undefined : 'campaign-free-workspace'}>
      <div
        className={canvasClass}
        style={{ width: canvasWidth, ...(canvasHeight ? { height: canvasHeight, minHeight: canvasHeight } : {}) }}
        onDragOver={!isRuntime && (onDropBlock || onDropLayout || onDropComponent) ? (event) => event.preventDefault() : undefined}
        onDrop={!isRuntime && (onDropBlock || onDropLayout || onDropComponent) ? (event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          const x = event.clientX - rect.left;
          const y = event.clientY - rect.top;
          const kind = (event.dataTransfer.getData('application/x-campaign-block') || draggingBlockKind) as CampaignBlockKind;
          const layoutKind = (event.dataTransfer.getData('application/x-campaign-layout') || draggingLayoutKind) as LayoutKind;
          const componentKey = (event.dataTransfer.getData('application/x-campaign-component') || draggingComponentKey) as ComponentRegistryKey;
          if (ALLOWED_BLOCK_KINDS.has(kind)) onDropBlock?.(kind, x, y);
          if (ALLOWED_LAYOUT_KINDS.has(layoutKind)) onDropLayout?.(layoutKind, x, y);
          if (componentKey) onDropComponent?.(componentKey, x, y);
          onDragDone?.();
          onLayoutDragDone?.();
        } : undefined}
      >
        {overlay}
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          const renderedChildren = childElements.length ? childElements.map((child) => (
            <span
              key={child.id}
              className={`${elementClass} nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${child.variant ?? 'primary'} ${isInteractive && selectedMatches(child.id, selectedNodeId) ? 'selected' : ''}`}
              style={freeElementStyle(child, true)}
              onPointerDown={isDesign ? (event) => onPointerDownNested?.(event, child) : undefined}
              onClick={isInteractive ? (event) => {
                event.stopPropagation();
                selectElement(child.id);
              } : undefined}
            >
              {renderRuntimeComponent(child, null)}
            </span>
          )) : null;
          return (
            <div
              key={element.id}
              className={`${elementClass} ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${element.variant ?? 'primary'} ${isInteractive && selectedMatches(element.id, selectedNodeId) ? 'selected' : ''}`}
              role={isRuntime ? undefined : 'button'}
              tabIndex={isDesign ? 0 : undefined}
              data-action={element.action ?? ''}
              style={freeElementStyle(element)}
              onPointerDown={isDesign ? (event) => onPointerDownElement?.(event, element) : undefined}
              onClick={isInteractive ? () => selectElement(element.id) : undefined}
            >
              {renderRuntimeComponent(element, renderedChildren)}
              {isDesign && selectedMatches(element.id, selectedNodeId) ? <span className="campaign-free-resize" aria-label="调整尺寸" onPointerDown={(event) => onResizePointerDown?.(event, element)} /> : null}
            </div>
          );
        })}
      </div>
    </section>
  );
}
