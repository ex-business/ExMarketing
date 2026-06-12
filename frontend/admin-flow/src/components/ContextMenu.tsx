import type { FlowContextMenu } from '../types';

type Props = {
  menu: FlowContextMenu;
  onOpenPicker: () => void;
  onAddNote: (x: number, y: number) => void;
  onPreview: () => void;
  onExport: () => void;
  onImport: () => void;
  onDuplicateNode: (id: string) => void;
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (ids: string[]) => void;
};

export function ContextMenu({
  menu,
  onOpenPicker,
  onAddNote,
  onPreview,
  onExport,
  onImport,
  onDuplicateNode,
  onDeleteNodes,
  onDeleteEdges,
}: Props) {
  return (
    <div className="context-menu" style={{ left: menu.x, top: menu.y }} onClick={(event) => event.stopPropagation()}>
      {menu.type === 'pane' ? (
        <>
          <button onClick={onOpenPicker}>添加节点</button>
          <button onClick={() => onAddNote(menu.flowX, menu.flowY)}>添加备注</button>
          <button onClick={onPreview}>预览</button>
          <button className="disabled" disabled><span>粘贴到这里</span><kbd>⌘ V</kbd></button>
          <div className="context-divider" />
          <button onClick={onExport}>导出 DSL</button>
          <button onClick={onImport}>导入应用</button>
        </>
      ) : null}
      {menu.type === 'node' ? (
        <>
          <div className="context-title">节点操作</div>
          <button onClick={() => onDuplicateNode(menu.nodeId)}>复制节点</button>
          <button className="danger" onClick={() => onDeleteNodes([menu.nodeId])}>删除节点</button>
        </>
      ) : null}
      {menu.type === 'edge' ? (
        <>
          <div className="context-title">连线操作</div>
          <button className="danger" onClick={() => onDeleteEdges([menu.edgeId])}>删除连线</button>
        </>
      ) : null}
    </div>
  );
}
