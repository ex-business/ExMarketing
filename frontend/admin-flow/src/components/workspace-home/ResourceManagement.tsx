import { useMemo, useState } from 'react';
import { Database, KeyRound, PlayCircle, Plus, RefreshCw, ShieldCheck } from 'lucide-react';
import type { ResourceAuditLogSummary, ResourceBindingSummary, ResourceOperation, ResourceSummary } from '../../types';
import { TaBadge, TaButton, TaEmptyState, TaInput, TaSelect, TaTab, TaTable, TaTableRow, TaTabs } from './TailAdminPrimitives';

type ResourceDraft = {
  code: string;
  name: string;
  description: string;
  type: string;
  driver: string;
  scope: string;
  status: string;
  environment: string;
  capabilitiesText: string;
  configText: string;
};

type BindingDraft = {
  workspaceId: string;
  folderId: string;
  activityId: string;
  nodeType: string;
  nodeId: string;
  minRole: string;
  allowedOperationsText: string;
  allowedNamespacesText: string;
  allowedPrefixesText: string;
  deniedPrefixesText: string;
  includeSubfolders: boolean;
};

type Props = {
  tenantId: string;
  workspaceId?: string;
  folderId?: string;
  resources: ResourceSummary[];
  selectedResourceCode: string;
  selectedResource?: ResourceSummary;
  bindings: ResourceBindingSummary[];
  audits: ResourceAuditLogSummary[];
  loading?: boolean;
  message?: string;
  onRefresh: () => void;
  onSelectResource: (code: string) => void;
  onSaveResource: (resource: Partial<ResourceSummary> & { code: string }) => Promise<void>;
  onSaveBinding: (resourceCode: string, binding: Partial<ResourceBindingSummary>) => Promise<void>;
  onTestResource: (resourceCode: string) => Promise<void>;
};

const RESOURCE_TYPES = ['MEMORY', 'WORKFLOW_CONTEXT', 'DATABASE_KV', 'DATABASE_TABLE', 'REDIS_KV', 'REDIS_HASH', 'REDIS_COUNTER', 'REDIS_LOCK', 'HTTP_PROVIDER', 'GRPC_PROVIDER', 'MQ_TOPIC', 'OBJECT_STORAGE', 'PLUGIN_PROVIDER', 'CUSTOM_PROVIDER'];
const RESOURCE_DRIVERS = ['INTERNAL', 'POSTGRES', 'HTTP', 'REST', 'REDIS', 'GRPC', 'PLUGIN', 'CUSTOM'];
const RESOURCE_SCOPES = ['TENANT', 'WORKSPACE', 'FOLDER', 'ACTIVITY', 'NODE'];
const RESOURCE_STATUSES = ['ACTIVE', 'DRAFT', 'DISABLED'];
const RESOURCE_OPERATIONS: ResourceOperation[] = ['READ', 'WRITE', 'UPSERT', 'APPEND', 'DELETE', 'CAS', 'CALL', 'PUBLISH', 'TEST_CONNECTION', 'CHECK_AND_INCREMENT'];
const WORKSPACE_ROLES = ['GUEST', 'VIEWER', 'EDITOR', 'ADMIN', 'OWNER'];

function splitList(value: string): string[] {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

function parseConfig(text: string): Record<string, unknown> {
  const trimmed = text.trim();
  if (!trimmed) return {};
  const parsed = JSON.parse(trimmed);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('配置必须是 JSON object');
  }
  return parsed as Record<string, unknown>;
}

function resourceDraft(resource: ResourceSummary | undefined, tenantId: string, workspaceId?: string, folderId?: string): ResourceDraft {
  return {
    code: resource?.code ?? '',
    name: resource?.name ?? '',
    description: resource?.description ?? '',
    type: resource?.type ?? 'DATABASE_KV',
    driver: resource?.driver ?? 'POSTGRES',
    scope: resource?.scope && resource.scope !== 'SYSTEM' ? resource.scope : 'WORKSPACE',
    status: resource?.status ?? 'ACTIVE',
    environment: resource?.environment ?? 'DEV',
    capabilitiesText: (resource?.capabilities?.length ? resource.capabilities : ['READ', 'UPSERT']).join('\n'),
    configText: JSON.stringify(resource?.config ?? { table: 'marketing_storage_state', tenantId, workspaceId, folderId }, null, 2),
  };
}

