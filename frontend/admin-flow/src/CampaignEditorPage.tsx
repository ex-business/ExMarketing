import { useEffect, useMemo, useReducer, useState, type PointerEvent as ReactPointerEvent } from 'react';
import {
  Bell,
  ChevronDown,
  Copy,
  Database,
  Eye,
  FileText,
  HelpCircle,
  MoreHorizontal,
  Plane,
  Redo2,
  Settings,
  Sparkles,
  Trash2,
  Undo2,
} from 'lucide-react';
import './CampaignEditorPage.css';
import type {
  CampaignBlock,
  CampaignBlockKind,
  CampaignPageSchema,
  CampaignPageSummary,
  CampaignPageVersion,
  CampaignValidationIssue,
  ComponentRegistryKey,
  DeviceWidth,
  EditorTab,
  FreeDesignElement,
  LayoutKind,
  PageTreeNode,
  ResourceRecord,
  WorkspaceSection,
} from './campaign-editor/types';
import {
  blockPalette,
  blockToFreeElement,
  flexPreset,
  normalizeLayoutKind,
} from './campaign-editor/layout';
import { campaignPageApi, defaultCampaignPage, normalizeCampaignPage, nowISO, pageSummary, syncCampaignPageFromEditor, validateCampaignPage } from './campaign-editor/campaignPageStore';
import { editorNodeFromFreeElement, editorPageToFreeElements, type Node as EditorNode } from './campaign-editor/editorSchema';
import { createEditorEngineState, editorEngineReducer, type EditorEngineAction, type EditorNodePatch } from './campaign-editor/editorEngine';
import { componentKeyForLayoutKind, componentRegistry, defaultElementFromComponent } from './campaign-editor/componentRegistry';
import { resourceStorageApi } from './campaign-editor/resourceStore';
import { defaultResourceRecords } from './campaign-editor/ResourceWorkspace';
import { CampaignWorkspace } from './campaign-editor/SecondaryWorkspace';
import { PageAuthoringWorkspace } from './campaign-editor/PageAuthoringWorkspace';
import { CampaignRuntimeView } from './campaign-editor/RuntimeView';
import { Button, type TreeNode } from './campaign-editor/ui';
export { CampaignRuntimePage } from './campaign-editor/RuntimeView';

type PageEditorMode = 'edit' | 'preview';
type LeftPageTab = 'structure' | 'add';

const navItems = [
  { id: 'page', label: '页面', icon: FileText },
  { id: 'resources', label: '资源', icon: Database },
  { id: 'settings', label: '设置', icon: Settings },
  { id: 'templates', label: '模板库', icon: Sparkles },
];

const blockNodeNameBase: Record<CampaignBlockKind, string> = {
  HeroBanner: '头图',
  Title: '标题',
  RewardCard: '奖励卡片',
  Countdown: '倒计时',
  ProgressBlock: '参与进度',
  ClaimButton: '按钮',
  RuleText: '活动规则',
  FAQBlock: 'FAQ',
};

