import React, { useCallback, useMemo, useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  addEdge,
  Background,
  Controls,
  Handle,
  MarkerType,
  MiniMap,
  Position,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
} from '@xyflow/react';
import type { Connection, Edge, Node, NodeProps, ReactFlowInstance } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import './styles.css';

type NodeKind =
  | 'entry'
  | 'store'
  | 'transform'
  | 'logic'
  | 'query'
  | 'reward'
  | 'quota'
  | 'grant'
  | 'dispatch'
  | 'callback'
  | 'reliable'
  | 'audit';

type ActivityNodeData = {
  id: string;
  code: string;
  title: string;
  kind: NodeKind;
  desc: string;
  inputs: string[];
  outputs: string[];
  onDelete?: (nodeId: string) => void;
};

type GroupNodeData = {
  title: string;
  width: number;
  height: number;
  onDelete?: (nodeId: string) => void;
};

type NoteNodeData = {
  text: string;
  onDelete?: (nodeId: string) => void;
};

type ActivityNode = Node<ActivityNodeData, 'activity'>;
type GroupNode = Node<GroupNodeData, 'group'>;
type NoteNode = Node<NoteNodeData, 'note'>;
type FlowNode = ActivityNode | GroupNode | NoteNode;
type ActivityEdge = Edge;
type CatalogGroup = {
  category: string;
  items: ActivityNodeData[];
};
type FlowContextMenu =
  | { type: 'pane'; x: number; y: number; flowX: number; flowY: number }
  | { type: 'node'; x: number; y: number; nodeId: string }
  | { type: 'edge'; x: number; y: number; edgeId: string };
type NodePickerState = {
  x: number;
  y: number;
  flowX: number;
  flowY: number;
};
type AlignmentGuide = {
  vertical?: number;
  horizontal?: number;
};

