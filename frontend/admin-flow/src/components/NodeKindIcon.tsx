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
  Combine,
  ListChecks,
  ListTree,
  Megaphone,
  Network,
  Radio,
  Reply,
  RotateCcw,
  ScrollText,
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
  'http-event': Webhook,
  'webhook-event': Webhook,
  'mq-event': Megaphone,
  'schedule-event': ScrollText,
  'state-event': Radio,
  'manual-event': ClipboardCheck,
  inbox: Inbox,
  normalize: Shuffle,
  match: Network,
  'activity-state': Database,
  'quota-check': ListChecks,
  condition: Split,
  qualify: ShieldCheck,
  switch: ListTree,
  'fact-query': FileSearch,
  code: Code2,
  combine: Combine,
  split: Split,
  'candidate-reward': Gift,
  reward: Gift,
  decimal: Gauge,
  limit: ListChecks,
  quota: Database,
  'budget-reserve': Database,
  participation: ClipboardCheck,
  'grant-order': BadgeCheck,
  outbox: Send,
  'http-call': Webhook,
  'mq-publish': Megaphone,
  webhook: Webhook,
  callback: Reply,
  'approval-gate': ShieldCheck,
  compensation: RotateCcw,
  'random-pick': Shuffle,
  'plugin-execute': Network,
  'ab-bucket': GitBranch,
  'risk-score': Gauge,
  'settlement-window': ScrollText,
  'manual-adjust': ClipboardCheck,
  retry: RotateCcw,
  dlq: Inbox,
  audit: ClipboardCheck,
  'reward-reconcile': Search,
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
