import type { ActivityEdge, ActivityNode, FlowNode, GroupNode, NoteNode } from '../types';
import { cx, iconFor } from '../utils/flow';

function PropertyList({ title, items }: { title: string; items: string[] }) {
  return (
    <section className="property-section">
      <h3>{title}</h3>
      <div className="property-tags">
        {items.length > 0 ? items.map((item) => <span key={item}>{item}</span>) : <em>未配置</em>}
      </div>
    </section>
  );
}

function configRowsFor(node: ActivityNode) {
  const common = [
    { label: '执行超时', value: '30s' },
    { label: 'Trace Key', value: '${eventId}' },
  ];
  if (node.data.kind === 'logic') {
    return [
      { label: '规则表达式', value: node.data.id === 'qualify' ? 'eligibility == true' : 'stage_result matched' },
      { label: '缺失字段策略', value: '跳过并记录原因' },
      ...common,
    ];
  }
  if (node.data.kind === 'reward') {
    return [
      { label: '奖励类型', value: 'FIXED / RATIO / LADDER' },
      { label: '精度模式', value: 'Decimal(18, 8)' },
      ...common,
    ];
  }
  if (node.data.kind === 'quota') {
    return [
      { label: '扣减策略', value: '冻结 -> 确认 -> 释放' },
      { label: '并发保护', value: 'Redis Lock + DB Version' },
      ...common,
    ];
  }
  if (node.data.kind === 'dispatch' || node.data.kind === 'callback') {
    return [
      { label: '目标通道', value: node.data.outputs.join(' / ') || 'HTTP' },
      { label: '失败策略', value: 'Outbox + Retry' },
      ...common,
    ];
  }
  return [
    { label: '输入映射', value: node.data.inputs[0] ?? 'payload' },
    { label: '输出字段', value: node.data.outputs[0] ?? 'result' },
    ...common,
  ];
}

type Props = {
  nodes: FlowNode[];
  edges: ActivityEdge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (ids: string[]) => void;
};

