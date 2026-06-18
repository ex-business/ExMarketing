import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { MouseEvent as ReactMouseEvent } from 'react';
import type { ReactNode } from 'react';
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, ChevronUp, Database, Eye, FileInput, FileOutput, Search, SlidersHorizontal, X } from 'lucide-react';
import type { ActivityEdge, ActivityNode, FlowNode, GroupNode, NoteNode, ResourceSummary, WorkflowEdgeRoute, WorkflowEdgeRouteType, WorkflowNodeConfigField, WorkflowNodeConfigSchema, WorkflowNodeConfigValidationRule, WorkflowValidationIssue } from '../types';
import { cx } from '../utils/flow';
import { NodeKindIcon } from './NodeKindIcon';
import type { LuaConfigVariable } from './LuaConfigCodeEditor';

const LazyLuaConfigCodeEditor = lazy(() => import('./LuaConfigCodeEditor').then((module) => ({ default: module.LuaConfigCodeEditor })));

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
type PropertyPanelTab = 'basic' | 'config' | 'lua' | 'other' | 'run' | 'debug';

const ROUTE_EXPRESSION_HINTS = [
  { label: '决策等于 PASS', expression: 'decision == "PASS"' },
  { label: '决策等于 BLOCK', expression: 'decision == "BLOCK"' },
  { label: '状态成功', expression: 'status == "SUCCESS"' },
  { label: '风险命中', expression: 'risk == true' },
  { label: '金额达标', expression: '$.fact.amount >= 100' },
];

function loadPropertyPanelTab(): PropertyPanelTab {
  const stored = window.localStorage.getItem(PROPERTY_PANEL_TAB_STORAGE_KEY);
  if (stored === 'settings') return 'config';
  return stored === 'basic' || stored === 'config' || stored === 'lua' || stored === 'other' || stored === 'run' || stored === 'debug' ? stored : 'basic';
}

function PropertyDisclosure({
  title,
  titleMeta,
  summary,
  description,
  children,
  defaultOpen = false,
  compact = false,
}: {
  title: string;
  titleMeta?: string;
  summary?: string;
  description?: string;
  children: ReactNode;
  defaultOpen?: boolean;
  compact?: boolean;
}) {
  return (
    <details className={cx('property-section property-disclosure', compact && 'compact')} open={defaultOpen}>
      <summary>
        <span>
          {title}
          {titleMeta ? <small>（{titleMeta}）</small> : null}
          {!titleMeta && summary ? <small>（{summary}）</small> : null}
          {description ? (
            <details className="property-info-popover" onClick={(event) => event.stopPropagation()}>
              <summary aria-label={`${title}说明`}>i</summary>
              <p>{description}</p>
            </details>
          ) : null}
        </span>
      </summary>
      <div className="property-disclosure-body">
        {children}
      </div>
    </details>
  );
}

function OutputRouteSummary({ node, schema }: { node: ActivityNode; schema?: WorkflowNodeConfigSchema }) {
  const outputs = (schema?.outputs ?? []).map((output) => output.key);
  const routeOutputs = node.data.outputs.length ? node.data.outputs : outputs;
  return (
    <div className="output-route-panel direct">
      <div className="output-route-row">
        <span>输出</span>
        <div className="property-tags compact-tags">
          {outputs.length ? outputs.map((item) => <b key={item}>{item}</b>) : <em>未配置输出</em>}
        </div>
      </div>
      <div className="output-route-row">
        <span>路由</span>
        <div className="route-chip-list">
          {routeOutputs.length ? routeOutputs.map((item, index) => (
            <i key={`${item}-${index}`}>{item}</i>
          )) : <em>默认路径</em>}
        </div>
      </div>
    </div>
  );
}

function ConfigAccordionSection({
  title,
  summary,
  icon,
  tone = 'blue',
  className,
  open,
  onToggle,
  children,
}: {
  title: string;
  summary?: string;
  icon?: ReactNode;
  tone?: 'blue' | 'purple' | 'green' | 'amber' | 'gray';
  className?: string;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <section className={cx('config-accordion-section', open && 'open', `tone-${tone}`, className)}>
      <button type="button" className="config-accordion-head" onClick={onToggle}>
        {icon ? <i>{icon}</i> : null}
        <span>
          <b>{title}</b>
          {summary ? <small>（{summary}）</small> : null}
        </span>
        <em aria-hidden="true">{open ? <ChevronUp /> : <ChevronDown />}</em>
      </button>
      {open ? <div className="config-accordion-body">{children}</div> : null}
    </section>
  );
}

function configSectionVisual(title: string): { icon: ReactNode; tone: 'blue' | 'purple' | 'green' | 'amber' | 'gray' } {
  if (title.includes('数据提供')) return { icon: <Database size={18} />, tone: 'purple' };
  if (title.includes('输入')) return { icon: <FileInput size={18} />, tone: 'green' };
  if (title.includes('输出')) return { icon: <FileOutput size={18} />, tone: 'amber' };
  if (title.includes('查询') || title.includes('配置') || title.includes('事实')) return { icon: <SlidersHorizontal size={18} />, tone: 'blue' };
  return { icon: <SlidersHorizontal size={18} />, tone: 'gray' };
}

type InputMappingValue = {
  sourceNodeId: string;
  outputKey: string;
  path?: string;
};

