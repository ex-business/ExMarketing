const apiBase = process.env.EXM_E2E_API_BASE || 'http://127.0.0.1:8080';
const actorId = process.env.EXM_E2E_ACTOR || 'admin-flow';
const fixtureNamePattern = /^(E2E|Visual) (Workspace|Role)|^Visual Workspace /;

async function apiJSON(method, path, body, actor = actorId) {
  const response = await fetch(`${apiBase}${path}`, {
    method,
    headers: { 'content-type': 'application/json', 'X-Actor-Id': actor },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  if (!response.ok) throw new Error(`${method} ${path} failed ${response.status}: ${text}`);
  return text ? JSON.parse(text) : {};
}

async function safe(method, path, body, actor = actorId) {
  try {
    return await apiJSON(method, path, body, actor);
  } catch (error) {
    console.warn(`[cleanup] ${error.message}`);
    return null;
  }
}

const workspaces = await safe('GET', '/admin/v1/workspaces?limit=1000');
const activities = await safe('GET', '/admin/v1/activities?limit=5000');
const targets = (workspaces?.items || []).filter((workspace) => workspace.status !== 'DELETED' && fixtureNamePattern.test(workspace.name || ''));

for (const workspace of targets) {
  const cleanupActor = workspace.ownerId || actorId;
  for (const activity of activities?.items || []) {
    if (activity.workspaceId !== workspace.id || activity.status === 'ARCHIVED') continue;
    await safe('PUT', `/admin/v1/activities/${activity.id}`, {
      operator: actorId,
      reason: 'e2e-cleanup-script',
      status: 'ARCHIVED',
    }, cleanupActor);
  }
  const deleted = await safe('DELETE', `/admin/v1/workspaces/${workspace.id}`, undefined, cleanupActor);
  if (deleted) console.log(`[cleanup] deleted ${workspace.name} (${workspace.id})`);
}

console.log(`[cleanup] done, ${targets.length} workspace fixture(s) processed`);
