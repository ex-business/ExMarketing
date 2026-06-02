import type { ActivityNodeData } from '../types';
import { flatCatalog, nodeCatalog } from '../data/nodeCatalog';
import { cx } from '../utils/flow';

type Props = {
  open: boolean;
  selectedTemplate: 'default' | 'simple';
  onAddNode: (def: ActivityNodeData) => void;
  onDefault: () => void;
  onSimple: () => void;
};

export function NodePalette({ open, selectedTemplate, onAddNode, onDefault, onSimple }: Props) {
  return (
    <aside className={cx('flow-sidebar', open && 'open')}>
      <div className="brand">
        <b>ExMarketing</b>
        <span>Activity Flow</span>
      </div>
      <button className="back-link" onClick={() => { window.location.href = '/admin/'; }}>返回旧后台</button>

      <div className="side-head">
        <h2>活动流程模板</h2>
        <span>{selectedTemplate === 'default' ? '完整链路' : '简化链路'}</span>
      </div>
      <div className="template-actions">
        <button onClick={onDefault}>完整活动引擎</button>
        <button onClick={onSimple}>简化发奖链路</button>
      </div>

      <div className="side-head">
        <h2>常用节点函数</h2>
        <span>{flatCatalog().length}</span>
      </div>
      <div className="node-palette">
        {nodeCatalog.map((group) => (
          <section key={group.category}>
            <h3>{group.category}</h3>
            {group.items.map((item) => (
              <button key={item.id} onClick={() => onAddNode(item)}>
                <span className={cx('palette-dot', item.kind)} />
                <strong>{item.title}</strong>
                <em>{item.code}</em>
              </button>
            ))}
          </section>
        ))}
      </div>
    </aside>
  );
}
