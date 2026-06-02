type Props = {
  showPalette: boolean;
  interactionMode: 'pointer' | 'hand';
  nodeCount: number;
  edgeCount: number;
  selectedCount: number;
  selectedNodeCount: number;
  lastAdded: string;
  zoomPercent: number;
  showChecklist: boolean;
  isPublished: boolean;
  onTogglePalette: () => void;
  onModeChange: (mode: 'pointer' | 'hand') => void;
  onDefault: () => void;
  onSimple: () => void;
  onAutoLayout: () => void;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFitView: () => void;
  onGroup: () => void;
  onDelete: () => void;
  onClear: () => void;
  onPreview: () => void;
  onRunHistory: () => void;
  onToggleChecklist: () => void;
  onPublish: () => void;
};

export function StudioChrome({
  showPalette,
  interactionMode,
  nodeCount,
  edgeCount,
  selectedCount,
  selectedNodeCount,
  lastAdded,
  zoomPercent,
  showChecklist,
  isPublished,
  onTogglePalette,
  onModeChange,
  onDefault,
  onSimple,
  onAutoLayout,
  onZoomIn,
  onZoomOut,
  onFitView,
  onGroup,
  onDelete,
  onClear,
  onPreview,
  onRunHistory,
  onToggleChecklist,
  onPublish,
}: Props) {
  return (
    <>
      <div className="workspace-topbar">
        <div className="workspace-brand">
          <strong>ExMarketing</strong>
          <span>/</span>
          <button>ex-unknownname</button>
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
      <div className="dify-top-status">自动保存 12:58:10 · {isPublished ? '已发布' : '未发布'}</div>
      <div className="dify-float-toolbar">
        <button onClick={onDefault}>完整链路</button>
        <button onClick={onSimple}>简化链路</button>
        <button onClick={onAutoLayout}>自动排齐</button>
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
        <b>{zoomPercent}%</b>
        <button onClick={onZoomIn}>+</button>
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
  );
}
