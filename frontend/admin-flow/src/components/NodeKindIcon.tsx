import {
  BadgeCheck,
  Bell,
  Code2,
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
import type { ActivityNodeData, NodeKind } from '../types';
import { cx } from '../utils/flow';

const iconByKind: Record<NodeKind, LucideIcon> = {
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

const iconById: Record<string, LucideIcon> = {
  'event-in': House,
  inbox: Inbox,
  normalize: Shuffle,
  match: Network,
  condition: Split,
  qualify: ShieldCheck,
  switch: ListTree,
  'fact-query': FileSearch,
  code: Code2,
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

type Props = {
  node: Pick<ActivityNodeData, 'id' | 'kind'>;
  className?: string;
  strokeWidth?: number;
};

export function NodeKindIcon({ node, className, strokeWidth = 2.3 }: Props) {
  const Icon = iconById[node.id] ?? iconByKind[node.kind];
  return (
    <span className={cx('node-kind-icon', node.kind, className)}>
      <Icon strokeWidth={strokeWidth} />
    </span>
  );
}
