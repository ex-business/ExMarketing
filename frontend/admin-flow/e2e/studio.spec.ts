import { expect, test, type Page, type Route } from '@playwright/test';

const activityId = 'act_e2e';
const secondaryActivityId = 'act_secondary';
const workspaceId = 'ws_e2e';
const folderId = 'folder_e2e';
const secondaryFolderId = 'folder_archive';

const workflowDefinition = {
  nodes: [
    {
      id: 'event-in',
      type: 'event-in',
      kind: 'entry',
      code: 'Event In',
      title: '事件入口',
      position: { x: 80, y: 380 },
      inputs: ['eventId'],
      outputs: ['raw_event'],
      config: {},
    },
    {
      id: 'inbox',
      type: 'inbox',
      kind: 'store',
      code: 'Inbox',
      title: '幂等收件箱',
      position: { x: 410, y: 380 },
      inputs: ['raw_event'],
      outputs: ['event_context'],
      config: {},
    },
    {
      id: 'match',
      type: 'match',
      kind: 'logic',
      code: 'MATCH',
      title: '活动匹配',
      position: { x: 740, y: 380 },
      inputs: ['event_context'],
      outputs: ['matched_activity'],
      config: {},
    },
  ],
  edges: [
    {
      id: 'event-in-inbox-default',
      source: 'event-in',
      target: 'inbox',
      label: '默认路径',
      route: { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true },
    },
    {
      id: 'inbox-match-default',
      source: 'inbox',
      target: 'match',
      label: '默认路径',
      route: { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true },
    },
  ],
  viewport: { x: 24, y: 36, zoom: 1 },
  meta: { activityId, workspaceId, folderId },
};

const conditionWorkflowDefinition = {
  nodes: [
    workflowDefinition.nodes[0],
    {
      id: 'condition',
      type: 'condition',
      kind: 'logic',
      code: 'CONDITION',
      title: '条件判断',
      position: { x: 410, y: 350 },
      inputs: ['fact'],
      outputs: ['IF', 'ELSE'],
      config: {},
    },
    {
      id: 'reward',
      type: 'reward',
      kind: 'reward',
      code: 'REWARD',
      title: '奖励计算',
      position: { x: 740, y: 300 },
      inputs: ['qualified_context'],
      outputs: ['reward_items'],
      config: {},
    },
  ],
  edges: [
    {
      id: 'event-in-condition',
      source: 'event-in',
      target: 'condition',
      label: '默认路径',
      route: { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true },
    },
    {
      id: 'condition-if',
      source: 'condition',
      target: 'reward',
      label: 'IF',
      route: { type: 'IF', key: 'IF', label: 'IF' },
    },
  ],
  viewport: { x: 24, y: 36, zoom: 1 },
  meta: { activityId, workspaceId, folderId },
};

const templateDefinition = {
  nodes: [
    workflowDefinition.nodes[0],
    workflowDefinition.nodes[1],
    {
      id: 'reward',
      type: 'reward',
      kind: 'reward',
      code: 'REWARD',
      title: '奖励计算',
      position: { x: 740, y: 380 },
      inputs: ['event_context'],
      outputs: ['reward_items'],
      config: { amount: 10, currency: 'USD' },
    },
    {
      id: 'quota',
      type: 'quota',
      kind: 'quota',
      code: 'QUOTA',
      title: '预算扣减',
      position: { x: 1070, y: 380 },
      inputs: ['reward_items'],
      outputs: ['quota_result'],
      config: { quotaCode: 'DEFAULT_REWARD_BUDGET' },
    },
  ],
  edges: [
    workflowDefinition.edges[0],
    {
      id: 'inbox-reward-default',
      source: 'inbox',
      target: 'reward',
      label: '默认路径',
      route: { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true },
    },
    {
      id: 'reward-quota-default',
      source: 'reward',
      target: 'quota',
      label: '默认路径',
      route: { type: 'DEFAULT', key: 'DEFAULT', label: '默认路径', default: true },
    },
  ],
  viewport: { x: 24, y: 36, zoom: 1 },
  meta: { activityId, workspaceId, folderId, visibility: 'TEAM' },
};

const workflowTemplateSummary = {
  id: 'reward-template',
  versionId: 'tplv_2',
  version: 2,
  name: '奖励预算模板',
  description: 'Event In · Inbox · Reward · Quota',
  category: '团队模板',
  source: 'SAVED',
  status: 'ACTIVE',
  definition: templateDefinition,
  createdBy: 'admin-flow',
  createdAt: '2026-06-02T09:00:00.000Z',
  updatedAt: '2026-06-02T10:00:00.000Z',
};

const workflowTemplateV1 = {
  ...workflowTemplateSummary,
  versionId: 'tplv_1',
  version: 1,
  name: '奖励预算模板 v1',
  updatedAt: '2026-06-02T08:00:00.000Z',
};

const schemaItems = [
  { nodeType: 'event-in', version: 1, title: 'Event In', fields: [], inputs: [], outputs: [{ key: 'raw_event', label: 'Raw Event', type: 'object', description: '标准化前的事件载荷。' }] },
  { nodeType: 'inbox', version: 1, title: 'Inbox', fields: [], inputs: [{ key: 'raw_event', label: 'Raw Event', type: 'object', required: true }], outputs: [{ key: 'event_context', label: 'Event Context', type: 'object', description: '幂等收件箱处理后的事件上下文。' }] },
  { nodeType: 'match', version: 1, title: 'Match', fields: [], inputs: [{ key: 'event_context', label: 'Event Context', type: 'object', required: true }], outputs: [{ key: 'matched_activity', label: 'Matched Activity', type: 'object' }] },
  { nodeType: 'condition', version: 1, title: 'Condition', fields: [], inputs: [{ key: 'fact', label: 'Fact', type: 'object' }], outputs: [{ key: 'IF', label: 'IF', type: 'boolean' }, { key: 'ELSE', label: 'ELSE', type: 'boolean' }] },
  { nodeType: 'reward', version: 1, title: 'Reward', fields: [], inputs: [], outputs: [] },
  { nodeType: 'quota', version: 1, title: 'Quota', fields: [], inputs: [], outputs: [] },
  { nodeType: 'grant-order', version: 1, title: 'Grant', fields: [], inputs: [], outputs: [] },
  { nodeType: 'outbox', version: 1, title: 'Outbox', fields: [], inputs: [], outputs: [] },
  { nodeType: 'callback', version: 1, title: 'Callback', fields: [], inputs: [], outputs: [] },
  { nodeType: 'audit', version: 1, title: 'Audit', fields: [], inputs: [], outputs: [] },
];

type StudioTestDefinition = typeof workflowDefinition | typeof conditionWorkflowDefinition;