function statusTone(status: string): 'success' | 'warning' | 'error' | 'gray' {
  if (status === 'ACTIVE' || status === 'SUCCESS') return 'success';
  if (status === 'DRAFT') return 'warning';
  if (status === 'FAILED' || status === 'DISABLED') return 'error';
  return 'gray';
}

function formatDate(value?: string): string {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString();
}

export function ResourceManagement({
  tenantId,
  workspaceId,
  folderId,
  resources,
  selectedResourceCode,
  selectedResource,
  bindings,
  audits,
  loading = false,
  message = '',
  onRefresh,
  onSelectResource,
  onSaveResource,
  onSaveBinding,
  onTestResource,
}: Props) {
  const [tab, setTab] = useState<'resource' | 'permission' | 'audit'>('resource');
  const [query, setQuery] = useState('');
  const [draft, setDraft] = useState<ResourceDraft>(() => resourceDraft(selectedResource, tenantId, workspaceId, folderId));
  const [bindingDraft, setBindingDraft] = useState<BindingDraft>({
    workspaceId: workspaceId ?? '',
    folderId: folderId ?? '',
    activityId: '',
    nodeType: 'state-store',
    nodeId: '',
    minRole: 'EDITOR',
    allowedOperationsText: 'READ\nUPSERT',
    allowedNamespacesText: 'workflow\ncheckin',
    allowedPrefixesText: '',
    deniedPrefixesText: '',
    includeSubfolders: true,
  });
  const [error, setError] = useState('');

  const filteredResources = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return resources.filter((resource) => !keyword || `${resource.code} ${resource.name} ${resource.type} ${resource.scope}`.toLowerCase().includes(keyword));
  }, [query, resources]);

  const selectResource = (resource: ResourceSummary) => {
    onSelectResource(resource.code);
    setDraft(resourceDraft(resource, tenantId, workspaceId, folderId));
    setError('');
  };

  const startCreate = () => {
    onSelectResource('');
    setDraft(resourceDraft(undefined, tenantId, workspaceId, folderId));
    setTab('resource');
    setError('');
  };

  const submitResource = async () => {
    try {
      setError('');
      const config = parseConfig(draft.configText);
      await onSaveResource({
        code: draft.code.trim(),
        name: draft.name.trim(),
        description: draft.description.trim(),
        tenantId,
        workspaceId: draft.scope === 'WORKSPACE' || draft.scope === 'FOLDER' ? workspaceId : '',
        folderId: draft.scope === 'FOLDER' ? folderId : '',
        type: draft.type,
        driver: draft.driver,
        scope: draft.scope,
        status: draft.status,
        environment: draft.environment.trim(),
        capabilities: splitList(draft.capabilitiesText) as ResourceOperation[],
        config,
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const submitBinding = async () => {
    if (!selectedResource) {
      setError('先选择资源');
      return;
    }
    try {
      setError('');
      await onSaveBinding(selectedResource.code, {
        tenantId,
        workspaceId: bindingDraft.workspaceId.trim(),
        folderId: bindingDraft.folderId.trim(),
        activityId: bindingDraft.activityId.trim(),
        nodeType: bindingDraft.nodeType.trim(),
        nodeId: bindingDraft.nodeId.trim(),
        minRole: bindingDraft.minRole,
        allowedOperations: splitList(bindingDraft.allowedOperationsText) as ResourceOperation[],
        allowedNamespaces: splitList(bindingDraft.allowedNamespacesText),
        includeSubfolders: bindingDraft.includeSubfolders,
        status: 'ACTIVE',
        keyPolicy: {
          allowedPrefixes: splitList(bindingDraft.allowedPrefixesText),
          deniedPrefixes: splitList(bindingDraft.deniedPrefixesText),
        },
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <div className="resource-manager">
      <header className="resource-manager-head">
        <div>
          <b>资源管理</b>
          <span>统一维护节点可引用的存储、Provider、Topic、权限和审计。</span>
        </div>
        <div>
          <TaButton onClick={onRefresh}><RefreshCw size={15} />刷新</TaButton>
          <TaButton tone="primary" onClick={startCreate}><Plus size={15} />新增资源</TaButton>
        </div>
      </header>
      {message ? <p className="resource-manager-message">{message}</p> : null}
      {error ? <p className="resource-manager-error">{error}</p> : null}
      <section className="resource-manager-grid">
        <aside className="resource-list">
          <label>
            <span>搜索资源</span>
            <TaInput value={query} placeholder="code / name / type" onChange={(event) => setQuery(event.target.value)} />
          </label>
          <div>
            {loading ? <TaEmptyState>资源加载中...</TaEmptyState> : filteredResources.length ? filteredResources.map((resource) => (
              <button key={resource.id || resource.code} type="button" className={resource.code === selectedResourceCode ? 'active' : ''} onClick={() => selectResource(resource)}>
                <i>{resource.type.slice(0, 1)}</i>
                <span>
                  <b>{resource.name || resource.code}</b>
                  <em>{resource.code}</em>
                </span>
                <TaBadge tone={statusTone(resource.status)}>{resource.status}</TaBadge>
              </button>
            )) : <TaEmptyState>暂无资源</TaEmptyState>}
          </div>
        </aside>
        <main className="resource-detail">
          <TaTabs>
            <TaTab active={tab === 'resource'} onClick={() => setTab('resource')}><Database size={15} />资源定义</TaTab>
            <TaTab active={tab === 'permission'} onClick={() => setTab('permission')} disabled={!selectedResource}><ShieldCheck size={15} />权限绑定</TaTab>
            <TaTab active={tab === 'audit'} onClick={() => setTab('audit')} disabled={!selectedResource}><KeyRound size={15} />审计</TaTab>
          </TaTabs>
          {tab === 'resource' ? (
            <div className="resource-form">
              <label><span>Code</span><TaInput value={draft.code} placeholder="workflow_state_store" onChange={(event) => setDraft((item) => ({ ...item, code: event.target.value }))} /></label>
              <label><span>名称</span><TaInput value={draft.name} placeholder="工作流状态存储" onChange={(event) => setDraft((item) => ({ ...item, name: event.target.value }))} /></label>
              <label><span>类型</span><TaSelect value={draft.type} onChange={(event) => setDraft((item) => ({ ...item, type: event.target.value }))}>{RESOURCE_TYPES.map((type) => <option key={type}>{type}</option>)}</TaSelect></label>
              <label><span>Driver</span><TaSelect value={draft.driver} onChange={(event) => setDraft((item) => ({ ...item, driver: event.target.value }))}>{RESOURCE_DRIVERS.map((driver) => <option key={driver}>{driver}</option>)}</TaSelect></label>
              <label><span>Scope</span><TaSelect value={draft.scope} onChange={(event) => setDraft((item) => ({ ...item, scope: event.target.value }))}>{RESOURCE_SCOPES.map((scope) => <option key={scope}>{scope}</option>)}</TaSelect></label>
              <label><span>Status</span><TaSelect value={draft.status} onChange={(event) => setDraft((item) => ({ ...item, status: event.target.value }))}>{RESOURCE_STATUSES.map((status) => <option key={status}>{status}</option>)}</TaSelect></label>
              <label><span>环境</span><TaInput value={draft.environment} placeholder="DEV / STAGING / PROD" onChange={(event) => setDraft((item) => ({ ...item, environment: event.target.value }))} /></label>
              <label><span>描述</span><TaInput value={draft.description} onChange={(event) => setDraft((item) => ({ ...item, description: event.target.value }))} /></label>
              <label className="wide"><span>能力</span><textarea value={draft.capabilitiesText} onChange={(event) => setDraft((item) => ({ ...item, capabilitiesText: event.target.value }))} /></label>
              <label className="wide"><span>Config JSON</span><textarea value={draft.configText} spellCheck={false} onChange={(event) => setDraft((item) => ({ ...item, configText: event.target.value }))} /></label>
              <div className="resource-actions">
                <TaButton tone="primary" onClick={submitResource}>保存资源</TaButton>
                <TaButton disabled={!selectedResource} onClick={() => selectedResource && onTestResource(selectedResource.code)}><PlayCircle size={15} />测试连接</TaButton>
              </div>
            </div>
          ) : tab === 'permission' ? (
            <div className="resource-permission">
              <section className="resource-binding-form">
                <label><span>Workspace</span><TaInput value={bindingDraft.workspaceId} onChange={(event) => setBindingDraft((item) => ({ ...item, workspaceId: event.target.value }))} /></label>
                <label><span>Folder</span><TaInput value={bindingDraft.folderId} onChange={(event) => setBindingDraft((item) => ({ ...item, folderId: event.target.value }))} /></label>
                <label><span>Activity</span><TaInput value={bindingDraft.activityId} onChange={(event) => setBindingDraft((item) => ({ ...item, activityId: event.target.value }))} /></label>
                <label><span>Node Type</span><TaInput value={bindingDraft.nodeType} onChange={(event) => setBindingDraft((item) => ({ ...item, nodeType: event.target.value }))} /></label>
                <label><span>Node ID</span><TaInput value={bindingDraft.nodeId} onChange={(event) => setBindingDraft((item) => ({ ...item, nodeId: event.target.value }))} /></label>
                <label><span>最低角色</span><TaSelect value={bindingDraft.minRole} onChange={(event) => setBindingDraft((item) => ({ ...item, minRole: event.target.value }))}>{WORKSPACE_ROLES.map((role) => <option key={role}>{role}</option>)}</TaSelect></label>
                <label className="wide"><span>允许操作</span><textarea value={bindingDraft.allowedOperationsText} onChange={(event) => setBindingDraft((item) => ({ ...item, allowedOperationsText: event.target.value }))} /></label>
                <label className="wide"><span>允许 namespace</span><textarea value={bindingDraft.allowedNamespacesText} onChange={(event) => setBindingDraft((item) => ({ ...item, allowedNamespacesText: event.target.value }))} /></label>
                <label className="wide"><span>允许 key 前缀</span><textarea value={bindingDraft.allowedPrefixesText} placeholder="site_001:checkin:" onChange={(event) => setBindingDraft((item) => ({ ...item, allowedPrefixesText: event.target.value }))} /></label>
                <label className="wide"><span>拒绝 key 前缀</span><textarea value={bindingDraft.deniedPrefixesText} onChange={(event) => setBindingDraft((item) => ({ ...item, deniedPrefixesText: event.target.value }))} /></label>
                <label className="resource-checkbox"><input type="checkbox" checked={bindingDraft.includeSubfolders} onChange={(event) => setBindingDraft((item) => ({ ...item, includeSubfolders: event.target.checked }))} /><span>包含子文件夹</span></label>
                <div className="resource-actions"><TaButton tone="primary" onClick={submitBinding}>保存权限绑定</TaButton></div>
              </section>
              <TaTable className="resource-binding-table">
                <TaTableRow header><span>范围</span><span>节点</span><span>操作</span><span>角色</span><span>状态</span></TaTableRow>
                {bindings.map((binding) => (
                  <TaTableRow key={binding.id}>
                    <span>{binding.workspaceId || '*'} / {binding.folderId || '*'}</span>
                    <span>{binding.nodeType || '*'} / {binding.nodeId || '*'}</span>
                    <span>{binding.allowedOperations?.join(', ') || '*'}</span>
                    <span>{binding.minRole || '-'}</span>
                    <span>{binding.status}</span>
                  </TaTableRow>
                ))}
              </TaTable>
            </div>
          ) : (
            <TaTable className="resource-audit-table">
              <TaTableRow header><span>时间</span><span>节点</span><span>操作</span><span>状态</span><span>错误</span></TaTableRow>
              {audits.length ? audits.map((audit) => (
                <TaTableRow key={audit.id}>
                  <span>{formatDate(audit.createdAt)}</span>
                  <span>{audit.nodeType || '-'} / {audit.nodeId || '-'}</span>
                  <span>{audit.operation}</span>
                  <span><TaBadge tone={statusTone(audit.status)}>{audit.status}</TaBadge></span>
                  <span>{audit.errorCode || audit.errorMessage || '-'}</span>
                </TaTableRow>
              )) : <TaEmptyState>暂无审计记录</TaEmptyState>}
            </TaTable>
          )}
        </main>
      </section>
    </div>
  );
}
