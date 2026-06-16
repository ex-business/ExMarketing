import { expect, test, type APIRequestContext } from '@playwright/test';

const actorId = 'admin-flow';
const apiBase = 'http://127.0.0.1:8080';
const cleanupWorkspaceIds = new Set<string>();

type Workspace = { id: string; name: string; tenantId: string };
type Folder = { id: string; workspaceId: string; name: string; status: string };
type Activity = { id: string; activityName: string; activityCode: string; workspaceId: string; folderId: string; status: string };

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

function workflowPayload(input: { name: string; code: string; workspaceId: string; folderId: string }) {
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
    visibility: 'TEAM',
    triggerEvents: ['STUDIO_EVENT'],
    startTime: start,
    endTime: end,
    timezone: 'UTC',
    priority: 100,
    stackable: true,
    version: 1,
    config: { display: { title: input.name }, metadata: { workspaceId: input.workspaceId, folderId: input.folderId } },
    rules: [{
      ruleCode: 'VISUAL_DEFAULT_RULE',
      rulePhase: 'QUALIFY',
      ruleType: 'CONDITION',
      ruleConfig: { logic: 'AND', conditions: [{ field: 'attributes.eventMatched', op: '=', value: true }] },
      sort: 1,
      enabled: true,
      version: 1,
    }],
    rewards: [{
      rewardCode: 'VISUAL_DEFAULT_REWARD',
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

async function createVisualFixture(request: APIRequestContext, suffix: string) {
  const workspace = await apiJSON<Workspace>(request, 'POST', '/admin/v1/workspaces', {
    tenantId: 'site_001',
    name: `Visual Workspace ${suffix}`,
    slug: `visual-workspace-${suffix.toLowerCase()}`,
    visibility: 'TEAM',
    defaultRole: 'VIEWER',
    ownerId: actorId,
    createdBy: actorId,
  });
  cleanupWorkspaceIds.add(workspace.id);
  const root = await apiJSON<Folder>(request, 'POST', `/admin/v1/workspaces/${workspace.id}/folders`, {
    name: `Visual 风控活动 ${suffix}`,
    visibility: 'TEAM',
    sortOrder: 10,
    createdBy: actorId,
  });
  for (const [index, name] of ['资格校验链路', 'Quota 保护奖励链路', 'Fact Query 数据增强'].entries()) {
    await apiJSON<Activity>(request, 'POST', '/admin/v1/activities', workflowPayload({
      name: `${name} ${suffix}`,
      code: `VISUAL_FLOW_${suffix}_${index}`,
      workspaceId: workspace.id,
      folderId: root.id,
    }));
  }
  return { workspace, root };
}

async function cleanupVisualFixtures(request: APIRequestContext) {
  const workspaceIds = [...cleanupWorkspaceIds].reverse();
  cleanupWorkspaceIds.clear();
  const activities = await safeApiJSON<{ items: Activity[] }>(request, 'GET', '/admin/v1/activities?limit=1000');
  for (const workspaceId of workspaceIds) {
    for (const activity of activities?.items ?? []) {
      if (activity.workspaceId !== workspaceId || activity.status === 'ARCHIVED') continue;
      await safeApiJSON<Activity>(request, 'PUT', `/admin/v1/activities/${activity.id}`, {
        operator: actorId,
        reason: 'visual-e2e-cleanup',
        status: 'ARCHIVED',
      });
    }
    await safeApiJSON<Workspace>(request, 'DELETE', `/admin/v1/workspaces/${workspaceId}`);
  }
}

test.describe('workspace visual acceptance', () => {
  test.setTimeout(75_000);

  test.afterEach(async ({ request }) => {
    await cleanupVisualFixtures(request);
  });

  test('captures desktop and mobile workspace layouts with real data', async ({ page, request }, testInfo) => {
    const suffix = String(Date.now());
    const { workspace, root } = await createVisualFixture(request, suffix);

    for (const viewport of [
      { name: '1280', width: 1280, height: 800 },
      { name: '1440', width: 1440, height: 900 },
      { name: '390', width: 390, height: 844 },
    ]) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await page.goto(`/admin-flow/?workspaceId=${workspace.id}&folderId=${root.id}`, { waitUntil: 'networkidle' });
      await expect(page.getByRole('heading', { level: 1, name: root.name })).toBeVisible({ timeout: 15_000 });
      await expect(page.locator('[data-testid="workspace-row-workflow"]').first()).toBeVisible({ timeout: 15_000 });

      if (viewport.width <= 390) {
        await expect(page.getByRole('button', { name: '打开导航' })).toBeVisible();
        await page.getByRole('button', { name: '打开详情' }).click();
        await expect(page.locator('.exm-workspace-detail')).toBeVisible();
        await page.locator('.exm-mobile-detail-toggle').click();
      } else {
        await expect(page.locator('.exm-workspace-sidebar')).toBeVisible();
        await expect(page.locator('.exm-workspace-detail')).toBeVisible();
      }

      const screenshot = await page.screenshot({ fullPage: true });
      await testInfo.attach(`admin-flow-${viewport.name}.png`, { body: screenshot, contentType: 'image/png' });
    }
  });
});