async function mockStudioApi(page: Page, options: { failFirstDraftSave?: boolean; validationBlocks?: boolean; routeValidationBlocks?: boolean; draftDefinition?: StudioTestDefinition; activeDefinition?: StudioTestDefinition; archivedDefinition?: StudioTestDefinition; withExecution?: boolean; largeWorkspace?: boolean; failBatchMove?: boolean; failFirstStorageUpload?: boolean } = {}) {
  let draftSaveCount = 0;
  let storageUploadCount = 0;
  let published = Boolean(options.activeDefinition);
  let rolledBack = false;
  let archivedHidden = false;
  const runBaseMs = Date.parse('2026-06-02T10:00:00.000Z');
  const runTime = (offsetMs: number) => new Date(runBaseMs + offsetMs).toISOString();
  const storageFiles = [{
    id: 'wsfile_existing',
    workspaceId,
    folderId,
    kind: 'IMPORT_WORKFLOW',
    fileName: 'existing-import.json',
    contentType: 'application/json',
    sizeBytes: 2048,
    storageProvider: 'local',
    storageKey: `${workspaceId}/existing-import.json`,
    checksumSha256: 'sha256-existing',
    createdBy: 'admin-flow',
    createdAt: runTime(-60_000),
  }];
  await page.route('**/admin/v1/**', async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const json = (body: unknown, status = 200) => route.fulfill({
      status,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });

    if (path === '/admin/v1/workspace-home/storage' && method === 'GET') {
      const fileBytes = storageFiles.reduce((sum, file) => sum + file.sizeBytes, 0);
      return json({ usedBytes: fileBytes + 1024 * 1024, totalBytes: 100 * 1024 * 1024 * 1024, workflows: 2, folders: 2, members: 1, auditLogs: 0, versions: 1, executions: 0, fileObjects: storageFiles.length, fileBytes });
    }
    if (path === '/admin/v1/workspace-files' && method === 'GET') {
      return json({ items: storageFiles });
    }
    if (path === '/admin/v1/workspace-files' && method === 'POST') {
      storageUploadCount++;
      if (options.failFirstStorageUpload && storageUploadCount === 1) {
        return json({ error: 'storage temporarily unavailable' }, 500);
      }
      const uploaded = {
        id: `wsfile_uploaded_${storageUploadCount}`,
        workspaceId,
        folderId,
        kind: 'EXECUTION_ATTACHMENT',
        fileName: 'retry-upload.json',
        contentType: 'application/json',
        sizeBytes: 18,
        storageProvider: 'local',
        storageKey: `${workspaceId}/retry-upload.json`,
        checksumSha256: 'sha256-uploaded',
        createdBy: 'admin-flow',
        createdAt: runTime(0),
      };
      storageFiles.unshift(uploaded);
      return json(uploaded, 201);
    }
    if (path.startsWith('/admin/v1/workspace-files/') && method === 'DELETE') {
      const id = path.split('/').pop() ?? '';
      const index = storageFiles.findIndex((file) => file.id === id);
      const [deleted] = index >= 0 ? storageFiles.splice(index, 1) : [];
      return json(deleted ?? { id, workspaceId, fileName: 'deleted.json', sizeBytes: 0, storageProvider: 'local', storageKey: '', checksumSha256: '', kind: 'IMPORT_WORKFLOW', createdAt: runTime(0) });
    }

    if (path === '/admin/v1/activities' && method === 'GET') {
      if (options.largeWorkspace) {
        return json({
          items: Array.from({ length: 1200 }).map((_, index) => ({
            id: `large_activity_${index}`,
            workspaceId,
            folderId,
            activityCode: `LARGE_FLOW_${index}`,
            activityName: index === 42 ? 'Large Retention Highlight Workflow' : `Large Workflow ${index}`,
            tenantId: 'site_001',
            status: index % 7 === 0 ? 'DRAFT' : 'ACTIVE',
            visibility: index % 5 === 0 ? 'PRIVATE' : 'TEAM',
            version: (index % 6) + 1,
            createdAt: runTime(-8_000_000 - index * 1000),
            updatedAt: runTime(-500_000 - index * 1000),
          })),
        });
      }
      return json({
        items: [{
          id: activityId,
          workspaceId,
          folderId,
          activityCode: 'E2E_FLOW',
          activityName: 'E2E Workflow',
          tenantId: 'site_001',
          status: 'ACTIVE',
          version: 1,
          createdAt: runTime(-4_000_000),
          updatedAt: runTime(-2_000_000),
        }, {
          id: secondaryActivityId,
          workspaceId,
          folderId,
          activityCode: 'RETENTION_FLOW',
          activityName: 'Retention Workflow',
          tenantId: 'site_001',
          status: 'ACTIVE',
          version: 3,
          createdAt: runTime(-8_000_000),
          updatedAt: runTime(-500_000),
        }],
      });
    }
    if (path === '/admin/v1/workspaces' && method === 'GET') {
      return json({
        items: [{
          id: workspaceId,
          tenantId: 'site_001',
          name: 'E2E Workspace',
          slug: 'e2e',
          visibility: 'TEAM',
          defaultRole: 'VIEWER',
          ownerId: 'admin-flow',
          status: 'ACTIVE',
        }],
      });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/folders` && method === 'GET') {
      if (options.largeWorkspace) {
        return json({
          items: Array.from({ length: 80 }).map((_, index) => ({
            id: index === 0 ? folderId : `large_folder_${index}`,
            workspaceId,
            name: index === 0 ? 'Campaigns' : `Large Folder ${index}`,
            visibility: 'TEAM',
            sortOrder: (index + 1) * 10,
            status: 'ACTIVE',
          })),
        });
      }
      return json({
        items: [
          { id: folderId, workspaceId, name: 'Campaigns', visibility: 'TEAM', sortOrder: 10, status: 'ACTIVE' },
          { id: secondaryFolderId, workspaceId, name: 'Archive', visibility: 'TEAM', sortOrder: 20, status: 'ACTIVE' },
        ],
      });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/folders/${folderId}` && method === 'PUT') {
      const body = await request.postDataJSON();
      return json({ id: folderId, workspaceId, name: 'Campaigns', visibility: body.visibility || 'TEAM', parentId: body.parentId || '', sortOrder: body.sortOrder ?? 10, status: 'ACTIVE' });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/folders/${secondaryFolderId}` && method === 'PUT') {
      const body = await request.postDataJSON();
      return json({ id: secondaryFolderId, workspaceId, name: 'Archive', visibility: body.visibility || 'TEAM', parentId: body.parentId || '', sortOrder: body.sortOrder ?? 20, status: 'ACTIVE' });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/batch` && method === 'POST') {
      if (options.failBatchMove) {
        return json({
          committed: false,
          items: [
            { id: secondaryActivityId, type: 'WORKFLOW', status: 'FAILED', error: 'workflow is archived' },
          ],
        }, 400);
      }
      return json({ committed: true, count: 2, items: [] });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/share-links` && method === 'GET') return json({
      items: [{
        id: 'share_existing',
        workspaceId,
        scope: 'WORKSPACE',
        permission: 'VIEW',
        token: 'share_token_full_visibility_regression_1234567890',
        enabled: true,
        accessCount: 2,
      }],
    });
    if (path === `/admin/v1/workspaces/${workspaceId}/members` && method === 'GET') {
      return json({ items: [{ id: 'member_owner', workspaceId, userId: 'admin-flow', displayName: 'Admin Flow', role: 'OWNER', status: 'ACTIVE' }] });
    }
    if (path === `/admin/v1/workspaces/${workspaceId}/audit-logs` && method === 'GET') return json({ items: [] });
    if (path === '/admin/v1/workspace-home/search' && method === 'GET') {
      return json({
        items: [{
          id: 'large_activity_42',
          type: 'workflow',
          title: 'Large Retention Highlight Workflow',
          subtitle: 'LARGE_FLOW_42 · ACTIVE',
          workspaceId,
          folderId,
          matchedBy: 'workflow',
          highlights: { title: 'Large <mark>Retention</mark> Highlight Workflow', subtitle: 'LARGE_FLOW_42 · ACTIVE' },
          permission: 'VIEW',
          permissionReason: 'workspace role OWNER',
          permissionSources: ['workspace OWNER'],
        }],
      });
    }
    if (path === '/admin/v1/workflow/node-config-schemas' && method === 'GET') return json({ items: schemaItems });
    if (path === '/admin/v1/workflow/draft-templates' && method === 'GET') return json({ items: [workflowTemplateSummary] });
    if (path === '/admin/v1/workflow/draft-templates/reward-template/versions' && method === 'GET') return json({ items: [workflowTemplateSummary, workflowTemplateV1] });
    if (path === '/admin/v1/workflow/node-presets' && method === 'GET') {
      return json({ items: [{ id: 'reward-fixed-usd', nodeType: 'reward', name: '固定美元奖励', category: 'reward', config: { amount: 10, currency: 'USD' } }] });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/draft` && method === 'GET') return json({ activityId, definition: options.draftDefinition ?? workflowDefinition });
    if (path === `/admin/v1/activities/${activityId}/workflow/draft` && method === 'PUT') {
      draftSaveCount += 1;
      if (options.failFirstDraftSave && draftSaveCount === 1) return json({ error: 'E2E save failure' }, 500);
      return json({ activityId, definition: workflowDefinition });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/draft-templates/reward-template/apply` && method === 'POST') {
      const body = await request.postDataJSON();
      return json({
        activityId,
        definition: {
          ...templateDefinition,
          nodes: templateDefinition.nodes.map((node) => {
            if (node.type === 'reward') return { ...node, config: { ...node.config, amount: body.parameters?.rewardAmount ?? 10, currency: body.parameters?.currency ?? 'USD' } };
            if (node.type === 'quota') return { ...node, config: { ...node.config, quotaCode: body.parameters?.quotaCode ?? 'DEFAULT_REWARD_BUDGET' } };
            if (node.type === 'event-in') return { ...node, config: { eventTypes: [body.parameters?.eventType ?? 'DEPOSIT_SUCCESS'] } };
            return node;
          }),
          meta: { activityId, workspaceId: body.workspaceId, folderId: body.folderId, templateParameters: body.parameters },
        },
      });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/draft-template` && method === 'POST') {
      const body = await request.postDataJSON();
      return json({
        ...workflowTemplateSummary,
        id: 'saved-team-template',
        versionId: 'saved_tplv_1',
        version: 1,
        name: body.name,
        definition: {
          ...workflowDefinition,
          meta: { activityId, workspaceId: body.workspaceId, folderId: body.folderId, visibility: body.visibility },
        },
      }, 201);
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/versions` && method === 'GET') {
      return json({
        items: published ? [{
          id: 'version_1',
          tenantId: 'site_001',
          activityId,
          version: rolledBack ? 2 : 1,
          status: 'ACTIVE',
          definition: rolledBack ? (options.archivedDefinition ?? workflowDefinition) : (options.activeDefinition ?? workflowDefinition),
          publishedAt: new Date().toISOString(),
          publishedBy: 'admin-flow',
          releaseNote: rolledBack ? 'Rollback to workflow v0' : '当前线上版本',
          action: rolledBack ? 'ROLLBACK' : 'PUBLISH',
          sourceVersionId: rolledBack ? 'version_0' : '',
        }, ...(options.archivedDefinition && !archivedHidden ? [
        {
          id: 'version_super',
          tenantId: 'site_001',
          activityId,
          version: -1,
          status: 'SUPERSEDED',
          definition: workflowDefinition,
          publishedAt: runTime(-1_800_000),
          publishedBy: 'admin-flow',
          releaseNote: '被新版本替代',
          action: 'PUBLISH',
        },
        {
          id: 'version_0',
          tenantId: 'site_001',
          activityId,
          version: 0,
          status: 'ARCHIVED',
          definition: options.archivedDefinition,
          publishedAt: runTime(-3_600_000),
          publishedBy: 'admin-flow',
          releaseNote: '旧版备用',
          action: 'PUBLISH',
        }] : [])] : [],
      });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/draft/validate` && method === 'POST') {
      if (options.routeValidationBlocks) {
        return json({
          valid: false,
          items: [
            { severity: 'BLOCK', code: 'WORKFLOW_BRANCH_DEFAULT_MISSING', message: '分支节点 "condition" 必须显式配置默认出口。', nodeId: 'condition', path: 'nodes[1].route.default' },
            { severity: 'BLOCK', code: 'WORKFLOW_EDGE_ROUTE_CONDITION_REQUIRED', message: '连线 "condition-if" 的条件路由必须配置 expression 或 conditionRef。', nodeId: 'condition', edgeId: 'condition-if', path: 'edges[1].route' },
          ],
          summary: { blockCount: 2, warningCount: 0 },
        });
      }
      if (options.validationBlocks) {
        return json({
          valid: false,
          items: [
            { severity: 'BLOCK', code: 'MISSING_CONFIG', message: 'Inbox 缺少必填配置', nodeId: 'inbox', path: '$.nodes.inbox.config' },
            { severity: 'WARNING', code: 'EDGE_ROUTE_REVIEW', message: '默认路径需要确认', edgeId: 'event-in-inbox-default', path: '$.edges[0].route' },
          ],
          summary: { blockCount: 1, warningCount: 1 },
        });
      }
      return json({ valid: true, items: [], summary: { blockCount: 0, warningCount: 0 } });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/publish` && method === 'POST') {
      published = true;
      const body = JSON.parse(request.postData() || '{}');
      return json({ id: 'version_1', tenantId: 'site_001', activityId, version: 1, status: 'ACTIVE', definition: workflowDefinition, publishedAt: new Date().toISOString(), releaseNote: body.releaseNote, action: 'PUBLISH' });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/debug-run` && method === 'POST') {
      return json({
        schemaVersion: 'workflow.execution.trace.v1',
        exportedAt: runTime(700),
        execution: {
          id: 'debug_execution_1',
          tenantId: 'site_001',
          activityId,
          workflowVersionId: 'debug-draft',
          workflowVersion: 0,
          status: 'SUCCESS',
          eventId: 'evt_1001',
          input: {
            event: {
              eventId: 'evt_1001',
              eventType: 'DEPOSIT_SUCCESS',
              sourceSystem: 'studio-debug',
              tenantId: 'site_001',
              subject: { type: 'USER', id: 'user_1' },
              biz: { bizType: 'DEBUG', bizId: 'debug_biz' },
              occurredAt: runTime(0),
              attributes: { amount: '200.00' },
            },
            mode: 'DEBUG_DRAFT',
            debug: true,
          },
          output: { mode: 'DEBUG_DRAFT', debug: true, executed: 1, skipped: 2, routeTrace: [] },
          startedAt: runTime(600),
          finishedAt: runTime(680),
        },
        nodeExecutions: [{
          id: 'debug_node_exec_inbox',
          executionId: 'debug_execution_1',
          nodeId: 'inbox',
          nodeType: 'inbox',
          nodeTitle: '幂等收件箱',
          status: 'SUCCESS',
          input: { raw_event: { userId: 'user_1', eventId: 'evt_1001' } },
          output: { event_context: { userId: 'user_1', debug: true } },
          startedAt: runTime(610),
          finishedAt: runTime(660),
        }],
        routeTrace: [],
      }, 201);
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/versions/version_0/rollback` && method === 'POST') {
      rolledBack = true;
      published = true;
      const body = JSON.parse(request.postData() || '{}');
      return json({ id: 'version_2', tenantId: 'site_001', activityId, version: 2, status: 'ACTIVE', definition: options.archivedDefinition ?? workflowDefinition, publishedAt: new Date().toISOString(), releaseNote: body.note, sourceVersionId: 'version_0', action: 'ROLLBACK' });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/versions/version_0/archive` && method === 'POST') {
      archivedHidden = true;
      return json({ id: 'version_0', tenantId: 'site_001', activityId, version: 0, status: 'ARCHIVED', definition: options.archivedDefinition ?? workflowDefinition, publishedAt: new Date().toISOString() });
    }
    if (path === `/admin/v1/activities/${activityId}/workflow/executions` && method === 'GET') {
      if (!options.withExecution) return json({ items: [] });
      return json({
        items: [
          {
            id: 'execution_failed',
            tenantId: 'site_001',
            activityId,
            workflowVersionId: 'version_1',
            workflowVersion: 1,
            status: 'FAILED',
            eventId: 'evt_1001',
            idempotencyKey: 'idem_1001',
            triggerPayload: { eventId: 'evt_1001' },
            input: { eventId: 'evt_1001' },
            output: {
              routeTrace: [
                {
                  edgeId: 'event-in-inbox-default',
                  source: 'event-in',
                  target: 'inbox',
                  routeType: 'DEFAULT',
                  routeLabel: '默认路径',
                  default: true,
                },
              ],
            },
            errorMessage: 'Inbox write failed',
            startedAt: runTime(0),
            finishedAt: runTime(300),
          },
          {
            id: 'execution_success',
            tenantId: 'site_001',
            activityId,
            workflowVersionId: 'version_1',
            workflowVersion: 1,
            status: 'SUCCESS',
            eventId: 'evt_1000',
            input: { eventId: 'evt_1000' },
            output: {
              routeTrace: [
                {
                  edgeId: 'event-in-inbox-default',
                  source: 'event-in',
                  target: 'inbox',
                  routeType: 'DEFAULT',
                  routeLabel: '默认路径',
                  default: true,
                },
                {
                  edgeId: 'inbox-match-default',
                  source: 'inbox',
                  target: 'match',
                  routeType: 'DEFAULT',
                  routeLabel: '默认路径',
                  default: true,
                },
              ],
            },
            startedAt: runTime(-2000),
            finishedAt: runTime(-1600),
          },
        ],
        limit: Number(url.searchParams.get('limit') || 20),
        offset: Number(url.searchParams.get('offset') || 0),
        nextOffset: 2,
        hasMore: false,
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_failed/nodes/node_exec_event_in' && method === 'GET') {
      return json({
        id: 'node_exec_event_in',
        executionId: 'execution_failed',
        nodeId: 'event-in',
        nodeType: 'event-in',
        nodeTitle: '事件入口',
        status: 'SUCCESS',
        input: { eventId: 'evt_1001' },
        output: { raw_event: { userId: 'user_1', eventId: 'evt_1001' } },
        startedAt: runTime(0),
        finishedAt: runTime(100),
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_failed/nodes/node_exec_inbox' && method === 'GET') {
      return json({
        id: 'node_exec_inbox',
        executionId: 'execution_failed',
        nodeId: 'inbox',
        nodeType: 'inbox',
        nodeTitle: '幂等收件箱',
        status: 'FAILED',
        input: { raw_event: { userId: 'user_1', eventId: 'evt_1001' } },
        output: { attempts: 2, errorCode: 'INBOX_WRITE_FAILED' },
        errorMessage: 'INBOX_WRITE_FAILED: Inbox write failed',
        startedAt: runTime(120),
        finishedAt: runTime(300),
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_failed/nodes' && method === 'GET') {
      const summary = url.searchParams.get('payload') === 'summary';
      return json({
        items: [
          {
            id: 'node_exec_event_in',
            executionId: 'execution_failed',
            nodeId: 'event-in',
            nodeType: 'event-in',
            nodeTitle: '事件入口',
            status: 'SUCCESS',
            ...(summary ? {} : {
              input: { eventId: 'evt_1001' },
              output: { raw_event: { userId: 'user_1', eventId: 'evt_1001' } },
            }),
            startedAt: runTime(0),
            finishedAt: runTime(100),
          },
          {
            id: 'node_exec_inbox',
            executionId: 'execution_failed',
            nodeId: 'inbox',
            nodeType: 'inbox',
            nodeTitle: '幂等收件箱',
            status: 'FAILED',
            ...(summary ? {} : {
              input: { raw_event: { userId: 'user_1', eventId: 'evt_1001' } },
              output: { attempts: 2, errorCode: 'INBOX_WRITE_FAILED' },
            }),
            errorMessage: 'INBOX_WRITE_FAILED: Inbox write failed',
            startedAt: runTime(120),
            finishedAt: runTime(300),
          },
        ],
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_failed/trace' && method === 'GET') {
      return json({
        schemaVersion: 'workflow.execution.trace.v1',
          exportedAt: runTime(500),
        execution: {
          id: 'execution_failed',
          tenantId: 'site_001',
          activityId,
          workflowVersionId: 'version_1',
          workflowVersion: 1,
          status: 'FAILED',
          eventId: 'evt_1001',
          startedAt: runTime(0),
          finishedAt: runTime(300),
        },
        nodeExecutions: [],
        routeTrace: [],
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_failed/compare' && method === 'GET') {
      if (url.searchParams.get('baselineExecutionId') !== 'execution_success') return json({ error: 'missing baseline' }, 404);
      return json({
        current: {
          id: 'execution_failed',
          status: 'FAILED',
          eventId: 'evt_1001',
          workflowVersion: 1,
          errorMessage: 'Inbox write failed',
          startedAt: runTime(0),
          finishedAt: runTime(300),
        },
        baseline: {
          id: 'execution_success',
          status: 'SUCCESS',
          eventId: 'evt_1000',
          workflowVersion: 1,
          startedAt: runTime(-2000),
          finishedAt: runTime(-1600),
        },
        currentPath: ['event-in', 'inbox'],
        baselinePath: ['event-in', 'inbox', 'match'],
        pathChanged: true,
        currentFailures: ['幂等收件箱'],
        baselineFailures: [],
        currentDurationMs: 280,
        baselineDurationMs: 300,
        durationDeltaMs: -20,
        payloadChanges: [{
          nodeId: 'inbox',
          label: '幂等收件箱',
          input: [
            { path: '$.raw_event.userId', type: 'changed', current: 'user_1', baseline: 'user_0' },
            { path: '$.raw_event.eventId', type: 'changed', current: 'evt_1001', baseline: 'evt_1000' },
          ],
          output: [
            { path: '$.errorCode', type: 'added', current: 'INBOX_WRITE_FAILED' },
            { path: '$.event_context', type: 'removed', baseline: { userId: 'user_0', accepted: true } },
          ],
        }],
      });
    }
    if (path === '/admin/v1/workflow/executions/execution_success/nodes' && method === 'GET') {
      return json({
        items: [
          {
            id: 'node_exec_success_event_in',
            executionId: 'execution_success',
            nodeId: 'event-in',
            nodeType: 'event-in',
            nodeTitle: '事件入口',
            status: 'SUCCESS',
            input: { eventId: 'evt_1000' },
            output: { raw_event: { userId: 'user_0', eventId: 'evt_1000' } },
            startedAt: runTime(-2000),
            finishedAt: runTime(-1920),
          },
          {
            id: 'node_exec_success_inbox',
            executionId: 'execution_success',
            nodeId: 'inbox',
            nodeType: 'inbox',
            nodeTitle: '幂等收件箱',
            status: 'SUCCESS',
            input: { raw_event: { userId: 'user_0', eventId: 'evt_1000' } },
            output: { event_context: { userId: 'user_0', accepted: true } },
            startedAt: runTime(-1910),
            finishedAt: runTime(-1800),
          },
          {
            id: 'node_exec_success_match',
            executionId: 'execution_success',
            nodeId: 'match',
            nodeType: 'match',
            nodeTitle: '活动匹配',
            status: 'SUCCESS',
            input: { event_context: { userId: 'user_0', accepted: true } },
            output: { matched_activity: { id: activityId } },
            startedAt: runTime(-1790),
            finishedAt: runTime(-1680),
          },
        ],
      });
    }
    return json({ error: `unhandled ${method} ${path}` }, 404);
  });
}

test('renders Workspace Studio home as a file manager with search, favorites, and detail actions', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await expect(page.locator('.exm-sidebar')).toContainText('E2E Workspace');
  await expect(page.locator('.exm-sidebar-folders')).toContainText('Campaigns');
  await expect(page.locator('.exm-sidebar-folders')).toContainText('Archive');
  await expect(page.locator('.exm-table-view')).toContainText('Retention Workflow');
  await expect(page.locator('.exm-detail-panel')).toContainText('Campaigns');
  const workbenchOverview = page.getByTestId('workspace-workbench-overview');
  await expect(workbenchOverview).toContainText('Campaigns 工作台');
  await expect(workbenchOverview).toContainText('最近编辑');
  await expect(workbenchOverview).toContainText('收藏');
  await expect(workbenchOverview).toContainText('归档');

  await workbenchOverview.locator('.exm-workbench-quick-actions button').filter({ hasText: '最近' }).click();
  await expect(page.locator('.exm-sidebar-nav button.active')).toContainText('最近编辑');

  await page.locator('.exm-workspace-switcher-trigger').click();
  await page.locator('.exm-workspace-switcher-search input').fill('missing');
  await expect(page.locator('.exm-workspace-switcher-empty')).toContainText('没有匹配的 workspace');
  await page.locator('.exm-workspace-switcher-search input').fill('E2E');
  await expect(page.locator('.exm-workspace-switcher-menu')).toContainText('E2E Workspace');
  await page.keyboard.press('Escape');

  await page.locator('.exm-folder-search input').fill('Retention');
  await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' })).toBeVisible();

  await page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' }).locator('.actions em').first().click();
  await page.locator('.exm-sidebar-nav button').filter({ hasText: '收藏' }).click();
  await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' })).toBeVisible();

  const retentionRow = page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' });
  await retentionRow.locator('[data-testid="workspace-row-more"]').click();
  await expect(page.getByTestId('workspace-row-action-rename')).toBeVisible();

  const detailToggle = page.getByRole('button', { name: '打开详情' });
  if (await detailToggle.isVisible()) await detailToggle.click();
  await page.locator('.exm-detail-tabs').getByRole('button', { name: '分享' }).click();
  await page.getByRole('button', { name: '管理分享' }).click();
  const shareDialog = page.getByTestId('home-utility-dialog');
  await expect(shareDialog).toContainText('tok_shar…7890');
  await expect(shareDialog).not.toContainText('share_token_full_visibility_regression_1234567890');
});

test('keeps workspace responsive with large mocked datasets and highlighted search', async ({ page }) => {
  await mockStudioApi(page, { largeWorkspace: true });

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await expect(page.locator('[data-testid="workspace-row-workflow"]').first()).toBeVisible();
  await page.locator('.exm-folder-search input').fill('Retention');
  await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Large Retention Highlight Workflow' })).toBeVisible();
  await expect(page.locator('.exm-table-view mark')).toContainText('Retention');
  await page.locator('.exm-global-search input').fill('Retention');
  await expect(page.locator('.exm-global-search-results mark')).toContainText('Retention');
});

test('shows contextual recommendations for empty workspace search results', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await page.locator('.exm-folder-search input').fill('NoMatchRisk2026');
  await page.locator('.exm-content-tools').getByRole('button', { name: '筛选' }).click();
  await page.locator('.exm-advanced-filters select').first().selectOption('Workflow');

  const emptyState = page.locator('.exm-content-empty.search');
  await expect(emptyState).toBeVisible();
  await expect(emptyState).toContainText('没有找到 "NoMatchRisk2026"');
  await expect(emptyState).toContainText('当前搜索和筛选没有匹配项。');
  await expect(emptyState.getByRole('button', { name: '清除筛选' })).toBeVisible();
  await expect(emptyState.getByRole('button', { name: '新建流程' })).toBeVisible();
  await expect(emptyState.getByRole('button', { name: '查看归档' })).toBeVisible();

  await emptyState.getByRole('button', { name: '查看归档' }).click();
  await expect(page.locator('.exm-content-empty.archived')).toContainText('归档为空');
});

test('keeps the detail panel as a drawer at 1280px so Template content is not squeezed', async ({ page }) => {
  await mockStudioApi(page);
  await page.setViewportSize({ width: 1280, height: 820 });

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await page.getByRole('button', { name: '模板' }).click();
  await expect(page.locator('.exm-grid-view button').first()).toBeVisible();

  const detailToggle = page.getByRole('button', { name: '打开详情' });
  await expect(detailToggle).toBeVisible();
  await expect(page.locator('.exm-workspace-layout.detail-open')).toHaveCount(0);

  const metrics = await page.locator('.exm-workspace-main').evaluate((element) => ({
    mainClientWidth: element.clientWidth,
    mainScrollWidth: element.scrollWidth,
    gridClientWidth: element.querySelector<HTMLElement>('.exm-grid-view')?.clientWidth ?? 0,
    gridScrollWidth: element.querySelector<HTMLElement>('.exm-grid-view')?.scrollWidth ?? 0,
    columns: window.getComputedStyle(document.querySelector<HTMLElement>('.exm-workspace-layout')!).gridTemplateColumns,
  }));
  expect(metrics.columns.trim().split(/\s+/)).toHaveLength(2);
  expect(metrics.mainScrollWidth).toBeLessThanOrEqual(metrics.mainClientWidth + 2);
  expect(metrics.gridScrollWidth).toBeLessThanOrEqual(metrics.gridClientWidth + 2);

  await detailToggle.click();
  await expect(page.locator('.exm-workspace-layout.detail-open')).toBeVisible();
  await expect(page.locator('.exm-mobile-detail-toggle')).toHaveAttribute('aria-label', '关闭详情');
});

test('renders mobile workspace rows as cards without horizontal table scroll', async ({ page }) => {
  await mockStudioApi(page);
  await page.setViewportSize({ width: 390, height: 844 });

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await expect(page.locator('.exm-table-row.head')).toBeHidden();
  const firstWorkflow = page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' });
  const firstWorkflowCells = firstWorkflow.locator(':scope > span');
  await expect(firstWorkflow).toBeVisible();
  await expect(firstWorkflow.locator('.name-cell')).toBeVisible();
  await expect(firstWorkflowCells.nth(2)).toHaveText('流程');
  await expect(firstWorkflowCells.nth(7)).toContainText('工作区');
  const typeLabel = await firstWorkflowCells.nth(2).evaluate((element) => window.getComputedStyle(element, '::before').content);
  expect(typeLabel).toContain('类型');

  const metrics = await page.locator('.exm-content').evaluate((element) => ({
    contentClientWidth: element.clientWidth,
    contentScrollWidth: element.scrollWidth,
    tableClientWidth: element.querySelector<HTMLElement>('.exm-table-view')?.clientWidth ?? 0,
    tableScrollWidth: element.querySelector<HTMLElement>('.exm-table-view')?.scrollWidth ?? 0,
  }));
  expect(metrics.contentScrollWidth).toBeLessThanOrEqual(metrics.contentClientWidth + 2);
  expect(metrics.tableScrollWidth).toBeLessThanOrEqual(metrics.tableClientWidth + 2);
});

test('shows per-item failure reasons when a batch operation rolls back', async ({ page }) => {
  await mockStudioApi(page, { failBatchMove: true });

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  const firstWorkflow = page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: 'Retention Workflow' });
  await expect(firstWorkflow).toBeVisible();
  await firstWorkflow.getByTestId('workspace-row-checkbox').click({ force: true });
  await page.getByTestId('workspace-bulk-move').click();
  await page.getByTestId('batch-target-folder').selectOption(secondaryFolderId);
  await page.getByTestId('batch-confirm').click();

  const failures = page.getByTestId('batch-failures');
  await expect(failures).toBeVisible();
  await expect(failures).toContainText('Retention Workflow');
  await expect(failures).toContainText('workflow is archived');
});

test('manages workspace files with upload retry and file list', async ({ page }) => {
  await mockStudioApi(page, { failFirstStorageUpload: true });

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  await page.getByRole('button', { name: '管理存储空间' }).click();
  const fileList = page.getByTestId('storage-file-list');
  await expect(fileList).toContainText('existing-import.json');
  await expect(page.getByText('单文件上限 64.0 MB')).toBeVisible();

  const uploadInput = page.locator('[data-testid="home-utility-dialog"] input[type="file"]');
  await uploadInput.setInputFiles({
    name: 'retry-upload.json',
    mimeType: 'application/json',
    buffer: Buffer.from('{"ok":true}'),
  });
  await expect(page.getByTestId('storage-upload-status')).toContainText('storage temporarily unavailable');
  await page.getByTestId('storage-upload-retry').click();
  await expect(page.getByTestId('storage-upload-status')).toContainText('上传完成');
  await expect(fileList).toContainText('retry-upload.json');
});

test('passes a basic workspace accessibility smoke check', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto('/admin-flow/');
  await expect(page.locator('.exm-workspace-layout')).toBeVisible();
  const unnamedButtons = await page.locator('.exm-workspace-layout button').evaluateAll((buttons) => buttons.filter((button) => {
    const style = window.getComputedStyle(button);
    const rect = button.getBoundingClientRect();
    if (style.display === 'none' || style.visibility === 'hidden' || rect.width === 0 || rect.height === 0) return false;
    const label = button.getAttribute('aria-label') || button.textContent || button.getAttribute('title') || '';
    return !label.trim();
  }).length);
  expect(unnamedButtons).toBe(0);
  await page.keyboard.press('Tab');
  await expect(page.locator(':focus')).toBeVisible();
});

test('loads Studio and shows visible checklist validation markers', async ({ page }) => {
  await mockStudioApi(page, { validationBlocks: true });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await expect(page.locator('.workflow-breadcrumb')).toContainText('E2E Workspace');
  await expect(page.locator('.workflow-breadcrumb')).toContainText('Campaigns');
  await expect(page.locator('.workflow-state-pill').filter({ hasText: '已保存' })).toBeVisible();

  await page.getByRole('button', { name: '检查清单' }).click();
  await expect(page.locator('.checklist-modal-layer')).toBeVisible();
  await expect(page.locator('.checklist-body')).toBeVisible();
  await expect(page.locator('.checklist-summary-row').first()).toBeVisible();
  await expect(page.locator('.checklist-summary-row')).not.toHaveCount(0);
  await expect(page.locator('.checklist-popover')).toContainText('MISSING_CONFIG');
  await expect(page.locator('.node-validation-dot.block')).toBeVisible();
  await expect(page.locator('.react-flow__edge.validation-warning')).toBeVisible();
});

test('selects upstream output variables and locates source node', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);

  await page.locator('.activity-node').filter({ hasText: '活动匹配' }).click();
  await expect(page.locator('.property-panel')).toContainText('输入映射');

  await page.locator('.variable-picker button').filter({ hasText: 'event_context' }).click();
  await expect(page.locator('.input-map-reference')).toContainText('来源节点');
  await expect(page.locator('.input-map-reference')).toContainText('幂等收件箱');
  await expect(page.locator('.input-map-reference')).toContainText('Output Key');
  await expect(page.locator('.input-map-reference')).toContainText('event_context');
  await expect(page.locator('.input-map-reference')).toContainText('类型');
  await expect(page.locator('.input-map-reference')).toContainText('object');
  await expect(page.locator('.input-map-reference')).toContainText('JSONPath');
  await expect(page.locator('.input-map-reference')).toContainText('$');

  await page.getByPlaceholder('$ 或 $.user.id').fill('$.subject.id');
  await expect(page.locator('.input-map-reference')).toContainText('$.subject.id');

  await page.getByRole('button', { name: '定位来源' }).click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位变量来源：inbox/);
});

test('edits route rules and shows branch route coverage on canvas', async ({ page }) => {
  await mockStudioApi(page, { draftDefinition: conditionWorkflowDefinition, routeValidationBlocks: true });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);

  await page.locator('.react-flow__edge').filter({ hasText: 'IF' }).click({ force: true });
  await expect(page.locator('.property-panel')).toContainText('路由配置');
  await expect(page.locator('.route-type-panel')).toContainText('默认路径');
  await expect(page.locator('.route-type-panel')).toContainText('条件分支');
  await expect(page.locator('.route-type-panel')).toContainText('业务结果');
  await expect(page.locator('.route-type-panel')).toContainText('终止/阻断');

  await page.locator('.route-type-group button').filter({ hasText: 'PASS' }).click();
  await expect(page.locator('.route-type-current')).toContainText('PASS');
  await expect(page.getByPlaceholder('例如 decision == "PASS" 或 $.fact.amount >= 100')).toHaveValue('decision == "PASS"');

  await page.getByRole('button', { name: '检查清单' }).click();
  await expect(page.locator('.branch-route-check')).toContainText('Route Check');
  await expect(page.locator('.branch-route-check')).toContainText('缺默认分支');
  await expect(page.locator('.branch-route-check')).toContainText('条件路由必须配置');
  await expect(page.locator('.react-flow__edge.validation-block')).toBeVisible();
});

