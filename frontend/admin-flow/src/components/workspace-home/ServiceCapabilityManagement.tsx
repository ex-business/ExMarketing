import { useEffect, useMemo, useState } from 'react';
import { Network, PlayCircle, Plus, RefreshCw, Route, ServerCog } from 'lucide-react';
import type { ResourceSummary, ServiceDefinitionSummary, ServiceInterfaceSummary, ServiceProviderSummary } from '../../types';
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

type Props = {
  tenantId: string;
  workspaceId?: string;
  folderId?: string;
  resources: ResourceSummary[];
  services: ServiceDefinitionSummary[];
  interfaces: ServiceInterfaceSummary[];
  providers: ServiceProviderSummary[];
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
  onTestInterface: (serviceCode: string, interfaceCode: string) => Promise<void>;
};

const SERVICE_TYPES = ['EXTERNAL', 'INTERNAL', 'SYSTEM'];
const SERVICE_SCOPES = ['TENANT', 'WORKSPACE', 'FOLDER'];
const STATUSES = ['ACTIVE', 'DRAFT', 'DISABLED'];
const INTERFACE_OPERATIONS = ['CALL', 'PUBLISH', 'REQUEST_REPLY', 'SUBSCRIBE_TRIGGER'];
const PROVIDER_PROTOCOLS = ['RESOURCE', 'HTTP', 'RPC', 'MQ', 'PLUGIN', 'TCP', 'LUA_ADAPTER'];

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
  onTestInterface,
}: Props) {
  const [tab, setTab] = useState<'service' | 'interface' | 'provider'>('service');
  const [query, setQuery] = useState('');
  const [serviceForm, setServiceForm] = useState<ServiceDraft>(() => serviceDraft(selectedService, workspaceId, folderId));
  const [interfaceForm, setInterfaceForm] = useState<InterfaceDraft>(() => interfaceDraft(selectedInterface));
  const [providerForm, setProviderForm] = useState<ProviderDraft>(() => providerDraft(providers[0], resources));
  const [error, setError] = useState('');

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
            <label className="ta-field span-2"><span>Resource Refs</span><textarea value={providerForm.resourceRefsText} onChange={(event) => setProviderForm((state) => ({ ...state, resourceRefsText: event.target.value }))} /></label>
            <label className="ta-field span-2"><span>Config JSON</span><textarea value={providerForm.configText} onChange={(event) => setProviderForm((state) => ({ ...state, configText: event.target.value }))} /></label>
            <div className="resource-form-actions span-2"><TaButton tone="primary" onClick={submitProvider}>保存提供者</TaButton></div>
          </div>
        ) : null}
      </section>
    </div>
  );
}
