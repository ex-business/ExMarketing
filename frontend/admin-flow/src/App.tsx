import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  addEdge,
  Background,
  Controls,
  MarkerType,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
} from '@xyflow/react';
import type { Connection, ReactFlowInstance } from '@xyflow/react';
import { ActivityNodeView } from './components/ActivityNodeView';
import { ContextMenu } from './components/ContextMenu';
import { GroupNodeView, NoteNodeView } from './components/FlowExtraNodes';
import { NodePalette } from './components/NodePalette';
import { NodePicker } from './components/NodePicker';
import { PropertyPanel } from './components/PropertyPanel';
import { StudioChrome } from './components/StudioChrome';
import { baseEdges, baseNodes, simpleTemplate } from './data/initialFlow';
import { getDef, nodeCatalog } from './data/nodeCatalog';
import type {
  ActivityEdge,
  ActivityNode,
  ActivityNodeData,
  AlignmentGuide,
  FlowContextMenu,
  FlowNode,
  GroupNode,
  NodePickerState,
  NoteNode,
  OutputAddRequest,
  WorkflowDefinition,
  WorkflowExecution,
  WorkflowNodeConfigSchema,
  WorkflowNodeExecution,
  WorkflowValidationIssue,
  WorkflowValidationResult,
  WorkflowVersion,
} from './types';
import { ACTIVITY_NODE_WIDTH, activityNodeHeight, cx, edge, flowNode, layoutFlowNodes } from './utils/flow';

type ActivitySummary = {
  id: string;
  workspaceId?: string;
  folderId?: string;
  activityCode: string;
  activityName: string;
  tenantId: string;
  status?: string;
  version?: number;
};

type WorkspaceSummary = {
  id: string;
  tenantId: string;
  name: string;
  slug: string;
  color?: string;
  visibility: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  status: string;
};

type WorkspaceFolderSummary = {
  id: string;
  workspaceId: string;
  parentId?: string;
  name: string;
  color?: string;
  visibility: string;
  sortOrder: number;
  status: string;
};

type WorkspaceShareLinkSummary = {
  id: string;
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY';
  enabled: boolean;
};

type ActivityPlacement = {
  workspaceId: string;
  folderId: string;
  shareLinkId?: string;
};

type CreateStudioWorkspaceInput = {
  name: string;
  tenantId: string;
};

type CreateStudioActivityInput = {
  tenantId: string;
  activityName: string;
  activityCode: string;
  workspaceId: string;
  folderId: string;
};

function isTestActivity(activity: ActivitySummary): boolean {
  const text = `${activity.activityName} ${activity.activityCode} ${activity.id}`.toLowerCase();
  return [
    'postgresql e2e',
    'load fault',
    'smoke',
    'e2e_',
    'load_fault',
    'pg_smoke',
  ].some((marker) => text.includes(marker));
}

function normalizeActivityCode(value: string): string {
  const code = value
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[^A-Za-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toUpperCase();
  return code || `ACTIVITY_${Date.now().toString(36).toUpperCase()}`;
}

function buildStudioActivityPayload(input: CreateStudioActivityInput) {
  const now = new Date();
  const start = new Date(now.getTime() - 60_000).toISOString();
  const end = new Date(now.getTime() + 365 * 24 * 60 * 60 * 1000).toISOString();
  const activityName = input.activityName.trim();
  const activityCode = normalizeActivityCode(input.activityCode || activityName);
  return {
    operator: 'studio',
    tenantId: input.tenantId.trim() || 'site_001',
    workspaceId: input.workspaceId,
    folderId: input.folderId,
    activityCode,
    activityName: activityName || activityCode,
    activityType: 'STUDIO_WORKFLOW',
    status: 'DRAFT',
    triggerEvents: ['STUDIO_EVENT'],
    startTime: start,
    endTime: end,
    timezone: 'UTC',
    priority: 100,
    exclusiveGroup: '',
    stackable: true,
    version: 1,
    config: {
      entry: {
        channels: ['APP', 'WEB'],
        entranceCodes: ['studio'],
        landingUrl: 'https://example.com/studio',
      },
      display: {
        title: activityName || activityCode,
        visibleFrom: start,
        visibleTo: end,
        sort: 100,
      },
      audience: {
        segmentCodes: ['all_users'],
      },
      metadata: {
        owner: 'studio',
        workspaceId: input.workspaceId,
        folderId: input.folderId,
      },
    },
    rules: [
      {
        ruleCode: 'STUDIO_DEFAULT_RULE',
        rulePhase: 'QUALIFY',
        ruleType: 'CONDITION',
        ruleConfig: {
          logic: 'AND',
          conditions: [
            { field: 'attributes.eventMatched', op: '=', value: true },
          ],
        },
        sort: 1,
        enabled: true,
        version: 1,
      },
    ],
    rewards: [
      {
        rewardCode: 'STUDIO_DEFAULT_REWARD',
        rewardType: 'POINT',
        calculateType: 'FIXED',
        rewardConfig: {
          points: 1,
          unit: 'POINT',
          pointType: 'DEFAULT',
          targetRef: 'https://wallet.example.com/reward',
        },
        grantMode: 'AUTO',
        grantChannel: 'HTTP',
        enabled: true,
        version: 1,
      },
    ],
  };
}

function serializeWorkflowDefinition(
  nodes: FlowNode[],
  edges: ActivityEdge[],
  nodeConfigSchemas: WorkflowNodeConfigSchema[],
  viewport?: { x: number; y: number; zoom: number },
  context?: { workspaceId?: string; folderId?: string; activityId?: string },
): WorkflowDefinition {
  const schemaByNodeType = new Map(nodeConfigSchemas.map((schema) => [schema.nodeType, schema]));
  return {
    nodes: nodes
      .filter((node): node is ActivityNode => node.type === 'activity')
      .map((node) => {
        const schema = schemaByNodeType.get(node.data.id);
        return {
          id: node.id,
          type: node.data.id,
          kind: node.data.kind,
          code: node.data.code,
          title: node.data.title,
          position: { x: node.position.x, y: node.position.y },
          inputs: node.data.inputs,
          outputs: node.data.outputs,
          config: materializeNodeConfig(node.data.config, schema),
        };
      }),
    edges: edges.map((item) => ({
      id: item.id,
      source: item.source,
      target: item.target,
      sourceHandle: item.sourceHandle ?? undefined,
      targetHandle: item.targetHandle ?? undefined,
      label: typeof item.label === 'string' ? item.label : undefined,
    })),
    viewport,
    meta: {
      app: 'ExMarketing',
      schema: 'activity-workflow/v1',
      exportedAt: new Date().toISOString(),
      ...(context ?? {}),
    },
  };
}

function materializeNodeConfig(config: Record<string, unknown> | undefined, schema: WorkflowNodeConfigSchema | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  if (!schema) {
    return { ...(config ?? {}) };
  }
  schema.fields.forEach((field) => {
    if (config && Object.prototype.hasOwnProperty.call(config, field.key)) {
      out[field.key] = config[field.key];
      return;
    }
    if (Object.prototype.hasOwnProperty.call(field, 'default')) {
      out[field.key] = field.default;
      return;
    }
    switch (field.type) {
      case 'BOOLEAN':
        out[field.key] = false;
        break;
      case 'NUMBER':
        out[field.key] = 0;
        break;
      case 'ARRAY':
        out[field.key] = [];
        break;
      case 'OBJECT':
        out[field.key] = {};
        break;
      default:
        out[field.key] = '';
        break;
    }
  });
  Object.entries(config ?? {}).forEach(([key, value]) => {
    if (!Object.prototype.hasOwnProperty.call(out, key)) {
      out[key] = value;
    }
  });
  return out;
}

function hydrateWorkflowDefinition(definition: WorkflowDefinition): { nodes: FlowNode[]; edges: ActivityEdge[] } {
  return {
    nodes: definition.nodes.map((node) => {
      const base = getDef(node.type);
      return flowNode(node.type, node.position?.x ?? 80, node.position?.y ?? 120, {
        id: node.id,
        data: {
          ...base,
          id: node.type,
          code: node.code ?? base.code,
          title: node.title ?? base.title,
          kind: (node.kind as ActivityNode['data']['kind']) ?? base.kind,
          inputs: node.inputs ?? base.inputs,
          outputs: node.outputs ?? base.outputs,
          config: isRecord(node.config) ? node.config : undefined,
        },
      });
    }),
    edges: definition.edges.map((item) => edge(item.source, item.target, item.label ?? '', item.sourceHandle, item.targetHandle)),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      'content-type': 'application/json',
      ...(init?.headers ?? {}),
    },
  });
  const contentType = response.headers.get('content-type') ?? '';
  if (!contentType.includes('application/json')) {
    throw new Error('接口不可用');
  }
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error ?? body.message ?? `${response.status} ${response.statusText}`);
  }
  return response.json() as Promise<T>;
}

