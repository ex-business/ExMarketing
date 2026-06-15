import type {
  ActivitySummary,
  CreateStudioActivityInput,
  WorkspaceMemberSummary,
  WorkspaceRole,
  WorkspaceSummary,
} from '../types';

export const WORKSPACE_FILE_UPLOAD_MAX_BYTES = 64 * 1024 * 1024;

const LAST_WORKSPACE_ID_STORAGE_KEY = 'exmarketing.studio.lastWorkspaceId';
const LAST_FOLDER_ID_STORAGE_KEY = 'exmarketing.studio.lastFolderId';

const WORKSPACE_ROLE_RANK: Record<WorkspaceRole, number> = {
  GUEST: 0,
  VIEWER: 1,
  EDITOR: 2,
  ADMIN: 3,
  OWNER: 4,
};

export function studioActorId(): string {
  return window.localStorage.getItem('exmarketing.studio.actorId') || 'admin-flow';
}

export function shouldSeedDemoWorkspace(): boolean {
  const params = new URLSearchParams(window.location.search);
  const queryValue = params.get('seedDemo');
  if (queryValue != null) {
    return ['1', 'true', 'yes'].includes(queryValue.trim().toLowerCase());
  }
  return String(import.meta.env.VITE_EXMARKETING_SEED_DEMO ?? '').trim().toLowerCase() === 'true';
}

export function workspaceRoleAllows(role: WorkspaceRole, required: WorkspaceRole): boolean {
  return WORKSPACE_ROLE_RANK[role] >= WORKSPACE_ROLE_RANK[required];
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = value;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }
  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}

export function loadStoredWorkspaceSelection(): { workspaceId: string; folderId: string } {
  return {
    workspaceId: window.localStorage.getItem(LAST_WORKSPACE_ID_STORAGE_KEY) || '',
    folderId: window.localStorage.getItem(LAST_FOLDER_ID_STORAGE_KEY) || '',
  };
}

export function loadInitialWorkspaceSelectionFromLocation(): { workspaceId: string; folderId: string } {
  const params = new URLSearchParams(window.location.search);
  const stored = loadStoredWorkspaceSelection();
  const workspaceId = params.get('workspaceId') || stored.workspaceId;
  const folderId = params.get('folderId') || (params.get('workspaceId') ? '' : stored.folderId);
  return { workspaceId, folderId };
}

export function persistWorkspaceSelection(workspaceId: string, folderId = '') {
  if (workspaceId) {
    window.localStorage.setItem(LAST_WORKSPACE_ID_STORAGE_KEY, workspaceId);
  } else {
    window.localStorage.removeItem(LAST_WORKSPACE_ID_STORAGE_KEY);
  }
  if (folderId) {
    window.localStorage.setItem(LAST_FOLDER_ID_STORAGE_KEY, folderId);
  } else {
    window.localStorage.removeItem(LAST_FOLDER_ID_STORAGE_KEY);
  }
}

export function isTestActivity(activity: ActivitySummary): boolean {
  const text = `${activity.activityName} ${activity.activityCode} ${activity.id}`.toLowerCase();
  return [
    'postgresql e2e',
    'load fault',
    'smoke',
    'e2e_',
    'load_fault',
    'pg_smoke',
  ].some((marker) => text.includes(marker));
}

export function workspaceRoleForActor(workspace: WorkspaceSummary | undefined, members: WorkspaceMemberSummary[], actorId: string): WorkspaceRole {
  if (!workspace || !actorId) return 'GUEST';
  if (workspace.ownerId && workspace.ownerId === actorId) return 'OWNER';
  const member = members.find((item) => (
    item.status === 'ACTIVE' &&
    ((item.userId && item.userId === actorId) || (item.email && item.email.toLowerCase() === actorId.toLowerCase()))
  ));
  if (member) return member.role;
  if (workspace.visibility === 'PUBLIC') return 'VIEWER';
  if (workspace.visibility === 'TEAM' && workspace.defaultRole === 'VIEWER') return 'VIEWER';
  return 'GUEST';
}

export function normalizeActivityCode(value: string): string {
  const code = value
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[^A-Za-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .toUpperCase();
  return code || `ACTIVITY_${Date.now().toString(36).toUpperCase()}`;
}

export function buildStudioActivityPayload(input: CreateStudioActivityInput) {
  const now = new Date();
  const start = new Date(now.getTime() - 60_000).toISOString();
  const end = new Date(now.getTime() + 365 * 24 * 60 * 60 * 1000).toISOString();
  const activityName = input.activityName.trim();
  const activityCode = normalizeActivityCode(input.activityCode || activityName);
  return {
    operator: 'studio',
    tenantId: input.tenantId.trim() || 'site_001',
    workspaceId: input.workspaceId,
    folderId: input.folderId,
    activityCode,
    activityName: activityName || activityCode,
    activityType: 'STUDIO_WORKFLOW',
    status: 'DRAFT',
    visibility: input.visibility || 'TEAM',
    triggerEvents: ['STUDIO_EVENT'],
    startTime: start,
    endTime: end,
    timezone: 'UTC',
    priority: 100,
    exclusiveGroup: '',
    stackable: true,
    version: 1,
    config: {
      entry: {
        channels: ['APP', 'WEB'],
        entranceCodes: ['studio'],
        landingUrl: 'https://example.com/studio',
      },
      display: {
        title: activityName || activityCode,
        visibleFrom: start,
        visibleTo: end,
        sort: 100,
      },
      audience: {
        segmentCodes: ['all_users'],
      },
      metadata: {
        owner: 'studio',
        workspaceId: input.workspaceId,
        folderId: input.folderId,
      },
    },
    rules: [
      {
        ruleCode: 'STUDIO_DEFAULT_RULE',
        rulePhase: 'QUALIFY',
        ruleType: 'CONDITION',
        ruleConfig: {
          logic: 'AND',
          conditions: [
            { field: 'attributes.eventMatched', op: '=', value: true },
          ],
        },
        sort: 1,
        enabled: true,
        version: 1,
      },
    ],
    rewards: [
      {
        rewardCode: 'STUDIO_DEFAULT_REWARD',
        rewardType: 'POINT',
        calculateType: 'FIXED',
        rewardConfig: {
          points: 1,
          unit: 'POINT',
          pointType: 'DEFAULT',
          targetRef: 'https://wallet.example.com/reward',
        },
        grantMode: 'AUTO',
        grantChannel: 'HTTP',
        enabled: true,
        version: 1,
      },
    ],
    workflowDraft: input.definition,
  };
}
