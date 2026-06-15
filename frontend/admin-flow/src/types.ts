import type { Edge, Node } from '@xyflow/react';

export type ActivitySummary = {
  id: string;
  workspaceId?: string;
  folderId?: string;
  activityCode: string;
  activityName: string;
  tenantId: string;
  triggerEvents?: string[];
  status?: string;
  visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  version?: number;
  createdAt?: string;
  updatedAt?: string;
};

export type WorkspaceRole = 'OWNER' | 'ADMIN' | 'EDITOR' | 'VIEWER' | 'GUEST';

export type WorkspaceSummary = {
  id: string;
  tenantId: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
  color?: string;
  visibility: 'PRIVATE' | 'TEAM' | 'PUBLIC';
  defaultRole?: WorkspaceRole;
  ownerId?: string;
  status: string;
};

export type WorkspaceFolderSummary = {
  id: string;
  workspaceId: string;
  parentId?: string;
  name: string;
  color?: string;
  visibility: string;
  sortOrder: number;
  status: string;
};

export type WorkspaceShareLinkSummary = {
  id: string;
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  scope: 'WORKSPACE' | 'FOLDER' | 'ACTIVITY';
  permission?: 'VIEW' | 'EDIT';
  expiresAt?: string;
  maxAccessCount?: number;
  accessCount?: number;
  lastAccessedAt?: string;
  disabledReason?: string;
  enabled: boolean;
  token?: string;
  createdBy?: string;
  createdAt?: string;
};

export type WorkspaceBatchResult = {
  committed?: boolean;
  count?: number;
  items?: Array<{
    id: string;
    type: string;
    status: string;
    error?: string;
  }>;
};

export type WorkspaceFileObjectSummary = {
  id: string;
  workspaceId: string;
  folderId?: string;
  activityId?: string;
  workflowVersionId?: string;
  executionId?: string;
  kind: string;
  fileName: string;
  contentType?: string;
  sizeBytes: number;
  storageProvider: string;
  storageKey: string;
  checksumSha256: string;
  createdBy?: string;
  status: string;
  createdAt: string;
};

export type WorkspaceMemberSummary = {
  id: string;
  workspaceId: string;
  userId?: string;
  email?: string;
  displayName?: string;
  role: WorkspaceRole;
  status: 'ACTIVE' | 'INVITED' | 'SUSPENDED' | 'REMOVED';
};

export type WorkspaceAuditLogSummary = {
  id: string;
  workspaceId: string;
  actorId?: string;
  action: string;
  targetType: string;
  targetId: string;
  createdAt: string;
};

export type WorkspaceActivityGrantSummary = {
  id: string;
  workspaceId: string;
  activityId: string;
  principalType: 'USER' | 'MEMBER' | 'ROLE';
  principalId: string;
  role: Exclude<WorkspaceRole, 'OWNER'>;
  createdBy?: string;
  updatedBy?: string;
  createdAt: string;
  updatedAt: string;
};

export type WorkspaceHomeSearchResult = {
  id: string;
  type: 'workspace' | 'workflow' | 'folder' | 'member';
  title: string;
  subtitle: string;
  workspaceId?: string;
  folderId?: string;
  highlights?: {
    title?: string;
    subtitle?: string;
  };
  permission?: string;
  permissionReason?: string;
  permissionSources?: string[];
  score?: number;
};

export type WorkspaceHomeStorageSummary = {
  usedBytes: number;
  totalBytes: number;
  workspaces: number;
  folders: number;
  workflows: number;
  members: number;
  shareLinks: number;
  auditLogs: number;
  drafts?: number;
  versions?: number;
  executions?: number;
  nodeExecutions?: number;
  activityGrants?: number;
  fileObjects?: number;
  fileBytes?: number;
};

export type ActivityPlacement = {
  workspaceId: string;
  folderId: string;
  shareLinkId?: string;
};

export type CreateStudioWorkspaceInput = {
  name: string;
  tenantId: string;
};

export type CreateStudioActivityInput = {
  tenantId: string;
  activityName: string;
  activityCode: string;
  workspaceId: string;
  folderId: string;
  definition?: WorkflowDefinition;
  visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC';
};