test('opens publish confirmation with diff summary before publishing', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);

  await page.getByRole('button', { name: '发布' }).click();
  await expect(page.getByRole('dialog', { name: '发布流程' })).toBeVisible();
  await expect(page.getByRole('dialog', { name: '发布流程' })).toContainText('当前没有线上版本');
  await expect(page.getByRole('dialog', { name: '发布流程' })).toContainText('暂无线上 active version');
  await page.getByPlaceholder('说明本次发布变更、风险或回滚依据').fill('首次发布：完整链路');

  await page.getByRole('button', { name: '确认发布 v1' }).click();
  await expect(page.locator('.workflow-state-pill').filter({ hasText: '已发布' })).toBeVisible();
  await expect(page.locator('.workflow-state-pill').filter({ hasText: 'v2' })).toBeVisible();
});

test('previews workflow templates, applies business parameters, and saves a team template', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await page.getByRole('button', { name: '模板' }).click();

  await expect(page.locator('.workflow-template-panel')).toBeVisible();
  await expect(page.locator('.workflow-template-preview-card')).toContainText('奖励预算模板');
  await expect(page.locator('.workflow-template-preview-card')).toContainText('可应用');
  await expect(page.locator('.workflow-template-node-preview')).toContainText('奖励计算');

  await page.locator('.workflow-template-preview > label').filter({ hasText: '版本' }).locator('select').selectOption('tplv_1');
  await page.locator('.workflow-template-params label').filter({ hasText: '奖励金额' }).locator('input').fill('25');
  await page.locator('.workflow-template-params label').filter({ hasText: '币种' }).locator('input').fill('EUR');
  await page.getByRole('button', { name: '从模板创建 Draft' }).click();
  await expect(page.locator('.workflow-template-panel')).toContainText('模板已应用');
  await expect(page.locator('.activity-node')).toHaveCount(4);

  await page.locator('.workflow-template-save input[placeholder="模板名称"]').fill('团队奖励模板');
  await page.locator('.workflow-template-save label').filter({ hasText: '可见性' }).locator('select').selectOption('PUBLIC');
  await page.getByRole('button', { name: '保存模板版本' }).click();
  await expect(page.locator('.workflow-template-panel')).toContainText('已保存模板 团队奖励模板 v1');
});

