import { useEffect, useMemo, useState } from 'react';
import { FileJson, History, Link2, Network, PlayCircle, Plus, RefreshCw, RotateCcw, Route, ServerCog } from 'lucide-react';
import type { ResourceSummary, ServiceCallAuditLogSummary, ServiceDefinitionSummary, ServiceInterfaceBindingCheckResult, ServiceInterfaceBindingSummary, ServiceInterfaceSummary, ServiceProviderRuntimeStateSummary, ServiceProviderSummary, ServiceSchemaSummary } from '../../types';
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
  luaCallable: boolean;
  requireApproval: boolean;
  deny: boolean;
  status: string;
};

type Props = {
  tenantId: string;
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
};

const SERVICE_TYPES = ['EXTERNAL', 'INTERNAL', 'SYSTEM'];
const SERVICE_SCOPES = ['TENANT', 'WORKSPACE', 'FOLDER', 'ACTIVITY', 'NODE'];
const STATUSES = ['ACTIVE', 'DRAFT', 'DISABLED'];
const INTERFACE_OPERATIONS = ['CALL', 'PUBLISH', 'REQUEST_REPLY', 'SUBSCRIBE_TRIGGER'];
const PROVIDER_PROTOCOLS = ['RESOURCE', 'HTTP', 'RPC', 'MQ', 'PLUGIN', 'TCP', 'LUA_ADAPTER'];
const SCHEMA_TYPES = ['INPUT', 'OUTPUT', 'ERROR'];
const PRINCIPAL_TYPES = ['ROLE', 'USER', 'GROUP', 'SERVICE_ACCOUNT', 'WORKSPACE', 'FOLDER', 'ACTIVITY', 'NODE'];
const PROVIDER_POLICIES = ['PRIMARY', 'PRIMARY_WITH_FALLBACK', 'WEIGHTED', 'WEIGHTED_WITH_FALLBACK', 'ENVIRONMENT_MATCH', 'MOCK_ONLY'];

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
    luaCallable: binding?.luaCallable ?? false,
    requireApproval: binding?.requireApproval ?? false,
    deny: binding?.deny ?? false,
    status: binding?.status ?? 'ACTIVE',
  };
}

function statusTone(status: string): 'success' | 'warning' | 'error' | 'gray' {
  if (status === 'ACTIVE') return 'success';
  if (status === 'DRAFT') return 'warning';
  if (status === 'DISABLED') return 'error';
  return 'gray';
}

export function ServiceCapabilityManagement({
  tenantId,
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
}: Props) {
  const [tab, setTab] = useState<'service' | 'interface' | 'provider' | 'schema' | 'binding' | 'audit'>('service');
  const [query, setQuery] = useState('');
  const [serviceForm, setServiceForm] = useState<ServiceDraft>(() => serviceDraft(selectedService, workspaceId, folderId));
  const [interfaceForm, setInterfaceForm] = useState<InterfaceDraft>(() => interfaceDraft(selectedInterface));
  const [providerForm, setProviderForm] = useState<ProviderDraft>(() => providerDraft(providers[0], resources));
  const [schemaForm, setSchemaForm] = useState<SchemaDraft>(() => schemaDraft());
  const [bindingForm, setBindingForm] = useState<BindingDraft>(() => bindingDraft(undefined, selectedServiceCode, selectedInterfaceCode, workspaceId, folderId));
  const [error, setError] = useState('');
  const [schemaCheck, setSchemaCheck] = useState('');
  const [schemaCompatibilityCheck, setSchemaCompatibilityCheck] = useState('');
  const [schemaExampleText, setSchemaExampleText] = useState('{}');
  const [schemaExampleCheck, setSchemaExampleCheck] = useState('');
  const [bindingCheck, setBindingCheck] = useState<ServiceInterfaceBindingCheckResult | undefined>();

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
          <TaTab active={tab === 'service'} onClick={() => setTab('service')}><Network size={15} />服务</TaTab>
          <TaTab active={tab === 'interface'} onClick={() => setTab('interface')}><Route size={15} />接口</TaTab>
          <TaTab active={tab === 'provider'} onClick={() => setTab('provider')}><ServerCog size={15} />提供者</TaTab>
          <TaTab active={tab === 'schema'} onClick={() => setTab('schema')}><FileJson size={15} />Schema</TaTab>
          <TaTab active={tab === 'binding'} onClick={() => setTab('binding')}><Link2 size={15} />Binding</TaTab>
          <TaTab active={tab === 'audit'} onClick={() => setTab('audit')}><History size={15} />审计</TaTab>
        </TaTabs>
        {error ? <div className="resource-error">{error}</div> : null}

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
                  <ServerCog size={15} /><span>{provider.name || provider.code}</span><TaBadge tone={statusTone(provider.status)}>{provider.protocol}</TaBadge>
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
                {selectedProviderUnavailableReason ? <div><b>不可选原因</b><span>{selectedProviderUnavailableReason}</span></div> : <div><b>可选状态</b><span>当前 provider 可参与策略选择</span></div>}
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
            <label className="ta-field span-2"><span>Allowed Node Types</span><textarea value={bindingForm.allowedNodeTypesText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedNodeTypesText: event.target.value }))} /></label>
            <label className="ta-field"><span>Allowed Operations</span><textarea value={bindingForm.allowedOperationsText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedOperationsText: event.target.value }))} /></label>
            <label className="ta-field"><span>Provider Policies</span><textarea value={bindingForm.allowedProviderPoliciesText} onChange={(event) => setBindingForm((state) => ({ ...state, allowedProviderPoliciesText: event.target.value }))} placeholder={PROVIDER_POLICIES.join('\n')} /></label>
            <label className="ta-check"><input type="checkbox" checked={bindingForm.allowExplicitProviderRef} onChange={(event) => setBindingForm((state) => ({ ...state, allowExplicitProviderRef: event.target.checked }))} />允许显式 providerRef</label>
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
      </section>
    </div>
  );
}