export function PropertyPanel({ nodes, edges, selectedNodeIds, selectedEdgeIds, onDeleteNodes, onDeleteEdges }: Props) {
  const selectedNode = nodes.find((node): node is ActivityNode => node.id === selectedNodeIds[0] && node.type === 'activity');
  const selectedGroup = nodes.find((node): node is GroupNode => node.id === selectedNodeIds[0] && node.type === 'group');
  const selectedNote = nodes.find((node): node is NoteNode => node.id === selectedNodeIds[0] && node.type === 'note');
  const selectedEdge = edges.find((item) => item.id === selectedEdgeIds[0]);
  const hasSelection = Boolean(selectedNode || selectedGroup || selectedNote || selectedEdge);

  return (
    <aside className={cx('property-panel', hasSelection && 'open')}>
      <div className="property-head">
        <div className="property-title-with-icon">
          {selectedNode ? <span className={cx('node-icon', selectedNode.data.kind)}>{iconFor(selectedNode.data.kind)}</span> : null}
          <b>{selectedNode ? selectedNode.data.title : selectedGroup ? selectedGroup.data.title : selectedNote ? '备注' : selectedEdge ? '流程连线' : '属性'}</b>
        </div>
        <span>{selectedNode ? '节点' : selectedGroup ? '组合' : selectedNote ? '备注' : selectedEdge ? '连线' : '未选择'}</span>
      </div>
      {selectedNode ? (
        <div className="property-tabs">
          <button className="active">设置</button>
          <button>上次运行</button>
        </div>
      ) : null}
      {selectedNode ? (
        <div className="property-body">
          <section className="property-card">
            <div className="property-title-row">
              <span className={cx('palette-dot', selectedNode.data.kind)} />
              <div>
                <h2>{selectedNode.data.title}</h2>
                <p>{selectedNode.data.code}</p>
              </div>
            </div>
            <dl>
              <div><dt>节点 ID</dt><dd>{selectedNode.id}</dd></div>
              <div><dt>节点类型</dt><dd>{selectedNode.data.kind}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedNode.position.x)} / y {Math.round(selectedNode.position.y)}</dd></div>
            </dl>
          </section>
          <section className="property-section">
            <h3>说明</h3>
            <p>{selectedNode.data.desc}</p>
          </section>
          <PropertyList title="输入变量" items={selectedNode.data.inputs} />
          <PropertyList title="输出变量" items={selectedNode.data.outputs} />
          <section className="property-section">
            <h3>下一步</h3>
            <div className="next-step-card">
              <span className={cx('palette-dot', selectedNode.data.kind)} />
              <div>
                <b>{selectedNode.data.outputs[0] ?? '默认路径'}</b>
                <p>连接下游活动节点，决定下一步执行链路。</p>
              </div>
            </div>
          </section>
          <section className="property-section">
            <h3>配置参数</h3>
            <div className="config-grid">
              {configRowsFor(selectedNode).map((row) => (
                <label key={row.label} className="config-row">
                  <span>{row.label}</span>
                  <input value={row.value} readOnly />
                </label>
              ))}
            </div>
          </section>
          <section className="property-section">
            <h3>失败时重试</h3>
            <label className="switch-row">
              <span>启用重试</span>
              <input type="checkbox" checked={selectedNode.data.kind === 'reliable' || selectedNode.data.kind === 'dispatch'} readOnly />
            </label>
            <div className="retry-row">
              <span>最大次数</span>
              <b>{selectedNode.data.kind === 'reliable' ? 5 : 3}</b>
              <span>退避</span>
              <b>指数退避</b>
            </div>
          </section>
          <section className="property-section">
            <h3>异常处理</h3>
            <button className="exception-select">无</button>
          </section>
          <section className="property-section">
            <h3>上次运行</h3>
            <div className="run-summary">
              <b>SUCCESS</b>
              <span>耗时 186ms</span>
              <span>输出 {selectedNode.data.outputs[0] ?? 'result'}</span>
            </div>
          </section>
        </div>
      ) : null}
      {!selectedNode && selectedGroup ? (
        <div className="property-body">
          <section className="property-card">
            <h2>{selectedGroup.data.title}</h2>
            <dl>
              <div><dt>组合 ID</dt><dd>{selectedGroup.id}</dd></div>
              <div><dt>尺寸</dt><dd>{Math.round(selectedGroup.data.width)} x {Math.round(selectedGroup.data.height)}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedGroup.position.x)} / y {Math.round(selectedGroup.position.y)}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteNodes([selectedGroup.id])}>删除组合</button>
          </section>
        </div>
      ) : null}
      {!selectedNode && !selectedGroup && selectedNote ? (
        <div className="property-body">
          <section className="property-card">
            <h2>备注</h2>
            <dl>
              <div><dt>备注 ID</dt><dd>{selectedNote.id}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedNote.position.x)} / y {Math.round(selectedNote.position.y)}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteNodes([selectedNote.id])}>删除备注</button>
          </section>
          <section className="property-section">
            <h3>内容</h3>
            <p>{selectedNote.data.text}</p>
          </section>
        </div>
      ) : null}
      {!selectedNode && !selectedGroup && !selectedNote && selectedEdge ? (
        <div className="property-body">
          <section className="property-card">
            <h2>流程连线</h2>
            <dl>
              <div><dt>连线 ID</dt><dd>{selectedEdge.id}</dd></div>
              <div><dt>来源节点</dt><dd>{selectedEdge.source}</dd></div>
              <div><dt>目标节点</dt><dd>{selectedEdge.target}</dd></div>
              <div><dt>标签</dt><dd>{selectedEdge.label || '默认路径'}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteEdges([selectedEdge.id])}>删除连线</button>
          </section>
        </div>
      ) : null}
      {!hasSelection ? (
        <div className="property-empty-state">
          <b>选择一个节点</b>
          <p>点击画布中的活动节点后，可以在这里查看输入、输出、说明和后续配置项。</p>
        </div>
      ) : null}
    </aside>
  );
}
