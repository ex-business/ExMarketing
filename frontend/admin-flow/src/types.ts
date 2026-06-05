import type { Edge, Node } from '@xyflow/react';

export type NodeKind =
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

export type OutputAddRequest = {
  nodeId: string;
  outputIndex: number;
  outputLabel: string;
  clientX: number;
  clientY: number;
};

export type ActivityNodeData = {
  id: string;
  code: string;
  title: string;
  kind: NodeKind;
  desc: string;
  inputs: string[];
  outputs: string[];
  config?: Record<string, unknown>;
  runStatus?: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
  lastRun?: {
    status: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
    errorMessage?: string;
    output?: unknown;
    finishedAt?: string;
  };
  onDelete?: (nodeId: string) => void;
  onAddFromOutput?: (request: OutputAddRequest) => void;
  onHoverChange?: (nodeId: string, hovering: boolean) => void;
};

export type GroupNodeData = {
  title: string;
  width: number;
  height: number;
  onDelete?: (nodeId: string) => void;
};

export type NoteNodeData = {
  text: string;
  onDelete?: (nodeId: string) => void;
};

export type ActivityNode = Node<ActivityNodeData, 'activity'>;
export type GroupNode = Node<GroupNodeData, 'group'>;
export type NoteNode = Node<NoteNodeData, 'note'>;
export type FlowNode = ActivityNode | GroupNode | NoteNode;
export type ActivityEdge = Edge;

export type CatalogGroup = {
  category: string;
  items: ActivityNodeData[];
};

export type FlowContextMenu =
  | { type: 'pane'; x: number; y: number; flowX: number; flowY: number }
  | { type: 'node'; x: number; y: number; nodeId: string }
  | { type: 'edge'; x: number; y: number; edgeId: string };

export type NodePickerState = {
  x: number;
  y: number;
  flowX: number;
  flowY: number;
  sourceNodeId?: string;
  sourceHandle?: string;
  sourceLabel?: string;
};

export type AlignmentGuide = {
  vertical?: { screen: number; start: number; end: number; kind: 'left' | 'center' | 'right' };
  horizontal?: { screen: number; start: number; end: number; kind: 'top' | 'center' | 'bottom' };
};

export type WorkflowDefinition = {
  nodes: Array<{
    id: string;
    type: string;
    kind?: string;
    code?: string;
    title?: string;
    position?: { x: number; y: number };
    inputs?: string[];
    outputs?: string[];
    config?: unknown;
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    sourceHandle?: string;
    targetHandle?: string;
    label?: string;
  }>;
  viewport?: { x: number; y: number; zoom: number };
  meta?: Record<string, unknown>;
};

export type WorkflowExecutionStatus = 'RUNNING' | 'SUCCESS' | 'FAILED';

export type WorkflowExecution = {
  id: string;
  tenantId: string;
  activityId: string;
  workflowVersionId: string;
  workflowVersion: number;
  eventInboxId?: string;
  eventId?: string;
  status: WorkflowExecutionStatus;
  startedAt: string;
  finishedAt?: string;
  errorMessage?: string;
};

export type WorkflowVersion = {
  id: string;
  tenantId: string;
  activityId: string;
  version: number;
  status: 'ACTIVE' | 'ARCHIVED';
  publishedBy?: string;
  publishedAt: string;
};

export type WorkflowValidationSeverity = 'BLOCK' | 'WARNING';

export type WorkflowValidationIssue = {
  severity: WorkflowValidationSeverity;
  code: string;
  message: string;
  nodeId?: string;
  edgeId?: string;
  path?: string;
};

export type WorkflowValidationResult = {
  valid: boolean;
  items: WorkflowValidationIssue[];
  summary: {
    blockCount: number;
    warningCount: number;
  };
};

export type WorkflowNodeConfigFieldType = 'STRING' | 'NUMBER' | 'BOOLEAN' | 'ARRAY' | 'OBJECT' | 'ENUM';

export type WorkflowNodeConfigValidationRule = {
  type: 'MIN_LENGTH' | 'MAX_LENGTH' | 'PATTERN' | 'MIN_VALUE' | 'MAX_VALUE' | 'MIN_ITEMS' | 'MAX_ITEMS';
  value: unknown;
  message?: string;
};

export type WorkflowNodeConfigField = {
  key: string;
  label: string;
  type: WorkflowNodeConfigFieldType;
  required?: boolean;
  default?: unknown;
  options?: string[];
  description?: string;
  validationRules?: WorkflowNodeConfigValidationRule[];
};

export type WorkflowNodeConfigSchema = {
  nodeType: string;
  version: number;
  title: string;
  description?: string;
  fields: WorkflowNodeConfigField[];
  inputs: WorkflowNodeInputVar[];
  outputs: WorkflowNodeOutputVar[];
};

export type WorkflowNodeInputVar = {
  key: string;
  label: string;
  type?: string;
  required?: boolean;
  description?: string;
};

export type WorkflowNodeOutputVar = {
  key: string;
  label: string;
  type?: string;
  description?: string;
};

export type WorkflowNodeExecution = {
  id: string;
  executionId: string;
  nodeId: string;
  nodeType: string;
  nodeTitle?: string;
  status: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
  output?: unknown;
  errorMessage?: string;
  finishedAt?: string;
};
