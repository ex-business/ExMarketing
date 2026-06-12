import type { ActivityNodeData, CatalogGroup, NodePickerState } from '../types';
import { cx } from '../utils/flow';

type Props = {
  picker: NodePickerState;
  search: string;
  filteredCatalog: CatalogGroup[];
  onSearch: (value: string) => void;
  onAdd: (def: ActivityNodeData, x: number, y: number) => void;
};

const CATEGORY_HINTS: Record<string, string> = {
  'CORE 核心链路': '生产编排主链路',
  'ADVANCED 高级集成': '外部系统与扩展动作',
  'OPS 运维节点': '重试、审计、对账和告警',
};

export function NodePicker({ picker, search, filteredCatalog, onSearch, onAdd }: Props) {
  return (
    <div className="node-picker" style={{ left: picker.x, top: picker.y }} onClick={(event) => event.stopPropagation()}>
      <div className="picker-tabs">
        <button className="active">节点</button>
        <button>工具</button>
      </div>
      <div className="picker-search">
        <span>⌕</span>
        <input autoFocus value={search} placeholder="搜索节点" onChange={(event) => onSearch(event.target.value)} />
      </div>
      <div className="picker-list">
        {filteredCatalog.map((group) => (
          <section key={group.category}>
            <h4>
              <b>{group.category}</b>
              <span>{CATEGORY_HINTS[group.category] ?? '节点模板'}</span>
            </h4>
            {group.items.map((item) => (
              <button key={item.id} onClick={() => onAdd(item, picker.flowX, picker.flowY)}>
                <span className={cx('picker-icon', item.kind)}>{item.code.slice(0, 1)}</span>
                <strong>{item.title}</strong>
                <em>{item.code}</em>
              </button>
            ))}
          </section>
        ))}
        {filteredCatalog.length === 0 ? <div className="picker-empty">没有匹配节点</div> : null}
      </div>
    </div>
  );
}
