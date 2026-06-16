import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const actorId = 'admin-flow';
const apiBase = 'http://127.0.0.1:8080';
const activitiesPath = '/admin/v1/activities?limit=1000';
const cleanupWorkspaceIds = new Set<string>();
const cleanupTemplateNames = new Set<string>();

type Workspace = { id: string; name: string; tenantId: string };
type Folder = { id: string; workspaceId: string; name: string; parentId?: string; status: string; visibility?: string };
type Activity = { id: string; activityName: string; activityCode: string; workspaceId: string; folderId: string; status: string; visibility?: string };
type ShareLink = { id: string; token?: string; activityId?: string; scope: string; enabled: boolean };
type Member = { id: string; workspaceId: string; email: string; status: string; inviteToken?: string };
type ActivityGrant = { id: string; activityId: string; principalType: string; principalId: string; role: string };
type Template = { id: string; versionId: string; name: string; version: number; status: string; source: string };

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

async function apiJSON<T>(request: APIRequestContext, method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<T> {
  const response = await request.fetch(`${apiBase}${path}`, {
    method,
    headers: { 'content-type': 'application/json', 'X-Actor-Id': actorId },
    data: body,
  });
  const text = await response.text();
  if (!response.ok()) throw new Error(`${method} ${path} failed ${response.status()}: ${text}`);
  return text ? JSON.parse(text) as T : {} as T;
}

async function safeApiJSON<T>(request: APIRequestContext, method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<T | null> {
  try {
    return await apiJSON<T>(request, method, path, body);
  } catch {
    return null;
  }
}

async function cleanupE2EFixtures(request: APIRequestContext) {
  const workspaceIds = [...cleanupWorkspaceIds].reverse();
  const templateNames = [...cleanupTemplateNames];
  cleanupWorkspaceIds.clear();
  cleanupTemplateNames.clear();

  for (const workspaceId of workspaceIds) {
    const activities = await safeApiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
    for (const activity of activities?.items ?? []) {
      if (activity.workspaceId !== workspaceId || activity.status === 'ARCHIVED') continue;
      await safeApiJSON<Activity>(request, 'PUT', `/admin/v1/activities/${activity.id}`, {
        operator: actorId,
        reason: 'e2e-cleanup',
        status: 'ARCHIVED',
      });
    }
    await safeApiJSON<Workspace>(request, 'DELETE', `/admin/v1/workspaces/${workspaceId}`);
  }

  if (!templateNames.length) return;
  const templates = await safeApiJSON<{ items: Template[] }>(request, 'GET', '/admin/v1/workflow/draft-templates?source=SAVED&latestOnly=true&limit=500');
  for (const template of templates?.items ?? []) {
    if (!templateNames.includes(template.name) || template.status === 'ARCHIVED') continue;
    await safeApiJSON<Template>(request, 'PUT', `/admin/v1/workflow/draft-templates/${template.id}`, {
      status: 'ARCHIVED',
      operator: actorId,
    });
  }
}

async function createWorkspaceFixture(request: APIRequestContext, suffix: string) {
  const workspace = await apiJSON<Workspace>(request, 'POST', '/admin/v1/workspaces', {
    tenantId: 'site_001',
    name: `E2E Workspace ${suffix}`,
    slug: `e2e-workspace-${suffix.toLowerCase()}`,
    visibility: 'TEAM',
    defaultRole: 'VIEWER',
    ownerId: actorId,
    createdBy: actorId,
  });
  cleanupWorkspaceIds.add(workspace.id);
  const root = await apiJSON<Folder>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/folders`, {
    name: `E2E Root ${suffix}`,
    visibility: 'TEAM',
    sortOrder: 10,
    createdBy: actorId,
  });
  const target = await apiJSON<Folder>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/folders`, {
    name: `E2E Target ${suffix}`,
    visibility: 'TEAM',
    sortOrder: 20,
    createdBy: actorId,
  });
  return { workspace, root, target };
}

async function createWorkspaceRoleFixture(request: APIRequestContext, suffix: string, role: 'OWNER' | 'ADMIN' | 'EDITOR' | 'VIEWER') {
  const workspace = await apiJSON<Workspace>(request, 'POST', '/admin/v1/workspaces', {
    tenantId: 'site_001',
    name: `E2E Role ${role} ${suffix}`,
    slug: `e2e-role-${role.toLowerCase()}-${suffix.toLowerCase()}`,
    visibility: 'TEAM',
    defaultRole: 'VIEWER',
    ownerId: actorId,
    createdBy: actorId,
  });
  cleanupWorkspaceIds.add(workspace.id);
  const root = await apiJSON<Folder>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/folders`, {
    name: `Role Root ${role} ${suffix}`,
    visibility: 'TEAM',
    sortOrder: 10,
    createdBy: actorId,
  });
  const workflow = await createWorkflow(request, {
    name: `Role Flow ${role} ${suffix}`,
    code: `ROLE_FLOW_${role}_${suffix}`,
    workspaceId: workspace.id,
    folderId: root.id,
  });
  if (role !== 'OWNER') {
    const members = await apiJSON<{ items: Member[] }>(request, 'GET', `/admin/v1/workspaces/${workspace.id}/members`);
    const actorMember = members.items.find((member) => member.email === actorId || member.id === actorId || member.status === 'ACTIVE') ?? members.items[0];
    await apiJSON<Member>(request, 'PUT', `/admin/v1/workspaces/${workspace.id}/members/${actorMember.id}`, {
      role,
      status: 'ACTIVE',
    });
    await apiJSON<Workspace>(request, 'PUT', `/admin/v1/workspaces/${workspace.id}`, {
      ownerId: `owner-${suffix}`,
    });
  }
  return { workspace, root, workflow };
}

function workflowPayload(input: { name: string; code: string; workspaceId: string; folderId: string; visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC' }) {
  const now = Date.now();
  const start = new Date(now - 60_000).toISOString();
  const end = new Date(now + 365 * 24 * 60 * 60 * 1000).toISOString();
  return {
    operator: actorId,
    tenantId: 'site_001',
    workspaceId: input.workspaceId,
    folderId: input.folderId,
    activityCode: input.code,
    activityName: input.name,
    activityType: 'STUDIO_WORKFLOW',
    status: 'DRAFT',
    visibility: input.visibility ?? 'TEAM',
    triggerEvents: ['STUDIO_EVENT'],
    startTime: start,
    endTime: end,
    timezone: 'UTC',
    priority: 100,
    exclusiveGroup: '',
    stackable: true,
    version: 1,
    config: {
      entry: { channels: ['APP', 'WEB'], entranceCodes: ['studio'], landingUrl: 'https://example.com/studio' },
      display: { title: input.name, visibleFrom: start, visibleTo: end, sort: 100 },
      audience: { segmentCodes: ['all_users'] },
      metadata: { owner: 'studio', workspaceId: input.workspaceId, folderId: input.folderId },
    },
    rules: [{
      ruleCode: 'STUDIO_DEFAULT_RULE',
      rulePhase: 'QUALIFY',
      ruleType: 'CONDITION',
      ruleConfig: { logic: 'AND', conditions: [{ field: 'attributes.eventMatched', op: '=', value: true }] },
      sort: 1,
      enabled: true,
      version: 1,
    }],
    rewards: [{
      rewardCode: 'STUDIO_DEFAULT_REWARD',
      rewardType: 'POINT',
      calculateType: 'FIXED',
      rewardConfig: { points: 1, unit: 'POINT', pointType: 'DEFAULT', targetRef: 'https://wallet.example.com/reward' },
      grantMode: 'AUTO',
      grantChannel: 'HTTP',
      enabled: true,
      version: 1,
    }],
  };
}

async function createWorkflow(request: APIRequestContext, input: { name: string; code: string; workspaceId: string; folderId: string; visibility?: 'PRIVATE' | 'TEAM' | 'PUBLIC' }) {
  return apiJSON<Activity>(request, 'POST', '/admin/v1/activities', workflowPayload(input));
}

async function selectWorkspace(page: Page, workspaceId: string) {
  await page.goto(`/admin-flow/?workspaceId=${workspaceId}`, { waitUntil: 'networkidle' });
  await expect(page.getByRole('heading', { level: 1, name: /E2E Root/ })).toBeVisible({ timeout: 15_000 });
}

async function openWorkflowMenu(page: Page, workflowName: string) {
  const row = page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: workflowName }).first();
  await expect(row).toBeVisible();
  await row.locator('[data-testid="workspace-row-more"]').click();
  const menu = page.getByTestId('workspace-row-action-rename');
  if (!(await menu.isVisible().catch(() => false))) {
    await row.locator('[data-testid="workspace-row-more"]').click();
  }
  await expect(menu).toBeVisible();
  return row;
}

async function openFolder(page: Page, folderName: string) {
  await page.getByRole('button', { name: new RegExp(escapeRegExp(folderName)) }).first().click();
  await expect(page.getByRole('heading', { level: 1, name: folderName })).toBeVisible({ timeout: 15_000 });
}

async function checkRow(page: Page, type: 'workflow' | 'folder', name: string) {
  const row = page.locator(`[data-testid="workspace-row-${type}"]`).filter({ hasText: name }).first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.getByTestId('workspace-row-checkbox').check();
  return row;
}

test.describe('real PostgreSQL workspace workflow operations', () => {
  test.setTimeout(90_000);

  test.afterEach(async ({ request }) => {
    await cleanupE2EFixtures(request);
  });

  test('creates, moves, shares, archives and persists after refresh', async ({ page, request }) => {
    const suffix = String(Date.now());
    const workflowName = `E2E Workflow ${suffix}`;
    const workflowCode = `E2E_WORKFLOW_${suffix}`;
    const { workspace, root, target } = await createWorkspaceFixture(request, suffix);

    await selectWorkspace(page, workspace.id);

    await page.getByTestId('workspace-new-workflow').click();
    await page.getByTestId('workflow-name-input').fill(workflowName);
    await page.getByTestId('workflow-code-input').fill(workflowCode);
    await page.getByTestId('workflow-create-confirm').click();

    await expect.poll(async () => {
      const activities = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      return activities.items.find((item) => item.activityName === workflowName)?.id ?? '';
    }, { timeout: 15_000, message: 'workflow persisted in PostgreSQL after UI create' }).not.toBe('');
    const activities = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
    const created = activities.items.find((item) => item.activityName === workflowName);
    const createdId = created?.id ?? '';
    expect(created, 'workflow persisted in PostgreSQL after UI create').toBeTruthy();
    expect(created?.workspaceId).toBe(workspace.id);
    expect(created?.folderId).toBe(root.id);

    await selectWorkspace(page, workspace.id);
    await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: workflowName }).first()).toBeVisible();
    await openWorkflowMenu(page, workflowName);
    await page.getByTestId('workspace-row-action-move').click();
    await page.getByTestId('detail-action-target-folder').selectOption(target.id);
    await page.getByTestId('detail-action-confirm').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      return body.items.find((item) => item.id === createdId)?.folderId ?? '';
    }, { timeout: 15_000, message: 'workflow move persisted in PostgreSQL' }).toBe(target.id);

    await page.reload({ waitUntil: 'networkidle' });
    await openFolder(page, target.name);
    await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: workflowName }).first()).toBeVisible({ timeout: 15_000 });

    await openWorkflowMenu(page, workflowName);
    await page.getByTestId('workspace-row-action-share').click();
    await expect.poll(async () => {
      const shares = await apiJSON<{ items: ShareLink[] }>(request, 'GET', `/admin/v1/workspaces/${workspace.id}/share-links?limit=500`);
      return (shares.items ?? []).some((item) => item.activityId === createdId && item.enabled);
    }, { timeout: 15_000, message: 'share link persisted in PostgreSQL after UI share' }).toBe(true);

    await openWorkflowMenu(page, workflowName);
    await page.getByTestId('workspace-row-action-archive').click();
    await page.getByTestId('detail-action-confirm').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      return body.items.find((item) => item.id === createdId)?.status ?? '';
    }, { timeout: 15_000, message: 'workflow archive persisted in PostgreSQL' }).toBe('ARCHIVED');

    await page.reload({ waitUntil: 'networkidle' });
    await openFolder(page, target.name);
    await page.locator('.exm-content-tools').getByRole('button', { name: '筛选' }).click();
    await page.locator('.exm-advanced-filters select').nth(1).selectOption('ARCHIVED');
    await expect(page.locator('[data-testid="workspace-row-workflow"]').filter({ hasText: workflowName }).first()).toBeVisible({ timeout: 15_000 });
  });

  test('creates folder through UI and persists after refresh', async ({ page, request }) => {
    const suffix = String(Date.now());
    const folderName = `E2E UI Folder ${suffix}`;
    const { workspace } = await createWorkspaceFixture(request, suffix);

    await selectWorkspace(page, workspace.id);
    await page.getByTestId('workspace-new-folder').click();
    await page.getByTestId('folder-name-input').fill(folderName);
    await page.getByTestId('folder-create-confirm').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Folder[] }>(request, 'GET', `/admin/v1/workspaces/${workspace.id}/folders?limit=500`);
      return body.items.some((folder) => folder.name === folderName && folder.status === 'ACTIVE');
    }, { timeout: 15_000, message: 'folder persisted in PostgreSQL after UI create' }).toBe(true);

    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.getByRole('button', { name: new RegExp(escapeRegExp(folderName)) }).first()).toBeVisible({ timeout: 15_000 });
  });

  test('bulk moves, updates permissions and archives selected items', async ({ page, request }) => {
    const suffix = String(Date.now());
    const { workspace, root, target } = await createWorkspaceFixture(request, suffix);
    const first = await createWorkflow(request, { name: `Bulk Flow A ${suffix}`, code: `QA_BULK_A_${suffix}`, workspaceId: workspace.id, folderId: root.id });
    const second = await createWorkflow(request, { name: `Bulk Flow B ${suffix}`, code: `QA_BULK_B_${suffix}`, workspaceId: workspace.id, folderId: root.id });

    await selectWorkspace(page, workspace.id);
    await checkRow(page, 'workflow', first.activityName);
    await checkRow(page, 'workflow', second.activityName);
    await page.getByTestId('workspace-bulk-move').click();
    await page.getByTestId('batch-target-folder').selectOption(target.id);
    await page.getByTestId('batch-confirm').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      const moved = body.items.filter((item) => item.id === first.id || item.id === second.id);
      return moved.every((item) => item.folderId === target.id);
    }, { timeout: 15_000, message: 'bulk move persisted in PostgreSQL' }).toBe(true);

    await page.reload({ waitUntil: 'networkidle' });
    await openFolder(page, target.name);
    await checkRow(page, 'workflow', first.activityName);
    await checkRow(page, 'workflow', second.activityName);
    await page.getByTestId('workspace-bulk-permission').click();
    await page.getByTestId('batch-visibility').selectOption('Public');
    await page.getByTestId('batch-confirm').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      const updated = body.items.filter((item) => item.id === first.id || item.id === second.id);
      return updated.every((item) => item.visibility === 'PUBLIC');
    }, { timeout: 15_000, message: 'bulk permission persisted in PostgreSQL' }).toBe(true);

    await page.reload({ waitUntil: 'networkidle' });
    await openFolder(page, target.name);
    await checkRow(page, 'workflow', first.activityName);
    await checkRow(page, 'workflow', second.activityName);
    await page.getByTestId('workspace-bulk-archive').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Activity[] }>(request, 'GET', activitiesPath);
      const archived = body.items.filter((item) => item.id === first.id || item.id === second.id);
      return archived.every((item) => item.status === 'ARCHIVED');
    }, { timeout: 15_000, message: 'bulk archive persisted in PostgreSQL' }).toBe(true);
  });

  test('manages workflow ACL grant through UI', async ({ page, request }) => {
    const suffix = String(Date.now());
    const { workspace, root } = await createWorkspaceFixture(request, suffix);
    const workflow = await createWorkflow(request, { name: `ACL Flow ${suffix}`, code: `QA_ACL_${suffix}`, workspaceId: workspace.id, folderId: root.id });
    const member = await apiJSON<Member>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/members`, {
      userId: `acl-user-${suffix}`,
      email: `acl-${suffix}@example.com`,
      displayName: `ACL User ${suffix}`,
      role: 'VIEWER',
      status: 'ACTIVE',
    });

    await selectWorkspace(page, workspace.id);
    await openWorkflowMenu(page, workflow.activityName);
    await page.getByTestId('workspace-row-action-permission').click();
    await page.getByTestId('detail-tab-permission').click();
    await page.getByTestId('acl-principal-type').selectOption('MEMBER');
    await page.getByTestId('acl-principal-id').selectOption(member.id);
    await page.getByTestId('acl-role').selectOption('EDITOR');
    await page.getByTestId('acl-add').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: ActivityGrant[] }>(request, 'GET', `/admin/v1/activities/${workflow.id}/acl`);
      return (body.items ?? []).find((grant) => grant.principalId === member.id && grant.role === 'EDITOR')?.id ?? '';
    }, { timeout: 15_000, message: 'ACL grant persisted in PostgreSQL after UI add' }).not.toBe('');
    const grants = await apiJSON<{ items: ActivityGrant[] }>(request, 'GET', `/admin/v1/activities/${workflow.id}/acl`);
    const grantId = (grants.items ?? []).find((grant) => grant.principalId === member.id && grant.role === 'EDITOR')?.id ?? '';

    await page.getByTestId('acl-delete').click();
    await expect.poll(async () => {
      const body = await apiJSON<{ items: ActivityGrant[] }>(request, 'GET', `/admin/v1/activities/${workflow.id}/acl`);
      return (body.items ?? []).some((grant) => grant.id === String(grantId));
    }, { timeout: 15_000, message: 'ACL grant removed after UI delete' }).toBe(false);
  });

  test('applies workspace role matrix to create and permission controls', async ({ page, request }) => {
    const suffix = String(Date.now());
    const cases = [
      { role: 'VIEWER' as const, workflow: true, folder: true, acl: true },
      { role: 'EDITOR' as const, workflow: false, folder: true, acl: true },
      { role: 'ADMIN' as const, workflow: false, folder: false, acl: true },
      { role: 'OWNER' as const, workflow: false, folder: false, acl: false },
    ];

    for (const item of cases) {
      const { workspace, root, workflow } = await createWorkspaceRoleFixture(request, `${suffix}-${item.role}`, item.role);
      await page.goto(`/admin-flow/?workspaceId=${workspace.id}`, { waitUntil: 'networkidle' });
      await expect(page.getByRole('heading', { level: 1, name: root.name })).toBeVisible({ timeout: 15_000 });
      if (item.workflow) await expect(page.getByTestId('workspace-new-workflow')).toBeDisabled();
      else await expect(page.getByTestId('workspace-new-workflow')).toBeEnabled();
      if (item.folder) await expect(page.getByTestId('workspace-new-folder')).toBeDisabled();
      else await expect(page.getByTestId('workspace-new-folder')).toBeEnabled();
      await openWorkflowMenu(page, workflow.activityName);
      if (item.acl) await expect(page.getByTestId('workspace-row-action-permission')).toBeDisabled();
      else await expect(page.getByTestId('workspace-row-action-permission')).toBeEnabled();
      await page.keyboard.press('Escape');
    }
  });

  test('opens share pages for read-only, password and expired states', async ({ page, request }) => {
    const suffix = String(Date.now());
    const { workspace, root } = await createWorkspaceFixture(request, suffix);
    const workflow = await createWorkflow(request, { name: `Shared Flow ${suffix}`, code: `QA_SHARED_${suffix}`, workspaceId: workspace.id, folderId: root.id });
    const readOnly = await apiJSON<ShareLink>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/share-links`, {
      scope: 'ACTIVITY',
      activityId: workflow.id,
      folderId: root.id,
      permission: 'VIEW',
      createdBy: actorId,
    });
    const password = await apiJSON<ShareLink>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/share-links`, {
      scope: 'ACTIVITY',
      activityId: workflow.id,
      folderId: root.id,
      permission: 'VIEW',
      passwordHash: 'secret-pass',
      createdBy: actorId,
    });
    const expired = await apiJSON<ShareLink>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/share-links`, {
      scope: 'ACTIVITY',
      activityId: workflow.id,
      folderId: root.id,
      permission: 'VIEW',
      expiresAt: '2020-01-01T00:00:00Z',
      createdBy: actorId,
    });

    await page.goto(`/share/${readOnly.token}`, { waitUntil: 'networkidle' });
    await expect(page.getByRole('heading', { name: workflow.activityName })).toBeVisible({ timeout: 15_000 });
    await expect(page.locator('.share-badge', { hasText: 'ACTIVITY · VIEW' })).toBeVisible();

    await page.goto(`/share/${password.token}`, { waitUntil: 'networkidle' });
    await page.getByPlaceholder('输入访问密码').fill('bad-pass');
    await page.getByRole('button', { name: '访问' }).click();
    await expect(page.getByText('密码错误', { exact: false })).toBeVisible({ timeout: 15_000 });
    await page.getByPlaceholder('输入访问密码').fill('secret-pass');
    await page.getByRole('button', { name: '访问' }).click();
    await expect(page.getByRole('heading', { name: workflow.activityName })).toBeVisible({ timeout: 15_000 });

    await page.goto(`/share/${expired.token}`, { waitUntil: 'networkidle' });
    await expect(page.getByText('链接已过期')).toBeVisible({ timeout: 15_000 });
  });

  test('accepts workspace invite from landing page', async ({ page, request }) => {
    const suffix = String(Date.now());
    const { workspace } = await createWorkspaceFixture(request, suffix);
    const invited = await apiJSON<Member & { inviteUrl?: string }>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/members`, {
      userId: '',
      email: `invite-${suffix}@example.com`,
      displayName: `Invite User ${suffix}`,
      role: 'EDITOR',
      status: 'INVITED',
      invitedBy: actorId,
    });
    const token = invited.inviteToken ?? invited.inviteUrl?.split('/invite/')[1] ?? '';
    expect(token).not.toBe('');

    await page.goto(`/invite/${token}`, { waitUntil: 'networkidle' });
    await expect(page.getByRole('heading', { name: new RegExp(escapeRegExp(workspace.name)) })).toBeVisible({ timeout: 15_000 });
    await page.getByPlaceholder('你的显示名称').fill(`Accepted User ${suffix}`);
    await page.getByPlaceholder('用户 ID').fill(`accepted-${suffix}`);
    await page.getByRole('button', { name: '接受邀请' }).click();
    await expect(page.getByText('邀请已接受')).toBeVisible({ timeout: 15_000 });

    await expect.poll(async () => {
      const members = await apiJSON<{ items: Member[] }>(request, 'GET', `/admin/v1/workspaces/${workspace.id}/members`);
      return members.items.find((member) => member.email === invited.email)?.status ?? '';
    }, { timeout: 15_000, message: 'invite accepted in PostgreSQL' }).toBe('ACTIVE');
  });

  test('copies, archives and compares workflow templates through UI', async ({ page, request }) => {
    const suffix = String(Date.now());
    const templateName = `E2E Template ${suffix}`;
    const copiedName = `E2E Template Copy ${suffix}`;
    cleanupTemplateNames.add(templateName);
    cleanupTemplateNames.add(copiedName);
    const { workspace } = await createWorkspaceFixture(request, suffix);
    await apiJSON<Template>(request, 'POST', '/admin/v1/workflow/draft-templates/simple-reward-chain/copy', {
      name: templateName,
      category: 'E2E Templates',
      workspaceId: workspace.id,
      visibility: 'TEAM',
      tags: ['e2e'],
      operator: actorId,
    });

    await selectWorkspace(page, workspace.id);
    await page.getByTestId('workspace-new-workflow').click();
    await page.getByTestId('workflow-name-input').fill(`E2E Template Source ${suffix}`);
    await page.getByTestId('workflow-code-input').fill(`E2E_TEMPLATE_SOURCE_${suffix}`);
    await page.getByTestId('workflow-create-confirm').click();
    await expect(page.getByRole('button', { name: '模板', exact: true })).toBeVisible({ timeout: 15_000 });
    await page.getByRole('button', { name: '模板', exact: true }).click();

    await page.getByTestId('template-source-filter').selectOption('SAVED');
    await page.getByTestId('template-catalog-item').filter({ hasText: templateName }).first().click();
    await page.getByTestId('template-copy-name').fill(copiedName);
    await page.getByTestId('template-copy-submit').click();
    await expect(page.getByText(`模板已复制到 workspace：${copiedName}`)).toBeVisible({ timeout: 20_000 });

    await page.getByTestId('template-catalog-item').filter({ hasText: templateName }).first().click();
    await page.getByTestId('template-compare-submit').click();
    await expect(page.getByTestId('template-compare-result')).toBeVisible({ timeout: 20_000 });
    await page.getByTestId('template-archive-toggle').click();

    await expect.poll(async () => {
      const body = await apiJSON<{ items: Template[] }>(request, 'GET', '/admin/v1/workflow/draft-templates?source=SAVED&status=ARCHIVED');
      return body.items.some((template) => template.name === templateName && template.status === 'ARCHIVED');
    }, { timeout: 20_000, message: 'template archived in PostgreSQL after UI action' }).toBe(true);

    const activeTemplates = await apiJSON<{ items: Template[] }>(request, 'GET', '/admin/v1/workflow/draft-templates?source=SAVED&status=ACTIVE');
    expect(activeTemplates.items.some((template) => template.name === copiedName)).toBe(true);
  });
});