type UpstreamOutputOption = {
  value: string;
  nodeId: string;
  nodeTitle: string;
  nodeCode: string;
  output: string;
  factKeys?: string[];
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
    ...(option.factKeys ?? []),
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

function displayInputMappingFor(node: ActivityNode): Record<string, InputMappingValue> {
  const mapping = inputMappingFor(node);
  if (node.data.id !== 'qualify') return mapping;
  const next: Record<string, InputMappingValue> = { ...mapping };
  const factsRef = mapping.facts ?? (mapping.event_context?.outputKey === 'facts' ? mapping.event_context : undefined);
  delete next.event_context;
  delete next.facts;
  if (factsRef) {
    (['USER_PROFILE', 'KYC_PROFILE', 'RISK_PROFILE', 'DEPOSIT_SUMMARY'] as const).forEach((key) => {
      if (!next[key]) {
        next[key] = {
          sourceNodeId: factsRef.sourceNodeId,
          outputKey: factsRef.outputKey,
          path: `$.${key}`,
        };
      }
    });
  }
  return next;
}

function displayInputsForNode(
  node: ActivityNode,
  schema?: WorkflowNodeConfigSchema,
): NonNullable<WorkflowNodeConfigSchema['inputs']> {
  const inputs = schema?.inputs?.length
    ? schema.inputs
    : node.data.inputs.map((input): NonNullable<WorkflowNodeConfigSchema['inputs']>[number] => ({ key: input, label: input, type: undefined, required: false, description: undefined }));
  if (node.data.id !== 'qualify') return inputs;
  const existing = new Map(inputs.filter((input) => input.key !== 'event_context' && input.key !== 'facts').map((input) => [input.key, input]));
  const next: NonNullable<WorkflowNodeConfigSchema['inputs']> = [
    existing.get('USER_PROFILE') ?? { key: 'USER_PROFILE', label: 'USER_PROFILE', type: 'object', required: false, description: '用户资料对象；未映射时按空对象处理。' },
    existing.get('KYC_PROFILE') ?? { key: 'KYC_PROFILE', label: 'KYC_PROFILE', type: 'object', required: false, description: 'KYC / 实名认证对象；未映射时按空对象处理。' },
    existing.get('RISK_PROFILE') ?? { key: 'RISK_PROFILE', label: 'RISK_PROFILE', type: 'object', required: false, description: '风控画像对象；未映射时按空对象处理。' },
    existing.get('DEPOSIT_SUMMARY') ?? { key: 'DEPOSIT_SUMMARY', label: 'DEPOSIT_SUMMARY', type: 'object', required: false, description: '充值汇总对象；未映射时按空对象处理。' },
  ];
  if (!next.some((input) => input.key === 'matched_activity')) {
    next.push({
      key: 'matched_activity',
      label: 'Matched Activity',
      type: 'object',
      required: false,
      description: '活动匹配结果；活动绑定流程可省略，默认使用当前 activity。',
    });
  }
  return next;
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

function factKeysForNode(node: ActivityNode): string[] {
  const value = node.data.config?.factKeys;
  if (Array.isArray(value)) return value.map((item) => String(item).trim()).filter(Boolean);
  if (typeof value === 'string') return parseArrayInput(value);
  return [];
}

function upstreamOutputOptions(node: ActivityNode, nodes: FlowNode[], edges: ActivityEdge[], schemas: WorkflowNodeConfigSchema[]): UpstreamOutputOption[] {
  const schemaByType = new Map(schemas.map((schema) => [schema.nodeType, schema]));
  const directUpstreamIds = new Set(edges.filter((edge) => edge.target === node.id).map((edge) => edge.source));
  const upstreamIds = upstreamNodeIds(node, edges);
  return upstreamIds
    .sort((left, right) => Number(!directUpstreamIds.has(left)) - Number(!directUpstreamIds.has(right)))
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
        factKeys: output.key === 'facts' ? factKeysForNode(sourceNode) : undefined,
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

function optionForInput(inputKey: string, options: UpstreamOutputOption[]): { option: UpstreamOutputOption; path?: string } | undefined {
  const direct = options.find((option) => option.output === inputKey)
    ?? options.find((option) => option.output.toLowerCase() === inputKey.toLowerCase());
  if (direct) return { option: direct };
  if (['USER_PROFILE', 'KYC_PROFILE', 'RISK_PROFILE', 'DEPOSIT_SUMMARY'].includes(inputKey)) {
    const facts = options.find((option) => option.output === 'facts' && option.factKeys?.some((key) => key.toUpperCase() === inputKey))
      ?? options.find((option) => option.output === 'facts');
    if (facts) return { option: facts, path: `$.${inputKey}` };
  }
  return undefined;
}

function inputSatisfiedByOption(inputKey: string, option: UpstreamOutputOption): boolean {
  return option.output === inputKey
    || option.output.toLowerCase() === inputKey.toLowerCase()
    || (['USER_PROFILE', 'KYC_PROFILE', 'RISK_PROFILE', 'DEPOSIT_SUMMARY'].includes(inputKey) && option.output === 'facts');
}

function InputMappingEditor({
  node,
  schema,
  nodes,
  edges,
  schemas,
  onNodeConfigChange,
  onLocateNode,
  defaultOpen = false,
  direct = false,
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  nodes: FlowNode[];
  edges: ActivityEdge[];
  schemas: WorkflowNodeConfigSchema[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
  onLocateNode?: (nodeId: string) => void;
  defaultOpen?: boolean;
  direct?: boolean;
}) {
  const [variableSearch, setVariableSearch] = useState('');
  const [typeFilter, setTypeFilter] = useState('ALL');
  const [expandedInputKey, setExpandedInputKey] = useState<string | null>(null);
  const options = upstreamOutputOptions(node, nodes, edges, schemas);
  const filteredOptions = filterVariableOptions(options, variableSearch, typeFilter);
  const optionGroups = upstreamOutputGroups(filteredOptions);
  const typeFilters = outputTypeFilters(options);
  const mapping = displayInputMappingFor(node);
  const hasMappedInputs = Object.keys(mapping).length > 0;
  const inputs = displayInputsForNode(node, schema);
  const autoMappingCandidates = inputs.filter((input) => !mapping[input.key] && Boolean(optionForInput(input.key, options)));
  const applyAutoMapping = () => {
    const next = { ...mapping };
    inputs.forEach((input) => {
      if (next[input.key]) return;
      const selected = optionForInput(input.key, options);
      if (selected) {
        next[input.key] = { sourceNodeId: selected.option.nodeId, outputKey: selected.option.output, path: selected.path };
      }
    });
    onNodeConfigChange(node.id, 'inputMapping', next);
  };
  const repairInvalidMappings = () => {
    const next: Record<string, InputMappingValue> = {};
    inputs.forEach((input) => {
      const current = mapping[input.key];
      const currentOption = current ? options.find((option) => option.nodeId === current.sourceNodeId && option.output === current.outputKey) : undefined;
      const replacement = optionForInput(input.key, options);
      if (current && currentOption) {
        next[input.key] = {
          ...current,
          path: current.path && current.path.trim().startsWith('$') ? current.path.trim() : undefined,
        };
        return;
      }
      if (replacement) {
        next[input.key] = { sourceNodeId: replacement.option.nodeId, outputKey: replacement.option.output, path: replacement.path };
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
        next[inputName] = {
          sourceNodeId: selected.nodeId,
          outputKey: selected.output,
          path: inputSatisfiedByOption(inputName, selected) && selected.output === 'facts' && inputName !== 'facts' ? `$.${inputName}` : undefined,
        };
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
  const content = inputs.length === 0 ? (
        <p className="property-empty">该节点没有输入变量。</p>
      ) : options.length === 0 && !hasMappedInputs ? (
        <p className="property-empty">先连接上游节点，再从上游输出中选择输入来源。</p>
      ) : (
        <div className="input-map-grid">
          {(autoMappingCandidates.length > 0 || invalidMappingCount > 0) ? (
            <div className="input-map-actions compact">
              {autoMappingCandidates.length > 0 ? <button type="button" onClick={applyAutoMapping}>自动映射</button> : null}
              {invalidMappingCount > 0 ? <button type="button" onClick={repairInvalidMappings}>修复</button> : null}
              {invalidMappingCount > 0 ? <button type="button" onClick={clearInvalidMappings}>清空失效</button> : null}
            </div>
          ) : null}
          {inputs.map((input) => {
            const selected = mapping[input.key];
            const selectedValue = selected ? `${selected.sourceNodeId}::${selected.outputKey}` : '';
            const selectedOption = selectedOptionFor(input.key);
            const invalidReference = Boolean(selected && !selectedOption);
            const missingRequired = Boolean(input.required && !selected);
            const invalidPath = Boolean(selected?.path && !selected.path.trim().startsWith('$'));
            const sourceNode = selected ? sourceNodeFor(selected.sourceNodeId) : undefined;
            const expanded = expandedInputKey === input.key;
            return (
              <div key={input.key} className={cx('input-map-row', (invalidReference || missingRequired || invalidPath) && 'invalid')}>
                <div className="input-map-name">
                  <span>
                    <b>{input.label || input.key}{input.required ? <em className="required-star" aria-label="必填">（*）</em> : null}</b>
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
                  <button type="button" className="input-map-expand" onClick={() => setExpandedInputKey(expanded ? null : input.key)}>
                    {expanded ? '收起' : '配置'}
                  </button>
                </div>
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
                {selected && expanded ? (
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
                {expanded ? (
                  <>
                    {input.description ? <small>{input.description}</small> : null}
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
                                  {option.factKeys?.length ? <small>{option.factKeys.join(' / ')}</small> : null}
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
                  </>
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
      );
  if (direct) {
    return <div className="input-mapping-direct">{content}</div>;
  }
  return (
    <PropertyDisclosure title="输入映射" summary={`${inputs.length} 个输入`} description="把当前节点的输入变量绑定到上游节点输出，可用 path 只取输出对象中的某一段。" defaultOpen={defaultOpen}>
      {content}
    </PropertyDisclosure>
  );
}

function configValue(node: ActivityNode, field: WorkflowNodeConfigField): unknown {
  if (node.data.config && Object.prototype.hasOwnProperty.call(node.data.config, field.key)) {
    return node.data.config[field.key];
  }
  return field.default ?? '';
}

function compactFieldValue(value: unknown): string {
  if (value === undefined || value === null || value === '') return '';
  if (Array.isArray(value)) return value.map((item) => compactFieldValue(item)).filter(Boolean).join(', ');
  if (typeof value === 'object') return '';
  return String(value);
}

function configGroupSummary(node: ActivityNode, group: NonNullable<WorkflowNodeConfigSchema['fieldGroups']>[number]): string {
  const providerChainField = group.fields.find((field) => field.key === 'providerChain');
  if (providerChainField) {
    const chain = providerChainValueFor(node, configValue(node, providerChainField));
    return chain.length ? chain.map((step) => step.source || 'PROVIDER').join(' → ') : '未配置';
  }
  const qualifyRulesField = group.fields.find((field) => field.key === 'qualifyRules');
  if (qualifyRulesField) {
    const rules = normalizeQualifyRules(configValue(node, qualifyRulesField), node.data.config?.eligibilityRule, node.data.config?.ruleCodes);
    return rules.length ? rules.map((rule) => rule.ruleCode || 'RULE').join(' → ') : '未配置';
  }
  if (group.fields.length === 1 && group.fields[0]?.key === 'factKeys') {
    return '';
  }
  const values = group.fields
    .map((field) => compactFieldValue(configValue(node, field)))
    .filter(Boolean);
  if (!values.length) return '';
  return values.slice(0, 2).join(' / ');
}

function configGroupDisplayTitle(group: NonNullable<WorkflowNodeConfigSchema['fieldGroups']>[number]): string {
  if (group.fields.some((field) => field.key === 'providerChain')) return '事实来源';
  if (group.fields.some((field) => field.key === 'qualifyRules')) return '资格规则';
  return group.title || group.key || '配置';
}

function qualifyRulesField(): WorkflowNodeConfigField {
  return {
    key: 'qualifyRules',
    label: '资格规则',
    type: 'ARRAY',
    required: false,
    description: '按顺序配置资格规则；每条规则可独立设置编码、条件和启用状态。',
    validationRules: [{ type: 'MAX_ITEMS', value: 50 }],
  };
}

function displayConfigGroupsForNode(
  node: ActivityNode,
  schema?: WorkflowNodeConfigSchema,
): NonNullable<WorkflowNodeConfigSchema['fieldGroups']> {
  const baseGroups = schema?.fieldGroups?.length
    ? schema.fieldGroups
    : schema
      ? [{ key: 'base', title: schema.title, description: schema.description, fields: schema.fields }]
      : [];
  if (node.data.id !== 'qualify') return baseGroups;
  return baseGroups.map((group) => {
    const hasRuleCodes = group.fields.some((field) => field.key === 'ruleCodes');
    const hasQualifyRules = group.fields.some((field) => field.key === 'qualifyRules');
    if (!hasRuleCodes && !hasQualifyRules) return group;
    return {
      ...group,
      title: '资格规则',
      description: '',
      fields: [qualifyRulesField()],
    };
  }).flatMap((group) => {
    const original = baseGroups.find((item) => item.key === group.key);
    const originalFields = original?.fields ?? group.fields;
    const strategyFields = node.data.id === 'qualify' && originalFields.some((field) => field.key === 'ruleCodes' || field.key === 'qualifyRules')
      ? originalFields.filter((field) => field.key !== 'ruleCodes' && field.key !== 'qualifyRules')
      : [];
    if (!strategyFields.length) return [group];
    return [
      group,
      {
        key: `${group.key || 'rules'}-strategy`,
        title: '执行策略',
        description: '',
        fields: strategyFields,
      },
    ];
  });
}

function nodeInputSummary(node: ActivityNode, schema?: WorkflowNodeConfigSchema): string {
  const inputs = displayInputsForNode(node, schema);
  const firstInput = inputs[0];
  if (!firstInput) return '无输入';
  const mappings = displayInputMappingFor(node);
  const mapping = mappings[firstInput.key];
  if (!mapping) return `${firstInput.label || firstInput.key} 未映射`;
  return `${firstInput.label || firstInput.key} ← ${mapping.sourceNodeId}/${mapping.outputKey}`;
}

function nodeOutputSummary(schema?: WorkflowNodeConfigSchema): string {
  if (!schema?.outputs?.length) return '无输出';
  return schema.outputs.slice(0, 2).map((item) => item.key).join(' / ');
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

type ProviderChainStepValue = {
  source: string;
  resourceRef?: string;
  namespace?: string;
  sourcePath?: string;
  cachePath?: string;
  keyTemplate?: string;
  valuePath?: string;
  endpoint?: string;
  method?: 'GET' | 'POST' | string;
  authRef?: string;
  providerCode?: string;
  redisRef?: string;
  grpcTarget?: string;
  pluginCode?: string;
  operation?: 'READ' | 'CALL' | string;
  timeoutSeconds?: string;
  cacheTtlSeconds?: string;
  onMiss?: 'NEXT' | 'BLOCK' | string;
};

const PROVIDER_CHAIN_SOURCES = ['WORKFLOW_CONTEXT', 'LOCAL_CACHE', 'FACT_SERVICE', 'HTTP_PROVIDER', 'REDIS', 'GRPC', 'PLUGIN', 'CUSTOM', 'LUA_QUERY'];

function providerChainNeedsResource(source: string): boolean {
  return ['FACT_SERVICE', 'HTTP_PROVIDER', 'REDIS', 'GRPC', 'PLUGIN', 'CUSTOM', 'LUA_QUERY'].includes(source);
}

function providerChainRequiresManagedResource(source: string): boolean {
  return ['REDIS', 'GRPC', 'PLUGIN', 'CUSTOM', 'LUA_QUERY'].includes(source);
}

function providerChainHasTarget(step: ProviderChainStepValue): boolean {
  if (!providerChainNeedsResource(step.source)) return true;
  if (step.resourceRef) return true;
  if (step.source === 'FACT_SERVICE' || step.source === 'HTTP_PROVIDER') {
    return Boolean(step.providerCode || step.endpoint);
  }
  return false;
}

function emptyProviderChainStep(source = 'FACT_SERVICE'): ProviderChainStepValue {
  return {
    source,
    namespace: 'facts',
    operation: source === 'HTTP_PROVIDER' ? 'CALL' : 'READ',
    onMiss: 'NEXT',
  };
}

function normalizeProviderChain(value: unknown): ProviderChainStepValue[] {
  const stringProp = (mapped: Record<string, unknown>, ...keys: string[]) => {
    for (const key of keys) {
      const candidate = mapped[key];
      if (typeof candidate === 'string') return candidate;
    }
    return undefined;
  };
  const luaProviderKeyMap: Record<string, keyof ProviderChainStepValue> = {
    SOURCE: 'source',
    RESOURCE_REF: 'resourceRef',
    RESOURCEREF: 'resourceRef',
    NAMESPACE: 'namespace',
    SOURCE_PATH: 'sourcePath',
    SOURCEPATH: 'sourcePath',
    CACHE_PATH: 'cachePath',
    CACHEPATH: 'cachePath',
    KEY_TEMPLATE: 'keyTemplate',
    KEYTEMPLATE: 'keyTemplate',
    VALUE_PATH: 'valuePath',
    VALUEPATH: 'valuePath',
    ENDPOINT: 'endpoint',
    METHOD: 'method',
    AUTH_REF: 'authRef',
    AUTHREF: 'authRef',
    PROVIDER_CODE: 'providerCode',
    PROVIDERCODE: 'providerCode',
    REDIS_REF: 'redisRef',
    REDISREF: 'redisRef',
    GRPC_TARGET: 'grpcTarget',
    GRPCTARGET: 'grpcTarget',
    PLUGIN_CODE: 'pluginCode',
    PLUGINCODE: 'pluginCode',
    OPERATION: 'operation',
    TIMEOUT_SECONDS: 'timeoutSeconds',
    TIMEOUTSECONDS: 'timeoutSeconds',
    CACHE_TTL_SECONDS: 'cacheTtlSeconds',
    CACHETTLSECONDS: 'cacheTtlSeconds',
    ON_MISS: 'onMiss',
    ONMISS: 'onMiss',
  };
  const stepFromString = (raw: string): ProviderChainStepValue | undefined => {
    const trimmed = raw.trim().replace(/\\"/g, '"').replace(/\\'/g, "'");
    const sourceMatch = trimmed.match(/\b(?:SOURCE|source)\s*=\s*["']?([A-Za-z0-9_/-]+)["']?/);
    if (!sourceMatch && /[={}:]/.test(trimmed)) return undefined;
    const source = sourceMatch?.[1] || trimmed;
    const step = emptyProviderChainStep(source.trim().toUpperCase());
    const pairPattern = /\b([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^"',}\n]+))/g;
    let pair: RegExpExecArray | null;
    while ((pair = pairPattern.exec(trimmed))) {
      const mappedKey = luaProviderKeyMap[pair[1].replace(/([a-z])([A-Z])/g, '$1_$2').toUpperCase()];
      if (mappedKey && mappedKey !== 'source') {
        (step as Record<string, string>)[mappedKey] = pair[2] ?? pair[3] ?? pair[4] ?? '';
      }
    }
    return step;
  };
  if (Array.isArray(value)) {
    const sourceFragments = value.every((item) => typeof item === 'string')
      && value.some((item) => /\b(?:SOURCE|source)\s*=/.test(String(item)))
      && value.some((item) => /\b(?:resourceRef|RESOURCE_REF|providerCode|endpoint)\s*=/.test(String(item)));
    const rawItems = sourceFragments ? [value.map((item) => String(item)).join(', ')] : value;
    return rawItems
      .map((item): ProviderChainStepValue | undefined => {
        if (typeof item === 'string') return stepFromString(item);
        if (!item || typeof item !== 'object' || Array.isArray(item)) return undefined;
        const mapped = item as Record<string, unknown>;
        const source = stringProp(mapped, 'source', 'SOURCE', 'provider', 'PROVIDER') ?? '';
        if (!source.trim()) return undefined;
        return {
          source: source.trim().toUpperCase(),
          resourceRef: stringProp(mapped, 'resourceRef', 'RESOURCE_REF', 'resourceref'),
          namespace: stringProp(mapped, 'namespace', 'NAMESPACE'),
          sourcePath: stringProp(mapped, 'sourcePath', 'SOURCE_PATH', 'sourcepath'),
          cachePath: stringProp(mapped, 'cachePath', 'CACHE_PATH', 'cachepath'),
          keyTemplate: stringProp(mapped, 'keyTemplate', 'KEY_TEMPLATE', 'keytemplate'),
          valuePath: stringProp(mapped, 'valuePath', 'VALUE_PATH', 'valuepath'),
          endpoint: stringProp(mapped, 'endpoint', 'ENDPOINT'),
          method: stringProp(mapped, 'method', 'METHOD'),
          authRef: stringProp(mapped, 'authRef', 'AUTH_REF', 'authref'),
          providerCode: stringProp(mapped, 'providerCode', 'PROVIDER_CODE', 'providercode'),
          redisRef: stringProp(mapped, 'redisRef', 'REDIS_REF', 'redisref'),
          grpcTarget: stringProp(mapped, 'grpcTarget', 'GRPC_TARGET', 'grpctarget'),
          pluginCode: stringProp(mapped, 'pluginCode', 'PLUGIN_CODE', 'plugincode'),
          operation: stringProp(mapped, 'operation', 'OPERATION'),
          timeoutSeconds: typeof mapped.timeoutSeconds === 'string' || typeof mapped.timeoutSeconds === 'number'
            ? String(mapped.timeoutSeconds)
            : typeof mapped.TIMEOUT_SECONDS === 'string' || typeof mapped.TIMEOUT_SECONDS === 'number'
              ? String(mapped.TIMEOUT_SECONDS)
              : undefined,
          cacheTtlSeconds: typeof mapped.cacheTtlSeconds === 'string' || typeof mapped.cacheTtlSeconds === 'number'
            ? String(mapped.cacheTtlSeconds)
            : typeof mapped.CACHE_TTL_SECONDS === 'string' || typeof mapped.CACHE_TTL_SECONDS === 'number'
              ? String(mapped.CACHE_TTL_SECONDS)
              : undefined,
          onMiss: stringProp(mapped, 'onMiss', 'ON_MISS', 'onmiss') ?? 'NEXT',
        };
      })
      .filter((item): item is ProviderChainStepValue => Boolean(item));
  }
  if (typeof value === 'string' && value.trim()) {
    const rawSteps = /\b(?:SOURCE|source)\s*=/.test(value)
      ? [value]
      : parseArrayInput(value);
    return rawSteps.map(stepFromString).filter((item): item is ProviderChainStepValue => Boolean(item));
  }
  return [];
}

function nodeConfigRecord(node: ActivityNode): Record<string, unknown> {
  const config = node.data.config;
  return config && typeof config === 'object' && !Array.isArray(config) ? config : {};
}

function legacyProviderChainStep(node: ActivityNode): ProviderChainStepValue | undefined {
  if (node.data.id !== 'fact-query') return undefined;
  const config = nodeConfigRecord(node);
  const source = typeof config.source === 'string' && config.source.trim() ? config.source.trim().toUpperCase() : 'FACT_SERVICE';
  const hasLegacyProvider = [
    'source',
    'resourceRef',
    'providerCode',
    'endpoint',
    'redisRef',
    'grpcTarget',
    'pluginCode',
    'sourcePath',
    'cachePath',
    'keyTemplate',
    'valuePath',
  ].some((key) => typeof config[key] === 'string' && String(config[key]).trim());
  if (!hasLegacyProvider) return undefined;
  const step: ProviderChainStepValue = {
    source,
    namespace: typeof config.namespace === 'string' ? config.namespace : 'facts',
    operation: typeof config.operation === 'string' ? config.operation : source === 'HTTP_PROVIDER' ? 'CALL' : 'READ',
    onMiss: config.failureAction === 'BLOCK' ? 'BLOCK' : 'NEXT',
  };
  (['resourceRef', 'sourcePath', 'cachePath', 'keyTemplate', 'valuePath', 'endpoint', 'method', 'authRef', 'providerCode', 'redisRef', 'grpcTarget', 'pluginCode'] as const).forEach((key) => {
    const value = config[key];
    if (typeof value === 'string' && value.trim()) {
      step[key] = value.trim();
    }
  });
  if (typeof config.timeoutSeconds === 'string' || typeof config.timeoutSeconds === 'number') {
    step.timeoutSeconds = String(config.timeoutSeconds);
  }
  if (typeof config.cacheTtlSeconds === 'string' || typeof config.cacheTtlSeconds === 'number') {
    step.cacheTtlSeconds = String(config.cacheTtlSeconds);
  }
  return step;
}

function providerChainValueFor(node: ActivityNode, value: unknown): ProviderChainStepValue[] {
  const steps = normalizeProviderChain(value);
  if (steps.length) return steps;
  const legacy = legacyProviderChainStep(node);
  return legacy ? [legacy] : [];
}

function defaultFactServiceStepFor(node: ActivityNode): ProviderChainStepValue {
  const config = nodeConfigRecord(node);
  const factKeys = Array.isArray(config.factKeys) ? config.factKeys.map((item) => String(item).trim().toUpperCase()) : [];
  let resourceRef = 'default_user_profile_store';
  if (!factKeys.includes('USER_PROFILE')) {
    if (factKeys.includes('KYC_PROFILE')) resourceRef = 'default_kyc_profile_store';
    if (factKeys.includes('RISK_PROFILE')) resourceRef = 'default_risk_profile_store';
  }
  return {
    ...emptyProviderChainStep('FACT_SERVICE'),
    resourceRef,
    keyTemplate: '${tenantId}:${subject.type}:${subject.id}',
    onMiss: 'BLOCK',
  };
}

function compactProviderChain(steps: ProviderChainStepValue[]): ProviderChainStepValue[] {
  return steps.map((step) => {
    const next: ProviderChainStepValue = { source: step.source || 'FACT_SERVICE' };
    (['resourceRef', 'namespace', 'sourcePath', 'cachePath', 'keyTemplate', 'valuePath', 'endpoint', 'method', 'authRef', 'providerCode', 'redisRef', 'grpcTarget', 'pluginCode', 'operation', 'timeoutSeconds', 'cacheTtlSeconds', 'onMiss'] as const).forEach((key) => {
      const value = step[key];
      if (typeof value === 'string' && value.trim()) {
        next[key] = value.trim();
      }
    });
    if (!next.onMiss) next.onMiss = 'NEXT';
    return next;
  });
}

function ProviderChainEditor({
  node,
  value,
  resourceOptions,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  value: unknown;
  resourceOptions: ResourceSummary[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const steps = providerChainValueFor(node, value);
  const [collapsedSteps, setCollapsedSteps] = useState<Set<number>>(() => new Set(steps.map((_, index) => index).filter((index) => index > 0)));
  const [advancedSteps, setAdvancedSteps] = useState<Set<number>>(() => new Set());
  useEffect(() => {
    setCollapsedSteps(new Set(steps.map((_, index) => index).filter((index) => index > 0)));
    setAdvancedSteps(new Set());
  }, [node.id]);
  const update = (next: ProviderChainStepValue[]) => onNodeConfigChange(node.id, 'providerChain', compactProviderChain(next));
  const toggleCollapsedStep = (index: number) => {
    setCollapsedSteps((current) => {
      const next = new Set(current);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };
  const toggleAdvancedStep = (index: number) => {
    setAdvancedSteps((current) => {
      const next = new Set(current);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };
  const patchStep = (index: number, patch: Partial<ProviderChainStepValue>) => {
    update(steps.map((step, itemIndex) => {
      if (itemIndex !== index) return step;
      const next = { ...step, ...patch };
      if (patch.source) {
        next.source = patch.source.toUpperCase();
        if (!providerChainNeedsResource(next.source)) {
          next.resourceRef = '';
        }
        if (next.source === 'HTTP_PROVIDER') next.operation = 'CALL';
        if (next.source === 'LOCAL_CACHE' && !next.cachePath) next.cachePath = '$.vars.execution.facts.USER_PROFILE';
        if (next.source === 'WORKFLOW_CONTEXT' && !next.sourcePath) next.sourcePath = '$.attributes.USER_PROFILE';
        if (next.source === 'HTTP_PROVIDER' && !next.method) next.method = 'POST';
      }
      return next;
    }));
  };
  const moveStep = (index: number, direction: -1 | 1) => {
    const target = index + direction;
    if (target < 0 || target >= steps.length) return;
    const next = [...steps];
    [next[index], next[target]] = [next[target], next[index]];
    update(next);
  };
  return (
    <div className="provider-chain-editor">
      {steps.length ? steps.map((step, index) => {
        const collapsed = collapsedSteps.has(index);
        const advancedOpen = advancedSteps.has(index);
        const needsResource = providerChainNeedsResource(step.source);
        const requiresManagedResource = providerChainRequiresManagedResource(step.source);
        const hasTarget = providerChainHasTarget(step);
        const resourceExists = !step.resourceRef || resourceOptions.some((resource) => resource.code === step.resourceRef);
        const stepSummary = step.source === 'WORKFLOW_CONTEXT'
          ? step.sourcePath
          : step.source === 'LOCAL_CACHE'
            ? step.cachePath
            : step.source === 'HTTP_PROVIDER'
              ? step.endpoint
              : step.source === 'REDIS'
                ? step.redisRef
                : step.source === 'GRPC'
                  ? step.grpcTarget
                  : step.source === 'PLUGIN'
                    ? step.pluginCode
                    : step.providerCode || step.resourceRef;
        const targetPlaceholder = step.source === 'WORKFLOW_CONTEXT'
          ? '$.attributes.USER_PROFILE'
          : step.source === 'LOCAL_CACHE'
            ? '$.vars.execution.facts.USER_PROFILE'
            : step.source === 'HTTP_PROVIDER'
              ? 'https://user-service/internal/facts'
              : step.source === 'REDIS'
                ? 'marketing-fact-cache'
                : step.source === 'GRPC'
                  ? 'user-profile-service'
                  : step.source === 'PLUGIN'
                    ? 'vip-profile-provider'
                    : 'user_profile_provider';
        return (
          <div key={`${index}-${step.source}-${step.resourceRef ?? ''}`} className={cx('provider-chain-step', !hasTarget && 'missing-resource', !resourceExists && 'missing-resource')}>
            <div className="provider-chain-step-head">
              <b>#{index + 1}</b>
              <select value={step.source} onChange={(event) => patchStep(index, { source: event.target.value })}>
                {PROVIDER_CHAIN_SOURCES.map((source) => <option key={source} value={source}>{source}</option>)}
              </select>
              {collapsed && stepSummary ? <span className="provider-chain-step-summary">{stepSummary}</span> : null}
              <select value={step.onMiss ?? 'NEXT'} onChange={(event) => patchStep(index, { onMiss: event.target.value })}>
                <option value="NEXT">miss 后继续</option>
                <option value="BLOCK">miss 后阻断</option>
              </select>
              <div>
                <button type="button" title="上移" aria-label="上移" onClick={() => moveStep(index, -1)} disabled={index === 0}><ArrowUp /></button>
                <button type="button" title="下移" aria-label="下移" onClick={() => moveStep(index, 1)} disabled={index === steps.length - 1}><ArrowDown /></button>
                <button type="button" className="danger" title="删除" aria-label="删除" onClick={() => update(steps.filter((_, itemIndex) => itemIndex !== index))}><X /></button>
                <button type="button" title={collapsed ? '展开' : '收起'} aria-label={collapsed ? '展开' : '收起'} onClick={() => toggleCollapsedStep(index)}>{collapsed ? <ChevronDown /> : <ChevronUp />}</button>
              </div>
            </div>
            {!collapsed ? (
              <>
                {!hasTarget ? (
                  <div className="provider-chain-warning">
                    {requiresManagedResource ? '当前数据提供者必须选择资源 Ref。' : '需要配置 resourceRef、providerCode 或 endpoint。'}
                  </div>
                ) : null}
                <div className="provider-chain-fields core">
                  {needsResource ? (
                    <label>
                      <span>资源</span>
                      <select className={resourceExists ? '' : 'invalid'} value={step.resourceRef ?? ''} onChange={(event) => patchStep(index, { resourceRef: event.target.value })}>
                        <option value="">{requiresManagedResource ? '资源 Ref *' : '资源 Ref'}</option>
                        {step.resourceRef && !resourceExists ? <option value={step.resourceRef}>{step.resourceRef}（未加载或无权限）</option> : null}
                        {resourceOptions.map((resource) => (
                          <option key={resource.id || resource.code} value={resource.code}>{resource.name || resource.code} · {resource.code} · {resource.type}</option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                  {step.source === 'WORKFLOW_CONTEXT' ? (
                    <label>
                      <span>Path</span>
                      <input value={step.sourcePath ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { sourcePath: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'LOCAL_CACHE' ? (
                    <label>
                      <span>Path</span>
                      <input value={step.cachePath ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { cachePath: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'HTTP_PROVIDER' ? (
                    <label>
                      <span>Endpoint</span>
                      <input value={step.endpoint ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { endpoint: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'REDIS' ? (
                    <label>
                      <span>Redis</span>
                      <input value={step.redisRef ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { redisRef: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'GRPC' ? (
                    <label>
                      <span>gRPC</span>
                      <input value={step.grpcTarget ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { grpcTarget: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'PLUGIN' ? (
                    <label>
                      <span>Plugin</span>
                      <input value={step.pluginCode ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { pluginCode: event.target.value })} />
                    </label>
                  ) : null}
                  {step.source === 'CUSTOM' || step.source === 'FACT_SERVICE' ? (
                    <label>
                      <span>Provider</span>
                      <input value={step.providerCode ?? ''} placeholder={targetPlaceholder} onChange={(event) => patchStep(index, { providerCode: event.target.value })} />
                    </label>
                  ) : null}
                  {needsResource ? (
                    <label>
                      <span>Key</span>
                      <input value={step.keyTemplate ?? ''} placeholder="${tenantId}:${subject.type}:${subject.id}" onChange={(event) => patchStep(index, { keyTemplate: event.target.value })} />
                    </label>
                  ) : null}
                  {needsResource ? (
                    <label>
                      <span>Value Path</span>
                      <input value={step.valuePath ?? ''} placeholder="facts" onChange={(event) => patchStep(index, { valuePath: event.target.value })} />
                    </label>
                  ) : null}
                </div>
                <button type="button" className="provider-chain-advanced-toggle" onClick={() => toggleAdvancedStep(index)}>
                  <span>{advancedOpen ? '收起高级设置' : '高级设置'}</span>
                  {advancedOpen ? <ChevronDown /> : <ChevronRight />}
                </button>
                {advancedOpen ? (
                  <div className="provider-chain-fields advanced">
                    {needsResource ? (
                      <label>
                        <span>Namespace</span>
                        <input value={step.namespace ?? ''} placeholder="namespace" onChange={(event) => patchStep(index, { namespace: event.target.value })} />
                      </label>
                    ) : null}
                    {step.source === 'HTTP_PROVIDER' ? (
                      <label>
                        <span>Method</span>
                        <select value={step.method ?? 'POST'} onChange={(event) => patchStep(index, { method: event.target.value })}>
                          <option value="POST">POST</option>
                          <option value="GET">GET</option>
                        </select>
                      </label>
                    ) : null}
                    {needsResource ? (
                      <label>
                        <span>操作</span>
                        <select value={step.operation ?? 'READ'} onChange={(event) => patchStep(index, { operation: event.target.value })}>
                          <option value="READ">READ</option>
                          <option value="CALL">CALL</option>
                        </select>
                      </label>
                    ) : null}
                    <label>
                      <span>超时</span>
                      <input value={step.timeoutSeconds ?? ''} placeholder="3" onChange={(event) => patchStep(index, { timeoutSeconds: event.target.value })} />
                    </label>
                    <label>
                      <span>缓存</span>
                      <input value={step.cacheTtlSeconds ?? ''} placeholder="0" onChange={(event) => patchStep(index, { cacheTtlSeconds: event.target.value })} />
                    </label>
                    {step.source === 'HTTP_PROVIDER' ? (
                      <label>
                        <span>Auth Ref</span>
                        <input value={step.authRef ?? ''} placeholder="authRef" onChange={(event) => patchStep(index, { authRef: event.target.value })} />
                      </label>
                    ) : null}
                  </div>
                ) : null}
              </>
              ) : null}
          </div>
        );
      }) : <p className="property-empty">未配置事实来源。请添加上下文、缓存、Store、HTTP、Redis、gRPC、Plugin 或 Lua 来源。</p>}
      <div className="provider-chain-actions">
        <button type="button" onClick={() => update([...steps, defaultFactServiceStepFor(node)])}>+ 事实来源</button>
      </div>
    </div>
  );
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
  if (field.key === 'providerChain') {
    return <ProviderChainEditor node={node} value={value} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />;
  }
  if (field.key === 'qualifyRules') {
    return <QualifyRulesEditor node={node} value={value} onNodeConfigChange={onNodeConfigChange} />;
  }
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
    if (field.key === 'factKeys') {
      return (
        <input
          value={Array.isArray(value) ? value.join(', ') : stringifyConfigValue(value)}
          placeholder="USER_PROFILE"
          onChange={(event) => onNodeConfigChange(node.id, field.key, parseArrayInput(event.target.value))}
        />
      );
    }
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
  const complexEditor = field.key === 'providerChain' || field.key === 'qualifyRules';
  const content = (
    <>
      <span>
        <b>{field.label}{field.required ? <em className="required-star" aria-label="必填">*</em> : null}</b>
      </span>
      <ConfigFieldInput node={node} field={field} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />
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
    </>
  );
  const className = cx('schema-config-row', `field-${field.key}`, complexEditor && 'provider-chain-row', hasBlockIssue && 'has-error', !hasBlockIssue && hasWarningIssue && 'has-warning');
  if (complexEditor) {
    return <div className={className} data-config-field={field.key}>{content}</div>;
  }
  return (
    <label className={className} data-config-field={field.key}>
      {content}
    </label>
  );
}

type QualifyRuleDetail = {
  title: string;
  description: string;
  reads: string[];
  examples: string[];
  input: { name: string; path: string; type: string };
  defaultConditions: QualifyCondition[];
};

type QualifyCondition = {
  field: string;
  op: string;
  value: unknown;
};

type QualifyConditionRule = {
  ruleCode?: string;
  inputObject?: string;
  enabled?: boolean;
  logic?: string;
  conditions?: QualifyCondition[];
};

const QUALIFY_CONDITION_OPS = ['=', '!=', '>', '>=', '<', '<=', 'IN', 'NOT_IN'];
const DEFAULT_ELIGIBILITY_CONDITIONS: QualifyCondition[] = [
  { field: 'eventType', op: '=', value: 'DEPOSIT_SUCCESS' },
  { field: 'subject.id', op: '!=', value: '' },
  { field: 'facts.USER_PROFILE.exists', op: '=', value: true },
  { field: 'facts.USER_PROFILE.status', op: '=', value: 'ACTIVE' },
];

const QUALIFY_RULE_DETAILS: Record<string, QualifyRuleDetail> = {
  ELIGIBILITY: {
    title: '通用参与资格',
    description: '基础门禁：确认当前事件和主体是否允许进入活动资格链路。具体判断需要在活动 Rules 中配置 ruleCode=ELIGIBILITY、rulePhase=QUALIFY。',
    reads: ['eventType', 'attributes.subject.id', 'facts.USER_PROFILE.exists', 'facts.USER_PROFILE.status'],
    examples: ['eventType = DEPOSIT_SUCCESS', 'facts.USER_PROFILE.exists = true', 'facts.USER_PROFILE.status = ACTIVE'],
    input: { name: 'EVENT_CONTEXT', path: 'eventType / sourceSystem / subject', type: 'event' },
    defaultConditions: DEFAULT_ELIGIBILITY_CONDITIONS,
  },
  KYC: {
    title: 'KYC 状态',
    description: '检查用户实名、认证等级或合规状态。建议由 fact-query 先写入 facts.USER_PROFILE 或 facts.KYC_PROFILE。',
    reads: ['facts.USER_PROFILE.kycStatus', 'facts.KYC_PROFILE.level'],
    examples: ['facts.USER_PROFILE.kycStatus = VERIFIED', 'facts.KYC_PROFILE.level >= 1'],
    input: { name: 'KYC_PROFILE', path: 'facts.KYC_PROFILE', type: 'fact' },
    defaultConditions: [{ field: 'facts.KYC_PROFILE.status', op: '=', value: 'VERIFIED' }],
  },
  RISK_SCORE: {
    title: '风险评分',
    description: '检查风控分、黑名单、设备或账户风险，命中后通常走 RISK 或 BLOCK 分支。',
    reads: ['facts.RISK_PROFILE.score', 'facts.RISK_PROFILE.blacklisted'],
    examples: ['facts.RISK_PROFILE.score < 80', 'facts.RISK_PROFILE.blacklisted = false'],
    input: { name: 'RISK_PROFILE', path: 'facts.RISK_PROFILE', type: 'fact' },
    defaultConditions: [
      { field: 'facts.RISK_PROFILE.score', op: '<', value: 80 },
      { field: 'facts.RISK_PROFILE.blacklisted', op: '=', value: false },
    ],
  },
  VIP_LEVEL: {
    title: 'VIP / 用户等级',
    description: '检查用户等级是否达到活动门槛。用户等级事实应来自事实查询节点。',
    reads: ['facts.USER_PROFILE.vipLevel', 'facts.USER_PROFILE.level', 'attributes.vipLevel'],
    examples: ['facts.USER_PROFILE.vipLevel >= 3', 'facts.USER_PROFILE.level >= 1'],
    input: { name: 'USER_PROFILE', path: 'facts.USER_PROFILE', type: 'fact' },
    defaultConditions: [{ field: 'facts.USER_PROFILE.vipLevel', op: '>=', value: 1 }],
  },
  DEPOSIT_REQUIRED: {
    title: '充值要求',
    description: '检查首存、累计充值、近期充值等门槛，适合充值赠送、回流活动。',
    reads: ['facts.DEPOSIT_SUMMARY.firstDeposit', 'facts.DEPOSIT_SUMMARY.total7d', 'attributes.amount'],
    examples: ['facts.DEPOSIT_SUMMARY.firstDeposit = true', 'attributes.amount >= 100'],
    input: { name: 'DEPOSIT_SUMMARY', path: 'facts.DEPOSIT_SUMMARY', type: 'fact' },
    defaultConditions: [{ field: 'facts.DEPOSIT_SUMMARY.totalAmount', op: '>=', value: 100 }],
  },
  COUNTRY_ALLOWED: {
    title: '国家 / 地区范围',
    description: '检查当前用户或事件来源是否在活动允许地区内。',
    reads: ['facts.USER_PROFILE.country', 'attributes.country'],
    examples: ['facts.USER_PROFILE.country in [BR, MX]', 'attributes.country != RESTRICTED'],
    input: { name: 'USER_PROFILE', path: 'facts.USER_PROFILE', type: 'fact' },
    defaultConditions: [{ field: 'facts.USER_PROFILE.country', op: 'IN', value: ['BR', 'MX'] }],
  },
  CHANNEL_ALLOWED: {
    title: '渠道范围',
    description: '检查事件来源渠道、推广渠道或注册渠道是否允许参加。',
    reads: ['sourceSystem', 'attributes.channel', 'facts.USER_PROFILE.registerChannel'],
    examples: ['sourceSystem = app', 'attributes.channel in [organic, campaign]'],
    input: { name: 'EVENT_CONTEXT', path: 'sourceSystem / attributes.channel', type: 'event' },
    defaultConditions: [{ field: 'sourceSystem', op: 'IN', value: ['app', 'payment'] }],
  },
  REGISTER_AGE: {
    title: '注册时长',
    description: '检查用户注册天数、新老用户身份或回流条件。',
    reads: ['facts.USER_PROFILE.registerDays', 'facts.USER_PROFILE.createdAt'],
    examples: ['facts.USER_PROFILE.registerDays >= 7', 'facts.USER_PROFILE.registerDays <= 30'],
    input: { name: 'USER_PROFILE', path: 'facts.USER_PROFILE', type: 'fact' },
    defaultConditions: [{ field: 'facts.USER_PROFILE.registerDays', op: '>=', value: 7 }],
  },
};

function selectedQualifyRuleCodes(node: ActivityNode): string[] {
  const qualifyRules = node.data.config?.qualifyRules;
  if (Array.isArray(qualifyRules)) {
    const codes = qualifyRules
      .map((item) => {
        if (!item || typeof item !== 'object' || Array.isArray(item)) return '';
        return String((item as Record<string, unknown>).ruleCode || '').trim();
      })
      .filter(Boolean);
    if (codes.length) return codes;
  }
  const value = node.data.config?.ruleCodes;
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === 'string') {
    return parseArrayInput(value);
  }
  return ['ELIGIBILITY'];
}

function qualifyRuleDefaultConditions(ruleCode: string): QualifyCondition[] {
  const detail = QUALIFY_RULE_DETAILS[ruleCode.trim().toUpperCase()];
  const conditions = detail?.defaultConditions?.length ? detail.defaultConditions : [{ field: '', op: '=', value: '' }];
  return conditions.map((condition) => ({ ...condition }));
}

function qualifyRuleInputObject(ruleCode: string): string {
  return QUALIFY_RULE_DETAILS[ruleCode.trim().toUpperCase()]?.input.name ?? 'EVENT_CONTEXT';
}

function qualifyRuleConditionsArePlaceholder(conditions: QualifyCondition[]): boolean {
  return conditions.length === 0 || conditions.every((condition) => !condition.field.trim());
}

function qualifyRuleConditionsMatchDefault(ruleCode: string, conditions: QualifyCondition[]): boolean {
  const defaults = qualifyRuleDefaultConditions(ruleCode);
  return JSON.stringify(conditions) === JSON.stringify(defaults);
}

function parseConditionValueInput(raw: string, op: string): unknown {
  const trimmed = raw.trim();
  if (op === 'IN' || op === 'NOT_IN') {
    if (!trimmed) return [];
    return trimmed.split(',').map((item) => item.trim()).filter(Boolean);
  }
  if (trimmed === 'true') return true;
  if (trimmed === 'false') return false;
  if (trimmed === 'null') return null;
  return raw;
}

function formatConditionValueInput(value: unknown): string {
  if (Array.isArray(value)) return value.map((item) => String(item)).join(', ');
  if (value === true) return 'true';
  if (value === false) return 'false';
  if (value === null) return 'null';
  if (value === undefined) return '';
  return String(value);
}

function normalizeQualifyRule(value: unknown, fallbackCode = 'ELIGIBILITY'): Required<QualifyConditionRule> {
  const mapped = value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
  const ruleCode = typeof mapped.ruleCode === 'string' && mapped.ruleCode.trim() ? mapped.ruleCode.trim() : fallbackCode;
  const defaultConditions = qualifyRuleDefaultConditions(ruleCode);
  const conditions = Array.isArray(mapped.conditions)
    ? mapped.conditions
      .map((item): QualifyCondition | undefined => {
        if (!item || typeof item !== 'object' || Array.isArray(item)) return undefined;
        const condition = item as Record<string, unknown>;
        return {
          field: typeof condition.field === 'string' ? condition.field : '',
          op: typeof condition.op === 'string' ? condition.op : '=',
          value: condition.value,
        };
      })
      .filter((item): item is QualifyCondition => Boolean(item))
    : defaultConditions;
  return {
    ruleCode,
    inputObject: typeof mapped.inputObject === 'string' && mapped.inputObject.trim() ? mapped.inputObject.trim() : qualifyRuleInputObject(ruleCode),
    enabled: typeof mapped.enabled === 'boolean' ? mapped.enabled : true,
    logic: String(mapped.logic || 'AND').toUpperCase() === 'OR' ? 'OR' : 'AND',
    conditions: conditions.length ? conditions : defaultConditions,
  };
}

function normalizeQualifyRules(value: unknown, legacyValue: unknown, legacyRuleCodes?: unknown): Array<Required<QualifyConditionRule>> {
  if (Array.isArray(value)) {
    const rules = value
      .map((item, index) => normalizeQualifyRule(item, index === 0 ? 'ELIGIBILITY' : `RULE_${index + 1}`))
      .filter((item) => item.ruleCode);
    if (rules.length) return rules;
  }
  if (legacyValue && typeof legacyValue === 'object' && !Array.isArray(legacyValue)) {
    return [normalizeQualifyRule({ ...(legacyValue as Record<string, unknown>), ruleCode: 'ELIGIBILITY' })];
  }
  const legacyCodes = Array.isArray(legacyRuleCodes)
    ? legacyRuleCodes.map((item) => String(item).trim()).filter(Boolean)
    : typeof legacyRuleCodes === 'string'
      ? parseArrayInput(legacyRuleCodes)
      : [];
  if (legacyCodes.length) {
    return legacyCodes.map((code, index) => normalizeQualifyRule({
      ruleCode: code,
      enabled: true,
      logic: 'AND',
      conditions: qualifyRuleDefaultConditions(code),
    }, code));
  }
  return [normalizeQualifyRule({ ruleCode: 'ELIGIBILITY' })];
}

function compactQualifyRules(rules: Array<Required<QualifyConditionRule>>): QualifyConditionRule[] {
  return rules.map((rule) => ({
    ruleCode: rule.ruleCode.trim() || 'ELIGIBILITY',
    inputObject: rule.inputObject.trim() || qualifyRuleInputObject(rule.ruleCode),
    enabled: rule.enabled,
    logic: rule.logic,
    conditions: rule.conditions.map((condition) => ({
      field: condition.field,
      op: condition.op || '=',
      value: condition.value,
    })),
  }));
}

function stopNonFormDoubleClick(event: ReactMouseEvent<HTMLElement>) {
  const target = event.target as HTMLElement | null;
  if (target?.closest('input, select, textarea, button, option')) return;
  event.preventDefault();
  window.getSelection()?.removeAllRanges();
}

function QualifyRulesEditor({
  node,
  value,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  value: unknown;
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const rules = normalizeQualifyRules(value, node.data.config?.eligibilityRule, node.data.config?.ruleCodes);
  const [collapsedRules, setCollapsedRules] = useState<Set<number>>(() => new Set(rules.map((_, index) => index).filter((index) => index > 0)));
  useEffect(() => {
    setCollapsedRules(new Set(rules.map((_, index) => index).filter((index) => index > 0)));
  }, [node.id]);
  const updateRules = (nextRules: Array<Required<QualifyConditionRule>>) => {
    onNodeConfigChange(node.id, 'qualifyRules', compactQualifyRules(nextRules));
  };
  const toggleCollapsedRule = (index: number) => {
    setCollapsedRules((current) => {
      const next = new Set(current);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };
  const patchRule = (ruleIndex: number, patch: Partial<QualifyConditionRule>) => {
    updateRules(rules.map((rule, index) => (index === ruleIndex ? { ...rule, ...patch } : rule)));
  };
  const changeRuleCode = (ruleIndex: number, ruleCode: string) => {
    const current = rules[ruleIndex];
    patchRule(ruleIndex, {
      ruleCode,
      inputObject: qualifyRuleInputObject(ruleCode),
      conditions: qualifyRuleConditionsArePlaceholder(current.conditions)
        || qualifyRuleConditionsMatchDefault(current.ruleCode, current.conditions)
        ? qualifyRuleDefaultConditions(ruleCode)
        : current.conditions,
    });
  };
  const patchCondition = (ruleIndex: number, conditionIndex: number, patch: Partial<QualifyCondition>) => {
    patchRule(ruleIndex, {
      conditions: rules[ruleIndex].conditions.map((condition, index) => (
        index === conditionIndex ? { ...condition, ...patch } : condition
      )),
    });
  };
  const moveRule = (ruleIndex: number, direction: -1 | 1) => {
    const targetIndex = ruleIndex + direction;
    if (targetIndex < 0 || targetIndex >= rules.length) return;
    const next = [...rules];
    [next[ruleIndex], next[targetIndex]] = [next[targetIndex], next[ruleIndex]];
    updateRules(next);
  };
  const addRule = () => {
    updateRules([
      ...rules,
      normalizeQualifyRule({
        ruleCode: 'VIP_LEVEL',
        inputObject: qualifyRuleInputObject('VIP_LEVEL'),
        enabled: true,
        logic: 'AND',
        conditions: qualifyRuleDefaultConditions('VIP_LEVEL'),
      }, `RULE_${rules.length + 1}`),
    ]);
  };
  return (
    <div className="provider-chain-editor qualify-rule-chain-editor">
      {rules.map((rule, ruleIndex) => {
        const collapsed = collapsedRules.has(ruleIndex);
        const summary = `${rule.logic} · ${rule.conditions.length} 条条件${rule.enabled ? '' : ' · 已停用'}`;
        const ruleDetail = QUALIFY_RULE_DETAILS[rule.ruleCode];
        return (
          <div key={`${rule.ruleCode}-${ruleIndex}`} className={cx('provider-chain-step qualify-rule-chain-step', !rule.enabled && 'missing-resource')} onDoubleClick={stopNonFormDoubleClick}>
            <div className="provider-chain-step-head">
              <b>#{ruleIndex + 1}</b>
              <select value={rule.ruleCode} onChange={(event) => changeRuleCode(ruleIndex, event.target.value)}>
                {Object.keys(QUALIFY_RULE_DETAILS).map((code) => <option key={code} value={code}>{code}</option>)}
              </select>
              {collapsed ? <span className="provider-chain-step-summary">{summary}</span> : null}
              <select value={rule.enabled ? 'ENABLED' : 'DISABLED'} onChange={(event) => patchRule(ruleIndex, { enabled: event.target.value === 'ENABLED' })}>
                <option value="ENABLED">启用</option>
                <option value="DISABLED">停用</option>
              </select>
              <div>
                <button type="button" title="上移" aria-label="上移" disabled={ruleIndex === 0} onClick={() => moveRule(ruleIndex, -1)}><ArrowUp /></button>
                <button type="button" title="下移" aria-label="下移" disabled={ruleIndex === rules.length - 1} onClick={() => moveRule(ruleIndex, 1)}><ArrowDown /></button>
                <button type="button" className="danger" title="删除" aria-label="删除" disabled={rules.length <= 1} onClick={() => updateRules(rules.filter((_, index) => index !== ruleIndex))}><X /></button>
                <button type="button" title={collapsed ? '展开' : '收起'} aria-label={collapsed ? '展开' : '收起'} onClick={() => toggleCollapsedRule(ruleIndex)}>{collapsed ? <ChevronDown /> : <ChevronUp />}</button>
              </div>
            </div>
            {!collapsed ? (
              <>
                <div className="provider-chain-fields core qualify-rule-core-fields">
                  <label>
                    <span>条件逻辑</span>
                    <select value={rule.logic} onChange={(event) => patchRule(ruleIndex, { logic: event.target.value })}>
                      <option value="AND">AND</option>
                      <option value="OR">OR</option>
                    </select>
                  </label>
                </div>
                {ruleDetail?.input ? (
                  <div className="qualify-rule-inputs">
                    <span>输入对象</span>
                    <div>
                      <code>
                        <b>{ruleDetail.input.name}</b>
                        <em>{ruleDetail.input.path}</em>
                      </code>
                    </div>
                  </div>
                ) : null}
                <div className="eligibility-condition-list">
                  {rule.conditions.map((condition, conditionIndex) => (
                    <div key={`${condition.field}-${conditionIndex}`} className="eligibility-condition-row">
                      <input value={condition.field} placeholder="字段 Path" onChange={(event) => patchCondition(ruleIndex, conditionIndex, { field: event.target.value })} />
                      <select value={condition.op || '='} onChange={(event) => patchCondition(ruleIndex, conditionIndex, { op: event.target.value, value: parseConditionValueInput(formatConditionValueInput(condition.value), event.target.value) })}>
                        {QUALIFY_CONDITION_OPS.map((op) => <option key={op} value={op}>{op}</option>)}
                      </select>
                      <input
                        value={formatConditionValueInput(condition.value)}
                        placeholder={condition.op === 'IN' || condition.op === 'NOT_IN' ? 'A, B, C' : '值'}
                        onChange={(event) => patchCondition(ruleIndex, conditionIndex, { value: parseConditionValueInput(event.target.value, condition.op) })}
                      />
                      <button
                        type="button"
                        aria-label="删除条件"
                        disabled={rule.conditions.length <= 1}
                        onClick={() => patchRule(ruleIndex, { conditions: rule.conditions.filter((_, index) => index !== conditionIndex) })}
                      >
                        <X />
                      </button>
                    </div>
                  ))}
                </div>
                <button type="button" className="provider-chain-advanced-toggle" onClick={() => patchRule(ruleIndex, { conditions: [...rule.conditions, { field: '', op: '=', value: '' }] })}>
                  <span>添加条件</span>
                  <ChevronRight />
                </button>
              </>
            ) : null}
          </div>
        );
      })}
      <div className="provider-chain-actions">
        <button type="button" onClick={addRule}>+ 资格规则</button>
      </div>
    </div>
  );
}

function QualifyRuleDetails({ node }: { node: ActivityNode }) {
  const ruleCodes = selectedQualifyRuleCodes(node);
  return (
    <section className="qualify-rule-details">
      <div className="qualify-rule-details-head">
        <b>规则详情预览</b>
        <span>{ruleCodes.length} 个规则编码</span>
      </div>
      <p>这里展示当前规则链编码；属性面板里的规则链会按顺序转换为资格校验规则并执行。</p>
      <div className="qualify-rule-card-list">
        {ruleCodes.map((code) => {
          const detail = QUALIFY_RULE_DETAILS[code] ?? {
            title: code,
            description: '自定义资格规则。请确保活动 Rules 中存在同名 ruleCode，并且 rulePhase=QUALIFY。',
            reads: ['event attributes', 'facts'],
            examples: [`ruleCode = ${code}`],
          };
          return (
            <article key={code} className="qualify-rule-card">
              <header>
                <b>{code}</b>
                <span>{detail.title}</span>
              </header>
              <p>{detail.description}</p>
              <div>
                <span>建议读取</span>
                <div>{detail.reads.map((item) => <code key={item}>{item}</code>)}</div>
              </div>
              <div>
                <span>条件示例</span>
                <ul>
                  {detail.examples.map((item) => <li key={item}>{item}</li>)}
                </ul>
              </div>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function NodeConfigPreviewBody({ schema }: { schema: WorkflowNodeConfigSchema }) {
  const reads = schema.preview?.reads?.length
    ? schema.preview.reads
    : schema.inputs.map((input) => `${input.key}${input.required ? ' *' : ''}`);
  const writes = schema.preview?.writes?.length
    ? schema.preview.writes
    : schema.outputs.map((output) => output.key);
  return (
    <div className="node-config-preview">
      <div>
        <span>读取</span>
        <div>{reads.length ? reads.map((item) => <b key={item}>{item}</b>) : <em>无需输入</em>}</div>
      </div>
      <div>
        <span>输出</span>
        <div>{writes.length ? writes.map((item) => <b key={item}>{item}</b>) : <em>无输出</em>}</div>
      </div>
    </div>
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
  const groups = displayConfigGroupsForNode(node, schema);
  const defaultOpenGroupKeys = (items: typeof groups) => {
    if (node.data.id === 'fact-query') {
      return items
        .map((group, index) => ({ group, key: group.key || `group-${index}` }))
        .filter(({ group }) => group.fields.some((field) => field.key === 'providerChain'))
        .map(({ key }) => key);
    }
    return items.slice(0, 1).map((group, index) => group.key || `group-${index}`);
  };
  const issuesByField = nodeConfigIssuesByField(validationIssues ?? []);
  const nodeLevelIssues = (validationIssues ?? []).filter((issue) => !nodeConfigIssueFieldKey(issue));
  const [openGroups, setOpenGroups] = useState<Set<string>>(() => new Set(defaultOpenGroupKeys(groups)));
  useEffect(() => {
    setOpenGroups(new Set(defaultOpenGroupKeys(groups)));
  }, [node.id, schema?.nodeType]);
  const toggleGroup = (key: string) => {
    setOpenGroups((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };
  const focusConfigField = (fieldKey: string) => {
    const row = Array.from(document.querySelectorAll<HTMLElement>('.property-panel [data-config-field]'))
      .find((item) => item.dataset.configField === fieldKey);
    row?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    row?.querySelector<HTMLElement>('input, select, textarea, button')?.focus();
  };
  const updateConfigAndLua = (nodeId: string, key: string, value: unknown) => {
    onNodeConfigChange(nodeId, key, value);
    if (schema && nodeId === node.id) {
      syncLuaScriptFromConfigField(node, schema, key, value, onNodeConfigChange);
    }
  };
  if (!schema) return <p className="property-empty">未找到该节点类型的配置 schema。</p>;
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
      {groups.map((group, index) => {
        const groupKey = group.key || `group-${index}`;
        const groupTitle = configGroupDisplayTitle(group);
        const visual = configSectionVisual(groupTitle);
        return (
          <ConfigAccordionSection
            key={groupKey}
            title={groupTitle}
            summary={configGroupSummary(node, group)}
            icon={visual.icon}
            tone={visual.tone}
            className={node.data.id === 'qualify' && group.fields.some((field) => field.key === 'qualifyRules') ? 'qualify-rules-section' : undefined}
            open={openGroups.has(groupKey)}
            onToggle={() => toggleGroup(groupKey)}
          >
            <div className="schema-config-group">
              {group.description ? <p>{group.description}</p> : null}
              {group.fields.map((field) => (
                <ConfigFieldRow key={field.key} node={node} field={field} issues={issuesByField.get(field.key)} resourceOptions={resourceOptions} onNodeConfigChange={updateConfigAndLua} />
              ))}
            </div>
          </ConfigAccordionSection>
        );
      })}
    </div>
  );
}

function luaConfigVariables(schema?: WorkflowNodeConfigSchema): WorkflowNodeConfigField[] {
  if (!schema) return [];
  return schema.fields.filter((field) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(field.key));
}

function configValueForLua(node: ActivityNode, field: WorkflowNodeConfigField): unknown {
  if (field.key === 'providerChain') {
    return compactProviderChain(providerChainValueFor(node, node.data.config?.providerChain));
  }
  return configValue(node, field);
}

function luaLiteralForConfigValue(value: unknown): string {
  if (value === undefined || value === null || value === '') return 'nil';
  if (typeof value === 'string') return JSON.stringify(value);
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : 'nil';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (Array.isArray(value)) return `{ ${value.map((item) => luaLiteralForConfigValue(item)).join(', ')} }`;
  if (value && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>).map(([key, item]) => {
      const luaKey = /^[A-Za-z_][A-Za-z0-9_]*$/.test(key) ? key : `[${JSON.stringify(key)}]`;
      return `${luaKey} = ${luaLiteralForConfigValue(item)}`;
    });
    return `{ ${entries.join(', ')} }`;
  }
  return JSON.stringify(String(value));
}

function upsertLuaConfigAssignment(script: string, fieldKey: string, value: unknown): string {
  const nextLine = `${fieldKey} = ${luaLiteralForConfigValue(value)}`;
  const lines = script.split('\n');
  const matcher = new RegExp(`^(\\s*)${fieldKey}\\s*=`);
  const index = lines.findIndex((line) => matcher.test(line));
  if (index >= 0) {
    const indent = lines[index].match(/^(\s*)/)?.[1] ?? '';
    lines[index] = `${indent}${nextLine}`;
    return lines.join('\n');
  }
  const trimmed = script.trimEnd();
  return trimmed ? `${trimmed}\n${nextLine}` : nextLine;
}

function cleanLuaConfigScriptForSchema(script: string, schema?: WorkflowNodeConfigSchema): string {
  const allowedKeys = new Set(luaConfigVariables(schema).map((field) => field.key));
  if (!allowedKeys.size) return script;
  return script
    .split('\n')
    .filter((line) => {
      const match = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=/);
      return !match || allowedKeys.has(match[1]);
    })
    .join('\n')
    .trim();
}

function syncLuaScriptFromConfigField(
  node: ActivityNode,
  schema: WorkflowNodeConfigSchema | undefined,
  fieldKey: string,
  value: unknown,
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void,
) {
  const field = luaConfigVariables(schema).find((item) => item.key === fieldKey);
  if (!field) return;
  const script = cleanLuaConfigScriptForSchema(stringifyConfigValue(node.data.config?.configScript ?? ''), schema);
  onNodeConfigChange(node.id, 'configScript', upsertLuaConfigAssignment(script, field.key, value));
}

function luaConfigStarter(schema: WorkflowNodeConfigSchema | undefined, node?: ActivityNode): string {
  const fields = luaConfigVariables(schema).slice(0, 8);
  if (!fields.length) return '-- 当前节点没有可脚本配置的字段';
  return fields
    .map((field) => `${field.key} = ${luaLiteralForConfigValue(node ? configValueForLua(node, field) : field.default)}`)
    .join('\n');
}

function parseLuaStringLiteral(value: string): string | undefined {
  const trimmed = value.trim();
  if (trimmed.startsWith('"')) {
    try {
      return JSON.parse(trimmed);
    } catch {
      return undefined;
    }
  }
  if (trimmed.startsWith("'") && trimmed.endsWith("'")) {
    return trimmed.slice(1, -1).replace(/\\'/g, "'").replace(/\\\\/g, '\\');
  }
  return undefined;
}

function stripLuaLineComment(value: string): string {
  let quote: '"' | "'" | undefined;
  for (let index = 0; index < value.length - 1; index += 1) {
    const char = value[index];
    const next = value[index + 1];
    if ((char === '"' || char === "'") && value[index - 1] !== '\\') {
      quote = quote === char ? undefined : quote ?? char;
    }
    if (!quote && char === '-' && next === '-') {
      return value.slice(0, index).trim();
    }
  }
  return value.trim();
}

function parseLuaConfigLiteral(rawValue: string, field: WorkflowNodeConfigField): unknown {
  const value = stripLuaLineComment(rawValue);
  if (!value || value === 'nil') return undefined;
  if (value === 'true') return true;
  if (value === 'false') return false;
  const stringValue = parseLuaStringLiteral(value);
  if (stringValue !== undefined) return field.type === 'NUMBER' ? Number(stringValue) : stringValue;
  if (/^-?\d+(?:\.\d+)?$/.test(value)) return field.type === 'STRING' || field.type === 'ENUM' ? value : Number(value);
  if (field.type === 'ARRAY' && value.startsWith('{') && value.endsWith('}')) {
    const body = value.slice(1, -1).trim();
    if (!body) return [];
    return body
      .split(',')
      .map((item) => parseLuaStringLiteral(item.trim()) ?? item.trim())
      .filter(Boolean);
  }
  return undefined;
}

function syncConfigFieldsFromLuaScript(
  node: ActivityNode,
  schema: WorkflowNodeConfigSchema | undefined,
  script: string,
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void,
) {
  const fields = luaConfigVariables(schema);
  if (!fields.length) return;
  const fieldsByKey = new Map(fields.map((field) => [field.key, field]));
  script.split('\n').forEach((line) => {
    const match = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.+)$/);
    if (!match) return;
    const field = fieldsByKey.get(match[1]);
    if (!field) return;
    const parsed = parseLuaConfigLiteral(match[2], field);
    if (parsed !== undefined) {
      onNodeConfigChange(node.id, field.key, parsed);
    }
  });
}

function NodeConfigLuaEditor({
  node,
  schema,
  onNodeConfigChange,
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const [showHints, setShowHints] = useState(false);
  if (!schema) return null;
  const variables = luaConfigVariables(schema);
  const editorVariables: LuaConfigVariable[] = variables.map((field) => ({
    key: field.key,
    label: field.label,
    type: `${field.type}${field.required ? '，必填' : ''}`,
    description: field.description,
    options: field.options,
  }));
  const rawScript = stringifyConfigValue(node.data.config?.configScript ?? '');
  const normalizedScript = useMemo(() => {
    const cleaned = cleanLuaConfigScriptForSchema(rawScript, schema);
    if (rawScript && rawScript !== cleaned) {
      return luaConfigStarter(schema, node);
    }
    return cleaned || luaConfigStarter(schema, node);
  }, [node, rawScript, schema]);
  const script = normalizedScript;
  const enabled = script.trim().length > 0 || node.data.config?.configMode === 'LUA';
  useEffect(() => {
    if (rawScript && rawScript !== normalizedScript) {
      onNodeConfigChange(node.id, 'configScript', normalizedScript);
    }
  }, [node.id, normalizedScript, onNodeConfigChange, rawScript]);
  const updateScript = (value: string) => {
    onNodeConfigChange(node.id, 'configMode', value.trim() ? 'LUA' : undefined);
    onNodeConfigChange(node.id, 'configScript', value);
    syncConfigFieldsFromLuaScript(node, schema, value, onNodeConfigChange);
  };
  return (
    <PropertyDisclosure title="节点 Lua 配置" summary={enabled ? `${variables.length} 个变量` : '未启用'} defaultOpen>
      <div className="node-lua-config-panel">
        <div className="node-lua-config-toolbar">
          <button type="button" onClick={() => onNodeConfigChange(node.id, 'configMode', 'LUA')}>启用 Lua 配置</button>
          <button type="button" onClick={() => updateScript(luaConfigStarter(schema, node))}>插入默认变量</button>
          <button
            type="button"
            onClick={() => {
              onNodeConfigChange(node.id, 'configMode', undefined);
              onNodeConfigChange(node.id, 'configScript', undefined);
            }}
          >
            清空
          </button>
        </div>
        <Suspense fallback={<textarea className="node-lua-code-fallback" value={script} readOnly />}>
          <LazyLuaConfigCodeEditor
            value={script}
            variables={editorVariables}
            onHint={() => setShowHints(true)}
            onChange={updateScript}
          />
        </Suspense>
        {showHints ? (
          <div className="node-lua-variable-help">
            <div>
              <b>当前节点可配置变量</b>
              <span>脚本里直接给这些固定变量赋值，临时变量请使用 local。</span>
            </div>
            <div className="node-lua-variable-list">
              {variables.map((field) => (
                <button
                  key={field.key}
                  type="button"
                  onClick={() => updateScript(`${script}${script.trim() ? '\n' : ''}${field.key} = ${field.type === 'STRING' || field.type === 'ENUM' ? '""' : field.type === 'BOOLEAN' ? 'false' : field.type === 'NUMBER' ? '0' : '{}'}`
                  )}
                >
                  <code>{field.key}</code>
                  <span>{field.label}</span>
                  <em>{field.description}</em>
                </button>
              ))}
            </div>
            <div className="node-lua-api-hints">
              <code>Context.get("event.subject.id")</code>
              <code>Resource.default.get("key")</code>
              <code>ResourceRef("resource").get("key")</code>
              <code>Node.get("nodeId", "output", "$.path")</code>
            </div>
          </div>
        ) : null}
      </div>
    </PropertyDisclosure>
  );
}

function requiredConfigCount(schema?: WorkflowNodeConfigSchema): number {
  if (!schema) return 0;
  return schema.fields.filter((field) => field.required).length;
}

function nodeInfoMeta(node: ActivityNode, schema?: WorkflowNodeConfigSchema): Array<{ label: string; value: string }> {
  const inputCount = schema?.inputs?.length ?? node.data.inputs?.length ?? 0;
  const outputCount = schema?.outputs?.length ?? node.data.outputs?.length ?? 0;
  return [
    { label: 'ID', value: node.id },
    { label: '类型', value: `${node.data.code} / ${node.data.kind}` },
    { label: '坐标', value: `x ${Math.round(node.position.x)} / y ${Math.round(node.position.y)}` },
    { label: 'I/O', value: `${inputCount} 入 / ${outputCount} 出` },
    { label: '配置', value: schema ? `v${schema.version} · ${requiredConfigCount(schema)} 必填` : '无 schema' },
  ];
}

function NodeInfoSummary({
  node,
  schema,
  onNodeMetaChange,
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  onNodeMetaChange?: (nodeId: string, patch: Partial<ActivityNode['data']>) => void;
}) {
  const meta = nodeInfoMeta(node, schema);
  return (
    <div className="property-node-summary">
      <label className="property-node-title-editor">
        <span>节点名称</span>
        <input
          value={node.data.title}
          onChange={(event) => onNodeMetaChange?.(node.id, { title: event.target.value })}
          onBlur={(event) => {
            const nextTitle = event.target.value.trim();
            if (!nextTitle) onNodeMetaChange?.(node.id, { title: schema?.title || node.data.code || node.id });
            else if (nextTitle !== event.target.value) onNodeMetaChange?.(node.id, { title: nextTitle });
          }}
        />
      </label>
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
            <b>Route Key{route.type === 'OUTPUT' || routeNeedsCondition ? <em className="required-star" aria-label="必填">（*）</em> : null}</b>
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
  onNodeMetaChange?: (nodeId: string, patch: Partial<ActivityNode['data']>) => void;
  onEdgeRouteChange: (edgeId: string, route: WorkflowEdgeRoute) => void;
  onLocateNode?: (nodeId: string) => void;
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (ids: string[]) => void;
  panelWidth: number;
  onResizeStart: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  resourceOptions: ResourceSummary[];
  validationIssues?: WorkflowValidationIssue[];
  edgeValidationIssues?: WorkflowValidationIssue[];
  debugContent?: ReactNode;
  debugOpen?: boolean;
};

export function PropertyPanel({ nodes, edges, selectedNodeIds, selectedEdgeIds, nodeConfigSchemas, onNodeConfigChange, onNodeMetaChange, onEdgeRouteChange, onLocateNode, onDeleteNodes, onDeleteEdges, panelWidth, onResizeStart, resourceOptions, validationIssues = [], edgeValidationIssues = [], debugContent, debugOpen = false }: Props) {
  const [activeTab, setActiveTab] = useState<PropertyPanelTab>(loadPropertyPanelTab);
  const [configPreviewOpen, setConfigPreviewOpen] = useState(false);
  const [configInputOpen, setConfigInputOpen] = useState(false);
  const [configOutputOpen, setConfigOutputOpen] = useState(false);
  const [otherRetryOpen, setOtherRetryOpen] = useState(true);
  const [otherExceptionOpen, setOtherExceptionOpen] = useState(false);
  const [runOverviewOpen, setRunOverviewOpen] = useState(true);
  const [runConfigOpen, setRunConfigOpen] = useState(false);
  const [runContextOpen, setRunContextOpen] = useState(false);
  const selectedNode = nodes.find((node): node is ActivityNode => node.id === selectedNodeIds[0] && node.type === 'activity');
  const selectedGroup = nodes.find((node): node is GroupNode => node.id === selectedNodeIds[0] && node.type === 'group');
  const selectedNote = nodes.find((node): node is NoteNode => node.id === selectedNodeIds[0] && node.type === 'note');
  const selectedEdge = edges.find((item) => item.id === selectedEdgeIds[0]);
  const selectedEdgeRoute = selectedEdge ? edgeRouteFor(selectedEdge) : undefined;
  const selectedNodeSchema = selectedNode ? nodeConfigSchemas.find((schema) => schema.nodeType === selectedNode.data.id) : undefined;
  const hasSelection = Boolean(selectedNode || selectedGroup || selectedNote || selectedEdge);
  useEffect(() => {
    setConfigPreviewOpen(false);
    setConfigInputOpen(false);
    setConfigOutputOpen(false);
    setOtherRetryOpen(true);
    setOtherExceptionOpen(false);
    setRunOverviewOpen(true);
    setRunConfigOpen(false);
    setRunContextOpen(false);
  }, [selectedNode?.id]);
  useEffect(() => {
    if (debugOpen && selectedNode) {
      setActiveTab('debug');
      return;
    }
    if (activeTab === 'debug' && (!debugOpen || !selectedNode)) {
      setActiveTab('basic');
    }
  }, [activeTab, debugOpen, selectedNode?.id]);
  const changeActiveTab = (tab: PropertyPanelTab) => {
    setActiveTab(tab);
    setConfigPreviewOpen(false);
    window.localStorage.setItem(PROPERTY_PANEL_TAB_STORAGE_KEY, tab);
  };
  return (
    <aside className={cx('property-panel', hasSelection && 'open')} style={{ width: panelWidth }}>
      <button className="property-resize-handle right-panel-resize-handle" aria-label="调整属性面板宽度" onPointerDown={onResizeStart} />
      <div className="property-head">
        <div className="property-title-with-icon">
          {selectedNode ? <NodeKindIcon node={selectedNode.data} className="node-icon" /> : null}
          <span>
            <b>{selectedNode ? selectedNode.data.title : selectedGroup ? selectedGroup.data.title : selectedNote ? '备注' : selectedEdge ? '流程连线' : '属性'}</b>
          </span>
        </div>
        {selectedNode ? (
          <button type="button" className="property-preview-head-button" disabled={!selectedNodeSchema} onClick={() => {
            setActiveTab('config');
            setConfigPreviewOpen((open) => !open);
          }}>
            <Eye size={17} />
            <span>预览</span>
          </button>
        ) : (
          <span>{selectedGroup ? '组合' : selectedNote ? '备注' : selectedEdge ? '连线' : '未选择'}</span>
        )}
      </div>
      {selectedNode ? (
        <div className="property-tabs">
          <div className={cx('property-tab-buttons', Boolean(debugContent) && 'has-debug-tab')}>
            <button className={activeTab === 'basic' ? 'active' : ''} onClick={() => changeActiveTab('basic')}>基础</button>
            <button className={activeTab === 'config' ? 'active' : ''} onClick={() => changeActiveTab('config')}>配置</button>
            <button className={activeTab === 'lua' ? 'active' : ''} onClick={() => changeActiveTab('lua')}>Lua脚本</button>
            <button className={activeTab === 'other' ? 'active' : ''} onClick={() => changeActiveTab('other')}>其他</button>
            <button className={activeTab === 'run' ? 'active' : ''} onClick={() => changeActiveTab('run')}>运行日志</button>
            {debugContent ? <button className={activeTab === 'debug' ? 'active' : ''} onClick={() => changeActiveTab('debug')}>调试</button> : null}
          </div>
        </div>
      ) : null}
      {selectedNode && activeTab === 'basic' ? (
        <div className="property-body config-body basic-body">
          <NodeInfoSummary node={selectedNode} schema={selectedNodeSchema} onNodeMetaChange={onNodeMetaChange} />
        </div>
      ) : null}
      {selectedNode && activeTab === 'config' ? (
        <div className="property-body config-body">
          {configPreviewOpen && selectedNodeSchema ? (
            <div className="config-preview-popover" role="dialog" aria-label="配置预览">
              <div>
                <b>配置预览</b>
                <button type="button" aria-label="关闭配置预览" onClick={() => setConfigPreviewOpen(false)}>×</button>
              </div>
              <NodeConfigPreviewBody schema={selectedNodeSchema} />
            </div>
          ) : null}
          <NodeConfigForm node={selectedNode} schema={selectedNodeSchema} validationIssues={validationIssues} resourceOptions={resourceOptions} onNodeConfigChange={onNodeConfigChange} />
          <ConfigAccordionSection
            title="输入"
            summary={nodeInputSummary(selectedNode, selectedNodeSchema)}
            icon={configSectionVisual('输入').icon}
            tone="green"
            open={configInputOpen}
            onToggle={() => setConfigInputOpen((open) => !open)}
          >
            <InputMappingEditor node={selectedNode} schema={selectedNodeSchema} nodes={nodes} edges={edges} schemas={nodeConfigSchemas} onNodeConfigChange={onNodeConfigChange} onLocateNode={onLocateNode} direct />
          </ConfigAccordionSection>
          <ConfigAccordionSection
            title="输出"
            summary={nodeOutputSummary(selectedNodeSchema)}
            icon={configSectionVisual('输出').icon}
            tone="amber"
            open={configOutputOpen}
            onToggle={() => setConfigOutputOpen((open) => !open)}
          >
            <OutputRouteSummary node={selectedNode} schema={selectedNodeSchema} />
          </ConfigAccordionSection>
        </div>
      ) : null}
      {selectedNode && activeTab === 'other' ? (
        <div className="property-body config-body other-body">
          <ConfigAccordionSection
            title="失败时重试"
            summary={selectedNode.data.kind === 'reliable' || selectedNode.data.kind === 'dispatch' ? '启用' : '关闭'}
            open={otherRetryOpen}
            onToggle={() => setOtherRetryOpen((open) => !open)}
          >
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
          </ConfigAccordionSection>
          <ConfigAccordionSection
            title="异常处理"
            summary="无"
            open={otherExceptionOpen}
            onToggle={() => setOtherExceptionOpen((open) => !open)}
          >
            <button className="exception-select">无</button>
          </ConfigAccordionSection>
        </div>
      ) : null}
      {selectedNode && activeTab === 'lua' ? (
        <div className="property-body">
          <NodeConfigLuaEditor node={selectedNode} schema={selectedNodeSchema} onNodeConfigChange={onNodeConfigChange} />
          <LuaScriptEditor node={selectedNode} onNodeConfigChange={onNodeConfigChange} />
        </div>
      ) : null}
      {selectedNode && activeTab === 'run' ? (
        <div className="property-body config-body run-body">
          <ConfigAccordionSection
            title="运行概览"
            summary={selectedNode.data.lastRun?.status ?? 'NONE'}
            open={runOverviewOpen}
            onToggle={() => setRunOverviewOpen((open) => !open)}
          >
            <NodeRunInspector node={selectedNode} />
          </ConfigAccordionSection>
          <ConfigAccordionSection
            title="节点配置快照"
            summary={selectedNode.data.id}
            open={runConfigOpen}
            onToggle={() => setRunConfigOpen((open) => !open)}
          >
            <CompactJsonViewer title="config" value={selectedNode.data.config ?? {}} />
          </ConfigAccordionSection>
          <ConfigAccordionSection
            title="流程上下文说明"
            summary="ctx"
            open={runContextOpen}
            onToggle={() => setRunContextOpen((open) => !open)}
          >
            <div className="lua-context-help">
              <b>调试时可关注这些上下文</b>
              <code>ctx.event 当前事件</code>
              <code>ctx.node.input 当前节点输入</code>
              <code>ctx.node.config 当前节点配置</code>
              <code>ctx.nodes 上游节点输出</code>
              <code>ctx.vars 流程公共参数</code>
              <span>Lua 节点可以在设置页编辑脚本，并通过 emit 输出到当前节点 output。</span>
            </div>
          </ConfigAccordionSection>
        </div>
      ) : null}
      {selectedNode && activeTab === 'debug' && debugContent ? (
        <div className="property-body config-body property-debug-body">
          {debugContent}
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
