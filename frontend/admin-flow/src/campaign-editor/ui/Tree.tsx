import { ChevronDown } from 'lucide-react';
import type { CSSProperties, DragEventHandler, ReactNode } from 'react';

export type TreeNode = {
  id: string;
  label: ReactNode;
  meta?: ReactNode;
  count?: ReactNode;
  active?: boolean;
  open?: boolean;
  disabled?: boolean;
  children?: TreeNode[];
  actions?: ReactNode;
  draggable?: boolean;
  depth?: number;
  onClick?: () => void;
  onToggle?: () => void;
  onDragStart?: DragEventHandler<HTMLDivElement>;
  onDragOver?: DragEventHandler<HTMLDivElement>;
  onDrop?: DragEventHandler<HTMLDivElement>;
};

type TreeProps = {
  nodes: TreeNode[];
  className?: string;
  rowClassName?: string;
  branchClassName?: string;
  childrenClassName?: string;
  noWrapper?: boolean;
  empty?: ReactNode;
};

function renderTreeNode(node: TreeNode, depth = 0, props: Pick<TreeProps, 'rowClassName' | 'branchClassName' | 'childrenClassName'> = {}) {
  const hasChildren = Boolean(node.children?.length);
  return (
    <div key={node.id} className={`exm-tree-branch ${props.branchClassName ?? ''}`.trim()}>
      <div
        className={`exm-tree-row ${props.rowClassName ?? ''} ${node.active ? 'active' : ''} ${node.disabled ? 'disabled' : ''}`.trim()}
        role="button"
        tabIndex={node.disabled ? -1 : 0}
        draggable={node.draggable}
        style={{ '--tree-depth': depth } as CSSProperties}
        onClick={node.disabled ? undefined : node.onClick}
        onKeyDown={(event) => {
          if (node.disabled) return;
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            node.onClick?.();
          }
        }}
        onDragStart={node.onDragStart}
        onDragOver={node.onDragOver}
        onDrop={node.onDrop}
      >
        {hasChildren || node.onToggle ? (
          <span
            className={`exm-tree-toggle ${node.open ? 'open' : ''}`.trim()}
            aria-hidden="true"
          >
            <ChevronDown size={14} />
          </span>
        ) : <span className="exm-tree-indent" />}
        <span className="exm-tree-label"><strong>{node.label}</strong>{node.meta ? <small>{node.meta}</small> : null}</span>
        {node.count !== undefined ? <b>{node.count}</b> : null}
        {node.actions ? <span className="exm-tree-actions" onClick={(event) => event.stopPropagation()}>{node.actions}</span> : null}
      </div>
      {node.open !== false && hasChildren ? (
        <div className={`exm-tree-children ${props.childrenClassName ?? ''}`.trim()}>
          {node.children!.map((child) => renderTreeNode(child, depth + 1, props))}
        </div>
      ) : null}
    </div>
  );
}

export function Tree({ nodes, className = '', empty, rowClassName, branchClassName, childrenClassName, noWrapper = false }: TreeProps) {
  const content = nodes.length ? nodes.map((node) => renderTreeNode(node, node.depth ?? 0, { rowClassName, branchClassName, childrenClassName })) : empty;
  if (noWrapper) return <>{content}</>;
  return (
    <div className={`exm-tree ${className}`.trim()}>
      {content}
    </div>
  );
}