export type StudioFolderPatch = {
  parentId?: string;
  name?: string;
  visibility?: string;
  sortOrder?: number;
  status?: string;
};

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
  category?: 'EVENT_INPUT' | 'CORE' | 'ADVANCED' | 'OPS' | string;
  kind: NodeKind;
  desc: string;
  inputs: string[];
  outputs: string[];
  config?: Record<string, unknown>;
  runStatus?: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
  validationState?: 'BLOCK' | 'WARNING';
  validationCount?: number;
  validationMessage?: string;
  routeIssues?: Array<{ code: string; message: string; path?: string }>;
  lastRun?: {
    status: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
    errorMessage?: string;
    input?: unknown;
    output?: unknown;
    startedAt?: string;
    finishedAt?: string;
    durationMs?: number;
    retryCount?: number;
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

export type WorkflowEdgeRouteType = 'DEFAULT' | 'OUTPUT' | 'IF' | 'ELIF' | 'ELSE' | 'PASS' | 'BLOCK' | 'RISK' | 'SKIP';

export type WorkflowEdgeRoute = {
  type: WorkflowEdgeRouteType;
  key?: string;
  label?: string;
  expression?: string;
  conditionRef?: string;
  default?: boolean;
};

export type ActivityEdgeData = {
  route?: WorkflowEdgeRoute;
  runSelected?: boolean;
};

export type ActivityEdge = Edge<ActivityEdgeData>;

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
    route?: WorkflowEdgeRoute;
  }>;
  viewport?: { x: number; y: number; zoom: number };
  meta?: Record<string, unknown>;
};

export type WorkflowExecutionStatus = 'RUNNING' | 'SUCCESS' | 'FAILED' | 'CANCELED';

export type WorkflowExecution = {
  id: string;
  tenantId: string;
  activityId: string;
  workflowVersionId: string;
  workflowVersion: number;
  eventInboxId?: string;
  eventId?: string;
  status: WorkflowExecutionStatus;
  input?: unknown;
  output?: unknown;
  startedAt: string;
  finishedAt?: string;
  errorMessage?: string;
};

export type WorkflowVersion = {
  id: string;
  tenantId: string;
  activityId: string;
  version: number;
  status: 'DRAFT' | 'ACTIVE' | 'SUPERSEDED' | 'ARCHIVED';
  definition: WorkflowDefinition;
  publishedBy?: string;
  publishedAt: string;
  releaseNote?: string;
  sourceVersionId?: string;
  action?: string;
  createdAt?: string;
};

export type WorkflowDiffSummary = {
  changed: boolean;
  summary: string;
  detail: string[];
};

export type WorkflowDiffItemKind = 'NODE_ADDED' | 'NODE_REMOVED' | 'NODE_CHANGED' | 'EDGE_ADDED' | 'EDGE_REMOVED' | 'EDGE_CHANGED';

export type WorkflowDiffItem = {
  id: string;
  kind: WorkflowDiffItemKind;
  label: string;
  nodeId?: string;
  edgeId?: string;
  source?: string;
  target?: string;
  detail?: string;
};

