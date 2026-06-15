import {
  Archive,
  ChevronDown,
  Clock3,
  Database,
  FileClock,
  Folder,
  FolderOpen,
  KeyRound,
  Plus,
  Star,
  Users,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { TaButton, TaIconButton, TaInput, TaMenu, TaMenuItem, TaNavButton } from './TailAdminPrimitives';

export type WorkspaceSidebarWorkspace = {
  id: string;
  name: string;
  slug: string;
  color?: string;
  visibility: string;
};

export type WorkspaceSidebarFolder = {
  id: string;
  workspaceId: string;
  parentId?: string;
  name: string;
  visibility: string;
  sortOrder: number;
};

type WorkspaceSidebarProps = {
  workspaces: WorkspaceSidebarWorkspace[];
  folders: WorkspaceSidebarFolder[];
  selectedWorkspaceId?: string;
  selectedFolderId?: string;
  activeNav: 'workspace' | 'recent' | 'shared' | 'favorites' | 'archived';
  workflowCountByFolder: Record<string, number>;
  recentCount: number;
  sharedCount: number;
  favoriteCount: number;
  archivedCount: number;
  memberCount: number;
  auditCount: number;
  storageUsedGb?: number;
  storageTotalGb?: number;
  isLoading?: boolean;
  onWorkspaceChange: (workspaceId: string) => void;
  onFolderSelect: (folderId: string) => void;
  onCreateWorkspace: () => void;
  onNavRecent: () => void;
  onNavWorkspace: () => void;
  onNavShared: () => void;
  onNavFavorites: () => void;
  onNavArchived: () => void;
  onNavMembers: () => void;
  onNavAudit: () => void;
  onNavResources: () => void;
  onManageStorage: () => void;
};

function folderChildren(folders: WorkspaceSidebarFolder[], parentId: string) {
  return folders
    .filter((folder) => (folder.parentId ?? '') === parentId)
    .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name));
}

function folderTotalCount(folder: WorkspaceSidebarFolder, folders: WorkspaceSidebarFolder[], workflowCountByFolder: Record<string, number>): number {
  return folderChildren(folders, folder.id).reduce((total, child) => (
    total + folderTotalCount(child, folders, workflowCountByFolder)
  ), workflowCountByFolder[folder.id] ?? 0);
}

