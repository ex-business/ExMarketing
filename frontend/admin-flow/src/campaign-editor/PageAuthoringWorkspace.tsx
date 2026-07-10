import type { PointerEvent as ReactPointerEvent } from 'react';
import { ChevronDown, Copy, Eye, FileText, Gift, HelpCircle, Image, ListChecks, MonitorSmartphone, Plane, Rows3, Trash2, Type } from 'lucide-react';
import type { EditorNodePatch } from './editorEngine';
import type { Node as EditorNode } from './editorSchema';
import { blockPalette, layoutPalette } from './layout';
import { componentRegistry } from './componentRegistry';
import { NodeInspector } from './Inspector';
import { CampaignBuildCanvas, CampaignModulePreview } from './PageWorkspace';
import type { CampaignBlockKind, CampaignPageSchema, ComponentRegistryKey, DeviceWidth, EditorTab, LayoutKind, ResourceRecord } from './types';
import { Button, SegmentedControl, Tabs, Tree, type TreeNode } from './ui';

type PageEditorMode = 'edit' | 'preview';
type LeftPageTab = 'structure' | 'add';

const blockIcons: Record<CampaignBlockKind, typeof Image> = {
  HeroBanner: Image,
  Title: Type,
  RewardCard: Gift,
  Countdown: MonitorSmartphone,
  ProgressBlock: ListChecks,
  ClaimButton: Plane,
  RuleText: FileText,
  FAQBlock: HelpCircle,
};

export type PageAuthoringWorkspaceProps = {
  pageName: string;
  previewPage: CampaignPageSchema;
  deviceWidth: DeviceWidth;
  mode: PageEditorMode;
  leftTab: LeftPageTab;
  treeQuery: string;
  treeOpen: boolean;
  treeNodes: TreeNode[];
  selectedNode?: EditorNode;
  selectedNodeId: string;
  hoveredNodeId?: string;
  inspectorTab: EditorTab;
  resources: ResourceRecord[];
  draggingBlockKind: CampaignBlockKind | null;
  draggingLayoutKind: LayoutKind | null;
  draggingComponentKey: ComponentRegistryKey | null;
  alignmentGuides: { x?: number; y?: number };
  onModeChange: (mode: PageEditorMode) => void;
  onLeftTabChange: (tab: LeftPageTab) => void;
  onTreeQueryChange: (query: string) => void;
  onTreeOpenChange: (open: boolean) => void;
  onMoveNodeToRoot: (nodeId: string) => void;
  onSelectNode: (nodeId: string) => void;
  onHoverNode: (nodeId?: string) => void;
  onAddBlock: (kind: CampaignBlockKind, x?: number, y?: number, parentId?: string, index?: number) => void;
  onAddLayout: (kind: LayoutKind, x?: number, y?: number, parentId?: string, index?: number) => void;
  onAddComponent: (key: ComponentRegistryKey, x?: number, y?: number, parentId?: string, index?: number) => void;
  onPlaceNode: (nodeId: string, parentId?: string, index?: number) => void;
  onDraggingBlockChange: (kind: CampaignBlockKind | null) => void;
  onDraggingLayoutChange: (kind: LayoutKind | null) => void;
  onDraggingComponentChange: (key: ComponentRegistryKey | null) => void;
  onPointerDownNode: (event: ReactPointerEvent<HTMLElement>, node: EditorNode) => void;
  onResizePointerDownNode: (event: ReactPointerEvent<HTMLSpanElement>, node: EditorNode) => void;
  onDuplicateNode: () => void;
  onDeleteNode: () => void;
  onInspectorTabChange: (tab: EditorTab) => void;
  onNodeChange: (patch: EditorNodePatch) => void;
  onReorderNode: (direction: -1 | 1) => void;
};

