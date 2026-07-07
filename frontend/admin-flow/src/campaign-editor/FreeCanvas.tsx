import { Copy, Gift, Image, Plane, Plus, Trash2, Type } from 'lucide-react';
import { useState, type PointerEvent as ReactPointerEvent } from 'react';
import type { CampaignBlockKind, ComponentRegistryKey, DeviceWidth, FreeDesignElement, LayoutKind } from './types';
import { findContainingLayout, fitLayouts, layoutChildWidth } from './layout';
import { CanvasRenderer } from './CanvasRenderer';

export function AddBetween({ onClick }: { onClick: () => void }) {
  return <button className="campaign-between-add" type="button" onClick={onClick}><Plus size={14} />添加内容块</button>;
}

export function FreeLayerPanel({
  elements,
  selectedId,
  onSelect,
  onAdd,
  onDuplicate,
  onDelete,
}: {
  elements: FreeDesignElement[];
  selectedId: string;
  onSelect: (id: string) => void;
  onAdd: (kind: FreeDesignElement['kind']) => void;
  onDuplicate: () => void;
  onDelete: () => void;
}) {
  const sortedElements = [...elements].sort((a, b) => b.z - a.z);
  return (
    <aside className="campaign-outline campaign-free-layers">
      <h2>自由图层</h2>
      <p>点击图层选中元素，在画布拖拽移动，拖右下角调整尺寸</p>
      <div className="campaign-free-layer-actions">
        <button type="button" onClick={() => onAdd('text')}><Type size={16} />文本</button>
        <button type="button" onClick={() => onAdd('button')}><Plane size={16} />按钮</button>
        <button type="button" onClick={() => onAdd('card')}><Gift size={16} />卡片</button>
        <button type="button" onClick={() => onAdd('image')}><Image size={16} />图片</button>
      </div>
      <div className="campaign-free-layer-list">
        {sortedElements.map((element) => (
          <button key={element.id} className={selectedId === element.id ? 'active' : ''} type="button" onClick={() => onSelect(element.id)}>
            <span>{element.kind === 'text' ? <Type size={18} /> : element.kind === 'button' ? <Plane size={18} /> : element.kind === 'image' ? <Image size={18} /> : <Gift size={18} />}</span>
            <strong>{element.text.split('\\n')[0]}</strong>
            <small>{element.w} x {element.h} · z{element.z}</small>
          </button>
        ))}
      </div>
      <div className="campaign-free-layer-footer">
        <button type="button" onClick={onDuplicate}><Copy size={16} />复制</button>
        <button type="button" onClick={onDelete}><Trash2 size={16} />删除</button>
      </div>
    </aside>
  );
}

