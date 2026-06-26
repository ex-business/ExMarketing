import { useEffect, useMemo, useState } from 'react';
import { Activity, DatabaseZap, FileJson, Gauge, GitCompare, History, Link2, Network, PlayCircle, Plus, RefreshCw, RotateCcw, Route, ServerCog, ShieldAlert } from 'lucide-react';
import type { DBMigrationAnalysis, DBMigrationApplyResponse, ResourceSummary, ServiceCallAuditLogSummary, ServiceDefinitionSummary, ServiceInterfaceBindingCheckResult, ServiceInterfaceBindingSummary, ServiceInterfaceSummary, ServiceProviderRuntimeStateSummary, ServiceProviderSummary, ServiceRateLimitPolicySummary, ServiceSchemaSummary, WorkflowServiceInterfaceMigrationAnalysis, WorkflowServiceInterfaceMigrationApplyResponse } from '../../types';
import { TaBadge, TaButton, TaEmptyState, TaInput, TaSelect, TaTab, TaTabs } from './TailAdminPrimitives';

type ServiceDraft = {
  code: string;
  name: string;
  description: string;
  serviceType: string;
  scope: string;
  status: string;
  owner: string;
  tagsText: string;
};

type InterfaceDraft = {
  code: string;
  name: string;
  description: string;
  operation: string;
  inputSchemaRef: string;
  outputSchemaRef: string;
  errorSchemaRef: string;
  idempotencyKeyTemplate: string;
  timeoutMs: string;
  retryPolicy: string;
  luaCallable: boolean;
  status: string;
};

type ProviderDraft = {
  code: string;
  name: string;
  description: string;
  protocol: string;
  endpoint: string;
  topic: string;
  method: string;
  path: string;
  authRef: string;
  resourceRefsText: string;
  configText: string;
  priority: string;
  environment: string;
  status: string;
};

type SchemaDraft = {
  schemaRef: string;
  schemaType: string;
  scope: string;
  version: string;
  status: string;
  description: string;
  schemaJsonText: string;
  examplesJsonText: string;
};

type BindingDraft = {
  serviceRef: string;
  interfaceRef: string;
  scope: string;
  principalType: string;
  principalId: string;
  allowedNodeTypesText: string;
  allowedOperationsText: string;
  allowedProviderPoliciesText: string;
  allowExplicitProviderRef: boolean;
  allowedProviderRefsText: string;
  includeSubfolders: boolean;
  minRole: string;
  environment: string;
  rateLimitRef: string;
  deletedAt: string;
  luaCallable: boolean;
  requireApproval: boolean;
  deny: boolean;
  status: string;
};

type RateLimitDraft = {
  code: string;
  name: string;
  description: string;
  serviceRef: string;
  interfaceRef: string;
  providerRef: string;
  backend: string;
  limit: string;
  windowSeconds: string;
  burst: string;
  status: string;
  metadataText: string;
};

type DBMigrationApplyInput = {
  dryRun: boolean;
  confirm: boolean;
  confirmation: string;
  breakGlass: boolean;
  breakGlassScope?: string;
  breakGlassActor?: string;
  breakGlassExpiresAt?: string;
  breakGlassTicket?: string;
  reason: string;
};

type Props = {
  tenantId: string;
  actorId?: string;
  activityId?: string;
  workflowMigrationTargets?: Array<{ id: string; label: string }>;
  workspaceId?: string;
  folderId?: string;
  resources: ResourceSummary[];
  services: ServiceDefinitionSummary[];
  interfaces: ServiceInterfaceSummary[];
  providers: ServiceProviderSummary[];
  schemas: ServiceSchemaSummary[];
  bindings: ServiceInterfaceBindingSummary[];
  audits: ServiceCallAuditLogSummary[];
  runtimeStates: Record<string, ServiceProviderRuntimeStateSummary | undefined>;
  rateLimitPolicies: ServiceRateLimitPolicySummary[];
  migrationAnalysis?: DBMigrationAnalysis;
  migrationMessage?: string;
  workflowMigrationAnalysis?: WorkflowServiceInterfaceMigrationAnalysis;
  workflowMigrationMessage?: string;
  selectedServiceCode: string;
  selectedInterfaceCode: string;
  selectedService?: ServiceDefinitionSummary;
  selectedInterface?: ServiceInterfaceSummary;
  loading?: boolean;
  message?: string;
  onRefresh: () => void;
  onSelectService: (code: string) => void;
  onSelectInterface: (code: string) => void;
  onSaveService: (service: Partial<ServiceDefinitionSummary> & { code: string }) => Promise<void>;
  onSaveInterface: (serviceCode: string, item: Partial<ServiceInterfaceSummary> & { code: string }) => Promise<void>;
  onSaveProvider: (serviceCode: string, interfaceCode: string, provider: Partial<ServiceProviderSummary> & { code: string }) => Promise<void>;
  onSaveSchema: (schema: Partial<ServiceSchemaSummary> & { schemaRef: string }) => Promise<void>;
  onSaveBinding: (binding: Partial<ServiceInterfaceBindingSummary>) => Promise<void>;
  onSaveRateLimitPolicy: (policy: Partial<ServiceRateLimitPolicySummary> & { code: string }) => Promise<void>;
  onResetProviderCircuit: (serviceCode: string, interfaceCode: string, providerCode: string) => Promise<void>;
  onCheckProviderHealth: (serviceCode: string, interfaceCode: string, providerCode: string) => Promise<void>;
  onValidateSchema: (schema: Partial<ServiceSchemaSummary> & { schemaRef: string }) => Promise<{ valid?: boolean; compatible?: boolean; code?: string; error?: string }>;
  onValidateSchemaExample: (schema: Partial<ServiceSchemaSummary> & { schemaRef: string }, example: unknown) => Promise<{ valid?: boolean; code?: string; error?: string }>;
  onCheckSchemaCompatibility: (schema: Partial<ServiceSchemaSummary> & { schemaRef: string }) => Promise<{ compatible?: boolean; code?: string; error?: string }>;
  onCheckBinding: (binding: Partial<ServiceInterfaceBindingSummary>) => Promise<ServiceInterfaceBindingCheckResult>;
  auditNextCursor?: string;
  auditRequestId?: string;
  onLoadMoreAudits: () => Promise<void>;
  onTestInterface: (serviceCode: string, interfaceCode: string) => Promise<void>;
  onRefreshMigrationStatus: () => Promise<DBMigrationAnalysis>;
  onAnalyzeMigrations: () => Promise<DBMigrationAnalysis>;
  onApplyMigrations: (input: DBMigrationApplyInput) => Promise<DBMigrationApplyResponse>;
  onAnalyzeWorkflowMigration: (activityId: string) => Promise<WorkflowServiceInterfaceMigrationAnalysis>;
  onApplyWorkflowMigration: (activityId: string, input: { dryRun: boolean; confirm: boolean; confirmation: string; paths: string[]; reason: string }) => Promise<WorkflowServiceInterfaceMigrationApplyResponse>;
};

const SERVICE_TYPES = ['EXTERNAL', 'INTERNAL', 'SYSTEM'];
const SERVICE_SCOPES = ['TENANT', 'WORKSPACE', 'FOLDER', 'ACTIVITY', 'NODE'];
const STATUSES = ['ACTIVE', 'DRAFT', 'DISABLED'];
const INTERFACE_OPERATIONS = ['CALL', 'PUBLISH', 'REQUEST_REPLY', 'SUBSCRIBE_TRIGGER'];
const PROVIDER_PROTOCOLS = ['RESOURCE', 'HTTP', 'RPC', 'MQ', 'PLUGIN', 'TCP', 'LUA_ADAPTER'];
const SCHEMA_TYPES = ['INPUT', 'OUTPUT', 'ERROR'];
const RATE_LIMIT_BACKENDS = ['POSTGRES', 'REDIS'];
const PRINCIPAL_TYPES = ['ROLE', 'USER', 'GROUP', 'SERVICE_ACCOUNT', 'WORKSPACE', 'FOLDER', 'ACTIVITY', 'NODE'];
const PROVIDER_POLICIES = ['PRIMARY', 'PRIMARY_WITH_FALLBACK', 'CACHE_ASIDE', 'WEIGHTED', 'WEIGHTED_WITH_FALLBACK', 'ENVIRONMENT_MATCH', 'MOCK_ONLY'];

function FieldInput({ label, value, onChange, placeholder = '' }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string }) {
  return (
    <label className="ta-field">
      <span>{label}</span>
      <TaInput value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

function FieldSelect({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (value: string) => void }) {
  return (
    <label className="ta-field">
      <span>{label}</span>
      <TaSelect value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => <option key={option} value={option}>{option}</option>)}
      </TaSelect>
    </label>
  );
}

function splitList(value: string): string[] {
  return value.split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

function parseObject(text: string): Record<string, unknown> {
  const trimmed = text.trim();
  if (!trimmed) return {};
  const parsed = JSON.parse(trimmed);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('配置必须是 JSON object');
  return parsed as Record<string, unknown>;
}

function parseJson(text: string, fallback: unknown): unknown {
  const trimmed = text.trim();
  if (!trimmed) return fallback;
  return JSON.parse(trimmed) as unknown;
}

function parseIntegerField(value: string, label: string, min: number): number {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < min) {
    throw new Error(`${label} 必须是${min > 0 ? '正整数' : '非负整数'}`);
  }
  return parsed;
}

function serviceDraft(service: ServiceDefinitionSummary | undefined, workspaceId?: string, folderId?: string): ServiceDraft {
  return {
    code: service?.code ?? '',
    name: service?.name ?? '',
    description: service?.description ?? '',
    serviceType: service?.serviceType ?? 'EXTERNAL',
    scope: service?.scope && service.scope !== 'SYSTEM' ? service.scope : 'WORKSPACE',
    status: service?.status ?? 'ACTIVE',
    owner: service?.owner ?? '',
    tagsText: service?.tags?.join('\n') ?? 'fact',
  };
}