test('shows visual draft vs active diff and locates diff items', async ({ page }) => {
  await mockStudioApi(page, {
    activeDefinition: {
      ...workflowDefinition,
      nodes: [workflowDefinition.nodes[0]],
      edges: [],
      meta: { activityId, workspaceId, folderId },
    },
  });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await expect(page.locator('.react-flow__node.diff-node-added')).toHaveCount(2);

  await page.getByRole('button', { name: '版本' }).click();
  await expect(page.locator('.version-popover')).toContainText('Diff 明细');
  await expect(page.locator('.workflow-diff-row.added')).toHaveCount(4);

  await page.locator('.workflow-diff-row.added').filter({ hasText: '幂等收件箱' }).click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位 diff 节点/);
});

test('confirms rollback with version diff before executing rollback', async ({ page }) => {
  await mockStudioApi(page, {
    activeDefinition: workflowDefinition,
    archivedDefinition: {
      ...workflowDefinition,
      nodes: [workflowDefinition.nodes[0]],
      edges: [],
      meta: { activityId, workspaceId, folderId },
    },
  });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.workflow-state-pill').filter({ hasText: 'v2' })).toBeVisible();

  await page.getByRole('button', { name: '版本' }).click();
  await expect(page.locator('.version-popover')).toContainText('流程 v0');
  await page.locator('.version-card').filter({ hasText: '流程 v0' }).getByRole('button', { name: '回滚' }).click();

  await expect(page.getByRole('dialog', { name: '回滚流程' })).toBeVisible();
  await expect(page.getByRole('dialog', { name: '回滚流程' })).toContainText('回滚会生成新的 active version');
  await expect(page.getByRole('dialog', { name: '回滚流程' })).toContainText('v0 vs active v1');
  await expect(page.getByRole('dialog', { name: '回滚流程' })).toContainText('回滚说明 / 审计备注');

  await page.getByRole('button', { name: '确认回滚到 v0' }).click();
  await expect(page.locator('.workflow-state-pill').filter({ hasText: 'v3' })).toBeVisible();
  await expect(page.locator('.version-section').filter({ hasText: 'Active Published Version' })).toContainText('流程 v2');
  await expect(page.locator('.version-audit-card')).toContainText('已提交回滚请求');
});