export function PageAuthoringWorkspace(props: PageAuthoringWorkspaceProps) {
  const preview = props.mode === 'preview';
  const dropToRoot = (event: React.DragEvent<HTMLElement>) => {
    if (preview || !event.dataTransfer.types.includes('application/x-campaign-node')) return;
    event.preventDefault();
    const nodeId = event.dataTransfer.getData('application/x-campaign-node');
    if (nodeId) props.onMoveNodeToRoot(nodeId);
  };
  return (
    <>
      <aside className="campaign-outline">
        <h2>{props.leftTab === 'structure' ? '页面结构' : '添加组件'}</h2>
        <Tabs value={props.leftTab === 'structure' ? '结构' : '添加'} items={['结构', '添加']} onChange={(tab) => props.onLeftTabChange(tab === '结构' ? 'structure' : 'add')} ariaLabel="页面左侧面板" />
        {props.leftTab === 'structure' ? (
          <>
            <label className="campaign-tree-search"><input value={props.treeQuery} onChange={(event) => props.onTreeQueryChange(event.target.value)} placeholder="搜索节点" aria-label="搜索页面树" /></label>
            <button className="campaign-page-tree-root" type="button" aria-expanded={props.treeOpen} onDragOver={(event) => { if (!preview && event.dataTransfer.types.includes('application/x-campaign-node')) event.preventDefault(); }} onDrop={dropToRoot} onClick={() => props.onTreeOpenChange(!props.treeOpen)}>
              <ChevronDown size={15} className={props.treeOpen ? 'open' : ''} /><FileText size={18} /><span><strong>{props.pageName}</strong></span>
            </button>
            {props.treeOpen ? <div className="campaign-outline-list" onDragOver={(event) => { if (!preview && event.dataTransfer.types.includes('application/x-campaign-node')) event.preventDefault(); }} onDrop={dropToRoot}><Tree nodes={props.treeNodes} className="campaign-outline-tree" rowClassName="campaign-outline-row" childrenClassName="campaign-outline-children" noWrapper />{props.treeNodes.length ? null : <p className="campaign-tree-empty">没有匹配节点</p>}</div> : null}
          </>
        ) : (
          <>
            <p className="campaign-panel-hint">{preview ? '预览模式为只读，切回编辑后添加组件。' : '点击插入到当前节点之后；选中容器时插入容器内。也可拖到插入线。'}</p>
            <div className="campaign-block-palette" aria-label="组件库">
              {blockPalette.map((item) => {
                const Icon = blockIcons[item.kind];
                return <button key={item.kind} type="button" disabled={preview} draggable={!preview} onClick={() => !preview && props.onAddBlock(item.kind)} onDragStart={(event) => { if (preview) return event.preventDefault(); props.onDraggingBlockChange(item.kind); event.dataTransfer.setData('application/x-campaign-block', item.kind); event.dataTransfer.effectAllowed = 'copy'; }} onDragEnd={() => props.onDraggingBlockChange(null)}><Icon size={16} />{item.label}</button>;
              })}
              <button type="button" disabled={preview} draggable={!preview} onClick={() => !preview && props.onAddComponent('Image')} onDragStart={(event) => { if (preview) return event.preventDefault(); props.onDraggingComponentChange('Image'); event.dataTransfer.setData('application/x-campaign-component', 'Image'); event.dataTransfer.effectAllowed = 'copy'; }} onDragEnd={() => props.onDraggingComponentChange(null)}><Image size={16} />{componentRegistry.Image.paletteLabel}</button>
              {layoutPalette.map((item) => <button key={item.kind} type="button" disabled={preview} draggable={!preview} onClick={() => !preview && props.onAddLayout(item.kind)} onDragStart={(event) => { if (preview) return event.preventDefault(); props.onDraggingLayoutChange(item.kind); event.dataTransfer.setData('application/x-campaign-layout', item.kind); event.dataTransfer.effectAllowed = 'copy'; }} onDragEnd={() => props.onDraggingLayoutChange(null)}><Rows3 size={16} />{item.label}</button>)}
            </div>
          </>
        )}
      </aside>

      <section className="campaign-canvas-workspace">
        <div className="campaign-toolbar"><Tabs value={props.mode === 'edit' ? '编辑' : '预览'} items={['编辑', '预览']} onChange={(mode) => props.onModeChange(mode === '编辑' ? 'edit' : 'preview')} ariaLabel="页面编辑模式" /></div>
        {props.mode === 'edit' ? (
          <CampaignBuildCanvas page={props.previewPage} selectedNodeId={props.selectedNodeId} hoveredNodeId={props.hoveredNodeId} draggingBlockKind={props.draggingBlockKind} draggingLayoutKind={props.draggingLayoutKind} draggingComponentKey={props.draggingComponentKey} onSelectNode={props.onSelectNode} onHoverNode={props.onHoverNode} onDropBlock={props.onAddBlock} onDropLayout={props.onAddLayout} onDropComponent={props.onAddComponent} onPlaceNode={props.onPlaceNode} onPointerDownNode={props.onPointerDownNode} onResizePointerDownNode={props.onResizePointerDownNode} alignmentGuides={props.alignmentGuides} onDragDone={() => props.onDraggingBlockChange(null)} onLayoutDragDone={() => { props.onDraggingLayoutChange(null); props.onDraggingComponentChange(null); }} />
        ) : <CampaignModulePreview page={props.previewPage} selectedId={props.selectedNodeId} onSelect={props.onSelectNode} />}
      </section>

      <aside className="campaign-inspector">
        {!preview ? (
          <>
            <header><div><Type size={20} /><h2>{props.selectedNode?.name || '页面节点'}</h2></div><span>{props.selectedNode?.resourceBindings.length ? '已绑定' : '未绑定'}</span><button type="button" aria-label="复制自由元素" onClick={props.onDuplicateNode}><Copy size={16} /></button><button type="button" aria-label="删除自由元素" onClick={props.onDeleteNode}><Trash2 size={16} /></button></header>
            <nav><SegmentedControl value={props.inspectorTab} items={[{ value: 'properties', label: '属性' }, { value: 'style', label: '样式' }, { value: 'data', label: '数据与动作' }]} onChange={props.onInspectorTabChange} ariaLabel="Inspector tabs" /></nav>
            <NodeInspector node={props.selectedNode} tab={props.inspectorTab} width={props.deviceWidth} resources={props.resources} onChange={props.onNodeChange} onReorder={props.onReorderNode} />
          </>
        ) : props.previewPage.editor.nodes.length ? <><header><div><Eye size={20} /><h2>预览模式</h2></div><span>只读</span></header><div className="campaign-inspector-body"><p>预览隐藏编辑辅助，只按当前草稿渲染。需要调整时切回编辑。</p></div></> : <div className="campaign-inspector-body"><p>当前页面没有内容，可以从左侧添加组件。</p></div>}
      </aside>
    </>
  );
}
