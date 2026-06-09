import { type ReactNode, useEffect, useMemo, useState } from 'react';
import {
  Archive,
  CheckSquare,
  ChevronLeft,
  ChevronRight,
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
  UserRoundCog,
  Workflow,
} from 'lucide-react';

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
  folders: WorkspaceContentFolder[];
  workflows: WorkspaceContentWorkflow[];
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
  folders,
  workflows,
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
  const emptyState = useMemo(() => {
    const hasSearch = searchValue.trim().length > 0;
    const hasAdvancedFilter = typeFilter !== 'ALL' || statusFilter !== 'ALL' || visibilityFilter !== 'ALL' || ownerFilter !== 'ALL';
    if (hasSearch) {
      return {
        kind: 'search',
        title: '没有搜索结果',
        description: `没有找到包含 "${searchValue.trim()}" 的 workflow、folder 或 template。`,
        primary: '清空搜索',
        secondary: '新建 Workflow',
      };
    }
    if (visibilityFilter === 'Private') {
      return {
        kind: 'permission',
        title: '没有可见的 Private 内容',
        description: '当前筛选下没有你可见的私有项目，可以切换可见性筛选或联系管理员授权。',
        primary: '重置筛选',
        secondary: '管理权限',
      };
    }
    if (statusFilter === 'ARCHIVED' || tab === 'all' && hasAdvancedFilter && archivedWorkflowIds.size === 0) {
      return {
        kind: 'archived',
        title: '归档为空',
        description: '当前 workspace 下还没有归档的 workflow。归档后的项目会在这里集中查看。',
        primary: '查看全部',
        secondary: '新建 Workflow',
      };
    }
    if (tab === 'template') {
      return {
        kind: 'template',
        title: '暂无模板',
        description: '还没有可用模板。你可以先创建 workflow，再保存为模板复用。',
        primary: '新建 Workflow',
        secondary: '查看全部',
      };
    }
    if (hasAdvancedFilter) {
      return {
        kind: 'filter',
        title: '没有符合筛选的内容',
        description: '当前筛选组合没有匹配项目，重置筛选后可以查看完整目录。',
        primary: '重置筛选',
        secondary: '新建 Workflow',
      };
    }
    return {
      kind: 'folder',
      title: '当前 folder 为空',
      description: '从新建 Folder、Workflow 或导入 Workflow 开始组织活动。',
      primary: '新建 Workflow',
      secondary: '新建 Folder',
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
    <span className={`exm-status-pill ${item.status.toLowerCase()}`}>{item.status}</span>
  );

  const renderVisibility = (item: ContentItem) => (
    <span className={`exm-visibility-pill ${visibilityClass(item.visibility)}`}>{item.visibility}</span>
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
    <div className="exm-item-menu exm-menu-scope" onClick={(event) => event.stopPropagation()}>
      {MENU_ACTIONS.map(({ action, label, icon: Icon, danger }) => (
        <button key={action} className={danger ? 'danger' : ''} type="button" data-testid={`workspace-row-action-${action}`} disabled={!canRunAction(action, item)} title={!canRunAction(action, item) ? disabledReason(label) : label} onClick={() => {
          setOpenMenuItemId('');
          onItemAction(action, actionPayload(item));
        }}>
          <Icon size={14} />{label}
        </button>
      ))}
    </div>
  );

  const resetFilters = () => {
    onSearchChange('');
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
    if (action === '重置筛选' || action === '查看全部') {
      resetFilters();
      setTab('all');
      return;
    }
    if (action === '新建 Folder') {
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
          {onRetryLoad ? <button type="button" onClick={onRetryLoad}>重试</button> : null}
        </div>
      ) : null}
      {!capabilities.canEditWorkspace ? (
        <div className="exm-permission-banner">
          <b>{capabilities.role}</b>
          <span>{capabilities.reason}。当前仍可浏览有权限的内容。</span>
        </div>
      ) : null}
      <header className="exm-folder-head">
        <div className="exm-folder-title">
          <span className="exm-folder-icon"><Folder size={24} fill="currentColor" /></span>
          <div>
            <h1>{folderName}</h1>
            <p>{workflows.length} workflows · {folders.length} folders · {folderVisibility} · {folderUpdatedLabel}</p>
          </div>
          <button className="icon" type="button" aria-label="收藏"><Star size={18} /></button>
          <div className="exm-current-menu-wrap exm-menu-scope">
            <button className="icon" type="button" aria-label="更多" onClick={() => setShowCurrentMenu((value) => !value)}><MoreHorizontal size={18} /></button>
            {showCurrentMenu ? (
              <div className="exm-item-menu current exm-menu-scope">
                {MENU_ACTIONS.map(({ action, label, icon: Icon, danger }) => (
                  <button key={action} className={danger ? 'danger' : ''} type="button" disabled={!canRunAction(action)} title={!canRunAction(action) ? disabledReason(label) : label} onClick={() => {
                    setShowCurrentMenu(false);
                    onCurrentAction(action);
                  }}>
                    <Icon size={14} />{label}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
        </div>
        <div className="exm-folder-actions">
          <button type="button" data-testid="workspace-new-folder" disabled={!capabilities.canManageFolders} title={!capabilities.canManageFolders ? disabledReason('新建 Folder') : '新建 Folder'} onClick={onCreateFolder}>新建 Folder</button>
          <button type="button" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('导入 Workflow') : '导入 Workflow'} onClick={onImportWorkflow}>导入 Workflow</button>
          <button className="primary" type="button" data-testid="workspace-new-workflow" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('新建 Workflow') : '新建 Workflow'} onClick={onCreateWorkflow}>新建 Workflow</button>
        </div>
      </header>

      <nav className="exm-content-breadcrumb">
        <button type="button">{workspaceName}</button>
        <span>/</span>
        <button type="button">{folderName}</button>
      </nav>

      <div className="exm-content-tabs">
        {([
          ['all', '全部'],
          ['workflow', 'Workflow'],
          ['folder', 'Folder'],
          ['template', 'Template'],
        ] as Array<[TabMode, string]>).map(([value, label]) => (
          <button key={value} className={tab === value ? 'active' : ''} type="button" onClick={() => {
            setTab(value);
            setPage(1);
          }}>{label}</button>
        ))}
      </div>

      <div className="exm-content-tools">
        <label className="exm-folder-search">
          <Search size={15} />
          <input value={searchValue} placeholder="搜索当前文件夹" onChange={(event) => {
            onSearchChange(event.target.value);
            setPage(1);
          }} />
        </label>
        <select value={typeFilter} onChange={(event) => {
          setTypeFilter(event.target.value as typeof typeFilter);
          setPage(1);
        }}>
          <option value="ALL">全部类型</option>
          <option value="Workflow">Workflow</option>
          <option value="Folder">Folder</option>
        </select>
        <select value={statusFilter} onChange={(event) => {
          onStatusFilterChange(event.target.value as typeof statusFilter);
          setPage(1);
        }}>
          <option value="ALL">全部状态</option>
          <option value="ACTIVE">Active</option>
          <option value="DRAFT">Draft</option>
          <option value="ARCHIVED">Archived</option>
        </select>
        <select value={visibilityFilter} onChange={(event) => {
          setVisibilityFilter(event.target.value as typeof visibilityFilter);
          setPage(1);
        }}>
          <option value="ALL">全部权限</option>
          <option value="Private">Private</option>
          <option value="Workspace">Workspace</option>
          <option value="Public">Public</option>
        </select>
        <select value={ownerFilter} onChange={(event) => {
          setOwnerFilter(event.target.value);
          setPage(1);
        }}>
          <option value="ALL">全部 Owner</option>
          {ownerOptions.map((owner) => <option key={owner} value={owner}>{owner}</option>)}
        </select>
        <select value={sortMode} onChange={(event) => onSortChange(event.target.value as typeof sortMode)}>
          <option value="recent">更新时间</option>
          <option value="name">名称</option>
          <option value="owner">Owner</option>
          <option value="code">Code</option>
          <option value="version">版本</option>
        </select>
        <button className={showAdvancedFilters ? 'active' : ''} type="button" onClick={() => setShowAdvancedFilters((value) => !value)}><SlidersHorizontal size={15} />筛选</button>
        <div className="exm-view-toggle">
          <button className={viewMode === 'list' ? 'active' : ''} type="button" onClick={() => setViewMode('list')}><List size={15} />列表</button>
          <button className={viewMode === 'grid' ? 'active' : ''} type="button" onClick={() => setViewMode('grid')}><Grid3X3 size={15} />网格</button>
          <button className={viewMode === 'compact' ? 'active' : ''} type="button" onClick={() => setViewMode('compact')}><Rows3 size={15} />紧凑</button>
        </div>
      </div>

      {showAdvancedFilters ? (
        <div className="exm-advanced-filters">
          <label>
            <span>类型</span>
            <select value={typeFilter} onChange={(event) => {
              setTypeFilter(event.target.value as typeof typeFilter);
              setPage(1);
            }}>
              <option value="ALL">全部类型</option>
              <option value="Workflow">Workflow</option>
              <option value="Folder">Folder</option>
            </select>
          </label>
          <label>
            <span>状态</span>
            <select value={statusFilter} onChange={(event) => {
              onStatusFilterChange(event.target.value as typeof statusFilter);
              setPage(1);
            }}>
              <option value="ALL">全部状态</option>
              <option value="ACTIVE">Active</option>
              <option value="DRAFT">Draft</option>
              <option value="ARCHIVED">Archived</option>
            </select>
          </label>
          <label>
            <span>可见性</span>
            <select value={visibilityFilter} onChange={(event) => {
              setVisibilityFilter(event.target.value as typeof visibilityFilter);
              setPage(1);
            }}>
              <option value="ALL">全部权限</option>
              <option value="Private">Private</option>
              <option value="Workspace">Workspace</option>
              <option value="Public">Public</option>
            </select>
          </label>
          <label>
            <span>Owner</span>
            <select value={ownerFilter} onChange={(event) => {
              setOwnerFilter(event.target.value);
              setPage(1);
            }}>
              <option value="ALL">全部 Owner</option>
              {ownerOptions.map((owner) => <option key={owner} value={owner}>{owner}</option>)}
            </select>
          </label>
          <button type="button" onClick={() => {
            setTypeFilter('ALL');
            onStatusFilterChange('ALL');
            setVisibilityFilter('ALL');
            setOwnerFilter('ALL');
            setPage(1);
          }}>重置筛选</button>
        </div>
      ) : null}

      {checkedItems.length ? (
        <div className="exm-bulk-bar">
          <span>已选择 {checkedItems.length} 项</span>
          <button type="button" data-testid="workspace-bulk-archive" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('批量归档') : '批量归档'} onClick={() => onBulkArchive(checkedItems.map(actionPayload))}>批量归档</button>
          <button type="button" data-testid="workspace-bulk-move" disabled={!capabilities.canEditWorkspace} title={!capabilities.canEditWorkspace ? disabledReason('批量移动') : '批量移动'} onClick={() => onBulkAction('move', checkedItems.map(actionPayload))}>批量移动</button>
          <button type="button" data-testid="workspace-bulk-permission" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? disabledReason('批量权限') : '批量权限'} onClick={() => onBulkAction('permission', checkedItems.map(actionPayload))}>批量权限</button>
          <button type="button" onClick={() => onBulkAction('copyLink', checkedItems.map(actionPayload))}>批量复制链接</button>
          <button type="button" onClick={() => setCheckedItemIds(new Set())}>清除选择</button>
        </div>
      ) : null}

      {isLoading ? (
        <div className="exm-table-view loading" aria-label="工作台加载中">
          <div className="exm-table-row head">
            <span />
            <span>Name</span>
            <span>Type</span>
            <span>Status</span>
            <span>Version</span>
            <span>Updated</span>
            <span>Owner</span>
            <span>Visibility</span>
            <span>操作</span>
          </div>
          {Array.from({ length: 8 }).map((_, index) => (
            <div key={index} className="exm-table-row skeleton-row">
              <span><i className="exm-skeleton" /></span>
              <span className="name-cell"><i className="exm-skeleton icon-skeleton" /><b className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
              <span><i className="exm-skeleton" /></span>
            </div>
          ))}
        </div>
      ) : isEmptyPage ? (
        <div className={`exm-content-empty ${emptyState.kind}`}>
          <b>{emptyState.title}</b>
          <span>{emptyState.description}</span>
          <div>
            <button type="button" onClick={() => runEmptyAction(emptyState.primary)}>{emptyState.primary}</button>
            <button className="secondary" type="button" onClick={() => runEmptyAction(emptyState.secondary)}>{emptyState.secondary}</button>
          </div>
        </div>
      ) : tab === 'template' ? (
        <div className="exm-grid-view">
          {pageTemplates.map((template) => (
            <button key={template.id} type="button" onClick={() => onTemplateOpen(template.id)}>
              <MoreHorizontal className="more" size={16} />
              <span className="workflow icon"><Workflow size={28} /></span>
              <b>{template.name}</b>
              <em>Template · {template.category || '团队模板'} · v{template.version ?? 1}</em>
              <footer><span>{formatDateLabel(template.updatedAt)}</span><strong>T</strong>Template</footer>
            </button>
          ))}
        </div>
      ) : viewMode === 'grid' ? (
        <div className="exm-grid-view">
          {pageItems.map((item) => (
            <div key={item.id} className={selectedItemId === item.id ? 'exm-grid-card selected' : 'exm-grid-card'} role="button" tabIndex={0} onClick={() => selectAndOpen(item)} onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') selectAndOpen(item);
            }}>
              <input
                className="grid-check"
                checked={checkedItemIds.has(item.id)}
                type="checkbox"
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
              <em>{item.type}{item.type === 'Workflow' ? ` · ${item.status} · ${item.version}` : ` · ${item.folderCount ?? 0} workflows`}</em>
              <footer><span>{item.updated}</span><strong>{item.owner.slice(0, 1)}</strong>{item.owner}</footer>
            </div>
          ))}
        </div>
      ) : (
        <div className={viewMode === 'compact' ? 'exm-table-view compact' : 'exm-table-view'}>
          <div className="exm-table-row head">
            <span><input checked={allPageChecked} type="checkbox" onChange={togglePageChecked} /></span>
            <span>Name</span>
            <span>Type</span>
            <span>Status</span>
            <span>Version</span>
            <span>Updated</span>
            <span>Owner</span>
            <span>Visibility</span>
            <span>操作</span>
          </div>
          {pageItems.map((item) => (
            <div key={item.id} className={selectedItemId === item.id ? 'exm-table-row selected' : 'exm-table-row'} data-testid={`workspace-row-${item.type.toLowerCase()}`} data-item-id={item.id} data-item-name={item.name} role="button" tabIndex={0} onClick={() => selectAndOpen(item)} onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') selectAndOpen(item);
            }}>
              <span><input data-testid="workspace-row-checkbox" checked={checkedItemIds.has(item.id)} type="checkbox" onClick={(event) => event.stopPropagation()} onChange={() => toggleChecked(item.id)} /></span>
              <span className="name-cell">{item.type === 'Folder' ? <Folder size={17} fill="currentColor" /> : <Workflow size={17} />}<b>{renderKeyword(item.name, searchValue)}</b></span>
              <span>{item.type}</span>
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
            </div>
          ))}
        </div>
      )}

      {totalCount > 0 ? (
        <footer className="exm-pagination">
          <span>共 {totalCount} 项</span>
          <div>
            <button type="button" aria-label="上一页" disabled={safePage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}><ChevronLeft size={15} /></button>
            <strong>{safePage}</strong>
            <button type="button" aria-label="下一页" disabled={safePage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}><ChevronRight size={15} /></button>
          </div>
          <select value={pageSize} onChange={(event) => {
            setPageSize(Number(event.target.value));
            setPage(1);
          }}>
            <option value={10}>10 / 页</option>
            <option value={20}>20 / 页</option>
            <option value={50}>50 / 页</option>
          </select>
        </footer>
      ) : null}
    </section>
  );
}
