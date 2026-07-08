import type { ResourceCategoryMap, ResourceRecord } from './types';
import { readJSON, writeJSON } from './campaignPageStore';

const CAMPAIGN_RESOURCE_STORAGE_KEY = 'exmarketing.campaignEditor.resources.v1';
const CAMPAIGN_RESOURCE_CATEGORY_STORAGE_KEY = 'exmarketing.campaignEditor.resourceCategories.v1';

export const resourceStorageApi = {
  loadRecords: (fallback: ResourceRecord[]) => readJSON<ResourceRecord[]>(CAMPAIGN_RESOURCE_STORAGE_KEY, fallback),
  saveRecords: (records: ResourceRecord[]) => writeJSON(CAMPAIGN_RESOURCE_STORAGE_KEY, records),
  loadCategories: (fallback: ResourceCategoryMap) => readJSON<ResourceCategoryMap>(CAMPAIGN_RESOURCE_CATEGORY_STORAGE_KEY, fallback),
  saveCategories: (categories: ResourceCategoryMap) => writeJSON(CAMPAIGN_RESOURCE_CATEGORY_STORAGE_KEY, categories),
  previewDataSource: async (record: ResourceRecord) => {
    if (record.tab !== '数据源') return null;
    if (!record.endpoint || record.endpoint.startsWith('local://')) {
      return {
        ok: true,
        latency: '--',
        body: { activity: { title: '夏日充值返利活动' }, reward: { amount: 188 } },
      };
    }
    const startedAt = performance.now();
    try {
      const response = await fetch(record.endpoint.replace(':activityId', 'act_20250520_001'), {
        method: record.method === 'POST' ? 'POST' : 'GET',
      });
      const body = await response.json().catch(() => ({}));
      return {
        ok: response.ok,
        latency: `${Math.round(performance.now() - startedAt)} ms`,
        body,
      };
    } catch {
      return {
        ok: false,
        latency: `${Math.round(performance.now() - startedAt)} ms`,
        body: { error: '接口暂不可用，使用 Mock 预览。', activity: { title: '夏日充值返利活动' }, reward: { amount: 188 } },
      };
    }
  },
};
