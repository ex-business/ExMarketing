import { type ReactNode, useEffect, useMemo, useState } from 'react';
import {
  Archive,
  CheckSquare,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Copy,
  Folder,
  Grid3X3,
  List,
  MoreHorizontal,
  Move,
  Pencil,
  Rows3,
  Search,
  Share2,
  SlidersHorizontal,
  Star,
  StarOff,
  UserRoundCog,
  Workflow,
} from 'lucide-react';
import {
  TaButton,
  TaCheckbox,
  TaEmptyState,
  TaIconButton,
  TaInput,
  TaMenu,
  TaMenuItem,
  TaPagination,
  TaSelect,
  TaTab,
  TaTable,
  TaTableRow,
  TaTabs,
} from './TailAdminPrimitives';

export type WorkspaceContentFolder = {
  id: string;
  name: string;
  visibility: string;
  color?: string;
  sortOrder: number;
};

export type WorkspaceContentWorkflow = {
  id: string;
  activityCode: string;
  activityName: string;
  tenantId: string;
  status?: string;
  visibility?: string;
  version?: number;
  createdAt?: string;
  updatedAt?: string;
};

type ViewMode = 'list' | 'grid' | 'compact';
type TabMode = 'all' | 'workflow' | 'folder' | 'template';
type ItemType = 'Folder' | 'Workflow';
export type WorkspaceSortMode = 'recent' | 'name' | 'code' | 'version' | 'owner';
export type WorkspaceItemAction = 'rename' | 'move' | 'archive' | 'copyLink' | 'permission' | 'share';
export type WorkspaceContentCapabilities = {
  role: string;
  canEditWorkspace: boolean;
  canManageFolders: boolean;
  canManageMembers: boolean;
  reason: string;
};
type TemplateItem = {
  id: string;
  name: string;
  category?: string;
  version?: number;
  updatedAt?: string;
};

type WorkspaceContentProps = {
  workspaceName: string;
  folderName: string;
  folderVisibility: string;
  folderUpdatedLabel: string;
  homeView: 'workspace' | 'recent' | 'favorites' | 'archived';
  folders: WorkspaceContentFolder[];
  workflows: WorkspaceContentWorkflow[];
  overviewWorkflows: WorkspaceContentWorkflow[];
  templates: TemplateItem[];
  workflowCountByFolder: Record<string, number>;
  searchValue: string;
  statusFilter: 'ACTIVE' | 'DRAFT' | 'ARCHIVED' | 'ALL';
  sortMode: WorkspaceSortMode;
  favoriteWorkflowIds: Set<string>;
  archivedWorkflowIds: Set<string>;
  isLoading?: boolean;
  errorMessage?: string;
  capabilities: WorkspaceContentCapabilities;
  onRetryLoad?: () => void;
  onSearchChange: (value: string) => void;
  onStatusFilterChange: (value: 'ACTIVE' | 'DRAFT' | 'ARCHIVED' | 'ALL') => void;
  onSortChange: (value: WorkspaceSortMode) => void;
  onCreateFolder: () => void;
  onCreateWorkflow: () => void;
  onImportWorkflow: () => void;
  onCurrentAction: (action: WorkspaceItemAction) => void;
  onItemSelect: (item: { id: string; type: ItemType; name: string; visibility: string; status?: string; version?: string; owner?: string }) => void;
  onFolderOpen: (folderId: string) => void;
  onWorkflowOpen: (workflowId: string) => void;
  onTemplateOpen: (templateId: string) => void;
  onToggleFavorite: (workflowId: string) => void;
  onToggleArchived: (workflowId: string) => void;
  onNavigateHomeView: (view: 'workspace' | 'recent' | 'favorites' | 'archived') => void;
  onItemAction: (action: WorkspaceItemAction, item: { id: string; type: ItemType; name: string; visibility: string; status?: string; version?: string; owner?: string }) => void;
  onBulkAction: (action: 'move' | 'permission' | 'copyLink', items: Array<{ id: string; type: ItemType; name: string; visibility: string; status?: string; version?: string; owner?: string }>) => void;
  onBulkArchive: (items: Array<{ id: string; type: ItemType; name: string; visibility: string; status?: string; version?: string; owner?: string }>) => void;
};

type ContentItem = {
  id: string;
  type: ItemType;
  name: string;
  code: string;
  status: string;
  version: string;
  updated: string;
  updatedAt: number;
  owner: string;
  visibility: string;
  favorite?: boolean;
  archived?: boolean;
  folderCount?: number;
};

type EmptyStateAction = {
  label: string;
  tone?: 'primary' | 'secondary';
};

type EmptyState = {
  kind: string;
  title: string;
  description: string;
  actions: EmptyStateAction[];
};

const OWNER_NAMES = ['Alice', 'Bob', 'Carol', 'David', 'Eve', 'Frank', 'Grace', 'Henry'];

const MENU_ACTIONS: Array<{ action: WorkspaceItemAction; label: string; icon: typeof Pencil; danger?: boolean }> = [
  { action: 'rename', label: '重命名', icon: Pencil },
  { action: 'move', label: '移动', icon: Move },
  { action: 'copyLink', label: '复制链接', icon: Copy },
  { action: 'permission', label: '权限', icon: UserRoundCog },
  { action: 'share', label: '分享', icon: Share2 },
  { action: 'archive', label: '归档', icon: Archive, danger: true },
];

