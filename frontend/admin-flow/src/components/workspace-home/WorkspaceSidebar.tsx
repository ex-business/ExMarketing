import {
  Archive,
  ChevronDown,
  Clock3,
  Database,
  FileClock,
  Folder,
  FolderOpen,
  Plus,
  Star,
  Users,
} from 'lucide-react';

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
  onManageStorage,
}: WorkspaceSidebarProps) {
  const selectedWorkspace = workspaces.find((workspace) => workspace.id === selectedWorkspaceId) ?? workspaces[0];
  const workspaceFolders = selectedWorkspace ? folders.filter((folder) => folder.workspaceId === selectedWorkspace.id) : [];
  const rootFolders = folderChildren(workspaceFolders, '');
  const usedPercent = storageTotalGb > 0 ? Math.min(100, Math.round((storageUsedGb / storageTotalGb) * 100)) : 0;

  const renderFolder = (folder: WorkspaceSidebarFolder, level = 0) => {
    const children = folderChildren(workspaceFolders, folder.id);
    const count = folderTotalCount(folder, workspaceFolders, workflowCountByFolder);
    const active = folder.id === selectedFolderId;
    return (
      <div key={folder.id} className="exm-sidebar-folder-group">
        <button
          className={active ? 'active' : ''}
          type="button"
          style={{ paddingLeft: 10 + level * 14 }}
          onClick={() => onFolderSelect(folder.id)}
        >
          {active ? <FolderOpen size={15} /> : <Folder size={15} />}
          <span>{folder.name}</span>
          <strong>{count}</strong>
        </button>
        {children.map((child) => renderFolder(child, level + 1))}
      </div>
    );
  };

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
          <select
            value={selectedWorkspace?.id ?? ''}
            onChange={(event) => onWorkspaceChange(event.target.value)}
            aria-label="切换工作区"
          >
            {workspaces.length ? workspaces.map((workspace) => (
              <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
            )) : <option value="">hello</option>}
          </select>
          <ChevronDown size={15} />
        </div>
      )}

      <nav className="exm-sidebar-nav" aria-label="Workspace navigation">
        {isLoading ? Array.from({ length: 5 }).map((_, index) => (
          <button key={index} className="skeleton-row" type="button" disabled aria-label="导航加载中"><i className="exm-skeleton" /><span className="exm-skeleton" /><strong className="exm-skeleton" /></button>
        )) : (
          <>
            <button type="button" onClick={onNavRecent}><Clock3 size={16} /><span>最近编辑</span><strong>{recentCount}</strong></button>
            <button className="active" type="button" onClick={onNavWorkspace}><Folder size={16} /><span>工作区</span></button>
            <button type="button" onClick={onNavShared}><Users size={16} /><span>与我共享</span><strong>{sharedCount}</strong></button>
            <button type="button" onClick={onNavFavorites}><Star size={16} /><span>收藏</span><strong>{favoriteCount}</strong></button>
            <button type="button" onClick={onNavArchived}><Archive size={16} /><span>归档</span><strong>{archivedCount}</strong></button>
          </>
        )}
      </nav>

      <div className="exm-sidebar-section">
        <div className="exm-sidebar-section-head">
          <span>工作区目录</span>
          <button type="button" aria-label="新建 Workspace" onClick={onCreateWorkspace}><Plus size={15} /></button>
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
          <button key={index} className="skeleton-row" type="button" disabled aria-label="操作加载中"><i className="exm-skeleton" /><span className="exm-skeleton" /><strong className="exm-skeleton" /></button>
        )) : (
          <>
            <button type="button" onClick={onNavMembers}><Users size={16} /><span>成员</span><strong>{memberCount}</strong></button>
            <button type="button" onClick={onNavAudit}><FileClock size={16} /><span>审计</span><strong>{auditCount}</strong></button>
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
            <button type="button" onClick={onManageStorage}><Database size={15} />管理存储空间</button>
          </>
        )}
      </div>
    </div>
  );
}
