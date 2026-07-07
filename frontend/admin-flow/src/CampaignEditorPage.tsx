import { useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react';
import {
  Bell,
  ChevronDown,
  Copy,
  Database,
  Eye,
  FileText,
  Gift,
  HelpCircle,
  Image,
  ListChecks,
  MonitorSmartphone,
  Plane,
  Rows3,
  Settings,
  Sparkles,
  Trash2,
  Type,
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
  WorkspaceItem,
  WorkspaceSection,
} from './campaign-editor/types';
import {
  blockPalette,
  blocksToFreeElements,
  blockToFreeElement,
  blockIdForFreeElementId,
  currentFreeElementIdForBlock,
  fitLayouts,
  flexPreset,
  initialBlocks,
  initialFreeElements,
  layoutChildWidth,
  layoutPalette,
  legacyFreeElementIdForBlock,
  moveBlock,
  normalizeLayoutKind,
  syncFreeElementsWithBlocks,
} from './campaign-editor/layout';
import { campaignPageApi, defaultCampaignPage, normalizeCampaignPage, nowISO, pageSummary, validateCampaignPage } from './campaign-editor/campaignPageStore';
import { editorPageToFreeElements } from './campaign-editor/editorSchema';
import { createEditorEngineState, editorEngineReducer, type EditorEngineAction } from './campaign-editor/editorEngine';
import { componentKeyForLayoutKind, componentRegistry, defaultElementFromComponent } from './campaign-editor/componentRegistry';
import { resourceStorageApi } from './campaign-editor/resourceStore';
import { CampaignResourceWorkspace, defaultResourceRecords } from './campaign-editor/ResourceWorkspace';
import { CampaignRuntimeView } from './campaign-editor/RuntimeView';
import { CampaignModulePreview } from './campaign-editor/PageWorkspace';
import { FreeElementInspector, InspectorContent } from './campaign-editor/Inspector';
import { FreeDesignCanvas } from './campaign-editor/FreeCanvas';
import { Button, DataTable, Field, Panel, SegmentedControl, Tabs, Tree, type DataTableColumn, type TreeNode } from './campaign-editor/ui';
export { CampaignRuntimePage } from './campaign-editor/RuntimeView';

const navItems = [
  { id: 'page', label: '页面', icon: FileText },
  { id: 'resources', label: '资源', icon: Database },
  { id: 'settings', label: '设置', icon: Settings },
  { id: 'templates', label: '模板库', icon: Sparkles },
];

const workspaceConfig: Record<Exclude<WorkspaceSection, 'page'>, {
  groups: string[];
  items: WorkspaceItem[];
  detailTitle: string;
}> = {
  resources: {
    groups: ['数据', '动作', '样式', '图片'],
    detailTitle: '资源详情',
    items: [
      { title: 'activity.title', meta: '数据 · 文本 · 示例：夏日充值返利活动', detail: '活动标题，用于头图、标题和分享卡片。', group: '数据', tags: ['Activity View Model', 'HeroBanner', 'Title'] },
      { title: 'reward.amount', meta: '数据 · 金额 · 示例：¥188', detail: '奖励金额，格式化后展示在奖励卡片。', group: '数据', tags: ['RewardCard', '默认值：--'] },
      { title: 'eligibility.canClaim', meta: '数据 · 布尔 · 示例：true', detail: '控制领取按钮禁用、显示条件和领取状态。', group: '数据', tags: ['ClaimButton', '{{eligibility.canClaim}}'] },
      { title: 'progress.percent', meta: '数据 · 数字 · 示例：62', detail: '活动参与进度，可绑定到进度条或任务状态。', group: '数据', tags: ['ProgressBlock'] },
      { title: 'CLAIM_REWARD', meta: '动作 · 触发组件：领取按钮', detail: '点击后调用活动事件 API，触发夏日充值返利流程。', group: '动作', tags: ['rewardId -> reward.id', 'activityId -> activity.id'] },
      { title: 'OPEN_LOGIN', meta: '动作 · 触发组件：登录提示', detail: '未登录用户打开登录入口，成功后刷新页面数据。', group: '动作', tags: ['登录态', '刷新数据'] },
      { title: 'NAVIGATE', meta: '动作 · 页面跳转', detail: '跳转到活动规则、账户页或外部落地页。', group: '动作', tags: ['白名单 URL'] },
      { title: 'TRACK_EVENT', meta: '动作 · 埋点事件', detail: '记录运营点击、曝光和转化链路。', group: '动作', tags: ['claim_reward_click'] },
      { title: '主色', meta: '样式 · Primary · theme accent', detail: '影响 CTA、选中描边和链接，不改变活动素材颜色。', group: '样式', tags: ['--theme-accent'] },
      { title: '页面背景', meta: '样式 · Canvas · 移动端活动页', detail: '用于活动页面底色，跟随主题 token 管理。', group: '样式', tags: ['campaign token'] },
      { title: '主按钮样式', meta: '样式 · 全宽 · 8px 圆角', detail: '领取按钮和关键 CTA 使用统一按钮样式。', group: '样式', tags: ['高度 48px', '点击区域正常'] },
      { title: '奖励卡片', meta: '样式 · 8px 圆角 · 细边框', detail: '用于奖励、进度、规则等信息容器。', group: '样式', tags: ['活动素材色不染色'] },
      { title: 'hero-summer-rebate.png', meta: '图片 · 头图素材 · 750x360', detail: '活动首屏头图，可拖入页面或绑定到头图组件。', group: '图片', tags: ['HeroBanner', '移动端 2x'] },
      { title: 'reward-card-bg.png', meta: '图片 · 奖励卡背景 · 690x240', detail: '奖励卡可复用背景图，用于强化金额和权益展示。', group: '图片', tags: ['RewardCard', '可裁剪'] },
      { title: 'share-cover.png', meta: '图片 · 分享封面 · 1:1', detail: '用于微信、Telegram 或活动链接预览。', group: '图片', tags: ['分享卡片'] },
    ],
  },
  settings: {
    groups: ['基础信息', '发布', '分享', '权限'],
    detailTitle: '发布预览',
    items: [
      { title: '页面名称', meta: '夏日充值返利活动', detail: '活动页名称，展示在编辑器和发布版本里。', group: '基础信息', tags: ['page_20250520_001'] },
      { title: '页面路径', meta: '/campaign/summer-rebate', detail: '发布后的访问路径。', group: '基础信息', tags: ['唯一路径'] },
      { title: '上线时间', meta: '2025-05-20 00:00:00', detail: '发布计划开始时间。', group: '发布', tags: ['Asia/Shanghai'] },
      { title: '发布环境', meta: 'Production · 待发布', detail: '发布前校验 schema、绑定字段、动作权限和移动端溢出。', group: '发布', tags: ['DRAFT'] },
      { title: '分享卡片', meta: '标题 + 封面图', detail: '用于运营验收移动端分享预览。', group: '分享', tags: ['复制链接'] },
      { title: '编辑权限', meta: '运营团队', detail: '没有发布权限时不能发布。', group: '权限', tags: ['Owner / Editor'] },
    ],
  },
  templates: {
    groups: ['全部', '充值返利', '签到任务', '邀请裂变', '优惠券', '新手活动', '排行榜'],
    detailTitle: '模板详情',
    items: [
      { title: '夏日充值返利', meta: '充值返利 · 5 字段 · 1 动作', detail: '包含头图、奖励卡、倒计时、参与进度、领取按钮、规则说明。', group: '充值返利', tags: ['activity.title', 'reward.amount', 'CLAIM_REWARD'] },
      { title: '每日签到领豪礼', meta: '签到任务 · 4 字段 · 2 动作', detail: '适合连续签到和每日奖励。', group: '签到任务', tags: ['OPEN_LOGIN', 'TRACK_EVENT'] },
      { title: '邀请好友奖励', meta: '邀请裂变 · 6 字段 · 2 动作', detail: '包含邀请码、奖励进度和邀请按钮。', group: '邀请裂变', tags: ['NAVIGATE'] },
      { title: '优惠券领取', meta: '优惠券 · 4 字段 · 1 动作', detail: '适合券包领取和权益展示。', group: '优惠券', tags: ['CLAIM_REWARD'] },
      { title: '新手任务礼包', meta: '新手活动 · 7 字段 · 2 动作', detail: '包含任务列表和阶段奖励。', group: '新手活动', tags: ['TRACK_EVENT'] },
      { title: '排行榜冲刺', meta: '排行榜 · 5 字段 · 1 动作', detail: '适合活动榜单冲刺页。', group: '排行榜', tags: ['progress.rank'] },
    ],
  },
};

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

function blockDescription(kind: CampaignBlockKind) {
  if (kind === 'ClaimButton') return '按钮文案、禁用条件、点击动作';
  if (kind === 'HeroBanner') return '活动主视觉、标题、副标题、时间';
  if (kind === 'RewardCard') return '奖励金额、领取条件、奖品图';
  if (kind === 'ProgressBlock') return '参与人数、目标人数、进度比例';
  return '内容、绑定字段和显示状态';
}

export function CampaignEditorPage() {
  const [pageSchema, setPageSchema] = useState<CampaignPageSchema>(() => {
    const loadedPage = campaignPageApi.loadPage();
    return {
      ...loadedPage,
      designMode: 'free',
      freeElements: loadedPage.freeElements?.length
        ? loadedPage.freeElements
        : blocksToFreeElements(loadedPage.blocks, loadedPage.device.defaultWidth),
    };
  });
  const editorEngineHistoryRef = useRef<{ past: CampaignPageSchema['editor'][]; future: CampaignPageSchema['editor'][] }>({ past: [], future: [] });
  const [resourceRecords, setResourceRecords] = useState<ResourceRecord[]>(() => resourceStorageApi.loadRecords(defaultResourceRecords));
  const [versions, setVersions] = useState<CampaignPageVersion[]>(() => campaignPageApi.loadVersions());
  const [pages, setPages] = useState<CampaignPageSummary[]>(() => campaignPageApi.loadPageList());
  const [selectedId, setSelectedId] = useState('claim');
  const [inspectorTab, setInspectorTab] = useState<EditorTab>('content');
  const [deviceWidth, setDeviceWidth] = useState<DeviceWidth>(pageSchema.device.defaultWidth);
  const [activeSection, setActiveSection] = useState<WorkspaceSection>('page');
  const [saveMessage, setSaveMessage] = useState('已自动保存');
  const [previewOpen, setPreviewOpen] = useState(false);
  const [validationIssues, setValidationIssues] = useState<CampaignValidationIssue[]>([]);
  const [pageEditorMode, setPageEditorMode] = useState<'controlled' | 'free'>('free');
  const [selectedFreeId, setSelectedFreeId] = useState(() => pageSchema.freeElements?.[0]?.id ?? 'free_from_hero');
  const [draggingBlockKind, setDraggingBlockKind] = useState<CampaignBlockKind | null>(null);
  const [draggingLayoutKind, setDraggingLayoutKind] = useState<LayoutKind | null>(null);
  const [draggingComponentKey, setDraggingComponentKey] = useState<ComponentRegistryKey | null>(null);
  const [treeQuery, setTreeQuery] = useState('');
  const [treeOpen, setTreeOpen] = useState(true);
  const [libraryOpen, setLibraryOpen] = useState(true);
  const blocks = pageSchema.blocks;
  const freeElements = pageSchema.editor?.nodes.length ? editorPageToFreeElements(pageSchema.editor) : pageSchema.freeElements ?? initialFreeElements;
  const selectedBlock = blocks.find((block) => block.id === selectedId) ?? blocks[0];
  const selectedFreeElement = freeElements.find((element) => element.id === selectedFreeId) ?? freeElements[0];
  const visibleBlocks = useMemo(() => blocks.filter((block) => block.visible), [blocks]);
  const treeNodes = useMemo<PageTreeNode[]>(() => {
    const keyword = treeQuery.trim().toLowerCase();
    const freeByBlockId = new Map<string, FreeDesignElement>(freeElements.flatMap((element): Array<[string, FreeDesignElement]> => {
      const blockId = blockIdForFreeElementId(element.id);
      return blockId ? [[blockId, element]] : [];
    }));
    const blockNodes: PageTreeNode[] = blocks.map((block) => ({ id: block.id, type: 'block' as const, title: block.title, meta: block.kind, block }));
    const blockNodeById = new Map(blockNodes.map((node) => [node.id, node]));
    const blockNodesByFreeId = new Map<string, PageTreeNode>();
    for (const block of blocks) {
      const node = blockNodeById.get(block.id);
      if (!node) continue;
      blockNodesByFreeId.set(`free_from_${block.id}`, node);
      const legacyId = legacyFreeElementIdForBlock(block);
      if (legacyId) blockNodesByFreeId.set(legacyId, node);
    }
    const layouts = freeElements
      .filter((element) => element.kind === 'layout')
      .map((element) => ({
        id: element.id,
        type: 'layout' as const,
        title: element.text.split('\\n')[0] || '布局盒子',
        meta: normalizeLayoutKind(element.layout) === 'vertical-layout' ? 'VerticalLayout' : 'HorizontalLayout',
        element,
        children: freeElements
          .filter((child) => child.parentId === element.id)
          .map((child) => blockNodesByFreeId.get(child.id))
          .filter((node): node is PageTreeNode => Boolean(node)),
      }));
    const parentedBlockIds = new Set(freeElements.filter((element) => element.parentId).map((element) => blockIdForFreeElementId(element.id)).filter(Boolean));
    const nodes: PageTreeNode[] = [
      ...blockNodes.filter((node) => !parentedBlockIds.has(node.id) && !freeByBlockId.get(node.id)?.parentId),
      ...layouts,
    ];
    const matches = (node: PageTreeNode): boolean => `${node.title} ${node.meta}`.toLowerCase().includes(keyword);
    if (!keyword) return nodes;
    return nodes
      .map((node) => node.children?.length ? { ...node, children: node.children.filter((child) => matches(child)) } : node)
      .filter((node) => matches(node) || Boolean(node.children?.length));
  }, [blocks, freeElements, treeQuery]);
  const SelectedIcon = selectedBlock ? blockIcons[selectedBlock.kind] : Type;

  const updatePage = (patch: Partial<CampaignPageSchema> | ((current: CampaignPageSchema) => CampaignPageSchema)) => {
    setPageSchema((current) => {
      const next = typeof patch === 'function' ? patch(current) : { ...current, ...patch };
      const nextPage = normalizeCampaignPage({ ...next, updatedAt: nowISO() });
      campaignPageApi.saveDraft(nextPage);
      return nextPage;
    });
  };

  const updateBlocks = (updater: (current: CampaignBlock[]) => CampaignBlock[]) => {
    updatePage((current) => {
      const blocks = updater(current.blocks);
      return {
        ...current,
        blocks,
        freeElements: syncFreeElementsWithBlocks(blocks, current.device.defaultWidth, current.freeElements ?? initialFreeElements),
        designMode: current.designMode,
        status: 'DRAFT',
      };
    });
  };

  const updateFreeElements = (updater: (current: FreeDesignElement[]) => FreeDesignElement[]) => {
    updatePage((current) => ({ ...current, freeElements: updater(current.freeElements ?? initialFreeElements), designMode: 'free', status: 'DRAFT' }));
  };

  const runEditorAction = (action: EditorEngineAction, selectedNodeId = selectedFreeId) => {
    const previewEngineState = editorEngineReducer({ ...createEditorEngineState(pageSchema.editor, selectedNodeId), ...editorEngineHistoryRef.current }, action);
    updatePage((current) => {
      const engineState = { ...createEditorEngineState(current.editor, selectedNodeId), ...editorEngineHistoryRef.current };
      const nextEngineState = editorEngineReducer(engineState, action);
      editorEngineHistoryRef.current = { past: nextEngineState.past, future: nextEngineState.future };
      return {
        ...current,
        editor: nextEngineState.page,
        freeElements: fitLayouts(editorPageToFreeElements(nextEngineState.page)),
        designMode: 'free',
        status: 'DRAFT',
      };
    });
    if (previewEngineState.selectedNodeId) {
      setSelectedFreeId(previewEngineState.selectedNodeId);
      const blockId = blockIdForFreeElementId(previewEngineState.selectedNodeId);
      if (blockId) setSelectedId(blockId);
    }
  };

  const deleteNodeWithEngine = (nodeId: string, removeBlockId?: string) => {
    if (!nodeId) return;
    const nextFreeIds = freeElements
      .filter((element) => element.id !== nodeId && element.parentId !== nodeId)
      .map((element) => element.id);
    const nextSelectedFreeId = nextFreeIds[0] ?? '';
    updatePage((current) => {
      const engineState = { ...createEditorEngineState(current.editor, nodeId), ...editorEngineHistoryRef.current };
      const nextEngineState = editorEngineReducer(engineState, { type: 'deleteNode', nodeId });
      editorEngineHistoryRef.current = { past: nextEngineState.past, future: nextEngineState.future };
      return {
        ...current,
        blocks: removeBlockId ? current.blocks.filter((block) => block.id !== removeBlockId) : current.blocks,
        editor: nextEngineState.page,
        freeElements: fitLayouts(editorPageToFreeElements(nextEngineState.page)),
        designMode: 'free',
        status: 'DRAFT',
      };
    });
    setSelectedFreeId(nextSelectedFreeId);
    const nextBlockId = removeBlockId ? blocks.find((block) => block.id !== removeBlockId)?.id ?? '' : blockIdForFreeElementId(nextSelectedFreeId);
    setSelectedId(nextBlockId);
  };

  const updateSelectedFreeElement = (patch: Partial<FreeDesignElement>) => {
    updateFreeElements((items) => {
      const selected = items.find((element) => element.id === selectedFreeId);
      return fitLayouts(items.map((element) => {
        if (element.id === selectedFreeId) {
          const next = { ...element, ...patch };
          const parent = next.parentId ? items.find((item) => item.id === next.parentId) : undefined;
          const maxX = Math.max(0, (parent?.w ?? pageSchema.device.defaultWidth) - next.w);
          return {
            ...next,
            x: Math.min(maxX, Math.max(0, next.x)),
            y: Math.max(0, next.y),
          };
        }
        if (selected?.kind === 'layout' && element.parentId === selected.id && patch.w !== undefined) {
          return { ...element, x: 0, w: layoutChildWidth({ ...selected, w: Math.max(0, Number(patch.w)) }) };
        }
        return element;
      }));
    });
  };

  const moveFreeElementIntoLayout = (freeElementId: string, layoutId: string) => {
    updatePage((current) => {
      let workingPage = current;
      if (!workingPage.editor.nodes.some((node) => node.id === freeElementId)) {
        const blockId = blockIdForFreeElementId(freeElementId);
        const sourceBlock = blockId ? workingPage.blocks.find((block) => block.id === blockId) : undefined;
        if (sourceBlock) {
          const currentElements = editorPageToFreeElements(workingPage.editor);
          const nextElement = blockToFreeElement(sourceBlock, workingPage.device.defaultWidth, Math.max(0, ...currentElements.map((element) => element.z)) + 1);
          workingPage = normalizeCampaignPage({
            ...workingPage,
            freeElements: [...currentElements, nextElement],
            designMode: 'free',
            status: 'DRAFT',
          });
        }
      }
      const engineState = { ...createEditorEngineState(workingPage.editor, freeElementId), ...editorEngineHistoryRef.current };
      const nextEngineState = editorEngineReducer(engineState, { type: 'moveIntoContainer', nodeId: freeElementId, containerId: layoutId });
      editorEngineHistoryRef.current = { past: nextEngineState.past, future: nextEngineState.future };
      return {
        ...workingPage,
        editor: nextEngineState.page,
        freeElements: fitLayouts(editorPageToFreeElements(nextEngineState.page)),
        designMode: 'free',
        status: 'DRAFT',
      };
    });
    setSelectedFreeId(freeElementId);
  };

  const moveFreeElementToRoot = (freeElementId: string) => {
    runEditorAction({ type: 'moveOutOfContainer', nodeId: freeElementId }, freeElementId);
    setSelectedFreeId(freeElementId);
  };

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

  const duplicateBlock = () => {
    const source = selectedBlock;
    if (!source) return;
    const copyBlock = { ...source, id: `${source.id}_${Date.now().toString(36)}`, title: `${source.title} 副本` };
    const index = blocks.findIndex((block) => block.id === source.id);
    const next = [...blocks];
    next.splice(index + 1, 0, copyBlock);
    updateBlocks(() => next);
    setSelectedId(copyBlock.id);
  };

  const duplicateBlockById = (blockId: string) => {
    const source = blocks.find((block) => block.id === blockId);
    if (!source) return;
    const copyBlock = { ...source, id: `${source.id}_${Date.now().toString(36)}`, title: `${source.title} 副本` };
    const index = blocks.findIndex((block) => block.id === source.id);
    updateBlocks((items) => {
      const next = [...items];
      next.splice(index + 1, 0, copyBlock);
      return next;
    });
    setSelectedId(copyBlock.id);
  };

  const duplicateFreeElement = () => {
    if (!selectedFreeElement) return;
    runEditorAction({ type: 'duplicateNode', nodeId: selectedFreeElement.id }, selectedFreeElement.id);
  };

  const deleteFreeElementById = (targetId: string) => {
    if (!targetId) return;
    const sourceBlockId = blockIdForFreeElementId(targetId);
    if (sourceBlockId) {
      deleteNodeWithEngine(targetId, sourceBlockId);
      return;
    }
    deleteNodeWithEngine(targetId);
  };

  const deleteFreeElement = () => {
    deleteFreeElementById(selectedFreeId);
  };

  const moveFreeLayer = (direction: -1 | 1) => {
    if (!selectedFreeElement) return;
    runEditorAction({ type: 'reorderNode', nodeId: selectedFreeElement.id, position: direction > 0 ? 'forward' : 'backward' }, selectedFreeElement.id);
  };

  const deleteBlock = () => {
    if (!selectedBlock) return;
    const freeElementId = currentFreeElementIdForBlock(selectedBlock.id, freeElements) ?? `free_from_${selectedBlock.id}`;
    deleteNodeWithEngine(freeElementId, selectedBlock.id);
  };

  const deleteBlockById = (blockId: string) => {
    const freeElementId = currentFreeElementIdForBlock(blockId, freeElements) ?? `free_from_${blockId}`;
    deleteNodeWithEngine(freeElementId, blockId);
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
      if (pageEditorMode === 'free') {
        deleteFreeElement();
        return;
      }
      deleteBlock();
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeSection, pageEditorMode, selectedFreeId, freeElements, blocks, selectedBlock, pageSchema.editor]);

  const createBlock = (kind: CampaignBlockKind = 'RewardCard'): CampaignBlock => {
    const preset = blockPalette.find((item) => item.kind === kind) ?? blockPalette[2];
    return {
      id: `block_${kind.toLowerCase()}_${Date.now().toString(36)}`,
      kind,
      title: preset.title,
      visible: true,
      bound: kind !== 'FAQBlock',
      text: kind === 'ClaimButton' ? '立即领取' : undefined,
      action: kind === 'ClaimButton' ? 'CLAIM_REWARD' : undefined,
      variant: kind === 'ClaimButton' ? 'primary' : undefined,
      width: kind === 'ClaimButton' ? 'full' : undefined,
      radius: kind === 'ClaimButton' ? '8' : undefined,
    };
  };

  const addBlock = (kind: CampaignBlockKind = 'RewardCard') => {
    const nextBlock = createBlock(kind);
    updateBlocks((items) => [...items, nextBlock]);
    setSelectedId(nextBlock.id);
    if (pageEditorMode === 'free') setSelectedFreeId(`free_from_${nextBlock.id}`);
  };

  const addBlockAtCanvasPosition = (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => {
    const nextBlock = createBlock(kind);
    updatePage((current) => {
      const nextBlocks = [...current.blocks, nextBlock];
      const z = Math.max(0, ...(current.freeElements ?? []).map((item) => item.z)) + 1;
      const nextFreeElement = blockToFreeElement(nextBlock, current.device.defaultWidth, z);
      const parent = parentId ? (current.freeElements ?? []).find((element) => element.id === parentId && element.kind === 'layout') : undefined;
      const positionedElement = parent ? {
        ...nextFreeElement,
        parentId: parent.id,
        x: 0,
        y: 0,
        w: layoutChildWidth(parent),
      } : {
        ...nextFreeElement,
        x: Math.min(Math.max(0, current.device.defaultWidth - nextFreeElement.w), Math.max(0, Math.round(x))),
        y: Math.max(0, Math.round(y)),
      };
      const nextFreeElements = fitLayouts([...(current.freeElements ?? []), positionedElement]);
      return { ...current, blocks: nextBlocks, freeElements: nextFreeElements, designMode: 'free', status: 'DRAFT' };
    });
    setSelectedId(nextBlock.id);
    setSelectedFreeId(`free_from_${nextBlock.id}`);
  };

  const createLayoutElement = (kind: LayoutKind, x = 18, y = 360): FreeDesignElement => {
    const componentKey = componentKeyForLayoutKind(kind);
    const element = defaultElementFromComponent({
      componentKey,
      id: `layout_${kind}_${Date.now().toString(36)}`,
      width: pageSchema.device.defaultWidth,
      z: Math.max(0, ...freeElements.map((item) => item.z)) + 1,
      x: Math.min(Math.max(0, pageSchema.device.defaultWidth - 345), Math.max(0, Math.round(x))),
      y: Math.max(0, Math.round(y)),
    });
    return { ...element, layout: kind, ...flexPreset(kind) };
  };

  const addLayoutAtCanvasPosition = (kind: LayoutKind, x = 18, y = 360) => {
    const nextLayout = createLayoutElement(kind, x, y);
    updateFreeElements((items) => [...items, nextLayout]);
    setSelectedFreeId(nextLayout.id);
  };

  const addComponentAtCanvasPosition = (componentKey: ComponentRegistryKey, x = 32, y = 360, parentId?: string) => {
    const nextId = `free_${componentKey.toLowerCase()}_${Date.now().toString(36)}`;
    updatePage((current) => {
      const parent = parentId ? (current.freeElements ?? []).find((element) => element.id === parentId && element.kind === 'layout') : undefined;
      const nextElement = defaultElementFromComponent({
        componentKey,
        id: nextId,
        width: current.device.defaultWidth,
        z: Math.max(0, ...(current.freeElements ?? []).map((element) => element.z)) + 1,
        x: parent ? 0 : 0,
        y: parent ? 0 : Math.max(0, Math.round(y)),
      });
      const positionedElement = parent ? {
        ...nextElement,
        parentId: parent.id,
        x: 0,
        y: 0,
        w: layoutChildWidth(parent),
      } : {
        ...nextElement,
        x: Math.min(Math.max(0, current.device.defaultWidth - nextElement.w), Math.max(0, Math.round(x))),
      };
      return {
        ...current,
        freeElements: fitLayouts([...(current.freeElements ?? []), positionedElement]),
        designMode: 'free',
        status: 'DRAFT',
      };
    });
    setSelectedFreeId(nextId);
  };

  const addFreeElement = (kind: FreeDesignElement['kind']) => {
    const componentKey = kind === 'button' ? 'Button' : kind === 'image' ? 'Image' : kind === 'card' ? 'RewardCard' : 'Title';
    const next = defaultElementFromComponent({
      componentKey,
      id: `free_${kind}_${Date.now().toString(36)}`,
      width: pageSchema.device.defaultWidth,
      z: Math.max(0, ...freeElements.map((element) => element.z)) + 1,
      x: 32,
      y: 360,
    });
    updateFreeElements((items) => [...items, next]);
    setSelectedFreeId(next.id);
  };

  const updateSelectedBlock = (patch: Partial<CampaignBlock>) => {
    if (!selectedBlock) return;
    updateBlocks((items) => items.map((block) => block.id === selectedBlock.id ? { ...block, ...patch } : block));
  };

  const setEditorMode = (mode: 'controlled' | 'free') => {
    setPageEditorMode(mode);
    if (mode === 'controlled') {
      const previewBlockIds = freeElements
        .map((element) => blockIdForFreeElementId(element.id))
        .filter((id) => id && blocks.some((block) => block.id === id));
      const selectedFreeBlockId = blockIdForFreeElementId(selectedFreeId);
      const nextSelectedId =
        (selectedFreeBlockId && previewBlockIds.includes(selectedFreeBlockId) ? selectedFreeBlockId : '') ||
        (previewBlockIds.includes(selectedId) ? selectedId : '') ||
        previewBlockIds[0] ||
        blocks[0]?.id ||
        '';
      setSelectedId(nextSelectedId);
      updatePage((current) => ({ ...current, designMode: 'controlled', status: 'DRAFT' }));
    }
    if (mode === 'free') {
      const nextFreeElements = pageSchema.freeElements?.length
        ? pageSchema.freeElements
        : blocksToFreeElements(pageSchema.blocks, pageSchema.device.defaultWidth);
      setSelectedFreeId((current) => nextFreeElements.some((element) => element.id === current) ? current : nextFreeElements[0]?.id ?? '');
      updatePage({ designMode: 'free', freeElements: nextFreeElements, status: 'DRAFT' });
    }
  };

  const toggleBlockVisible = (blockId: string) => {
    updateBlocks((items) => items.map((block) => block.id === blockId ? { ...block, visible: !block.visible } : block));
  };

  const publishPage = () => {
    const issues = validateCampaignPage(pageSchema);
    setValidationIssues(issues);
    if (issues.length) {
      setActiveSection('settings');
      return;
    }
    const { publishedPage, version } = campaignPageApi.publish(pageSchema, versions);
    setVersions((items) => [version, ...items]);
    setPageSchema(publishedPage);
  };

  const rollbackVersion = (version: CampaignPageVersion) => {
    const nextPage = campaignPageApi.rollbackToVersion(version);
    setPageSchema({ ...nextPage, designMode: 'free' });
    setSelectedId(nextPage.blocks[0]?.id ?? '');
    setSelectedFreeId(nextPage.freeElements[0]?.id ?? '');
    setPageEditorMode('free');
    setActiveSection('page');
  };

  const createFromTemplate = (templateName: string) => {
    const nextPage = defaultCampaignPage();
    nextPage.id = `page_${Date.now().toString(36)}`;
    nextPage.name = `${templateName} 页面草稿`;
    nextPage.path = `/campaign/${templateName.toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'template-page'}`;
    nextPage.updatedAt = nowISO();
    setPageSchema(nextPage);
    setSelectedId(nextPage.blocks[0]?.id ?? '');
    setSelectedFreeId(nextPage.freeElements[0]?.id ?? '');
    setPageEditorMode('free');
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

  const buildTreeNode = (node: PageTreeNode): TreeNode => {
    const isLayoutNode = node.type === 'layout';
    const block = node.type === 'block' ? node.block : undefined;
    const freeElementId = isLayoutNode ? node.id : currentFreeElementIdForBlock(node.id, freeElements) ?? `free_from_${node.id}`;
    const active = pageEditorMode === 'free' ? selectedFreeId === freeElementId : node.id === selectedId;
    return {
      id: `${node.type}_${node.id}`,
      label: node.title,
      meta: node.meta,
      active,
      draggable: Boolean(block),
      onDragStart: (event) => {
            if (!block) return;
            event.dataTransfer.setData('application/x-campaign-tree-free-id', freeElementId);
            event.dataTransfer.effectAllowed = 'move';
      },
      onDragOver: (event) => {
            if (!isLayoutNode) return;
            if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
            event.preventDefault();
            event.dataTransfer.dropEffect = 'move';
      },
      onDrop: (event) => {
            if (!isLayoutNode) return;
            const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
            if (!draggedFreeId) return;
            event.preventDefault();
            event.stopPropagation();
            moveFreeElementIntoLayout(draggedFreeId, node.id);
      },
      onClick: () => {
        if (block) setSelectedId(block.id);
        if (pageEditorMode === 'free') setSelectedFreeId(freeElementId);
      },
      actions: (
        <>
          <Button size="sm" variant="ghost" icon={<Eye size={16} />} iconOnly aria-label={block?.visible ? '隐藏内容块' : '显示内容块'} disabled={!block} onClick={() => {
            if (block) toggleBlockVisible(block.id);
          }}>显隐</Button>
          <Button size="sm" variant="ghost" icon={<Copy size={16} />} iconOnly aria-label="复制内容块" disabled={!block} onClick={() => {
            if (block) duplicateBlockById(block.id);
          }}>复制内容块</Button>
          <Button size="sm" variant="ghost" icon={<Trash2 size={16} />} iconOnly aria-label="删除内容块" onClick={() => {
            if (block) deleteBlockById(block.id);
            if (isLayoutNode) deleteFreeElementById(node.id);
          }}>删除内容块</Button>
        </>
      ),
      children: node.children?.map((child) => buildTreeNode(child)),
    };
  };
  const setEditorDeviceWidth = (width: DeviceWidth) => {
    setDeviceWidth(width);
    updatePage((current) => ({ ...current, device: { ...current.device, defaultWidth: width }, status: 'DRAFT' }));
  };
  const previewPage = normalizeCampaignPage({ ...pageSchema, device: { ...pageSchema.device, defaultWidth: deviceWidth } });

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
          <>
              <aside className="campaign-outline">
                <h2>页面树</h2>
                <label className="campaign-tree-search">
                  <input value={treeQuery} onChange={(event) => setTreeQuery(event.target.value)} placeholder="搜索" aria-label="搜索页面树" />
                </label>
                <button
                  className="campaign-page-tree-root"
                  type="button"
                  aria-expanded={treeOpen}
                  onDragOver={(event) => {
                    if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
                    event.preventDefault();
                    event.dataTransfer.dropEffect = 'move';
                  }}
                  onDrop={(event) => {
                    const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
                    if (!draggedFreeId) return;
                    event.preventDefault();
                    event.stopPropagation();
                    moveFreeElementToRoot(draggedFreeId);
                  }}
                  onClick={() => setTreeOpen((open) => !open)}
                >
                  <ChevronDown size={15} className={treeOpen ? 'open' : ''} />
                  <FileText size={18} />
                  <span><strong>{pageSchema.name}</strong></span>
                </button>
                {treeOpen ? (
                  <div
                    className="campaign-outline-list"
                    onDragOver={(event) => {
                      if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
                      event.preventDefault();
                      event.dataTransfer.dropEffect = 'move';
                    }}
                    onDrop={(event) => {
                      const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
                      if (!draggedFreeId) return;
                      event.preventDefault();
                      moveFreeElementToRoot(draggedFreeId);
                    }}
                  >
                    <Tree
                      nodes={treeNodes.map((node) => buildTreeNode(node))}
                      className="campaign-outline-tree"
                      rowClassName="campaign-outline-row"
                      childrenClassName="campaign-outline-children"
                      noWrapper
                    />
                    {treeNodes.length === 0 ? <p className="campaign-tree-empty">没有匹配节点</p> : null}
                  </div>
                ) : null}
                <button className="campaign-library-title" type="button" aria-expanded={libraryOpen} onClick={() => setLibraryOpen((open) => !open)}>
                  <ChevronDown size={14} className={libraryOpen ? 'open' : ''} />
                  常用组件
                </button>
                {libraryOpen ? (
                  <div className="campaign-block-palette" aria-label="组件库">
                    {blockPalette.map((item) => {
                      const Icon = blockIcons[item.kind];
                      return (
                        <button
                          key={item.kind}
                          type="button"
                          draggable
                          onClick={() => pageEditorMode === 'free' ? addBlockAtCanvasPosition(item.kind, 32, 360) : addBlock(item.kind)}
                          onDragStart={(event) => {
                            setDraggingBlockKind(item.kind);
                            event.dataTransfer.setData('application/x-campaign-block', item.kind);
                            event.dataTransfer.effectAllowed = 'copy';
                          }}
                          onDragEnd={() => setDraggingBlockKind(null)}
                        >
                          <Icon size={16} />
                          {item.label}
                        </button>
                      );
                    })}
                    <button
                      type="button"
                      draggable
                      onClick={() => addComponentAtCanvasPosition('Image', 32, 360)}
                      onDragStart={(event) => {
                        setDraggingComponentKey('Image');
                        event.dataTransfer.setData('application/x-campaign-component', 'Image');
                        event.dataTransfer.effectAllowed = 'copy';
                      }}
                      onDragEnd={() => setDraggingComponentKey(null)}
                    >
                      <Image size={16} />
                      {componentRegistry.Image.paletteLabel}
                    </button>
                    {layoutPalette.map((item) => {
                      const Icon = Rows3;
                      return (
                        <button
                          key={item.kind}
                          type="button"
                          draggable
                          onClick={() => addLayoutAtCanvasPosition(item.kind, 24, 380)}
                          onDragStart={(event) => {
                            setDraggingLayoutKind(item.kind);
                            event.dataTransfer.setData('application/x-campaign-layout', item.kind);
                            event.dataTransfer.effectAllowed = 'copy';
                          }}
                          onDragEnd={() => setDraggingLayoutKind(null)}
                        >
                          <Icon size={16} />
                          {item.label}
                        </button>
                      );
                    })}
                  </div>
                ) : null}
              </aside>

            <section className="campaign-canvas-workspace">
              <div className="campaign-toolbar">
                <Tabs
                  value={pageEditorMode === 'free' ? '页面设计' : '模块预览'}
                  items={['页面设计', '模块预览']}
                  onChange={(mode) => setEditorMode(mode === '页面设计' ? 'free' : 'controlled')}
                  ariaLabel="页面编辑模式"
                />
              </div>
              {pageEditorMode === 'free' ? (
                <FreeDesignCanvas
                  width={deviceWidth}
                  elements={freeElements}
                  selectedId={selectedFreeId}
                  onSelect={setSelectedFreeId}
                  onChange={updateFreeElements}
                  draggingBlockKind={draggingBlockKind}
                  draggingLayoutKind={draggingLayoutKind}
                  draggingComponentKey={draggingComponentKey}
                  onDropBlock={(kind, x, y, parentId) => addBlockAtCanvasPosition(kind, x, y, parentId)}
                  onDropLayout={(kind, x, y) => addLayoutAtCanvasPosition(kind, x, y)}
                  onDropComponent={(componentKey, x, y, parentId) => addComponentAtCanvasPosition(componentKey, x, y, parentId)}
                  onDragDone={() => setDraggingBlockKind(null)}
                  onLayoutDragDone={() => {
                    setDraggingLayoutKind(null);
                    setDraggingComponentKey(null);
                  }}
                  onMoveElement={(id, x, y) => runEditorAction({ type: 'moveNode', nodeId: id, position: { x, y } }, id)}
                  onResizeElement={(id, w, h) => runEditorAction({ type: 'resizeNode', nodeId: id, size: { width: w, height: h } }, id)}
                  onMoveIntoLayout={(id, layoutId) => moveFreeElementIntoLayout(id, layoutId)}
                />
              ) : (
                <CampaignModulePreview
                  page={pageSchema}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                  draggingBlockKind={draggingBlockKind}
                  onDropBlock={(kind) => addBlock(kind)}
                  onDragDone={() => setDraggingBlockKind(null)}
                />
              )}
            </section>

            <aside className="campaign-inspector">
              {pageEditorMode === 'free' ? (
                <>
                  <header>
                    <div><Type size={20} /><h2>{selectedFreeElement?.text || '自由元素'}</h2></div>
                    <span>{selectedFreeElement?.binding ? '已绑定' : '未绑定'}</span>
                    <button type="button" aria-label="复制自由元素" onClick={duplicateFreeElement}><Copy size={16} /></button>
                    <button type="button" aria-label="删除自由元素" onClick={deleteFreeElement}><Trash2 size={16} /></button>
                  </header>
                  <nav>
                    <SegmentedControl
                      value={inspectorTab}
                      items={[
                        { value: 'content', label: '内容' },
                        { value: 'binding', label: '绑定' },
                        { value: 'action', label: '动作' },
                      ]}
                      onChange={setInspectorTab}
                      ariaLabel="Inspector tabs"
                    />
                  </nav>
                  <FreeElementInspector element={selectedFreeElement} tab={inspectorTab} width={deviceWidth} onChange={updateSelectedFreeElement} onLayerMove={moveFreeLayer} />
                </>
              ) : selectedBlock ? (
                <>
                  <header>
                    <div><SelectedIcon size={20} /><h2>{selectedBlock.title}</h2></div>
                    <span>{selectedBlock.bound ? '已绑定' : '未绑定'}</span>
                    <Copy size={16} />
                    <Trash2 size={16} />
                  </header>
                  <nav>
                    <SegmentedControl
                      value={inspectorTab}
                      items={[
                        { value: 'content', label: '内容' },
                        { value: 'binding', label: '绑定' },
                        { value: 'action', label: '动作' },
                      ]}
                      onChange={setInspectorTab}
                      ariaLabel="Inspector tabs"
                    />
                  </nav>
                  <InspectorContent block={selectedBlock} tab={inspectorTab} deviceWidth={deviceWidth} onBlockChange={updateSelectedBlock} onMove={(direction) => updateBlocks((items) => moveBlock(items, selectedBlock.id, direction))} />
                </>
              ) : (
                <div className="campaign-inspector-body"><p>当前页面没有内容块，可以从左侧常用组件继续添加。</p></div>
              )}
            </aside>
          </>
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

function sectionTitle(section: Exclude<WorkspaceSection, 'page'>) {
  if (section === 'resources') return ['资源', '集中管理数据、动作、样式和图片素材'];
  if (section === 'settings') return ['设置', '配置发布路径、环境、分享预览和版本信息'];
  if (section === 'templates') return ['模板库', '从标准活动模板快速创建页面草稿'];
  return ['资源', '集中管理数据、动作、样式和图片素材'];
}

function CampaignWorkspace({
  section,
  page,
  versions,
  validationIssues,
  resourceRecords,
  onResourceRecordsChange,
  onPagePatch,
  onCreateFromTemplate,
  onRollbackVersion,
}: {
  section: Exclude<WorkspaceSection, 'page'>;
  page: CampaignPageSchema;
  versions: CampaignPageVersion[];
  validationIssues: CampaignValidationIssue[];
  resourceRecords: ResourceRecord[];
  onResourceRecordsChange: Dispatch<SetStateAction<ResourceRecord[]>>;
  onPagePatch: (patch: Partial<CampaignPageSchema>) => void;
  onCreateFromTemplate: (templateName: string) => void;
  onRollbackVersion: (version: CampaignPageVersion) => void;
}) {
  const config = workspaceConfig[section];
  const [activeGroup, setActiveGroup] = useState(config.groups[0]);
  const [selected, setSelected] = useState(0);
  const [title, description] = sectionTitle(section);
  const cards = config.items.filter((item) => activeGroup === '全部' || item.group === activeGroup);
  const current = cards[selected] ?? cards[0] ?? config.items[0];
  const groupNodes: TreeNode[] = config.groups.map((group) => ({
    id: `workspace_group_${section}_${group}`,
    label: group,
    count: config.items.filter((item) => group === '全部' || item.group === group).length,
    active: activeGroup === group,
    onClick: () => {
      setActiveGroup(group);
      setSelected(0);
    },
  }));
  const itemColumns: Array<DataTableColumn<WorkspaceItem>> = [
    {
      key: 'item',
      header: activeGroup,
      render: (item) => (
        <span className="campaign-secondary-table-item">
          <strong>{item.title}</strong>
          <small>{item.meta}</small>
          <em>{item.detail}</em>
        </span>
      ),
    },
  ];
  if (section === 'resources') return <CampaignResourceWorkspace records={resourceRecords} onRecordsChange={onResourceRecordsChange} />;
  return (
    <section className="campaign-secondary-workspace">
      <aside className="campaign-secondary-groups">
        <h2>{title}</h2>
        <p>{description}</p>
        <Tree nodes={groupNodes} className="campaign-secondary-group-tree" />
      </aside>
      <div className="campaign-secondary-list">
        <header>
          <h2>{activeGroup}</h2>
          <p>{config.detailTitle === '模板详情' ? '选择模板后可从模板创建页面草稿。' : description}</p>
        </header>
        <DataTable
          className="campaign-secondary-table"
          columns={itemColumns}
          rows={cards}
          rowKey={(item) => item.title}
          selectedKey={current.title}
          onRowClick={(item) => setSelected(cards.findIndex((card) => card.title === item.title))}
        />
      </div>
      <Panel title={config.detailTitle} className="campaign-secondary-detail" bodyClassName="campaign-secondary-detail-body">
        <h2>{current.title}</h2>
        <p>{current.detail}</p>
        <div className="campaign-secondary-tags">
          {current.tags.map((tag) => <b key={tag}>{tag}</b>)}
        </div>
        <dl>
          <div><dt>状态</dt><dd>{current.meta}</dd></div>
          <div><dt>{section === 'templates' ? '创建流程' : '安全边界'}</dt><dd>{section === 'templates' ? '从模板创建 -> 检查缺失字段和动作 -> 进入页面设计。' : '发布前校验 schema、路径、绑定字段和动作权限。'}</dd></div>
          <div><dt>发布校验</dt><dd>{versions.length ? `最近发布 ${versions[0].version} · ${new Date(versions[0].publishedAt).toLocaleString()}` : '缺失绑定、动作权限和移动端溢出会阻止发布。'}</dd></div>
        </dl>
        {section === 'settings' ? (
          <div className={validationIssues.length ? 'campaign-validation-list blocking' : 'campaign-validation-list'}>
            <h3>发布校验</h3>
            {validationIssues.length ? validationIssues.map((issue) => (
              <p key={issue.code}><strong>{issue.code}</strong>{issue.message}</p>
            )) : <p><strong>PASS</strong>schema、绑定字段、动作白名单和路径格式通过。</p>}
          </div>
        ) : null}
        {section === 'settings' ? (
          <div className="campaign-secondary-form">
            <Field label="页面名称"><input value={page.name} onChange={(event) => onPagePatch({ name: event.target.value })} /></Field>
            <Field label="页面路径"><input value={page.path} onChange={(event) => onPagePatch({ path: event.target.value })} /></Field>
            <Field label="活动 ID"><input value={page.activityId} onChange={(event) => onPagePatch({ activityId: event.target.value })} /></Field>
          </div>
        ) : null}
        {section === 'settings' && versions.length ? (
          <div className="campaign-version-list">
            <h3>历史版本</h3>
            {versions.map((version) => (
              <button key={version.id} type="button" onClick={() => onRollbackVersion(version)}>
                <strong>{version.version}</strong>
                <span>{new Date(version.publishedAt).toLocaleString()}</span>
                <small>回滚为草稿</small>
              </button>
            ))}
          </div>
        ) : null}
        {section === 'templates' ? (
          <Button className="campaign-secondary-primary" variant="primary" onClick={() => onCreateFromTemplate(current.title)}>从模板创建</Button>
        ) : null}
      </Panel>
    </section>
  );
}
