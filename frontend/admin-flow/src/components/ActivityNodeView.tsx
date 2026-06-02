import { Handle, Position } from '@xyflow/react';
import type { NodeProps } from '@xyflow/react';
import type { ActivityNode, ActivityNodeData } from '../types';
import { cx, iconFor } from '../utils/flow';

type BranchRow = {
  caseLabel: string;
  output: string;
  expression: string;
};

function branchRowsFor(data: ActivityNodeData): BranchRow[] {
  if (data.id === 'condition') {
    return [
      { caseLabel: 'CASE 1', output: 'IF', expression: 'fact.amount >= threshold' },
      { caseLabel: 'CASE 2', output: 'ELSE', expression: 'otherwise' },
    ];
  }
  if (data.id === 'qualify') {
    return [
      { caseLabel: 'CASE 1', output: 'IF', expression: 'eligibility == true' },
      { caseLabel: 'CASE 2', output: 'ELIF', expression: 'eligibility == false' },
      { caseLabel: 'CASE 3', output: 'ELSE', expression: 'risk_level > limit' },
    ];
  }
  if (data.id === 'switch') {
    return [
      { caseLabel: 'CASE 1', output: 'IF', expression: 'stage_result == PASS' },
      { caseLabel: 'CASE 2', output: 'ELIF', expression: 'stage_result == BLOCK' },
      { caseLabel: 'CASE 3', output: 'ELSE', expression: 'stage_result == SKIP' },
    ];
  }
  return data.outputs.slice(0, 4).map((output, index) => ({
    caseLabel: index === 0 ? 'CASE 1' : index === data.outputs.length - 1 ? 'ELSE' : `CASE ${index + 1}`,
    output,
    expression: index === 0 ? `${data.inputs[0] ?? 'input'} matched` : 'fallback',
  }));
}

function mainVariableFor(data: ActivityNodeData): string {
  return data.inputs[0] ?? data.outputs[0] ?? 'payload';
}

function sectionTitleFor(kind: ActivityNodeData['kind']): string {
  return kind === 'entry' ? 'START VARIABLES' : kind === 'dispatch' ? 'OUTPUT TARGET' : 'INPUT VARIABLE';
}

export function ActivityNodeView({ id, data, selected }: NodeProps<ActivityNode>) {
  const inputs = data.inputs.length > 0 ? data.inputs : ['input'];
  const outputs = data.outputs.length > 0 ? data.outputs : ['output'];
  const branchRows = ['condition', 'qualify', 'switch'].includes(data.id) ? branchRowsFor(data) : [];
  const isBranchNode = branchRows.length > 0;
  const handleTop = (index: number) => (isBranchNode ? `${56 + index * 31}px` : `${70 + index * 21}px`);
  const inputTop = () => (isBranchNode ? '50%' : '50%');

  return (
    <div
      className={cx('activity-node', data.kind, isBranchNode && 'branch-node', selected && 'is-selected')}
      onMouseEnter={() => data.onHoverChange?.(id, true)}
      onMouseLeave={() => data.onHoverChange?.(id, false)}
    >
      <Handle
        id="input-0"
        type="target"
        position={Position.Left}
        className="input-anchor-handle"
        style={{ top: inputTop() }}
      />
      <span className="input-handle-visual" aria-hidden="true" />
      <div className="node-hover-toolbar nodrag">
        <button
          title="添加节点"
          onClick={(event) => {
            event.stopPropagation();
            data.onAddFromOutput?.({
              nodeId: id,
              outputIndex: 0,
              outputLabel: outputs[0] ?? 'output',
              clientX: event.clientX,
              clientY: event.clientY,
            });
          }}
        >
          +
        </button>
        <button title="运行此步骤">▷</button>
        <button title="更多操作">•••</button>
      </div>
      <div className="activity-node-head">
        <span className={cx('node-icon', data.kind)}>{iconFor(data.kind)}</span>
        <div className="node-title-block">
          <strong>{data.title}</strong>
          <span>{data.code}</span>
        </div>
        <button
          className="node-delete"
          title="删除节点"
          onClick={(event) => {
            event.stopPropagation();
            data.onDelete?.(id);
          }}
        >
          x
        </button>
      </div>
      {isBranchNode ? (
        <div className="branch-list node-selectable nodrag">
          {branchRows.map((item, index) => (
            <div key={`${item.output}-${index}`} className="branch-row">
              <div className="branch-row-head">
                <span>{item.caseLabel}</span>
              </div>
              <div className="branch-expression">
                <span>x</span>
                <code>{item.expression}</code>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <>
          <div className="field-section node-selectable nodrag">
            <b>{sectionTitleFor(data.kind)}</b>
            <div className="variable-row">
              <span className="variable-source">{data.kind === 'entry' ? '{x}' : '↳'}</span>
              <code>{mainVariableFor(data)}</code>
              {data.kind === 'reward' ? <em>DECIMAL</em> : null}
              {data.kind === 'quota' ? <em>ATOMIC</em> : null}
              {data.kind === 'reliable' ? <em>RETRY</em> : null}
            </div>
          </div>
          <p className="node-selectable nodrag">{data.desc}</p>
        </>
      )}
      <div className="node-drag-hint">
        <span>点击添加节点</span>
        <span>拖拽连接节点</span>
      </div>
      <div className={cx('output-handles', isBranchNode && 'branch-output-handles')} aria-hidden="true">
        {outputs.slice(0, 4).map((item, index) => (
          <span key={item} className="output-handle-label" style={{ top: handleTop(index) }}>
            {isBranchNode ? (index === 0 ? 'IF' : index === outputs.length - 1 ? 'ELSE' : 'ELIF') : item}
          </span>
        ))}
      </div>
      {outputs.slice(0, 4).map((item, index) => (
        <div key={item}>
          <Handle
            id={`output-${index}`}
            type="source"
            position={Position.Right}
            className={cx('output-anchor-handle', isBranchNode && 'branch-output-anchor-handle')}
            style={{ top: handleTop(index) }}
            onClick={(event) => {
              event.stopPropagation();
              data.onAddFromOutput?.({
                nodeId: id,
                outputIndex: index,
                outputLabel: item,
                clientX: event.clientX,
                clientY: event.clientY,
              });
            }}
          />
          <span className={cx('output-handle-visual', isBranchNode && 'branch-output-handle-visual')} style={{ top: handleTop(index) }} aria-hidden="true" />
          {isBranchNode ? (
            <span className="output-handle-tooltip" style={{ top: handleTop(index) }}>
              <b>Click to add</b>
              <span>Drag to</span>
              <em>connect</em>
            </span>
          ) : null}
        </div>
      ))}
    </div>
  );
}
