import { useMemo, useState } from 'react';
import {
  Bell,
  BadgeCheck,
  ClipboardCheck,
  Database,
  FileSearch,
  Gauge,
  Gift,
  GitBranch,
  House,
  Inbox,
  ListChecks,
  ListTree,
  Megaphone,
  Network,
  Reply,
  RotateCcw,
  Search,
  Send,
  ShieldCheck,
  Shuffle,
  Split,
  Webhook,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import type { ActivityNodeData } from '../types';
import type { NodeKind } from '../types';
import { nodeCatalog } from '../data/nodeCatalog';
import { cx } from '../utils/flow';

type Props = {
  open: boolean;
  onAddNode: (def: ActivityNodeData) => void;
  selectedTemplate: 'default' | 'simple';
  onDefault: () => void;
  onSimple: () => void;
};

const paletteIconByKind: Record<NodeKind, LucideIcon> = {
  entry: House,
  store: Database,
  transform: Shuffle,
  logic: GitBranch,
  query: Search,
  reward: Gift,
  quota: Gauge,
  grant: BadgeCheck,
  dispatch: Send,
  callback: Reply,
  reliable: RotateCcw,
  audit: ClipboardCheck,
};

const paletteIconById: Record<string, LucideIcon> = {
  'event-in': House,
  inbox: Inbox,
  normalize: Shuffle,
  match: Network,
  condition: Split,
  qualify: ShieldCheck,
  switch: ListTree,
  'fact-query': FileSearch,
  reward: Gift,
  decimal: Gauge,
  limit: ListChecks,
  quota: Database,
  participation: ClipboardCheck,
  'grant-order': BadgeCheck,
  outbox: Send,
  'http-call': Webhook,
  'mq-publish': Megaphone,
  webhook: Webhook,
  callback: Reply,
  retry: RotateCcw,
  dlq: Inbox,
  audit: ClipboardCheck,
  reconcile: Search,
  alert: Bell,
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
  const Icon = paletteIconById[item.id] ?? paletteIconByKind[item.kind];
  return (
    <button onClick={() => onAddNode(item)}>
      <span className={cx('palette-dot', item.kind)}>
        <Icon strokeWidth={2.3} />
      </span>
      <strong>{item.title}</strong>
      <em>{item.code}</em>
    </button>
  );
}