function interfaceDraft(item: ServiceInterfaceSummary | undefined): InterfaceDraft {
  return {
    code: item?.code ?? '',
    name: item?.name ?? '',
    description: item?.description ?? '',
    operation: item?.operation ?? 'CALL',
    inputSchemaRef: item?.inputSchemaRef ?? '',
    outputSchemaRef: item?.outputSchemaRef ?? '',
    errorSchemaRef: item?.errorSchemaRef ?? '',
    idempotencyKeyTemplate: item?.idempotencyKeyTemplate ?? '${tenantId}:${subject.id}',
    timeoutMs: String(item?.timeoutMs ?? 3000),
    retryPolicy: item?.retryPolicy ?? 'STANDARD',
    luaCallable: item?.luaCallable ?? true,
    status: item?.status ?? 'ACTIVE',
  };
}

function providerDraft(provider: ServiceProviderSummary | undefined, resources: ResourceSummary[]): ProviderDraft {
  const targetRef = provider?.resourceRefs?.[0] || resources.find((item) => item.type !== 'PROVIDER')?.code || '';
  return {
    code: provider?.code ?? '',
    name: provider?.name ?? '',
    description: provider?.description ?? '',
    protocol: provider?.protocol ?? 'RESOURCE',
    endpoint: provider?.endpoint ?? '',
    topic: provider?.topic ?? '',
    method: provider?.method ?? 'POST',
    path: provider?.path ?? '',
    authRef: provider?.authRef ?? '',
    resourceRefsText: provider?.resourceRefs?.join('\n') ?? targetRef,
    configText: JSON.stringify(provider?.config ?? (targetRef ? { targetResourceRef: targetRef } : {}), null, 2),
    priority: String(provider?.priority ?? 0),
    environment: provider?.environment ?? 'DEV',
    status: provider?.status ?? 'ACTIVE',
  };
}

function schemaDraft(schema?: ServiceSchemaSummary): SchemaDraft {
  return {
    schemaRef: schema?.schemaRef ?? '',
    schemaType: schema?.schemaType ?? 'INPUT',
    scope: schema?.scope && schema.scope !== 'SYSTEM' ? schema.scope : 'WORKSPACE',
    version: String(schema?.version ?? 1),
    status: schema?.status ?? 'ACTIVE',
    description: schema?.description ?? '',
    schemaJsonText: JSON.stringify(schema?.schemaJson ?? {
      type: 'object',
      properties: {},
      required: [],
      additionalProperties: true,
    }, null, 2),
    examplesJsonText: JSON.stringify(schema?.examplesJson ?? [], null, 2),
  };
}

function bindingDraft(binding: ServiceInterfaceBindingSummary | undefined, serviceCode: string, interfaceCode: string, workspaceId?: string, folderId?: string): BindingDraft {
  return {
    serviceRef: binding?.serviceRef ?? serviceCode,
    interfaceRef: binding?.interfaceRef ?? interfaceCode,
    scope: binding?.scope ?? (folderId ? 'FOLDER' : workspaceId ? 'WORKSPACE' : 'TENANT'),
    principalType: binding?.principalType ?? 'ROLE',
    principalId: binding?.principalId ?? 'EDITOR',
    allowedNodeTypesText: binding?.allowedNodeTypes?.join('\n') ?? 'fact-query',
    allowedOperationsText: binding?.allowedOperations?.join('\n') ?? 'CALL',
    allowedProviderPoliciesText: binding?.allowedProviderPolicies?.join('\n') ?? 'PRIMARY\nPRIMARY_WITH_FALLBACK',
    allowExplicitProviderRef: binding?.allowExplicitProviderRef ?? false,
    allowedProviderRefsText: binding?.allowedProviderRefs?.join('\n') ?? '',
    includeSubfolders: binding?.includeSubfolders ?? false,
    minRole: binding?.minRole ?? '',
    environment: binding?.environment ?? '',
    rateLimitRef: binding?.rateLimitRef ?? '',
    deletedAt: binding?.deletedAt ?? '',
    luaCallable: binding?.luaCallable ?? false,
    requireApproval: binding?.requireApproval ?? false,
    deny: binding?.deny ?? false,
    status: binding?.status ?? 'ACTIVE',
  };
}

function rateLimitDraft(policy: ServiceRateLimitPolicySummary | undefined, serviceCode: string, interfaceCode: string, providerCode = ''): RateLimitDraft {
  const metadata = policy?.metadata ?? {};
  return {
    code: policy?.code ?? '',
    name: policy?.name ?? '',
    description: policy?.description ?? '',
    serviceRef: policy?.serviceRef ?? serviceCode,
    interfaceRef: policy?.interfaceRef ?? interfaceCode,
    providerRef: policy?.providerRef ?? providerCode,
    backend: policy?.backend ?? 'POSTGRES',
    limit: String(policy?.limit ?? 100),
    windowSeconds: String(policy?.windowSeconds ?? 60),
    burst: metadata.burst == null ? '' : String(metadata.burst),
    status: policy?.status ?? 'ACTIVE',
    metadataText: JSON.stringify(metadata, null, 2),
  };
}

function statusTone(status: string): 'success' | 'warning' | 'error' | 'gray' {
  if (status === 'ACTIVE') return 'success';
  if (status === 'DRAFT') return 'warning';
  if (status === 'DISABLED') return 'error';
  return 'gray';
}

function isActiveAsset(item?: { status?: string; deletedAt?: string }): boolean {
  return Boolean(item && item.status === 'ACTIVE' && !item.deletedAt);
}

function assetRisk(item?: { status?: string; deletedAt?: string }): string {
  if (!item) return 'missing';
  if (item.deletedAt) return 'deleted';
  if (item.status !== 'ACTIVE') return String(item.status || 'disabled').toLowerCase();
  return '';
}

function rateLimitPolicyLabel(policy: ServiceRateLimitPolicySummary): string {
  const target = [policy.serviceRef || '*', policy.interfaceRef || '*', policy.providerRef || '*'].join('/');
  return `${policy.code} · ${target} · ${policy.limit}/${policy.windowSeconds}s · ${policy.backend}`;
}

function workflowMigrationTone(analysis?: WorkflowServiceInterfaceMigrationAnalysis): 'success' | 'warning' | 'error' | 'gray' {
  if (!analysis) return 'gray';
  if (analysis.status === 'BLOCKED' || analysis.summary.blockers > 0) return 'error';
  if (analysis.status === 'NEEDS_REVIEW' || analysis.summary.manualReview > 0) return 'warning';
  if (analysis.summary.legacySteps === 0) return 'success';
  return 'warning';
}

function migrationTone(analysis?: DBMigrationAnalysis): 'success' | 'warning' | 'error' | 'gray' {
  if (!analysis) return 'gray';
  if (analysis.drift || analysis.errors?.length) return 'error';
  if (!analysis.latest || analysis.pending?.length || analysis.missingRequiredTables?.length) return 'warning';
  return 'success';
}