export function FreeDesignCanvas({
  width,
  elements,
  selectedId,
  onSelect,
  onChange,
  draggingBlockKind,
  draggingLayoutKind,
  draggingComponentKey,
  onDropBlock,
  onDropLayout,
  onDropComponent,
  onDragDone,
  onLayoutDragDone,
  onMoveElement,
  onResizeElement,
  onMoveIntoLayout,
}: {
  width: DeviceWidth;
  elements: FreeDesignElement[];
  selectedId: string;
  onSelect: (id: string) => void;
  onChange: (updater: (current: FreeDesignElement[]) => FreeDesignElement[]) => void;
  draggingBlockKind: CampaignBlockKind | null;
  draggingLayoutKind: LayoutKind | null;
  draggingComponentKey?: ComponentRegistryKey | null;
  onDropBlock: (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => void;
  onDropLayout: (kind: LayoutKind, x: number, y: number) => void;
  onDropComponent?: (componentKey: ComponentRegistryKey, x: number, y: number, parentId?: string) => void;
  onDragDone: () => void;
  onLayoutDragDone: () => void;
  onMoveElement?: (id: string, x: number, y: number) => void;
  onResizeElement?: (id: string, w: number, h: number) => void;
  onMoveIntoLayout?: (id: string, layoutId: string) => void;
}) {
  const [guides, setGuides] = useState<{ x?: number; y?: number }>({});
  const nearestGuide = (value: number, targets: number[]) => targets.reduce((nearest, target) => Math.abs(target - value) < Math.abs(nearest - value) ? target : nearest, targets[0]);

  const startDrag = (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => {
    const startX = event.clientX;
    const startY = event.clientY;
    const original = { x: element.x, y: element.y };
    let latest = original;
    event.currentTarget.setPointerCapture(event.pointerId);
    onSelect(element.id);
    const move = (moveEvent: PointerEvent) => {
      const parent = element.parentId ? elements.find((item) => item.id === element.parentId) : undefined;
      const maxX = Math.max(0, (parent?.w ?? width) - element.w);
      const nextX = Math.min(maxX, Math.max(0, original.x + moveEvent.clientX - startX));
      const nextY = Math.max(0, original.y + moveEvent.clientY - startY);
      latest = { x: Math.round(nextX), y: Math.round(nextY) };
      const verticalGuide = nearestGuide(nextX, [0, Math.round(maxX / 2), maxX]);
      const maxY = parent ? Math.max(0, parent.h - element.h) : 320;
      const horizontalGuide = nearestGuide(nextY, [0, Math.round(maxY / 2), maxY]);
      setGuides({
        x: (parent?.x ?? 0) + verticalGuide + element.w / 2,
        y: (parent?.y ?? 0) + horizontalGuide,
      });
      if (onMoveElement) {
        onMoveElement(element.id, latest.x, latest.y);
      } else {
        onChange((items) => items.map((item) => item.id === element.id ? { ...item, ...latest } : item));
      }
    };
    const end = () => {
      setGuides({});
      if (element.kind !== 'layout') {
        const centerX = latest.x + element.w / 2;
        const centerY = latest.y + element.h / 2;
        const parent = findContainingLayout(elements.filter((item) => item.id !== element.id), centerX, centerY);
        if (parent) {
          const localX = Math.max(0, Math.min(Math.max(0, parent.w - element.w), Math.round(latest.x - parent.x)));
          if (onMoveIntoLayout) {
            onMoveIntoLayout(element.id, parent.id);
          } else {
            onChange((items) => fitLayouts(items.map((item) => item.id === element.id ? {
              ...item,
              parentId: parent.id,
              x: localX,
              y: Math.max(0, Math.round(latest.y - parent.y)),
              w: layoutChildWidth(parent),
            } : item)));
          }
        }
      }
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  const startResize = (event: ReactPointerEvent<HTMLSpanElement>, element: FreeDesignElement) => {
    event.stopPropagation();
    const startX = event.clientX;
    const startY = event.clientY;
    const original = { w: element.w, h: element.h };
    event.currentTarget.setPointerCapture(event.pointerId);
    onSelect(element.id);
    const move = (moveEvent: PointerEvent) => {
      const nextW = Math.max(0, original.w + moveEvent.clientX - startX);
      const nextH = Math.max(0, original.h + moveEvent.clientY - startY);
      if (onResizeElement) {
        onResizeElement(element.id, Math.round(nextW), Math.round(nextH));
      } else {
        onChange((items) => items.map((item) => item.id === element.id ? { ...item, w: Math.round(nextW), h: Math.round(nextH) } : item));
      }
    };
    const end = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  return (
    <section className="campaign-free-workspace">
      <CanvasRenderer
        mode="design"
        width={width}
        elements={elements}
        selectedNodeId={selectedId}
        draggingBlockKind={draggingBlockKind}
        draggingLayoutKind={draggingLayoutKind}
        draggingComponentKey={draggingComponentKey}
        onSelectNode={onSelect}
        onDropBlock={(kind, x, y) => {
          const parent = findContainingLayout(elements, x, y);
          onDropBlock(kind, x, y, parent?.id);
        }}
        onDropLayout={onDropLayout}
        onDropComponent={(componentKey, x, y) => {
          const parent = findContainingLayout(elements, x, y);
          onDropComponent?.(componentKey, x, y, parent?.id);
        }}
        onDragDone={onDragDone}
        onLayoutDragDone={onLayoutDragDone}
        onPointerDownElement={(event, element) => startDrag(event, element)}
        onPointerDownNested={(event, child) => {
          event.stopPropagation();
          if (selectedId === child.id) {
            startDrag(event, child);
            return;
          }
          onSelect(child.id);
        }}
        onResizePointerDown={(event, element) => startResize(event, element)}
        overlay={(
          <>
            {guides.x !== undefined ? <span className="campaign-guide vertical" style={{ left: guides.x }} /> : null}
            {guides.y !== undefined ? <span className="campaign-guide horizontal" style={{ top: guides.y }} /> : null}
          </>
        )}
      />
      <div className="campaign-mobile-check">自由设计：元素可拖拽定位，保存到 Campaign Page schema。</div>
    </section>
  );
}
