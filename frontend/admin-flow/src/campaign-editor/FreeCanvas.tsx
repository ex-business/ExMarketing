import { Copy, Gift, Image, Plane, Plus, Trash2, Type } from 'lucide-react';
import { useState, type PointerEvent as ReactPointerEvent } from 'react';
import type { CampaignBlockKind, DeviceWidth, FreeDesignElement, LayoutKind } from './types';
import { ALLOWED_BLOCK_KINDS, ALLOWED_LAYOUT_KINDS, findContainingLayout, fitLayouts, freeCanvasHeight, freeElementStyle, layoutChildWidth } from './layout';
import { renderFreeElementText } from './renderUtils';

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
  onDropBlock,
  onDropLayout,
  onDragDone,
  onLayoutDragDone,
}: {
  width: DeviceWidth;
  elements: FreeDesignElement[];
  selectedId: string;
  onSelect: (id: string) => void;
  onChange: (updater: (current: FreeDesignElement[]) => FreeDesignElement[]) => void;
  draggingBlockKind: CampaignBlockKind | null;
  draggingLayoutKind: LayoutKind | null;
  onDropBlock: (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => void;
  onDropLayout: (kind: LayoutKind, x: number, y: number) => void;
  onDragDone: () => void;
  onLayoutDragDone: () => void;
}) {
  const [guides, setGuides] = useState<{ x?: number; y?: number }>({});
  const nearestGuide = (value: number, targets: number[]) => targets.reduce((nearest, target) => Math.abs(target - value) < Math.abs(nearest - value) ? target : nearest, targets[0]);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  const canvasWidth = width;
  const canvasHeight = freeCanvasHeight(elements);

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
      onChange((items) => items.map((item) => item.id === element.id ? { ...item, ...latest } : item));
    };
    const end = () => {
      setGuides({});
      if (element.kind !== 'layout') {
        const centerX = latest.x + element.w / 2;
        const centerY = latest.y + element.h / 2;
        const parent = findContainingLayout(elements.filter((item) => item.id !== element.id), centerX, centerY);
        if (parent) {
          const localX = Math.max(0, Math.min(Math.max(0, parent.w - element.w), Math.round(latest.x - parent.x)));
          onChange((items) => fitLayouts(items.map((item) => item.id === element.id ? {
            ...item,
            parentId: parent.id,
            x: localX,
            y: Math.max(0, Math.round(latest.y - parent.y)),
            w: layoutChildWidth(parent),
          } : item)));
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
      onChange((items) => items.map((item) => item.id === element.id ? { ...item, w: Math.round(nextW), h: Math.round(nextH) } : item));
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
      <div
        className="campaign-free-canvas"
        style={{ width: canvasWidth, height: canvasHeight }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          const blockKind = (event.dataTransfer.getData('application/x-campaign-block') || draggingBlockKind) as CampaignBlockKind;
          const layoutKind = (event.dataTransfer.getData('application/x-campaign-layout') || draggingLayoutKind) as LayoutKind;
          if (ALLOWED_BLOCK_KINDS.has(blockKind)) {
            const x = event.clientX - rect.left;
            const y = event.clientY - rect.top;
            const parent = findContainingLayout(elements, x, y);
            onDropBlock(blockKind, x, y, parent?.id);
          }
          if (ALLOWED_LAYOUT_KINDS.has(layoutKind)) {
            onDropLayout(layoutKind, event.clientX - rect.left, event.clientY - rect.top);
          }
          onDragDone();
          onLayoutDragDone();
        }}
      >
        {guides.x !== undefined ? <span className="campaign-guide vertical" style={{ left: guides.x }} /> : null}
        {guides.y !== undefined ? <span className="campaign-guide horizontal" style={{ top: guides.y }} /> : null}
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          return (
          <div
            key={element.id}
            className={`campaign-free-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${selectedId === element.id ? 'selected' : ''}`}
            role="button"
            tabIndex={0}
            style={freeElementStyle(element)}
            onPointerDown={(event) => startDrag(event, element)}
          >
            {childElements.length ? childElements.map((child) => (
              <span
                key={child.id}
                className={`campaign-free-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${selectedId === child.id ? 'selected' : ''}`}
                style={freeElementStyle(child, true)}
                onPointerDown={(event) => {
                  event.stopPropagation();
                  if (selectedId === child.id) {
                    startDrag(event, child);
                    return;
                  }
                  onSelect(child.id);
                }}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelect(child.id);
                }}
              >
                {renderFreeElementText(child.text)}
              </span>
            )) : renderFreeElementText(element.text)}
            {selectedId === element.id ? <span className="campaign-free-resize" aria-label="调整尺寸" onPointerDown={(event) => startResize(event, element)} /> : null}
          </div>
          );
        })}
      </div>
      <div className="campaign-mobile-check">自由设计：元素可拖拽定位，保存到 Campaign Page schema。</div>
    </section>
  );
}
