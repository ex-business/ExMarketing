import { useCallback, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';
import type { ActivityEdge, ActivityNode, FlowNode, GroupNode, NoteNode, WorkflowNodeConfigField, WorkflowNodeConfigSchema, WorkflowNodeConfigValidationRule } from '../types';
import { cx } from '../utils/flow';
import { NodeKindIcon } from './NodeKindIcon';

function PropertyList({ title, items }: { title: string; items: string[] }) {
  return (
    <section className="property-section">
      <h3>{title}</h3>
      <div className="property-tags">
        {items.length > 0 ? items.map((item) => <span key={item}>{item}</span>) : <em>未配置</em>}
      </div>
    </section>
  );
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
  output: string;
  label: string;
  type?: string;
};

type UpstreamOutputGroup = {
  nodeId: string;
  nodeTitle: string;
  options: UpstreamOutputOption[];
};

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

function upstreamOutputOptions(node: ActivityNode, nodes: FlowNode[], edges: ActivityEdge[], schemas: WorkflowNodeConfigSchema[]): UpstreamOutputOption[] {
  const schemaByType = new Map(schemas.map((schema) => [schema.nodeType, schema]));
  return edges
    .filter((edge) => edge.target === node.id)
    .flatMap((edge) => {
      const sourceNode = nodes.find((item): item is ActivityNode => item.id === edge.source && item.type === 'activity');
      if (!sourceNode) return [];
      const schemaOutputs = schemaByType.get(sourceNode.data.id)?.outputs ?? [];
      const outputs = schemaOutputs.length > 0
        ? schemaOutputs
        : sourceNode.data.outputs.map((output) => ({ key: output, label: output, type: undefined, description: undefined }));
      return outputs.map((output) => ({
        value: `${sourceNode.id}::${output.key}`,
        nodeId: sourceNode.id,
        nodeTitle: sourceNode.data.title,
        output: output.key,
        label: `${sourceNode.data.title} / ${output.label || output.key}`,
        type: output.type,
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
}: {
  node: ActivityNode;
  schema?: WorkflowNodeConfigSchema;
  nodes: FlowNode[];
  edges: ActivityEdge[];
  schemas: WorkflowNodeConfigSchema[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const options = upstreamOutputOptions(node, nodes, edges, schemas);
  const optionGroups = upstreamOutputGroups(options);
  const mapping = inputMappingFor(node);
  const hasMappedInputs = Object.keys(mapping).length > 0;
  const inputs = schema?.inputs?.length
    ? schema.inputs
    : node.data.inputs.map((input): NonNullable<WorkflowNodeConfigSchema['inputs']>[number] => ({ key: input, label: input, type: undefined, required: false, description: undefined }));
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
  return (
    <section className="property-section">
      <h3>输入映射</h3>
      {inputs.length === 0 ? (
        <p className="property-empty">该节点没有输入变量。</p>
      ) : options.length === 0 && !hasMappedInputs ? (
        <p className="property-empty">先连接上游节点，再从上游输出中选择输入来源。</p>
      ) : (
        <div className="input-map-grid">
          {inputs.map((input) => {
            const selected = mapping[input.key];
            const selectedValue = selected ? `${selected.sourceNodeId}::${selected.outputKey}` : '';
            const selectedOption = selectedOptionFor(input.key);
            const invalidReference = Boolean(selected && !selectedOption);
            return (
              <div key={input.key} className={cx('input-map-row', invalidReference && 'invalid')}>
                <div className="input-map-name">
                  <span>
                    <b>{input.label || input.key}</b>
                    {input.required ? <em>必填</em> : null}
                  </span>
                  {selectedOption ? (
                    <strong>{selectedOption.nodeTitle} / {selectedOption.output}</strong>
                  ) : selected ? (
                    <strong>引用已失效</strong>
                  ) : (
                    <strong>未选择</strong>
                  )}
                </div>
                {invalidReference ? (
                  <p className="input-map-invalid">
                    上游输出 {selected.sourceNodeId} / {selected.outputKey} 不可用，请重新选择。
                  </p>
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
                              <em>{option.label.replace(`${option.nodeTitle} / `, '')}</em>
                              {option.type ? <i>{option.type}</i> : null}
                            </button>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="property-empty">当前没有可用的上游输出。</p>
                )}
                {selected ? (
                  <div className="input-map-ref-tools">
                    <label>
                      <span>Path</span>
                      <input
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
                {input.description ? <small>{input.description}</small> : null}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

function configValue(node: ActivityNode, field: WorkflowNodeConfigField): unknown {
  if (node.data.config && Object.prototype.hasOwnProperty.call(node.data.config, field.key)) {
    return node.data.config[field.key];
  }
  return field.default ?? '';
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
  onNodeConfigChange,
}: {
  node: ActivityNode;
  field: WorkflowNodeConfigField;
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
}) {
  const value = configValue(node, field);
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

type Props = {
  nodes: FlowNode[];
  edges: ActivityEdge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  nodeConfigSchemas: WorkflowNodeConfigSchema[];
  onNodeConfigChange: (nodeId: string, key: string, value: unknown) => void;
  onDeleteNodes: (ids: string[]) => void;
  onDeleteEdges: (ids: string[]) => void;
};

export function PropertyPanel({ nodes, edges, selectedNodeIds, selectedEdgeIds, nodeConfigSchemas, onNodeConfigChange, onDeleteNodes, onDeleteEdges }: Props) {
  const [panelWidth, setPanelWidth] = useState(390);
  const selectedNode = nodes.find((node): node is ActivityNode => node.id === selectedNodeIds[0] && node.type === 'activity');
  const selectedGroup = nodes.find((node): node is GroupNode => node.id === selectedNodeIds[0] && node.type === 'group');
  const selectedNote = nodes.find((node): node is NoteNode => node.id === selectedNodeIds[0] && node.type === 'note');
  const selectedEdge = edges.find((item) => item.id === selectedEdgeIds[0]);
  const selectedNodeSchema = selectedNode ? nodeConfigSchemas.find((schema) => schema.nodeType === selectedNode.data.id) : undefined;
  const hasSelection = Boolean(selectedNode || selectedGroup || selectedNote || selectedEdge);
  const startResize = useCallback((event: ReactPointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    const updateWidth = (moveEvent: PointerEvent) => {
      const nextWidth = Math.min(560, Math.max(320, window.innerWidth - moveEvent.clientX));
      setPanelWidth(nextWidth);
    };
    const stopResize = () => {
      document.body.classList.remove('property-panel-resizing');
      window.removeEventListener('pointermove', updateWidth);
      window.removeEventListener('pointerup', stopResize);
    };
    document.body.classList.add('property-panel-resizing');
    window.addEventListener('pointermove', updateWidth);
    window.addEventListener('pointerup', stopResize, { once: true });
  }, []);

  return (
    <aside className={cx('property-panel', hasSelection && 'open')} style={{ width: panelWidth }}>
      <button className="property-resize-handle" aria-label="调整属性面板宽度" onPointerDown={startResize} />
      <div className="property-head">
        <div className="property-title-with-icon">
          {selectedNode ? <NodeKindIcon node={selectedNode.data} className="node-icon" /> : null}
          <b>{selectedNode ? selectedNode.data.title : selectedGroup ? selectedGroup.data.title : selectedNote ? '备注' : selectedEdge ? '流程连线' : '属性'}</b>
        </div>
        <span>{selectedNode ? '节点' : selectedGroup ? '组合' : selectedNote ? '备注' : selectedEdge ? '连线' : '未选择'}</span>
      </div>
      {selectedNode ? (
        <div className="property-tabs">
          <button className="active">设置</button>
          <button>上次运行</button>
        </div>
      ) : null}
      {selectedNode ? (
        <div className="property-body">
          <section className="property-card">
            <div className="property-title-row">
              <NodeKindIcon node={selectedNode.data} className="palette-dot" />
              <div>
                <h2>{selectedNode.data.title}</h2>
                <p>{selectedNode.data.code}</p>
              </div>
            </div>
            <dl>
              <div><dt>节点 ID</dt><dd>{selectedNode.id}</dd></div>
              <div><dt>节点类型</dt><dd>{selectedNode.data.kind}</dd></div>
              <div><dt>位置</dt><dd>x {Math.round(selectedNode.position.x)} / y {Math.round(selectedNode.position.y)}</dd></div>
            </dl>
          </section>
          <section className="property-section">
            <h3>说明</h3>
            <p>{selectedNode.data.desc}</p>
          </section>
          <InputMappingEditor node={selectedNode} schema={selectedNodeSchema} nodes={nodes} edges={edges} schemas={nodeConfigSchemas} onNodeConfigChange={onNodeConfigChange} />
          <PropertyList title="输出变量" items={(selectedNodeSchema?.outputs ?? []).map((output) => output.key)} />
          <section className="property-section">
            <h3>下一步</h3>
            <div className="next-step-card">
              <NodeKindIcon node={selectedNode.data} className="palette-dot" />
              <div>
                <b>{selectedNode.data.outputs[0] ?? '默认路径'}</b>
                <p>连接下游活动节点，决定下一步执行链路。</p>
              </div>
            </div>
          </section>
          <section className="property-section">
            <h3>配置参数</h3>
            {selectedNodeSchema ? (
              <div className="schema-config-grid">
                <div className="schema-config-head">
                  <b>{selectedNodeSchema.title}</b>
                  <span>v{selectedNodeSchema.version}</span>
                </div>
                {selectedNodeSchema.description ? <p className="schema-config-desc">{selectedNodeSchema.description}</p> : null}
                {selectedNodeSchema.fields.map((field) => (
                  <label key={field.key} className="schema-config-row">
                    <span>
                      <b>{field.label}</b>
                      {field.required ? <em>必填</em> : null}
                    </span>
                    <ConfigFieldInput node={selectedNode} field={field} onNodeConfigChange={onNodeConfigChange} />
                    {field.description ? <small>{field.description}</small> : null}
                    {field.validationRules?.length ? (
                      <div className="schema-rule-list">
                        {field.validationRules.map((rule) => <i key={`${field.key}-${rule.type}-${stringifyConfigValue(rule.value)}`}>{ruleLabel(rule)}</i>)}
                      </div>
                    ) : null}
                  </label>
                ))}
              </div>
            ) : (
              <p className="property-empty">未找到该节点类型的配置 schema。</p>
            )}
          </section>
          <section className="property-section">
            <h3>失败时重试</h3>
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
          </section>
          <section className="property-section">
            <h3>异常处理</h3>
            <button className="exception-select">无</button>
          </section>
          <section className="property-section">
            <h3>上次运行</h3>
            {selectedNode.data.lastRun ? (
              <div className={cx('run-summary', `run-${selectedNode.data.lastRun.status.toLowerCase()}`)}>
                <b>{selectedNode.data.lastRun.status}</b>
                <span>{selectedNode.data.lastRun.finishedAt ? new Date(selectedNode.data.lastRun.finishedAt).toLocaleString() : '运行中'}</span>
                <span>{selectedNode.data.lastRun.errorMessage || `输出 ${selectedNode.data.outputs[0] ?? 'result'}`}</span>
              </div>
            ) : (
              <div className="run-summary empty">
                <b>NONE</b>
                <span>暂无运行记录</span>
                <span>触发活动事件后刷新运行态</span>
              </div>
            )}
          </section>
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
            </dl>
            <button className="property-danger" onClick={() => onDeleteEdges([selectedEdge.id])}>删除连线</button>
          </section>
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
