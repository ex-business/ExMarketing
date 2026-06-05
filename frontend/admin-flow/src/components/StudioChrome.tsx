import { useMemo, useState } from 'react';
import type { WorkflowValidationIssue, WorkflowValidationResult } from '../types';

type ActivityOption = {
  id: string;
  workspaceId?: string;
  folderId?: string;
  tenantId?: string;
  activityCode: string;
  activityName: string;
  status?: string;
  version?: number;
};

type WorkspaceOption = {
  id: string;
  name: string;
  slug: string;
  color?: string;
  visibility: string;
  status: string;
};

type FolderOption = {
  id: string;
  workspaceId: string;
  name: string;
  color?: string;
  visibility: string;
  sortOrder: number;
  status: string;
};

type ActivityPlacement = {
  workspaceId: string;
  folderId: string;
};

type Props = {
  homeMode: boolean;
  showPalette: boolean;
  interactionMode: 'pointer' | 'hand';
  nodeCount: number;
  edgeCount: number;
  selectedCount: number;
  selectedNodeCount: number;
  lastAdded: string;
  activities: ActivityOption[];
  workspaces: WorkspaceOption[];
  folders: FolderOption[];
  placements: Record<string, ActivityPlacement>;
  activityId: string;
  activityLabel: string;
  workflowStatus: string;
  workflowVersionLabel: string;
  zoomPercent: number;
  showChecklist: boolean;
  validatingWorkflow: boolean;
  workflowValidation: WorkflowValidationResult | null;
  isPublished: boolean;
  onTogglePalette: () => void;
  onActivityChange: (activityId: string) => void;
  onWorkspaceNavigate: (workspaceId: string, folderId?: string) => void;
  onModeChange: (mode: 'pointer' | 'hand') => void;
  onDefault: () => void;
  onSimple: () => void;
  onAutoLayout: () => void;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onZoomTo: (zoom: number) => void;
  onFitView: () => void;
  showMiniMap: boolean;
  onToggleMiniMap: () => void;
  onGroup: () => void;
  onDelete: () => void;
  onClear: () => void;
  onSaveDraft: () => void;
  onPreview: () => void;
  onRunHistory: () => void;
  onToggleChecklist: () => void;
  onValidationIssueFocus: (item: WorkflowValidationIssue) => void;
  onPublish: () => void;
};

