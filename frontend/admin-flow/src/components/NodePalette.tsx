import { useMemo, useState } from 'react';
import type { ActivityNodeData } from '../types';
import { nodeCatalog } from '../data/nodeCatalog';
import { cx } from '../utils/flow';
import { NodeKindIcon } from './NodeKindIcon';

type Props = {
  open: boolean;
  onAddNode: (def: ActivityNodeData) => void;
  selectedTemplate: 'default' | 'simple';
  onDefault: () => void;
  onSimple: () => void;
};

export function NodePalette({ open, onAddNode }: Props) {
  const [search, setSearch] = useState('');
  const filteredCatalog = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return nodeCatalog;
    return nodeCatalog
      .map((group) => ({
        ...group,
        items: group.items.filter((item) => (
          item.title.toLowerCase().includes(keyword)
          || item.code.toLowerCase().includes(keyword)
          || item.desc.toLowerCase().includes(keyword)
        )),
      }))
      .filter((group) => group.items.length > 0);
  }, [search]);

  return (
    <aside className={cx('flow-sidebar', open && 'open')}>
      <div className="palette-head">
        <b>添加节点</b>
        <span>选择一个步骤加入当前流程</span>
      </div>
      <input
        className="palette-search"
        value={search}
        placeholder="搜索节点名称、代码或说明"
        onChange={(event) => setSearch(event.target.value)}
      />
      <div className="node-palette">
        {filteredCatalog.map((group) => (
          <section key={group.category}>
            <h3>{group.category}</h3>
            {group.items.map((item) => (
              <PaletteNodeButton key={item.id} item={item} onAddNode={onAddNode} />
            ))}
          </section>
        ))}
      </div>
    </aside>
  );
}

function PaletteNodeButton({ item, onAddNode }: { item: ActivityNodeData; onAddNode: (def: ActivityNodeData) => void }) {
  return (
    <button onClick={() => onAddNode(item)}>
      <NodeKindIcon node={item} className="palette-dot" />
      <strong>{item.title}</strong>
      <em>{item.code}</em>
    </button>
  );
}