test('confirms archive and filters workflow versions by lifecycle status', async ({ page }) => {
  await mockStudioApi(page, {
    activeDefinition: workflowDefinition,
    archivedDefinition: {
      ...workflowDefinition,
      nodes: [workflowDefinition.nodes[0]],
      edges: [],
      meta: { activityId, workspaceId, folderId },
    },
  });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await page.getByRole('button', { name: '版本' }).click();
  const historyList = page.locator('.version-section').filter({ hasText: '历史 Versions' }).locator('.version-list');

  await page.locator('.version-filter-tabs').getByRole('button', { name: 'ACTIVE' }).click();
  await expect(historyList.locator('.version-card')).toHaveCount(1);
  await expect(historyList.locator('.version-card')).toContainText('流程 v1');
  await expect(historyList.locator('.version-card').getByRole('button', { name: '归档' })).toHaveCount(0);

  await page.locator('.version-filter-tabs').getByRole('button', { name: 'SUPERSEDED' }).click();
  await expect(historyList.locator('.version-card')).toHaveCount(1);
  await expect(historyList.locator('.version-card')).toContainText('流程 v-1');
  await expect(historyList.locator('.version-card')).toContainText('被新版本替代');
  await historyList.locator('.version-card').filter({ hasText: '流程 v-1' }).getByRole('button', { name: '详情' }).click();
  await expect(page.locator('.version-detail-card')).toContainText('workflow v-1');
  await expect(page.locator('.version-detail-card')).toContainText('节点');
  await expect(page.locator('.version-governance-card')).toContainText('SUPERSEDED');

  await page.locator('.version-filter-tabs').getByRole('button', { name: 'ARCHIVED' }).click();
  await expect(historyList.locator('.version-card')).toHaveCount(1);
  await expect(historyList.locator('.version-card')).toContainText('流程 v0');
  await page.locator('.version-section').filter({ hasText: '版本对比' }).locator('select').nth(1).selectOption('version_0');
  await expect(page.locator('.version-section').filter({ hasText: '版本对比' })).toContainText('新增节点');
  await expect(page.locator('.version-section').filter({ hasText: '版本对比' }).locator('.workflow-diff-row')).toHaveCount(4);

  await historyList.locator('.version-card').filter({ hasText: '流程 v0' }).getByRole('button', { name: '归档' }).click();
  await expect(page.getByRole('dialog', { name: '归档流程版本' })).toBeVisible();
  await expect(page.getByRole('dialog', { name: '归档流程版本' })).toContainText('归档后该版本只保留历史查看用途');
  await expect(page.getByRole('button', { name: '确认归档 v0' })).toBeVisible();

  await page.getByRole('button', { name: '确认归档 v0' }).click();
  await expect(historyList.locator('.version-empty')).toContainText('当前筛选下没有版本');
});