function uniqueNodeName(baseName: string, usedNames: Iterable<string>) {
  const base = baseName.replace(/\s+(?:副本|\d+)$/u, '').trim() || '节点';
  const used = new Set(Array.from(usedNames).filter(Boolean));
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base} ${index}`)) index += 1;
  return `${base} ${index}`;
}

export function CampaignEditorPage() {
  const initialPage = useMemo<CampaignPageSchema>(() => {
    const loadedPage = campaignPageApi.loadPage();
    return {
      ...loadedPage,
      designMode: 'controlled',
    };
  }, []);
  const [pageSchema, setPageSchema] = useState<CampaignPageSchema>(initialPage);
  const [editorState, dispatchEditor] = useReducer(editorEngineReducer, initialPage.editor, (editorPage) => createEditorEngineState(editorPage));
  const [resourceRecords, setResourceRecords] = useState<ResourceRecord[]>(() => resourceStorageApi.loadRecords(defaultResourceRecords));
  const [versions, setVersions] = useState<CampaignPageVersion[]>(() => campaignPageApi.loadVersions());
  const [pages, setPages] = useState<CampaignPageSummary[]>(() => campaignPageApi.loadPageList());
  const [inspectorTab, setInspectorTab] = useState<EditorTab>('properties');
  const [deviceWidth, setDeviceWidth] = useState<DeviceWidth>(pageSchema.device.defaultWidth);
  const [activeSection, setActiveSection] = useState<WorkspaceSection>('page');
  const [saveMessage, setSaveMessage] = useState('已自动保存');
  const [previewOpen, setPreviewOpen] = useState(false);
  const [validationIssues, setValidationIssues] = useState<CampaignValidationIssue[]>([]);
  const [pageEditorMode, setPageEditorMode] = useState<PageEditorMode>('edit');
  const [leftPageTab, setLeftPageTab] = useState<LeftPageTab>('structure');
  const [draggingBlockKind, setDraggingBlockKind] = useState<CampaignBlockKind | null>(null);
  const [draggingLayoutKind, setDraggingLayoutKind] = useState<LayoutKind | null>(null);
  const [draggingComponentKey, setDraggingComponentKey] = useState<ComponentRegistryKey | null>(null);
  const [treeQuery, setTreeQuery] = useState('');
  const [treeOpen, setTreeOpen] = useState(true);
  const [hoveredNodeId, setHoveredNodeId] = useState<string>();
  const [alignmentGuides, setAlignmentGuides] = useState<{ x?: number; y?: number }>({});
  const freeElements = useMemo(() => editorPageToFreeElements(editorState.page), [editorState.page]);
  const selectedFreeId = editorState.selectedNodeId ?? freeElements[0]?.id ?? '';
  const usedNodeNames = editorState.page.nodes.map((node) => node.name);
  const nextNodeName = (baseName: string, extraUsedNames: string[] = []) => uniqueNodeName(baseName, [...usedNodeNames, ...extraUsedNames]);
  const selectedNode = editorState.page.nodes.find((node) => node.id === selectedFreeId) ?? editorState.page.nodes[0];
  const treeNodes = useMemo<PageTreeNode[]>(() => {
    const keyword = treeQuery.trim().toLowerCase();
    const nodesById = new Map(editorState.page.nodes.map((node) => [node.id, node]));
    const buildNode = (nodeId: string): PageTreeNode | undefined => {
      const node = nodesById.get(nodeId);
      if (!node) return undefined;
      return {
        id: node.id,
        title: node.name,
        meta: node.type === 'container'
          ? normalizeLayoutKind(node.layout) === 'vertical-layout' ? 'VerticalLayout' : 'HorizontalLayout'
          : node.component,
        node,
        children: node.type === 'container'
          ? node.children.map(buildNode).filter((child): child is PageTreeNode => Boolean(child))
          : undefined,
      };
    };
    const nodes = editorState.page.rootNodeIds.map(buildNode).filter((node): node is PageTreeNode => Boolean(node));
    const matches = (node: PageTreeNode): boolean => `${node.title} ${node.meta}`.toLowerCase().includes(keyword);
    if (!keyword) return nodes;
    const filterNode = (node: PageTreeNode): PageTreeNode | undefined => {
      const children = node.children?.map(filterNode).filter((child): child is PageTreeNode => Boolean(child));
      return matches(node) || children?.length ? { ...node, children } : undefined;
    };
    return nodes.map(filterNode).filter((node): node is PageTreeNode => Boolean(node));
  }, [editorState.page, treeQuery]);

  const updatePage = (patch: Partial<CampaignPageSchema> | ((current: CampaignPageSchema) => CampaignPageSchema)) => {
    setPageSchema((current) => {
      const next = typeof patch === 'function' ? patch(current) : { ...current, ...patch };
      const nextPage = normalizeCampaignPage({ ...next, updatedAt: nowISO() });
      campaignPageApi.saveDraft(nextPage);
      return nextPage;
    });
  };

  const runEditorAction = (action: EditorEngineAction) => dispatchEditor(action);

  const deleteNodeWithEngine = (nodeId: string) => {
    if (!nodeId) return;
    runEditorAction({ type: 'deleteNode', nodeId });
  };

  const updateSelectedNode = (patch: EditorNodePatch) => {
    if (selectedFreeId) runEditorAction({ type: 'updateNode', nodeId: selectedFreeId, patch });
  };

  const placeNode = (nodeId: string, parentId?: string, index?: number) => {
    runEditorAction({ type: 'placeNode', nodeId, parentId, index });
  };

  useEffect(() => {
    if (editorState.transactionPage) return;
    setPageSchema((current) => syncCampaignPageFromEditor({
      ...current,
      designMode: 'controlled',
      status: 'DRAFT',
      updatedAt: nowISO(),
    }, editorState.page));
  }, [editorState.page, editorState.transactionPage, pageEditorMode]);

  useEffect(() => {
    campaignPageApi.saveDraft(pageSchema);
    const nextPages = [pageSummary(pageSchema), ...pages.filter((page) => page.id !== pageSchema.id)];
    setPages(nextPages);
    campaignPageApi.savePageList(nextPages);
    setSaveMessage(`已自动保存 ${new Date(pageSchema.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageSchema]);

  useEffect(() => {
    campaignPageApi.saveVersions(versions);
  }, [versions]);

  useEffect(() => {
    resourceStorageApi.saveRecords(resourceRecords);
  }, [resourceRecords]);

  useEffect(() => {
    if (!selectedFreeId || pageEditorMode !== 'edit') return;
    const frame = window.requestAnimationFrame(() => {
      const node = document.querySelector<HTMLElement>(`.campaign-free-canvas [data-node-id="${selectedFreeId}"]`);
      node?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [selectedFreeId, pageEditorMode]);

  const duplicateFreeElement = () => {
    if (!selectedNode) return;
    runEditorAction({ type: 'duplicateNode', nodeId: selectedNode.id });
  };

  const deleteFreeElementById = (targetId: string) => {
    if (!targetId) return;
    deleteNodeWithEngine(targetId);
  };

  const deleteFreeElement = () => {
    deleteFreeElementById(selectedFreeId);
  };

  const moveFreeLayer = (direction: -1 | 1) => {
    if (!selectedNode) return;
    runEditorAction({ type: 'reorderNode', nodeId: selectedNode.id, position: direction > 0 ? 'forward' : 'backward' });
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest('input, textarea, select') || target?.isContentEditable) return;
      if (activeSection !== 'page') return;
      const shortcutKey = event.key.toLowerCase();
      const isUndo = (event.metaKey || event.ctrlKey) && shortcutKey === 'z' && !event.shiftKey;
      const isRedo = ((event.metaKey || event.ctrlKey) && shortcutKey === 'z' && event.shiftKey) || (event.ctrlKey && shortcutKey === 'y');
      if (isUndo || isRedo) {
        event.preventDefault();
        runEditorAction({ type: isUndo ? 'undo' : 'redo' });
        return;
      }
      if (event.key !== 'Delete' && event.key !== 'Backspace') return;
      event.preventDefault();
      if (pageEditorMode !== 'preview') {
        deleteFreeElement();
        return;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeSection, pageEditorMode, selectedFreeId, editorState.page]);

  const createBlock = (kind: CampaignBlockKind = 'RewardCard'): CampaignBlock => {
    const preset = blockPalette.find((item) => item.kind === kind) ?? blockPalette[2];
    const title = nextNodeName(blockNodeNameBase[kind] ?? preset.title);
    return {
      id: `block_${kind.toLowerCase()}_${Date.now().toString(36)}`,
      kind,
      title,
      visible: true,
      bound: kind !== 'FAQBlock',
      text: kind === 'ClaimButton' ? '立即领取' : undefined,
      action: kind === 'ClaimButton' ? 'CLAIM_REWARD' : undefined,
      variant: kind === 'ClaimButton' ? 'primary' : undefined,
      width: kind === 'ClaimButton' ? 'full' : undefined,
      radius: kind === 'ClaimButton' ? '8' : undefined,
    };
  };

  const insertionTarget = (parentId?: string, index?: number) => {
    if (parentId !== undefined || index !== undefined) return { parentId, index };
    if (!selectedNode) return { parentId: undefined, index: editorState.page.rootNodeIds.length };
    if (selectedNode.type === 'container') return { parentId: selectedNode.id, index: selectedNode.children.length };
    const selectedParent = selectedNode.parentId ? editorState.page.nodes.find((node) => node.id === selectedNode.parentId) : undefined;
    const siblings = selectedParent?.type === 'container' ? selectedParent.children : editorState.page.rootNodeIds;
    return { parentId: selectedNode.parentId, index: Math.max(0, siblings.indexOf(selectedNode.id) + 1) };
  };

  const addBlockAtCanvasPosition = (kind: CampaignBlockKind, _x = 0, _y = 0, parentId?: string, index?: number) => {
    const nextBlock = createBlock(kind);
    const z = Math.max(0, ...freeElements.map((item) => item.z)) + 1;
    const nextFreeElement = blockToFreeElement(nextBlock, deviceWidth, z);
    const target = insertionTarget(parentId, index);
    const positionedElement = { ...nextFreeElement, parentId: target.parentId, positioning: 'flow' as const, widthMode: 'fill' as const, heightMode: 'auto' as const };
    runEditorAction({ type: 'insertNode', node: editorNodeFromFreeElement(positionedElement, nextBlock), parentId: target.parentId, index: target.index });
    setLeftPageTab('structure');
  };

  const createLayoutElement = (kind: LayoutKind, x = 18, y = 360): FreeDesignElement => {
    const componentKey = componentKeyForLayoutKind(kind);
    const name = nextNodeName(kind === 'vertical-layout' ? '垂直布局' : '水平布局');
    const element = defaultElementFromComponent({
      componentKey,
      id: `layout_${kind}_${Date.now().toString(36)}`,
      width: pageSchema.device.defaultWidth,
      z: Math.max(0, ...freeElements.map((item) => item.z)) + 1,
      x: Math.min(Math.max(0, pageSchema.device.defaultWidth - 345), Math.max(0, Math.round(x))),
      y: Math.max(0, Math.round(y)),
    });
    return { ...element, name, text: name, layout: kind, ...flexPreset(kind) };
  };

  const addLayoutAtCanvasPosition = (kind: LayoutKind, x = 18, y = 360, parentId?: string, index?: number) => {
    const nextLayout = createLayoutElement(kind, x, y);
    const target = insertionTarget(parentId, index);
    runEditorAction({ type: 'insertNode', node: editorNodeFromFreeElement({ ...nextLayout, parentId: target.parentId, positioning: 'flow', widthMode: 'fill', heightMode: 'auto' }), parentId: target.parentId, index: target.index });
    setLeftPageTab('structure');
  };

  const addComponentAtCanvasPosition = (componentKey: ComponentRegistryKey, _x = 0, _y = 0, parentId?: string, index?: number) => {
    const nextId = `free_${componentKey.toLowerCase()}_${Date.now().toString(36)}`;
    const nextName = nextNodeName(componentRegistry[componentKey].paletteLabel);
    const target = insertionTarget(parentId, index);
    const nextElement = defaultElementFromComponent({
      componentKey,
      id: nextId,
      width: deviceWidth,
      z: Math.max(0, ...freeElements.map((element) => element.z)) + 1,
      x: 0,
      y: 0,
    });
    const positionedElement = { ...nextElement, name: nextName, parentId: target.parentId, positioning: 'flow' as const, widthMode: 'fill' as const, heightMode: 'auto' as const };
    runEditorAction({ type: 'insertNode', node: editorNodeFromFreeElement(positionedElement), parentId: target.parentId, index: target.index });
    setLeftPageTab('structure');
  };

  const setEditorMode = (mode: PageEditorMode) => {
    setPageEditorMode(mode);
    if (!editorState.selectedNodeId && editorState.page.rootNodeIds[0]) runEditorAction({ type: 'selectNode', nodeId: editorState.page.rootNodeIds[0] });
  };

  const toggleNodeVisible = (nodeId: string) => {
    const node = editorState.page.nodes.find((item) => item.id === nodeId);
    if (node) runEditorAction({ type: 'updateNode', nodeId, patch: { visible: !node.visible } });
  };

  const publishPage = () => {
    const registeredActionIds = resourceRecords
      .filter((record) => record.tab === '动作' && record.status === '启用')
      .map((record) => record.name);
    const issues = validateCampaignPage(previewPage, registeredActionIds);
    setValidationIssues(issues);
    if (issues.length) {
      setActiveSection('settings');
      return;
    }
    const { publishedPage, version } = campaignPageApi.publish(previewPage, versions);
    setVersions((items) => [version, ...items]);
    setPageSchema(publishedPage);
  };

  const rollbackVersion = (version: CampaignPageVersion) => {
    const nextPage = campaignPageApi.rollbackToVersion(version);
    setPageSchema({ ...nextPage, designMode: 'controlled' });
    runEditorAction({ type: 'replacePage', page: nextPage.editor });
    setPageEditorMode('edit');
    setActiveSection('page');
  };

  const createFromTemplate = (templateName: string) => {
    const nextPage = defaultCampaignPage();
    nextPage.id = `page_${Date.now().toString(36)}`;
    nextPage.name = `${templateName} 页面草稿`;
    nextPage.path = `/campaign/${templateName.toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'template-page'}`;
    nextPage.updatedAt = nowISO();
    setPageSchema(nextPage);
    runEditorAction({ type: 'replacePage', page: nextPage.editor });
    setPageEditorMode('edit');
    setActiveSection('page');
  };

  const exportSchema = () => {
    const blob = new Blob([JSON.stringify(pageSchema, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${pageSchema.id}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const isPreviewMode = pageEditorMode === 'preview';
  const buildTreeNode = (node: PageTreeNode): TreeNode => {
    const isLayoutNode = node.node.type === 'container';
    const active = selectedFreeId === node.id;
    return {
      id: `node_${node.id}`,
      label: node.title,
      meta: node.meta,
      active,
      draggable: !isPreviewMode,
      onDragStart: (event) => {
        if (isPreviewMode) {
          event.preventDefault();
          return;
        }
        event.dataTransfer.setData('application/x-campaign-node', node.id);
        event.dataTransfer.effectAllowed = 'move';
      },
      onDragOver: (event) => {
        if (isPreviewMode || !event.dataTransfer.types.includes('application/x-campaign-node')) return;
        event.preventDefault();
        event.dataTransfer.dropEffect = 'move';
      },
      onDrop: (event) => {
        if (isPreviewMode) return;
        const draggedNodeId = event.dataTransfer.getData('application/x-campaign-node');
        if (!draggedNodeId) return;
        event.preventDefault();
        event.stopPropagation();
        const rect = event.currentTarget.getBoundingClientRect();
        const ratio = (event.clientY - rect.top) / Math.max(1, rect.height);
        if (isLayoutNode && ratio >= 0.25 && ratio <= 0.75) {
          placeNode(draggedNodeId, node.id, node.node.type === 'container' ? node.node.children.length : 0);
          return;
        }
        const treeParent = node.node.parentId ? editorState.page.nodes.find((item) => item.id === node.node.parentId) : undefined;
        const siblings = treeParent?.type === 'container' ? treeParent.children : editorState.page.rootNodeIds;
        const siblingIndex = siblings.indexOf(node.id);
        placeNode(draggedNodeId, node.node.parentId, siblingIndex + (ratio > 0.5 ? 1 : 0));
      },
      onClick: () => runEditorAction({ type: 'selectNode', nodeId: node.id }),
      actions: (
        <details className="campaign-tree-actions-menu">
          <summary aria-label={`${node.title} 操作`}><MoreHorizontal size={17} /></summary>
          <div>
            <Button size="sm" variant="ghost" icon={<Eye size={15} />} aria-label={node.node.visible ? '隐藏内容块' : '显示内容块'} disabled={isPreviewMode} onClick={() => {
              if (!isPreviewMode) toggleNodeVisible(node.id);
            }}>显隐</Button>
            <Button size="sm" variant="ghost" icon={<Copy size={15} />} aria-label="复制内容块" disabled={isPreviewMode} onClick={() => {
              if (!isPreviewMode) runEditorAction({ type: 'duplicateNode', nodeId: node.id });
            }}>复制</Button>
            <Button size="sm" variant="ghost" icon={<Trash2 size={15} />} aria-label="删除内容块" disabled={isPreviewMode} onClick={() => {
              if (!isPreviewMode) deleteFreeElementById(node.id);
            }}>删除</Button>
          </div>
        </details>
      ),
      children: node.children?.map((child) => buildTreeNode(child)),
    };
  };
  const setEditorDeviceWidth = (width: DeviceWidth) => {
    setDeviceWidth(width);
  };
  const previewPage = syncCampaignPageFromEditor({ ...pageSchema, device: { ...pageSchema.device, defaultWidth: deviceWidth } }, {
    ...editorState.page,
    device: { ...editorState.page.device, defaultWidth: deviceWidth },
  });
  const selectFreeNode = (nodeId: string) => {
    runEditorAction({ type: 'selectNode', nodeId: nodeId || undefined });
  };

  const startNodeDrag = (event: ReactPointerEvent<HTMLElement>, node: EditorNode) => {
    if (node.positioning !== 'absolute' || node.locked) return;
    event.preventDefault();
    event.stopPropagation();
    selectFreeNode(node.id);
    const startX = event.clientX;
    const startY = event.clientY;
    const original = node.frame;
    const parent = node.parentId ? editorState.page.nodes.find((item) => item.id === node.parentId) : undefined;
    const boundsWidth = parent?.frame.width ?? deviceWidth;
    const parentX = parent?.frame.x ?? 0;
    runEditorAction({ type: 'beginTransaction' });
    const move = (moveEvent: PointerEvent) => {
      let x = Math.max(0, Math.min(Math.max(0, boundsWidth - original.width), original.x + moveEvent.clientX - startX));
      const y = Math.max(0, original.y + moveEvent.clientY - startY);
      const centerGuide = Math.round((boundsWidth - original.width) / 2);
      const snapCenter = Math.abs(x - centerGuide) <= 6;
      if (snapCenter) x = centerGuide;
      setAlignmentGuides({ x: snapCenter ? parentX + Math.round(boundsWidth / 2) : undefined });
      runEditorAction({ type: 'moveNode', nodeId: node.id, position: { x: Math.round(x), y: Math.round(y) }, transient: true });
    };
    const end = () => {
      setAlignmentGuides({});
      runEditorAction({ type: 'commitTransaction' });
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  const startNodeResize = (event: ReactPointerEvent<HTMLSpanElement>, node: EditorNode) => {
    if (node.locked) return;
    event.preventDefault();
    event.stopPropagation();
    const startX = event.clientX;
    const startY = event.clientY;
    const original = node.frame;
    runEditorAction({ type: 'beginTransaction' });
    const move = (moveEvent: PointerEvent) => runEditorAction({
      type: 'resizeNode',
      nodeId: node.id,
      size: { width: Math.max(0, Math.round(original.width + moveEvent.clientX - startX)), height: Math.max(0, Math.round(original.height + moveEvent.clientY - startY)) },
      transient: true,
    });
    const end = () => {
      runEditorAction({ type: 'commitTransaction' });
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  return (
    <main className="campaign-editor">
      <header className="campaign-editor-header">
        <div className="campaign-brand">
          <span className="campaign-brand-mark">X</span>
          <div><b>ExMarketing</b><small>Campaign Editor</small></div>
        </div>
        <button className="campaign-select" type="button">{pageSchema.name} <ChevronDown size={16} /></button>
        <select value={deviceWidth} onChange={(event) => setEditorDeviceWidth(Number(event.target.value) as DeviceWidth)}>
          <option value={375}>375px</option>
          <option value={393}>393px</option>
          <option value={414}>414px</option>
        </select>
        <span className="campaign-save-state">● {saveMessage}</span>
        <div className="campaign-header-spacer" />
        <button className="icon" type="button" aria-label="撤销" title="撤销" disabled={!editorState.past.length} onClick={() => runEditorAction({ type: 'undo' })}><Undo2 size={18} /></button>
        <button className="icon" type="button" aria-label="重做" title="重做" disabled={!editorState.future.length} onClick={() => runEditorAction({ type: 'redo' })}><Redo2 size={18} /></button>
        <button type="button" onClick={() => setPreviewOpen(true)}><Eye size={17} />预览</button>
        <button type="button" onClick={exportSchema}><FileText size={17} />导出 Schema</button>
        <button className="primary" type="button" onClick={publishPage}><Plane size={17} />发布 <ChevronDown size={15} /></button>
        <button className="icon" type="button" aria-label="帮助"><HelpCircle size={18} /></button>
        <button className="icon" type="button" aria-label="通知"><Bell size={18} /></button>
        <button className="campaign-user" type="button"><span>运</span>运营团队 <ChevronDown size={14} /></button>
      </header>

      <section className="campaign-editor-body">
        <aside className="campaign-left-nav" aria-label="Campaign editor navigation">
          {navItems.map(({ id, label, icon: Icon }) => (
            <button key={id} className={id === activeSection ? 'active' : ''} type="button" onClick={() => setActiveSection(id as WorkspaceSection)}>
              <Icon size={22} />
              <span>{label}</span>
            </button>
          ))}
        </aside>

        {activeSection === 'page' ? (
          <PageAuthoringWorkspace
            pageName={pageSchema.name}
            previewPage={previewPage}
            deviceWidth={deviceWidth}
            mode={pageEditorMode}
            leftTab={leftPageTab}
            treeQuery={treeQuery}
            treeOpen={treeOpen}
            treeNodes={treeNodes.map((node) => buildTreeNode(node))}
            selectedNode={selectedNode}
            selectedNodeId={selectedFreeId}
            hoveredNodeId={hoveredNodeId}
            inspectorTab={inspectorTab}
            resources={resourceRecords}
            draggingBlockKind={draggingBlockKind}
            draggingLayoutKind={draggingLayoutKind}
            draggingComponentKey={draggingComponentKey}
            alignmentGuides={alignmentGuides}
            onModeChange={setEditorMode}
            onLeftTabChange={setLeftPageTab}
            onTreeQueryChange={setTreeQuery}
            onTreeOpenChange={setTreeOpen}
            onMoveNodeToRoot={(nodeId) => placeNode(nodeId, undefined, editorState.page.rootNodeIds.length)}
            onSelectNode={selectFreeNode}
            onHoverNode={setHoveredNodeId}
            onAddBlock={addBlockAtCanvasPosition}
            onAddLayout={addLayoutAtCanvasPosition}
            onAddComponent={addComponentAtCanvasPosition}
            onPlaceNode={placeNode}
            onDraggingBlockChange={setDraggingBlockKind}
            onDraggingLayoutChange={setDraggingLayoutKind}
            onDraggingComponentChange={setDraggingComponentKey}
            onPointerDownNode={startNodeDrag}
            onResizePointerDownNode={startNodeResize}
            onDuplicateNode={duplicateFreeElement}
            onDeleteNode={deleteFreeElement}
            onInspectorTabChange={setInspectorTab}
            onNodeChange={updateSelectedNode}
            onReorderNode={moveFreeLayer}
          />
        ) : (
          <CampaignWorkspace
            section={activeSection}
            page={pageSchema}
            versions={versions}
            validationIssues={validationIssues}
            resourceRecords={resourceRecords}
            onResourceRecordsChange={setResourceRecords}
            onPagePatch={(patch) => updatePage(patch)}
            onCreateFromTemplate={createFromTemplate}
            onRollbackVersion={rollbackVersion}
          />
        )}
      </section>
      {previewOpen ? (
        <div className="campaign-preview-modal" role="dialog" aria-modal="true" aria-label="移动端预览">
          <button className="campaign-preview-backdrop" type="button" aria-label="关闭预览" onClick={() => setPreviewOpen(false)} />
          <section className="campaign-phone-frame">
            <header><span />移动端预览<button type="button" onClick={() => setPreviewOpen(false)}>关闭</button></header>
            <div className="campaign-phone-screen" style={{ width: deviceWidth }}>
              <CampaignRuntimeView page={previewPage} embedded />
            </div>
          </section>
        </div>
      ) : null}
    </main>
  );
}