function formatDateLabel(value?: string): string {
  if (!value) return '-';
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return '-';
  const diff = Date.now() - timestamp;
  const hour = 60 * 60 * 1000;
  const day = 24 * hour;
  if (diff < hour) return '刚刚';
  if (diff < day) return `${Math.max(1, Math.floor(diff / hour))} 小时前`;
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`;
  return new Date(value).toLocaleDateString();
}

function ownerFor(seed: string): string {
  const total = Array.from(seed).reduce((sum, char) => sum + char.charCodeAt(0), 0);
  return OWNER_NAMES[total % OWNER_NAMES.length];
}

function visibilityClass(value: string): string {
  const normalized = value.toLowerCase();
  if (normalized.includes('public')) return 'public';
  if (normalized.includes('team') || normalized.includes('workspace')) return 'workspace';
  return 'private';
}

function typeLabel(value: ItemType): string {
  return value === 'Folder' ? '文件夹' : '流程';
}

function statusLabel(value: string): string {
  const normalized = value.toUpperCase();
  if (normalized === 'ACTIVE' || normalized === 'PUBLISHED') return '已启用';
  if (normalized === 'DRAFT') return '草稿';
  if (normalized === 'ARCHIVED') return '已归档';
  return value;
}

function visibilityLabel(value: string): string {
  const normalized = value.toLowerCase();
  if (normalized.includes('public')) return '公开';
  if (normalized.includes('team') || normalized.includes('workspace')) return '工作区';
  return '私有';
}

function renderKeyword(value: string, keyword: string): ReactNode {
  const needle = keyword.trim();
  if (!needle) return value;
  const index = value.toLowerCase().indexOf(needle.toLowerCase());
  if (index < 0) return value;
  return (
    <>
      {value.slice(0, index)}
      <mark>{value.slice(index, index + needle.length)}</mark>
      {value.slice(index + needle.length)}
    </>
  );
}

export function WorkspaceContent({
  workspaceName,
  folderName,
  folderVisibility,
  folderUpdatedLabel,
  homeView,
  folders,
  workflows,
  overviewWorkflows,
  templates,
  workflowCountByFolder,
  searchValue,
  statusFilter,
  sortMode,
  favoriteWorkflowIds,
  archivedWorkflowIds,
  isLoading = false,
  errorMessage = '',
  capabilities,
  onRetryLoad,
  onSearchChange,
  onStatusFilterChange,
  onSortChange,
  onCreateFolder,
  onCreateWorkflow,
  onImportWorkflow,
  onCurrentAction,
  onItemSelect,
  onFolderOpen,
  onWorkflowOpen,
  onTemplateOpen,
  onToggleFavorite,
  onToggleArchived,
  onNavigateHomeView,
  onItemAction,
  onBulkAction,
  onBulkArchive,
}: WorkspaceContentProps) {
  const [tab, setTab] = useState<TabMode>('all');
  const [viewMode, setViewMode] = useState<ViewMode>('list');
  const [selectedItemId, setSelectedItemId] = useState('');
  const [checkedItemIds, setCheckedItemIds] = useState<Set<string>>(() => new Set());
  const [openMenuItemId, setOpenMenuItemId] = useState('');
  const [showCurrentMenu, setShowCurrentMenu] = useState(false);
  const [showAdvancedFilters, setShowAdvancedFilters] = useState(false);
  const [typeFilter, setTypeFilter] = useState<'ALL' | ItemType>('ALL');
  const [visibilityFilter, setVisibilityFilter] = useState<'ALL' | 'Private' | 'Workspace' | 'Public'>('ALL');
  const [ownerFilter, setOwnerFilter] = useState('ALL');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  useEffect(() => {
    if (!openMenuItemId && !showCurrentMenu) return undefined;
    const closeMenus = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest('.exm-menu-scope')) return;
      setOpenMenuItemId('');
      setShowCurrentMenu(false);
    };
    const closeWithKeyboard = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      setOpenMenuItemId('');
      setShowCurrentMenu(false);
    };
    document.addEventListener('pointerdown', closeMenus);
    document.addEventListener('keydown', closeWithKeyboard);
    return () => {
      document.removeEventListener('pointerdown', closeMenus);
      document.removeEventListener('keydown', closeWithKeyboard);
    };
  }, [openMenuItemId, showCurrentMenu]);

  const items = useMemo<ContentItem[]>(() => {
    const folderItems: ContentItem[] = folders.map((folder) => ({
      id: folder.id,
      type: 'Folder' as ItemType,
      name: folder.name,
      code: '-',
      status: '-',
      version: '-',
      updated: '-',
      updatedAt: 0,
      owner: ownerFor(folder.id),
      visibility: folder.visibility || folderVisibility,
      folderCount: workflowCountByFolder[folder.id] ?? 0,
    }));
    const workflowItems: ContentItem[] = workflows.map((workflow) => {
      const archived = archivedWorkflowIds.has(workflow.id) || workflow.status === 'ARCHIVED';
      const status = archived ? 'ARCHIVED' : workflow.status || 'ACTIVE';
      const updatedValue = workflow.updatedAt || workflow.createdAt;
      return {
        id: workflow.id,
        type: 'Workflow' as ItemType,
        name: workflow.activityName || workflow.activityCode,
        code: workflow.activityCode,
        status,
        version: `v${workflow.version ?? 1}`,
        updated: formatDateLabel(updatedValue),
        updatedAt: updatedValue ? new Date(updatedValue).getTime() || 0 : 0,
        owner: ownerFor(workflow.id),
        visibility: workflow.visibility === 'PUBLIC' ? 'Public' : workflow.visibility === 'PRIVATE' ? 'Private' : 'Workspace',
        favorite: favoriteWorkflowIds.has(workflow.id),
        archived,
      };
    });
    return folderItems.concat(workflowItems);
  }, [archivedWorkflowIds, favoriteWorkflowIds, folderVisibility, folders, workflowCountByFolder, workflows]);
  const overviewWorkflowItems = useMemo<ContentItem[]>(() => (
    overviewWorkflows.map((workflow) => {
      const archived = archivedWorkflowIds.has(workflow.id) || workflow.status === 'ARCHIVED';
      const status = archived ? 'ARCHIVED' : workflow.status || 'ACTIVE';
      const updatedValue = workflow.updatedAt || workflow.createdAt;
      return {
        id: workflow.id,
        type: 'Workflow' as ItemType,
        name: workflow.activityName || workflow.activityCode,
        code: workflow.activityCode,
        status,
        version: `v${workflow.version ?? 1}`,
        updated: formatDateLabel(updatedValue),
        updatedAt: updatedValue ? new Date(updatedValue).getTime() || 0 : 0,
        owner: ownerFor(workflow.id),
        visibility: workflow.visibility === 'PUBLIC' ? 'Public' : workflow.visibility === 'PRIVATE' ? 'Private' : 'Workspace',
        favorite: favoriteWorkflowIds.has(workflow.id),
        archived,
      };
    })
  ), [archivedWorkflowIds, favoriteWorkflowIds, overviewWorkflows]);

  const filteredItems = useMemo(() => {
    const keyword = searchValue.trim().toLowerCase();
    return items
      .filter((item) => {
        if (tab === 'workflow' && item.type !== 'Workflow') return false;
        if (tab === 'folder' && item.type !== 'Folder') return false;
        if (tab === 'template') return false;
        if (typeFilter !== 'ALL' && item.type !== typeFilter) return false;
        if (statusFilter !== 'ALL' && item.type === 'Workflow' && item.status !== statusFilter) return false;
        if (visibilityFilter !== 'ALL' && visibilityClass(item.visibility) !== visibilityFilter.toLowerCase()) return false;
        if (ownerFilter !== 'ALL' && item.owner !== ownerFilter) return false;
        if (!keyword) return true;
        return `${item.name} ${item.code} ${item.owner} ${item.visibility}`.toLowerCase().includes(keyword);
      })
      .sort((a, b) => {
        if (sortMode === 'name') return a.name.localeCompare(b.name);
        if (sortMode === 'code') return a.code.localeCompare(b.code);
        if (sortMode === 'version') return b.version.localeCompare(a.version);
        if (sortMode === 'owner') return a.owner.localeCompare(b.owner) || a.name.localeCompare(b.name);
        return b.updatedAt - a.updatedAt || a.name.localeCompare(b.name);
      });
  }, [items, ownerFilter, searchValue, sortMode, statusFilter, tab, typeFilter, visibilityFilter]);
  const filteredTemplates = useMemo(() => {
    const keyword = searchValue.trim().toLowerCase();
    if (tab !== 'template') return [];
    return templates
      .filter((template) => !keyword || `${template.name} ${template.category ?? ''}`.toLowerCase().includes(keyword))
      .sort((a, b) => {
        if (sortMode === 'name') return a.name.localeCompare(b.name);
        return (new Date(b.updatedAt ?? 0).getTime() || 0) - (new Date(a.updatedAt ?? 0).getTime() || 0);
      });
  }, [searchValue, sortMode, tab, templates]);

  const totalCount = tab === 'template' ? filteredTemplates.length : filteredItems.length;
  const pageCount = Math.max(1, Math.ceil(totalCount / pageSize));
  const safePage = Math.min(page, pageCount);
  const pageItems = filteredItems.slice((safePage - 1) * pageSize, safePage * pageSize);
  const pageTemplates = filteredTemplates.slice((safePage - 1) * pageSize, safePage * pageSize);
  const ownerOptions = useMemo(() => Array.from(new Set(items.map((item) => item.owner))).sort(), [items]);
  const checkedItems = useMemo(() => items.filter((item) => checkedItemIds.has(item.id)), [checkedItemIds, items]);
  const allPageChecked = pageItems.length > 0 && pageItems.every((item) => checkedItemIds.has(item.id));
  const isEmptyPage = !isLoading && ((tab === 'template' && pageTemplates.length === 0) || (tab !== 'template' && pageItems.length === 0));
  const activeWorkflowCount = useMemo(() => overviewWorkflowItems.filter((item) => item.status === 'ACTIVE').length, [overviewWorkflowItems]);
  const draftWorkflowCount = useMemo(() => overviewWorkflowItems.filter((item) => item.status === 'DRAFT').length, [overviewWorkflowItems]);
  const archivedWorkflowItems = useMemo(() => overviewWorkflowItems.filter((item) => item.archived), [overviewWorkflowItems]);
  const favoriteWorkflowItems = useMemo(() => overviewWorkflowItems.filter((item) => item.favorite && !item.archived), [overviewWorkflowItems]);
  const recentWorkflowItems = useMemo(() => (
    overviewWorkflowItems
      .filter((item) => !item.archived)
      .sort((a, b) => b.updatedAt - a.updatedAt || a.name.localeCompare(b.name))
      .slice(0, 4)
  ), [overviewWorkflowItems]);
  const showWorkbenchOverview = !isLoading && !searchValue.trim() && tab === 'all';
  const homeViewTitle = homeView === 'recent' ? '最近编辑'
    : homeView === 'favorites' ? '收藏'
      : homeView === 'archived' ? '归档'
        : '工作区';
  const homeViewDescription = homeView === 'recent' ? '按更新时间组织当前目录的流程。'
    : homeView === 'favorites' ? '只看已收藏的重点活动流程。'
      : homeView === 'archived' ? '集中查看已归档的流程，必要时可以恢复。'
        : '按 workspace、folder、workflow 组织活动资产。';
  const emptyState = useMemo<EmptyState>(() => {
    const hasSearch = searchValue.trim().length > 0;
    const hasAdvancedFilter = typeFilter !== 'ALL' || statusFilter !== 'ALL' || visibilityFilter !== 'ALL' || ownerFilter !== 'ALL';
    const searchTerm = searchValue.trim();
    if (hasSearch) {
      return {
        kind: 'search',
        title: `没有找到 "${searchTerm}"`,
        description: hasAdvancedFilter ? '当前搜索和筛选没有匹配项。' : '换个关键词，或直接创建新的流程。',
        actions: [
          { label: hasAdvancedFilter ? '清除筛选' : '清空搜索', tone: 'primary' },
          { label: '新建流程', tone: 'secondary' },
          { label: '查看归档', tone: 'secondary' },
        ],
      };
    }
    if (visibilityFilter === 'Private') {
      return {
        kind: 'permission',
        title: '没有可见的私有内容',
        description: '当前筛选下没有你可见的私有项目，可以切换可见性筛选或联系管理员授权。',
        actions: [
          { label: '重置筛选', tone: 'primary' },
          { label: '管理权限', tone: 'secondary' },
        ],
      };
    }
    if (statusFilter === 'ARCHIVED' || tab === 'all' && hasAdvancedFilter && archivedWorkflowIds.size === 0) {
      return {
        kind: 'archived',
        title: '归档为空',
        description: '当前 workspace 下还没有归档的 workflow。归档后的项目会在这里集中查看。',
        actions: [
          { label: '查看全部', tone: 'primary' },
          { label: '新建流程', tone: 'secondary' },
        ],
      };
    }
    if (tab === 'template') {
      return {
        kind: 'template',
        title: '暂无模板',
        description: '还没有可用模板。你可以先创建流程，再保存为模板复用。',
        actions: [
          { label: '新建流程', tone: 'primary' },
          { label: '查看全部', tone: 'secondary' },
        ],
      };
    }
    if (hasAdvancedFilter) {
      return {
        kind: 'filter',
        title: '没有符合筛选的内容',
        description: '当前筛选组合没有匹配项目，重置筛选后可以查看完整目录。',
        actions: [
          { label: '重置筛选', tone: 'primary' },
          { label: '新建流程', tone: 'secondary' },
        ],
      };
    }
    return {
      kind: 'folder',
      title: '当前 folder 为空',
      description: '从新建文件夹、新建流程或导入流程开始组织活动。',
      actions: [
        { label: '新建流程', tone: 'primary' },
        { label: '新建文件夹', tone: 'secondary' },
      ],
    };
  }, [archivedWorkflowIds.size, ownerFilter, searchValue, statusFilter, tab, typeFilter, visibilityFilter]);

  const actionPayload = (item: ContentItem) => ({
    id: item.id,
    type: item.type,
    name: item.name,
    visibility: item.visibility,
    status: item.status,
    version: item.version,
    owner: item.owner,
  });

  const selectAndOpen = (item: ContentItem) => {
    setSelectedItemId(item.id);
    onItemSelect(actionPayload(item));
    if (item.type === 'Folder') onFolderOpen(item.id);
    else onWorkflowOpen(item.id);
  };

  const selectOnly = (item: ContentItem) => {
    setSelectedItemId(item.id);
    onItemSelect(actionPayload(item));
  };

  const toggleChecked = (itemId: string) => {
    setCheckedItemIds((current) => {
      const next = new Set(current);
      if (next.has(itemId)) next.delete(itemId);
      else next.add(itemId);
      return next;
    });
  };

  const togglePageChecked = () => {
    setCheckedItemIds((current) => {
      const next = new Set(current);
      if (allPageChecked) pageItems.forEach((item) => next.delete(item.id));
      else pageItems.forEach((item) => next.add(item.id));
      return next;
    });
  };

  const renderStatus = (item: ContentItem) => (
    <span className={`exm-status-pill ${item.status.toLowerCase()}`}>{statusLabel(item.status)}</span>
  );

  const renderVisibility = (item: ContentItem) => (
    <span className={`exm-visibility-pill ${visibilityClass(item.visibility)}`}>{visibilityLabel(item.visibility)}</span>
  );

  const canRunAction = (action: WorkspaceItemAction, item?: ContentItem): boolean => {
    if (action === 'copyLink') return true;
    if (action === 'permission' || action === 'share') return capabilities.canManageMembers;
    if (action === 'move' || action === 'rename' || action === 'archive') {
      return item?.type === 'Folder' ? capabilities.canManageFolders : capabilities.canEditWorkspace;
    }
    return capabilities.canEditWorkspace;
  };

  const disabledReason = (action: string): string => `${capabilities.reason}，不能执行${action}。`;

  const renderItemMenu = (item: ContentItem) => (
    <TaMenu className="exm-item-menu exm-menu-scope" onClick={(event) => event.stopPropagation()}>
      {MENU_ACTIONS.map(({ action, label, icon: Icon, danger }) => (
        <TaMenuItem key={action} danger={danger} data-testid={`workspace-row-action-${action}`} disabled={!canRunAction(action, item)} title={!canRunAction(action, item) ? disabledReason(label) : label} onClick={() => {
          setOpenMenuItemId('');
          onItemAction(action, actionPayload(item));
        }}>
          <Icon size={14} />{label}
        </TaMenuItem>
      ))}
    </TaMenu>
  );

  const resetFilters = () => {
    onSearchChange('');
    setTypeFilter('ALL');
    onStatusFilterChange('ALL');
    setVisibilityFilter('ALL');
    setOwnerFilter('ALL');
    setPage(1);
  };

  const clearAdvancedFilters = () => {
    setTypeFilter('ALL');
    onStatusFilterChange('ALL');
    setVisibilityFilter('ALL');
    setOwnerFilter('ALL');
    setPage(1);
  };

  const runEmptyAction = (action: string) => {
    if (action === '清空搜索') {
      onSearchChange('');
      setPage(1);
      return;
    }
    if (action === '清除筛选') {
      clearAdvancedFilters();
      return;
    }
    if (action === '查看归档') {
      onSearchChange('');
      setTab('all');
      setTypeFilter('ALL');
      onStatusFilterChange('ARCHIVED');
      setVisibilityFilter('ALL');
      setOwnerFilter('ALL');
      setPage(1);
      return;
    }
    if (action === '重置筛选' || action === '查看全部') {
      resetFilters();
      setTab('all');
      return;
    }
    if (action === '新建文件夹') {
      onCreateFolder();
      return;
    }
    if (action === '管理权限') {
      onCurrentAction('permission');
      return;
    }
    onCreateWorkflow();
  };

  return (
    <section className="exm-content">
      {errorMessage ? (
        <div className="exm-error-banner" role="alert">
          <b>数据同步异常</b>
          <span>{errorMessage}</span>
          {onRetryLoad ? <TaButton size="sm" onClick={onRetryLoad}>重试</TaButton> : null}
        </div>
      ) : null}
      <header className="exm-folder-head ta-card">
        <div className="exm-folder-title">
          <span className="exm-folder-icon"><Folder size={24} fill="currentColor" /></span>
          <div>
            <h1>{folderName}</h1>
            <p>
              <span>{workflows.length} 个流程 · {folders.length} 个文件夹 · {visibilityLabel(folderVisibility)} · {folderUpdatedLabel}</span>
              {!capabilities.canEditWorkspace ? (
                <span className="exm-permission-inline" title={`${capabilities.reason}。当前仍可浏览有权限的内容。`}>
                  {capabilities.role} · 只读浏览
                </span>
              ) : null}
            </p>
          </div>
          <TaIconButton className="icon" aria-label="收藏"><Star size={18} /></TaIconButton>
          <div className="exm-current-menu-wrap exm-menu-scope">
            <TaIconButton className="icon" aria-label="更多" onClick={() => setShowCurrentMenu((value) => !value)}><MoreHorizontal size={18} /></TaIconButton>
            {showCurrentMenu ? (
              <TaMenu className="exm-item-menu current exm-menu-scope">
                {MENU_ACTIONS.map(({ action, label, icon: Icon, danger }) => (
                  <TaMenuItem key={action} danger={danger} disabled={!canRunAction(action)} title={!canRunAction(action) ? disabledReason(label) : label} onClick={() => {
                    setShowCurrentMenu(false);
                    onCurrentAction(action);
                  }}>
                    <Icon size={14} />{label}
                  </TaMenuItem>
                ))}
              </TaMenu>
            ) : null}
          </div>
        </div>
        <div className="exm-folder-actions">
          {!capabilities.canEditWorkspace ? <span className="exm-action-note">{capabilities.role} 只能查看内容</span> : null}
          <TaButton data-testid="workspace-new-folder" disabled={!capabilities.canManageFolders} title={!capabilities.canManageFolders ? disabledReason('新建文件夹') : '新建文件夹'} onClick={onCreateFolder}>新建文件夹</TaButton>
          <TaButton disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('导入流程') : '导入流程'} onClick={onImportWorkflow}>导入流程</TaButton>
          <TaButton className="primary" tone="primary" data-testid="workspace-new-workflow" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('新建流程') : '新建流程'} onClick={onCreateWorkflow}>新建流程</TaButton>
        </div>
      </header>

      <nav className="exm-content-breadcrumb">
        <div className="exm-breadcrumb-path">
          <TaButton tone="ghost" size="sm">{workspaceName}</TaButton>
          <span>/</span>
          <TaButton tone="ghost" size="sm">{folderName}</TaButton>
        </div>
        <div className="exm-breadcrumb-actions">
          {!capabilities.canEditWorkspace ? <span className="exm-action-note">{capabilities.role} 只能查看内容</span> : null}
          <TaButton size="sm" data-testid="workspace-new-folder" disabled={!capabilities.canManageFolders} title={!capabilities.canManageFolders ? disabledReason('新建文件夹') : '新建文件夹'} onClick={onCreateFolder}>新建文件夹</TaButton>
          <TaButton size="sm" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('导入流程') : '导入流程'} onClick={onImportWorkflow}>导入流程</TaButton>
          <TaButton size="sm" tone="primary" data-testid="workspace-new-workflow" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('新建流程') : '新建流程'} onClick={onCreateWorkflow}>新建流程</TaButton>
        </div>
      </nav>

      {showWorkbenchOverview ? (
        <section className="exm-workbench-overview ta-card" data-testid="workspace-workbench-overview">
          <div className="exm-workbench-overview-head">
            <div>
              <span>{homeViewTitle}</span>
              <h2>{folderName} 工作台</h2>
              <p>{homeViewDescription}</p>
            </div>
            <div className="exm-workbench-quick-actions">
              <TaButton size="sm" onClick={() => onNavigateHomeView('recent')}><Clock3 size={14} />最近</TaButton>
              <TaButton size="sm" onClick={() => onNavigateHomeView('favorites')}><Star size={14} />收藏</TaButton>
              <TaButton size="sm" onClick={() => onNavigateHomeView('archived')}><Archive size={14} />归档</TaButton>
            </div>
          </div>
          <div className="exm-workbench-stats">
            <button type="button" onClick={() => onNavigateHomeView('workspace')}>
              <span>流程</span>
              <strong>{overviewWorkflowItems.length}</strong>
              <em>{activeWorkflowCount} 启用 · {draftWorkflowCount} 草稿</em>
            </button>
            <button type="button" onClick={() => setTab('folder')}>
              <span>文件夹</span>
              <strong>{folders.length}</strong>
              <em>{visibilityLabel(folderVisibility)} 可见</em>
            </button>
            <button type="button" onClick={() => onNavigateHomeView('favorites')}>
              <span>收藏</span>
              <strong>{favoriteWorkflowItems.length}</strong>
              <em>重点流程快速定位</em>
            </button>
            <button type="button" onClick={() => onNavigateHomeView('archived')}>
              <span>归档</span>
              <strong>{archivedWorkflowItems.length}</strong>
              <em>历史流程集中管理</em>
            </button>
          </div>
          <div className="exm-workbench-lanes">
            <div>
              <header><Clock3 size={14} />最近编辑</header>
              {recentWorkflowItems.length ? recentWorkflowItems.slice(0, 3).map((item) => (
                <button key={item.id} type="button" onClick={() => selectAndOpen(item)}>
                  <b>{item.name}</b>
                  <span>{item.updated} · {item.version}</span>
                </button>
              )) : <p>暂无最近编辑流程</p>}
            </div>
            <div>
              <header><Star size={14} />收藏</header>
              {favoriteWorkflowItems.length ? favoriteWorkflowItems.slice(0, 3).map((item) => (
                <button key={item.id} type="button" onClick={() => selectAndOpen(item)}>
                  <b>{item.name}</b>
                  <span>{item.owner} · {statusLabel(item.status)}</span>
                </button>
              )) : (
                <button type="button" className="empty-action" onClick={() => onNavigateHomeView('workspace')}>
                  <StarOff size={14} />
                  <span>从列表星标重点流程</span>
                </button>
              )}
            </div>
            <div>
              <header><Archive size={14} />归档</header>
              {archivedWorkflowItems.length ? archivedWorkflowItems.slice(0, 3).map((item) => (
                <button key={item.id} type="button" onClick={() => selectAndOpen(item)}>
                  <b>{item.name}</b>
                  <span>{item.code}</span>
                </button>
              )) : <p>当前目录没有归档流程</p>}
            </div>
          </div>
        </section>
      ) : null}

      <TaTabs className="exm-content-tabs">
        {([
          ['all', '全部'],
          ['workflow', '流程'],
          ['folder', '文件夹'],
          ['template', '模板'],
        ] as Array<[TabMode, string]>).map(([value, label]) => (
          <TaTab key={value} active={tab === value} onClick={() => {
            setTab(value);
            setPage(1);
          }}>{label}</TaTab>
        ))}
      </TaTabs>

      <div className="exm-content-tools ta-card">
        <label className="exm-folder-search">
          <Search size={15} />
          <TaInput value={searchValue} placeholder="搜索当前文件夹" onChange={(event) => {
            onSearchChange(event.target.value);
            setPage(1);
          }} />
        </label>
        <TaSelect value={sortMode} onChange={(event) => onSortChange(event.target.value as typeof sortMode)}>
          <option value="recent">更新时间</option>
          <option value="name">名称</option>
          <option value="owner">负责人</option>
          <option value="code">编码</option>
          <option value="version">版本</option>
        </TaSelect>
        <TaButton className={showAdvancedFilters ? 'active' : ''} onClick={() => setShowAdvancedFilters((value) => !value)}><SlidersHorizontal size={15} />筛选</TaButton>
        <div className="exm-view-toggle">
          <TaButton className={viewMode === 'list' ? 'active' : ''} onClick={() => setViewMode('list')}><List size={15} />列表</TaButton>
          <TaButton className={viewMode === 'grid' ? 'active' : ''} onClick={() => setViewMode('grid')}><Grid3X3 size={15} />网格</TaButton>
          <TaButton className={viewMode === 'compact' ? 'active' : ''} onClick={() => setViewMode('compact')}><Rows3 size={15} />紧凑</TaButton>
        </div>
      </div>

      {showAdvancedFilters ? (
        <div className="exm-advanced-filters">
          <label>
            <span>类型</span>
            <TaSelect value={typeFilter} onChange={(event) => {
              setTypeFilter(event.target.value as typeof typeFilter);
              setPage(1);
            }}>
              <option value="ALL">全部类型</option>
              <option value="Workflow">流程</option>
              <option value="Folder">文件夹</option>
            </TaSelect>
          </label>
          <label>
            <span>状态</span>
            <TaSelect value={statusFilter} onChange={(event) => {
              onStatusFilterChange(event.target.value as typeof statusFilter);
              setPage(1);
            }}>
              <option value="ALL">全部状态</option>
              <option value="ACTIVE">已启用</option>
              <option value="DRAFT">草稿</option>
              <option value="ARCHIVED">已归档</option>
            </TaSelect>
          </label>
          <label>
            <span>可见性</span>
            <TaSelect value={visibilityFilter} onChange={(event) => {
              setVisibilityFilter(event.target.value as typeof visibilityFilter);
              setPage(1);
            }}>
              <option value="ALL">全部权限</option>
              <option value="Private">私有</option>
              <option value="Workspace">工作区</option>
              <option value="Public">公开</option>
            </TaSelect>
          </label>
          <label>
            <span>负责人</span>
            <TaSelect value={ownerFilter} onChange={(event) => {
              setOwnerFilter(event.target.value);
              setPage(1);
            }}>
              <option value="ALL">全部负责人</option>
              {ownerOptions.map((owner) => <option key={owner} value={owner}>{owner}</option>)}
            </TaSelect>
          </label>
          <TaButton size="sm" onClick={() => {
            setTypeFilter('ALL');
            onStatusFilterChange('ALL');
            setVisibilityFilter('ALL');
            setOwnerFilter('ALL');
            setPage(1);
          }}>重置筛选</TaButton>
        </div>
      ) : null}

      {checkedItems.length ? (
        <div className="exm-bulk-bar">
          <span>已选择 {checkedItems.length} 项</span>
          <TaButton size="sm" data-testid="workspace-bulk-archive" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('批量归档') : '批量归档'} onClick={() => onBulkArchive(checkedItems.map(actionPayload))}>批量归档</TaButton>
          <TaButton size="sm" data-testid="workspace-bulk-move" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('批量移动') : '批量移动'} onClick={() => onBulkAction('move', checkedItems.map(actionPayload))}>批量移动</TaButton>
          <TaButton size="sm" data-testid="workspace-bulk-permission" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? disabledReason('批量权限') : '批量权限'} onClick={() => onBulkAction('permission', checkedItems.map(actionPayload))}>批量权限</TaButton>
          <TaButton size="sm" onClick={() => onBulkAction('copyLink', checkedItems.map(actionPayload))}>批量复制链接</TaButton>
          <TaButton tone="ghost" size="sm" onClick={() => setCheckedItemIds(new Set())}>清除选择</TaButton>
        </div>
      ) : null}

      {isLoading ? (
        <div className="exm-content-loading" aria-label="工作台加载中" />
      ) : isEmptyPage ? (
        <TaEmptyState className={`exm-content-empty ${emptyState.kind}`}>
          <b>{emptyState.title}</b>
          <span>{emptyState.description}</span>
          <div>
            {emptyState.actions.map((action, index) => (
              <TaButton key={action.label} tone={index === 0 && action.tone !== 'secondary' ? 'primary' : 'outline'} className={index === 0 && action.tone !== 'secondary' ? '' : 'secondary'} onClick={() => runEmptyAction(action.label)}>{action.label}</TaButton>
            ))}
          </div>
        </TaEmptyState>
      ) : tab === 'template' ? (
        <div className="exm-grid-view exm-template-grid">
          {pageTemplates.map((template) => (
            <TaButton key={template.id} className="exm-template-card" onClick={() => onTemplateOpen(template.id)}>
              <MoreHorizontal className="more" size={16} />
              <span className="template icon"><Workflow size={28} /></span>
              <b>{template.name}</b>
              <em>{template.category || '团队模板'} · v{template.version ?? 1}</em>
              <footer className="template-footer"><span>{formatDateLabel(template.updatedAt)}</span></footer>
            </TaButton>
          ))}
        </div>
      ) : viewMode === 'grid' ? (
        <div className="exm-grid-view">
          {pageItems.map((item) => (
            <div key={item.id} className={selectedItemId === item.id ? 'exm-grid-card selected' : 'exm-grid-card'} role="button" tabIndex={0} onClick={() => selectAndOpen(item)} onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') selectAndOpen(item);
            }}>
              <TaCheckbox
                className="grid-check"
                checked={checkedItemIds.has(item.id)}
                onClick={(event) => event.stopPropagation()}
                onChange={() => toggleChecked(item.id)}
              />
              <MoreHorizontal className="more exm-menu-scope" size={16} onClick={(event) => {
                event.stopPropagation();
                selectOnly(item);
                setOpenMenuItemId(openMenuItemId === item.id ? '' : item.id);
              }} />
              {openMenuItemId === item.id ? (
                renderItemMenu(item)
              ) : null}
              <span className={item.type === 'Folder' ? 'folder icon' : 'workflow icon'}>{item.type === 'Folder' ? <Folder size={28} fill="currentColor" /> : <Workflow size={28} />}</span>
              <b>{renderKeyword(item.name, searchValue)}</b>
              <em>{typeLabel(item.type)}{item.type === 'Workflow' ? ` · ${statusLabel(item.status)} · ${item.version}` : ` · ${item.folderCount ?? 0} 个流程`}</em>
              <footer><span>{item.updated}</span><strong>{item.owner.slice(0, 1)}</strong>{item.owner}</footer>
            </div>
          ))}
        </div>
      ) : (
        <TaTable className={viewMode === 'compact' ? 'exm-table-view compact' : 'exm-table-view'}>
          <TaTableRow header className="exm-table-row">
            <span><TaCheckbox checked={allPageChecked} onChange={togglePageChecked} /></span>
            <span>名称</span>
            <span>类型</span>
            <span>状态</span>
            <span>版本</span>
            <span>更新</span>
            <span>负责人</span>
            <span>可见性</span>
            <span>操作</span>
          </TaTableRow>
          {pageItems.map((item) => (
            <TaTableRow key={item.id} selected={selectedItemId === item.id} className="exm-table-row" data-testid={`workspace-row-${item.type.toLowerCase()}`} data-item-id={item.id} data-item-name={item.name} role="button" tabIndex={0} onClick={() => selectAndOpen(item)} onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') selectAndOpen(item);
            }}>
              <span><TaCheckbox data-testid="workspace-row-checkbox" checked={checkedItemIds.has(item.id)} onClick={(event) => event.stopPropagation()} onChange={() => toggleChecked(item.id)} /></span>
              <span className="name-cell">{item.type === 'Folder' ? <Folder size={17} fill="currentColor" /> : <Workflow size={17} />}<b>{renderKeyword(item.name, searchValue)}</b></span>
              <span>{typeLabel(item.type)}</span>
              <span>{item.type === 'Workflow' ? renderStatus(item) : '-'}</span>
              <span>{item.version}</span>
              <span>{item.updated}</span>
              <span><i>{item.owner.slice(0, 1)}</i>{item.owner}</span>
              <span>{renderVisibility(item)}</span>
              <span className="actions exm-menu-scope">
                {item.type === 'Workflow' ? <em onClick={(event) => { event.stopPropagation(); onToggleFavorite(item.id); }}>{item.favorite ? '★' : '☆'}</em> : null}
                {item.type === 'Workflow' ? <em onClick={(event) => { event.stopPropagation(); onToggleArchived(item.id); }}>{item.archived ? '恢复' : '归档'}</em> : null}
                <MoreHorizontal data-testid="workspace-row-more" size={16} onClick={(event) => {
                  event.stopPropagation();
                  selectOnly(item);
                  setOpenMenuItemId(openMenuItemId === item.id ? '' : item.id);
                }} />
                {openMenuItemId === item.id ? (
                  renderItemMenu(item)
                ) : null}
              </span>
            </TaTableRow>
          ))}
        </TaTable>
      )}

      {totalCount > 0 ? (
        <TaPagination className="exm-pagination">
          <span>共 {totalCount} 项</span>
          <div>
            <TaIconButton aria-label="上一页" disabled={safePage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft size={15} /></TaIconButton>
            <strong>{safePage}</strong>
            <TaIconButton aria-label="下一页" disabled={safePage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}><ChevronRight size={15} /></TaIconButton>
          </div>
          <TaSelect value={pageSize} onChange={(event) => {
            setPageSize(Number(event.target.value));
            setPage(1);
          }}>
            <option value={10}>10 / 页</option>
            <option value={20}>20 / 页</option>
            <option value={50}>50 / 页</option>
          </TaSelect>
        </TaPagination>
      ) : null}
    </section>
  );
}
