import type { CampaignBlockKind, CampaignPageSchema, CampaignPageSummary, CampaignPageVersion, CampaignValidationIssue, LayoutKind } from './types';
import { initialBlocks, initialFreeElements } from './layout';
import { withEditorPage } from './editorSchema';
import { normalizePage } from './editorEngine';

const CAMPAIGN_PAGE_STORAGE_KEY = 'exmarketing.campaignEditor.page.v1';
const CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY = 'exmarketing.campaignEditor.publishedPage.v1';
const CAMPAIGN_PUBLISHED_PAGE_MAP_STORAGE_KEY = 'exmarketing.campaignEditor.publishedPages.v1';
const CAMPAIGN_VERSION_STORAGE_KEY = 'exmarketing.campaignEditor.versions.v1';
const CAMPAIGN_PAGE_LIST_STORAGE_KEY = 'exmarketing.campaignEditor.pages.v1';
const ALLOWED_ACTIONS = new Set(['CLAIM_REWARD', 'OPEN_LOGIN', 'NAVIGATE', 'TRACK_EVENT']);
const ALLOWED_BLOCK_KINDS = new Set<CampaignBlockKind>(['HeroBanner', 'Title', 'RewardCard', 'Countdown', 'ProgressBlock', 'ClaimButton', 'RuleText', 'FAQBlock']);
const ALLOWED_LAYOUT_KINDS = new Set<LayoutKind>(['horizontal-layout', 'vertical-layout']);
type CampaignPageInput = Parameters<typeof withEditorPage>[0];

export function nowISO() {
  return new Date().toISOString();
}

export function readJSON<T>(key: string, fallback: T): T {
  if (typeof window === 'undefined') return fallback;
  try {
    const value = window.localStorage.getItem(key);
    return value ? JSON.parse(value) as T : fallback;
  } catch {
    return fallback;
  }
}

export function writeJSON(key: string, value: unknown) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(key, JSON.stringify(value));
}

export function normalizeCampaignPage(page: CampaignPageInput): CampaignPageSchema {
  const pageWithEditor = withEditorPage(page);
  return {
    ...pageWithEditor,
    editor: normalizePage(pageWithEditor.editor),
  };
}

export function defaultCampaignPage(): CampaignPageSchema {
  return normalizeCampaignPage({
    id: 'page_20250520_001',
    activityId: 'act_20250520_001',
    name: '夏日充值返利活动',
    path: '/campaign/summer-rebate',
    designMode: 'controlled',
    device: {
      defaultWidth: 393,
      supportedWidths: [375, 393, 414],
    },
    theme: {
      primaryToken: '--theme-accent',
      cardRadius: 8,
      contentGap: 12,
      fontFamily: 'System UI',
    },
    blocks: initialBlocks,
    freeElements: initialFreeElements,
    status: 'DRAFT',
    version: 'v1.0.0',
    updatedAt: nowISO(),
  });
}

export function pageSummary(page: CampaignPageSchema): CampaignPageSummary {
  return {
    id: page.id,
    name: page.name,
    status: page.status,
    version: page.version,
    path: page.path,
    updatedAt: page.updatedAt,
    createdBy: '运营团队',
  };
}

export function nextVersionLabel(versions: CampaignPageVersion[]) {
  return `v1.0.${versions.length + 1}`;
}

function publishedPageKey(path: string) {
  return path.replace(/^\/admin-flow/, '');
}

function unpublishedRuntimePage(path: string): CampaignPageSchema {
  const fallback = defaultCampaignPage();
  return normalizeCampaignPage({
    ...fallback,
    name: '页面未发布',
    path: publishedPageKey(path),
    blocks: [],
    freeElements: [],
    status: 'OFFLINE',
  });
}

function readPublishedPageMap() {
  return readJSON<Record<string, CampaignPageSchema>>(CAMPAIGN_PUBLISHED_PAGE_MAP_STORAGE_KEY, {});
}