export function StudioChrome({
  homeMode,
  showPalette,
  interactionMode,
  nodeCount,
  edgeCount,
  selectedCount,
  selectedNodeCount,
  lastAdded,
  activities,
  workspaces,
  folders,
  placements,
  activityId,
  activityLabel,
  workflowStatus,
  workflowVersionLabel,
  zoomPercent,
  showChecklist,
  validatingWorkflow,
  workflowValidation,
  isPublished,
  onTogglePalette,
  onActivityChange,
  onWorkspaceNavigate,
  onModeChange,
  onDefault,
  onSimple,
  onAutoLayout,
  onZoomIn,
  onZoomOut,
  onZoomTo,
  onFitView,
  showMiniMap,
  onToggleMiniMap,
  onGroup,
  onDelete,
  onClear,
  onSaveDraft,
  onPreview,
  onRunHistory,
  onToggleChecklist,
  onValidationIssueFocus,
  onPublish,
}: Props) {
  const [showZoomMenu, setShowZoomMenu] = useState(false);
  const [showActivityPicker, setShowActivityPicker] = useState(false);
  const [activitySearch, setActivitySearch] = useState('');
  const [showTestActivities, setShowTestActivities] = useState(false);
  const [pickerWorkspaceId, setPickerWorkspaceId] = useState('');
  const [pickerFolderId, setPickerFolderId] = useState('');
  const zoomOptions = [200, 100, 75, 50, 25];
  const chooseZoom = (value: number) => {
    onZoomTo(value / 100);
    setShowZoomMenu(false);
  };
  const activeActivity = activities.find((item) => item.id === activityId);
  const activePlacement = activityId ? placements[activityId] : undefined;
  const activeWorkspace = activePlacement ? workspaces.find((workspace) => workspace.id === activePlacement.workspaceId) : undefined;
  const activeFolder = activePlacement ? folders.find((folder) => folder.id === activePlacement.folderId) : undefined;
  const validationBlockCount = workflowValidation?.summary.blockCount ?? 0;
  const validationWarningCount = workflowValidation?.summary.warningCount ?? 0;
  const checklistBadge = validatingWorkflow
    ? '...'
    : workflowValidation
      ? workflowValidation.valid ? 'PASS' : String(validationBlockCount)
      : '-';
  const checklistStatus = validatingWorkflow
    ? '检查中'
    : workflowValidation
      ? workflowValidation.valid ? 'PASS' : `${validationBlockCount} 阻塞 / ${validationWarningCount} 警告`
      : '未检查';
  const activeWorkspaces = useMemo(() => workspaces.filter((workspace) => workspace.status !== 'DELETED'), [workspaces]);
  const folderOptions = useMemo(() => (
    folders
      .filter((folder) => folder.status !== 'DELETED')
      .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name))
  ), [folders]);
  const workspaceWorkflowCount = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      if (!showTestActivities && isTestActivity(activity)) return;
      const placement = placements[activity.id];
      if (!placement?.workspaceId) return;
      next[placement.workspaceId] = (next[placement.workspaceId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements, showTestActivities]);
  const folderWorkflowCount = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      if (!showTestActivities && isTestActivity(activity)) return;
      const placement = placements[activity.id];
      if (!placement?.folderId) return;
      next[placement.folderId] = (next[placement.folderId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements, showTestActivities]);
  const selectedPickerWorkspaceId = pickerWorkspaceId || activePlacement?.workspaceId || activeWorkspaces[0]?.id || '';
  const selectedPickerFolders = folderOptions.filter((folder) => folder.workspaceId === selectedPickerWorkspaceId);
  const selectedPickerFolderId = pickerFolderId || activePlacement?.folderId || selectedPickerFolders[0]?.id || '';
  const selectedPickerWorkspace = activeWorkspaces.find((workspace) => workspace.id === selectedPickerWorkspaceId);
  const selectedPickerFolder = selectedPickerFolders.find((folder) => folder.id === selectedPickerFolderId);
  const activityOptions = useMemo(() => {
    const keyword = activitySearch.trim().toLowerCase();
    return activities
      .filter((activity) => showTestActivities || !isTestActivity(activity))
      .filter((activity) => {
        const placement = placements[activity.id];
        if (selectedPickerWorkspaceId && placement?.workspaceId !== selectedPickerWorkspaceId) return false;
        if (selectedPickerFolderId && placement?.folderId !== selectedPickerFolderId) return false;
        if (!keyword) return true;
        const workspace = activeWorkspaces.find((item) => item.id === placement?.workspaceId);
        const folder = folderOptions.find((item) => item.id === placement?.folderId);
        return `${activity.activityName} ${activity.activityCode} ${activity.tenantId} ${workspace?.name ?? ''} ${folder?.name ?? ''}`.toLowerCase().includes(keyword);
      })
      .slice(0, 24);
  }, [activities, activitySearch, activeWorkspaces, folderOptions, placements, selectedPickerFolderId, selectedPickerWorkspaceId, showTestActivities]);
  const hiddenTestCount = activities.filter(isTestActivity).length;

  return (
    <>
      <div className="workspace-topbar">
        <div className="workspace-brand">
          <strong>ExMarketing</strong>
          <span>/</span>
          {homeMode ? (
            <div className="workspace-section-title">Activity Studio</div>
          ) : (
            <>
              <button
                className="activity-current"
                title={activityLabel}
                onClick={() => setShowActivityPicker((value) => !value)}
              >
                <b>{activeActivity ? activeActivity.activityName || activeActivity.activityCode : '选择活动'}</b>
                <small>{activeActivity ? activeActivity.activityCode : '未绑定'}</small>
              </button>
              {activeActivity ? <em className="activity-status">{activeActivity.status || 'UNKNOWN'} · v{activeActivity.version ?? '-'}</em> : null}
            </>
          )}
          {!homeMode && showActivityPicker ? (
            <div className="activity-picker-popover">
              <div className="activity-picker-head">
                <b>切换 Workflow</b>
                <a href="/admin/">创建活动</a>
              </div>
              <input value={activitySearch} placeholder="搜索 workspace、folder 或 workflow" onChange={(event) => setActivitySearch(event.target.value)} />
              <div className="activity-picker-workspace-view">
                <aside className="activity-picker-tree">
                  {activeWorkspaces.length ? activeWorkspaces.map((workspace) => {
                    const workspaceFolders = folderOptions.filter((folder) => folder.workspaceId === workspace.id);
                    return (
                      <div key={workspace.id} className="activity-picker-tree-group">
                        <button
                          className={workspace.id === selectedPickerWorkspaceId ? 'active workspace' : 'workspace'}
                          onClick={() => {
                            setPickerWorkspaceId(workspace.id);
                            setPickerFolderId(workspaceFolders[0]?.id ?? '');
                          }}
                        >
                          <i>▦</i>
                          <span>
                            <b>{workspace.name}</b>
                            <em>{workspace.slug}</em>
                          </span>
                          <strong>{workspaceWorkflowCount[workspace.id] ?? 0}</strong>
                        </button>
                        {workspace.id === selectedPickerWorkspaceId ? (
                          <div className="activity-picker-folder-list">
                            {workspaceFolders.length ? workspaceFolders.map((folder) => (
                              <button
                                key={folder.id}
                                className={folder.id === selectedPickerFolderId ? 'active folder' : 'folder'}
                                onClick={() => setPickerFolderId(folder.id)}
                              >
                                <i>▸</i>
                                <span>
                                  <b>{folder.name}</b>
                                  <em>{folder.visibility}</em>
                                </span>
                                <strong>{folderWorkflowCount[folder.id] ?? 0}</strong>
                              </button>
                            )) : <div className="activity-picker-empty compact">暂无 folder</div>}
                          </div>
                        ) : null}
                      </div>
                    );
                  }) : <div className="activity-picker-empty">暂无 workspace</div>}
                </aside>
                <section className="activity-picker-workflow-pane">
                  <header>
                    <span>
                      <b>{selectedPickerFolder?.name ?? selectedPickerWorkspace?.name ?? 'Workflow'}</b>
                      <em>{selectedPickerWorkspace?.name ?? '选择 workspace'}</em>
                    </span>
                    <strong>{activityOptions.length}</strong>
                  </header>
                  <div className="activity-picker-workflow-list">
                    {activityOptions.length ? activityOptions.map((activity) => (
                    <button
                      key={activity.id}
                      className={activity.id === activityId ? 'active' : ''}
                      onClick={() => {
                        onActivityChange(activity.id);
                        setShowActivityPicker(false);
                      }}
                    >
                      <i>◇</i>
                      <span>
                        <b>{activity.activityName || activity.activityCode}</b>
                        <em>{activity.activityCode}</em>
                      </span>
                      <strong>{activity.status || 'UNKNOWN'} · v{activity.version ?? '-'}</strong>
                    </button>
                    )) : <div className="activity-picker-empty">没有匹配 workflow。</div>}
                  </div>
                </section>
              </div>
              {hiddenTestCount > 0 ? (
                <label className="activity-picker-toggle">
                  <input type="checkbox" checked={showTestActivities} onChange={(event) => setShowTestActivities(event.target.checked)} />
                  <span>显示 {hiddenTestCount} 个测试活动</span>
                </label>
              ) : null}
            </div>
          ) : null}
        </div>
        <nav className="app-tabs">
          <button className="active">编排</button>
          <button>访问 API</button>
          <button>日志与标注</button>
          <button>监测</button>
        </nav>
        <div className="workspace-actions">
          <button>插件</button>
          <span>D</span>
        </div>
      </div>
      {homeMode ? null : (
        <>
      <div className="dify-top-status">
        <span>{workflowStatus}</span>
        {activeWorkspace ? (
          <>
            <i>/</i>
            <button onClick={() => onWorkspaceNavigate(activeWorkspace.id)}>{activeWorkspace.name}</button>
          </>
        ) : null}
        {activeWorkspace && activeFolder ? (
          <>
            <i>/</i>
            <button onClick={() => onWorkspaceNavigate(activeWorkspace.id, activeFolder.id)}>{activeFolder.name}</button>
          </>
        ) : null}
        <i>·</i>
        <span>{workflowVersionLabel}</span>
        <i>·</i>
        <span>{isPublished ? '已发布' : '未发布'}</span>
      </div>
      <div className="dify-float-toolbar">
        <button onClick={onDefault}>完整链路</button>
        <button onClick={onSimple}>简化链路</button>
        <button onClick={onAutoLayout}>自动排齐</button>
        <button onClick={onSaveDraft}>保存草稿</button>
        <button onClick={onRunHistory}>运行历史</button>
        <button className="checklist-action" onClick={onToggleChecklist}>检查清单 <span>{checklistBadge}</span></button>
        <button className="preview-action" onClick={onPreview}>▷ 预览</button>
        <button className="publish-action" onClick={onPublish}>发布</button>
      </div>
      <button
        className={`node-library-fab ${showPalette ? 'open' : ''}`}
        title={showPalette ? '收起节点库' : '展开节点库'}
        onClick={onTogglePalette}
      >
        <span>{showPalette ? '×' : '+'}</span>
        <b>{showPalette ? '收起' : '节点'}</b>
      </button>
      <div className="dify-left-rail">
        <button className={interactionMode === 'pointer' ? 'active' : ''} title="指针模式" onClick={() => onModeChange('pointer')}>⌖</button>
        <button className={interactionMode === 'hand' ? 'active' : ''} title="手模式" onClick={() => onModeChange('hand')}>☝</button>
        <button title="整理节点" disabled={selectedNodeCount < 2} onClick={onGroup}>▦</button>
        <button title="最大化画布" onClick={onFitView}>□</button>
        <button title="删除选中" disabled={selectedCount === 0} onClick={onDelete}>⌫</button>
      </div>
      <div className="zoom-toolbar">
        <button onClick={onZoomOut}>-</button>
        <button className="zoom-percent-button" onClick={() => setShowZoomMenu((value) => !value)}>{zoomPercent}%</button>
        <button onClick={onZoomIn}>+</button>
        {showZoomMenu ? (
          <div className="zoom-menu">
            {zoomOptions.map((value) => (
              <button key={value} className={zoomPercent === value ? 'active' : ''} onClick={() => chooseZoom(value)}>
                <span>{value}%</span>
                {value === 100 ? <kbd>⇧ 1</kbd> : value === 50 ? <kbd>⇧ 5</kbd> : null}
              </button>
            ))}
            <button onClick={() => { onFitView(); setShowZoomMenu(false); }}>
              <span>Zoom to Fit</span>
              <kbd>⌘ 1</kbd>
            </button>
            <button onClick={onToggleMiniMap}>
              <span>{showMiniMap ? '✓' : ''} Minimap</span>
            </button>
          </div>
        ) : null}
      </div>
      {showChecklist ? (
        <div className="checklist-popover">
          <div className="checklist-head">
            <b>发布检查清单</b>
            <span>{checklistStatus}</span>
          </div>
          {validatingWorkflow ? (
            <div className="checklist-empty">正在保存当前画布并执行校验...</div>
          ) : workflowValidation ? (
            workflowValidation.items.length > 0 ? workflowValidation.items.map((item) => (
              <ValidationIssueRow
                key={`${item.code}-${item.nodeId ?? ''}-${item.edgeId ?? ''}-${item.path ?? ''}`}
                item={item}
                onFocus={onValidationIssueFocus}
              />
            )) : (
              <div className="checklist-pass">
                <b>全部检查通过</b>
                <span>当前 draft 可以发布。</span>
              </div>
            )
          ) : (
            <div className="checklist-empty">点击检查清单后，会展示当前 draft 的真实校验结果。</div>
          )}
        </div>
      ) : null}
      <header className="studio-head">
        <div>
          <h1>活动数据流设计器</h1>
          <p>事件进入、规则判断、奖励计算、额度扣减、发奖、回调、审计和重试。</p>
        </div>
        <div className="head-actions">
          {lastAdded ? <span className="status-pill green">已添加：{lastAdded}</span> : null}
          <span className="status-pill">节点 {nodeCount}</span>
          <span className="status-pill">连线 {edgeCount}</span>
          <button className="ghost-action" disabled={selectedNodeCount < 2} onClick={onGroup}>组合选中</button>
          <button className="ghost-action" onClick={onAutoLayout}>自动排齐</button>
          <button className="ghost-action" onClick={onClear}>清空画布</button>
          <button className="danger-action" disabled={selectedCount === 0} onClick={onDelete}>删除选中 {selectedCount > 0 ? selectedCount : ''}</button>
        </div>
      </header>
        </>
      )}
    </>
  );
}

function isTestActivity(activity: ActivityOption): boolean {
  const text = `${activity.activityName} ${activity.activityCode} ${activity.id}`.toLowerCase();
  return ['postgresql e2e', 'load fault', 'smoke', 'e2e_', 'load_fault', 'pg_smoke'].some((marker) => text.includes(marker));
}

function ValidationIssueRow({ item, onFocus }: { item: WorkflowValidationIssue; onFocus: (item: WorkflowValidationIssue) => void }) {
  const target = [item.nodeId ? `node ${item.nodeId}` : '', item.edgeId ? `edge ${item.edgeId}` : '', item.path ?? ''].filter(Boolean).join(' · ');
  const canFocus = Boolean(item.nodeId || item.edgeId);
  return (
    <button
      type="button"
      className={`checklist-row validation-${item.severity.toLowerCase()}`}
      disabled={!canFocus}
      onClick={() => onFocus(item)}
    >
      <i>{item.severity === 'BLOCK' ? '!' : '?'}</i>
      <span>
        <b>{item.code}</b>
        <em>{item.message}</em>
        {target ? <small>{target}</small> : null}
      </span>
    </button>
  );
}
