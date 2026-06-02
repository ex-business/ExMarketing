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
  WorkflowNodeExecution,
} from './types';
import { ACTIVITY_NODE_WIDTH, activityNodeHeight, cx, edge, flowNode, layoutFlowNodes } from './utils/flow';

type ActivitySummary = {
  id: string;
  activityCode: string;
  activityName: string;
  tenantId: string;
};

function serializeWorkflowDefinition(nodes: FlowNode[], edges: ActivityEdge[], viewport?: { x: number; y: number; zoom: number }): WorkflowDefinition {
  return {
    nodes: nodes
      .filter((node): node is ActivityNode => node.type === 'activity')
      .map((node) => ({
        id: node.id,
        type: node.data.id,
        kind: node.data.kind,
        code: node.data.code,
        title: node.data.title,
        position: { x: node.position.x, y: node.position.y },
        inputs: node.data.inputs,
        outputs: node.data.outputs,
      })),
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
    },
  };
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
        },
      });
    }),
    edges: definition.edges.map((item) => edge(item.source, item.target, item.label ?? '', item.sourceHandle, item.targetHandle)),
  };
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
  const [isPublished, setIsPublished] = useState(false);
  const [activityId, setActivityId] = useState('');
  const [activityLabel, setActivityLabel] = useState('未绑定活动');
  const [workflowStatus, setWorkflowStatus] = useState('未连接后端');

  const nodeTypes = useMemo(() => ({ activity: ActivityNodeView, group: GroupNodeView, note: NoteNodeView }), []);
  const selectedCount = selectedNodeIds.length + selectedEdgeIds.length;

  useEffect(() => {
    let cancelled = false;
    const params = new URLSearchParams(window.location.search);
    const requestedActivityId = params.get('activityId') ?? '';
    fetchJSON<{ items: ActivitySummary[] }>('/admin/v1/activities')
      .then((body) => {
        if (cancelled) return;
        const activity = body.items.find((item) => item.id === requestedActivityId) ?? body.items[0];
        if (!activity) {
          setWorkflowStatus('没有可绑定活动');
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
    if (!activityId) return;
    let cancelled = false;
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
        setWorkflowStatus(error.message.includes('not found') ? '当前活动暂无草稿' : `草稿加载失败：${error.message}`);
      });
    return () => {
      cancelled = true;
    };
  }, [activityId, setEdges, setNodes]);

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

  const visibleNodes = useMemo(() => nodes.map((node) => {
    if (node.type === 'activity') {
      return {
        ...node,
        data: {
          ...node.data,
          onDelete: (nodeId: string) => deleteNodes([nodeId]),
          onAddFromOutput: openOutputNodePicker,
          onHoverChange: (nodeId: string, hovering: boolean) => setHoveredNodeId(hovering ? nodeId : ''),
        },
      } as ActivityNode;
    }
    return { ...node, data: { ...node.data, onDelete: (nodeId: string) => deleteNodes([nodeId]) } } as FlowNode;
  }), [deleteNodes, nodes, openOutputNodePicker]);

  const visibleEdges = useMemo(() => edges.map((item) => ({
    ...item,
    className: cx(
      item.className,
      item.id === hoveredEdgeId && 'edge-hover',
      hoveredNodeId !== '' && (item.source === hoveredNodeId || item.target === hoveredNodeId) && 'edge-node-hover',
      selectedNodeIds.some((nodeId) => item.source === nodeId || item.target === nodeId) && 'edge-related',
    ),
  })), [edges, hoveredEdgeId, hoveredNodeId, selectedNodeIds]);

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

  const saveDraft = useCallback(async (): Promise<boolean> => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return false;
    }
    const definition = serializeWorkflowDefinition(nodes, edges, reactFlowInstance?.getViewport());
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
  }, [activityId, edges, nodes, reactFlowInstance]);

  const publishWorkflow = useCallback(async () => {
    if (!activityId) {
      setWorkflowStatus('需要先绑定活动');
      return;
    }
    try {
      const draftSaved = await saveDraft();
      if (!draftSaved) {
        return;
      }
      await fetchJSON(`/admin/v1/activities/${activityId}/workflow/publish`, {
        method: 'POST',
        body: JSON.stringify({ operator: 'admin-flow' }),
      });
      setIsPublished(true);
      setShowChecklist(true);
      setWorkflowStatus('已发布 workflow version');
      setLastAdded('发布版本');
    } catch (error) {
      setWorkflowStatus(`发布失败：${(error as Error).message}`);
    }
  }, [activityId, saveDraft]);

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
      <NodePalette open={showPalette} selectedTemplate={selectedTemplate} onAddNode={addCatalogNode} onDefault={resetDefault} onSimple={loadSimpleTemplate} />
      <main className="studio">
        <StudioChrome
          showPalette={showPalette}
          interactionMode={interactionMode}
          nodeCount={nodes.length}
          edgeCount={edges.length}
          selectedCount={selectedCount}
          selectedNodeCount={selectedNodeIds.length}
          lastAdded={lastAdded}
          activityLabel={activityLabel}
          workflowStatus={workflowStatus}
          zoomPercent={zoomPercent}
          showChecklist={showChecklist}
          isPublished={isPublished}
          onTogglePalette={() => setShowPalette((value) => !value)}
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
          onToggleChecklist={() => setShowChecklist((value) => !value)}
          onPublish={publishWorkflow}
        />
        <section className="canvas-card">
          <ReactFlowProvider>
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
          </ReactFlowProvider>
        </section>
      </main>
      <PropertyPanel nodes={nodes} edges={edges} selectedNodeIds={selectedNodeIds} selectedEdgeIds={selectedEdgeIds} onDeleteNodes={deleteNodes} onDeleteEdges={deleteEdges} />
    </div>
  );
}