function savePublishedPageMap(items: Record<string, CampaignPageSchema>) {
  writeJSON(CAMPAIGN_PUBLISHED_PAGE_MAP_STORAGE_KEY, items);
}

export const campaignPageApi = {
  loadPage: () => {
    const fallback = defaultCampaignPage();
    const stored = readJSON<CampaignPageSchema>(CAMPAIGN_PAGE_STORAGE_KEY, fallback);
    return normalizeCampaignPage({
      ...fallback,
      ...stored,
      designMode: stored.designMode ?? 'controlled',
      freeElements: stored.freeElements ?? initialFreeElements,
    });
  },
  loadPublishedPage: (path?: string) => {
    const legacy = readJSON<CampaignPageSchema | null>(CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY, null);
    if (!path) return legacy ? normalizeCampaignPage(legacy) : null;
    const map = readPublishedPageMap();
    const key = publishedPageKey(path);
    if (map[key]) return normalizeCampaignPage(map[key]);
    return legacy && publishedPageKey(legacy.path) === key ? normalizeCampaignPage(legacy) : null;
  },
  loadRuntimePage: (path: string) => {
    const published = campaignPageApi.loadPublishedPage(path);
    if (published) return published;
    const draft = campaignPageApi.loadPage();
    return publishedPageKey(draft.path) === publishedPageKey(path) ? draft : unpublishedRuntimePage(path);
  },
  saveDraft: (page: CampaignPageSchema) => writeJSON(CAMPAIGN_PAGE_STORAGE_KEY, normalizeCampaignPage(page)),
  savePublishedPage: (page: CampaignPageSchema) => {
    const normalizedPage = normalizeCampaignPage(page);
    writeJSON(CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY, normalizedPage);
    savePublishedPageMap({
      ...readPublishedPageMap(),
      [publishedPageKey(normalizedPage.path)]: normalizedPage,
      [`/admin-flow${publishedPageKey(normalizedPage.path)}`]: normalizedPage,
    });
  },
  savePageList: (items: CampaignPageSummary[]) => writeJSON(CAMPAIGN_PAGE_LIST_STORAGE_KEY, items),
  loadPageList: () => readJSON<CampaignPageSummary[]>(CAMPAIGN_PAGE_LIST_STORAGE_KEY, [pageSummary(normalizeCampaignPage(readJSON(CAMPAIGN_PAGE_STORAGE_KEY, defaultCampaignPage())))]),
  saveVersions: (items: CampaignPageVersion[]) => writeJSON(CAMPAIGN_VERSION_STORAGE_KEY, items),
  loadVersions: () => readJSON<CampaignPageVersion[]>(CAMPAIGN_VERSION_STORAGE_KEY, []),
  publish: (page: CampaignPageSchema, versions: CampaignPageVersion[]) => {
    const versionLabel = nextVersionLabel(versions);
    const publishedPage: CampaignPageSchema = normalizeCampaignPage({ ...page, status: 'PUBLISHED', version: versionLabel, updatedAt: nowISO() });
    const version: CampaignPageVersion = {
      id: `ver_${Date.now().toString(36)}`,
      pageId: publishedPage.id,
      version: versionLabel,
      status: 'PUBLISHED',
      schemaSnapshot: publishedPage,
      publishedBy: '运营团队',
      publishedAt: nowISO(),
    };
    campaignPageApi.savePublishedPage(publishedPage);
    return { publishedPage, version };
  },
  rollbackToVersion: (version: CampaignPageVersion) => {
    const rolledBackPage: CampaignPageSchema = normalizeCampaignPage({
      ...version.schemaSnapshot,
      status: 'DRAFT',
      updatedAt: nowISO(),
    });
    campaignPageApi.saveDraft(rolledBackPage);
    return rolledBackPage;
  },
};

function validateBindingExpression(value: string | undefined): boolean {
  if (!value) return true;
  return /^\{\{\s*!?[a-z]+(?:\.[A-Za-z0-9_]+)+\s*\}\}$/.test(value.trim());
}

