import type { NodeProps } from '@xyflow/react';
import type { GroupNode, NoteNode } from '../types';

export function GroupNodeView({ id, data }: NodeProps<GroupNode>) {
  return (
    <div className="group-node" style={{ width: data.width, height: data.height }}>
      <div className="group-node-head">
        <strong>{data.title}</strong>
        <button
          className="node-delete"
          title="删除组合"
          onClick={(event) => {
            event.stopPropagation();
            data.onDelete?.(id);
          }}
        >
          x
        </button>
      </div>
    </div>
  );
}

export function NoteNodeView({ id, data }: NodeProps<NoteNode>) {
  return (
    <div className="note-flow-node">
      <button
        className="node-delete"
        title="删除备注"
        onClick={(event) => {
          event.stopPropagation();
          data.onDelete?.(id);
        }}
      >
        x
      </button>
      <p className="node-selectable nodrag">{data.text}</p>
      <span>ExMarketing</span>
    </div>
  );
}