test('warns before leaving with unsaved workflow changes', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await page.getByRole('button', { name: '简化链路' }).click();
  await expect(page.locator('.workflow-state-pill').filter({ hasText: '未保存' })).toBeVisible();

  const dialogPromise = page.waitForEvent('dialog');
  const reloadPromise = page.reload({ waitUntil: 'domcontentloaded', timeout: 8_000 }).catch((error: Error) => error);
  const dialog = await dialogPromise;
  expect(dialog.type()).toBe('beforeunload');
  await dialog.dismiss();
  await reloadPromise;
});

test('uses product dialog before in-app navigation with unsaved workflow changes', async ({ page }) => {
  await mockStudioApi(page);

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await page.getByRole('button', { name: '简化链路' }).click();
  await expect(page.locator('.workflow-state-pill').filter({ hasText: '未保存' })).toBeVisible();

  await page.getByRole('button', { name: /E2E Workflow/ }).first().click();
  await page.getByRole('button', { name: /Retention Workflow/ }).click();
  const leaveDialog = page.getByRole('dialog', { name: '未保存的 Workflow Draft' });
  await expect(leaveDialog).toBeVisible();
  await expect(leaveDialog).toContainText('继续操作会放弃当前未保存内容');

  await page.getByRole('button', { name: '继续编辑' }).click();
  await expect(leaveDialog).toBeHidden();
  await expect(page.getByRole('button', { name: /E2E Workflow/ }).first()).toBeVisible();
});