const nodeCatalog: CatalogGroup[] = [
  {
    category: '入口',
    items: [
      nodeDef('event-in', 'Event In', '事件入口', 'entry', 'HTTP / MQ / Webhook 标准事件接入', ['eventId', 'eventType', 'tenantId', 'subject', 'attributes'], ['raw_event']),
      nodeDef('inbox', 'Inbox', '幂等收件箱', 'store', '写入 marketing_event_inbox，重复事件直接返回已有结果', ['raw_event', 'Idempotency-Key'], ['event_context']),
      nodeDef('normalize', 'Normalize', '事件标准化', 'transform', '字段映射、默认值、时间格式、租户和业务主体规范化', ['raw_event'], ['normalized_event']),
    ],
  },
  {
    category: '判断与规则',
    items: [
      nodeDef('match', 'MATCH', '活动匹配', 'logic', '按活动状态、时间窗、事件类型、来源系统匹配活动', ['event_context', 'activity_config'], ['matched_activity']),
      nodeDef('condition', 'CONDITION', '条件判断', 'logic', '字段路径读取、类型转换、缺失字段处理和错误原因输出', ['fact', 'condition_expr'], ['condition_result']),
      nodeDef('qualify', 'QUALIFY', '资格校验', 'logic', '用户资格、活动门槛、参与范围、黑白名单和风险规则校验', ['event_context', 'matched_activity'], ['qualified_context']),
      nodeDef('switch', 'SWITCH', '条件分支', 'logic', '基于阶段结果选择后续路径，支持 PASS / BLOCK / SKIP', ['stage_result'], ['branch']),
      nodeDef('fact-query', 'Fact Query', '事实查询', 'query', '查询用户、订单、交易、历史参与和外部事实数据', ['subject', 'fact_keys'], ['facts']),
    ],
  },
  {
    category: '奖励与限制',
    items: [
      nodeDef('reward', 'REWARD', '奖励计算', 'reward', '支持 FIXED / RATIO / LADDER / RANDOM / PROBABILITY / POOL_SPLIT', ['qualified_context', 'reward_config'], ['reward_items']),
      nodeDef('decimal', 'Decimal', '精度处理', 'transform', '金额、比例、阶梯、概率和奖池拆分使用 Decimal 精度', ['amount_expr'], ['decimal_amount']),
      nodeDef('limit', 'LIMIT', '次数限制', 'quota', '用户、活动、周期、事件维度参与次数限制', ['subject', 'activity_id'], ['limit_hold']),
      nodeDef('quota', 'Quota', '预算库存扣减', 'quota', '预算、库存、次数限制冻结/确认/释放，保证并发原子性', ['reward_items', 'limit_hold'], ['quota_hold']),
      nodeDef('participation', 'Participation', '参与记录', 'store', '写入参与流水，关联事件、活动、奖励、quota 和阶段结果', ['qualified_context', 'quota_hold'], ['participation_id']),
    ],
  },
  {
    category: '发奖与回调',
    items: [
      nodeDef('grant-order', 'GrantOrder', '发奖订单', 'grant', '生成发奖订单，状态流转 CREATED / SENT / SUCCESS / FAILED', ['participation_id', 'reward_items'], ['grant_order']),
      nodeDef('outbox', 'Outbox', '消息发件箱', 'dispatch', '写入 outbox，后续 worker 发送 HTTP / MQ / Webhook', ['grant_order'], ['outbox_message']),
      nodeDef('http-call', 'HTTP Call', 'HTTP 发奖', 'dispatch', '调用外部发奖接口，记录响应、耗时和错误码', ['outbox_message'], ['http_result']),
      nodeDef('mq-publish', 'MQ Publish', 'MQ 发奖', 'dispatch', '发布发奖指令到消息队列，支持 key、headers 和 traceId', ['outbox_message'], ['mq_result']),
      nodeDef('webhook', 'Webhook', 'Webhook 通知', 'dispatch', '向业务系统推送发奖、失败、参与和风险结果', ['stage_result'], ['webhook_result']),
      nodeDef('callback', 'Callback', '结果回调', 'callback', '处理成功/失败回调，保证 grant、quota、participation 状态一致', ['provider_callback'], ['callback_result']),
    ],
  },
  {
    category: '可靠性',
    items: [
      nodeDef('retry', 'Retry', '失败重试', 'reliable', '指数退避、最大次数、可重入执行和最后错误保存', ['failed_message'], ['retry_message']),
      nodeDef('dlq', 'DLQ', '死信队列', 'reliable', '超过重试次数进入 DLQ，支持人工修复和重新投递', ['failed_message'], ['dlq_record']),
      nodeDef('audit', 'Audit', '审计记录', 'audit', '记录事件、规则、奖励、quota、outbox、callback 全链路证据', ['stage_result'], ['audit_log']),
      nodeDef('reconcile', 'Reconcile', '对账校验', 'audit', '对 grant、quota、callback、outbox 做一致性扫描和修复建议', ['date_range'], ['reconcile_report']),
      nodeDef('alert', 'Alert', '告警通知', 'reliable', '关键失败、DLQ 增长、回调失败和预算异常告警', ['metric_event'], ['alert_event']),
    ],
  },
];

const baseNodes: FlowNode[] = [
  flowNode('event-in', 80, 310),
  flowNode('inbox', 360, 310),
  flowNode('match', 640, 310),
  flowNode('fact-query', 640, 80),
  flowNode('qualify', 920, 310),
  flowNode('condition', 920, 80),
  flowNode('reward', 1200, 220),
  flowNode('limit', 1200, 400),
  flowNode('quota', 1480, 310),
  flowNode('participation', 1760, 310),
  flowNode('grant-order', 2040, 310),
  flowNode('outbox', 2320, 310),
  flowNode('http-call', 2600, 160),
  flowNode('mq-publish', 2600, 310),
  flowNode('webhook', 2600, 460),
  flowNode('callback', 2880, 310),
  flowNode('audit', 1760, 590),
  flowNode('retry', 2320, 590),
  flowNode('dlq', 2600, 590),
];

const baseEdges: ActivityEdge[] = [
  edge('event-in', 'inbox'),
  edge('inbox', 'match'),
  edge('match', 'qualify'),
  edge('fact-query', 'qualify', 'fact'),
  edge('condition', 'qualify', 'condition'),
  edge('qualify', 'reward'),
  edge('qualify', 'limit'),
  edge('reward', 'quota'),
  edge('limit', 'quota'),
  edge('quota', 'participation'),
  edge('participation', 'grant-order'),
  edge('grant-order', 'outbox'),
  edge('outbox', 'http-call'),
  edge('outbox', 'mq-publish'),
  edge('outbox', 'webhook'),
  edge('http-call', 'callback'),
  edge('mq-publish', 'callback'),
  edge('webhook', 'callback'),
  edge('participation', 'audit', 'audit'),
  edge('outbox', 'retry', 'failed'),
  edge('retry', 'dlq', 'exhausted'),
];

