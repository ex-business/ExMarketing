import { useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { ReactNode } from 'react';
import type { ActivityEdge, ActivityNode, FlowNode, GroupNode, NoteNode, ResourceSummary, WorkflowEdgeRoute, WorkflowEdgeRouteType, WorkflowNodeConfigField, WorkflowNodeConfigSchema, WorkflowNodeConfigValidationRule, WorkflowValidationIssue } from '../types';
import { cx } from '../utils/flow';
import { NodeKindIcon } from './NodeKindIcon';

const ROUTE_TYPE_GROUPS: Array<{ title: string; description: string; items: WorkflowEdgeRouteType[] }> = [
  { title: '默认路径', description: '没有命中其它分支时执行。', items: ['DEFAULT', 'ELSE'] },
  { title: '条件分支', description: '需要 expression 或 conditionRef。', items: ['IF', 'ELIF'] },
  { title: '业务结果', description: '按 handler 输出的决策路由。', items: ['PASS', 'RISK'] },
  { title: '终止/阻断', description: '阻断、跳过或兜底终止路径。', items: ['BLOCK', 'SKIP'] },
  { title: '输出变量', description: '按 output key 连接指定输出。', items: ['OUTPUT'] },
];

const DEFAULT_ROUTE_EXPRESSIONS: Partial<Record<WorkflowEdgeRouteType, string>> = {
  IF: 'condition == true',
  ELIF: 'condition == true',
  PASS: 'decision == "PASS"',
  BLOCK: 'decision == "BLOCK"',
  RISK: 'decision == "RISK"',
  SKIP: 'decision == "SKIP"',
};

const DEFAULT_ROUTE_TYPES = new Set<WorkflowEdgeRouteType>(['DEFAULT', 'ELSE', 'RISK', 'SKIP']);
const PROPERTY_PANEL_TAB_STORAGE_KEY = 'exmarketing.studio.propertyPanelTab';
type PropertyPanelTab = 'settings' | 'run';

const ROUTE_EXPRESSION_HINTS = [
  { label: '决策等于 PASS', expression: 'decision == "PASS"' },
  { label: '决策等于 BLOCK', expression: 'decision == "BLOCK"' },
  { label: '状态成功', expression: 'status == "SUCCESS"' },
  { label: '风险命中', expression: 'risk == true' },
  { label: '金额达标', expression: '$.fact.amount >= 100' },
];

function loadPropertyPanelTab(): PropertyPanelTab {
  return window.localStorage.getItem(PROPERTY_PANEL_TAB_STORAGE_KEY) === 'run' ? 'run' : 'settings';
}

function PropertyList({ title, items }: { title: string; items: string[] }) {
  return (
    <PropertyDisclosure title={title} summary={`${items.length} 项`}>
      <div className="property-tags">
        {items.length > 0 ? items.map((item) => <span key={item}>{item}</span>) : <em>未配置</em>}
      </div>
    </PropertyDisclosure>
  );
}

function PropertyDisclosure({
  title,
  titleMeta,
  summary,
  children,
  defaultOpen = false,
  compact = false,
}: {
  title: string;
  titleMeta?: string;
  summary?: string;
  children: ReactNode;
  defaultOpen?: boolean;
  compact?: boolean;
}) {
  return (
    <details className={cx('property-section property-disclosure', compact && 'compact')} open={defaultOpen}>
      <summary>
        <span>
          {title}
          {titleMeta ? <small>{titleMeta}</small> : null}
        </span>
        {summary ? <em>{summary}</em> : null}
      </summary>
      <div className="property-disclosure-body">
        {children}
      </div>
    </details>
  );
}

type InputMappingValue = {
  sourceNodeId: string;
  outputKey: string;
  path?: string;
};

type FieldBindingValue = {
  mode?: 'EXPRESSION' | 'SCRIPT';
  language?: 'LUA';
  source?: string;
  onError?: 'FAIL' | 'USE_DEFAULT' | 'USE_FALLBACK' | 'NULL';
  fallback?: unknown;
  valueType?: 'ANY' | 'STRING' | 'NUMBER' | 'BOOLEAN' | 'OBJECT' | 'ARRAY';
};

type UpstreamOutputOption = {
  value: string;
  nodeId: string;
  nodeTitle: string;
  nodeCode: string;
  output: string;
  label: string;
  type?: string;
  description?: string;
};

type UpstreamOutputGroup = {
  nodeId: string;
  nodeTitle: string;
  options: UpstreamOutputOption[];
};

function normalizedVarType(value?: string): string {
  return (value || 'unknown').trim().toLowerCase();
}

function variableOptionMatchesSearch(option: UpstreamOutputOption, keyword: string): boolean {
  const query = keyword.trim().toLowerCase();
  if (!query) return true;
  return [
    option.output,
    option.nodeTitle,
    option.nodeCode,
    option.type,
    option.description,
  ].some((item) => (item ?? '').toLowerCase().includes(query));
}

function filterVariableOptions(options: UpstreamOutputOption[], search: string, typeFilter: string): UpstreamOutputOption[] {
  return options.filter((option) => {
    if (!variableOptionMatchesSearch(option, search)) return false;
    if (typeFilter === 'ALL') return true;
    return normalizedVarType(option.type) === typeFilter;
  });
}

function outputTypeFilters(options: UpstreamOutputOption[]): string[] {
  return Array.from(new Set(options.map((option) => normalizedVarType(option.type)))).sort();
}

function jsonPathPresets(inputKey: string, option?: UpstreamOutputOption): string[] {
  const presets = new Set<string>(['$']);
  const key = inputKey.toLowerCase();
  const output = option?.output.toLowerCase() ?? '';
  const type = normalizedVarType(option?.type);
  if (type === 'object' || type === 'any' || type === 'unknown') {
    presets.add('$.id');
    presets.add('$.status');
  }
  if (key.includes('event') || output.includes('event')) {
    presets.add('$.eventId');
    presets.add('$.eventType');
    presets.add('$.subject');
    presets.add('$.attributes');
  }
  if (key.includes('subject') || output.includes('subject')) {
    presets.add('$.id');
    presets.add('$.type');
  }
  if (key.includes('reward') || output.includes('reward')) {
    presets.add('$[0]');
    presets.add('$[0].amount');
    presets.add('$[0].rewardCode');
  }
  if (key.includes('fact') || output.includes('fact')) {
    presets.add('$.USER_PROFILE');
    presets.add('$.VIP_LEVEL');
    presets.add('$.riskScore');
  }
  return Array.from(presets).slice(0, 8);
}

function inputMappingFor(node: ActivityNode): Record<string, InputMappingValue> {
  const value = node.data.config?.inputMapping;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const out: Record<string, InputMappingValue> = {};
  Object.entries(value as Record<string, unknown>).forEach(([inputName, mapping]) => {
    if (!mapping || typeof mapping !== 'object' || Array.isArray(mapping)) return;
    const item = mapping as Record<string, unknown>;
    const sourceNodeId = typeof item.sourceNodeId === 'string' ? item.sourceNodeId : typeof item.nodeId === 'string' ? item.nodeId : '';
    const outputKey = typeof item.outputKey === 'string'
      ? item.outputKey
      : typeof item.sourceOutputKey === 'string'
        ? item.sourceOutputKey
        : typeof item.output === 'string'
          ? item.output
          : '';
    const path = typeof item.path === 'string' ? item.path : undefined;
    if (!sourceNodeId || !outputKey) return;
    out[inputName] = { sourceNodeId, outputKey, path };
  });
  return out;
}

function upstreamNodeIds(node: ActivityNode, edges: ActivityEdge[]): string[] {
  const seen = new Set<string>();
  const queue = edges.filter((edge) => edge.target === node.id).map((edge) => edge.source);
  while (queue.length > 0) {
    const source = queue.shift();
    if (!source || seen.has(source)) continue;
    seen.add(source);
    edges.filter((edge) => edge.target === source).forEach((edge) => queue.push(edge.source));
  }
  return Array.from(seen);
}

function upstreamOutputOptions(node: ActivityNode, nodes: FlowNode[], edges: ActivityEdge[], schemas: WorkflowNodeConfigSchema[]): UpstreamOutputOption[] {
  const schemaByType = new Map(schemas.map((schema) => [schema.nodeType, schema]));
  const upstreamIds = upstreamNodeIds(node, edges);
  return upstreamIds
    .flatMap((edge) => {
      const sourceNode = nodes.find((item): item is ActivityNode => item.id === edge && item.type === 'activity');
      if (!sourceNode) return [];
      const schemaOutputs = schemaByType.get(sourceNode.data.id)?.outputs ?? [];
      const outputs = schemaOutputs.length > 0
        ? schemaOutputs
        : sourceNode.data.outputs.map((output) => ({ key: output, label: output, type: undefined, description: undefined }));
      return outputs.map((output) => ({
        value: `${sourceNode.id}::${output.key}`,
        nodeId: sourceNode.id,
        nodeTitle: sourceNode.data.title,
        nodeCode: sourceNode.data.code,
        output: output.key,
        label: `${sourceNode.data.title} / ${output.label || output.key}`,
        type: output.type,
        description: output.description,
      }));
    });
}

function upstreamOutputGroups(options: UpstreamOutputOption[]): UpstreamOutputGroup[] {
  const groups: UpstreamOutputGroup[] = [];
  options.forEach((option) => {
    let group = groups.find((item) => item.nodeId === option.nodeId);
    if (!group) {
      group = { nodeId: option.nodeId, nodeTitle: option.nodeTitle, options: [] };
      groups.push(group);
    }
    group.options.push(option);
  });
  return groups;
}

function InputMappingEditor({
  node,
  schema,
  nodes,
  edges,
  schemas,
  onNodeConfigChange,
  onLocateNode,
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  nodes: FlowNode[];
  edges: ActivityEdge[];
  schemas: WorkflowNodeConfigSchema[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
  onLocateNode?: (nodeId: string) => void;
}) {
  const [variableSearch, setVariableSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('ALL');
  const options = upstreamOutputOptions(node, nodes, edges, schemas);
  const filteredOptions = filterVariableOptions(options, variableSearch, typeFilter);
  const optionGroups = upstreamOutputGroups(filteredOptions);
  const typeFilters = outputTypeFilters(options);
  const mapping = inputMappingFor(node);
  const hasMappedInputs = Object.keys(mapping).length > 0;
  const inputs = schema?.inputs?.length
    ? schema.inputs
    : node.data.inputs.map((input): NonNullable<WorkflowNodeConfigSchema['inputs']>[number] => ({ key: input, label: input, type: undefined, required: false, description: undefined }));
  const autoMappingCandidates = inputs.filter((input) => !mapping[input.key] && options.some((option) => option.output === input.key || option.output.toLowerCase() === input.key.toLowerCase()));
  const applyAutoMapping = () => {
    const next = { ...mapping };
    inputs.forEach((input) => {
      if (next[input.key]) return;
      const selected = options.find((option) => option.output === input.key)
        ?? options.find((option) => option.output.toLowerCase() === input.key.toLowerCase());
      if (selected) {
        next[input.key] = { sourceNodeId: selected.nodeId, outputKey: selected.output };
      }
    });
    onNodeConfigChange(node.id, 'inputMapping', next);
  };
  const repairInvalidMappings = () => {
    const next: Record<string, InputMappingValue> = {};
    inputs.forEach((input) => {
      const current = mapping[input.key];
      const currentOption = current ? options.find((option) => option.nodeId === current.sourceNodeId && option.output === current.outputKey) : undefined;
      const replacement = options.find((option) => option.output === input.key)
        ?? options.find((option) => option.output.toLowerCase() === input.key.toLowerCase());
      if (current && currentOption) {
        next[input.key] = {
          ...current,
          path: current.path && current.path.trim().startsWith('$') ? current.path.trim() : undefined,
        };
        return;
      }
      if (replacement) {
        next[input.key] = { sourceNodeId: replacement.nodeId, outputKey: replacement.output };
      }
    });
    onNodeConfigChange(node.id, 'inputMapping', next);
  };
  const clearInvalidMappings = () => {
    const next: Record<string, InputMappingValue> = {};
    Object.entries(mapping).forEach(([inputName, ref]) => {
      const valid = options.some((option) => option.nodeId === ref.sourceNodeId && option.output === ref.outputKey);
      const validPath = !ref.path || ref.path.trim().startsWith('$');
      if (valid && validPath) next[inputName] = ref;
    });
    onNodeConfigChange(node.id, 'inputMapping', next);
  };
  const updateMapping = (inputName: string, selectedValue: string) => {
    const next = { ...mapping };
    if (!selectedValue) {
      delete next[inputName];
    } else {
      const selected = options.find((option) => option.value === selectedValue);
      if (selected) {
        next[inputName] = { sourceNodeId: selected.nodeId, outputKey: selected.output };
      }
    }
    onNodeConfigChange(node.id, 'inputMapping', next);
  };
  const updatePath = (inputName: string, path: string) => {
    const selected = mapping[inputName];
    if (!selected) return;
    onNodeConfigChange(node.id, 'inputMapping', {
      ...mapping,
      [inputName]: { ...selected, path: path.trim() || undefined },
    });
  };
  const selectedOptionFor = (inputName: string) => {
    const selected = mapping[inputName];
    if (!selected) return undefined;
    return options.find((option) => option.nodeId === selected.sourceNodeId && option.output === selected.outputKey);
  };
  const sourceNodeFor = (sourceNodeId: string) => nodes.find((item): item is ActivityNode => item.id === sourceNodeId && item.type === 'activity');
  const invalidMappingCount = inputs.filter((input) => {
    const selected = mapping[input.key];
    if (!selected) return false;
    const selectedOption = selectedOptionFor(input.key);
    return !selectedOption || Boolean(selected.path && !selected.path.trim().startsWith('$'));
  }).length;
  return (
    <PropertyDisclosure title="输入映射" summary={`${inputs.length} 个输入`}>
      {inputs.length === 0 ? (
        <p className="property-empty">该节点没有输入变量。</p>
      ) : options.length === 0 && !hasMappedInputs ? (
        <p className="property-empty">先连接上游节点，再从上游输出中选择输入来源。</p>
      ) : (
        <div className="input-map-grid">
          <div className="input-map-actions">
            <span>{autoMappingCandidates.length ? `可自动匹配 ${autoMappingCandidates.length} 个输入` : '没有可自动匹配的未映射输入'}</span>
            <button type="button" disabled={autoMappingCandidates.length === 0} onClick={applyAutoMapping}>自动映射</button>
            <button type="button" disabled={invalidMappingCount === 0 && autoMappingCandidates.length === 0} onClick={repairInvalidMappings}>批量修复</button>
            <button type="button" disabled={invalidMappingCount === 0} onClick={clearInvalidMappings}>清空失效</button>
          </div>
          <div className="variable-filter-bar">
            <label>
              <span>搜索变量</span>
              <input value={variableSearch} placeholder="节点名 / code / output key" onChange={(event) => setVariableSearch(event.target.value)} />
            </label>
            <label>
              <span>类型</span>
              <select value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}>
                <option value="ALL">全部类型</option>
                {typeFilters.map((type) => <option key={type} value={type}>{type}</option>)}
              </select>
            </label>
            <em>{filteredOptions.length} / {options.length}</em>
          </div>
          {inputs.map((input) => {
            const selected = mapping[input.key];
            const selectedValue = selected ? `${selected.sourceNodeId}::${selected.outputKey}` : '';
            const selectedOption = selectedOptionFor(input.key);
            const invalidReference = Boolean(selected && !selectedOption);
            const missingRequired = Boolean(input.required && !selected);
            const invalidPath = Boolean(selected?.path && !selected.path.trim().startsWith('$'));
            const sourceNode = selected ? sourceNodeFor(selected.sourceNodeId) : undefined;
            return (
              <div key={input.key} className={cx('input-map-row', (invalidReference || missingRequired || invalidPath) && 'invalid')}>
                <div className="input-map-name">
                  <span>
                    <b>{input.label || input.key}</b>
                    {input.required ? <em>必填</em> : null}
                  </span>
                  {selectedOption ? (
                    <strong>{selectedOption.nodeTitle} / {selectedOption.output}</strong>
                  ) : selected ? (
                    <strong>引用已失效</strong>
                  ) : missingRequired ? (
                    <strong>必填未映射</strong>
                  ) : (
                    <strong>未选择</strong>
                  )}
                </div>
                {input.description ? <small>{input.description}</small> : null}
                {missingRequired ? (
                  <p className="input-map-invalid">
                    必填输入 {input.key} 还没有绑定上游输出。
                  </p>
                ) : null}
                {invalidReference ? (
                  <p className="input-map-invalid">
                    上游输出 {selected.sourceNodeId} / {selected.outputKey} 不可用，请重新选择。
                  </p>
                ) : null}
                {selected ? (
                  <div className={cx('input-map-reference', (invalidReference || invalidPath) && 'invalid')}>
                    <div>
                      <span>来源节点</span>
                      <b>{selectedOption?.nodeTitle ?? sourceNode?.data.title ?? selected.sourceNodeId}</b>
                    </div>
                    <div>
                      <span>Output Key</span>
                      <b>{selected.outputKey}</b>
                    </div>
                    <div>
                      <span>类型</span>
                      <b>{selectedOption?.type ?? 'unknown'}</b>
                    </div>
                    <div>
                      <span>JSONPath</span>
                      <b>{selected.path?.trim() || '$'}</b>
                    </div>
                    {onLocateNode && sourceNode ? (
                      <button type="button" onClick={() => onLocateNode(sourceNode.id)}>定位来源</button>
                    ) : null}
                  </div>
                ) : null}
                {optionGroups.length > 0 ? (
                  <div className="variable-picker">
                    {optionGroups.map((group) => (
                      <div key={`${input.key}-${group.nodeId}`} className="variable-group">
                        <b>{group.nodeTitle}</b>
                        <div>
                          {group.options.map((option) => (
                            <button
                              key={`${input.key}-${option.value}`}
                              type="button"
                              className={cx(selectedValue === option.value && 'active')}
                              onClick={() => updateMapping(input.key, option.value)}
                            >
                              <span>{option.output}</span>
                              <em>{option.nodeTitle} · {option.nodeCode}</em>
                              {option.type ? <i>{option.type}</i> : null}
                              {option.description ? <small>{option.description}</small> : null}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="property-empty">没有匹配当前搜索或类型过滤的上游输出。</p>
                )}
                {selected ? (
                  <div className="input-map-ref-tools">
                    <label>
                      <span>Path</span>
                      <input
                        className={cx(invalidPath && 'invalid')}
                        value={selected.path ?? ''}
                        placeholder="$ 或 $.user.id"
                        onChange={(event) => updatePath(input.key, event.target.value)}
                      />
                    </label>
                    <button type="button" className="input-map-clear" onClick={() => updateMapping(input.key, '')}>
                      清除引用
                    </button>
                  </div>
                ) : null}
                {selected ? (
                  <div className="jsonpath-preset-row">
                    <span>Path 快捷选择</span>
                    <div>
                      {jsonPathPresets(input.key, selectedOption).map((preset) => (
                        <button
                          key={`${input.key}-${selectedValue}-${preset}`}
                          type="button"
                          className={cx((selected.path?.trim() || '$') === preset && 'active')}
                          onClick={() => updatePath(input.key, preset)}
                        >
                          {preset}
                        </button>
                      ))}
                    </div>
                  </div>
                ) : null}
                {invalidPath ? (
                  <p className="input-map-invalid">
                    Path 必须以 $ 开头，例如 $ 或 $.user.id。
                  </p>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </PropertyDisclosure>
  );
}

function configValue(node: ActivityNode, field: WorkflowNodeConfigField): unknown {
  if (node.data.config && Object.prototype.hasOwnProperty.call(node.data.config, field.key)) {
    return node.data.config[field.key];
  }
  return field.default ?? '';
}

function fieldBindingsFor(node: ActivityNode): Record<string, FieldBindingValue> {
  const value = node.data.config?.fieldBindings;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const out: Record<string, FieldBindingValue> = {};
  Object.entries(value as Record<string, unknown>).forEach(([key, item]) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return;
    out[key] = item as FieldBindingValue;
  });
  return out;
}

function updateFieldBinding(
  node: ActivityNode,
  fieldKey: string,
  binding: FieldBindingValue | undefined,
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void,
) {
  const next = { ...fieldBindingsFor(node) };
  if (!binding) delete next[fieldKey];
  else next[fieldKey] = binding;
  onNodeConfigChange(node.id, 'fieldBindings', Object.keys(next).length ? next : undefined);
}

function stringifyConfigValue(value: unknown): string {
  if (value === undefined || value === null) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return JSON.stringify(value, null, 2);
}

function parseArrayInput(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function toggleArrayOption(current: unknown, option: string): string[] {
  const values = Array.isArray(current) ? current.map((item) => String(item)) : parseArrayInput(stringifyConfigValue(current));
  return values.includes(option) ? values.filter((item) => item !== option) : [...values, option];
}

function parseObjectInput(value: string): unknown {
  const trimmed = value.trim();
  if (!trimmed) return {};
  try {
    const parsed = JSON.parse(trimmed);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : value;
  } catch {
    return value;
  }
}

function ruleLabel(rule: WorkflowNodeConfigValidationRule): string {
  const value = stringifyConfigValue(rule.value);
  switch (rule.type) {
    case 'MIN_LENGTH': return `最少 ${value} 字符`;
    case 'MAX_LENGTH': return `最多 ${value} 字符`;
    case 'PATTERN': return rule.message || `匹配 ${value}`;
    case 'MIN_VALUE': return `最小 ${value}`;
    case 'MAX_VALUE': return `最大 ${value}`;
    case 'MIN_ITEMS': return `至少 ${value} 项`;
    case 'MAX_ITEMS': return `最多 ${value} 项`;
    default: return rule.message || rule.type;
  }
}

function ConfigFieldInput({
  node,
  field,
  resourceOptions = [],
  onNodeConfigChange,
}: {
  node: ActivityNode;
  field: WorkflowNodeConfigField;
  resourceOptions?: ResourceSummary[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const value = configValue(node, field);
  if (field.key === 'resourceRef') {
    const current = typeof value === 'string' ? value : '';
    const exists = !current || resourceOptions.some((resource) => resource.code === current);
    return (
      <select className={exists ? '' : 'invalid'} value={current} onChange={(event) => onNodeConfigChange(node.id, field.key, event.target.value)}>
        <option value="">选择资源</option>
        {!exists ? <option value={current}>{current}（未加载或无权限）</option> : null}
        {resourceOptions.map((resource) => (
          <option key={resource.id || resource.code} value={resource.code}>
            {resource.name || resource.code} · {resource.code} · {resource.type}
          </option>
        ))}
      </select>
    );
  }
  if (field.type === 'BOOLEAN') {
    return (
      <label className="schema-switch-row">
        <input
          type="checkbox"
          checked={Boolean(value)}
          onChange={(event) => onNodeConfigChange(node.id, field.key, event.target.checked)}
        />
        <span>{Boolean(value) ? '已启用' : '未启用'}</span>
      </label>
    );
  }
  if (field.type === 'ENUM') {
    return (
      <select value={typeof value === 'string' ? value : ''} onChange={(event) => onNodeConfigChange(node.id, field.key, event.target.value)}>
        <option value="">选择</option>
        {(field.options ?? []).map((option) => <option key={option} value={option}>{option}</option>)}
      </select>
    );
  }
  if (field.type === 'NUMBER') {
    return (
      <input
        type="number"
        value={typeof value === 'number' ? value : stringifyConfigValue(value)}
        onChange={(event) => onNodeConfigChange(node.id, field.key, event.target.value === '' ? '' : Number(event.target.value))}
      />
    );
  }
  if (field.type === 'ARRAY') {
    if (field.options?.length) {
      const selected = Array.isArray(value) ? value.map((item) => String(item)) : parseArrayInput(stringifyConfigValue(value));
      return (
        <details className="schema-multi-select">
          <summary>
            <span>{selected.length ? selected.join(' / ') : '选择'}</span>
            <em>{selected.length} 项</em>
          </summary>
          <div>
            {field.options.map((option) => (
              <label key={option}>
                <input
                  type="checkbox"
                  checked={selected.includes(option)}
                  onChange={() => onNodeConfigChange(node.id, field.key, toggleArrayOption(selected, option))}
                />
                <span>{option}</span>
              </label>
            ))}
          </div>
        </details>
      );
    }
    return (
      <textarea
        value={Array.isArray(value) ? value.join('\n') : stringifyConfigValue(value)}
        placeholder="每行一项，或用逗号分隔"
        onChange={(event) => onNodeConfigChange(node.id, field.key, parseArrayInput(event.target.value))}
      />
    );
  }
  if (field.type === 'OBJECT') {
    return (
      <textarea
        value={stringifyConfigValue(value)}
        placeholder='{"key":"value"}'
        onChange={(event) => onNodeConfigChange(node.id, field.key, parseObjectInput(event.target.value))}
      />
    );
  }
  return (
    <input
      value={stringifyConfigValue(value)}
      onChange={(event) => onNodeConfigChange(node.id, field.key, event.target.value)}
    />
  );
}

function ConfigFieldRow({
  node,
  field,
  issues,
  resourceOptions,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  field: WorkflowNodeConfigField;
  issues?: WorkflowValidationIssue[];
  resourceOptions: ResourceSummary[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const hasBlockIssue = issues?.some((issue) => issue.severity === 'BLOCK');
  const hasWarningIssue = issues?.some((issue) => issue.severity === 'WARNING');
  const binding = fieldBindingsFor(node)[field.key];
  const bindingEnabled = Boolean(binding);
  return (
    <label className={cx('schema-config-row', hasBlockIssue && 'has-error', !hasBlockIssue && hasWarningIssue && 'has-warning')} data-config-field={field.key}>
      <span>
        <b>{field.label}</b>
        {field.required ? <em>必填</em> : null}
        <button
          type="button"
          className={cx('field-fx-button', bindingEnabled && 'active')}
          title="字段表达式"
          onClick={(event) => {
            event.preventDefault();
            event.stopPropagation();
            updateFieldBinding(
              node,
              field.key,
              bindingEnabled ? undefined : { mode: 'EXPRESSION', language: 'LUA', source: '', onError: 'FAIL', valueType: 'ANY' },
              onNodeConfigChange,
            );
          }}
        >
          fx
        </button>
      </span>
      <ConfigFieldInput node={node} field={field} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />
      {bindingEnabled ? (
        <div className="field-binding-editor" onClick={(event) => event.stopPropagation()}>
          <div className="field-binding-toolbar">
            <select
              value={binding?.mode ?? 'EXPRESSION'}
              onChange={(event) => updateFieldBinding(node, field.key, { ...binding, mode: event.target.value as FieldBindingValue['mode'] }, onNodeConfigChange)}
            >
              <option value="EXPRESSION">表达式</option>
              <option value="SCRIPT">Lua 脚本</option>
            </select>
            <select
              value={binding?.onError ?? 'FAIL'}
              onChange={(event) => updateFieldBinding(node, field.key, { ...binding, onError: event.target.value as FieldBindingValue['onError'] }, onNodeConfigChange)}
            >
              <option value="FAIL">失败即阻断</option>
              <option value="USE_DEFAULT">使用固定值</option>
              <option value="USE_FALLBACK">使用 fallback</option>
              <option value="NULL">置空</option>
            </select>
            <select
              value={binding?.valueType ?? 'ANY'}
              onChange={(event) => updateFieldBinding(node, field.key, { ...binding, valueType: event.target.value as FieldBindingValue['valueType'] }, onNodeConfigChange)}
            >
              <option value="ANY">ANY</option>
              <option value="STRING">STRING</option>
              <option value="NUMBER">NUMBER</option>
              <option value="BOOLEAN">BOOLEAN</option>
              <option value="OBJECT">OBJECT</option>
              <option value="ARRAY">ARRAY</option>
            </select>
          </div>
          <textarea
            value={binding?.source ?? ''}
            placeholder={binding?.mode === 'SCRIPT'
              ? 'local vip = Resource.userProfile.get(Context.get("event.subject.id"))\nif vip.level >= 5 then return 5 end\nreturn 1'
              : 'Resource.activityConfig.get("daily_checkin_max_count")'}
            onChange={(event) => updateFieldBinding(node, field.key, { ...binding, mode: binding?.mode ?? 'EXPRESSION', language: 'LUA', source: event.target.value }, onNodeConfigChange)}
          />
          <input
            value={stringifyConfigValue(binding?.fallback ?? '')}
            placeholder="fallback，可选"
            onChange={(event) => updateFieldBinding(node, field.key, { ...binding, fallback: parseObjectInput(event.target.value) }, onNodeConfigChange)}
          />
          <p>表达式不需要 return；Lua 脚本需要 return。可用 Context、Resource、ResourceRef、Node、Template、Time。</p>
        </div>
      ) : null}
      {issues?.length ? (
        <div className="schema-field-issues">
          {issues.map((issue) => (
            <i key={`${issue.code}-${issue.path ?? issue.message}`}>{issue.message}</i>
          ))}
        </div>
      ) : null}
      {field.description ? <small>{field.description}</small> : null}
      {field.validationRules?.length ? (
        <div className="schema-rule-list">
          {field.validationRules.map((rule) => <i key={`${field.key}-${rule.type}-${stringifyConfigValue(rule.value)}`}>{ruleLabel(rule)}</i>)}
        </div>
      ) : null}
    </label>
  );
}

function NodeConfigPreview({ schema }: { schema: WorkflowNodeConfigSchema }) {
  const reads = schema.preview?.reads?.length
    ? schema.preview.reads
    : schema.inputs.map((input) => `${input.key}${input.required ? ' *' : ''}`);
  const writes = schema.preview?.writes?.length
    ? schema.preview.writes
    : schema.outputs.map((output) => output.key);
  const notes = schema.preview?.notes ?? [];
  return (
    <PropertyDisclosure title="配置预览" summary={`${reads.length} 读 / ${writes.length} 写`}>
      <div className="node-config-preview">
        <div>
          <span>读取</span>
          <div>{reads.length ? reads.map((item) => <b key={item}>{item}</b>) : <em>无需输入</em>}</div>
        </div>
        <div>
          <span>输出</span>
          <div>{writes.length ? writes.map((item) => <b key={item}>{item}</b>) : <em>无输出</em>}</div>
        </div>
        {notes.length ? (
          <p>{notes.join(' ')}</p>
        ) : null}
      </div>
    </PropertyDisclosure>
  );
}

function NodeConfigForm({
  node,
  schema,
  validationIssues,
  resourceOptions,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  validationIssues?: WorkflowValidationIssue[];
  resourceOptions: ResourceSummary[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  if (!schema) return <p className="property-empty">未找到该节点类型的配置 schema。</p>;
  const groups = schema.fieldGroups?.length
    ? schema.fieldGroups
    : [{ key: 'base', title: schema.title, description: schema.description, fields: schema.fields }];
  const issuesByField = nodeConfigIssuesByField(validationIssues ?? []);
  const nodeLevelIssues = (validationIssues ?? []).filter((issue) => !nodeConfigIssueFieldKey(issue));
  const focusConfigField = (fieldKey: string) => {
    const row = Array.from(document.querySelectorAll<HTMLElement>('.property-panel [data-config-field]'))
      .find((item) => item.dataset.configField === fieldKey);
    row?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    row?.querySelector<HTMLElement>('input, select, textarea, button')?.focus();
  };
  return (
    <div className="schema-config-grid">
      {validationIssues?.length ? (
        <div className="schema-validation-panel">
          <div>
            <b>配置检查</b>
            <span>{validationIssues.length} 项</span>
          </div>
          {validationIssues.map((issue) => {
            const fieldKey = nodeConfigIssueFieldKey(issue);
            return (
              <button
                key={`${issue.code}-${issue.path ?? issue.message}`}
                type="button"
                className={issue.severity.toLowerCase()}
                onClick={() => fieldKey && focusConfigField(fieldKey)}
                disabled={!fieldKey}
              >
                <i>{issue.severity === 'BLOCK' ? '阻塞' : '警告'}</i>
                <span>{issue.message}</span>
                {fieldKey ? <em>{fieldKey}</em> : null}
              </button>
            );
          })}
          {nodeLevelIssues.length ? <p>节点级问题请结合画布连线、输入映射或配置预览处理。</p> : null}
        </div>
      ) : null}
      {groups.map((group) => (
        <div key={group.key} className="schema-config-group">
          <div className="schema-config-group-head">
            <b>{group.title}</b>
            <span>{group.fields.length} 项</span>
          </div>
          {group.description ? <p>{group.description}</p> : null}
          {group.fields.map((field) => (
            <ConfigFieldRow key={field.key} node={node} field={field} issues={issuesByField.get(field.key)} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />
          ))}
        </div>
      ))}
    </div>
  );
}

function requiredConfigCount(schema?: WorkflowNodeConfigSchema): number {
  if (!schema) return 0;
  return schema.fields.filter((field) => field.required).length;
}

function nodeInfoMeta(node: ActivityNode, schema?: WorkflowNodeConfigSchema): Array<{ label: string; value: string }> {
  const inputCount = schema?.inputs.length ?? node.data.inputs.length;
  const outputCount = schema?.outputs.length ?? node.data.outputs.length;
  return [
    { label: 'ID', value: node.id },
    { label: '类型', value: `${node.data.code} / ${node.data.kind}` },
    { label: '坐标', value: `x ${Math.round(node.position.x)} / y ${Math.round(node.position.y)}` },
    { label: 'I/O', value: `${inputCount} 入 / ${outputCount} 出` },
    { label: '配置', value: schema ? `v${schema.version} · ${requiredConfigCount(schema)} 必填` : '无 schema' },
  ];
}

function NodeInfoSummary({ node, schema }: { node: ActivityNode; schema?: WorkflowNodeConfigSchema }) {
  const meta = nodeInfoMeta(node, schema);
  return (
    <div className="property-node-summary">
      <div className="property-node-summary-main">
        <NodeKindIcon node={node.data} className="property-node-summary-icon" />
        <div>
          <h2>{node.data.title}</h2>
          <p>{node.data.desc || schema?.description || '暂无说明'}</p>
        </div>
      </div>
      <div className="property-node-summary-meta">
        {meta.map((item) => (
          <span key={item.label}>
            <em>{item.label}</em>
            <b>{item.value}</b>
          </span>
        ))}
      </div>
    </div>
  );
}

function nodeConfigIssueFieldKey(issue: WorkflowValidationIssue): string {
  const path = issue.path ?? '';
  const marker = '.config.';
  const index = path.indexOf(marker);
  if (index < 0) return '';
  return path.slice(index + marker.length).split(/[.[\]]/)[0] ?? '';
}

function nodeConfigIssuesByField(issues: WorkflowValidationIssue[]): Map<string, WorkflowValidationIssue[]> {
  const out = new Map<string, WorkflowValidationIssue[]>();
  issues.forEach((issue) => {
    const fieldKey = nodeConfigIssueFieldKey(issue);
    if (!fieldKey) return;
    out.set(fieldKey, [...(out.get(fieldKey) ?? []), issue]);
  });
  return out;
}

function safeJSONString(value: unknown): string {
  if (value === undefined) return 'undefined';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function syntaxHighlightJSON(value: unknown): ReactNode {
  const text = safeJSONString(value);
  const parts: ReactNode[] = [];
  const pattern = /("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*"(?=\s*:))|("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*")|\b(true|false)\b|\b(null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    const token = match[0];
    const className = match[1] ? 'json-key' : match[2] ? 'json-string' : match[3] ? 'json-boolean' : match[4] ? 'json-null' : 'json-number';
    parts.push(<span key={`${match.index}-${token}`} className={className}>{token}</span>);
    lastIndex = match.index + token.length;
  }
  if (lastIndex < text.length) parts.push(text.slice(lastIndex));
  return parts;
}

function parseJSONConfig(value: string): unknown {
  const trimmed = value.trim();
  if (!trimmed) return {};
  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function JsonBlock({ title, value, empty = '暂无数据' }: { title: string; value: unknown; empty?: string }) {
  const hasValue = value !== undefined && value !== null && value !== '';
  return (
    <div className="node-run-json-block">
      <span>{title}</span>
      {hasValue ? <pre className="json-syntax">{syntaxHighlightJSON(value)}</pre> : <em>{empty}</em>}
    </div>
  );
}

const RUN_KEY_FIELD_NAMES = new Set([
  'activityCode',
  'activityId',
  'amount',
  'billNo',
  'decision',
  'duplicate',
  'eventId',
  'eventType',
  'failedStage',
  'grantChannel',
  'grantStatus',
  'id',
  'idempotencyKey',
  'messageType',
  'reason',
  'reasonCode',
  'rewardAmount',
  'rewardType',
  'sendStatus',
  'status',
  'subject',
  'targetChannel',
  'targetRef',
  'tenantId',
]);

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function compactRunValue(value: unknown): string {
  if (value === null) return 'null';
  if (value === undefined) return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  if (Array.isArray(value)) return `${value.length} items`;
  if (isPlainRecord(value)) {
    const id = value.id ?? value.eventId ?? value.bizId ?? value.type;
    return id === undefined ? '{...}' : String(id);
  }
  return String(value);
}

function collectRunKeyData(value: unknown, prefix = '', depth = 0, out: Array<{ key: string; value: string }> = []): Array<{ key: string; value: string }> {
  if (out.length >= 18 || depth > 4 || value === null || value === undefined) return out;
  if (Array.isArray(value)) {
    value.slice(0, 3).forEach((item, index) => collectRunKeyData(item, `${prefix}[${index}]`, depth + 1, out));
    return out;
  }
  if (!isPlainRecord(value)) return out;
  Object.entries(value).forEach(([key, item]) => {
    if (out.length >= 18) return;
    const path = prefix ? `${prefix}.${key}` : key;
    if (RUN_KEY_FIELD_NAMES.has(key) && !isPlainRecord(item) && !Array.isArray(item)) {
      out.push({ key: path, value: compactRunValue(item) });
      return;
    }
    if (RUN_KEY_FIELD_NAMES.has(key) && (isPlainRecord(item) || Array.isArray(item))) {
      out.push({ key: path, value: compactRunValue(item) });
    }
    collectRunKeyData(item, path, depth + 1, out);
  });
  return out;
}

function mergeRunKeyData(...sources: unknown[]): Array<{ key: string; value: string }> {
  const seen = new Set<string>();
  const rows: Array<{ key: string; value: string }> = [];
  sources.forEach((source) => {
    collectRunKeyData(source).forEach((row) => {
      const dedupeKey = `${row.key}:${row.value}`;
      if (!row.value || seen.has(dedupeKey)) return;
      seen.add(dedupeKey);
      rows.push(row);
    });
  });
  return rows.slice(0, 14);
}

function RunKeyData({
  title,
  rows,
  defaultOpen = true,
}: {
  title: string;
  rows: Array<{ key: string; value: string }>;
  defaultOpen?: boolean;
}) {
  return (
    <details className="node-run-key-data" open={defaultOpen}>
      <summary>
        <span>{title}</span>
        <em>{rows.length ? `${rows.length} 项` : '无'}</em>
      </summary>
      <div className="node-run-key-data-body">
        {rows.length ? (
          <dl>
            {rows.map((row) => (
              <div key={`${row.key}-${row.value}`}>
                <dt>{row.key}</dt>
                <dd>{row.value}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <em>没有可提取的关键字段。</em>
        )}
      </div>
    </details>
  );
}

function CompactJsonViewer({ title, value }: { title: string; value: unknown }) {
  const [showFullData, setShowFullData] = useState(false);
  const rows = mergeRunKeyData(value);
  return (
    <div className="compact-json-viewer">
      <RunKeyData title={`关键 ${title}`} rows={rows} />
      <label className="node-run-full-toggle">
        <span>显示完整 {title}</span>
        <input type="checkbox" checked={showFullData} onChange={(event) => setShowFullData(event.target.checked)} />
      </label>
      {showFullData ? <JsonBlock title={title} value={value} /> : null}
    </div>
  );
}

function outputLogsFrom(value: unknown): string[] {
  if (!value || typeof value !== 'object') return [];
  const record = value as Record<string, unknown>;
  const candidates = [record.logs, record.log, record.audit, record.events, record.trace];
  const out: string[] = [];
  candidates.forEach((candidate) => {
    if (Array.isArray(candidate)) {
      candidate.forEach((item) => out.push(typeof item === 'string' ? item : safeJSONString(item)));
    } else if (typeof candidate === 'string') {
      out.push(candidate);
    } else if (candidate && typeof candidate === 'object') {
      out.push(safeJSONString(candidate));
    }
  });
  return out;
}

function compactRunLogs(logs: string[], errorMessage?: string): string[] {
  const important = logs.filter((line) => /error|failed|exception|warn|retry|skip|decision|status|grant|reward|outbox|event|idempotency/i.test(line));
  const rows = [...(errorMessage ? [`error ${errorMessage}`] : []), ...(important.length ? important : logs.slice(0, 3))];
  return rows.slice(0, 6);
}

function logClassName(line: string): string {
  if (/error|failed|exception/i.test(line)) return 'log-error';
  if (/warn|retry|skip/i.test(line)) return 'log-warn';
  return 'log-info';
}

function NodeRunInspector({ node }: { node: ActivityNode }) {
  const [showFullData, setShowFullData] = useState(false);
  const run = node.data.lastRun;
  const logs = outputLogsFrom(run?.output);
  const inputKeyRows = run ? mergeRunKeyData(run.input) : [];
  const outputKeyRows = run ? mergeRunKeyData(run.output) : [];
  const visibleLogs = run ? compactRunLogs(logs, run.errorMessage) : [];
  return (
    <div className="node-run-inspector">
      {run ? (
        <>
          <div className={cx('node-run-status-card', `run-${run.status.toLowerCase()}`)}>
            <b>{run.status}</b>
            <span>{run.finishedAt ? new Date(run.finishedAt).toLocaleString() : '运行中'}</span>
            <em>{run.errorMessage || `耗时 ${run.durationMs ?? 0}ms · retry ${run.retryCount ?? 0}`}</em>
          </div>
          <RunKeyData title="关键输入 Input" rows={inputKeyRows} />
          <RunKeyData title="关键输出 Output" rows={outputKeyRows} />
          <div className="node-run-log-block">
            <span>关键日志</span>
            {visibleLogs.length ? visibleLogs.map((line, index) => <code className={logClassName(line)} key={`${index}-${line.slice(0, 16)}`}>{line}</code>) : <em>当前节点输出里没有关键日志。</em>}
          </div>
          <label className="node-run-full-toggle">
            <span>显示完整输入 / 输出 / 日志</span>
            <input type="checkbox" checked={showFullData} onChange={(event) => setShowFullData(event.target.checked)} />
          </label>
          {showFullData ? (
            <div className="node-run-full-data">
              <JsonBlock title="完整节点输入 Input" value={run.input} />
              <JsonBlock title="完整节点输出 Output" value={run.output} />
              <div className="node-run-log-block">
                <span>完整输出日志</span>
                {logs.length ? logs.map((line, index) => <code className={logClassName(line)} key={`${index}-${line.slice(0, 16)}`}>{line}</code>) : <em>当前节点输出里没有 logs / audit / trace 字段。</em>}
              </div>
            </div>
          ) : null}
        </>
      ) : (
        <div className="run-summary empty">
          <b>NONE</b>
          <span>暂无运行记录</span>
          <span>点击调试运行或调用 OpenAPI 后，这里会展示当前节点的输入、输出和日志。</span>
        </div>
      )}
    </div>
  );
}

function isLuaCapableNode(node: ActivityNode): boolean {
  return ['code', 'combine', 'split'].includes(node.data.id) || String(node.data.config?.language ?? '').toUpperCase() === 'LUA';
}

function LuaScriptEditor({
  node,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  if (!isLuaCapableNode(node)) return null;
  const source = stringifyConfigValue(node.data.config?.source ?? '');
  const outputPath = stringifyConfigValue(node.data.config?.outputPath ?? (node.data.id === 'split' ? 'split_items' : node.data.id === 'combine' ? 'combined_result' : 'code_result'));
  const publicParams = node.data.config?.publicParams ?? {};
  const contextPatch = node.data.config?.contextPatch ?? {};
  return (
    <PropertyDisclosure title="Lua 脚本编辑器" summary={node.data.config?.language ? String(node.data.config.language) : 'LUA'} defaultOpen>
      <div className="lua-editor-panel">
        <label>
          <span>输出路径</span>
          <input value={outputPath} onChange={(event) => onNodeConfigChange(node.id, 'outputPath', event.target.value)} />
        </label>
        <label>
          <span>Lua Source</span>
          <textarea
            value={source}
            spellCheck={false}
            placeholder="emit({ result = ctx.node.input })"
            onChange={(event) => onNodeConfigChange(node.id, 'source', event.target.value)}
          />
        </label>
        <div className="lua-context-help">
          <b>可用上下文</b>
          <code>ctx.event</code>
          <code>ctx.activity</code>
          <code>ctx.runtime</code>
          <code>ctx.node.config</code>
          <code>ctx.node.input</code>
          <code>ctx.nodes</code>
          <code>ctx.vars</code>
          <span>使用 emit(&#123;...&#125;) 输出结果；公共参数通过 ctx.vars 读取。</span>
        </div>
        <label>
          <span>流程公共参数 publicParams</span>
          <textarea
            value={safeJSONString(publicParams)}
            spellCheck={false}
            placeholder='{"timezone":"Asia/Shanghai","campaign":"daily-checkin"}'
            onChange={(event) => onNodeConfigChange(node.id, 'publicParams', parseJSONConfig(event.target.value))}
          />
        </label>
        <label>
          <span>上下文写入 contextPatch</span>
          <textarea
            value={safeJSONString(contextPatch)}
            spellCheck={false}
            placeholder='{"vars.checkinRewardEnabled":true}'
            onChange={(event) => onNodeConfigChange(node.id, 'contextPatch', parseJSONConfig(event.target.value))}
          />
        </label>
      </div>
    </PropertyDisclosure>
  );
}

function edgeRouteFor(edge: ActivityEdge): WorkflowEdgeRoute {
  if (edge.data?.route) return edge.data.route;
  const label = typeof edge.label === 'string' ? edge.label : '';
  return {
    type: label ? 'OUTPUT' : 'DEFAULT',
    key: label || edge.sourceHandle || 'DEFAULT',
    label: label || '默认路径',
    default: !label,
  };
}

function compactRoute(route: WorkflowEdgeRoute): WorkflowEdgeRoute {
  return {
    type: route.type,
    key: route.key?.trim() || undefined,
    label: route.label?.trim() || undefined,
    expression: route.expression?.trim() || undefined,
    conditionRef: route.conditionRef?.trim() || undefined,
    default: route.default || undefined,
  };
}

function routeHealthIssues(route: WorkflowEdgeRoute): Array<{ severity: 'BLOCK' | 'WARNING'; message: string }> {
  const issues: Array<{ severity: 'BLOCK' | 'WARNING'; message: string }> = [];
  const needsCondition = ['IF', 'ELIF', 'PASS', 'BLOCK', 'RISK', 'SKIP'].includes(route.type);
  if ((route.type === 'OUTPUT' || needsCondition) && !route.key?.trim()) {
    issues.push({ severity: 'BLOCK', message: 'Route Key 不能为空。' });
  }
  if (needsCondition && !route.expression?.trim() && !route.conditionRef?.trim()) {
    issues.push({ severity: 'BLOCK', message: '条件分支需要 expression 或 conditionRef。' });
  }
  if (route.type === 'DEFAULT' && !route.default) {
    issues.push({ severity: 'WARNING', message: '默认路径建议勾选默认分支。' });
  }
  if (route.expression?.trim() && route.conditionRef?.trim()) {
    issues.push({ severity: 'WARNING', message: 'expression 和 conditionRef 同时存在时，建议只保留一种条件来源。' });
  }
  return issues;
}

function routeExpressionPreview(route: WorkflowEdgeRoute): { label: string; detail: string; result: 'pass' | 'fail' | 'unknown' } {
  if (route.default) return { label: '兜底命中', detail: '没有其它条件命中时选择该分支。', result: 'pass' };
  if (route.conditionRef?.trim()) return { label: '引用条件', detail: `${route.conditionRef.trim()} 为 truthy 时命中。`, result: 'unknown' };
  const expression = route.expression?.trim();
  if (!expression) return { label: '缺少条件', detail: '填写 expression 或 conditionRef 后可预览。', result: 'unknown' };
  const key = route.key?.trim() || route.type;
  const sample: Record<string, unknown> = {
    condition: true,
    decision: key,
    route: key,
    routeKey: key,
    status: key,
    risk: key === 'RISK',
    fact: { amount: 100 },
  };
  const match = expression.match(/^\$?\.?([a-zA-Z0-9_.]+)\s*(==|!=|>=|<=|>|<)\s*("?[^"]+"?|\d+(?:\.\d+)?|true|false)$/);
  if (!match) return { label: '语法未解析', detail: '支持常见 field == value / $.field >= number 预览；复杂表达式以运行时为准。', result: 'unknown' };
  const [, rawPath, operator, rawExpected] = match;
  const actual = rawPath.split('.').reduce<unknown>((current, part) => (current && typeof current === 'object' ? (current as Record<string, unknown>)[part] : undefined), sample);
  const expected = rawExpected.replace(/^"|"$/g, '');
  const actualNumber = typeof actual === 'number' ? actual : Number(actual);
  const expectedNumber = Number(expected);
  let passed = false;
  if (!Number.isNaN(actualNumber) && !Number.isNaN(expectedNumber) && ['>=', '<=', '>', '<'].includes(operator)) {
    passed = operator === '>=' ? actualNumber >= expectedNumber : operator === '<=' ? actualNumber <= expectedNumber : operator === '>' ? actualNumber > expectedNumber : actualNumber < expectedNumber;
  } else {
    const left = String(actual);
    passed = operator === '==' ? left === expected : operator === '!=' ? left !== expected : false;
  }
  return { label: passed ? '样例命中' : '样例不命中', detail: `样例 ${rawPath}=${String(actual)}，表达式 ${passed ? '为 true' : '为 false'}。`, result: passed ? 'pass' : 'fail' };
}

function EdgeRouteEditor({
  edge,
  validationIssues,
  onEdgeRouteChange,
}: {
  edge: ActivityEdge;
  validationIssues?: WorkflowValidationIssue[];
  onEdgeRouteChange: (edgeId: string, route: WorkflowEdgeRoute) => void;
}) {
  const route = edgeRouteFor(edge);
  const routeNeedsCondition = ['IF', 'ELIF', 'PASS', 'BLOCK', 'RISK', 'SKIP'].includes(route.type);
  const healthIssues = routeHealthIssues(route);
  const preview = routeExpressionPreview(route);
  const updateRoute = (patch: Partial<WorkflowEdgeRoute>) => {
    onEdgeRouteChange(edge.id, compactRoute({ ...route, ...patch }));
  };
  const changeType = (type: WorkflowEdgeRouteType) => {
    updateRoute({
      type,
      key: type === 'DEFAULT' ? 'DEFAULT' : type === 'OUTPUT' ? route.key : type,
      label: type === 'DEFAULT' ? '默认路径' : type === 'OUTPUT' ? route.label : type,
      expression: DEFAULT_ROUTE_EXPRESSIONS[type],
      conditionRef: undefined,
      default: DEFAULT_ROUTE_TYPES.has(type),
    });
  };
  const applyDefaultExpression = () => {
    const expression = DEFAULT_ROUTE_EXPRESSIONS[route.type];
    if (expression) updateRoute({ expression, conditionRef: undefined });
  };
  return (
    <PropertyDisclosure title="路由配置" summary={route.type} defaultOpen>
      <div className="schema-config-grid edge-route-editor">
        <div className="route-type-panel">
          <div className="route-type-current">
            <span>当前 Route</span>
            <b>{route.type}</b>
            <em>{route.default ? 'Default branch' : routeNeedsCondition ? 'Conditional' : 'Output'}</em>
          </div>
          <div className={cx('route-health-card', healthIssues.some((issue) => issue.severity === 'BLOCK') && 'block', healthIssues.length === 0 && 'pass')}>
            <div>
              <b>{healthIssues.length ? '需要处理' : 'Route 配置完整'}</b>
              <span>{preview.label} · {preview.detail}</span>
            </div>
            {!route.default ? <button type="button" onClick={() => updateRoute({ type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', expression: undefined, conditionRef: undefined, default: true })}>设为默认</button> : null}
          </div>
          {validationIssues?.length || healthIssues.length ? (
            <div className="route-issue-list">
              {healthIssues.map((issue) => <span key={issue.message} className={issue.severity.toLowerCase()}>{issue.message}</span>)}
              {(validationIssues ?? []).map((issue) => <span key={`${issue.code}-${issue.path ?? issue.message}`} className={issue.severity.toLowerCase()}>{issue.message}</span>)}
            </div>
          ) : null}
          {ROUTE_TYPE_GROUPS.map((group) => (
            <div key={group.title} className="route-type-group">
              <div>
                <b>{group.title}</b>
                <span>{group.description}</span>
              </div>
              <div>
                {group.items.map((type) => (
                  <button key={type} type="button" className={cx(route.type === type && 'active')} onClick={() => changeType(type)}>
                    {type}
                  </button>
                ))}
              </div>
            </div>
          ))}
        </div>
        <label className="schema-config-row">
          <span>
            <b>Route Key</b>
            {route.type === 'OUTPUT' || routeNeedsCondition ? <em>必填</em> : null}
          </span>
          <input value={route.key ?? ''} placeholder={route.type === 'OUTPUT' ? '例如 event_context' : route.type} onChange={(event) => updateRoute({ key: event.target.value })} />
          <small>用于分支覆盖、重复出口检测和 route key fallback。</small>
        </label>
        <label className="schema-config-row">
          <span><b>Label</b></span>
          <input value={route.label ?? ''} placeholder="画布上显示的连线文字" onChange={(event) => updateRoute({ label: event.target.value })} />
        </label>
        <label className="schema-config-row">
          <span>
            <b>条件表达式</b>
            {routeNeedsCondition && !route.conditionRef ? <em>条件</em> : null}
          </span>
          <textarea
            value={route.expression ?? ''}
            placeholder='例如 decision == "PASS" 或 $.fact.amount >= 100'
            onChange={(event) => updateRoute({ expression: event.target.value, conditionRef: event.target.value.trim() ? undefined : route.conditionRef })}
          />
          {DEFAULT_ROUTE_EXPRESSIONS[route.type] ? (
            <button type="button" className="route-expression-action" onClick={applyDefaultExpression}>使用默认表达式</button>
          ) : null}
          <div className="route-expression-hints">
            {ROUTE_EXPRESSION_HINTS.map((hint) => (
              <button key={hint.label} type="button" onClick={() => updateRoute({ expression: hint.expression, conditionRef: undefined })}>
                {hint.label}
              </button>
            ))}
          </div>
          <div className={cx('route-expression-preview', preview.result)}>
            <b>{preview.label}</b>
            <span>{preview.detail}</span>
          </div>
          <small>Executor 会按该表达式或 Condition Ref 判断是否选择这条边。</small>
        </label>
        <label className="schema-config-row">
          <span><b>Condition Ref</b></span>
          <input
            value={route.conditionRef ?? ''}
            placeholder="例如 fact.risk.flagged"
            onChange={(event) => updateRoute({ conditionRef: event.target.value, expression: event.target.value.trim() ? undefined : route.expression })}
          />
          <small>指向上游节点输出里的布尔值或 truthy 字段。</small>
        </label>
        <div className="route-default-card">
          <label className="switch-row">
            <span>默认分支</span>
            <input type="checkbox" checked={Boolean(route.default)} onChange={(event) => updateRoute({ default: event.target.checked })} />
          </label>
          <p>分支节点必须显式存在默认分支。默认分支会在条件未命中时作为兜底路径。</p>
        </div>
      </div>
    </PropertyDisclosure>
  );
}

type Props = {
  nodes: FlowNode[];
  edges: ActivityEdge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  nodeConfigSchemas: WorkflowNodeConfigSchema[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
  onEdgeRouteChange: (edgeId: string, route: WorkflowEdgeRoute) => void;
  onLocateNode?: (nodeId: string) => void;
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (ids: string[]) => void;
  panelWidth: number;
  onResizeStart: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  resourceOptions: ResourceSummary[];
  validationIssues?: WorkflowValidationIssue[];
  edgeValidationIssues?: WorkflowValidationIssue[];
};

export function PropertyPanel({ nodes, edges, selectedNodeIds, selectedEdgeIds, nodeConfigSchemas, onNodeConfigChange, onEdgeRouteChange, onLocateNode, onDeleteNodes, onDeleteEdges, panelWidth, onResizeStart, resourceOptions, validationIssues = [], edgeValidationIssues = [] }: Props) {
  const [activeTab, setActiveTab] = useState<PropertyPanelTab>(loadPropertyPanelTab);
  const selectedNode = nodes.find((node): node is ActivityNode => node.id === selectedNodeIds[0] && node.type === 'activity');
  const selectedGroup = nodes.find((node): node is GroupNode => node.id === selectedNodeIds[0] && node.type === 'group');
  const selectedNote = nodes.find((node): node is NoteNode => node.id === selectedNodeIds[0] && node.type === 'note');
  const selectedEdge = edges.find((item) => item.id === selectedEdgeIds[0]);
  const selectedEdgeRoute = selectedEdge ? edgeRouteFor(selectedEdge) : undefined;
  const selectedNodeSchema = selectedNode ? nodeConfigSchemas.find((schema) => schema.nodeType === selectedNode.data.id) : undefined;
  const hasSelection = Boolean(selectedNode || selectedGroup || selectedNote || selectedEdge);
  const changeActiveTab = (tab: PropertyPanelTab) => {
    setActiveTab(tab);
    window.localStorage.setItem(PROPERTY_PANEL_TAB_STORAGE_KEY, tab);
  };
  const setAllSettingPanelsOpen = (open: boolean) => {
    document.querySelectorAll<HTMLDetailsElement>('.property-panel .property-body .property-disclosure').forEach((item) => {
      item.open = open;
    });
  };

  return (
    <aside className={cx('property-panel', hasSelection && 'open')} style={{ width: panelWidth }}>
      <button className="property-resize-handle right-panel-resize-handle" aria-label="调整属性面板宽度" onPointerDown={onResizeStart} />
      <div className="property-head">
        <div className="property-title-with-icon">
          {selectedNode ? <NodeKindIcon node={selectedNode.data} className="node-icon" /> : null}
          <b>{selectedNode ? selectedNode.data.title : selectedGroup ? selectedGroup.data.title : selectedNote ? '备注' : selectedEdge ? '流程连线' : '属性'}</b>
        </div>
        <span>{selectedNode ? '节点' : selectedGroup ? '组合' : selectedNote ? '备注' : selectedEdge ? '连线' : '未选择'}</span>
      </div>
      {selectedNode ? (
        <div className="property-tabs">
          <div className="property-tab-buttons">
            <button className={activeTab === 'settings' ? 'active' : ''} onClick={() => changeActiveTab('settings')}>设置</button>
            <button className={activeTab === 'run' ? 'active' : ''} onClick={() => changeActiveTab('run')}>上次运行</button>
          </div>
          <div className="property-tab-actions">
            <button type="button" onClick={() => setAllSettingPanelsOpen(true)}>全部展开</button>
            <button type="button" onClick={() => setAllSettingPanelsOpen(false)}>全部收起</button>
          </div>
        </div>
      ) : null}
      {selectedNode && activeTab === 'settings' ? (
        <div className="property-body">
          <PropertyDisclosure
            title="节点信息"
            titleMeta={`x ${Math.round(selectedNode.position.x)} / y ${Math.round(selectedNode.position.y)}`}
            summary={`${selectedNode.data.code} · ${selectedNodeSchema ? `v${selectedNodeSchema.version}` : selectedNode.data.kind}`}
            compact
            defaultOpen
          >
            <NodeInfoSummary node={selectedNode} schema={selectedNodeSchema} />
          </PropertyDisclosure>
          <PropertyDisclosure title="配置参数" summary={selectedNodeSchema ? `${selectedNodeSchema.fields.length} 项 / ${requiredConfigCount(selectedNodeSchema)} 必填` : '未找到 schema'}>
            <NodeConfigForm node={selectedNode} schema={selectedNodeSchema} validationIssues={validationIssues} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />
          </PropertyDisclosure>
          <LuaScriptEditor node={selectedNode} onNodeConfigChange={onNodeConfigChange} />
          <InputMappingEditor node={selectedNode} schema={selectedNodeSchema} nodes={nodes} edges={edges} schemas={nodeConfigSchemas} onNodeConfigChange={onNodeConfigChange} onLocateNode={onLocateNode} />
          {selectedNodeSchema ? <NodeConfigPreview schema={selectedNodeSchema} /> : null}
          <PropertyList title="输出变量" items={(selectedNodeSchema?.outputs ?? []).map((output) => output.key)} />
          <PropertyDisclosure title="下一步" summary={selectedNode.data.outputs[0] ?? '默认路径'}>
            <div className="next-step-card">
              <NodeKindIcon node={selectedNode.data} className="palette-dot" />
              <div>
                <b>{selectedNode.data.outputs[0] ?? '默认路径'}</b>
                <p>连接下游活动节点，决定下一步执行链路。</p>
              </div>
            </div>
          </PropertyDisclosure>
          <PropertyDisclosure title="失败时重试" summary={selectedNode.data.kind === 'reliable' || selectedNode.data.kind === 'dispatch' ? '启用' : '关闭'}>
            <label className="switch-row">
              <span>启用重试</span>
              <input type="checkbox" checked={selectedNode.data.kind === 'reliable' || selectedNode.data.kind === 'dispatch'} readOnly />
            </label>
            <div className="retry-row">
              <span>最大次数</span>
              <b>{selectedNode.data.kind === 'reliable' ? 5 : 3}</b>
              <span>退避</span>
              <b>指数退避</b>
            </div>
          </PropertyDisclosure>
          <PropertyDisclosure title="异常处理" summary="无">
            <button className="exception-select">无</button>
          </PropertyDisclosure>
        </div>
      ) : null}
      {selectedNode && activeTab === 'run' ? (
        <div className="property-body">
          <PropertyDisclosure title="运行概览" summary={selectedNode.data.lastRun?.status ?? 'NONE'} defaultOpen>
            <NodeRunInspector node={selectedNode} />
          </PropertyDisclosure>
          <PropertyDisclosure title="节点配置快照" summary={selectedNode.data.id}>
            <CompactJsonViewer title="config" value={selectedNode.data.config ?? {}} />
          </PropertyDisclosure>
          <PropertyDisclosure title="流程上下文说明" summary="ctx">
            <div className="lua-context-help">
              <b>调试时可关注这些上下文</b>
              <code>ctx.event 当前事件</code>
              <code>ctx.node.input 当前节点输入</code>
              <code>ctx.node.config 当前节点配置</code>
              <code>ctx.nodes 上游节点输出</code>
              <code>ctx.vars 流程公共参数</code>
              <span>Lua 节点可以在设置页编辑脚本，并通过 emit 输出到当前节点 output。</span>
            </div>
          </PropertyDisclosure>
        </div>
      ) : null}
      {!selectedNode && selectedGroup ? (
        <div className="property-body">
          <section className="property-card">
            <h2>{selectedGroup.data.title}</h2>
            <dl>
              <div><dt>组合 ID</dt><dd>{selectedGroup.id}</dd></div>
              <div><dt>尺寸</dt><dd>{Math.round(selectedGroup.data.width)} x {Math.round(selectedGroup.data.height)}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedGroup.position.x)} / y {Math.round(selectedGroup.position.y)}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteNodes([selectedGroup.id])}>删除组合</button>
          </section>
        </div>
      ) : null}
      {!selectedNode && !selectedGroup && selectedNote ? (
        <div className="property-body">
          <section className="property-card">
            <h2>备注</h2>
            <dl>
              <div><dt>备注 ID</dt><dd>{selectedNote.id}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedNote.position.x)} / y {Math.round(selectedNote.position.y)}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteNodes([selectedNote.id])}>删除备注</button>
          </section>
          <section className="property-section">
            <h3>内容</h3>
            <p>{selectedNote.data.text}</p>
          </section>
        </div>
      ) : null}
      {!selectedNode && !selectedGroup && !selectedNote && selectedEdge ? (
        <div className="property-body">
          <section className="property-card">
            <h2>流程连线</h2>
            <dl>
              <div><dt>连线 ID</dt><dd>{selectedEdge.id}</dd></div>
              <div><dt>来源节点</dt><dd>{selectedEdge.source}</dd></div>
              <div><dt>目标节点</dt><dd>{selectedEdge.target}</dd></div>
              <div><dt>标签</dt><dd>{selectedEdge.label || '默认路径'}</dd></div>
              <div><dt>Route Type</dt><dd>{selectedEdgeRoute?.type ?? 'DEFAULT'}</dd></div>
              <div><dt>Route Key</dt><dd>{selectedEdgeRoute?.key ?? selectedEdge.sourceHandle ?? 'DEFAULT'}</dd></div>
              <div><dt>Expression</dt><dd>{selectedEdgeRoute?.expression ?? '未配置'}</dd></div>
              <div><dt>Condition Ref</dt><dd>{selectedEdgeRoute?.conditionRef ?? '未配置'}</dd></div>
              <div><dt>Default</dt><dd>{selectedEdgeRoute?.default ? '是' : '否'}</dd></div>
            </dl>
            <button className="property-danger" onClick={() => onDeleteEdges([selectedEdge.id])}>删除连线</button>
          </section>
          <EdgeRouteEditor edge={selectedEdge} validationIssues={edgeValidationIssues} onEdgeRouteChange={onEdgeRouteChange} />
        </div>
      ) : null}
      {!hasSelection ? (
        <div className="property-empty-state">
          <b>选择一个节点</b>
          <p>点击画布中的活动节点后，可以在这里查看输入、输出、说明和后续配置项。</p>
        </div>
      ) : null}
    </aside>
  );
}