test('locates failed run nodes, highlights route trace, and inspects payloads', async ({ page }) => {
  await mockStudioApi(page, { activeDefinition: workflowDefinition, withExecution: true });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await expect(page.locator('.activity-node')).toHaveCount(3);

  await page.getByRole('button', { name: '运行历史' }).click();
  await expect(page.locator('.run-history-panel')).toContainText('execution execution_');
  await expect(page.locator('.run-execution-item.run-failed')).toContainText('FAILED');
  await expect(page.locator('.run-history-pagination')).toContainText('1-2');
  await expect(page.locator('.run-execution-detail')).toContainText('错误分类');
  await expect(page.locator('.run-execution-detail')).toContainText('事件接入');
  await expect(page.locator('.activity-node.run-failed')).toBeVisible();
  await expect(page.locator('.edge-run-selected')).toHaveCount(1);
  await expect(page.locator('.run-timeline')).toContainText('Execution Timeline');
  await expect(page.locator('.run-timeline-node')).toHaveCount(2);
  await expect(page.locator('.run-timeline-node.run-failed')).toContainText('Inbox write failed');
  await expect(page.locator('.run-timeline-route')).toContainText('默认路径');
  await expect(page.locator('.run-timeline-controls')).toContainText('1/3');

  await page.locator('.run-timeline-controls').getByRole('button', { name: '下一步' }).click();
  await expect(page.locator('.run-timeline-route.replay-active')).toContainText('默认路径');
  await expect(page.locator('.run-timeline-controls')).toContainText('2/3');
  await expect(page).toHaveURL(/executionId=execution_failed/);
  await expect(page).toHaveURL(/replayStep=1/);
  await page.getByRole('button', { name: '复制定位链接' }).click();
  await expect(page.locator('.run-history-panel header')).toContainText('已复制');
  const traceDownload = page.waitForEvent('download');
  await page.getByRole('button', { name: '导出 Trace' }).click();
  await expect((await traceDownload).suggestedFilename()).toContain('workflow-trace-execution_failed');

  await page.locator('.run-compare-selector select').selectOption('execution_success');
  await expect(page).toHaveURL(/compareExecutionId=execution_success/);
  await expect(page.locator('.run-compare-panel')).toContainText('Execution Compare');
  await expect(page.locator('.run-compare-panel')).toContainText('路径');
  await expect(page.locator('.run-compare-panel')).toContainText('失败节点');
  await expect(page.locator('.run-compare-panel')).toContainText('幂等收件箱');
  await expect(page.locator('.run-compare-panel')).toContainText('Payload');
  await expect(page.locator('.run-compare-payload-diff')).toContainText('Payload Diff');
  await page.locator('.run-compare-payload-diff section').filter({ hasText: '幂等收件箱' }).getByRole('button').click();
  await expect(page.locator('.run-json-diff-rows')).toContainText('$.raw_event.userId');
  await expect(page.locator('.run-json-diff-rows')).toContainText('current: "user_1"');
  await expect(page.locator('.run-json-diff-rows')).toContainText('baseline: "user_0"');

  await page.locator('.run-timeline-controls').getByRole('button', { name: '下一步' }).click();
  await expect(page.locator('.run-timeline-node.run-failed.replay-active')).toContainText('Inbox write failed');
  await expect(page.locator('.run-timeline-controls')).toContainText('3/3');
  await expect(page).toHaveURL(/replayStep=2/);

  await page.locator('.run-timeline-controls').getByRole('button', { name: '重置' }).click();
  await expect(page.locator('.run-timeline-controls')).toContainText('1/3');
  await expect(page.locator('.run-timeline-controls')).toContainText('事件入口');
  await expect(page).toHaveURL(/replayStep=0/);

  await page.locator('.run-node-item.run-failed').click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位运行节点/);

  await page.getByPlaceholder('搜索 JSON').fill('user_1');
  await expect(page.locator('.run-payload-card.match')).toBeVisible();
  await expect(page.locator('.run-payload-tabs mark')).toContainText('user_1');
  await expect(page.locator('.run-payload-card').filter({ hasText: 'Input Payload' }).getByRole('button', { name: '下载' })).toBeVisible();

  await page.locator('.run-payload-card').filter({ hasText: 'Output Payload' }).getByRole('button', { name: '折叠' }).click();
  await expect(page.locator('.run-payload-card.collapsed').filter({ hasText: 'Output Payload' })).toBeVisible();

  await page.locator('.run-route-trace button').click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位运行路径/);

  await page.locator('.run-timeline-node.run-success').click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位运行节点/);
  await page.locator('.run-timeline-route').click();
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位运行路径/);
});