function nodeDef(id: string, code: string, title: string, kind: NodeKind, desc: string, inputs: string[], outputs: string[]): ActivityNodeData {
  return { id, code, title, kind, desc, inputs, outputs };
}

function flatCatalog(): ActivityNodeData[] {
  return nodeCatalog.flatMap((group) => group.items);
}

function getDef(id: string): ActivityNodeData {
  return flatCatalog().find((item) => item.id === id) ?? flatCatalog()[0];
}

function flowNode(defId: string, x: number, y: number, extra: Partial<ActivityNode> = {}): ActivityNode {
  const def = getDef(defId);
  return {
    id: extra.id ?? def.id,
    type: 'activity',
    position: { x, y },
    data: { ...def, ...(extra.data ?? {}) },
  };
}

function edge(source: string, target: string, label = ''): ActivityEdge {
  return {
    id: `${source}-${target}-${label || 'default'}`,
    source,
    target,
    label,
    type: 'smoothstep',
    markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 },
  };
}

function cx(...names: Array<string | false | null | undefined>): string {
  return names.filter(Boolean).join(' ');
}

function ActivityNodeView({ id, data }: NodeProps<ActivityNode>) {
  return (
    <div className={cx('activity-node', data.kind)}>
      <Handle type="target" position={Position.Left} />
      <div className="activity-node-head">
        <span className="node-badge">{data.code}</span>
        <strong>{data.title}</strong>
        <button
          className="node-delete"
          title="删除节点"
          onClick={(event) => {
            event.stopPropagation();
            data.onDelete?.(id);
          }}
        >
          ×
        </button>
      </div>
      <p className="node-selectable nodrag">{data.desc}</p>
      <div className="ports node-selectable nodrag">
        <div>
          <b>输入</b>
          {data.inputs.slice(0, 4).map((item) => <span key={item}>{item}</span>)}
        </div>
        <div>
          <b>输出</b>
          {data.outputs.slice(0, 4).map((item) => <span key={item}>{item}</span>)}
        </div>
      </div>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

function GroupNodeView({ id, data }: NodeProps<GroupNode>) {
  return (
    <div className="group-node" style={{ width: data.width, height: data.height }}>
      <div className="group-node-head">
        <strong>{data.title}</strong>
        <button
          className="node-delete"
          title="删除组合"
          onClick={(event) => {
            event.stopPropagation();
            data.onDelete?.(id);
          }}
        >
          ×
        </button>
      </div>
    </div>
  );
}

function NoteNodeView({ id, data }: NodeProps<NoteNode>) {
  return (
    <div className="note-flow-node">
      <button
        className="node-delete"
        title="删除备注"
        onClick={(event) => {
          event.stopPropagation();
          data.onDelete?.(id);
        }}
      >
        ×
      </button>
      <p className="node-selectable nodrag">{data.text}</p>
      <span>ExMarketing</span>
    </div>
  );
}

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

function App() {
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

  const nodeTypes = useMemo(() => ({ activity: ActivityNodeView, group: GroupNodeView, note: NoteNodeView }), []);
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

  const closeContextMenu = useCallback(() => {
    setContextMenu(null);
    setNodePicker(null);
  }, []);

  const addNodeAt = useCallback((def: ActivityNodeData, x: number, y: number) => {
    const id = `${def.id}-${Date.now().toString(36)}`;
    setNodes((items) => items.concat(flowNode(def.id, x, y, { id })));
    setLastAdded(def.title);
    closeContextMenu();
  }, [closeContextMenu, setNodes]);

  const addNoteAt = useCallback((x: number, y: number) => {
    const id = `note-${Date.now().toString(36)}`;
    const note: NoteNode = {
      id,
      type: 'note',
      position: { x, y },
      data: {
        text: '补充活动流程说明、配置约束或异常处理备注。',
      },
    };
    setNodes((items) => items.concat(note));
    setLastAdded('备注');
    closeContextMenu();
  }, [closeContextMenu, setNodes]);

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
          return {
            ...rest,
            position: {
              x: parentPosition.x + item.position.x,
              y: parentPosition.y + item.position.y,
            },
          } as FlowNode;
        });
    });
    setEdges((items) => items.filter((item) => !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target)));
    setSelectedNodeIds((items) => items.filter((item) => !nodeIdSet.has(item)));
    closeContextMenu();
  }, [closeContextMenu, setEdges, setNodes]);

  const deleteEdges = useCallback((edgeIds: string[]) => {
    if (edgeIds.length === 0) return;
    const edgeIdSet = new Set(edgeIds);
    setEdges((items) => items.filter((item) => !edgeIdSet.has(item.id)));
    setSelectedEdgeIds((items) => items.filter((item) => !edgeIdSet.has(item)));
    closeContextMenu();
  }, [closeContextMenu, setEdges]);

  const duplicateNode = useCallback((nodeId: string) => {
    const current = nodes.find((item) => item.id === nodeId);
    if (!current) return;
    const id = `${current.type}-${Date.now().toString(36)}`;
    const duplicate = {
      ...current,
      id,
      selected: false,
      parentId: undefined,
      extent: undefined,
      position: {
        x: current.position.x + 42,
        y: current.position.y + 42,
      },
      data: { ...current.data },
    } as FlowNode;
    setNodes((items) => items.concat(duplicate));
    setLastAdded(`${'title' in current.data ? current.data.title : '备注'} 副本`);
    closeContextMenu();
  }, [closeContextMenu, nodes, setNodes]);

  const openPaneMenu = useCallback((event: MouseEvent | React.MouseEvent) => {
    event.preventDefault();
    const flowPosition = reactFlowInstance?.screenToFlowPosition({
      x: event.clientX,
      y: event.clientY,
    }) ?? { x: 120, y: 120 };
    setContextMenu({
      type: 'pane',
      x: event.clientX,
      y: event.clientY,
      flowX: flowPosition.x,
      flowY: flowPosition.y,
    });
  }, [reactFlowInstance]);

  const openNodeMenu = useCallback((event: React.MouseEvent, node: FlowNode) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedNodeIds([node.id]);
    setSelectedEdgeIds([]);
    setContextMenu({
      type: 'node',
      x: event.clientX,
      y: event.clientY,
      nodeId: node.id,
    });
  }, []);

  const openEdgeMenu = useCallback((event: React.MouseEvent, edgeItem: ActivityEdge) => {
    event.preventDefault();
    event.stopPropagation();
    setSelectedEdgeIds([edgeItem.id]);
    setContextMenu({
      type: 'edge',
      x: event.clientX,
      y: event.clientY,
      edgeId: edgeItem.id,
    });
  }, []);

  const openEdgeMenuFromClick = useCallback((event: React.MouseEvent, edgeItem: ActivityEdge) => {
    event.stopPropagation();
    setSelectedEdgeIds([edgeItem.id]);
    setContextMenu({
      type: 'edge',
      x: event.clientX,
      y: event.clientY,
      edgeId: edgeItem.id,
    });
  }, []);

  const deleteSelected = useCallback(() => {
    const nodeIdSet = new Set(selectedNodeIds);
    const edgeIdSet = new Set(selectedEdgeIds);
    setNodes((items) => items.filter((item) => !nodeIdSet.has(item.id)));
    setEdges((items) => items.filter((item) => {
      if (edgeIdSet.has(item.id)) return false;
      return !nodeIdSet.has(item.source) && !nodeIdSet.has(item.target);
    }));
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeContextMenu();
  }, [closeContextMenu, selectedEdgeIds, selectedNodeIds, setEdges, setNodes]);

  const clearCanvas = useCallback(() => {
    setSelectedTemplate('default');
    setNodes([]);
    setEdges([]);
    setLastAdded('');
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeContextMenu();
  }, [closeContextMenu, setEdges, setNodes]);

  const visibleNodes = useMemo(() => nodes.map((node) => {
    if (node.type === 'activity') {
      return {
        ...node,
        data: {
          ...node.data,
          onDelete: (nodeId: string) => deleteNodes([nodeId]),
        },
      } as ActivityNode;
    }
    if (node.type === 'group') {
      return {
        ...node,
        data: {
          ...node.data,
          onDelete: (nodeId: string) => deleteNodes([nodeId]),
        },
      } as GroupNode;
    }
    return {
      ...node,
      data: {
        ...node.data,
        onDelete: (nodeId: string) => deleteNodes([nodeId]),
      },
    } as NoteNode;
  }), [deleteNodes, nodes]);

  const onConnect = useCallback((params: Connection) => {
    setEdges((items) => addEdge({
      ...params,
      type: 'smoothstep',
      markerEnd: { type: MarkerType.ArrowClosed, width: 16, height: 16 },
    }, items));
  }, [setEdges]);

  const addCatalogNode = useCallback((def: ActivityNodeData) => {
    const offset = nodes.length * 18;
    addNodeAt(def, 180 + (offset % 760), 120 + (offset % 420));
  }, [addNodeAt, nodes.length]);

  const resetDefault = useCallback(() => {
    setSelectedTemplate('default');
    setNodes(baseNodes);
    setEdges(baseEdges);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeContextMenu();
  }, [closeContextMenu, setEdges, setNodes]);

  const loadSimpleTemplate = useCallback(() => {
    setSelectedTemplate('simple');
    setNodes([
      flowNode('event-in', 80, 260),
      flowNode('inbox', 360, 260),
      flowNode('match', 640, 260),
      flowNode('reward', 920, 260),
      flowNode('quota', 1200, 260),
      flowNode('grant-order', 1480, 260),
      flowNode('outbox', 1760, 260),
      flowNode('callback', 2040, 260),
      flowNode('audit', 1200, 500),
    ]);
    setEdges([
      edge('event-in', 'inbox'),
      edge('inbox', 'match'),
      edge('match', 'reward'),
      edge('reward', 'quota'),
      edge('quota', 'grant-order'),
      edge('grant-order', 'outbox'),
      edge('outbox', 'callback'),
      edge('quota', 'audit', 'audit'),
    ]);
    setSelectedNodeIds([]);
    setSelectedEdgeIds([]);
    closeContextMenu();
  }, [closeContextMenu, setEdges, setNodes]);

  const groupSelectedNodes = useCallback(() => {
    const selected = nodes.filter((node) => selectedNodeIds.includes(node.id) && node.type === 'activity');
    if (selected.length < 2) return;
    const minX = Math.min(...selected.map((node) => node.position.x));
    const minY = Math.min(...selected.map((node) => node.position.y));
    const maxX = Math.max(...selected.map((node) => node.position.x + 250));
    const maxY = Math.max(...selected.map((node) => node.position.y + 180));
    const groupId = `group-${Date.now().toString(36)}`;
    const groupX = minX - 28;
    const groupY = minY - 58;
    const groupNode: GroupNode = {
      id: groupId,
      type: 'group',
      position: { x: groupX, y: groupY },
      data: {
        title: `组合 ${selected.length} 个节点`,
        width: maxX - minX + 56,
        height: maxY - minY + 92,
      },
      selectable: true,
    };
    const selectedSet = new Set(selected.map((node) => node.id));
    setNodes((items) => [
      groupNode,
      ...items.map((node) => {
        if (!selectedSet.has(node.id)) return node;
        return {
          ...node,
          parentId: groupId,
          extent: 'parent',
          position: {
            x: node.position.x - groupX,
            y: node.position.y - groupY,
          },
        } as FlowNode;
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
    const draggedCenterX = dragged.position.x + 125;
    const draggedCenterY = dragged.position.y + 90;
    nodes.forEach((node) => {
      if (node.id === dragged.id || node.type !== 'activity' || node.parentId || dragged.parentId) return;
      const centerX = node.position.x + 125;
      const centerY = node.position.y + 90;
      if (Math.abs(centerX - draggedCenterX) <= tolerance) {
        nextGuide.vertical = reactFlowInstance?.flowToScreenPosition({ x: centerX, y: centerY }).x ?? centerX;
      }
      if (Math.abs(centerY - draggedCenterY) <= tolerance) {
        nextGuide.horizontal = reactFlowInstance?.flowToScreenPosition({ x: centerX, y: centerY }).y ?? centerY;
      }
    });
    setAlignmentGuide(nextGuide);
  }, [nodes, reactFlowInstance]);

  const selectedCount = selectedNodeIds.length + selectedEdgeIds.length;
  const selectedNode = useMemo(
    () => nodes.find((node): node is ActivityNode => node.id === selectedNodeIds[0] && node.type === 'activity'),
    [nodes, selectedNodeIds],
  );
  const selectedGroup = useMemo(
    () => nodes.find((node): node is GroupNode => node.id === selectedNodeIds[0] && node.type === 'group'),
    [nodes, selectedNodeIds],
  );
  const selectedNote = useMemo(
    () => nodes.find((node): node is NoteNode => node.id === selectedNodeIds[0] && node.type === 'note'),
    [nodes, selectedNodeIds],
  );
  const selectedEdge = useMemo(
    () => edges.find((item) => item.id === selectedEdgeIds[0]),
    [edges, selectedEdgeIds],
  );

  return (
    <div className="flow-shell">
      <aside className="flow-sidebar">
        <div className="brand">
          <b>ExMarketing</b>
          <span>Activity Flow</span>
        </div>
        <button className="back-link" onClick={() => { window.location.href = '/admin/'; }}>返回旧后台</button>

        <div className="side-head">
          <h2>活动流程模板</h2>
          <span>{selectedTemplate === 'default' ? '完整链路' : '简化链路'}</span>
        </div>
        <div className="template-actions">
          <button onClick={resetDefault}>完整活动引擎</button>
          <button onClick={loadSimpleTemplate}>简化发奖链路</button>
        </div>

        <div className="side-head">
          <h2>常用节点函数</h2>
          <span>{flatCatalog().length}</span>
        </div>
        <div className="node-palette">
          {nodeCatalog.map((group) => (
            <section key={group.category}>
              <h3>{group.category}</h3>
              {group.items.map((item) => (
                <button key={item.id} onClick={() => addCatalogNode(item)}>
                  <span className={cx('palette-dot', item.kind)} />
                  <strong>{item.title}</strong>
                  <em>{item.code}</em>
                </button>
              ))}
            </section>
          ))}
        </div>
      </aside>

      <main className="studio">
        <header className="studio-head">
          <div>
            <h1>活动数据流设计器</h1>
            <p>用于设计活动引擎的数据执行链路：事件进入、规则判断、奖励计算、额度扣减、发奖、回调、审计和重试。</p>
          </div>
          <div className="head-actions">
            {lastAdded ? <span className="status-pill green">已添加：{lastAdded}</span> : null}
            <span className="status-pill">节点 {nodes.length}</span>
            <span className="status-pill">连线 {edges.length}</span>
            <button className="ghost-action" disabled={selectedNodeIds.length < 2} onClick={groupSelectedNodes}>
              组合选中
            </button>
            <button className="ghost-action" onClick={clearCanvas}>清空画布</button>
            <button className="danger-action" disabled={selectedCount === 0} onClick={deleteSelected}>
              删除选中 {selectedCount > 0 ? selectedCount : ''}
            </button>
          </div>
        </header>

        <section className="canvas-card">
          <ReactFlowProvider>
            <ReactFlow
              nodes={visibleNodes}
              edges={edges}
              nodeTypes={nodeTypes}
              onInit={setReactFlowInstance}
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onConnect={onConnect}
              onPaneClick={() => {
                closeContextMenu();
                setSelectedNodeIds([]);
                setSelectedEdgeIds([]);
              }}
              onPaneContextMenu={openPaneMenu}
              onNodeClick={(event, node) => {
                closeContextMenu();
                if (event.shiftKey || event.metaKey || event.ctrlKey) {
                  setSelectedNodeIds((items) => (
                    items.includes(node.id)
                      ? items.filter((item) => item !== node.id)
                      : items.concat(node.id)
                  ));
                } else {
                  setSelectedNodeIds([node.id]);
                }
                setSelectedEdgeIds([]);
              }}
              onNodeContextMenu={openNodeMenu}
              onEdgeClick={openEdgeMenuFromClick}
              onEdgeContextMenu={openEdgeMenu}
              onNodeDrag={(_, node) => updateAlignmentGuide(node)}
              onNodeDragStop={() => setAlignmentGuide({})}
              onSelectionChange={({ nodes: selectedNodes, edges: selectedEdges }) => {
                setSelectedNodeIds(selectedNodes.map((node) => node.id));
                setSelectedEdgeIds(selectedEdges.map((item) => item.id));
              }}
              defaultViewport={{ x: 24, y: 36, zoom: 0.72 }}
              minZoom={0.25}
              maxZoom={1.6}
              panOnScroll
              selectionOnDrag
            >
              <Background color="#d9e2ee" gap={16} size={1} />
              <Controls position="top-left" showInteractive={false} />
              <MiniMap position="bottom-right" pannable zoomable nodeStrokeWidth={2} />
            </ReactFlow>
            {alignmentGuide.vertical !== undefined ? (
              <div className="align-guide vertical" style={{ left: alignmentGuide.vertical }} />
            ) : null}
            {alignmentGuide.horizontal !== undefined ? (
              <div className="align-guide horizontal" style={{ top: alignmentGuide.horizontal }} />
            ) : null}
            {contextMenu ? (
              <div
                className="context-menu"
                style={{ left: contextMenu.x, top: contextMenu.y }}
                onClick={(event) => event.stopPropagation()}
              >
                {contextMenu.type === 'pane' ? (
                  <>
                    <button onClick={() => {
                      setNodeSearch('');
                      setNodePicker({
                        x: contextMenu.x + 160,
                        y: contextMenu.y - 8,
                        flowX: contextMenu.flowX,
                        flowY: contextMenu.flowY,
                      });
                    }}>Add Node</button>
                    <button onClick={() => addNoteAt(contextMenu.flowX, contextMenu.flowY)}>Add Note</button>
                    <button onClick={() => setLastAdded('Preview')}>Preview</button>
                    <button className="disabled" disabled>
                      <span>Paste Here</span>
                      <kbd>⌘ V</kbd>
                    </button>
                    <div className="context-divider" />
                    <button onClick={() => setLastAdded('Export DSL')}>Export DSL</button>
                    <button onClick={() => setLastAdded('Import App')}>Import App</button>
                  </>
                ) : null}
                {contextMenu.type === 'node' ? (
                  <>
                    <div className="context-title">节点操作</div>
                    <button onClick={() => duplicateNode(contextMenu.nodeId)}>复制节点</button>
                    <button className="danger" onClick={() => deleteNodes([contextMenu.nodeId])}>删除节点</button>
                  </>
                ) : null}
                {contextMenu.type === 'edge' ? (
                  <>
                    <div className="context-title">连线操作</div>
                    <button className="danger" onClick={() => deleteEdges([contextMenu.edgeId])}>删除连线</button>
                  </>
                ) : null}
              </div>
            ) : null}
            {nodePicker ? (
              <div
                className="node-picker"
                style={{ left: nodePicker.x, top: nodePicker.y }}
                onClick={(event) => event.stopPropagation()}
              >
                <div className="picker-tabs">
                  <button className="active">Nodes</button>
                  <button>Tools</button>
                </div>
                <div className="picker-search">
                  <span>⌕</span>
                  <input
                    autoFocus
                    value={nodeSearch}
                    placeholder="Search node"
                    onChange={(event) => setNodeSearch(event.target.value)}
                  />
                </div>
                <div className="picker-list">
                  {filteredCatalog.map((group) => (
                    <section key={group.category}>
                      <h4>{group.category}</h4>
                      {group.items.map((item) => (
                        <button key={item.id} onClick={() => addNodeAt(item, nodePicker.flowX, nodePicker.flowY)}>
                          <span className={cx('picker-icon', item.kind)}>{item.code.slice(0, 1)}</span>
                          <strong>{item.title}</strong>
                          <em>{item.code}</em>
                        </button>
                      ))}
                    </section>
                  ))}
                  {filteredCatalog.length === 0 ? <div className="picker-empty">No nodes found</div> : null}
                </div>
              </div>
            ) : null}
          </ReactFlowProvider>
        </section>
      </main>
      <aside className="property-panel">
        <div className="property-head">
          <b>属性</b>
          <span>{selectedNode ? '节点' : selectedGroup ? '组合' : selectedNote ? '备注' : selectedEdge ? '连线' : '未选择'}</span>
        </div>
        {selectedNode ? (
          <div className="property-body">
            <section className="property-card">
              <div className="property-title-row">
                <span className={cx('palette-dot', selectedNode.data.kind)} />
                <div>
                  <h2>{selectedNode.data.title}</h2>
                  <p>{selectedNode.data.code}</p>
                </div>
              </div>
              <dl>
                <div>
                  <dt>节点 ID</dt>
                  <dd>{selectedNode.id}</dd>
                </div>
                <div>
                  <dt>节点类型</dt>
                  <dd>{selectedNode.data.kind}</dd>
                </div>
                <div>
                  <dt>位置</dt>
                  <dd>x {Math.round(selectedNode.position.x)} / y {Math.round(selectedNode.position.y)}</dd>
                </div>
              </dl>
            </section>
            <section className="property-section">
              <h3>说明</h3>
              <p>{selectedNode.data.desc}</p>
            </section>
            <PropertyList title="输入字段" items={selectedNode.data.inputs} />
            <PropertyList title="输出字段" items={selectedNode.data.outputs} />
            <section className="property-section">
              <h3>配置参数</h3>
              <div className="property-empty">后续可在这里编辑规则表达式、奖励参数、Quota 策略、回调目标和重试策略。</div>
            </section>
          </div>
        ) : null}
        {!selectedNode && selectedGroup ? (
          <div className="property-body">
            <section className="property-card">
              <h2>{selectedGroup.data.title}</h2>
              <dl>
                <div>
                  <dt>组合 ID</dt>
                  <dd>{selectedGroup.id}</dd>
                </div>
                <div>
                  <dt>尺寸</dt>
                  <dd>{Math.round(selectedGroup.data.width)} × {Math.round(selectedGroup.data.height)}</dd>
                </div>
                <div>
                  <dt>位置</dt>
                  <dd>x {Math.round(selectedGroup.position.x)} / y {Math.round(selectedGroup.position.y)}</dd>
                </div>
              </dl>
              <button className="property-danger" onClick={() => deleteNodes([selectedGroup.id])}>删除组合</button>
            </section>
            <section className="property-section">
              <h3>组合说明</h3>
              <p>组合用于把多个活动节点作为一个流程块管理，拖动组合可以整体移动内部节点。</p>
            </section>
          </div>
        ) : null}
        {!selectedNode && !selectedGroup && selectedNote ? (
          <div className="property-body">
            <section className="property-card">
              <h2>备注</h2>
              <dl>
                <div>
                  <dt>备注 ID</dt>
                  <dd>{selectedNote.id}</dd>
                </div>
                <div>
                  <dt>位置</dt>
                  <dd>x {Math.round(selectedNote.position.x)} / y {Math.round(selectedNote.position.y)}</dd>
                </div>
              </dl>
              <button className="property-danger" onClick={() => deleteNodes([selectedNote.id])}>删除备注</button>
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
                <div>
                  <dt>连线 ID</dt>
                  <dd>{selectedEdge.id}</dd>
                </div>
                <div>
                  <dt>来源节点</dt>
                  <dd>{selectedEdge.source}</dd>
                </div>
                <div>
                  <dt>目标节点</dt>
                  <dd>{selectedEdge.target}</dd>
                </div>
                <div>
                  <dt>标签</dt>
                  <dd>{selectedEdge.label || '默认路径'}</dd>
                </div>
              </dl>
              <button className="property-danger" onClick={() => deleteEdges([selectedEdge.id])}>删除连线</button>
            </section>
          </div>
        ) : null}
        {!selectedNode && !selectedGroup && !selectedNote && !selectedEdge ? (
          <div className="property-empty-state">
            <b>选择一个节点</b>
            <p>点击画布中的活动节点后，可以在这里查看输入、输出、说明和后续配置项。</p>
          </div>
        ) : null}
      </aside>
    </div>
  );
}

const root = document.getElementById('root');
if (!root) {
  throw new Error('Root element #root not found');
}

createRoot(root).render(<App />);