function StudioActivityStart({
  activities,
  workspaces,
  folders,
  placements,
  hiddenTestActivityCount,
  workflowStatus,
  initialWorkspaceId,
  initialFolderId,
  onSelectActivity,
  onCreateWorkspace,
  onCreateActivity,
  onCreateFolder,
}: {
  activities: ActivitySummary[];
  workspaces: WorkspaceSummary[];
  folders: WorkspaceFolderSummary[];
  placements: Record<string, ActivityPlacement>;
  hiddenTestActivityCount: number;
  workflowStatus: string;
  initialWorkspaceId: string;
  initialFolderId: string;
  onSelectActivity: (activityId: string) => void;
  onCreateWorkspace: (input: CreateStudioWorkspaceInput) => Promise<WorkspaceSummary>;
  onCreateActivity: (input: CreateStudioActivityInput) => Promise<void>;
  onCreateFolder: (workspaceId: string, name: string) => Promise<WorkspaceFolderSummary>;
}) {
  const [search, setSearch] = useState('');
  const [workspaceName, setWorkspaceName] = useState('');
  const [workspaceTenant, setWorkspaceTenant] = useState('site_001');
  const [workspaceError, setWorkspaceError] = useState('');
  const [creatingWorkspace, setCreatingWorkspace] = useState(false);
  const [showWorkspaceDialog, setShowWorkspaceDialog] = useState(false);
  const [folderName, setFolderName] = useState('');
  const [folderError, setFolderError] = useState('');
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [showFolderDialog, setShowFolderDialog] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createCode, setCreateCode] = useState('');
  const [createTenant, setCreateTenant] = useState('site_001');
  const [createWorkspaceId, setCreateWorkspaceId] = useState('');
  const [createFolderId, setCreateFolderId] = useState('');
  const [createFolderName, setCreateFolderName] = useState('');
  const [createError, setCreateError] = useState('');
  const [creating, setCreating] = useState(false);
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState('');
  const [selectedFolderId, setSelectedFolderId] = useState('');
  const foldersByWorkspace = useMemo(() => {
    const next: Record<string, WorkspaceFolderSummary[]> = {};
    folders
      .filter((folder) => folder.status !== 'DELETED')
      .forEach((folder) => {
        next[folder.workspaceId] = [...(next[folder.workspaceId] ?? []), folder];
      });
    Object.values(next).forEach((items) => items.sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name)));
    return next;
  }, [folders]);
  const activeWorkspaces = useMemo(() => workspaces.filter((workspace) => workspace.status !== 'DELETED'), [workspaces]);
  useEffect(() => {
    if (initialWorkspaceId) setSelectedWorkspaceId(initialWorkspaceId);
    if (initialFolderId) setSelectedFolderId(initialFolderId);
  }, [initialFolderId, initialWorkspaceId]);
  const workflowCountByWorkspace = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      const placement = placements[activity.id];
      if (!placement?.workspaceId) return;
      next[placement.workspaceId] = (next[placement.workspaceId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements]);
  const workflowCountByFolder = useMemo(() => {
    const next: Record<string, number> = {};
    activities.forEach((activity) => {
      const placement = placements[activity.id];
      if (!placement?.folderId) return;
      next[placement.folderId] = (next[placement.folderId] ?? 0) + 1;
    });
    return next;
  }, [activities, placements]);
  const selectedWorkspace = activeWorkspaces.find((workspace) => workspace.id === selectedWorkspaceId) ?? activeWorkspaces[0];
  const selectedWorkspaceFolders = selectedWorkspace ? foldersByWorkspace[selectedWorkspace.id] ?? [] : [];
  const selectedFolder = selectedWorkspaceFolders.find((folder) => folder.id === selectedFolderId) ?? selectedWorkspaceFolders[0];
  useEffect(() => {
    if (!selectedWorkspace && selectedWorkspaceId) {
      setSelectedWorkspaceId('');
      setSelectedFolderId('');
      return;
    }
    if (selectedWorkspace && selectedWorkspace.id !== selectedWorkspaceId) {
      setSelectedWorkspaceId(selectedWorkspace.id);
    }
    if (selectedFolder && selectedFolder.id !== selectedFolderId) {
      setSelectedFolderId(selectedFolder.id);
    }
    if (!selectedFolder && selectedFolderId) {
      setSelectedFolderId('');
    }
  }, [selectedFolder, selectedFolderId, selectedWorkspace, selectedWorkspaceId]);
  const selectedWorkflows = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return activities
      .filter((activity) => {
        if (selectedWorkspace) {
          const placement = placements[activity.id];
          if (!placement || placement.workspaceId !== selectedWorkspace.id) return false;
          if (selectedFolder && placement.folderId !== selectedFolder.id) return false;
        }
        if (!keyword) return true;
        const folder = placements[activity.id]?.folderId ? folders.find((item) => item.id === placements[activity.id]?.folderId) : undefined;
        return `${activity.activityName} ${activity.activityCode} ${activity.tenantId} ${selectedWorkspace?.name ?? ''} ${folder?.name ?? ''}`.toLowerCase().includes(keyword);
      });
  }, [activities, folders, placements, search, selectedFolder, selectedWorkspace]);
  const suggestedCode = normalizeActivityCode(createCode || createName || search);
  const openCreateDialog = (name?: string, context?: { workspaceId?: string; folderId?: string }) => {
    if (name) setCreateName(name);
    const fallbackWorkspace = context?.workspaceId || selectedWorkspace?.id || createWorkspaceId || workspaces[0]?.id || '';
    const fallbackFolder = context?.folderId || selectedFolder?.id || createFolderId || foldersByWorkspace[fallbackWorkspace]?.[0]?.id || '';
    setCreateWorkspaceId(fallbackWorkspace);
    setCreateFolderId(fallbackFolder);
    const fallbackWorkspaceItem = workspaces.find((workspace) => workspace.id === fallbackWorkspace);
    if (fallbackWorkspaceItem?.tenantId) setCreateTenant(fallbackWorkspaceItem.tenantId);
    setCreateError('');
    setShowCreateDialog(true);
  };
  const workspaceFolders = foldersByWorkspace[createWorkspaceId] ?? [];
  const submitWorkspace = async () => {
    const nextName = workspaceName.trim();
    if (!nextName) {
      setWorkspaceError('先输入 workspace 名称。');
      return;
    }
    setCreatingWorkspace(true);
    setWorkspaceError('');
    try {
      const workspace = await onCreateWorkspace({ name: nextName, tenantId: workspaceTenant.trim() || 'site_001' });
      setSelectedWorkspaceId(workspace.id);
      setSelectedFolderId('');
      setCreateWorkspaceId(workspace.id);
      setCreateTenant(workspace.tenantId);
      setWorkspaceName('');
      setShowWorkspaceDialog(false);
    } catch (error) {
      setWorkspaceError((error as Error).message);
    } finally {
      setCreatingWorkspace(false);
    }
  };
  const submitFolder = async () => {
    if (!selectedWorkspace) {
      setFolderError('先选择 workspace。');
      return;
    }
    const nextName = folderName.trim();
    if (!nextName) {
      setFolderError('先输入 folder 名称。');
      return;
    }
    setCreatingFolder(true);
    setFolderError('');
    try {
      const folder = await onCreateFolder(selectedWorkspace.id, nextName);
      setSelectedFolderId(folder.id);
      setFolderName('');
      setShowFolderDialog(false);
    } catch (error) {
      setFolderError((error as Error).message);
    } finally {
      setCreatingFolder(false);
    }
  };
  const submitCreate = async () => {
    const nextName = createName.trim() || search.trim();
    if (!nextName) {
      setCreateError('先输入活动名称。');
      return;
    }
    if (!createWorkspaceId) {
      setCreateError('先选择工作区。');
      return;
    }
    const selectedWorkspace = workspaces.find((workspace) => workspace.id === createWorkspaceId);
    let nextFolderId = createFolderId;
    if (!nextFolderId && createFolderName.trim()) {
      try {
        const folder = await onCreateFolder(createWorkspaceId, createFolderName.trim());
        nextFolderId = folder.id;
        setCreateFolderId(folder.id);
      } catch (error) {
        setCreateError((error as Error).message);
        return;
      }
    }
    if (!nextFolderId) {
      setCreateError('先选择文件夹，或输入新文件夹名称。');
      return;
    }
    setCreating(true);
    setCreateError('');
    try {
      await onCreateActivity({
        tenantId: selectedWorkspace?.tenantId || createTenant,
        activityName: nextName,
        activityCode: createCode || suggestedCode,
        workspaceId: createWorkspaceId,
        folderId: nextFolderId,
      });
      setShowCreateDialog(false);
      setCreateFolderName('');
    } catch (error) {
      setCreateError((error as Error).message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="studio-start-overlay">
      <section className="studio-start-shell">
        <header className="studio-home-top">
          <div>
            <b>ExMarketing</b>
            <span>Activity Studio</span>
          </div>
          <a href="/admin/">活动后台</a>
        </header>
        <div className="studio-start-body">
          <aside className="studio-workspace-panel">
            <div className="studio-section-head">
              <b>Workspace</b>
              <button onClick={() => {
                setWorkspaceError('');
                setShowWorkspaceDialog(true);
              }}>+</button>
            </div>
            <div className="studio-start-search">
              <input value={search} placeholder="搜索 workflow" onChange={(event) => setSearch(event.target.value)} />
            </div>
            <div className="studio-workspace-list">
              {activeWorkspaces.length ? activeWorkspaces.map((workspace) => (
                <button
                  key={workspace.id}
                  className={cx(workspace.id === selectedWorkspace?.id && 'active')}
                  onClick={() => {
                    setSelectedWorkspaceId(workspace.id);
                    setSelectedFolderId(foldersByWorkspace[workspace.id]?.[0]?.id ?? '');
                  }}
                >
                  <span style={{ background: workspace.color || '#155eef' }}>{(workspace.name || workspace.slug || 'W').slice(0, 1).toUpperCase()}</span>
                  <div>
                    <b>{workspace.name}</b>
                    <em>{workspace.slug} · {workspace.visibility}</em>
                  </div>
                  <strong>{workflowCountByWorkspace[workspace.id] ?? 0}</strong>
                </button>
              )) : <div className="studio-start-empty">暂无 workspace</div>}
            </div>
            {hiddenTestActivityCount > 0 ? (
              <p className="studio-start-note">已隐藏 {hiddenTestActivityCount} 个测试活动</p>
            ) : null}
          </aside>

          <aside className="studio-folder-panel">
            <div className="studio-section-head">
              <div>
                <b>Folder</b>
                <p>{selectedWorkspace?.name ?? '选择 workspace'}</p>
              </div>
              <button
                onClick={() => {
                  setFolderError('');
                  setShowFolderDialog(true);
                }}
                disabled={!selectedWorkspace}
              >
                +
              </button>
            </div>
            <div className="studio-folder-list">
              {selectedWorkspaceFolders.length ? selectedWorkspaceFolders.map((folder) => (
                <button
                  key={folder.id}
                  className={cx(folder.id === selectedFolder?.id && 'active')}
                  onClick={() => setSelectedFolderId(folder.id)}
                >
                  <span style={{ background: folder.color || '#eef4ff' }}>▰</span>
                  <div>
                    <b>{folder.name}</b>
                    <em>{folder.visibility} · {folder.status}</em>
                  </div>
                  <strong>{workflowCountByFolder[folder.id] ?? 0}</strong>
                </button>
              )) : <div className="studio-start-empty">当前 workspace 没有 folder</div>}
            </div>
          </aside>

          <section className="studio-workflow-panel">
            <div className="studio-workflow-head">
              <div>
                <span>Workflow</span>
                <b>{selectedFolder?.name ?? selectedWorkspace?.name ?? '选择一个 workspace'}</b>
                <p>{workflowStatus}</p>
              </div>
              <button onClick={() => openCreateDialog('新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id })} disabled={!selectedWorkspace}>
                <span>+</span>
                New Workflow
              </button>
            </div>
            {selectedWorkflows.length ? (
              <div className="studio-workflow-grid">
                {selectedWorkflows.map((activity) => (
                  <button key={activity.id} onClick={() => onSelectActivity(activity.id)}>
                    <div className="studio-workflow-icon">{(activity.activityName || activity.activityCode || 'W').slice(0, 1).toUpperCase()}</div>
                    <span>
                      <b>{activity.activityName || activity.activityCode}</b>
                      <em>{activity.activityCode}</em>
                    </span>
                    <strong>{activity.status || 'UNKNOWN'}</strong>
                  </button>
                ))}
              </div>
            ) : (
              <div className="studio-empty-state">
                <div className="studio-empty-icon">＋</div>
                <b>这个 folder 还没有 workflow</b>
                <span>从当前 workspace / folder 创建后，会直接进入编排画布。</span>
                <div className="studio-empty-actions">
                  <button onClick={() => openCreateDialog(search || '新 workflow', { workspaceId: selectedWorkspace?.id, folderId: selectedFolder?.id })}>创建 Workflow</button>
                  <a href="/admin/">活动后台</a>
                </div>
              </div>
            )}
          </section>
        </div>
      </section>
      {showWorkspaceDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建 Workspace</b>
                <span>用于组织 folder 和 workflow。</span>
              </div>
              <button onClick={() => setShowWorkspaceDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={workspaceName} placeholder="Marketing Studio" onChange={(event) => setWorkspaceName(event.target.value)} />
              </label>
              <label>
                <span>Tenant</span>
                <input value={workspaceTenant} placeholder="site_001" onChange={(event) => setWorkspaceTenant(event.target.value)} />
              </label>
              <button onClick={submitWorkspace} disabled={creatingWorkspace}>{creatingWorkspace ? '创建中...' : '创建 Workspace'}</button>
            </div>
            {workspaceError ? <p>{workspaceError}</p> : null}
          </section>
        </div>
      ) : null}
      {showFolderDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建 Folder</b>
                <span>{selectedWorkspace ? `创建到 ${selectedWorkspace.name}` : '先选择 workspace。'}</span>
              </div>
              <button onClick={() => setShowFolderDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={folderName} placeholder="增长活动" onChange={(event) => setFolderName(event.target.value)} />
              </label>
              <button onClick={submitFolder} disabled={creatingFolder || !selectedWorkspace}>{creatingFolder ? '创建中...' : '创建 Folder'}</button>
            </div>
            {folderError ? <p>{folderError}</p> : null}
          </section>
        </div>
      ) : null}
      {showCreateDialog ? (
        <div className="studio-create-dialog-backdrop">
          <section className="studio-create-dialog" onClick={(event) => event.stopPropagation()}>
            <div className="studio-create-dialog-head">
              <div>
                <b>新建活动</b>
                <span>创建后直接进入编排画布。</span>
              </div>
              <button onClick={() => setShowCreateDialog(false)}>×</button>
            </div>
            <div className="studio-create-grid">
              <label>
                <span>名称</span>
                <input value={createName} placeholder="618 拉新活动" onChange={(event) => setCreateName(event.target.value)} />
              </label>
              <label>
                <span>Code</span>
                <input value={createCode} placeholder={suggestedCode} onChange={(event) => setCreateCode(event.target.value)} />
              </label>
              <label>
                <span>Tenant</span>
                <input value={createTenant} placeholder="site_001" onChange={(event) => setCreateTenant(event.target.value)} />
              </label>
              <label>
                <span>Workspace</span>
                <select value={createWorkspaceId} onChange={(event) => {
                  const nextWorkspace = event.target.value;
                  setCreateWorkspaceId(nextWorkspace);
                  setCreateFolderId(foldersByWorkspace[nextWorkspace]?.[0]?.id ?? '');
                  const workspace = workspaces.find((item) => item.id === nextWorkspace);
                  if (workspace?.tenantId) setCreateTenant(workspace.tenantId);
                }}>
                  <option value="">选择工作区</option>
                  {workspaces.map((workspace) => (
                    <option key={workspace.id} value={workspace.id}>{workspace.name}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>Folder</span>
                <select value={createFolderId} onChange={(event) => setCreateFolderId(event.target.value)}>
                  <option value="">选择文件夹</option>
                  {workspaceFolders.map((folder) => (
                    <option key={folder.id} value={folder.id}>{folder.name}</option>
                  ))}
                </select>
              </label>
              <label>
                <span>新文件夹</span>
                <input value={createFolderName} placeholder="没有文件夹时填写" onChange={(event) => setCreateFolderName(event.target.value)} />
              </label>
              <button onClick={submitCreate} disabled={creating}>{creating ? '创建中...' : '创建并编排'}</button>
            </div>
            {createError ? <p>{createError}</p> : null}
          </section>
        </div>
      ) : null}
    </div>
  );
}

export function App() {
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowNode>(baseNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState<ActivityEdge>(baseEdges);
  const [selectedTemplate, setSelectedTemplate] = useState<'default' | 'simple'>('default');
  const [lastAdded, setLastAdded] = useState('');
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [selectedEdgeIds, setSelectedEdgeIds] = useState<string[]>([]);
  const [contextMenu, setContextMenu] = useState<FlowContextMenu | null>(null);
  const [nodePicker, setNodePicker] = useState<NodePickerState | null>(null);
  const [nodeSearch, setNodeSearch] = useState('');
  const [reactFlowInstance, setReactFlowInstance] = useState<ReactFlowInstance<FlowNode, ActivityEdge> | null>(null);
  const [alignmentGuide, setAlignmentGuide] = useState<AlignmentGuide>({});
  const [hoveredEdgeId, setHoveredEdgeId] = useState('');
  const [hoveredNodeId, setHoveredNodeId] = useState('');
  const [showPalette, setShowPalette] = useState(false);
  const [interactionMode, setInteractionMode] = useState<'pointer' | 'hand'>('pointer');
  const [zoomPercent, setZoomPercent] = useState(100);
  const [showMiniMap, setShowMiniMap] = useState(true);
  const [showChecklist, setShowChecklist] = useState(false);
  const [validatingWorkflow, setValidatingWorkflow] = useState(false);
  const [workflowValidation, setWorkflowValidation] = useState<WorkflowValidationResult | null>(null);
  const [nodeConfigSchemas, setNodeConfigSchemas] = useState<WorkflowNodeConfigSchema[]>([]);
  const [isPublished, setIsPublished] = useState(false);
  const [activities, setActivities] = useState<ActivitySummary[]>([]);
  const [workspaces, setWorkspaces] = useState<WorkspaceSummary[]>([]);
  const [workspaceFolders, setWorkspaceFolders] = useState<WorkspaceFolderSummary[]>([]);
  const [activityPlacements, setActivityPlacements] = useState<Record<string, ActivityPlacement>>({});
  const [activityId, setActivityId] = useState('');
  const [activityLabel, setActivityLabel] = useState('选择活动');
  const [workflowStatus, setWorkflowStatus] = useState('未连接后端');
  const [workflowVersionLabel, setWorkflowVersionLabel] = useState('workflow 未发布');
  const [homeWorkspaceId, setHomeWorkspaceId] = useState('');
  const [homeFolderId, setHomeFolderId] = useState('');

  const nodeTypes = useMemo(() => ({ activity: ActivityNodeView, group: GroupNodeView, note: NoteNodeView }), []);
  const selectedCount = selectedNodeIds.length + selectedEdgeIds.length;
  const visibleStudioActivities = useMemo(() => activities.filter((activity) => !isTestActivity(activity)), [activities]);
  const hiddenTestActivityCount = activities.length - visibleStudioActivities.length;
  const activePlacement = activityId ? activityPlacements[activityId] : undefined;
  const activeWorkspace = activePlacement ? workspaces.find((workspace) => workspace.id === activePlacement.workspaceId) : undefined;
  const activeFolder = activePlacement ? workspaceFolders.find((folder) => folder.id === activePlacement.folderId) : undefined;

  useEffect(() => {
    let cancelled = false;
    const params = new URLSearchParams(window.location.search);
    const requestedActivityId = params.get('activityId') ?? '';
    Promise.all([
      fetchJSON<{ items: ActivitySummary[] }>('/admin/v1/activities'),
      fetchJSON<{ items: WorkspaceSummary[] }>('/admin/v1/workspaces?status=ACTIVE&limit=100'),
    ])
      .then(async ([activityBody, workspaceBody]) => {
        if (cancelled) return;
        const activityItems = activityBody.items ?? [];
        const workspaceItems = workspaceBody.items ?? [];
        const activeWorkspaces = workspaceItems.filter((workspace) => workspace.status !== 'DELETED');
        const folderResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceFolderSummary[] }>(`/admin/v1/workspaces/${workspace.id}/folders?status=ACTIVE&limit=200`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        const shareResults = await Promise.all(activeWorkspaces.map((workspace) => (
          fetchJSON<{ items: WorkspaceShareLinkSummary[] }>(`/admin/v1/workspaces/${workspace.id}/share-links?scope=ACTIVITY&enabledOnly=true&limit=500`)
            .then((body) => body.items ?? [])
            .catch(() => [])
        )));
        if (cancelled) return;
        const nextFolders = folderResults.flat();
        const nextPlacements: Record<string, ActivityPlacement> = {};
        activityItems.forEach((activity) => {
          if (!activity.workspaceId || !activity.folderId) return;
          nextPlacements[activity.id] = {
            workspaceId: activity.workspaceId,
            folderId: activity.folderId,
          };
        });
        shareResults.flat().forEach((share) => {
          if (!share.activityId || !share.folderId || !share.enabled) return;
          if (nextPlacements[share.activityId]) return;
          nextPlacements[share.activityId] = {
            workspaceId: share.workspaceId,
            folderId: share.folderId,
            shareLinkId: share.id,
          };
        });
        setActivities(activityItems);
        setWorkspaces(activeWorkspaces);
        setWorkspaceFolders(nextFolders);
        setActivityPlacements(nextPlacements);
        if (!activityItems.length) {
          setWorkflowStatus('后端已连接 · 暂无 workflow');
          return;
        }
        if (!requestedActivityId) {
          setWorkflowStatus('请选择活动开始编排');
          return;
        }
        const activity = activityItems.find((item) => item.id === requestedActivityId);
        if (!activity) {
          setWorkflowStatus('URL 中的活动不存在，请重新选择');
          return;
        }
        setActivityId(activity.id);
        setActivityLabel(`${activity.activityName} / ${activity.activityCode}`);
        setWorkflowStatus('已绑定活动');
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setWorkflowStatus(`后端未连接：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchJSON<{ items: WorkflowNodeConfigSchema[] }>('/admin/v1/workflow/node-config-schemas')
      .then((body) => {
        if (cancelled) return;
        setNodeConfigSchemas(body.items ?? []);
      })
      .catch((error: Error) => {
        if (cancelled) return;
        setWorkflowStatus(`节点 schema 加载失败：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!activityId) return;
    let cancelled = false;
    const activity = activities.find((item) => item.id === activityId);
    if (activity) {
      setActivityLabel(`${activity.activityName} / ${activity.activityCode}`);
      const params = new URLSearchParams(window.location.search);
      params.set('activityId', activity.id);
      window.history.replaceState(null, '', `${window.location.pathname}?${params.toString()}`);
    }
    fetchJSON<{ definition: WorkflowDefinition }>(`/admin/v1/activities/${activityId}/workflow/draft`)
      .then((draft) => {
        if (cancelled || !draft.definition) return;
        const next = hydrateWorkflowDefinition(draft.definition);
        setNodes(next.nodes);
        setEdges(next.edges);
        setWorkflowStatus('已加载草稿');
      })
      .catch((error: Error) => {
        if (cancelled) return;
        if (error.message.includes('not found')) {
          setNodes(baseNodes);
          setEdges(baseEdges);
          setSelectedTemplate('default');
          setWorkflowStatus('当前活动暂无草稿');
          return;
        }
        setWorkflowStatus(`草稿加载失败：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, [activities, activityId, setEdges, setNodes]);

  const refreshWorkflowVersions = useCallback(async (nextActivityId = activityId) => {
    if (!nextActivityId) {
      setWorkflowVersionLabel('workflow 未发布');
      setIsPublished(false);
      return;
    }
    try {
      const versions = await fetchJSON<{ items: WorkflowVersion[] }>(`/admin/v1/activities/${nextActivityId}/workflow/versions`);
      const active = versions.items.find((item) => item.status === 'ACTIVE');
      if (!active) {
        setWorkflowVersionLabel('workflow 未发布');
        setIsPublished(false);
        return;
      }
      setWorkflowVersionLabel(`workflow v${active.version} · ${new Date(active.publishedAt).toLocaleString()}`);
      setIsPublished(true);
    } catch (error) {
      setWorkflowVersionLabel(`版本加载失败：${(error as Error).message}`);
      setIsPublished(false);
    }
  }, [activityId]);

  useEffect(() => {
    void refreshWorkflowVersions(activityId);
  }, [activityId, refreshWorkflowVersions]);

  const filteredCatalog = useMemo(() => {
    const keyword = nodeSearch.trim().toLowerCase();
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
  }, [nodeSearch]);

  const closeFloatingUi = useCallback(() => {
    setContextMenu(null);
    setNodePicker(null);
  }, []);

  const updateNodeConfig = useCallback((nodeId: string, key: string, value: unknown) => {
    setNodes((items) => items.map((node) => {
      if (node.type !== 'activity' || node.id !== nodeId) return node;
      return {
        ...node,
        data: {
          ...node.data,
          config: {
            ...(node.data.config ?? {}),
            [key]: value,
          },
        },
      } as FlowNode;
    }));
  }, [setNodes]);

  const addNodeAt = useCallback((def: ActivityNodeData, x: number, y: number) => {
    const id = `${def.id}-${Date.now().toString(36)}`;
    setNodes((items) => items.concat(flowNode(def.id, x, y, { id })));
    if (nodePicker?.sourceNodeId) {
      const newEdge = edge(nodePicker.sourceNodeId, id, nodePicker.sourceLabel ?? '', nodePicker.sourceHandle);
      setEdges((items) => items.concat(newEdge));
      setSelectedEdgeIds([newEdge.id]);
      setSelectedNodeIds([]);
    }
    setLastAdded(def.title);
    closeFloatingUi();
  }, [closeFloatingUi, nodePicker, setEdges, setNodes]);

  const addCatalogNode = useCallback((def: ActivityNodeData) => {
    const offset = nodes.length * 18;
    addNodeAt(def, 180 + (offset % 760), 120 + (offset % 420));
  }, [addNodeAt, nodes.length]);

  const openOutputNodePicker = useCallback((request: OutputAddRequest) => {
    const flowPosition = reactFlowInstance?.screenToFlowPosition({
      x: request.clientX + 220,
      y: request.clientY - 60,
    }) ?? { x: request.clientX + 180, y: request.clientY };
    setNodeSearch('');
    setContextMenu(null);
    setSelectedNodeIds([request.nodeId]);
    setSelectedEdgeIds([]);
    setNodePicker({
      x: Math.min(request.clientX + 18, window.innerWidth - 286),
      y: Math.max(14, Math.min(request.clientY - 64, window.innerHeight - 590)),
      flowX: flowPosition.x,
      flowY: flowPosition.y,
      sourceNodeId: request.nodeId,
      sourceHandle: `output-${request.outputIndex}`,
      sourceLabel: request.outputLabel,
    });
  }, [reactFlowInstance]);

  const addNoteAt = useCallback((x: number, y: number) => {
    const id = `note-${Date.now().toString(36)}`;
    setNodes((items) => items.concat({
      id,
      type: 'note',
      position: { x, y },
      data: { text: '补充活动流程说明、配置约束或异常处理备注。' },
    } satisfies NoteNode));
    setLastAdded('备注');
    closeFloatingUi();
  }, [closeFloatingUi, setNodes]);

  const deleteNodes = useCallback((nodeIds: string[]) => {
    if (nodeIds.length === 0) return;
    const nodeIdSet = new Set(nodeIds);
    setNodes((items) => {
      const deletedGroups = items.filter((item) => nodeIdSet.has(item.id) && item.type === 'group');
      const groupPositionById = new Map(deletedGroups.map((item) => [item.id, item.position]));
      return items
        .filter((item) => !nodeIdSet.has(item.id))
        .map((item) => {
          if (!item.parentId || !groupPositionById.has(item.parentId)) return item;
          const parentPosition = groupPositionById.get(item.parentId) ?? { x: 0, y: 0 };
          const { parentId: _parentId, extent: _extent, ...rest } = item;
          return { ...rest, position: { x: parentPosition.x + item.position.x, y: parentPosition.y + item.position.y } } as FlowNode;
        });
    });
    setEdges((items) => items.filter((item) => !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target)));
    setSelectedNodeIds((items) => items.filter((item) => !nodeIdSet.has(item)));
    closeFloatingUi();
  }, [closeFloatingUi, setEdges, setNodes]);

  const deleteEdges = useCallback((edgeIds: string[]) => {
    if (edgeIds.length === 0) return;
    const edgeIdSet = new Set(edgeIds);
    setEdges((items) => items.filter((item) => !edgeIdSet.has(item.id)));
    setSelectedEdgeIds((items) => items.filter((item) => !edgeIdSet.has(item)));
    closeFloatingUi();
  }, [closeFloatingUi, setEdges]);

  const duplicateNode = useCallback((nodeId: string) => {
    const current = nodes.find((item) => item.id === nodeId);
    if (!current) return;
    const duplicate = {
      ...current,
      id: `${current.type}-${Date.now().toString(36)}`,
      selected: false,
      parentId: undefined,
      extent: undefined,
      position: { x: current.position.x + 42, y: current.position.y + 42 },
      data: { ...current.data },
    } as FlowNode;
    setNodes((items) => items.concat(duplicate));
    setLastAdded(`${'title' in current.data ? current.data.title : '备注'} 副本`);
    closeFloatingUi();
  }, [closeFloatingUi, nodes, setNodes]);

  const openPaneMenu = useCallback((event: MouseEvent | React.MouseEvent) => {
    event.preventDefault();
    const flowPosition = reactFlowInstance?.screenToFlowPosition({ x: event.clientX, y: event.clientY }) ?? { x: 120, y: 120 };
    setContextMenu({ type: 'pane', x: event.clientX, y: event.clientY, flowX: flowPosition.x, flowY: flowPosition.y });
  }, [reactFlowInstance]);

  const openNodeMenu = useCallback((event: React.MouseEvent, node: FlowNode) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedNodeIds([node.id]);
    setSelectedEdgeIds([]);
    setContextMenu({ type: 'node', x: event.clientX, y: event.clientY, nodeId: node.id });
  }, []);

  const openEdgeMenu = useCallback((event: React.MouseEvent, edgeItem: ActivityEdge) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedEdgeIds([edgeItem.id]);
    setContextMenu({ type: 'edge', x: event.clientX, y: event.clientY, edgeId: edgeItem.id });
  }, []);

  const deleteSelected = useCallback(() => {
    const nodeIdSet = new Set(selectedNodeIds);
    const edgeIdSet = new Set(selectedEdgeIds);
    setNodes((items) => items.filter((item) => !nodeIdSet.has(item.id)));
    setEdges((items) => items.filter((item) => !edgeIdSet.has(item.id) && !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target)));
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
  }, [closeFloatingUi, selectedEdgeIds, selectedNodeIds, setEdges, setNodes]);

  const clearCanvas = useCallback(() => {
    setSelectedTemplate('default');
    setNodes([]);
    setEdges([]);
    setLastAdded('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
  }, [closeFloatingUi, setEdges, setNodes]);

  const resetDefault = useCallback(() => {
    setSelectedTemplate('default');
    setNodes(baseNodes);
    setEdges(baseEdges);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.14, duration: 360 }));
  }, [closeFloatingUi, reactFlowInstance, setEdges, setNodes]);

  const loadSimpleTemplate = useCallback(() => {
    const template = simpleTemplate();
    setSelectedTemplate('simple');
    setNodes(template.nodes);
    setEdges(template.edges);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => reactFlowInstance?.fitView({ padding: 0.16, duration: 360 }));
  }, [closeFloatingUi, reactFlowInstance, setEdges, setNodes]);

  const autoLayout = useCallback(() => {
    setNodes((items) => layoutFlowNodes(items));
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeFloatingUi();
    window.requestAnimationFrame(() => {
      reactFlowInstance?.setViewport({ x: 24, y: 36, zoom: 1 }, { duration: 360 });
      setZoomPercent(100);
    });
  }, [closeFloatingUi, reactFlowInstance, setNodes]);

  const groupSelectedNodes = useCallback(() => {
    const selected = nodes.filter((node): node is ActivityNode => selectedNodeIds.includes(node.id) && node.type === 'activity');
    if (selected.length < 2) return;
    const minX = Math.min(...selected.map((node) => node.position.x));
    const minY = Math.min(...selected.map((node) => node.position.y));
    const maxX = Math.max(...selected.map((node) => node.position.x + ACTIVITY_NODE_WIDTH));
    const maxY = Math.max(...selected.map((node) => node.position.y + activityNodeHeight(node)));
    const groupId = `group-${Date.now().toString(36)}`;
    const groupX = minX - 28;
    const groupY = minY - 58;
    const groupNode: GroupNode = {
      id: groupId,
      type: 'group',
      position: { x: groupX, y: groupY },
      data: { title: `组合 ${selected.length} 个节点`, width: maxX - minX + 56, height: maxY - minY + 92 },
      selectable: true,
    };
    const selectedSet = new Set(selected.map((node) => node.id));
    setNodes((items) => [
      groupNode,
      ...items.map((node) => {
        if (!selectedSet.has(node.id)) return node;
        return { ...node, parentId: groupId, extent: 'parent', position: { x: node.position.x - groupX, y: node.position.y - groupY } } as FlowNode;
      }),
    ]);
    setSelectedNodeIds([groupId]);
    setSelectedEdgeIds([]);
    setLastAdded(groupNode.data.title);
  }, [nodes, selectedNodeIds, setNodes]);

  const updateAlignmentGuide = useCallback((dragged: FlowNode) => {
    if (dragged.type !== 'activity') {
      setAlignmentGuide({});
      return;
    }
    const tolerance = 8;
    const nextGuide: AlignmentGuide = {};
    const draggedHeight = activityNodeHeight(dragged);
    const draggedX = { left: dragged.position.x, center: dragged.position.x + ACTIVITY_NODE_WIDTH / 2, right: dragged.position.x + ACTIVITY_NODE_WIDTH };
    const draggedY = { top: dragged.position.y, center: dragged.position.y + draggedHeight / 2, bottom: dragged.position.y + draggedHeight };
    nodes.forEach((node) => {
      if (node.id === dragged.id || node.type !== 'activity' || node.parentId || dragged.parentId) return;
      const referenceHeight = activityNodeHeight(node);
      const referenceX = { left: node.position.x, center: node.position.x + ACTIVITY_NODE_WIDTH / 2, right: node.position.x + ACTIVITY_NODE_WIDTH };
      const referenceY = { top: node.position.y, center: node.position.y + referenceHeight / 2, bottom: node.position.y + referenceHeight };
      (Object.keys(referenceX) as Array<keyof typeof referenceX>).forEach((refKind) => {
        (Object.keys(draggedX) as Array<keyof typeof draggedX>).forEach((dragKind) => {
          if (nextGuide.vertical || Math.abs(referenceX[refKind] - draggedX[dragKind]) > tolerance) return;
          const flowStart = Math.min(node.position.y, dragged.position.y) - 28;
          const flowEnd = Math.max(node.position.y + referenceHeight, dragged.position.y + draggedHeight) + 28;
          nextGuide.vertical = {
            kind: refKind,
            start: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: flowStart }).y ?? flowStart,
            end: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: flowEnd }).y ?? flowEnd,
            screen: reactFlowInstance?.flowToScreenPosition({ x: referenceX[refKind], y: node.position.y }).x ?? referenceX[refKind],
          };
        });
      });
      (Object.keys(referenceY) as Array<keyof typeof referenceY>).forEach((refKind) => {
        (Object.keys(draggedY) as Array<keyof typeof draggedY>).forEach((dragKind) => {
          if (nextGuide.horizontal || Math.abs(referenceY[refKind] - draggedY[dragKind]) > tolerance) return;
          const flowStart = Math.min(node.position.x, dragged.position.x) - 28;
          const flowEnd = Math.max(node.position.x + ACTIVITY_NODE_WIDTH, dragged.position.x + ACTIVITY_NODE_WIDTH) + 28;
          nextGuide.horizontal = {
            kind: refKind,
            start: reactFlowInstance?.flowToScreenPosition({ x: flowStart, y: referenceY[refKind] }).x ?? flowStart,
            end: reactFlowInstance?.flowToScreenPosition({ x: flowEnd, y: referenceY[refKind] }).x ?? flowEnd,
            screen: reactFlowInstance?.flowToScreenPosition({ x: node.position.x, y: referenceY[refKind] }).y ?? referenceY[refKind],
          };
        });
      });
    });
    setAlignmentGuide(nextGuide);
  }, [nodes, reactFlowInstance]);

  const validationHighlights = useMemo(() => {
    const blockNodeIds = new Set<string>();
    const warningNodeIds = new Set<string>();
    const blockEdgeIds = new Set<string>();
    const warningEdgeIds = new Set<string>();
    workflowValidation?.items.forEach((item) => {
      const nodeSet = item.severity === 'BLOCK' ? blockNodeIds : warningNodeIds;
      const edgeSet = item.severity === 'BLOCK' ? blockEdgeIds : warningEdgeIds;
      if (item.nodeId) nodeSet.add(item.nodeId);
      if (item.edgeId) edgeSet.add(item.edgeId);
    });
    return { blockNodeIds, warningNodeIds, blockEdgeIds, warningEdgeIds };
  }, [workflowValidation]);

  const visibleNodes = useMemo(() => nodes.map((node) => {
    const validationClass = validationHighlights.blockNodeIds.has(node.id)
      ? 'validation-block'
      : validationHighlights.warningNodeIds.has(node.id)
        ? 'validation-warning'
        : '';
    if (node.type === 'activity') {
      return {
        ...node,
        className: cx(node.className, validationClass),
        data: {
          ...node.data,
          onDelete: (nodeId: string) => deleteNodes([nodeId]),
          onAddFromOutput: openOutputNodePicker,
          onHoverChange: (nodeId: string, hovering: boolean) => setHoveredNodeId(hovering ? nodeId : ''),
        },
      } as ActivityNode;
    }
    return { ...node, className: cx(node.className, validationClass), data: { ...node.data, onDelete: (nodeId: string) => deleteNodes([nodeId]) } } as FlowNode;
  }), [deleteNodes, nodes, openOutputNodePicker, validationHighlights]);

  const visibleEdges = useMemo(() => edges.map((item) => ({
    ...item,
    className: cx(
      item.className,
      validationHighlights.blockEdgeIds.has(item.id) && 'validation-block',
      !validationHighlights.blockEdgeIds.has(item.id) && validationHighlights.warningEdgeIds.has(item.id) && 'validation-warning',
      item.id === hoveredEdgeId && 'edge-hover',
      hoveredNodeId !== '' && (item.source === hoveredNodeId || item.target === hoveredNodeId) && 'edge-node-hover',
      selectedNodeIds.some((nodeId) => item.source === nodeId || item.target === nodeId) && 'edge-related',
    ),
  })), [edges, hoveredEdgeId, hoveredNodeId, selectedNodeIds, validationHighlights]);

  const onConnect = useCallback((params: Connection) => {
    setEdges((items) => addEdge({
      ...params,
      markerEnd: { type: MarkerType.ArrowClosed, width: 5, height: 5, color: '#c4ccd8' },
    }, items));
  }, [setEdges]);

  const syncZoom = useCallback(() => {
    const zoom = reactFlowInstance?.getZoom() ?? 1;
    setZoomPercent(Math.round(zoom * 100));
  }, [reactFlowInstance]);

  const zoomTo = useCallback((zoom: number) => {
    reactFlowInstance?.setViewport({ x: 24, y: 36, zoom }, { duration: 180 });
    setZoomPercent(Math.round(zoom * 100));
  }, [reactFlowInstance]);

  const focusValidationIssue = useCallback((item: WorkflowValidationIssue) => {
    if (item.nodeId) {
      const node = nodes.find((candidate) => candidate.id === item.nodeId);
      if (node) {
        setSelectedNodeIds([node.id]);
        setSelectedEdgeIds([]);
        const nodeWidth = node.type === 'activity' ? ACTIVITY_NODE_WIDTH : (node.width ?? 180);
        const nodeHeight = node.type === 'activity' ? activityNodeHeight(node) : (node.height ?? 120);
        reactFlowInstance?.setCenter(node.position.x + nodeWidth / 2, node.position.y + nodeHeight / 2, {
          zoom: Math.max(reactFlowInstance.getZoom(), 1),
          duration: 360,
        });
        setWorkflowStatus(`已定位节点：${node.id}`);
        return;
      }
    }
    if (item.edgeId) {
      const targetEdge = edges.find((candidate) => candidate.id === item.edgeId);
      if (targetEdge) {
        setSelectedNodeIds([]);
        setSelectedEdgeIds([targetEdge.id]);
        const relatedNodes = nodes.filter((node) => node.id === targetEdge.source || node.id === targetEdge.target);
        if (relatedNodes.length > 0) {
          reactFlowInstance?.fitView({ nodes: relatedNodes.map((node) => ({ id: node.id })), padding: 0.28, duration: 360 });
        }
        setWorkflowStatus(`已定位连线：${targetEdge.id}`);
        return;
      }
    }
    setWorkflowStatus(`未找到检查项定位目标：${item.code}`);
  }, [edges, nodes, reactFlowInstance]);

  const saveDraft = useCallback(async (): Promise<boolean> => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return false;
    }
    const placement = activityPlacements[activityId];
    if (!placement?.workspaceId || !placement.folderId) {
      setWorkflowStatus('保存前需要先把活动绑定到 workspace / folder');
      return false;
    }
    if (nodeConfigSchemas.length === 0) {
      setWorkflowStatus('节点配置 schema 尚未加载，稍后再保存');
      return false;
    }
    const definition = serializeWorkflowDefinition(nodes, edges, nodeConfigSchemas, reactFlowInstance?.getViewport(), {
      workspaceId: placement.workspaceId,
      folderId: placement.folderId,
      activityId,
    });
    try {
      await fetchJSON(`/admin/v1/activities/${activityId}/workflow/draft`, {
        method: 'PUT',
        body: JSON.stringify({ operator: 'admin-flow', definition }),
      });
      setWorkflowStatus('草稿已保存');
      setLastAdded('保存草稿');
      return true;
    } catch (error) {
      setWorkflowStatus(`保存失败：${(error as Error).message}`);
      return false;
    }
  }, [activityId, activityPlacements, edges, nodeConfigSchemas, nodes, reactFlowInstance]);

  const validateCurrentDraft = useCallback(async (): Promise<WorkflowValidationResult | null> => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return null;
    }
    setShowChecklist(true);
    setValidatingWorkflow(true);
    try {
      const draftSaved = await saveDraft();
      if (!draftSaved) {
        return null;
      }
      const result = await fetchJSON<WorkflowValidationResult>(`/admin/v1/activities/${activityId}/workflow/draft/validate`, {
        method: 'POST',
      });
      setWorkflowValidation(result);
      if (result.valid) {
        setWorkflowStatus('发布检查通过');
      } else {
        setWorkflowStatus(`发布检查未通过：${result.summary.blockCount} 个阻塞项`);
      }
      return result;
    } catch (error) {
      setWorkflowStatus(`检查失败：${(error as Error).message}`);
      return null;
    } finally {
      setValidatingWorkflow(false);
    }
  }, [activityId, saveDraft]);

  const publishWorkflow = useCallback(async () => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return;
    }
    try {
      const validation = await validateCurrentDraft();
      if (!validation?.valid) {
        return;
      }
      await fetchJSON(`/admin/v1/activities/${activityId}/workflow/publish`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow' }),
      });
      await refreshWorkflowVersions(activityId);
      setIsPublished(true);
      setShowChecklist(true);
      setWorkflowStatus('已发布 workflow version');
      setLastAdded('发布版本');
    } catch (error) {
      setWorkflowStatus(`发布失败：${(error as Error).message}`);
    }
  }, [activityId, refreshWorkflowVersions, validateCurrentDraft]);

  const changeActivity = useCallback((nextActivityId: string) => {
    setActivityId(nextActivityId);
    setHomeWorkspaceId('');
    setHomeFolderId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setWorkflowValidation(null);
    setContextMenu(null);
    setNodePicker(null);
    setWorkflowStatus('切换活动中...');
  }, []);

  const navigateWorkspaceHome = useCallback((workspaceId: string, folderId = '') => {
    setHomeWorkspaceId(workspaceId);
    setHomeFolderId(folderId);
    setActivityId('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setContextMenu(null);
    setNodePicker(null);
    window.history.replaceState(null, '', window.location.pathname);
    setWorkflowStatus(folderId ? '已返回 folder 视图' : '已返回 workspace 视图');
  }, []);

  const createStudioWorkspace = useCallback(async (input: CreateStudioWorkspaceInput) => {
    const slug = normalizeActivityCode(input.name).toLowerCase().replace(/_/g, '-');
    const created = await fetchJSON<WorkspaceSummary>('/admin/v1/workspaces', {
      method: 'POST',
      body: JSON.stringify({
        tenantId: input.tenantId,
        name: input.name,
        slug,
        visibility: 'TEAM',
        defaultRole: 'VIEWER',
        ownerId: 'admin-flow',
        createdBy: 'admin-flow',
      }),
    });
    setWorkspaces((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    setWorkflowStatus(`已创建 workspace：${created.name}`);
    return created;
  }, []);

  const createWorkspaceFolder = useCallback(async (workspaceId: string, name: string) => {
    const workspace = workspaces.find((item) => item.id === workspaceId);
    const created = await fetchJSON<WorkspaceFolderSummary>(`/admin/v1/workspaces/${workspaceId}/folders`, {
      method: 'POST',
      body: JSON.stringify({
        name,
        visibility: 'TEAM',
        sortOrder: (workspaceFolders.filter((folder) => folder.workspaceId === workspaceId).length + 1) * 10,
        createdBy: 'admin-flow',
      }),
    });
    setWorkspaceFolders((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    if (workspace?.tenantId) {
      setWorkflowStatus(`已创建文件夹：${workspace.name} / ${created.name}`);
    }
    return created;
  }, [workspaceFolders, workspaces]);

  const createStudioActivity = useCallback(async (input: CreateStudioActivityInput) => {
    const payload = buildStudioActivityPayload(input);
    const created = await fetchJSON<ActivitySummary>('/admin/v1/activities', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    setActivities((items) => [created, ...items.filter((item) => item.id !== created.id)]);
    setActivityPlacements((items) => ({
      ...items,
      [created.id]: {
        workspaceId: created.workspaceId || input.workspaceId,
        folderId: created.folderId || input.folderId,
      },
    }));
    setActivityId(created.id);
    setActivityLabel(`${created.activityName} / ${created.activityCode}`);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    setContextMenu(null);
    setNodePicker(null);
    setNodes(baseNodes);
    setEdges(baseEdges);
    setSelectedTemplate('default');
    setWorkflowStatus('活动已创建，开始编排');
    setWorkflowVersionLabel('workflow 未发布');
    setIsPublished(false);
  }, [setEdges, setNodes]);

  const refreshRunStatus = useCallback(async () => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return;
    }
    try {
      const executions = await fetchJSON<{ items: WorkflowExecution[] }>(`/admin/v1/activities/${activityId}/workflow/executions?limit=1`);
      const latest = executions.items[0];
      if (!latest) {
        setWorkflowStatus('暂无运行日志');
        setNodes((items) => items.map((node) => (
          node.type === 'activity' ? { ...node, data: { ...node.data, runStatus: undefined, lastRun: undefined } } as FlowNode : node
        )));
        return;
      }
      const nodesBody = await fetchJSON<{ items: WorkflowNodeExecution[] }>(`/admin/v1/workflow/executions/${latest.id}/nodes`);
      const nodeRunById = new Map(nodesBody.items.map((item) => [item.nodeId, item]));
      setNodes((items) => items.map((node) => {
        if (node.type !== 'activity') return node;
        const run = nodeRunById.get(node.id);
        return {
          ...node,
          data: {
            ...node.data,
            runStatus: run?.status,
            lastRun: run ? {
              status: run.status,
              errorMessage: run.errorMessage,
              output: run.output,
              finishedAt: run.finishedAt,
            } : undefined,
          },
        } as FlowNode;
      }));
      setWorkflowStatus(`运行态：${latest.status} / v${latest.workflowVersion}`);
      setLastAdded('刷新运行态');
    } catch (error) {
      setWorkflowStatus(`运行态失败：${(error as Error).message}`);
    }
  }, [activityId, setNodes]);

  return (
    <div className="flow-shell">
      <NodePalette open={activityId ? showPalette : false} selectedTemplate={selectedTemplate} onAddNode={addCatalogNode} onDefault={resetDefault} onSimple={loadSimpleTemplate} />
      <main className={cx('studio', !activityId && 'studio-home')}>
        <StudioChrome
          homeMode={!activityId}
          showPalette={showPalette}
          interactionMode={interactionMode}
          nodeCount={nodes.length}
          edgeCount={edges.length}
          selectedCount={selectedCount}
          selectedNodeCount={selectedNodeIds.length}
          lastAdded={lastAdded}
          activities={activities}
          workspaces={workspaces}
          folders={workspaceFolders}
          placements={activityPlacements}
          activityId={activityId}
          activityLabel={activityLabel}
          workflowStatus={workflowStatus}
          workflowVersionLabel={workflowVersionLabel}
          zoomPercent={zoomPercent}
          showChecklist={showChecklist}
          validatingWorkflow={validatingWorkflow}
          workflowValidation={workflowValidation}
          isPublished={isPublished}
          onTogglePalette={() => setShowPalette((value) => !value)}
          onActivityChange={changeActivity}
          onWorkspaceNavigate={navigateWorkspaceHome}
          onModeChange={setInteractionMode}
          onDefault={resetDefault}
          onSimple={loadSimpleTemplate}
          onAutoLayout={autoLayout}
          onZoomIn={() => {
            reactFlowInstance?.zoomIn({ duration: 160 });
            window.setTimeout(syncZoom, 180);
          }}
          onZoomOut={() => {
            reactFlowInstance?.zoomOut({ duration: 160 });
            window.setTimeout(syncZoom, 180);
          }}
          onZoomTo={zoomTo}
          onFitView={() => {
            reactFlowInstance?.fitView({ padding: 0.14, duration: 260 });
            window.setTimeout(syncZoom, 280);
          }}
          showMiniMap={showMiniMap}
          onToggleMiniMap={() => setShowMiniMap((value) => !value)}
          onGroup={groupSelectedNodes}
          onDelete={deleteSelected}
          onClear={clearCanvas}
          onSaveDraft={saveDraft}
          onPreview={() => setLastAdded('Preview')}
          onRunHistory={refreshRunStatus}
          onToggleChecklist={() => {
            if (showChecklist) {
              setShowChecklist(false);
              return;
            }
            void validateCurrentDraft();
          }}
          onValidationIssueFocus={focusValidationIssue}
          onPublish={publishWorkflow}
        />
        <section className={cx('canvas-card', !activityId && 'studio-home-card')}>
          <ReactFlowProvider>
            {activityId ? (
              <>
            <ReactFlow
              nodes={visibleNodes}
              edges={visibleEdges}
              nodeTypes={nodeTypes}
              onInit={setReactFlowInstance}
              onMoveEnd={syncZoom}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onPaneClick={() => {
                closeFloatingUi();
                setSelectedNodeIds([]);
                setSelectedEdgeIds([]);
              }}
              onPaneContextMenu={openPaneMenu}
              onNodeClick={(event, node) => {
                closeFloatingUi();
                if (event.shiftKey || event.metaKey || event.ctrlKey) {
                  setSelectedNodeIds((items) => (items.includes(node.id) ? items.filter((item) => item !== node.id) : items.concat(node.id)));
                } else {
                  setSelectedNodeIds([node.id]);
                }
                setSelectedEdgeIds([]);
              }}
              onNodeContextMenu={openNodeMenu}
              onNodeMouseEnter={(_, node) => setHoveredNodeId(node.id)}
              onNodeMouseLeave={() => setHoveredNodeId('')}
              onEdgeClick={(event, edgeItem) => {
                event.stopPropagation();
                setSelectedEdgeIds([edgeItem.id]);
                setContextMenu({ type: 'edge', x: event.clientX, y: event.clientY, edgeId: edgeItem.id });
              }}
              onEdgeContextMenu={openEdgeMenu}
              onEdgeMouseEnter={(_, item) => setHoveredEdgeId(item.id)}
              onEdgeMouseLeave={() => setHoveredEdgeId('')}
              onNodeDrag={(_, node) => updateAlignmentGuide(node)}
              onNodeDragStop={() => setAlignmentGuide({})}
              onSelectionChange={({ nodes: selectedNodes, edges: selectedEdges }) => {
                setSelectedNodeIds(selectedNodes.map((node) => node.id));
                setSelectedEdgeIds(selectedEdges.map((item) => item.id));
              }}
              defaultViewport={{ x: 24, y: 36, zoom: 1 }}
              minZoom={0.25}
              maxZoom={1.6}
              panOnScroll
              panOnDrag={interactionMode === 'hand'}
              nodesDraggable
              selectionOnDrag={interactionMode === 'pointer'}
            >
              <Background color="#e7edf5" gap={16} size={1} />
              <Controls position="bottom-left" showInteractive={false} />
              {showMiniMap ? <MiniMap position="bottom-right" pannable zoomable nodeStrokeWidth={2} /> : null}
            </ReactFlow>
            {alignmentGuide.vertical ? (
              <div className={cx('align-guide vertical', alignmentGuide.vertical.kind)} style={{ left: alignmentGuide.vertical.screen, top: alignmentGuide.vertical.start, height: alignmentGuide.vertical.end - alignmentGuide.vertical.start }} />
            ) : null}
            {alignmentGuide.horizontal ? (
              <div className={cx('align-guide horizontal', alignmentGuide.horizontal.kind)} style={{ top: alignmentGuide.horizontal.screen, left: alignmentGuide.horizontal.start, width: alignmentGuide.horizontal.end - alignmentGuide.horizontal.start }} />
            ) : null}
            {contextMenu ? (
              <ContextMenu
                menu={contextMenu}
                onOpenPicker={() => {
                  if (contextMenu.type !== 'pane') return;
                  setNodeSearch('');
                  setNodePicker({ x: contextMenu.x + 160, y: contextMenu.y - 8, flowX: contextMenu.flowX, flowY: contextMenu.flowY });
                }}
                onAddNote={addNoteAt}
                onPreview={() => setLastAdded('Preview')}
                onExport={() => setLastAdded('Export DSL')}
                onImport={() => setLastAdded('Import App')}
                onDuplicateNode={duplicateNode}
                onDeleteNodes={deleteNodes}
                onDeleteEdges={deleteEdges}
              />
            ) : null}
            {nodePicker ? <NodePicker picker={nodePicker} search={nodeSearch} filteredCatalog={filteredCatalog} onSearch={setNodeSearch} onAdd={addNodeAt} /> : null}
              </>
            ) : (
              <StudioActivityStart
                activities={visibleStudioActivities}
                workspaces={workspaces}
                folders={workspaceFolders}
                placements={activityPlacements}
                hiddenTestActivityCount={hiddenTestActivityCount}
                workflowStatus={workflowStatus}
                initialWorkspaceId={homeWorkspaceId}
                initialFolderId={homeFolderId}
                onSelectActivity={changeActivity}
                onCreateWorkspace={createStudioWorkspace}
                onCreateActivity={createStudioActivity}
                onCreateFolder={createWorkspaceFolder}
              />
            )}
          </ReactFlowProvider>
        </section>
      </main>
      {activityId ? <PropertyPanel nodes={nodes} edges={edges} selectedNodeIds={selectedNodeIds} selectedEdgeIds={selectedEdgeIds} nodeConfigSchemas={nodeConfigSchemas} onNodeConfigChange={updateNodeConfig} onDeleteNodes={deleteNodes} onDeleteEdges={deleteEdges} /> : null}
    </div>
  );
}