export function WorkspaceSidebar({
  workspaces,
  folders,
  selectedWorkspaceId,
  selectedFolderId,
  activeNav,
  workflowCountByFolder,
  recentCount,
  sharedCount,
  favoriteCount,
  archivedCount,
  memberCount,
  auditCount,
  storageUsedGb = 32.4,
  storageTotalGb = 100,
  isLoading = false,
  onWorkspaceChange,
  onFolderSelect,
  onCreateWorkspace,
  onNavRecent,
  onNavWorkspace,
  onNavShared,
  onNavFavorites,
  onNavArchived,
  onNavMembers,
  onNavAudit,
  onNavResources,
  onManageStorage,
}: WorkspaceSidebarProps) {
  const [workspaceMenuOpen, setWorkspaceMenuOpen] = useState(false);
  const [workspaceQuery, setWorkspaceQuery] = useState('');
  const selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceId) ?? workspaces[0];
  const workspaceFolders = selectedWorkspace ? folders.filter((folder) => folder.workspaceId === selectedWorkspace.id) : [];
  const rootFolders = folderChildren(workspaceFolders, '');
  const usedPercent = storageTotalGb > 0 ? Math.min(100, Math.round((storageUsedGb / storageTotalGb) * 100)) : 0;
  const filteredWorkspaces = workspaces.filter((workspace) => {
    const keyword = workspaceQuery.trim().toLowerCase();
    if (!keyword) return true;
    return `${workspace.name} ${workspace.slug}`.toLowerCase().includes(keyword);
  });

  useEffect(() => {
    if (!workspaceMenuOpen) return undefined;
    const close = (event: PointerEvent) => {
      if (event.target instanceof Element && event.target.closest('.exm-workspace-switcher')) return;
      setWorkspaceMenuOpen(false);
    };
    const closeWithKeyboard = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setWorkspaceMenuOpen(false);
    };
    document.addEventListener('pointerdown', close);
    document.addEventListener('keydown', closeWithKeyboard);
    return () => {
      document.removeEventListener('pointerdown', close);
      document.removeEventListener('keydown', closeWithKeyboard);
    };
  }, [workspaceMenuOpen]);

  useEffect(() => {
    if (!workspaceMenuOpen) setWorkspaceQuery('');
  }, [workspaceMenuOpen]);

  const renderFolder = (folder: WorkspaceSidebarFolder, level = 0) => {
    const children = folderChildren(workspaceFolders, folder.id);
    const count = folderTotalCount(folder, workspaceFolders, workflowCountByFolder);
    const active = folder.id === selectedFolderId;
    return (
      <div key={folder.id} className="exm-sidebar-folder-group">
        <TaNavButton
          active={active}
          style={{ paddingLeft: 10 + level * 14 }}
          onClick={() => onFolderSelect(folder.id)}
        >
          {active ? <FolderOpen size={15} /> : <Folder size={15} />}
          <span>{folder.name}</span>
          <strong>{count}</strong>
        </TaNavButton>
        {children.map((child) => renderFolder(child, level + 1))}
      </div>
    );
  };

  if (isLoading) {
    return <div className="exm-sidebar" aria-busy="true" />;
  }

  return (
    <div className="exm-sidebar">
      {isLoading ? (
        <div className="exm-workspace-switcher skeleton-state">
          <span className="exm-skeleton" />
          <i className="exm-skeleton" />
        </div>
      ) : (
        <div className="exm-workspace-switcher">
          <span>{(selectedWorkspace?.name || 'H').slice(0, 1).toUpperCase()}</span>
          <TaButton
            className="exm-workspace-switcher-trigger"
            tone="ghost"
            aria-label="切换工作区"
            aria-expanded={workspaceMenuOpen}
            onClick={() => setWorkspaceMenuOpen((value) => !value)}
          >
            <strong>{selectedWorkspace?.name ?? 'hello'}</strong>
            <ChevronDown size={15} />
          </TaButton>
          {workspaceMenuOpen ? (
            <TaMenu className="exm-workspace-switcher-menu" role="menu">
              <label className="exm-workspace-switcher-search">
                <TaInput
                  autoFocus
                  value={workspaceQuery}
                  placeholder="搜索 workspace"
                  onChange={(event) => setWorkspaceQuery(event.target.value)}
                  onKeyDown={(event) => event.stopPropagation()}
                />
              </label>
              {filteredWorkspaces.length ? filteredWorkspaces.map((workspace) => (
                <TaMenuItem
                  key={workspace.id}
                  className={workspace.id === selectedWorkspace?.id ? 'active' : ''}
                  role="menuitem"
                  onClick={() => {
                    onWorkspaceChange(workspace.id);
                    setWorkspaceMenuOpen(false);
                    setWorkspaceQuery('');
                  }}
                >
                  <i>{workspace.name.slice(0, 1).toUpperCase()}</i>
                  <span>{workspace.name}</span>
                </TaMenuItem>
              )) : workspaces.length ? (
                <p className="exm-workspace-switcher-empty">没有匹配的 workspace</p>
              ) : (
                <TaMenuItem className="active" role="menuitem" onClick={() => setWorkspaceMenuOpen(false)}>
                  <i>H</i>
                  <span>hello</span>
                </TaMenuItem>
              )}
            </TaMenu>
          ) : null}
        </div>
      )}

      <nav className="exm-sidebar-nav" aria-label="Workspace navigation">
        {isLoading ? Array.from({ length: 5 }).map((_, index) => (
          <TaNavButton key={index} className="skeleton-row" disabled aria-label="导航加载中"><i className="exm-skeleton" /><span className="exm-skeleton" /><strong className="exm-skeleton" /></TaNavButton>
        )) : (
          <>
            <TaNavButton active={activeNav === 'recent'} onClick={onNavRecent}><Clock3 size={16} /><span>最近编辑</span><strong>{recentCount}</strong></TaNavButton>
            <TaNavButton active={activeNav === 'workspace'} onClick={onNavWorkspace}><Folder size={16} /><span>工作区</span></TaNavButton>
            <TaNavButton active={activeNav === 'shared'} onClick={onNavShared}><Users size={16} /><span>与我共享</span><strong>{sharedCount}</strong></TaNavButton>
            <TaNavButton active={activeNav === 'favorites'} onClick={onNavFavorites}><Star size={16} /><span>收藏</span><strong>{favoriteCount}</strong></TaNavButton>
            <TaNavButton active={activeNav === 'archived'} onClick={onNavArchived}><Archive size={16} /><span>归档</span><strong>{archivedCount}</strong></TaNavButton>
          </>
        )}
      </nav>

      <div className="exm-sidebar-section">
        <div className="exm-sidebar-section-head">
          <span>工作区目录</span>
          <TaIconButton aria-label="新建 Workspace" onClick={onCreateWorkspace}><Plus size={15} /></TaIconButton>
        </div>
        <div className="exm-sidebar-folders">
          {isLoading ? Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="exm-sidebar-folder-skeleton">
              <i className="exm-skeleton" />
              <span className="exm-skeleton" />
              <b className="exm-skeleton" />
            </div>
          )) : rootFolders.length ? rootFolders.map((folder) => renderFolder(folder)) : (
            <div className="exm-sidebar-empty">暂无目录</div>
          )}
        </div>
      </div>

      <nav className="exm-sidebar-nav exm-sidebar-secondary" aria-label="Workspace operations">
        {isLoading ? Array.from({ length: 2 }).map((_, index) => (
          <TaNavButton key={index} className="skeleton-row" disabled aria-label="操作加载中"><i className="exm-skeleton" /><span className="exm-skeleton" /><strong className="exm-skeleton" /></TaNavButton>
        )) : (
          <>
            <TaNavButton onClick={onNavMembers}><Users size={16} /><span>成员</span><strong>{memberCount}</strong></TaNavButton>
            <TaNavButton onClick={onNavResources}><KeyRound size={16} /><span>资源</span></TaNavButton>
            <TaNavButton onClick={onNavAudit}><FileClock size={16} /><span>审计</span><strong>{auditCount}</strong></TaNavButton>
          </>
        )}
      </nav>

      <div className="exm-sidebar-storage">
        {isLoading ? (
          <div className="exm-storage-skeleton">
            <span className="exm-skeleton" />
            <p className="exm-skeleton" />
            <i className="exm-skeleton" />
            <b className="exm-skeleton" />
          </div>
        ) : (
          <>
            <div>
              <span>存储空间</span>
              <strong>{usedPercent}%</strong>
            </div>
            <p>{storageUsedGb.toFixed(1)} GB / {storageTotalGb.toFixed(0)} GB</p>
            <div className="exm-sidebar-storage-track"><i style={{ width: `${usedPercent}%` }} /></div>
            <TaButton onClick={onManageStorage}><Database size={15} />管理存储空间</TaButton>
          </>
        )}
      </div>
    </div>
  );
}
