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
  '基础节点': '生产活动主链路和常用业务节点',
  '高级节点': '用户分层、事实标准化、风控、插件、审批和对账',
  '组合节点': '数据组合、脚本处理、条件分支和表达式编排',
  '其他节点': '事件适配、底层查询、旧节点、投递边界和运维辅助',
  'EVENT_INPUT 事件输入': 'HTTP、Webhook、MQ、定时和人工触发',
  'CORE 核心链路': '生产编排主链路',
  'FACT_QUERY 事实查询': '上下文、缓存、HTTP、Redis、gRPC 和插件查询',
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