test('debug panel saves payload templates, reuses execution payload, and links validation', async ({ page }) => {
  await mockStudioApi(page, { activeDefinition: workflowDefinition, withExecution: true });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await page.getByRole('button', { name: '运行历史' }).click();
  await page.locator('.run-node-item.run-failed').click();

  await page.getByRole('button', { name: '调试' }).click();
  await expect(page.locator('.debug-run-panel')).toContainText('DEBUG_DRAFT');
  await page.getByRole('button', { name: '使用选中 execution payload' }).click();
  await expect(page.locator('.debug-run-panel textarea')).toContainText('evt_1001');
  await page.getByRole('button', { name: '使用节点 Input' }).click();
  await expect(page.locator('.debug-run-panel textarea')).toContainText('raw_event');

  page.once('dialog', async (dialog) => {
    await dialog.accept('历史失败样例');
  });
  await page.getByRole('button', { name: '保存为模板' }).click();
  await expect(page.locator('.debug-run-panel')).toContainText('已保存 payload 模板');
  await expect(page.locator('.debug-run-panel select').nth(1)).toContainText('历史失败样例');
  page.once('dialog', async (dialog) => {
    await dialog.accept('节点输出样例');
  });
  await page.getByRole('button', { name: 'Output 存模板' }).click();
  await expect(page.locator('.debug-run-panel')).toContainText('已保存节点 OUTPUT 模板');
  await expect(page.locator('.debug-run-panel select').nth(1)).toContainText('节点输出样例');

  await page.getByRole('textbox', { name: '从节点继续跑' }).fill('inbox');
  await expect(page.locator('.debug-resume-preview')).toContainText('上游 output');
  await page.getByRole('button', { name: '从节点继续' }).click();
  await expect(page.locator('.debug-run-panel')).toContainText('已生成 DEBUG_DRAFT');
  await expect(page.locator('.run-execution-item.debug-execution')).toContainText('DEBUG_DRAFT');
  await expect(page.locator('.checklist-popover')).toContainText('全部检查通过');
  await expect(page.locator('.checklist-popover')).toContainText('当前 draft 可以发布');
});

test('imports offline trace and keeps sparse node details offline', async ({ page }) => {
  await mockStudioApi(page, { activeDefinition: workflowDefinition, withExecution: true });
  let offlineNodeDetailRequests = 0;
  await page.route('**/admin/v1/workflow/executions/offline_execution/nodes/**', async (route) => {
    offlineNodeDetailRequests++;
    await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'offline should not fetch node detail' }) });
  });

  await page.goto(`/admin-flow/?activityId=${activityId}`);
  await page.getByRole('button', { name: '运行历史' }).click();
  const trace = {
    schemaVersion: 'workflow.execution.trace.v1',
    exportedAt: '2026-06-02T10:00:01.000Z',
    execution: {
      id: 'offline_execution',
      tenantId: 'site_001',
      activityId,
      workflowVersionId: 'version_1',
      workflowVersion: 1,
      status: 'FAILED',
      eventId: 'evt_offline',
      startedAt: '2026-06-02T10:00:00.000Z',
      finishedAt: '2026-06-02T10:00:00.300Z',
      errorMessage: 'INBOX_WRITE_FAILED: offline import',
    },
    nodeExecutions: [{
      id: 'offline_node_exec_inbox',
      executionId: 'offline_execution',
      nodeId: 'inbox',
      nodeType: 'inbox',
      nodeTitle: '幂等收件箱',
      status: 'FAILED',
      errorMessage: 'INBOX_WRITE_FAILED: offline import',
      startedAt: '2026-06-02T10:00:00.120Z',
      finishedAt: '2026-06-02T10:00:00.300Z',
    }],
    nodeSummaries: [{
      nodeId: 'inbox',
      nodeType: 'inbox',
      nodeTitle: '幂等收件箱',
      status: 'FAILED',
      errorCode: 'INBOX_WRITE_FAILED',
      explanation: 'offline sparse summary',
    }],
    routeTrace: [],
  };
  await page.locator('.run-history-header-actions input[type="file"]').setInputFiles({
    name: 'offline-trace.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify(trace)),
  });

  await expect(page.locator('.run-history-panel header')).toContainText('离线 Trace 查看');
  await expect(page.locator('.run-execution-item.active')).toContainText('evt_offline');
  await expect(page.locator('.run-node-item.active')).toContainText('幂等收件箱');
  await page.locator('.run-node-item.active').click();
  expect(offlineNodeDetailRequests).toBe(0);
});

test('restores run history execution, replay step, and compare baseline from URL', async ({ page }) => {
  await mockStudioApi(page, { activeDefinition: workflowDefinition, withExecution: true });

  await page.goto(`/admin-flow/?activityId=${activityId}&executionId=execution_failed&replayStep=1&compareExecutionId=execution_success`);
  await expect(page.locator('.activity-node')).toHaveCount(3);
  await expect(page.locator('.run-history-panel')).toContainText('execution execution_');
  await expect(page.locator('.run-timeline-controls')).toContainText('2/3');
  await expect(page.locator('.run-timeline-route.replay-active')).toContainText('默认路径');
  await expect(page.locator('.workflow-breadcrumb')).toHaveAttribute('title', /已定位运行路径/);
  await expect(page.locator('.run-compare-selector select')).toHaveValue('execution_success');
  await expect(page.locator('.run-compare-panel')).toContainText('Execution Compare');
  await expect(page.locator('.run-compare-payload-diff')).toContainText('Payload Diff');

  await page.locator('.run-history-panel header').getByRole('button', { name: '×' }).click();
  await expect(page).not.toHaveURL(/executionId=/);
  await expect(page).not.toHaveURL(/replayStep=/);
  await expect(page).not.toHaveURL(/compareExecutionId=/);
});