export function ServiceCapabilityManagement({
  tenantId,
  actorId = '',
  activityId = '',
  workflowMigrationTargets = [],
  workspaceId,
  folderId,
  resources,
  services,
  interfaces,
  providers,
  schemas,
  bindings,
  audits,
  runtimeStates,
  rateLimitPolicies,
  migrationAnalysis,
  migrationMessage = '',
  workflowMigrationAnalysis,
  workflowMigrationMessage = '',
  selectedServiceCode,
  selectedInterfaceCode,
  selectedService,
  selectedInterface,
  loading = false,
  message = '',
  onRefresh,
  onSelectService,
  onSelectInterface,
  onSaveService,
  onSaveInterface,
  onSaveProvider,
  onSaveSchema,
  onSaveBinding,
  onSaveRateLimitPolicy,
  onResetProviderCircuit,
  onCheckProviderHealth,
  onValidateSchema,
  onValidateSchemaExample,
  onCheckSchemaCompatibility,
  onCheckBinding,
  auditNextCursor = '',
  auditRequestId = '',
  onLoadMoreAudits,
  onTestInterface,
  onRefreshMigrationStatus,
  onAnalyzeMigrations,
  onApplyMigrations,
  onAnalyzeWorkflowMigration,
  onApplyWorkflowMigration,
}: Props) {
  const [tab, setTab] = useState<'overview' | 'service' | 'interface' | 'provider' | 'schema' | 'binding' | 'rateLimit' | 'audit' | 'migration'>('overview');
  const [query, setQuery] = useState('');
  const [serviceForm, setServiceForm] = useState<ServiceDraft>(() => serviceDraft(selectedService, workspaceId, folderId));
  const [interfaceForm, setInterfaceForm] = useState<InterfaceDraft>(() => interfaceDraft(selectedInterface));
  const [providerForm, setProviderForm] = useState<ProviderDraft>(() => providerDraft(providers[0], resources));
  const [schemaForm, setSchemaForm] = useState<SchemaDraft>(() => schemaDraft());
  const [bindingForm, setBindingForm] = useState<BindingDraft>(() => bindingDraft(undefined, selectedServiceCode, selectedInterfaceCode, workspaceId, folderId));
  const [rateLimitForm, setRateLimitForm] = useState<RateLimitDraft>(() => rateLimitDraft(undefined, selectedServiceCode, selectedInterfaceCode));
  const [error, setError] = useState('');
  const [schemaCheck, setSchemaCheck] = useState('');
  const [schemaCompatibilityCheck, setSchemaCompatibilityCheck] = useState('');
  const [schemaExampleText, setSchemaExampleText] = useState('{}');
  const [schemaExampleCheck, setSchemaExampleCheck] = useState('');
  const [bindingCheck, setBindingCheck] = useState<ServiceInterfaceBindingCheckResult | undefined>();
  const [migrationApplyConfirm, setMigrationApplyConfirm] = useState('');
  const [migrationApplyReason, setMigrationApplyReason] = useState('');
  const [migrationBreakGlass, setMigrationBreakGlass] = useState(false);
  const [migrationBreakGlassScope, setMigrationBreakGlassScope] = useState('db-migration:apply');
  const [migrationBreakGlassActor, setMigrationBreakGlassActor] = useState('');
  const [migrationBreakGlassTicket, setMigrationBreakGlassTicket] = useState('');
  const [migrationBreakGlassExpiresAt, setMigrationBreakGlassExpiresAt] = useState('');
  const [migrationActionMessage, setMigrationActionMessage] = useState('');
  const [workflowMigrationActivityId, setWorkflowMigrationActivityId] = useState(activityId || workflowMigrationTargets[0]?.id || '');
  const [workflowMigrationConfirm, setWorkflowMigrationConfirm] = useState('');
  const [workflowMigrationReason, setWorkflowMigrationReason] = useState('');
  const [workflowMigrationActionMessage, setWorkflowMigrationActionMessage] = useState('');
  const [selectedWorkflowMigrationPaths, setSelectedWorkflowMigrationPaths] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    setServiceForm(serviceDraft(selectedService, workspaceId, folderId));
  }, [selectedService, workspaceId, folderId]);

  useEffect(() => {
    setInterfaceForm(interfaceDraft(selectedInterface));
  }, [selectedInterface]);

  useEffect(() => {
    const firstProvider = selectedInterface
      ? providers.find((provider) => provider.interfaceRef === selectedInterface.code && provider.serviceRef === selectedInterface.serviceRef)
      : undefined;
    setProviderForm(providerDraft(firstProvider, resources));
  }, [providers, resources, selectedInterface]);

  useEffect(() => {
    setBindingForm(bindingDraft(
      bindings.find((item) => item.serviceRef === selectedServiceCode && item.interfaceRef === selectedInterfaceCode),
      selectedServiceCode,
      selectedInterfaceCode,
      workspaceId,
      folderId,
    ));
  }, [bindings, folderId, selectedInterfaceCode, selectedServiceCode, workspaceId]);

  useEffect(() => {
    const firstPolicy = rateLimitPolicies.find((policy) => (
      policy.serviceRef === selectedServiceCode &&
      policy.interfaceRef === selectedInterfaceCode &&
      (!providerForm.code || !policy.providerRef || policy.providerRef === providerForm.code)
    ));
    setRateLimitForm(rateLimitDraft(firstPolicy, selectedServiceCode, selectedInterfaceCode, providerForm.code));
  }, [providerForm.code, rateLimitPolicies, selectedInterfaceCode, selectedServiceCode]);

  useEffect(() => {
    const paths = new Set((workflowMigrationAnalysis?.items ?? [])
      .filter((item) => !item.blockingIssues?.length)
      .map((item) => item.path));
    setSelectedWorkflowMigrationPaths(paths);
  }, [workflowMigrationAnalysis]);

  useEffect(() => {
    if (!migrationBreakGlass) return;
    if (!migrationBreakGlassScope) setMigrationBreakGlassScope('db-migration:apply');
    if (!migrationBreakGlassActor && actorId) setMigrationBreakGlassActor(actorId);
    if (!migrationBreakGlassExpiresAt) setMigrationBreakGlassExpiresAt(new Date(Date.now() + 30 * 60 * 1000).toISOString());
  }, [actorId, migrationBreakGlass, migrationBreakGlassActor, migrationBreakGlassExpiresAt, migrationBreakGlassScope]);

  useEffect(() => {
    if (activityId) {
      setWorkflowMigrationActivityId(activityId);
      return;
    }
    if (!workflowMigrationActivityId && workflowMigrationTargets[0]?.id) {
      setWorkflowMigrationActivityId(workflowMigrationTargets[0].id);
    }
  }, [activityId, workflowMigrationActivityId, workflowMigrationTargets]);

  const filteredServices = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return services.filter((service) => !keyword || `${service.code} ${service.name} ${service.owner ?? ''}`.toLowerCase().includes(keyword));
  }, [query, services]);

  const selectService = (service: ServiceDefinitionSummary) => {
    onSelectService(service.code);
    setServiceForm(serviceDraft(service, workspaceId, folderId));
    setInterfaceForm(interfaceDraft(undefined));
    setProviderForm(providerDraft(undefined, resources));
    setTab('interface');
    setError('');
  };

  const selectInterface = (item: ServiceInterfaceSummary) => {
    onSelectInterface(item.code);
    setInterfaceForm(interfaceDraft(item));
    const firstProvider = providers.find((provider) => provider.interfaceRef === item.code && provider.serviceRef === item.serviceRef);
    setProviderForm(providerDraft(firstProvider, resources));
    setTab('provider');
    setError('');
  };

  const startService = () => {
    onSelectService('');
    onSelectInterface('');
    setServiceForm(serviceDraft(undefined, workspaceId, folderId));
    setTab('service');
    setError('');
  };

  const startInterface = () => {
    onSelectInterface('');
    setInterfaceForm(interfaceDraft(undefined));
    setProviderForm(providerDraft(undefined, resources));
    setTab('interface');
    setError('');
  };

  const startProvider = () => {
    setProviderForm(providerDraft(undefined, resources));
    setTab('provider');
    setError('');
  };

  const startSchema = () => {
    setSchemaForm(schemaDraft());
    setTab('schema');
    setError('');
    setSchemaCheck('');
    setSchemaCompatibilityCheck('');
    setSchemaExampleCheck('');
  };

  const selectSchema = (schema: ServiceSchemaSummary) => {
    setSchemaForm(schemaDraft(schema));
    setTab('schema');
    setError('');
    setSchemaCheck('');
    setSchemaCompatibilityCheck('');
    setSchemaExampleCheck('');
  };

  const startBinding = () => {
    setBindingForm(bindingDraft(undefined, selectedServiceCode, selectedInterfaceCode, workspaceId, folderId));
    setTab('binding');
    setError('');
  };

  const selectBinding = (binding: ServiceInterfaceBindingSummary) => {
    setBindingForm(bindingDraft(binding, selectedServiceCode, selectedInterfaceCode, workspaceId, folderId));
    setTab('binding');
    setError('');
  };

  const startRateLimitPolicy = () => {
    setRateLimitForm(rateLimitDraft(undefined, selectedServiceCode, selectedInterfaceCode, providerForm.code));
    setTab('rateLimit');
    setError('');
  };

  const selectRateLimitPolicy = (policy: ServiceRateLimitPolicySummary) => {
    setRateLimitForm(rateLimitDraft(policy, selectedServiceCode, selectedInterfaceCode, providerForm.code));
    setTab('rateLimit');
    setError('');
  };

  const submitService = async () => {
    try {
      setError('');
      await onSaveService({
        code: serviceForm.code.trim(),
        name: serviceForm.name.trim(),
        description: serviceForm.description.trim(),
        tenantId,
        workspaceId: serviceForm.scope === 'WORKSPACE' || serviceForm.scope === 'FOLDER' ? workspaceId : '',
        folderId: serviceForm.scope === 'FOLDER' ? folderId : '',
        serviceType: serviceForm.serviceType,
        scope: serviceForm.scope,
        status: serviceForm.status,
        owner: serviceForm.owner.trim(),
        tags: splitList(serviceForm.tagsText),
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const submitInterface = async () => {
    if (!selectedServiceCode) {
      setError('先选择服务');
      return;
    }
    try {
      setError('');
      await onSaveInterface(selectedServiceCode, {
        code: interfaceForm.code.trim(),
        name: interfaceForm.name.trim(),
        description: interfaceForm.description.trim(),
        tenantId,
        serviceRef: selectedServiceCode,
        operation: interfaceForm.operation,
        inputSchemaRef: interfaceForm.inputSchemaRef.trim(),
        outputSchemaRef: interfaceForm.outputSchemaRef.trim(),
        errorSchemaRef: interfaceForm.errorSchemaRef.trim(),
        idempotencyKeyTemplate: interfaceForm.idempotencyKeyTemplate.trim(),
        timeoutMs: Number(interfaceForm.timeoutMs) || 0,
        retryPolicy: interfaceForm.retryPolicy.trim(),
        luaCallable: interfaceForm.luaCallable,
        status: interfaceForm.status,
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const submitProvider = async () => {
    if (!selectedServiceCode || !selectedInterfaceCode) {
      setError('先选择服务接口');
      return;
    }
    try {
      setError('');
      await onSaveProvider(selectedServiceCode, selectedInterfaceCode, {
        code: providerForm.code.trim(),
        name: providerForm.name.trim(),
        description: providerForm.description.trim(),
        tenantId,
        serviceRef: selectedServiceCode,
        interfaceRef: selectedInterfaceCode,
        protocol: providerForm.protocol,
        endpoint: providerForm.endpoint.trim(),
        topic: providerForm.topic.trim(),
        method: providerForm.method.trim(),
        path: providerForm.path.trim(),
        authRef: providerForm.authRef.trim(),
        resourceRefs: splitList(providerForm.resourceRefsText),
        config: parseObject(providerForm.configText),
        priority: Number(providerForm.priority) || 0,
        environment: providerForm.environment.trim(),
        status: providerForm.status,
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const schemaPayload = (): Partial<ServiceSchemaSummary> & { schemaRef: string } => ({
    tenantId,
    workspaceId: schemaForm.scope === 'WORKSPACE' || schemaForm.scope === 'FOLDER' ? workspaceId : '',
    folderId: schemaForm.scope === 'FOLDER' ? folderId : '',
    schemaRef: schemaForm.schemaRef.trim(),
    schemaType: schemaForm.schemaType,
    scope: schemaForm.scope,
    version: Number(schemaForm.version) || 1,
    status: schemaForm.status,
    description: schemaForm.description.trim(),
    schemaJson: parseJson(schemaForm.schemaJsonText, {}),
    examplesJson: parseJson(schemaForm.examplesJsonText, []),
  });

  const formatSchemaCompatibility = (result: { compatible?: boolean; code?: string; error?: string }) => (
    result.compatible ? '兼容性通过' : `${result.code || 'SERVICE_SCHEMA_INCOMPATIBLE'} ${result.error || ''}`.trim()
  );

  const checkSchemaCompatibility = async () => {
    try {
      setError('');
      const result = await onCheckSchemaCompatibility(schemaPayload());
      const message = formatSchemaCompatibility(result);
      setSchemaCompatibilityCheck(message);
      return result;
    } catch (err) {
      const message = (err as Error).message;
      setSchemaCompatibilityCheck(message);
      throw err;
    }
  };

  const submitSchema = async () => {
    try {
      setError('');
      const compatibility = await checkSchemaCompatibility();
      if (!compatibility.compatible) return;
      await onSaveSchema(schemaPayload());
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const validateSchema = async () => {
    try {
      setError('');
      const result = await onValidateSchema(schemaPayload());
      setSchemaCheck(result.valid || result.compatible ? '校验通过' : `${result.code || 'SCHEMA_INVALID'} ${result.error || ''}`.trim());
    } catch (err) {
      setSchemaCheck((err as Error).message);
    }
  };

  const bindingPayload = (): Partial<ServiceInterfaceBindingSummary> => ({
        tenantId,
        serviceRef: bindingForm.serviceRef.trim() || selectedServiceCode,
        interfaceRef: bindingForm.interfaceRef.trim() || selectedInterfaceCode,
        scope: bindingForm.scope,
        workspaceId: bindingForm.scope === 'WORKSPACE' || bindingForm.scope === 'FOLDER' ? workspaceId : '',
        folderId: bindingForm.scope === 'FOLDER' ? folderId : '',
        principalType: bindingForm.principalType,
        principalId: bindingForm.principalId.trim(),
        allowedNodeTypes: splitList(bindingForm.allowedNodeTypesText),
        allowedOperations: splitList(bindingForm.allowedOperationsText),
        allowedProviderPolicies: splitList(bindingForm.allowedProviderPoliciesText),
        allowExplicitProviderRef: bindingForm.allowExplicitProviderRef,
        allowedProviderRefs: splitList(bindingForm.allowedProviderRefsText),
        includeSubfolders: bindingForm.includeSubfolders,
        minRole: bindingForm.minRole.trim(),
        environment: bindingForm.environment.trim(),
        rateLimitRef: bindingForm.rateLimitRef.trim(),
        deletedAt: bindingForm.deletedAt.trim() || undefined,
        luaCallable: bindingForm.luaCallable,
        requireApproval: bindingForm.requireApproval,
        deny: bindingForm.deny,
        status: bindingForm.status,
  });

  const validateSchemaExample = async () => {
    try {
      setError('');
      const result = await onValidateSchemaExample(schemaPayload(), parseJson(schemaExampleText, {}));
      setSchemaExampleCheck(result.valid ? 'Example 校验通过' : `${result.code || 'EXAMPLE_INVALID'} ${result.error || ''}`.trim());
    } catch (err) {
      setSchemaExampleCheck((err as Error).message);
    }
  };

  const checkBinding = async () => {
    try {
      setError('');
      const result = await onCheckBinding(bindingPayload());
      setBindingCheck(result);
    } catch (err) {
      setBindingCheck({ allowed: false, code: 'BINDING_CHECK_FAILED', message: (err as Error).message });
    }
  };

  const submitBinding = async () => {
    try {
      setError('');
      await onSaveBinding(bindingPayload());
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const rateLimitPayload = (): Partial<ServiceRateLimitPolicySummary> & { code: string } => {
    const metadata = parseObject(rateLimitForm.metadataText);
    const limit = parseIntegerField(rateLimitForm.limit, 'Limit', 0);
    const windowSeconds = parseIntegerField(rateLimitForm.windowSeconds, 'Window Seconds', 1);
    if (rateLimitForm.burst.trim()) {
      metadata.burst = parseIntegerField(rateLimitForm.burst, 'Burst', 0);
    } else {
      delete metadata.burst;
    }
    return {
      tenantId,
      code: rateLimitForm.code.trim(),
      name: rateLimitForm.name.trim(),
      description: rateLimitForm.description.trim(),
      serviceRef: rateLimitForm.serviceRef.trim(),
      interfaceRef: rateLimitForm.interfaceRef.trim(),
      providerRef: rateLimitForm.providerRef.trim(),
      backend: rateLimitForm.backend,
      limit,
      windowSeconds,
      status: rateLimitForm.status,
      metadata,
    };
  };

  const submitRateLimitPolicy = async () => {
    try {
      setError('');
      await onSaveRateLimitPolicy(rateLimitPayload());
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const selectedProviderRuntime = providerForm.code ? runtimeStates[providerForm.code] : undefined;
  const selectedProviderUnavailableReason = selectedProviderRuntime?.circuitStatus === 'OPEN'
    ? 'CIRCUIT_OPEN'
    : selectedProviderRuntime?.healthStatus === 'UNHEALTHY'
      ? 'UNHEALTHY'
      : selectedProviderRuntime?.healthStatus === 'DEGRADED'
        ? 'DEGRADED'
        : '';
  const selectedProviderConfig = useMemo(() => {
    try {
      return parseObject(providerForm.configText);
    } catch {
      return {};
    }
  }, [providerForm.configText]);
  const legacyMigrationDiffs = useMemo(() => resources.filter((resource) => {
    const config = resource.config ?? {};
    return resource.type === 'PROVIDER' || Boolean(config.providerCode) || Boolean(config.endpoint) || Boolean(config.resourceRef);
  }), [resources]);
  const schemaPreview = useMemo(() => {
    try {
      return JSON.stringify(parseJson(schemaForm.schemaJsonText, {}), null, 2);
    } catch (err) {
      return (err as Error).message;
    }
  }, [schemaForm.schemaJsonText]);
  const activeInterfaces = interfaces.filter(isActiveAsset);
  const activeProviders = providers.filter(isActiveAsset);
  const activeSchemas = schemas.filter(isActiveAsset);
  const activeBindings = bindings.filter(isActiveAsset);
  const activeRateLimitPolicies = rateLimitPolicies.filter(isActiveAsset);
  const matchingRateLimitPolicies = rateLimitPolicies.filter((policy) => (
    (!policy.serviceRef || policy.serviceRef === selectedServiceCode) &&
    (!policy.interfaceRef || policy.interfaceRef === selectedInterfaceCode) &&
    (!policy.providerRef || policy.providerRef === providerForm.code)
  ));
  const assetCompleteness = [
    { label: 'Services', value: services.length, risk: services.some((item) => !isActiveAsset(item)) ? '含禁用/删除项' : '' },
    { label: 'Interfaces', value: interfaces.length, risk: activeInterfaces.length === 0 ? '缺少 ACTIVE interface' : '' },
    { label: 'Providers', value: providers.length, risk: activeProviders.length === 0 ? '缺少可用 provider' : '' },
    { label: 'Schemas', value: schemas.length, risk: activeSchemas.length === 0 ? '缺少 schema registry' : '' },
    { label: 'Bindings', value: bindings.length, risk: activeBindings.length === 0 ? '缺少调用授权' : '' },
    { label: 'Rate Limit', value: rateLimitPolicies.length, risk: activeRateLimitPolicies.length === 0 ? '未配置限流策略' : '' },
    { label: 'Audit', value: audits.length, risk: audits.length === 0 ? '暂无调用审计' : '' },
    { label: 'Migrations', value: migrationAnalysis?.pending?.length ?? 0, risk: migrationAnalysis?.drift ? '存在 drift' : migrationAnalysis?.latest ? '' : '有 pending' },
    { label: 'Workflow Migration', value: workflowMigrationAnalysis?.summary.legacySteps ?? 0, risk: workflowMigrationAnalysis?.summary.blockers ? '有 blocker' : workflowMigrationAnalysis?.summary.manualReview ? '需 review' : '' },
  ];
  const selectedInputSchema = schemas.find((schema) => schema.schemaRef === interfaceForm.inputSchemaRef && schema.schemaType === 'INPUT');
  const selectedOutputSchema = schemas.find((schema) => schema.schemaRef === interfaceForm.outputSchemaRef && schema.schemaType === 'OUTPUT');
  const selectedErrorSchema = schemas.find((schema) => schema.schemaRef === interfaceForm.errorSchemaRef && schema.schemaType === 'ERROR');
  const selectedInterfaceRisks = [
    assetRisk(selectedInterface) ? `interface ${assetRisk(selectedInterface)}` : '',
    interfaceForm.inputSchemaRef && !isActiveAsset(selectedInputSchema) ? `input schema ${assetRisk(selectedInputSchema)}` : '',
    interfaceForm.outputSchemaRef && !isActiveAsset(selectedOutputSchema) ? `output schema ${assetRisk(selectedOutputSchema)}` : '',
    interfaceForm.errorSchemaRef && !isActiveAsset(selectedErrorSchema) ? `error schema ${assetRisk(selectedErrorSchema)}` : '',
    !activeBindings.some((binding) => binding.serviceRef === (bindingForm.serviceRef || selectedServiceCode) && binding.interfaceRef === (bindingForm.interfaceRef || selectedInterfaceCode)) ? 'no binding' : '',
  ].filter(Boolean);
  const providerRisks = (provider: ServiceProviderSummary): string[] => {
    const state = runtimeStates[provider.code];
    const config = provider.config ?? {};
    return [
      assetRisk(provider) ? `provider ${assetRisk(provider)}` : '',
      provider.environment && bindingForm.environment && provider.environment !== bindingForm.environment ? 'env mismatch' : '',
      config.mock === true && !bindingForm.allowedProviderPoliciesText.includes('MOCK_ONLY') ? 'mock denied' : '',
      state?.healthStatus === 'UNHEALTHY' ? 'unhealthy' : '',
      state?.healthStatus === 'DEGRADED' ? 'degraded' : '',
      state?.circuitStatus === 'OPEN' ? 'circuit open' : '',
      state?.metadata?.rateLimited === true ? 'rate limited' : '',
      selectedInterfaceRisks.length ? selectedInterfaceRisks.join(', ') : '',
    ].filter(Boolean);
  };
  const selectedProviderRisks = providerForm.code
    ? providerRisks({ ...providerForm, id: '', tenantId, serviceRef: selectedServiceCode, interfaceRef: selectedInterfaceCode, protocol: providerForm.protocol, status: providerForm.status, config: selectedProviderConfig, priority: Number(providerForm.priority) || 0 })
    : [];
  const runMigrationDryRun = async () => {
    try {
      setMigrationActionMessage('');
      const result = await onApplyMigrations({ dryRun: true, confirm: false, confirmation: '', breakGlass: false, reason: '' });
      setMigrationActionMessage(`Dry-run 完成，pending ${result.analysis?.pending?.length ?? 0} 项`);
    } catch (err) {
      setMigrationActionMessage((err as Error).message);
    }
  };
  const runMigrationApply = async () => {
    try {
      setMigrationActionMessage('');
      const result = await onApplyMigrations({
        dryRun: false,
        confirm: true,
        confirmation: migrationApplyConfirm,
        breakGlass: migrationBreakGlass,
        breakGlassScope: migrationBreakGlassScope,
        breakGlassActor: migrationBreakGlassActor,
        breakGlassExpiresAt: migrationBreakGlassExpiresAt,
        breakGlassTicket: migrationBreakGlassTicket,
        reason: migrationApplyReason,
      });
      setMigrationActionMessage(`Apply 完成，pending ${result.after?.pending?.length ?? 0} 项`);
    } catch (err) {
      setMigrationActionMessage((err as Error).message);
    }
  };
  const runWorkflowMigrationAnalyze = async () => {
    try {
      setWorkflowMigrationActionMessage('');
      if (!workflowMigrationActivityId) throw new Error('请选择 workflow draft');
      const analysis = await onAnalyzeWorkflowMigration(workflowMigrationActivityId);
      setWorkflowMigrationActionMessage(`Analyze 完成，legacy ${analysis.summary.legacySteps} 项，可自动迁移 ${analysis.summary.autoMigratable} 项`);
    } catch (err) {
      setWorkflowMigrationActionMessage((err as Error).message);
    }
  };
  const runWorkflowMigrationDryRun = async () => {
    try {
      setWorkflowMigrationActionMessage('');
      if (!workflowMigrationActivityId) throw new Error('请选择 workflow draft');
      const result = await onApplyWorkflowMigration(workflowMigrationActivityId, {
        dryRun: true,
        confirm: false,
        confirmation: '',
        paths: Array.from(selectedWorkflowMigrationPaths),
        reason: '',
      });
      setWorkflowMigrationActionMessage(`Dry-run 完成，计划 apply ${result.appliedItems?.length ?? 0} 项`);
    } catch (err) {
      setWorkflowMigrationActionMessage((err as Error).message);
    }
  };
  const runWorkflowMigrationApply = async () => {
    try {
      setWorkflowMigrationActionMessage('');
      if (!workflowMigrationActivityId) throw new Error('请选择 workflow draft');
      const result = await onApplyWorkflowMigration(workflowMigrationActivityId, {
        dryRun: false,
        confirm: true,
        confirmation: workflowMigrationConfirm,
        paths: Array.from(selectedWorkflowMigrationPaths),
        reason: workflowMigrationReason,
      });
      setWorkflowMigrationActionMessage(result.applied ? `Apply 完成，已迁移 ${result.appliedItems?.length ?? 0} 项` : 'Apply 完成，无可迁移项');
    } catch (err) {
      setWorkflowMigrationActionMessage((err as Error).message);
    }
  };

  return (
    <div className="resource-manager">
      <aside className="resource-list-panel">
        <div className="resource-list-header">
          <div>
            <b>服务能力</b>
            <span>{services.length} services</span>
          </div>
          <TaButton tone="ghost" onClick={onRefresh}><RefreshCw size={15} />刷新</TaButton>
        </div>
        <label className="ta-field">
          <span>搜索服务</span>
          <TaInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="service / owner" />
        </label>
        <div className="resource-list">
          {loading ? <TaEmptyState>服务加载中...</TaEmptyState> : filteredServices.length ? filteredServices.map((service) => (
            <button key={service.code} className={service.code === selectedServiceCode ? 'active' : ''} onClick={() => selectService(service)}>
              <Network size={16} />
              <span><b>{service.name || service.code}</b><small>{service.code}</small></span>
              <TaBadge tone={statusTone(service.status)}>{service.status}</TaBadge>
            </button>
          )) : <TaEmptyState>暂无服务</TaEmptyState>}
        </div>
        <TaButton tone="primary" onClick={startService}><Plus size={15} />新增服务</TaButton>
      </aside>

      <section className="resource-detail-panel">
        <div className="resource-detail-head">
          <div>
            <b>{selectedService?.name || serviceForm.name || '服务能力'}</b>
            <span>{message || 'Service / Interface / Provider'}</span>
          </div>
          {selectedServiceCode && selectedInterfaceCode ? (
            <TaButton tone="ghost" onClick={() => void onTestInterface(selectedServiceCode, selectedInterfaceCode)}><PlayCircle size={15} />测试接口</TaButton>
          ) : null}
        </div>
        <TaTabs>
          <TaTab active={tab === 'overview'} onClick={() => setTab('overview')}><Activity size={15} />总览</TaTab>
          <TaTab active={tab === 'service'} onClick={() => setTab('service')}><Network size={15} />服务</TaTab>
          <TaTab active={tab === 'interface'} onClick={() => setTab('interface')}><Route size={15} />接口</TaTab>
          <TaTab active={tab === 'provider'} onClick={() => setTab('provider')}><ServerCog size={15} />提供者</TaTab>
          <TaTab active={tab === 'schema'} onClick={() => setTab('schema')}><FileJson size={15} />Schema</TaTab>
          <TaTab active={tab === 'binding'} onClick={() => setTab('binding')}><Link2 size={15} />Binding</TaTab>
          <TaTab active={tab === 'rateLimit'} onClick={() => setTab('rateLimit')}><Gauge size={15} />限流</TaTab>
          <TaTab active={tab === 'audit'} onClick={() => setTab('audit')}><History size={15} />审计</TaTab>
          <TaTab active={tab === 'migration'} onClick={() => setTab('migration')}><DatabaseZap size={15} />迁移</TaTab>
        </TaTabs>
        {error ? <div className="resource-error">{error}</div> : null}

        {tab === 'overview' ? (
          <div className="resource-form-grid">
            <div className="resource-audit-list span-2">
              <div><b>能力资产完整性</b><span>{selectedServiceCode || '全局'} · {selectedInterfaceCode || '未选接口'}</span></div>
              {assetCompleteness.map((item) => (
                <div key={item.label}>
                  <b>{item.label}</b>
                  <span>{item.value}{item.risk ? ` · ${item.risk}` : ' · OK'}</span>
                </div>
              ))}
            </div>
            <div className="resource-audit-list span-2">
              <div><b>当前接口发布风险</b><span>{selectedInterfaceRisks.length ? `${selectedInterfaceRisks.length} 项需要处理` : 'Schema / Binding 基础检查通过'}</span></div>
              {selectedInterfaceRisks.length ? selectedInterfaceRisks.map((risk) => <small key={risk}>{risk}</small>) : <small>继续检查 provider runtime、rate-limit 和 migration 状态。</small>}
              <div><b>DB Migration</b><span>{migrationAnalysis ? `${migrationAnalysis.latest ? 'latest' : 'pending'} · drift ${migrationAnalysis.drift ? 'yes' : 'no'}` : '未加载'}</span></div>
              {migrationMessage ? <small>{migrationMessage}</small> : null}
            </div>
            <div className="resource-audit-list span-2">
              <div><b>Rate Limit Policies</b><span>{activeRateLimitPolicies.length} active / {rateLimitPolicies.length} total</span></div>
              {rateLimitPolicies.slice(0, 5).map((policy) => <small key={policy.id || policy.code}>{rateLimitPolicyLabel(policy)}</small>)}
              {!rateLimitPolicies.length ? <small>未配置独立限流策略，运行时会回退 provider config/default。</small> : null}
            </div>
            <div className="resource-form-actions span-2">
              <TaButton tone="outline" onClick={() => setTab('schema')}>查看 Schema</TaButton>
              <TaButton tone="outline" onClick={() => setTab('binding')}>查看 Binding</TaButton>
              <TaButton tone="outline" onClick={() => setTab('rateLimit')}>查看限流</TaButton>
              <TaButton tone="outline" onClick={() => setTab('migration')}>查看 Migration</TaButton>
            </div>
          </div>
        ) : null}

        {tab === 'service' ? (
          <div className="resource-form-grid">
            <FieldInput label="Service Code" value={serviceForm.code} onChange={(value) => setServiceForm((state) => ({ ...state, code: value }))} />
            <FieldInput label="名称" value={serviceForm.name} onChange={(value) => setServiceForm((state) => ({ ...state, name: value }))} />
            <FieldSelect label="类型" value={serviceForm.serviceType} options={SERVICE_TYPES} onChange={(value) => setServiceForm((state) => ({ ...state, serviceType: value }))} />
            <FieldSelect label="Scope" value={serviceForm.scope} options={SERVICE_SCOPES} onChange={(value) => setServiceForm((state) => ({ ...state, scope: value }))} />
            <FieldSelect label="状态" value={serviceForm.status} options={STATUSES} onChange={(value) => setServiceForm((state) => ({ ...state, status: value }))} />
            <FieldInput label="Owner" value={serviceForm.owner} onChange={(value) => setServiceForm((state) => ({ ...state, owner: value }))} />
            <label className="ta-field span-2"><span>描述</span><textarea value={serviceForm.description} onChange={(event) => setServiceForm((state) => ({ ...state, description: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Tags</span><textarea value={serviceForm.tagsText} onChange={(event) => setServiceForm((state) => ({ ...state, tagsText: event.target.value }))} /></label>
            <div className="resource-audit-list span-2">
              <div><b>Legacy Migration Diff</b><span>{legacyMigrationDiffs.length} 个旧 Provider/endpoint resource 需要迁移确认</span></div>
              {legacyMigrationDiffs.slice(0, 5).map((item) => <small key={item.code}>{item.code} · {item.type} · {String(item.config?.providerCode || item.config?.endpoint || item.config?.resourceRef || 'resourceRef')}</small>)}
            </div>
            <div className="resource-form-actions span-2"><TaButton tone="primary" onClick={submitService}>保存服务</TaButton></div>
          </div>
        ) : null}

        {tab === 'interface' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-inline-list">
              {interfaces.map((item) => (
                <button key={item.code} className={item.code === selectedInterfaceCode ? 'active' : ''} onClick={() => selectInterface(item)}>
                  <Route size={15} /><span>{item.name || item.code}</span><TaBadge tone={statusTone(item.status)}>{item.operation}</TaBadge>
                </button>
              ))}
              <TaButton tone="ghost" onClick={startInterface}><Plus size={15} />新增接口</TaButton>
            </div>
            <FieldInput label="Interface Code" value={interfaceForm.code} onChange={(value) => setInterfaceForm((state) => ({ ...state, code: value }))} />
            <FieldInput label="名称" value={interfaceForm.name} onChange={(value) => setInterfaceForm((state) => ({ ...state, name: value }))} />
            <FieldSelect label="Operation" value={interfaceForm.operation} options={INTERFACE_OPERATIONS} onChange={(value) => setInterfaceForm((state) => ({ ...state, operation: value }))} />
            <FieldInput label="Timeout ms" value={interfaceForm.timeoutMs} onChange={(value) => setInterfaceForm((state) => ({ ...state, timeoutMs: value }))} />
            <FieldInput label="Input Schema" value={interfaceForm.inputSchemaRef} onChange={(value) => setInterfaceForm((state) => ({ ...state, inputSchemaRef: value }))} />
            <FieldInput label="Output Schema" value={interfaceForm.outputSchemaRef} onChange={(value) => setInterfaceForm((state) => ({ ...state, outputSchemaRef: value }))} />
            <FieldInput label="Retry Policy" value={interfaceForm.retryPolicy} onChange={(value) => setInterfaceForm((state) => ({ ...state, retryPolicy: value }))} />
            <FieldSelect label="状态" value={interfaceForm.status} options={STATUSES} onChange={(value) => setInterfaceForm((state) => ({ ...state, status: value }))} />
            <label className="ta-field span-2"><span>描述</span><textarea value={interfaceForm.description} onChange={(event) => setInterfaceForm((state) => ({ ...state, description: event.target.value }))} /></label>
            <label className="ta-check"><input type="checkbox" checked={interfaceForm.luaCallable} onChange={(event) => setInterfaceForm((state) => ({ ...state, luaCallable: event.target.checked }))} />Lua 可调用</label>
            <div className="resource-form-actions span-2"><TaButton tone="primary" onClick={submitInterface}>保存接口</TaButton></div>
          </div>
        ) : null}

        {tab === 'provider' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-inline-list">
              {providers.map((provider) => (
                <button key={provider.code} onClick={() => setProviderForm(providerDraft(provider, resources))}>
                  <ServerCog size={15} />
                  <span>
                    {provider.name || provider.code}
                    {providerRisks(provider).length ? <small>{providerRisks(provider).slice(0, 2).join(' / ')}</small> : <small>{provider.environment || 'default'} · selectable</small>}
                  </span>
                  <TaBadge tone={providerRisks(provider).length ? 'error' : statusTone(provider.status)}>{provider.protocol}</TaBadge>
                </button>
              ))}
              <TaButton tone="ghost" onClick={startProvider}><Plus size={15} />新增提供者</TaButton>
            </div>
            <FieldInput label="Provider Code" value={providerForm.code} onChange={(value) => setProviderForm((state) => ({ ...state, code: value }))} />
            <FieldInput label="名称" value={providerForm.name} onChange={(value) => setProviderForm((state) => ({ ...state, name: value }))} />
            <FieldSelect label="Protocol" value={providerForm.protocol} options={PROVIDER_PROTOCOLS} onChange={(value) => setProviderForm((state) => ({ ...state, protocol: value }))} />
            <FieldInput label="Priority" value={providerForm.priority} onChange={(value) => setProviderForm((state) => ({ ...state, priority: value }))} />
            <FieldInput label="Endpoint" value={providerForm.endpoint} onChange={(value) => setProviderForm((state) => ({ ...state, endpoint: value }))} />
            <FieldInput label="Topic" value={providerForm.topic} onChange={(value) => setProviderForm((state) => ({ ...state, topic: value }))} />
            <FieldInput label="Method" value={providerForm.method} onChange={(value) => setProviderForm((state) => ({ ...state, method: value }))} />
            <FieldInput label="Auth Ref" value={providerForm.authRef} onChange={(value) => setProviderForm((state) => ({ ...state, authRef: value }))} />
            <FieldInput label="Environment" value={providerForm.environment} onChange={(value) => setProviderForm((state) => ({ ...state, environment: value }))} />
            <FieldSelect label="状态" value={providerForm.status} options={STATUSES} onChange={(value) => setProviderForm((state) => ({ ...state, status: value }))} />
            <label className="ta-field span-2"><span>Resource Refs</span><textarea value={providerForm.resourceRefsText} onChange={(event) => setProviderForm((state) => ({ ...state, resourceRefsText: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Config JSON</span><textarea value={providerForm.configText} onChange={(event) => setProviderForm((state) => ({ ...state, configText: event.target.value }))} /></label>
            {selectedProviderRuntime ? (
              <div className="resource-audit-list span-2">
                <div><b>Runtime</b><span>{selectedProviderRuntime.healthStatus} · {selectedProviderRuntime.circuitStatus} · failures {selectedProviderRuntime.consecutiveFailures}</span></div>
                {selectedProviderRisks.length ? <div><b>不可选原因</b><span>{selectedProviderRisks.join(' / ')}</span></div> : selectedProviderUnavailableReason ? <div><b>不可选原因</b><span>{selectedProviderUnavailableReason}</span></div> : <div><b>可选状态</b><span>当前 provider 可参与策略选择</span></div>}
                <div><b>环境 / 权重</b><span>{providerForm.environment || 'default'} · weight {(selectedProviderConfig.weight as number | undefined) ?? 'config'} · priority {providerForm.priority}</span></div>
                <div><b>Half-open</b><span>probe {selectedProviderRuntime.halfOpenProbeCount ?? 0}/{selectedProviderRuntime.halfOpenProbeLimit ?? 1} · success {selectedProviderRuntime.halfOpenSuccessCount ?? 0}/{selectedProviderRuntime.halfOpenSuccessThreshold ?? 1}</span></div>
                {selectedProviderRuntime.healthMessage ? <p>{selectedProviderRuntime.healthMessage}</p> : null}
              </div>
            ) : providerForm.code ? (
              <div className="resource-audit-list span-2"><div><b>Runtime</b><span>暂无 runtime state，运行后由后端写入。</span></div></div>
            ) : null}
            <div className="resource-form-actions span-2">
              <TaButton tone="primary" onClick={submitProvider}>保存提供者</TaButton>
              {selectedServiceCode && selectedInterfaceCode && providerForm.code ? (
                <>
                  <TaButton tone="outline" onClick={() => void onCheckProviderHealth(selectedServiceCode, selectedInterfaceCode, providerForm.code.trim())}><PlayCircle size={15} />健康检查</TaButton>
                  <TaButton tone="outline" onClick={() => void onResetProviderCircuit(selectedServiceCode, selectedInterfaceCode, providerForm.code.trim())}><RotateCcw size={15} />重置熔断</TaButton>
                </>
              ) : null}
            </div>
          </div>
        ) : null}

        {tab === 'schema' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-inline-list">
              {schemas.map((schema) => (
                <button key={`${schema.schemaRef}:${schema.version}:${schema.schemaType}`} onClick={() => selectSchema(schema)}>
                  <FileJson size={15} /><span>{schema.schemaRef} v{schema.version}</span><TaBadge tone={statusTone(schema.status)}>{schema.schemaType}</TaBadge>
                </button>
              ))}
              <TaButton tone="ghost" onClick={startSchema}><Plus size={15} />新增 Schema</TaButton>
            </div>
            <FieldInput label="Schema Ref" value={schemaForm.schemaRef} onChange={(value) => setSchemaForm((state) => ({ ...state, schemaRef: value }))} />
            <FieldSelect label="Schema Type" value={schemaForm.schemaType} options={SCHEMA_TYPES} onChange={(value) => setSchemaForm((state) => ({ ...state, schemaType: value }))} />
            <FieldSelect label="Scope" value={schemaForm.scope} options={SERVICE_SCOPES} onChange={(value) => setSchemaForm((state) => ({ ...state, scope: value }))} />
            <FieldInput label="Version" value={schemaForm.version} onChange={(value) => setSchemaForm((state) => ({ ...state, version: value }))} />
            <FieldSelect label="状态" value={schemaForm.status} options={STATUSES} onChange={(value) => setSchemaForm((state) => ({ ...state, status: value }))} />
            <FieldInput label="描述" value={schemaForm.description} onChange={(value) => setSchemaForm((state) => ({ ...state, description: value }))} />
            <label className="ta-field span-2"><span>Schema JSON</span><textarea value={schemaForm.schemaJsonText} onChange={(event) => setSchemaForm((state) => ({ ...state, schemaJsonText: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Examples JSON</span><textarea value={schemaForm.examplesJsonText} onChange={(event) => setSchemaForm((state) => ({ ...state, examplesJsonText: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Example Validate Payload</span><textarea value={schemaExampleText} onChange={(event) => setSchemaExampleText(event.target.value)} /></label>
            <div className="resource-audit-list span-2">
              <div>
                <b>Schema Preview</b>
                <pre>{schemaPreview}</pre>
              </div>
              {schemaCheck ? <div><b>校验结果</b><span>{schemaCheck}</span></div> : null}
              {schemaCompatibilityCheck ? <div><b>兼容性结果</b><span>{schemaCompatibilityCheck}</span></div> : null}
              {schemaExampleCheck ? <div><b>Example 结果</b><span>{schemaExampleCheck}</span></div> : null}
            </div>
            <div className="resource-form-actions span-2">
              <TaButton tone="outline" onClick={() => void validateSchema()}>校验 Schema</TaButton>
              <TaButton tone="outline" onClick={() => void checkSchemaCompatibility()}>兼容性检查</TaButton>
              <TaButton tone="outline" onClick={() => void validateSchemaExample()}>校验 Example</TaButton>
              <TaButton tone="primary" onClick={submitSchema}>保存 Schema</TaButton>
            </div>
          </div>
        ) : null}

        {tab === 'binding' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-inline-list">
              {bindings.map((binding) => (
                <button key={binding.id || `${binding.serviceRef}:${binding.interfaceRef}:${binding.principalType}:${binding.principalId}`} onClick={() => selectBinding(binding)}>
                  <Link2 size={15} /><span>{binding.serviceRef}/{binding.interfaceRef}</span><TaBadge tone={binding.deny ? 'error' : statusTone(binding.status)}>{binding.principalType}:{binding.principalId}</TaBadge>
                </button>
              ))}
              <TaButton tone="ghost" onClick={startBinding}><Plus size={15} />新增 Binding</TaButton>
            </div>
            <FieldInput label="Service Ref" value={bindingForm.serviceRef} onChange={(value) => setBindingForm((state) => ({ ...state, serviceRef: value }))} />
            <FieldInput label="Interface Ref" value={bindingForm.interfaceRef} onChange={(value) => setBindingForm((state) => ({ ...state, interfaceRef: value }))} />
            <FieldSelect label="Scope" value={bindingForm.scope} options={SERVICE_SCOPES} onChange={(value) => setBindingForm((state) => ({ ...state, scope: value }))} />
            <FieldSelect label="Principal Type" value={bindingForm.principalType} options={PRINCIPAL_TYPES} onChange={(value) => setBindingForm((state) => ({ ...state, principalType: value }))} />
            <FieldInput label="Principal ID" value={bindingForm.principalId} onChange={(value) => setBindingForm((state) => ({ ...state, principalId: value }))} />
            <FieldSelect label="状态" value={bindingForm.status} options={STATUSES} onChange={(value) => setBindingForm((state) => ({ ...state, status: value }))} />
            <FieldInput label="Min Role" value={bindingForm.minRole} onChange={(value) => setBindingForm((state) => ({ ...state, minRole: value }))} />
            <FieldInput label="Environment" value={bindingForm.environment} onChange={(value) => setBindingForm((state) => ({ ...state, environment: value }))} />
            <FieldInput label="Rate Limit Ref" value={bindingForm.rateLimitRef} onChange={(value) => setBindingForm((state) => ({ ...state, rateLimitRef: value }))} />
            <FieldInput label="Deleted At" value={bindingForm.deletedAt} onChange={(value) => setBindingForm((state) => ({ ...state, deletedAt: value }))} />
            <div className="resource-audit-list span-2">
              <div><b>Binding 风险提示</b><span>{bindingForm.deny || bindingForm.deletedAt || bindingForm.status !== 'ACTIVE' ? '当前 binding 不会放行调用' : '当前 binding 可参与匹配'}</span></div>
              <small>minRole 只对 ROLE principal 按 VIEWER &lt; EDITOR &lt; ADMIN &lt; OWNER 比较；自定义角色保持精确匹配。</small>
              <small>environment 非空时必须和 providerChain/runtime environment 一致，否则发布或运行时会被拒绝。</small>
              <small>rateLimitRef 非空时运行时按该 ref 精确解析限流策略；策略缺失、禁用或 scope 不覆盖会阻断调用。</small>
              <small>includeSubfolders 会按 workspace folder parent 链覆盖子 Folder；folder 记录缺失时不扩大授权。</small>
              {matchingRateLimitPolicies.length ? <small>匹配限流策略：{matchingRateLimitPolicies.map((policy) => policy.code).join(', ')}</small> : <small>当前 service/interface/provider 未匹配到独立限流策略。</small>}
              {bindingForm.deletedAt ? <small>deletedAt 非空会被视为软删除，不参与 binding checker。</small> : null}
            </div>
            <label className="ta-field span-2"><span>Allowed Node Types</span><textarea value={bindingForm.allowedNodeTypesText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedNodeTypesText: event.target.value }))} /></label>
            <label className="ta-field"><span>Allowed Operations</span><textarea value={bindingForm.allowedOperationsText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedOperationsText: event.target.value }))} /></label>
            <label className="ta-field"><span>Provider Policies</span><textarea value={bindingForm.allowedProviderPoliciesText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedProviderPoliciesText: event.target.value }))} placeholder={PROVIDER_POLICIES.join('\n')} /></label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.allowExplicitProviderRef} onChange={(event) => setBindingForm((state) => ({ ...state, allowExplicitProviderRef: event.target.checked }))} />允许显式 providerRef</label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.includeSubfolders} onChange={(event) => setBindingForm((state) => ({ ...state, includeSubfolders: event.target.checked }))} />包含子 Folder</label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.luaCallable} onChange={(event) => setBindingForm((state) => ({ ...state, luaCallable: event.target.checked }))} />Lua 可调用</label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.requireApproval} onChange={(event) => setBindingForm((state) => ({ ...state, requireApproval: event.target.checked }))} />需要审批</label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.deny} onChange={(event) => setBindingForm((state) => ({ ...state, deny: event.target.checked }))} />Deny</label>
            <label className="ta-field span-2"><span>Allowed Provider Refs</span><textarea value={bindingForm.allowedProviderRefsText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedProviderRefsText: event.target.value }))} /></label>
            {bindingCheck ? (
              <div className="resource-audit-list span-2">
                <div><b>Binding Check</b><span>{bindingCheck.allowed ? '允许调用' : `${bindingCheck.code || 'DENIED'} ${bindingCheck.message || ''}`}</span></div>
                {bindingCheck.matchedBindingIds?.length ? <small>matched {bindingCheck.matchedBindingIds.join(', ')} · request {bindingCheck.requestId || '-'}</small> : null}
                {bindingForm.luaCallable ? <small>Lua service_call 会走同一套 binding / schema / audit 链路。</small> : <small>当前 binding 未开放 Lua service_call。</small>}
              </div>
            ) : null}
            <div className="resource-form-actions span-2">
              <TaButton tone="outline" onClick={() => void checkBinding()}>预检 Binding</TaButton>
              <TaButton tone="primary" onClick={submitBinding}>保存 Binding</TaButton>
            </div>
          </div>
        ) : null}

        {tab === 'rateLimit' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-inline-list">
              {rateLimitPolicies.map((policy) => (
                <button key={policy.id || policy.code} onClick={() => selectRateLimitPolicy(policy)}>
                  <Gauge size={15} />
                  <span>
                    {policy.name || policy.code}
                    <small>{rateLimitPolicyLabel(policy)}</small>
                  </span>
                  <TaBadge tone={statusTone(policy.status)}>{policy.status}</TaBadge>
                </button>
              ))}
              <TaButton tone="ghost" onClick={startRateLimitPolicy}><Plus size={15} />新增限流策略</TaButton>
            </div>
            <FieldInput label="Policy Code" value={rateLimitForm.code} onChange={(value) => setRateLimitForm((state) => ({ ...state, code: value }))} />
            <FieldInput label="名称" value={rateLimitForm.name} onChange={(value) => setRateLimitForm((state) => ({ ...state, name: value }))} />
            <FieldInput label="Service Ref" value={rateLimitForm.serviceRef} onChange={(value) => setRateLimitForm((state) => ({ ...state, serviceRef: value }))} />
            <FieldInput label="Interface Ref" value={rateLimitForm.interfaceRef} onChange={(value) => setRateLimitForm((state) => ({ ...state, interfaceRef: value }))} />
            <FieldInput label="Provider Ref" value={rateLimitForm.providerRef} onChange={(value) => setRateLimitForm((state) => ({ ...state, providerRef: value }))} />
            <FieldSelect label="Backend" value={rateLimitForm.backend} options={RATE_LIMIT_BACKENDS} onChange={(value) => setRateLimitForm((state) => ({ ...state, backend: value }))} />
            <FieldInput label="Limit" value={rateLimitForm.limit} onChange={(value) => setRateLimitForm((state) => ({ ...state, limit: value }))} />
            <FieldInput label="Window Seconds" value={rateLimitForm.windowSeconds} onChange={(value) => setRateLimitForm((state) => ({ ...state, windowSeconds: value }))} />
            <FieldInput label="Burst" value={rateLimitForm.burst} onChange={(value) => setRateLimitForm((state) => ({ ...state, burst: value }))} />
            <FieldSelect label="状态" value={rateLimitForm.status} options={STATUSES} onChange={(value) => setRateLimitForm((state) => ({ ...state, status: value }))} />
            <label className="ta-field span-2"><span>描述</span><textarea value={rateLimitForm.description} onChange={(event) => setRateLimitForm((state) => ({ ...state, description: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Metadata JSON</span><textarea value={rateLimitForm.metadataText} onChange={(event) => setRateLimitForm((state) => ({ ...state, metadataText: event.target.value }))} /></label>
            <div className="resource-audit-list span-2">
              <div><b>Tenant</b><span>{tenantId}</span></div>
              <div><b>Scope 预览</b><span>{rateLimitForm.providerRef ? 'PROVIDER' : rateLimitForm.interfaceRef ? 'INTERFACE' : rateLimitForm.serviceRef ? 'SERVICE' : 'TENANT'} · {rateLimitForm.serviceRef || '*'} / {rateLimitForm.interfaceRef || '*'} / {rateLimitForm.providerRef || '*'}</span></div>
              <small>Provider Ref 非空时必须同时填写 Service Ref 和 Interface Ref；Interface Ref 非空时必须填写 Service Ref。</small>
              <small>Binding 的 rateLimitRef 会精确匹配 policy code；为空时运行时按 service/interface/provider 最佳匹配。</small>
              <small>metadata 可配置 runtimeScope/rateLimitScope=GLOBAL 让 Redis backend 使用全局维度，否则默认按 provider subject 隔离。</small>
            </div>
            <div className="resource-form-actions span-2">
              <TaButton tone="primary" onClick={submitRateLimitPolicy}>保存限流策略</TaButton>
            </div>
          </div>
        ) : null}

        {tab === 'audit' ? (
          <div className="resource-form-grid">
            <div className="span-2 resource-audit-list">
              {audits.length ? audits.map((audit) => (
                <div key={audit.id}>
                  <b>{audit.serviceRef}/{audit.interfaceRef}{audit.providerRef ? `/${audit.providerRef}` : ''}</b>
                  <span>{audit.status}{audit.errorCode ? ` · ${audit.errorCode}` : ''} · {audit.latencyMs ?? 0}ms · {audit.createdAt ? new Date(audit.createdAt).toLocaleString() : ''}</span>
                  {audit.errorMessage ? <p>{audit.errorMessage}</p> : null}
                  <small>{audit.nodeId || audit.nodeType || 'workflow'} · req {audit.requestHash || '-'} · res {audit.responseHash || '-'}</small>
                  {audit.providerTrace ? <pre>{JSON.stringify(audit.providerTrace, null, 2)}</pre> : null}
                </div>
              )) : <TaEmptyState>暂无 service call audit。</TaEmptyState>}
              <div>
                <b>分页 / 权限</b>
                <span>request {auditRequestId || '-'} · cursor {auditNextCursor || '-'}</span>
                <small>审计详情按 tenant/workspace/folder 只读权限过滤，payload 只展示 hash 和 trace。</small>
              </div>
              {auditNextCursor ? <TaButton tone="outline" onClick={() => void onLoadMoreAudits()}>加载更多</TaButton> : null}
            </div>
          </div>
        ) : null}

        {tab === 'migration' ? (
          <div className="resource-form-grid">
            <div className="resource-audit-list span-2">
              <div>
                <b>DB Migration Status</b>
                <span>{migrationAnalysis ? `${migrationAnalysis.latest ? 'LATEST' : 'PENDING'} · safe ${migrationAnalysis.safeToApply ? 'yes' : 'no'}` : '未加载'}</span>
                <TaBadge tone={migrationTone(migrationAnalysis)}>{migrationAnalysis?.drift ? 'DRIFT' : migrationAnalysis?.latest ? 'LATEST' : 'PENDING'}</TaBadge>
              </div>
              {migrationMessage ? <small>{migrationMessage}</small> : null}
              <small>pending {migrationAnalysis?.pending?.length ?? 0} · applied {migrationAnalysis?.applied?.length ?? 0} · required missing {migrationAnalysis?.missingRequiredTables?.length ?? 0}</small>
              {migrationAnalysis?.errors?.map((item) => <small key={item}>error: {item}</small>)}
              {migrationAnalysis?.legacyUntrackedTables?.length ? <small>legacy untracked: {migrationAnalysis.legacyUntrackedTables.join(', ')}</small> : null}
            </div>
            <div className="resource-audit-list span-2">
              <div><b>Pending Migrations</b><span>{migrationAnalysis?.pending?.length ?? 0} 项</span></div>
              {migrationAnalysis?.pending?.slice(0, 8).map((item) => <small key={item.fileName}>{item.version} · {item.fileName} · {item.checksum.slice(0, 12)}</small>)}
              {!migrationAnalysis?.pending?.length ? <small>暂无 pending migration。</small> : null}
            </div>
            <div className="resource-audit-list span-2">
              <div><b>Required Tables</b><span>{migrationAnalysis?.requiredTables?.filter((item) => item.missing).length ?? 0} missing</span></div>
              {migrationAnalysis?.requiredTables?.slice(0, 12).map((item) => <small key={item.name}>{item.exists ? 'OK' : 'MISSING'} · {item.name}</small>)}
            </div>
            <div className="resource-audit-list span-2">
              <div>
                <b>Workflow Legacy Service-Interface Migration</b>
                <span>{workflowMigrationActivityId ? `activity ${workflowMigrationActivityId}` : '请选择 workflow draft'}</span>
                <TaBadge tone={workflowMigrationTone(workflowMigrationAnalysis)}>{workflowMigrationAnalysis?.status || 'NOT_ANALYZED'}</TaBadge>
              </div>
              <label className="ta-field">
                <span>Workflow Draft</span>
                <TaSelect value={workflowMigrationActivityId} onChange={(event) => setWorkflowMigrationActivityId(event.target.value)}>
                  <option value="">选择 Workflow</option>
                  {workflowMigrationTargets.map((target) => <option key={target.id} value={target.id}>{target.label}</option>)}
                </TaSelect>
              </label>
              {workflowMigrationMessage ? <small>{workflowMigrationMessage}</small> : null}
              <small>legacy {workflowMigrationAnalysis?.summary.legacySteps ?? 0} · auto {workflowMigrationAnalysis?.summary.autoMigratable ?? 0} · review {workflowMigrationAnalysis?.summary.manualReview ?? 0} · blockers {workflowMigrationAnalysis?.summary.blockers ?? 0}</small>
              {workflowMigrationAnalysis?.blockers?.map((blocker) => <small key={`${blocker.path}:${blocker.reason}`}>blocker: {blocker.nodeId} · {blocker.path} · {blocker.reason}</small>)}
              {!workflowMigrationActivityId ? <small>该迁移只作用于 workflow draft；请选择一个 workflow 后再 analyze/apply。</small> : null}
            </div>
            <div className="resource-audit-list span-2">
              <div><b>Workflow Migration Diff Items</b><span>{workflowMigrationAnalysis?.items?.length ?? 0} 项</span></div>
              {workflowMigrationAnalysis?.items?.length ? workflowMigrationAnalysis.items.map((item) => {
                const selected = selectedWorkflowMigrationPaths.has(item.path);
                const blocked = Boolean(item.blockingIssues?.length);
                return (
                  <div key={item.path}>
                    <label className="ta-check">
                      <input
                        type="checkbox"
                        checked={selected}
                        disabled={blocked}
                        onChange={(event) => setSelectedWorkflowMigrationPaths((paths) => {
                          const next = new Set(paths);
                          if (event.target.checked) next.add(item.path);
                          else next.delete(item.path);
                          return next;
                        })}
                      />
                      {item.nodeId} · {item.path}
                    </label>
                    <span>{item.legacyType} → {item.recommendedTarget || 'MANUAL'} · confidence {item.confidence} · risk {item.risk}</span>
                    <small>{[item.serviceRef, item.interfaceRef, item.providerRef, item.resourceRef, item.operation].filter(Boolean).join(' / ') || 'manual mapping required'}</small>
                    {item.legacyFields?.length ? <small>legacy fields: {item.legacyFields.join(', ')}</small> : null}
                    {item.blockingIssues?.map((issue) => <small key={issue}>blocker: {issue}</small>)}
                    <pre>{JSON.stringify({ before: item.before, after: item.after }, null, 2)}</pre>
                  </div>
                );
              }) : <small>尚未分析，或当前 draft 无 legacy provider/resource 字段。</small>}
            </div>
            <FieldInput label="Workflow 迁移确认短语" value={workflowMigrationConfirm} onChange={setWorkflowMigrationConfirm} placeholder="APPLY_WORKFLOW_SERVICE_INTERFACE_MIGRATION" />
            <FieldInput label="Workflow 迁移原因" value={workflowMigrationReason} onChange={setWorkflowMigrationReason} placeholder="migrate legacy provider fields" />
            <div className="resource-audit-list">
              <div><b>Workflow Apply Guard</b><span>已选择 {selectedWorkflowMigrationPaths.size} 个 path</span></div>
              <small>真实 apply 需要确认短语和 reason；blocker 项不会自动迁移。</small>
              {workflowMigrationActionMessage ? <small>{workflowMigrationActionMessage}</small> : null}
            </div>
            <FieldInput label="确认短语" value={migrationApplyConfirm} onChange={setMigrationApplyConfirm} placeholder="APPLY_DB_MIGRATIONS" />
            <FieldInput label="变更原因" value={migrationApplyReason} onChange={setMigrationApplyReason} placeholder="ops reviewed migration apply" />
            <label className="ta-check"><input type="checkbox" checked={migrationBreakGlass} onChange={(event) => setMigrationBreakGlass(event.target.checked)} />Break-glass 确认</label>
            <FieldInput label="Break-glass Scope" value={migrationBreakGlassScope} onChange={setMigrationBreakGlassScope} placeholder="db-migration:apply" />
            <FieldInput label="Break-glass Actor" value={migrationBreakGlassActor} onChange={setMigrationBreakGlassActor} placeholder="与 X-Actor-Id 一致" />
            <FieldInput label="Break-glass Ticket" value={migrationBreakGlassTicket} onChange={setMigrationBreakGlassTicket} placeholder="INC-123 / approval ticket" />
            <FieldInput label="Break-glass Expires At" value={migrationBreakGlassExpiresAt} onChange={setMigrationBreakGlassExpiresAt} placeholder="2026-06-26T12:30:00Z" />
            <div className="resource-audit-list">
              <div><b>Apply Guard</b><span>真实 apply 必须确认短语、break-glass 和 reason。</span></div>
              <small>默认先 dry-run；真实 apply 需要 scope=db-migration:apply、actor 与请求头一致、ticket 非空、expiresAt 未过期且不超过 2 小时。</small>
              <small>当前 actor 可从请求头 X-Actor-Id 判定；expiresAt 建议填写 ISO 时间，例如当前时间后 30 分钟。</small>
              {migrationActionMessage ? <small>{migrationActionMessage}</small> : null}
            </div>
            <div className="resource-form-actions span-2">
              <TaButton tone="outline" onClick={() => void onRefreshMigrationStatus()}>刷新状态</TaButton>
              <TaButton tone="outline" onClick={() => void onAnalyzeMigrations()}>Analyze</TaButton>
              <TaButton tone="outline" onClick={() => void runMigrationDryRun()}>Dry-run Apply</TaButton>
              <TaButton tone="primary" onClick={() => void runMigrationApply()}><ShieldAlert size={15} />确认 Apply</TaButton>
              <TaButton tone="outline" disabled={!workflowMigrationActivityId} onClick={() => void runWorkflowMigrationAnalyze()}><GitCompare size={15} />Workflow Analyze</TaButton>
              <TaButton tone="outline" disabled={!workflowMigrationActivityId} onClick={() => void runWorkflowMigrationDryRun()}>Workflow Dry-run</TaButton>
              <TaButton tone="primary" disabled={!workflowMigrationActivityId} onClick={() => void runWorkflowMigrationApply()}><ShieldAlert size={15} />Workflow Apply</TaButton>
            </div>
          </div>
        ) : null}
      </section>
    </div>
  );
}
