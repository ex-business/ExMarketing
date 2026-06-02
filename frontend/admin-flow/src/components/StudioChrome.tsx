import { useMemo, useState } from 'react';

type ActivityOption = {
  id: string;
  tenantId?: string;
  activityCode: string;
  activityName: string;
  status?: string;
  version?: number;
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
  activityId: string;
  activityLabel: string;
  workflowStatus: string;
  workflowVersionLabel: string;
  zoomPercent: number;
  showChecklist: boolean;
  isPublished: boolean;
  onTogglePalette: () => void;
  onActivityChange: (activityId: string) => void;
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
  activityId,
  activityLabel,
  workflowStatus,
  workflowVersionLabel,
  zoomPercent,
  showChecklist,
  isPublished,
  onTogglePalette,
  onActivityChange,
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
  onPublish,
}: Props) {
  const [showZoomMenu, setShowZoomMenu] = useState(false);
  const [showActivityPicker, setShowActivityPicker] = useState(false);
  const [activitySearch, setActivitySearch] = useState('');
  const [showTestActivities, setShowTestActivities] = useState(false);
  const zoomOptions = [200, 100, 75, 50, 25];
  const chooseZoom = (value: number) => {
    onZoomTo(value / 100);
    setShowZoomMenu(false);
  };
  const activeActivity = activities.find((item) => item.id === activityId);
  const activityOptions = useMemo(() => {
    const keyword = activitySearch.trim().toLowerCase();
    return activities
      .filter((activity) => showTestActivities || !isTestActivity(activity))
      .filter((activity) => {
        if (!keyword) return true;
        return `${activity.activityName} ${activity.activityCode} ${activity.tenantId}`.toLowerCase().includes(keyword);
      })
      .slice(0, 24);
  }, [activities, activitySearch, showTestActivities]);
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
                <b>切换活动</b>
                <a href="/admin/">创建活动</a>
              </div>
              <input value={activitySearch} placeholder="搜索活动名称、code 或 tenant" onChange={(event) => setActivitySearch(event.target.value)} />
              <div className="activity-picker-list">
                {activityOptions.length ? activityOptions.map((activity) => (
                  <button
                    key={activity.id}
                    className={activity.id === activityId ? 'active' : ''}
                    onClick={() => {
                      onActivityChange(activity.id);
                      setShowActivityPicker(false);
                    }}
                  >
                    <span>
                      <b>{activity.activityName || activity.activityCode}</b>
                      <em>{activity.activityCode}</em>
                    </span>
                    <strong>{activity.status || 'UNKNOWN'} · v{activity.version ?? '-'}</strong>
                  </button>
                )) : <div className="activity-picker-empty">没有匹配活动。</div>}
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
      <div className="dify-top-status">{workflowStatus} · {workflowVersionLabel} · {isPublished ? '已发布' : '未发布'}</div>
      <div className="dify-float-toolbar">
        <button onClick={onDefault}>完整链路</button>
        <button onClick={onSimple}>简化链路</button>
        <button onClick={onAutoLayout}>自动排齐</button>
        <button onClick={onSaveDraft}>保存草稿</button>
        <button onClick={onRunHistory}>运行历史</button>
        <button className="checklist-action" onClick={onToggleChecklist}>检查清单 <span>5</span></button>
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
            <span>{isPublished ? 'PASS' : '5 项'}</span>
          </div>
          {['入口节点已配置', '关键规则存在分支', '奖励与 Quota 已串联', '失败重试已开启', '审计节点已连接'].map((item, index) => (
            <label key={item} className="checklist-row">
              <input type="checkbox" checked={index < 3 || isPublished} readOnly />
              <span>{item}</span>
            </label>
          ))}
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