export type WorkflowDiffDetail = WorkflowDiffSummary & {
  items: WorkflowDiffItem[];
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
  items: WorkflowValidationIssue[] | null;
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

export type WorkflowNodeConfigFieldGroup = {
  key: string;
  title: string;
  description?: string;
  fields: WorkflowNodeConfigField[];
};

export type WorkflowNodeConfigPreview = {
  reads?: string[];
  writes?: string[];
  notes?: string[];
};

export type WorkflowNodeConfigSchema = {
  nodeType: string;
  version: number;
  category?: 'EVENT_INPUT' | 'CORE' | 'ADVANCED' | 'OPS' | string;
  title: string;
  description?: string;
  fields: WorkflowNodeConfigField[];
  fieldGroups?: WorkflowNodeConfigFieldGroup[];
  preview?: WorkflowNodeConfigPreview;
  inputs: WorkflowNodeInputVar[];
  outputs: WorkflowNodeOutputVar[];
};

export type ResourceOperation =
  | 'READ'
  | 'WRITE'
  | 'UPSERT'
  | 'APPEND'
  | 'DELETE'
  | 'CAS'
  | 'CALL'
  | 'PUBLISH'
  | 'TEST_CONNECTION'
  | 'CHECK_AND_INCREMENT';

export type ResourceSummary = {
  id: string;
  tenantId: string;
  workspaceId?: string;
  folderId?: string;
  activityId?: string;
  code: string;
  name: string;
  description?: string;
  type: string;
  driver: string;
  scope: string;
  status: string;
  environment?: string;
  config?: Record<string, unknown>;
  capabilities?: ResourceOperation[];
  schemaVersion?: number;
  createdBy?: string;
  updatedBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type ResourceBindingSummary = {
  id: string;
  resourceId: string;
  tenantId: string;
  workspaceId?: string;
  folderId?: string;
  activityId?: string;
  nodeType?: string;
  nodeId?: string;
  principalType?: string;
  principalId?: string;
  minRole?: string;
  allowedOperations?: ResourceOperation[];
  allowedNamespaces?: string[];
  keyPolicy?: Record<string, unknown>;
  includeSubfolders?: boolean;
  requireApproval?: boolean;
  status: string;
  createdBy?: string;
  updatedBy?: string;
  createdAt?: string;
  updatedAt?: string;
};

export type ResourceAuditLogSummary = {
  id: string;
  tenantId: string;
  workspaceId?: string;
  folderId?: string;
  activityId?: string;
  workflowExecutionId?: string;
  nodeId?: string;
  nodeType?: string;
  resourceId: string;
  resourceCode: string;
  operation: ResourceOperation;
  status: string;
  durationMs?: number;
  errorCode?: string;
  errorMessage?: string;
  actorId?: string;
  createdAt?: string;
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
  input?: unknown;
  output?: unknown;
  errorMessage?: string;
  startedAt: string;
  finishedAt?: string;
};

export type WorkflowNodeSummary = {
  nodeId: string;
  nodeType: string;
  nodeTitle?: string;
  status: 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';
  durationMs?: number;
  inputKeys?: string[];
  outputKeys?: string[];
  decision?: string;
  errorCode?: string;
  explanation?: string;
};

export type WorkflowExecutionTrace = {
  schemaVersion?: string;
  exportedAt?: string;
  execution: WorkflowExecution;
  nodeExecutions: WorkflowNodeExecution[];
  nodeSummaries?: WorkflowNodeSummary[];
  routeTrace?: unknown;
};

export type WorkflowExecutionJSONDiff = {
  path: string;
  type: 'added' | 'removed' | 'changed';
  current?: unknown;
  baseline?: unknown;
};

export type WorkflowExecutionPayloadNodeDiff = {
  nodeId: string;
  label: string;
  input: WorkflowExecutionJSONDiff[];
  output: WorkflowExecutionJSONDiff[];
};

export type WorkflowExecutionNodeStatusDiff = {
  nodeId: string;
  label: string;
  currentStatus?: WorkflowNodeExecution['status'];
  baselineStatus?: WorkflowNodeExecution['status'];
  currentError?: string;
  baselineError?: string;
  currentErrorCode?: string;
  baselineErrorCode?: string;
  currentAttempt?: number;
  baselineAttempt?: number;
  currentDurationMs?: number;
  baselineDurationMs?: number;
};

export type WorkflowExecutionSideEffectDiff = {
  kind: string;
  changes: WorkflowExecutionJSONDiff[];
  current?: unknown;
  baseline?: unknown;
};

export type WorkflowExecutionCompareResult = {
  current: Pick<WorkflowExecution, 'id' | 'status' | 'eventId' | 'workflowVersion' | 'errorMessage' | 'startedAt' | 'finishedAt'>;
  baseline: Pick<WorkflowExecution, 'id' | 'status' | 'eventId' | 'workflowVersion' | 'errorMessage' | 'startedAt' | 'finishedAt'>;
  currentPath: string[];
  baselinePath: string[];
  pathChanged: boolean;
  nodeStatusChanges?: WorkflowExecutionNodeStatusDiff[];
  sideEffectChanges?: WorkflowExecutionSideEffectDiff[];
  currentFailures: string[];
  baselineFailures: string[];
  currentDurationMs: number;
  baselineDurationMs: number;
  durationDeltaMs: number;
  payloadChanges: WorkflowExecutionPayloadNodeDiff[];
  payloadChangeLimit?: number;
  jsonDiffLimit?: number;
  truncated?: boolean;
};
