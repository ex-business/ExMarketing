import { useEffect, useMemo, useState } from 'react';
import type { ActivitySummary, WorkspaceFolderSummary, WorkspaceSummary } from '../types';

export type ActivitySettingsInput = {
  activityName: string;
  workspaceId: string;
  folderId: string;
};

export function ActivitySettingsModal({
  activity,
  workspaces,
  folders,
  placement,
  saving,
  error,
  onClose,
  onSubmit,
}: {
  activity: ActivitySummary;
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  placement?: { workspaceId: string; folderId: string };
  saving: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (input: ActivitySettingsInput) => void;
}) {
  const [activityName, setActivityName] = useState(activity.activityName);
  const [workspaceId, setWorkspaceId] = useState(activity.workspaceId || placement?.workspaceId || workspaces[0]?.id || '');
  const availableFolders = useMemo(() => folders.filter((folder) => folder.workspaceId === workspaceId && folder.status !== 'DELETED'), [folders, workspaceId]);
  const [folderId, setFolderId] = useState(activity.folderId || placement?.folderId || availableFolders[0]?.id || '');

  useEffect(() => {
    const nextWorkspaceId = activity.workspaceId || placement?.workspaceId || workspaces[0]?.id || '';
    const nextFolders = folders.filter((folder) => folder.workspaceId === nextWorkspaceId && folder.status !== 'DELETED');
    setActivityName(activity.activityName);
    setWorkspaceId(nextWorkspaceId);
    setFolderId(activity.folderId || placement?.folderId || nextFolders[0]?.id || '');
  }, [activity.activityName, activity.folderId, activity.workspaceId, folders, placement?.folderId, placement?.workspaceId, workspaces]);

  useEffect(() => {
    if (!folderId || availableFolders.some((folder) => folder.id === folderId)) return;
    setFolderId(availableFolders[0]?.id || '');
  }, [availableFolders, folderId]);

  return (
    <div className="publish-confirm-backdrop" role="presentation">
      <section className="activity-settings-modal" role="dialog" aria-modal="true" aria-labelledby="activity-settings-title">
        <header>
          <span>
            <b id="activity-settings-title">活动基础信息</b>
            <em>用于 Studio 工作区、文件夹和画布上下文。</em>
          </span>
          <button type="button" onClick={onClose}>x</button>
        </header>
        <div className="activity-settings-grid">
          <label>
            <span>名称</span>
            <input value={activityName} onChange={(event) => setActivityName(event.target.value)} />
          </label>
          <label>
            <span>Code</span>
            <input value={activity.activityCode} disabled />
          </label>
          <label>
            <span>Tenant</span>
            <input value={activity.tenantId} disabled />
          </label>
          <label>
            <span>Workspace</span>
            <select value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
              {workspaces.filter((workspace) => workspace.status !== 'DELETED').map((workspace) => (
                <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
              ))}
            </select>
          </label>
          <label className="wide">
            <span>Folder</span>
            <select value={folderId} onChange={(event) => setFolderId(event.target.value)}>
              {availableFolders.map((folder) => (
                <option key={folder.id} value={folder.id}>{folder.name} · {folder.visibility}</option>
              ))}
            </select>
          </label>
        </div>
        {error ? <p>{error}</p> : <p>Code 和 tenant 属于活动身份字段，当前只展示不修改。</p>}
        <footer>
          <button type="button" onClick={onClose}>取消</button>
          <button
            className="publish-confirm-primary"
            type="button"
            disabled={saving || !activityName.trim() || !workspaceId || !folderId}
            onClick={() => onSubmit({ activityName, workspaceId, folderId })}
          >
            {saving ? '保存中...' : '保存信息'}
          </button>
        </footer>
      </section>
    </div>
  );
}