export function validateCampaignPage(page: CampaignPageSchema): CampaignValidationIssue[] {
  const issues: CampaignValidationIssue[] = [];
  if (!page.name.trim()) issues.push({ code: 'PAGE_NAME_REQUIRED', message: '页面名称不能为空。' });
  if (!page.path.startsWith('/campaign/')) issues.push({ code: 'PAGE_PATH_INVALID', message: '页面路径必须以 /campaign/ 开头。' });
  if (!page.editor.nodes.length) issues.push({ code: 'PAGE_NODES_REQUIRED', message: '页面至少需要一个节点。' });
  if (!page.editor.rootNodeIds.length && page.editor.nodes.length) issues.push({ code: 'PAGE_ROOT_NODES_REQUIRED', message: '页面需要至少一个根节点。' });
  const nodeIds = new Set(page.editor.nodes.map((node) => node.id));
  for (const node of page.editor.nodes) {
    if (!node.name.trim()) issues.push({ code: 'NODE_NAME_REQUIRED', message: `${node.id} 的节点名称不能为空。` });
    if (node.parentId && !nodeIds.has(node.parentId)) issues.push({ code: 'NODE_PARENT_MISSING', message: `${node.name} 的父节点不存在。` });
    for (const binding of node.resourceBindings) {
      if (!validateBindingExpression(binding.expression)) issues.push({ code: 'NODE_BINDING_INVALID', message: `${node.name} 的资源绑定表达式不合法。` });
    }
    for (const action of node.actionBindings) {
      if (!ALLOWED_ACTIONS.has(action.actionId)) issues.push({ code: 'NODE_ACTION_UNSUPPORTED', message: `${node.name} 的动作不在白名单内。` });
    }
    if (node.type === 'container') {
      if (!ALLOWED_LAYOUT_KINDS.has(node.layout)) issues.push({ code: 'NODE_LAYOUT_UNSUPPORTED', message: `${node.name} 的布局类型不在白名单内。` });
      for (const childId of node.children) {
        if (!nodeIds.has(childId)) issues.push({ code: 'NODE_CHILD_MISSING', message: `${node.name} 的子节点不存在。` });
      }
    }
  }
  for (const block of page.blocks) {
    if (!ALLOWED_BLOCK_KINDS.has(block.kind)) issues.push({ code: 'BLOCK_TYPE_UNSUPPORTED', message: `${block.title} 的内容块类型不在白名单内。` });
    if (!validateBindingExpression(block.binding)) issues.push({ code: 'BLOCK_BINDING_INVALID', message: `${block.title} 的绑定表达式不合法。` });
    if (!validateBindingExpression(block.visibleWhen)) issues.push({ code: 'BLOCK_VISIBLE_EXPR_INVALID', message: `${block.title} 的展示条件不合法。` });
    if (block.action && !ALLOWED_ACTIONS.has(block.action)) issues.push({ code: 'BLOCK_ACTION_UNSUPPORTED', message: `${block.title} 的动作不在白名单内。` });
  }
  for (const element of page.freeElements ?? []) {
    if (!element.text.trim()) issues.push({ code: 'FREE_ELEMENT_TEXT_REQUIRED', message: `${element.id} 的自由元素内容不能为空。` });
    if (!validateBindingExpression(element.binding)) issues.push({ code: 'FREE_ELEMENT_BINDING_INVALID', message: `${element.id} 的绑定表达式不合法。` });
    if (!validateBindingExpression(element.visibleWhen)) issues.push({ code: 'FREE_ELEMENT_VISIBLE_EXPR_INVALID', message: `${element.id} 的展示条件不合法。` });
    if (element.action && !ALLOWED_ACTIONS.has(element.action)) issues.push({ code: 'FREE_ELEMENT_ACTION_UNSUPPORTED', message: `${element.id} 的动作不在白名单内。` });
  }
  return issues;
}
