import type { ActivityNodeData, CatalogGroup, NodePickerState } from '../types';
import { cx } from '../utils/flow';

type Props = {
  picker: NodePickerState;
  search: string;
  filteredCatalog: CatalogGroup[];
  onSearch: (value: string) => void;
  onAdd: (def: ActivityNodeData, x: number, y: number) => void;
};

export function NodePicker({ picker, search, filteredCatalog, onSearch, onAdd }: Props) {
  return (
    <div className="node-picker" style={{ left: picker.x, top: picker.y }} onClick={(event) => event.stopPropagation()}>
      <div className="picker-tabs">
        <button className="active">Nodes</button>
        <button>Tools</button>
      </div>
      <div className="picker-search">
        <span>⌕</span>
        <input autoFocus value={search} placeholder="Search node" onChange={(event) => onSearch(event.target.value)} />
      </div>
      <div className="picker-list">
        {filteredCatalog.map((group) => (
          <section key={group.category}>
            <h4>{group.category}</h4>
            {group.items.map((item) => (
              <button key={item.id} onClick={() => onAdd(item, picker.flowX, picker.flowY)}>
                <span className={cx('picker-icon', item.kind)}>{item.code.slice(0, 1)}</span>
                <strong>{item.title}</strong>
                <em>{item.code}</em>
              </button>
            ))}
          </section>
        ))}
        {filteredCatalog.length === 0 ? <div className="picker-empty">No nodes found</div> : null}
      </div>
    </div>
  );
}
