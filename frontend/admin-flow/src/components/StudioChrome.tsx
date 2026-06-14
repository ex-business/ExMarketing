import { useEffect, useMemo, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { WorkflowDefinition, WorkflowDiffDetail, WorkflowDiffItem, WorkflowDiffSummary, WorkflowValidationIssue, WorkflowValidationResult, WorkflowVersion } from '../types';
import { BrandMark } from './workspace-home';

type ActivityOption = {
  id: string;
  workspaceId?: string;
  folderId?: string;
  tenantId?: string;
  activityCode: string;
  activityName: string;
  status?: string;
  version?: number;
};

type WorkspaceOption = {
  id: string;
  name: string;
  slug: string;
  color?: string;
  visibility: string;
  status: string;
};

type FolderOption = {
  id: string;
  workspaceId: string;
  name: string;
  color?: string;
  visibility: string;
  sortOrder: number;
  status: string;
};

type ActivityPlacement = {
  workspaceId: string;
  folderId: string;
};

type ChecklistStatus = 'pass' | 'warning' | 'block' | 'unchecked';

type ChecklistDefinition = {
  id: string;
  title: string;
  description: string;
  match: (item: WorkflowValidationIssue) => boolean;
};

type ChecklistItem = ChecklistDefinition & {
  status: ChecklistStatus;
  count: number;
};

const CHECKLIST_DEFINITIONS: ChecklistDefinition[] = [
  {
    id: 'entry',
    title: '标准事件入口存在',
    description: '必须有标准事件入口节点。',
    match: (item) => item.code === 'WORKFLOW_ENTRY_MISSING',
  },
  {
    id: 'graph',
    title: '节点和连线合法',
    description: '节点 ID、连线端点、重复出口和孤岛节点都需要合法。',
    match: (item) => [
      'WORKFLOW_NODES_REQUIRED',
      'WORKFLOW_NODE_ID_REQUIRED',
      'WORKFLOW_NODE_ID_DUPLICATED',
      'WORKFLOW_NODE_TYPE_REQUIRED',
      'WORKFLOW_NODE_TYPE_UNSUPPORTED',
      'WORKFLOW_EDGE_ENDPOINT_REQUIRED',
      'WORKFLOW_EDGE_SOURCE_MISSING',
      'WORKFLOW_EDGE_TARGET_MISSING',
      'WORKFLOW_OUTPUT_DUPLICATED_EDGE',
      'WORKFLOW_NODE_ISLAND',
      'WORKFLOW_NODE_UNREACHABLE',
    ].includes(item.code),
  },
  {
    id: 'config',
    title: '必填节点配置完成',
    description: '每类节点 schema 的必填字段和规则都要通过。',
    match: (item) => item.code.startsWith('WORKFLOW_NODE_CONFIG_'),
  },
  {
    id: 'variables',
    title: '变量引用有效',
    description: '下游输入必须引用真实可达的上游输出。',
    match: (item) => item.code.startsWith('WORKFLOW_NODE_INPUT_MAPPING_'),
  },
  {
    id: 'routes',
    title: '分支路由合法',
    description: 'route model、默认分支和分支覆盖必须完整。',
    match: (item) => item.code.startsWith('WORKFLOW_EDGE_ROUTE_') || item.code.startsWith('WORKFLOW_BRANCH_'),
  },
  {
    id: 'risk-config',
    title: 'Reward / Quota / Limit 风险配置完整',
    description: '奖励、额度、限制类节点必须满足配置 schema。',
    match: (item) => item.code.startsWith('WORKFLOW_NODE_CONFIG_') && /reward|quota|limit|decimal/i.test(`${item.nodeId ?? ''} ${item.path ?? ''}`),
  },
  {
    id: 'cycles',
    title: '没有不支持的环',
    description: '当前 executor 不允许无法编译的循环依赖。',
    match: (item) => item.code === 'WORKFLOW_UNSUPPORTED_CYCLE',
  },
  {
    id: 'executor-plan',
    title: '可以编译成 executor plan',
    description: '通过入口、拓扑、路由、变量和配置检查后才能发布。',
    match: (item) => item.severity === 'BLOCK',
  },
];

type Props = {
  homeMode: boolean;
  showPalette: boolean;
  interactionMode: 'pointer' | 'hand';
  nodeCount: number;
  edgeCount: number;
  selectedCount: number;
  selectedNodeCount: number;
  lastAdded: string;
  activities: ActivityOption[];
  workspaces: WorkspaceOption[];
  folders: FolderOption[];
  placements: Record<string, ActivityPlacement>;
  activityId: string;
  activityLabel: string;
  workflowStatus: string;
  workflowVersionLabel: string;
  workflowVersions: WorkflowVersion[];
  draftDirty: boolean;
  draftSaveState: 'idle' | 'dirty' | 'saving' | 'saved' | 'failed';
  draftSaveError: string;
  lastSavedAt: string;
  publishState: 'idle' | 'publishing' | 'published' | 'failed';
  publishError: string;
  draftActiveDiff: WorkflowDiffDetail;
  zoomPercent: number;
  showChecklist: boolean;
  validatingWorkflow: boolean;
  workflowValidation: WorkflowValidationResult | null;
  isPublished: boolean;
  publishDisabled: boolean;
  canEditWorkflow: boolean;
  canPublishWorkflow: boolean;
  workspaceRole: string;
  onTogglePalette: () => void;
  onActivityChange: (activityId: string) => void;
  onWorkspaceNavigate: (workspaceId: string, folderId?: string) => void;
  onActivitySettings: () => void;
  onModeChange: (mode: 'pointer' | 'hand') => void;
  onDefault: () => void;
  onSimple: () => void;
  onTemplates: () => void;
  onAutoLayout: () => void;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onZoomTo: (zoom: number) => void;
  onFitView: () => void;
  showMiniMap: boolean;
  onToggleMiniMap: () => void;
  onGroup: () => void;
  onDelete: () => void;
  onClear: () => void;
  onSaveDraft: () => void;
  onPreview: () => void;
  onRunHistory: () => void;
  runHistoryOpen: boolean;
  onCloseRunHistory: () => void;
  onGlobalPanelOpen: () => void;
  onDebugRun: () => void;
  onToggleChecklist: () => void;
  onRightPanelResizeStart: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  onValidationIssueFocus: (item: WorkflowValidationIssue) => void;
  onWorkflowDiffFocus: (item: WorkflowDiffItem) => void;
  onPublish: (releaseNote: string) => void;
  onRollbackVersion: (versionId: string, note: string) => void;
  onArchiveVersion: (versionId: string, note: string) => void;
};

function buildChecklistItems(validation: WorkflowValidationResult): ChecklistItem[] {
  const validationItems = workflowValidationItems(validation);
  return CHECKLIST_DEFINITIONS.map((definition) => {
    const related = validationItems.filter(definition.match);
    const hasBlock = related.some((item) => item.severity === 'BLOCK');
    const hasWarning = related.some((item) => item.severity === 'WARNING');
    return {
      ...definition,
      status: hasBlock ? 'block' : hasWarning ? 'warning' : 'pass',
      count: related.length,
    };
  });
}

function workflowValidationItems(validation: WorkflowValidationResult | null): WorkflowValidationIssue[] {
  return Array.isArray(validation?.items) ? validation.items : [];
}

function statusIcon(status: ChecklistStatus): string {
  switch (status) {
    case 'pass': return '✓';
    case 'warning': return '?';
    case 'block': return '!';
    default: return '-';
  }
}

function statusLabel(status: ChecklistStatus, count: number): string {
  switch (status) {
    case 'pass': return 'PASS';
    case 'warning': return `${count} 警告`;
    case 'block': return `${count} 阻塞`;
    default: return '未检查';
  }
}

function VersionCard({ version, active, onRollback, onArchive }: { version: WorkflowVersion; active?: boolean; onRollback?: () => void; onArchive?: () => void }) {
  return (
    <div className={`version-card ${active ? 'active' : ''} status-${version.status.toLowerCase()}`}>
      <i>{active ? 'ACTIVE' : version.status}</i>
      <span>
        <b>流程 v{version.version}</b>
        <em>{new Date(version.publishedAt).toLocaleString()} · {version.publishedBy || 'system'}</em>
        {version.releaseNote ? <small>{version.releaseNote}</small> : null}
        {version.action ? <small>{version.action}{version.sourceVersionId ? ` · from ${version.sourceVersionId.slice(0, 10)}` : ''}</small> : null}
      </span>
      <div>
        {onRollback ? <button onClick={onRollback}>回滚</button> : null}
        {onArchive ? <button onClick={onArchive}>归档</button> : null}
      </div>
    </div>
  );
}

function diffKindLabel(kind: WorkflowDiffItem['kind']): string {
  switch (kind) {
    case 'NODE_ADDED': return '新增节点';
    case 'NODE_REMOVED': return '删除节点';
    case 'NODE_CHANGED': return '修改节点';
    case 'EDGE_ADDED': return '新增连线';
    case 'EDGE_REMOVED': return '删除连线';
    case 'EDGE_CHANGED': return '修改连线';
    default: return kind;
  }
}

function diffKindClass(kind: WorkflowDiffItem['kind']): string {
  if (kind.endsWith('_ADDED')) return 'added';
  if (kind.endsWith('_REMOVED')) return 'removed';
  return 'changed';
}

function WorkflowDiffList({ items, onFocus }: { items: WorkflowDiffItem[]; onFocus: (item: WorkflowDiffItem) => void }) {
  const groups: Array<{ title: string; kinds: WorkflowDiffItem['kind'][] }> = [
    { title: '新增节点', kinds: ['NODE_ADDED'] },
    { title: '删除节点', kinds: ['NODE_REMOVED'] },
    { title: '修改节点', kinds: ['NODE_CHANGED'] },
    { title: '新增连线', kinds: ['EDGE_ADDED'] },
    { title: '删除连线', kinds: ['EDGE_REMOVED'] },
    { title: '修改连线', kinds: ['EDGE_CHANGED'] },
  ];
  if (items.length === 0) return <div className="version-empty">暂无可定位的 diff 项。</div>;
  return (
    <div className="workflow-diff-list">
      {groups.map((group) => {
        const groupItems = items.filter((item) => group.kinds.includes(item.kind));
        if (groupItems.length === 0) return null;
        return (
          <section key={group.title}>
            <h4>{group.title}<span>{groupItems.length}</span></h4>
            {groupItems.map((item) => (
              <button key={item.id} className={`workflow-diff-row ${diffKindClass(item.kind)}`} type="button" onClick={() => onFocus(item)}>
                <i>{diffKindLabel(item.kind)}</i>
                <span>
                  <b>{item.label}</b>
                  <em>{item.detail || item.edgeId || item.nodeId}</em>
                </span>
              </button>
            ))}
          </section>
        );
      })}
    </div>
  );
}

function PublishConfirmModal({
  activeVersion,
  draftActiveDiff,
  draftDirty,
  validatingWorkflow,
  workflowValidation,
  publishState,
  onClose,
  onConfirm,
}: {
  activeVersion?: WorkflowVersion;
  draftActiveDiff: WorkflowDiffSummary;
  draftDirty: boolean;
  validatingWorkflow: boolean;
  workflowValidation: WorkflowValidationResult | null;
  publishState: 'idle' | 'publishing' | 'published' | 'failed';
  onClose: () => void;
  onConfirm: (releaseNote: string) => void;
}) {
  const [releaseNote, setReleaseNote] = useState('');
  const blockCount = workflowValidation?.summary.blockCount ?? 0;
  const warningCount = workflowValidation?.summary.warningCount ?? 0;
  const canConfirm = !validatingWorkflow && publishState !== 'publishing' && blockCount === 0;
  const targetVersion = activeVersion ? activeVersion.version + 1 : 1;
  return (
    <div className="publish-confirm-backdrop" role="presentation">
      <section className="publish-confirm-modal" role="dialog" aria-modal="true" aria-labelledby="publish-confirm-title">
        <header>
          <span>
            <b id="publish-confirm-title">发布流程</b>
            <em>{activeVersion ? `当前线上 v${activeVersion.version}，本次将发布 v${targetVersion}` : '当前没有线上版本，本次将发布 v1'}</em>
          </span>
          <button type="button" onClick={onClose}>x</button>
        </header>
        <div className="publish-confirm-grid">
          <div className={`publish-check-card ${blockCount > 0 ? 'block' : warningCount > 0 ? 'warning' : workflowValidation ? 'pass' : 'unchecked'}`}>
            <i>{validatingWorkflow ? '...' : blockCount > 0 ? '!' : warningCount > 0 ? '?' : workflowValidation ? '✓' : '-'}</i>
            <span>
              <b>{validatingWorkflow ? '正在执行发布检查' : workflowValidation ? blockCount > 0 ? '存在阻塞项' : warningCount > 0 ? '有警告但可发布' : '发布检查通过' : '尚未执行检查'}</b>
              <em>{workflowValidation ? `${blockCount} 阻塞 / ${warningCount} 警告` : '确认发布时会先保存 draft 并执行真实校验'}</em>
            </span>
          </div>
          <div className={`version-draft-card ${draftActiveDiff.changed ? 'changed' : 'clean'}`}>
            <b>{draftDirty ? 'Draft 有未保存变更' : 'Draft 状态'}</b>
            <span>{draftActiveDiff.summary}</span>
            {draftActiveDiff.detail.length ? draftActiveDiff.detail.slice(0, 8).map((item) => <em key={item}>{item}</em>) : <em>没有可展示的结构差异。</em>}
          </div>
          <label className="version-note-field">
            <span>发布说明</span>
            <textarea value={releaseNote} placeholder="说明本次发布变更、风险或回滚依据" onChange={(event) => setReleaseNote(event.target.value)} />
          </label>
        </div>
        <footer>
          <button type="button" onClick={onClose}>取消</button>
          <button className="publish-confirm-primary" type="button" disabled={!canConfirm} onClick={() => onConfirm(releaseNote)}>
            {publishState === 'publishing' ? '发布中...' : blockCount > 0 ? '修复阻塞项后发布' : `确认发布 v${targetVersion}`}
          </button>
        </footer>
      </section>
    </div>
  );
}

function RollbackConfirmModal({
  activeVersion,
  targetVersion,
  onClose,
  onConfirm,
}: {
  activeVersion?: WorkflowVersion;
  targetVersion: WorkflowVersion;
  onClose: () => void;
  onConfirm: (note: string) => void;
}) {
  const diff = workflowDiffSummaryLocal(targetVersion.definition, activeVersion?.definition, '当前没有 active version 可对比');
  const nextVersion = activeVersion ? activeVersion.version + 1 : targetVersion.version + 1;
  const [note, setNote] = useState(`Rollback to workflow v${targetVersion.version}`);
  return (
    <div className="publish-confirm-backdrop" role="presentation">
      <section className="publish-confirm-modal rollback-confirm-modal" role="dialog" aria-modal="true" aria-labelledby="rollback-confirm-title">
        <header>
          <span>
            <b id="rollback-confirm-title">回滚流程</b>
            <em>{activeVersion ? `从当前 active v${activeVersion.version} 回滚到 v${targetVersion.version}` : `恢复 v${targetVersion.version}`}</em>
          </span>
          <button type="button" onClick={onClose}>x</button>
        </header>
        <div className="publish-confirm-grid">
          <div className="publish-check-card warning">
            <i>!</i>
            <span>
              <b>回滚会生成新的 active version</b>
              <em>确认后不会直接复活旧版本，而是基于 v{targetVersion.version} 创建并发布 v{nextVersion}。</em>
            </span>
          </div>
          <div className={`version-draft-card ${diff.changed ? 'changed' : 'clean'}`}>
            <b>v{targetVersion.version} vs {activeVersion ? `active v${activeVersion.version}` : 'current active'}</b>
            <span>{diff.summary}</span>
            {diff.detail.length ? diff.detail.slice(0, 8).map((item) => <em key={item}>{item}</em>) : <em>没有可展示的结构差异。</em>}
          </div>
          <label className="version-note-field">
            <span>回滚说明 / 审计备注</span>
            <textarea value={note} onChange={(event) => setNote(event.target.value)} />
          </label>
        </div>
        <footer>
          <button type="button" onClick={onClose}>取消</button>
          <button className="publish-confirm-primary rollback" type="button" onClick={() => onConfirm(note)}>确认回滚到 v{targetVersion.version}</button>
        </footer>
      </section>
    </div>
  );
}

function ArchiveConfirmModal({
  version,
  activeVersion,
  onClose,
  onConfirm,
}: {
  version: WorkflowVersion;
  activeVersion?: WorkflowVersion;
  onClose: () => void;
  onConfirm: (note: string) => void;
}) {
  const diff = workflowDiffSummaryLocal(version.definition, activeVersion?.definition, '当前没有 active version 可对比');
  const [note, setNote] = useState(`Archive workflow v${version.version}`);
  return (
    <div className="publish-confirm-backdrop" role="presentation">
      <section className="publish-confirm-modal archive-confirm-modal" role="dialog" aria-modal="true" aria-labelledby="archive-confirm-title">
        <header>
          <span>
            <b id="archive-confirm-title">归档流程版本</b>
            <em>即将归档流程 v{version.version}</em>
          </span>
          <button type="button" onClick={onClose}>x</button>
        </header>
        <div className="publish-confirm-grid">
          <div className="publish-check-card warning">
            <i>!</i>
            <span>
              <b>归档后该版本只保留历史查看用途</b>
              <em>Active version 不允许归档；归档历史版本不会影响当前线上执行。</em>
            </span>
          </div>
          <div className={`version-draft-card ${diff.changed ? 'changed' : 'clean'}`}>
            <b>v{version.version} vs {activeVersion ? `active v${activeVersion.version}` : 'current active'}</b>
            <span>{diff.summary}</span>
            {diff.detail.length ? diff.detail.slice(0, 6).map((item) => <em key={item}>{item}</em>) : <em>没有可展示的结构差异。</em>}
          </div>
          <label className="version-note-field">
            <span>归档说明 / 审计备注</span>
            <textarea value={note} onChange={(event) => setNote(event.target.value)} />
          </label>
        </div>
        <footer>
          <button type="button" onClick={onClose}>取消</button>
          <button className="publish-confirm-primary archive" type="button" onClick={() => onConfirm(note)}>确认归档 v{version.version}</button>
        </footer>
      </section>
    </div>
  );
}

function comparableWorkflowDefinition(definition: WorkflowDefinition | undefined): WorkflowDefinition | undefined {
  if (!definition) return undefined;
  return {
    ...definition,
    viewport: undefined,
    meta: definition.meta ? { ...definition.meta, exportedAt: undefined } : undefined,
    nodes: [...(definition.nodes ?? [])].sort((a, b) => a.id.localeCompare(b.id)),
    edges: [...(definition.edges ?? [])].sort((a, b) => a.id.localeCompare(b.id)),
  };
}

function stableWorkflowString(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableWorkflowString).join(',')}]`;
  if (value && typeof value === 'object') {
    return `{${Object.entries(value as Record<string, unknown>)
      .filter(([, item]) => item !== undefined)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([key, item]) => `${JSON.stringify(key)}:${stableWorkflowString(item)}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

function workflowDiffSummaryLocal(left: WorkflowDefinition | undefined, right: WorkflowDefinition | undefined, empty = '缺少对比版本'): WorkflowDiffDetail {
  const a = comparableWorkflowDefinition(left);
  const b = comparableWorkflowDefinition(right);
  if (!a || !b) return { changed: Boolean(a || b), summary: empty, detail: [], items: [] };
  if (stableWorkflowString(a) === stableWorkflowString(b)) {
    return { changed: false, summary: '无差异', detail: ['节点、连线、配置和 route 均一致。'], items: [] };
  }
  const detail: string[] = [];
  const items: WorkflowDiffItem[] = [];
  const leftNodes = new Map(a.nodes.map((node) => [node.id, node]));
  const rightNodes = new Map(b.nodes.map((node) => [node.id, node]));
  const leftEdges = new Map(a.edges.map((edge) => [edge.id, edge]));
  const rightEdges = new Map(b.edges.map((edge) => [edge.id, edge]));
  const nodeAdded = [...leftNodes.keys()].filter((id) => !rightNodes.has(id));
  const nodeRemoved = [...rightNodes.keys()].filter((id) => !leftNodes.has(id));
  const nodeChanged = [...leftNodes.keys()].filter((id) => rightNodes.has(id) && stableWorkflowString(leftNodes.get(id)) !== stableWorkflowString(rightNodes.get(id)));
  const edgeAdded = [...leftEdges.keys()].filter((id) => !rightEdges.has(id));
  const edgeRemoved = [...rightEdges.keys()].filter((id) => !leftEdges.has(id));
  const edgeChanged = [...leftEdges.keys()].filter((id) => rightEdges.has(id) && stableWorkflowString(leftEdges.get(id)) !== stableWorkflowString(rightEdges.get(id)));
  if (nodeAdded.length) detail.push(`新增节点：${nodeAdded.join(', ')}`);
  if (nodeRemoved.length) detail.push(`删除节点：${nodeRemoved.join(', ')}`);
  if (nodeChanged.length) detail.push(`变更节点：${nodeChanged.join(', ')}`);
  if (edgeAdded.length) detail.push(`新增连线：${edgeAdded.join(', ')}`);
  if (edgeRemoved.length) detail.push(`删除连线：${edgeRemoved.join(', ')}`);
  if (edgeChanged.length) detail.push(`变更连线：${edgeChanged.join(', ')}`);
  nodeAdded.forEach((id) => {
    const node = leftNodes.get(id);
    items.push({ id: `node-added-${id}`, kind: 'NODE_ADDED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: '左侧版本新增节点' });
  });
  nodeRemoved.forEach((id) => {
    const node = rightNodes.get(id);
    items.push({ id: `node-removed-${id}`, kind: 'NODE_REMOVED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: '右侧版本存在，左侧版本不存在' });
  });
  nodeChanged.forEach((id) => {
    const node = leftNodes.get(id);
    items.push({ id: `node-changed-${id}`, kind: 'NODE_CHANGED', label: node?.title ?? node?.code ?? id, nodeId: id, detail: '节点位置、配置、输入或输出发生变化' });
  });
  edgeAdded.forEach((id) => {
    const edge = leftEdges.get(id);
    items.push({ id: `edge-added-${id}`, kind: 'EDGE_ADDED', label: edge?.label || id, edgeId: id, source: edge?.source, target: edge?.target, detail: '左侧版本新增连线' });
  });
  edgeRemoved.forEach((id) => {
    const edge = rightEdges.get(id);
    items.push({ id: `edge-removed-${id}`, kind: 'EDGE_REMOVED', label: edge?.label || id, edgeId: id, source: edge?.source, target: edge?.target, detail: '右侧版本存在，左侧版本不存在' });
  });
  edgeChanged.forEach((id) => {
    const edge = leftEdges.get(id);
    items.push({ id: `edge-changed-${id}`, kind: 'EDGE_CHANGED', label: edge?.label || id, edgeId: id, source: edge?.source, target: edge?.target, detail: '连线端点、handle、label 或 route 发生变化' });
  });
  return {
    changed: true,
    summary: `节点 +${nodeAdded.length} / -${nodeRemoved.length} / ~${nodeChanged.length}，连线 +${edgeAdded.length} / -${edgeRemoved.length} / ~${edgeChanged.length}`,
    detail,
    items,
  };
}

export function StudioChrome({
  homeMode,
  showPalette,
  interactionMode,
  nodeCount,
  edgeCount,
  selectedCount,
  selectedNodeCount,
  lastAdded,
  activities,
  workspaces,
  folders,
  placements,
  activityId,
  activityLabel,
  workflowStatus,
  workflowVersionLabel,
  workflowVersions,
  draftDirty,
  draftSaveState,
  draftSaveError,
  lastSavedAt,
  publishState,
  publishError,
  draftActiveDiff,
  zoomPercent,
  showChecklist,
  validatingWorkflow,
  workflowValidation,
  isPublished,
  publishDisabled,
  canEditWorkflow,
  canPublishWorkflow,
  workspaceRole,
  onTogglePalette,
  onActivityChange,
  onWorkspaceNavigate,
  onActivitySettings,
  onModeChange,
  onDefault,
  onSimple,
  onTemplates,
  onAutoLayout,
  onZoomIn,
  onZoomOut,
  onZoomTo,
  onFitView,
  showMiniMap,
  onToggleMiniMap,
  onGroup,
  onDelete,
  onClear,
  onSaveDraft,
  onPreview,
  onRunHistory,
  runHistoryOpen,
  onCloseRunHistory,
  onGlobalPanelOpen,
  onDebugRun,
  onToggleChecklist,
  onRightPanelResizeStart,
  onValidationIssueFocus,
  onWorkflowDiffFocus,
  onPublish,
  onRollbackVersion,
  onArchiveVersion,
}: Props) {
  const [showZoomMenu, setShowZoomMenu] = useState(false);
  const [showActivityPicker, setShowActivityPicker] = useState(false);
  const [showVersionPanel, setShowVersionPanel] = useState(false);
  const [showPublishConfirm, setShowPublishConfirm] = useState(false);
  const [rollbackTargetVersionId, setRollbackTargetVersionId] = useState('');
  const [archiveTargetVersionId, setArchiveTargetVersionId] = useState('');
  const [versionStatusFilter, setVersionStatusFilter] = useState<'ALL' | WorkflowVersion['status']>('ALL');
  const [versionActionMessage, setVersionActionMessage] = useState('');
  const [compareLeftId, setCompareLeftId] = useState('');
  const [compareRightId, setCompareRightId] = useState('');
  const [activitySearch, setActivitySearch] = useState('');
  const [showTestActivities, setShowTestActivities] = useState(false);
  const [pickerWorkspaceId, setPickerWorkspaceId] = useState('');
  const [pickerFolderId, setPickerFolderId] = useState('');
  const [activityPickerPage, setActivityPickerPage] = useState(0);
  const zoomOptions = [200, 100, 75, 50, 25];
  const chooseZoom = (value: number) => {
    onZoomTo(value / 100);
    setShowZoomMenu(false);
  };
  useEffect(() => {
    if (selectedNodeCount > 0) setShowVersionPanel(false);
  }, [selectedNodeCount]);
  const activeActivity = activities.find((item) => item.id === activityId);
  const activeVersion = workflowVersions.find((item) => item.status === 'ACTIVE');
  const compareLeft = workflowVersions.find((item) => item.id === (compareLeftId || workflowVersions[0]?.id));
  const compareRight = workflowVersions.find((item) => item.id === (compareRightId || workflowVersions[1]?.id));
  const rollbackTargetVersion = workflowVersions.find((item) => item.id === rollbackTargetVersionId);
  const archiveTargetVersion = workflowVersions.find((item) => item.id === archiveTargetVersionId);
  const filteredWorkflowVersions = workflowVersions.filter((version) => (
    versionStatusFilter === 'ALL' || version.status === versionStatusFilter
  ));
  const versionCompare = workflowDiffSummaryLocal(compareLeft?.definition, compareRight?.definition, '请选择两个历史版本');
  const activePlacement = activityId ? placements[activityId] : undefined;
  const activeWorkspace = activePlacement ? workspaces.find((workspace) => workspace.id === activePlacement.workspaceId) : undefined;
  const activeFolder = activePlacement ? folders.find((folder) => folder.id === activePlacement.folderId) : undefined;
  const validationBlockCount = workflowValidation?.summary.blockCount ?? 0;
  const validationWarningCount = workflowValidation?.summary.warningCount ?? 0;
  const checklistItems = useMemo(() => workflowValidation ? buildChecklistItems(workflowValidation) : CHECKLIST_DEFINITIONS.map((item) => ({ ...item, status: 'unchecked' as ChecklistStatus, count: 0 })), [workflowValidation]);
  const checklistBadge = validatingWorkflow
    ? '...'
    : workflowValidation
      ? workflowValidation.valid ? 'PASS' : String(validationBlockCount)
      : '-';
  const checklistStatus = validatingWorkflow
    ? '检查中'
    : workflowValidation
      ? workflowValidation.valid ? 'PASS' : `${validationBlockCount} 阻塞 / ${validationWarningCount} 警告`
      : '未检查';
  const activeWorkspaces = useMemo(() => workspaces.filter((workspace) => workspace.status !== 'DELETED'), [workspaces]);
  const folderOptions = useMemo(() => (
    folders
      .filter((folder) => folder.status !== 'DELETED')
      .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name))
  ), [folders]);
  const workspaceWorkflowCount = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      if (!showTestActivities && isTestActivity(activity)) return;
      const placement = placements[activity.id];
      if (!placement?.workspaceId) return;
      next[placement.workspaceId] = (next[placement.workspaceId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements, showTestActivities]);
  const folderWorkflowCount = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      if (!showTestActivities && isTestActivity(activity)) return;
      const placement = placements[activity.id];
      if (!placement?.folderId) return;
      next[placement.folderId] = (next[placement.folderId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements, showTestActivities]);
  const selectedPickerWorkspaceId = pickerWorkspaceId || activePlacement?.workspaceId || activeWorkspaces[0]?.id || '';
  const selectedPickerFolders = folderOptions.filter((folder) => folder.workspaceId === selectedPickerWorkspaceId);
  const selectedPickerFolderId = pickerFolderId || activePlacement?.folderId || selectedPickerFolders[0]?.id || '';
  const selectedPickerWorkspace = activeWorkspaces.find((workspace) => workspace.id === selectedPickerWorkspaceId);
  const selectedPickerFolder = selectedPickerFolders.find((folder) => folder.id === selectedPickerFolderId);
  const activityOptions = useMemo(() => {
    const keyword = activitySearch.trim().toLowerCase();
    return activities
      .filter((activity) => showTestActivities || !isTestActivity(activity))
      .filter((activity) => {
        const placement = placements[activity.id];
        if (selectedPickerWorkspaceId && placement?.workspaceId !== selectedPickerWorkspaceId) return false;
        if (selectedPickerFolderId && placement?.folderId !== selectedPickerFolderId) return false;
        if (!keyword) return true;
        const workspace = activeWorkspaces.find((item) => item.id === placement?.workspaceId);
        const folder = folderOptions.find((item) => item.id === placement?.folderId);
        return `${activity.activityName} ${activity.activityCode} ${activity.tenantId} ${workspace?.name ?? ''} ${folder?.name ?? ''}`.toLowerCase().includes(keyword);
      });
  }, [activities, activitySearch, activeWorkspaces, folderOptions, placements, selectedPickerFolderId, selectedPickerWorkspaceId, showTestActivities]);
  const activityPickerPageSize = 10;
  const activityPickerPageCount = Math.max(1, Math.ceil(activityOptions.length / activityPickerPageSize));
  const activityPickerSafePage = Math.min(activityPickerPage, activityPickerPageCount - 1);
  const pagedActivityOptions = activityOptions.slice(
    activityPickerSafePage * activityPickerPageSize,
    activityPickerSafePage * activityPickerPageSize + activityPickerPageSize,
  );
  useEffect(() => {
    setActivityPickerPage(0);
  }, [activitySearch, selectedPickerFolderId, selectedPickerWorkspaceId, showTestActivities]);
  useEffect(() => {
    if (activityPickerPage !== activityPickerSafePage) setActivityPickerPage(activityPickerSafePage);
  }, [activityPickerPage, activityPickerSafePage]);
  const hiddenTestCount = activities.filter(isTestActivity).length;
  const saveBadgeText = draftSaveState === 'saving'
    ? '保存中'
    : draftSaveState === 'failed'
      ? '保存失败'
      : draftDirty || draftSaveState === 'dirty'
        ? '未保存'
        : draftSaveState === 'saved'
          ? lastSavedAt ? `已保存 ${new Date(lastSavedAt).toLocaleTimeString()}` : '已保存'
          : '未保存';
  const saveBadgeClass = draftSaveState === 'failed'
    ? 'failed'
    : draftSaveState === 'saving'
      ? 'saving'
      : draftDirty || draftSaveState === 'dirty'
        ? 'dirty'
        : draftSaveState === 'saved'
          ? 'saved'
          : 'idle';
  const publishBadgeText = publishState === 'publishing'
    ? '发布中'
    : publishState === 'failed'
      ? '发布失败'
      : publishState === 'published'
        ? '已发布'
        : isPublished ? '已发布' : '未发布';
  const publishBadgeClass = publishState === 'failed'
    ? 'failed'
    : publishState === 'publishing'
      ? 'saving'
      : isPublished || publishState === 'published'
        ? 'saved'
        : 'idle';

  return (
    <>
      <div className="workspace-topbar">
        <div className="workspace-brand">
          <BrandMark />
          <span>/</span>
          {homeMode ? (
            <div className="workspace-section-title">Activity Studio</div>
          ) : (
            <>
              <button
                className="activity-current"
                title={activityLabel}
                onClick={() => setShowActivityPicker((value) => !value)}
              >
                <b>{activeActivity ? activeActivity.activityName || activeActivity.activityCode : '选择活动'}</b>
                <small>{activeActivity ? activeActivity.activityCode : '未绑定'}</small>
              </button>
              {activeActivity ? <em className="activity-status">{activeActivity.status || 'UNKNOWN'} · v{activeActivity.version ?? '-'}</em> : null}
              {activeActivity ? <button className="activity-info-button" type="button" onClick={onActivitySettings}>活动信息</button> : null}
            </>
          )}
          {!homeMode && showActivityPicker ? (
            <div className="activity-picker-popover">
              <div className="activity-picker-head">
                <div>
                  <b>切换流程</b>
                  <span>选择工作区、文件夹或流程</span>
                </div>
                <div className="activity-picker-head-actions">
                  <a href="/admin/">创建活动</a>
                  <button type="button" aria-label="关闭流程切换" onClick={() => setShowActivityPicker(false)}>×</button>
                </div>
              </div>
              <div className="activity-picker-search">
                <input value={activitySearch} placeholder="搜索工作区、文件夹或流程" onChange={(event) => setActivitySearch(event.target.value)} />
              </div>
              <div className="activity-picker-workspace-view">
                <aside className="activity-picker-tree">
                  {activeWorkspaces.length ? activeWorkspaces.map((workspace) => {
                    const workspaceFolders = folderOptions.filter((folder) => folder.workspaceId === workspace.id);
                    return (
                      <div key={workspace.id} className="activity-picker-tree-group">
                        <button
                          className={workspace.id === selectedPickerWorkspaceId ? 'active workspace' : 'workspace'}
                          onClick={() => {
                            setPickerWorkspaceId(workspace.id);
                            setPickerFolderId(workspaceFolders[0]?.id ?? '');
                          }}
                        >
                          <i>▦</i>
                          <span>
                            <b>{workspace.name}</b>
                            <em>{workspace.slug}</em>
                          </span>
                          <strong>{workspaceWorkflowCount[workspace.id] ?? 0}</strong>
                        </button>
                        {workspace.id === selectedPickerWorkspaceId ? (
                          <div className="activity-picker-folder-list">
                            {workspaceFolders.length ? workspaceFolders.map((folder) => (
                              <button
                                key={folder.id}
                                className={folder.id === selectedPickerFolderId ? 'active folder' : 'folder'}
                                onClick={() => setPickerFolderId(folder.id)}
                              >
                                <i>▸</i>
                                <span>
                                  <b>{folder.name}</b>
                                  <em>{folder.visibility}</em>
                                </span>
                                <strong>{folderWorkflowCount[folder.id] ?? 0}</strong>
                              </button>
                            )) : <div className="activity-picker-empty compact">暂无文件夹</div>}
                          </div>
                        ) : null}
                      </div>
                    );
                  }) : <div className="activity-picker-empty">暂无工作区</div>}
                </aside>
                <section className="activity-picker-workflow-pane">
                  <header>
                    <span>
                      <b>{selectedPickerFolder?.name ?? selectedPickerWorkspace?.name ?? '流程'}</b>
                      <em>{selectedPickerWorkspace?.name ?? '选择工作区'}</em>
                    </span>
                    <strong>{activityOptions.length}</strong>
                  </header>
                  <div className="activity-picker-workflow-columns">
                    <span>流程</span>
                    <span>编码</span>
                    <span>版本</span>
                  </div>
                  <div className="activity-picker-workflow-list">
                    {pagedActivityOptions.length ? pagedActivityOptions.map((activity) => (
                    <button
                      key={activity.id}
                      className={activity.id === activityId ? 'active' : ''}
                      onClick={() => {
                        onActivityChange(activity.id);
                        setShowActivityPicker(false);
                      }}
                    >
                      <i>◇</i>
                      <span>
                        <b>{activity.activityName || activity.activityCode}</b>
                        <em>{activity.tenantId}</em>
                      </span>
                      <code>{activity.activityCode}</code>
                      <strong>{activity.status || 'UNKNOWN'} · v{activity.version ?? '-'}</strong>
                    </button>
                    )) : <div className="activity-picker-empty">没有匹配流程。</div>}
                  </div>
                  <footer className="activity-picker-workflow-pager">
                    <span>
                      显示 {activityOptions.length ? `${activityPickerSafePage * activityPickerPageSize + 1}-${Math.min(activityOptions.length, (activityPickerSafePage + 1) * activityPickerPageSize)}` : '0'}
                      <b>共 {activityOptions.length}</b>
                    </span>
                    <div className="activity-picker-page-buttons">
                      <button type="button" aria-label="上一页" title="上一页" disabled={activityPickerSafePage === 0} onClick={() => setActivityPickerPage((page) => Math.max(0, page - 1))}>‹</button>
                      <button type="button" aria-label="下一页" title="下一页" disabled={activityPickerSafePage >= activityPickerPageCount - 1} onClick={() => setActivityPickerPage((page) => Math.min(activityPickerPageCount - 1, page + 1))}>›</button>
                    </div>
                  </footer>
                </section>
              </div>
              <footer className="activity-picker-footer">
                {hiddenTestCount > 0 ? (
                  <label className="activity-picker-toggle">
                    <input type="checkbox" checked={showTestActivities} onChange={(event) => setShowTestActivities(event.target.checked)} />
                    <span>显示 {hiddenTestCount} 个测试活动</span>
                  </label>
                ) : <span />}
              </footer>
            </div>
          ) : null}
        </div>
        <nav className="app-tabs">
          <button className="active">编排</button>
          <button>访问 API</button>
          <button>日志与标注</button>
          <button>监测</button>
        </nav>
        <div className="workspace-actions">
          <button>插件</button>
          <span>D</span>
        </div>
      </div>
      {homeMode ? null : (
        <>
      <div className="dify-top-status">
        <div className="workflow-title-row">
          <button className="workflow-title-button" type="button" onClick={() => setShowActivityPicker((value) => !value)}>
            {activeActivity ? activeActivity.activityName || activeActivity.activityCode : '流程'}
          </button>
          <button className="workflow-edit-button" type="button" onClick={onActivitySettings} title="编辑活动信息">✎</button>
        </div>
        <div className="workflow-state-line">
          <span className="workflow-state-pill neutral" title={workflowVersionLabel}>{activeVersion ? `DRAFT · v${activeVersion.version + 1}` : 'DRAFT · v1'}</span>
          <span className={`workflow-state-pill ${saveBadgeClass}`} title={draftSaveError || workflowStatus}>{saveBadgeText}</span>
          <span className={`workflow-state-pill ${publishBadgeClass}`} title={publishError || workflowVersionLabel}>{publishBadgeText}</span>
          <span className="workflow-state-pill neutral">{workspaceRole}</span>
        </div>
        <div className="workflow-breadcrumb" title={workflowStatus}>
          {activeWorkspace ? <button onClick={() => onWorkspaceNavigate(activeWorkspace.id)}>{activeWorkspace.name}</button> : <span>工作区</span>}
          <i>/</i>
          {activeWorkspace && activeFolder ? <button onClick={() => onWorkspaceNavigate(activeWorkspace.id, activeFolder.id)}>{activeFolder.name}</button> : <span>文件夹</span>}
          <i>/</i>
          <button onClick={() => setShowActivityPicker((value) => !value)}>{activeActivity ? activeActivity.activityName || activeActivity.activityCode : '流程'}</button>
        </div>
        {(draftSaveError || publishError) ? (
          <div className="workflow-error-detail">
            <span>{draftSaveError || publishError}</span>
            {draftSaveError ? <button type="button" onClick={onSaveDraft}>重试保存</button> : null}
          </div>
        ) : null}
      </div>
      <div className="dify-float-toolbar">
        <button disabled={!canEditWorkflow} onClick={onDefault}>完整链路</button>
        <button disabled={!canEditWorkflow} onClick={onSimple}>简化链路</button>
        <button disabled={!canEditWorkflow} onClick={onTemplates}>模板</button>
        <button disabled={!canEditWorkflow} onClick={onAutoLayout}>自动排齐</button>
        <button disabled={!canEditWorkflow || draftSaveState === 'saving'} title="Cmd/Ctrl+S" onClick={onSaveDraft}>{draftSaveState === 'saving' ? '保存中' : '保存草稿'}</button>
        <button
          className="debug-action"
          disabled={!canEditWorkflow}
          onClick={() => {
            onGlobalPanelOpen();
            setShowVersionPanel(false);
            if (showChecklist) onToggleChecklist();
            if (runHistoryOpen) onCloseRunHistory();
            onDebugRun();
          }}
        >
          调试
        </button>
        <button
          onClick={() => {
            onGlobalPanelOpen();
            setShowVersionPanel(false);
            if (showChecklist) onToggleChecklist();
            onRunHistory();
          }}
        >
          运行历史
        </button>
        <button
          className="version-action"
          onClick={() => {
            onGlobalPanelOpen();
            if (showChecklist) onToggleChecklist();
            if (runHistoryOpen) onCloseRunHistory();
            setShowVersionPanel((value) => {
              const next = !value;
              return next;
            });
          }}
        >
          版本
        </button>
        <button
          className="checklist-action"
          onClick={() => {
            onGlobalPanelOpen();
            setShowVersionPanel(false);
            if (runHistoryOpen) onCloseRunHistory();
            onToggleChecklist();
          }}
        >
          检查清单 <span>{checklistBadge}</span>
        </button>
        <button className="preview-action" onClick={onPreview}>▷ 预览</button>
        <button className="publish-action" disabled={!canPublishWorkflow || publishState === 'publishing'} title={!canPublishWorkflow ? '当前角色不能发布' : '打开发布确认'} onClick={() => setShowPublishConfirm(true)}>{publishState === 'publishing' ? '发布中' : '发布'}</button>
      </div>
      {showPublishConfirm ? (
        <PublishConfirmModal
          activeVersion={activeVersion}
          draftActiveDiff={draftActiveDiff}
          draftDirty={draftDirty}
          validatingWorkflow={validatingWorkflow}
          workflowValidation={workflowValidation}
          publishState={publishState}
          onClose={() => setShowPublishConfirm(false)}
          onConfirm={(releaseNote) => {
            setShowPublishConfirm(false);
            onPublish(releaseNote);
          }}
        />
      ) : null}
      {rollbackTargetVersion ? (
        <RollbackConfirmModal
          activeVersion={activeVersion}
          targetVersion={rollbackTargetVersion}
          onClose={() => setRollbackTargetVersionId('')}
          onConfirm={(note) => {
            setRollbackTargetVersionId('');
            setVersionActionMessage(`已提交回滚请求：v${rollbackTargetVersion.version} · ${note || '无备注'}`);
            onRollbackVersion(rollbackTargetVersion.id, note);
          }}
        />
      ) : null}
      {archiveTargetVersion ? (
        <ArchiveConfirmModal
          activeVersion={activeVersion}
          version={archiveTargetVersion}
          onClose={() => setArchiveTargetVersionId('')}
          onConfirm={(note) => {
            setArchiveTargetVersionId('');
            setVersionActionMessage(`已提交归档请求：v${archiveTargetVersion.version} · ${note || '无备注'}`);
            onArchiveVersion(archiveTargetVersion.id, note);
          }}
        />
      ) : null}
      {showVersionPanel ? (
        <div className="version-popover">
          <button className="right-panel-resize-handle" type="button" aria-label="调整版本面板宽度" onPointerDown={onRightPanelResizeStart} />
          <div className="version-head">
            <div>
              <b>版本管理</b>
              <em>{activeVersion ? `ACTIVE · v${activeVersion.version}` : '未发布'}</em>
            </div>
            <button type="button" aria-label="关闭版本管理" onClick={() => setShowVersionPanel(false)}>×</button>
          </div>
          <section className="version-section">
            <h3>Draft vs Active</h3>
            <div className={`version-draft-card ${draftActiveDiff.changed ? 'changed' : 'clean'}`}>
              <b>{draftDirty ? 'Draft 有未保存变更' : 'Draft 已保存'}</b>
              <span>{draftActiveDiff.summary}</span>
              {draftActiveDiff.detail.slice(0, 4).map((item) => <em key={item}>{item}</em>)}
            </div>
          </section>
          {versionActionMessage ? (
            <section className="version-section">
              <h3>版本操作记录</h3>
              <div className="version-audit-card">{versionActionMessage}</div>
            </section>
          ) : null}
          <section className="version-section">
            <h3>Diff 明细</h3>
            <WorkflowDiffList items={draftActiveDiff.items} onFocus={onWorkflowDiffFocus} />
          </section>
          <section className="version-section">
            <h3>Active Published Version</h3>
            {activeVersion ? <VersionCard version={activeVersion} active /> : <div className="version-empty">当前没有 active version。</div>}
          </section>
          <section className="version-section">
            <h3>历史 Versions</h3>
            <div className="version-filter-tabs">
              {(['ALL', 'DRAFT', 'ACTIVE', 'SUPERSEDED', 'ARCHIVED'] as const).map((status) => (
                <button
                  key={status}
                  className={versionStatusFilter === status ? 'active' : ''}
                  type="button"
                  onClick={() => setVersionStatusFilter(status)}
                >
                  {status === 'ALL' ? '全部' : status}
                </button>
              ))}
            </div>
            <div className="version-list">
              {filteredWorkflowVersions.length ? filteredWorkflowVersions.map((version) => (
                <VersionCard
                  key={version.id}
                  version={version}
                  active={version.status === 'ACTIVE'}
                  onRollback={canPublishWorkflow && version.status !== 'ACTIVE' ? () => setRollbackTargetVersionId(version.id) : undefined}
                  onArchive={canPublishWorkflow && version.status !== 'ACTIVE' ? () => setArchiveTargetVersionId(version.id) : undefined}
                />
              )) : <div className="version-empty">当前筛选下没有版本。</div>}
            </div>
          </section>
          <section className="version-section">
            <h3>版本对比</h3>
            <div className="version-compare-selects">
              <select value={compareLeft?.id ?? ''} onChange={(event) => setCompareLeftId(event.target.value)}>
                {workflowVersions.map((version) => <option key={version.id} value={version.id}>v{version.version} · {version.status}</option>)}
              </select>
              <select value={compareRight?.id ?? ''} onChange={(event) => setCompareRightId(event.target.value)}>
                {workflowVersions.map((version) => <option key={version.id} value={version.id}>v{version.version} · {version.status}</option>)}
              </select>
            </div>
            <div className={`version-diff-card ${versionCompare.changed ? 'changed' : 'clean'}`}>
              <b>{compareLeft && compareRight ? `v${compareLeft.version} vs v${compareRight.version}` : '选择两个版本'}</b>
              <span>{versionCompare.summary}</span>
              {versionCompare.detail.slice(0, 6).map((item) => <em key={item}>{item}</em>)}
            </div>
            <WorkflowDiffList items={versionCompare.items} onFocus={onWorkflowDiffFocus} />
          </section>
        </div>
      ) : null}
      {canEditWorkflow ? <button
        className={`node-library-fab ${showPalette ? 'open' : ''}`}
        title={showPalette ? '收起节点库' : '展开节点库'}
        onClick={onTogglePalette}
      >
        <span>{showPalette ? '×' : '+'}</span>
        <b>{showPalette ? '收起' : '节点'}</b>
      </button> : null}
      <div className="dify-left-rail">
        <button className={interactionMode === 'pointer' ? 'active' : ''} title="指针模式" onClick={() => onModeChange('pointer')}>⌖</button>
        <button className={interactionMode === 'hand' ? 'active' : ''} title="手模式" onClick={() => onModeChange('hand')}>☝</button>
        <button title="整理节点" disabled={!canEditWorkflow || selectedNodeCount < 2} onClick={onGroup}>▦</button>
        <button title="最大化画布" onClick={onFitView}>□</button>
        <button title="删除选中" disabled={!canEditWorkflow || selectedCount === 0} onClick={onDelete}>⌫</button>
      </div>
      <div className="zoom-toolbar">
        <button onClick={onZoomOut}>-</button>
        <button className="zoom-percent-button" onClick={() => setShowZoomMenu((value) => !value)}>{zoomPercent}%</button>
        <button onClick={onZoomIn}>+</button>
        {showZoomMenu ? (
          <div className="zoom-menu">
            {zoomOptions.map((value) => (
              <button key={value} className={zoomPercent === value ? 'active' : ''} onClick={() => chooseZoom(value)}>
                <span>{value}%</span>
                {value === 100 ? <kbd>⇧ 1</kbd> : value === 50 ? <kbd>⇧ 5</kbd> : null}
              </button>
            ))}
            <button onClick={() => { onFitView(); setShowZoomMenu(false); }}>
              <span>Zoom to Fit</span>
              <kbd>⌘ 1</kbd>
            </button>
            <button onClick={onToggleMiniMap}>
              <span>{showMiniMap ? '✓' : ''} 画布缩略图</span>
            </button>
          </div>
        ) : null}
      </div>
      {showChecklist ? (
        <div className="checklist-modal-layer">
        <div className="checklist-popover">
          <button className="right-panel-resize-handle" type="button" aria-label="调整检查清单面板宽度" onPointerDown={onRightPanelResizeStart} />
          <div className="checklist-head">
            <div>
              <b>发布检查清单</b>
              <em>来自后端 workflow validation 的实时结果</em>
            </div>
            <span>{checklistStatus}</span>
            <button type="button" aria-label="关闭检查清单" onClick={onToggleChecklist}>×</button>
          </div>
          <div className="checklist-body">
          {validatingWorkflow ? (
            <div className="checklist-empty">正在保存当前画布并执行校验...</div>
          ) : workflowValidation ? (
            <>
              <div className="checklist-summary">
                {checklistItems.map((item) => (
                  <div key={item.id} className={`checklist-summary-row status-${item.status}`}>
                    <i>{statusIcon(item.status)}</i>
                    <span>
                      <b>{item.title}</b>
                      <em>{item.description}</em>
                    </span>
                    <strong>{statusLabel(item.status, item.count)}</strong>
                  </div>
                ))}
              </div>
              {workflowValidationItems(workflowValidation).length > 0 ? workflowValidationItems(workflowValidation).map((item) => (
                <ValidationIssueRow
                  key={`${item.code}-${item.nodeId ?? ''}-${item.edgeId ?? ''}-${item.path ?? ''}`}
                  item={item}
                  onFocus={onValidationIssueFocus}
                />
              )) : (
                <div className="checklist-pass">
                  <b>全部检查通过</b>
                  <span>当前 draft 可以发布。</span>
                </div>
              )}
            </>
          ) : (
            <>
              <div className="checklist-summary">
                {checklistItems.map((item) => (
                  <div key={item.id} className="checklist-summary-row status-unchecked">
                    <i>-</i>
                    <span>
                      <b>{item.title}</b>
                      <em>{item.description}</em>
                    </span>
                    <strong>未检查</strong>
                  </div>
                ))}
              </div>
              <div className="checklist-empty">点击检查清单后，会保存当前画布并展示后端真实校验结果。</div>
            </>
          )}
          </div>
        </div>
        </div>
      ) : null}
      <header className="studio-head">
        <div>
          <h1>活动数据流设计器</h1>
          <p>事件进入、规则判断、奖励计算、额度扣减、发奖、回调、审计和重试。</p>
        </div>
        <div className="head-actions">
          {lastAdded ? <span className="status-pill green">已添加：{lastAdded}</span> : null}
          <span className="status-pill">节点 {nodeCount}</span>
          <span className="status-pill">连线 {edgeCount}</span>
          <button className="ghost-action" disabled={!canEditWorkflow || selectedNodeCount < 2} onClick={onGroup}>组合选中</button>
          <button className="ghost-action" disabled={!canEditWorkflow} onClick={onAutoLayout}>自动排齐</button>
          <button className="ghost-action" disabled={!canEditWorkflow} onClick={onClear}>清空画布</button>
          <button className="danger-action" disabled={!canEditWorkflow || selectedCount === 0} onClick={onDelete}>删除选中 {selectedCount > 0 ? selectedCount : ''}</button>
        </div>
      </header>
        </>
      )}
    </>
  );
}

function isTestActivity(activity: ActivityOption): boolean {
  const text = `${activity.activityName} ${activity.activityCode} ${activity.id}`.toLowerCase();
  return ['postgresql e2e', 'load fault', 'smoke', 'e2e_', 'load_fault', 'pg_smoke'].some((marker) => text.includes(marker));
}

function ValidationIssueRow({ item, onFocus }: { item: WorkflowValidationIssue; onFocus: (item: WorkflowValidationIssue) => void }) {
  const target = [item.nodeId ? `node ${item.nodeId}` : '', item.edgeId ? `edge ${item.edgeId}` : '', item.path ?? ''].filter(Boolean).join(' · ');
  const canFocus = Boolean(item.nodeId || item.edgeId);
  return (
    <button
      type="button"
      className={`checklist-row validation-${item.severity.toLowerCase()}`}
      disabled={!canFocus}
      onClick={() => onFocus(item)}
    >
      <i>{item.severity === 'BLOCK' ? '!' : '?'}</i>
      <span>
        <b>{item.code}</b>
        <em>{item.message}</em>
        {target ? <small>{target}</small> : null}
      </span>
    </button>
  );
}
