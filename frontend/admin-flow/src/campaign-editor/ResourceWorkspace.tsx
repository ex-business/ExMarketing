import { useEffect, useState, type Dispatch, type SetStateAction } from 'react';
import { ChevronDown, ListChecks, Plus } from 'lucide-react';
import type { ResourceRecord, ResourceStatus, ResourceTab, UploadQueueItem } from './types';
import { resourceStorageApi } from './resourceStore';
import { Button, Field, Panel, ResourceTable, Toolbar, Tree, type TreeNode } from './ui';

const resourceTabs: ResourceTab[] = ['数据', '数据源', '动作', '样式', '图片'];

const resourceGroupLabels: Record<ResourceTab, string> = {
  数据: '数据字段',
  数据源: '数据源',
  动作: '动作事件',
  样式: '样式 Token',
  图片: '图片素材',
};

const resourceTree: Record<ResourceTab, Array<{ name: string; count: number }>> = {
  数据: [
    { name: '活动信息', count: 4 },
    { name: '奖励配置', count: 3 },
    { name: '用户信息', count: 2 },
    { name: '时间配置', count: 3 },
  ],
  数据源: [
    { name: '业务接口', count: 3 },
    { name: '实时接口', count: 2 },
    { name: '第三方接口', count: 1 },
    { name: 'Mock 数据', count: 1 },
  ],
  动作: [
    { name: '按钮事件', count: 3 },
    { name: '页面事件', count: 2 },
    { name: '系统事件', count: 3 },
  ],
  样式: [
    { name: '颜色', count: 5 },
    { name: '字体', count: 3 },
    { name: '间距', count: 2 },
    { name: '圆角', count: 2 },
  ],
  图片: [
    { name: '头图', count: 3 },
    { name: '活动配图', count: 2 },
    { name: '图标', count: 2 },
    { name: '背景图', count: 2 },
  ],
};

export const defaultResourceRecords: ResourceRecord[] = [
  { id: 'data_title', tab: '数据', category: '活动信息', name: 'activity.title', identifier: 'activity.title', type: '文本', source: 'Activity View Model', usage: '4 个组件', status: '启用', updatedAt: '2025-05-23 17:32', description: '活动标题，用于头图、标题和分享卡片。', tags: ['头图', '标题', '分享卡片'] },
  { id: 'data_reward_amount', tab: '数据', category: '奖励配置', name: 'reward.amount', identifier: 'reward.amount', type: '数值', source: 'Activity View Model', usage: '3 个组件', status: '启用', updatedAt: '2025-05-23 17:30', description: '奖励金额，展示在奖励卡片和领取确认。', tags: ['奖励卡', '按钮'] },
  { id: 'data_reward_name', tab: '数据', category: '奖励配置', name: 'reward.item.name', identifier: 'reward.item.name', type: '文本', source: 'Activity View Model', usage: '2 个组件', status: '启用', updatedAt: '2025-05-23 17:28', description: '奖励项目名称。', tags: ['奖励卡'] },
  { id: 'data_countdown', tab: '数据', category: '时间配置', name: 'countdown.endTime', identifier: 'countdown.endTime', type: '时间', source: 'Activity View Model', usage: '2 个组件', status: '启用', updatedAt: '2025-05-23 17:25', description: '活动倒计时结束时间。', tags: ['倒计时'] },
  { id: 'data_progress', tab: '数据', category: '活动信息', name: 'progress.block', identifier: 'progress.block', type: '数值', source: 'Activity View Model', usage: '2 个组件', status: '启用', updatedAt: '2025-05-23 17:24', description: '参与进度百分比。', tags: ['进度'] },
  { id: 'data_rules', tab: '数据', category: '活动信息', name: 'activity.rules', identifier: 'activity.rules', type: '文本', source: 'Activity View Model', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 17:23', description: '活动规则文案。', tags: ['规则'] },
  { id: 'data_faq', tab: '数据', category: '活动信息', name: 'faq.block', identifier: 'faq.block', type: '文本', source: 'Activity View Model', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 17:21', description: 'FAQ 内容。', tags: ['FAQ'] },
  { id: 'data_claim', tab: '数据', category: '用户信息', name: 'claim.button.text', identifier: 'claim.button.text', type: '文本', source: 'Activity View Model', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 17:20', description: '领取按钮文案。', tags: ['按钮'] },
  { id: 'source_activity_view', tab: '数据源', category: '业务接口', name: 'Activity View Model API', identifier: 'activity.view-model', type: 'REST API', source: '实时接口', usage: '字段 8 个', status: '启用', updatedAt: '2025-05-23 18:35', description: '活动页面运行时视图模型接口，提供活动、奖励、资格和进度数据。', endpoint: '/api/activities/:activityId/view-model', method: 'GET', refresh: '页面加载 + 30s 轮询', latency: '86 ms', tags: ['activity.title', 'reward.amount', 'eligibility.canClaim'] },
  { id: 'source_reward_stock', tab: '数据源', category: '实时接口', name: 'Reward Stock Stream', identifier: 'reward.stock.stream', type: 'WebSocket', source: '实时接口', usage: '字段 3 个', status: '启用', updatedAt: '2025-05-23 18:31', description: '奖励库存实时数据源，用于展示剩余名额和领取状态。', endpoint: 'wss://api.example.com/rewards/stock', method: 'SUBSCRIBE', refresh: 'WebSocket 实时', latency: '42 ms', tags: ['reward.stock', 'claim.status'] },
  { id: 'source_user_profile', tab: '数据源', category: '业务接口', name: 'User Profile API', identifier: 'user.profile', type: 'REST API', source: '系统内置', usage: '字段 4 个', status: '启用', updatedAt: '2025-05-23 18:24', description: '用户资料接口，提供登录状态、等级、权益和白名单信息。', endpoint: '/api/users/me/profile', method: 'GET', refresh: '登录后刷新', latency: '64 ms', tags: ['user.loggedIn', 'user.level'] },
  { id: 'source_mock_campaign', tab: '数据源', category: 'Mock 数据', name: 'Campaign Preview Mock', identifier: 'campaign.preview.mock', type: 'Mock JSON', source: '页面自定义', usage: '字段 6 个', status: '停用', updatedAt: '2025-05-23 18:10', description: '编辑器预览用 Mock 数据，不参与发布运行时。', endpoint: 'local://campaign-preview.json', method: 'LOCAL', refresh: '手动刷新', latency: '--', tags: ['预览', 'Mock'] },
  { id: 'action_claim', tab: '动作', category: '按钮事件', name: 'CLAIM_REWARD', identifier: 'claim.reward', type: 'Workflow Event', source: '系统内置', usage: '2 个组件', status: '启用', updatedAt: '2025-05-23 18:20', description: '用户领取奖励的工作流事件。', tags: ['按钮', '卡片'] },
  { id: 'action_purchase', tab: '动作', category: '按钮事件', name: 'CompletePurchase', identifier: 'purchase.complete', type: 'Workflow Event', source: '系统内置', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 18:16', description: '完成购买事件。', tags: ['按钮'] },
  { id: 'action_share', tab: '动作', category: '按钮事件', name: 'ShareSuccess', identifier: 'share.success', type: 'Workflow Event', source: '页面自定义', usage: '3 个组件', status: '启用', updatedAt: '2025-05-23 18:12', description: '分享成功事件。', tags: ['按钮', '图片'] },
  { id: 'action_view', tab: '动作', category: '页面事件', name: 'ViewRule', identifier: 'rule.view', type: 'Workflow Event', source: '页面自定义', usage: '2 个组件', status: '启用', updatedAt: '2025-05-23 18:05', description: '查看活动规则。', tags: ['规则'] },
  { id: 'action_exposure', tab: '动作', category: '页面事件', name: 'PageExposure', identifier: 'page.exposure', type: 'Page Event', source: '系统内置', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 17:55', description: '页面曝光事件。', tags: ['页面曝光'] },
  { id: 'action_timer', tab: '动作', category: '系统事件', name: 'TimerTrigger', identifier: 'timer.trigger', type: 'System Event', source: '系统内置', usage: '1 个组件', status: '启用', updatedAt: '2025-05-23 17:42', description: '定时触发事件。', tags: ['系统'] },
  { id: 'style_primary', tab: '样式', category: '颜色', name: '主按钮样式', identifier: 'btn-primary', type: '颜色', source: '页面自定义', usage: '4 个组件', status: '启用', updatedAt: '2025-05-23 15:42', description: '主要按钮颜色 token。', value: '#245BFF', tags: ['按钮', '卡片', '标题'] },
  { id: 'style_secondary', tab: '样式', category: '颜色', name: '次要按钮样式', identifier: 'btn-secondary', type: '颜色', source: '页面自定义', usage: '3 个组件', status: '启用', updatedAt: '2025-05-23 14:58', description: '次要按钮颜色 token。', value: '#F8FAFF', tags: ['按钮'] },
  { id: 'style_title', tab: '样式', category: '颜色', name: '标题文字', identifier: 'text-title', type: '颜色', source: '页面自定义', usage: '6 个组件', status: '启用', updatedAt: '2025-05-23 14:31', description: '标题文字颜色。', value: '#102A56', tags: ['标题'] },
  { id: 'style_radius', tab: '样式', category: '圆角', name: '圆角-卡片', identifier: 'radius-card', type: '圆角', source: '页面自定义', usage: '5 个组件', status: '启用', updatedAt: '2025-05-23 11:56', description: '卡片组件圆角。', value: '8px', tags: ['卡片'] },
  { id: 'image_hero', tab: '图片', category: '头图', name: 'hero-summer-rebate.png', identifier: 'hero-summer-rebate.png', type: '头图', source: '页面上传', usage: '6', status: '启用', updatedAt: '2025-05-23 16:48', description: '活动首页 Hero 区域主视觉图。', size: '750 x 360', fileSize: '120 KB', thumbnail: '🎁', tags: ['头图', '图片', '分享卡片'] },
  { id: 'image_card', tab: '图片', category: '活动配图', name: 'reward-card-bg.png', identifier: 'reward-card-bg.png', type: '卡片背景', source: '页面上传', usage: '4', status: '启用', updatedAt: '2025-05-23 16:40', description: '奖励卡片背景图。', size: '690 x 240', fileSize: '85 KB', thumbnail: '🧾', tags: ['奖励卡'] },
  { id: 'image_icon', tab: '图片', category: '图标', name: 'reward-icon.png', identifier: 'reward-icon.png', type: '图标', source: '素材库', usage: '8', status: '启用', updatedAt: '2025-05-23 16:35', description: '奖励图标。', size: '128 x 128', fileSize: '12 KB', thumbnail: '🎁', tags: ['图标'] },
  { id: 'image_bg', tab: '图片', category: '背景图', name: 'bg-pattern-light.png', identifier: 'bg-pattern-light.png', type: '背景图', source: '素材库', usage: '3', status: '启用', updatedAt: '2025-05-23 16:30', description: '浅色背景纹理。', size: '1920 x 1080', fileSize: '256 KB', thumbnail: '☁️', tags: ['背景图'] },
];

function resourceStatusClass(status: ResourceStatus) {
  if (status === '启用') return 'enabled';
  if (status === '待保存') return 'pending';
  if (status === '尺寸提醒') return 'warning';
  if (status === '大小错误') return 'danger';
  return 'disabled';
}

function recommendedImageSize(purpose: string) {
  if (purpose === '头图') return '750 x 360';
  if (purpose === '活动配图') return '690 x 240';
  if (purpose === '分享图') return '1:1';
  if (purpose === '图标') return '128 x 128';
  return '1920 x 1080';
}

export function CampaignResourceWorkspace({
  records,
  onRecordsChange,
}: {
  records: ResourceRecord[];
  onRecordsChange: Dispatch<SetStateAction<ResourceRecord[]>>;
}) {
  const [activeTab, setActiveTab] = useState<ResourceTab>('数据');
  const [activeCategory, setActiveCategory] = useState(resourceTree.数据[0].name);
  const [expandedGroups, setExpandedGroups] = useState<Record<ResourceTab, boolean>>({
    数据: true,
    数据源: false,
    动作: false,
    样式: false,
    图片: false,
  });
  const [query, setQuery] = useState('');
  const [sourceFilter, setSourceFilter] = useState('全部');
  const [statusFilter, setStatusFilter] = useState('全部');
  const [selectedId, setSelectedId] = useState(defaultResourceRecords[0].id);
  const [addMode, setAddMode] = useState<ResourceTab | null>(null);
  const [draftName, setDraftName] = useState('activity.title');
  const [draftType, setDraftType] = useState('文本');
  const [draftSource, setDraftSource] = useState('Activity View Model');
  const [draftValue, setDraftValue] = useState('');
  const [uploadQueue, setUploadQueue] = useState<UploadQueueItem[]>([]);
  const [editingRecord, setEditingRecord] = useState<ResourceRecord | null>(null);
  const normalizedQuery = query.trim().toLowerCase();
  const activeChildren = resourceTree[activeTab];
  const activeCategories = activeChildren.map((item) => item.name);
  const filteredRecords = records.filter((record) => {
    const categoryMatch = activeCategories.includes(activeCategory) ? record.category === activeCategory : true;
    const queryMatch = !normalizedQuery || [record.name, record.identifier, record.type, record.source, record.description, ...record.tags].join(' ').toLowerCase().includes(normalizedQuery);
    const sourceMatch = sourceFilter === '全部' || record.source === sourceFilter;
    const statusMatch = statusFilter === '全部' || record.status === statusFilter;
    return record.tab === activeTab && categoryMatch && queryMatch && sourceMatch && statusMatch;
  });
  const selectedRecord = records.find((record) => record.id === selectedId && record.tab === activeTab) ?? filteredRecords[0] ?? records.find((record) => record.tab === activeTab) ?? records[0];
  const sourceOptions = ['全部', ...Array.from(new Set(records.filter((record) => record.tab === activeTab).map((record) => record.source)))];
  const treeNodes: TreeNode[] = resourceTabs.map((tab) => ({
    id: `resource_group_${tab}`,
    label: resourceGroupLabels[tab],
    count: records.filter((record) => record.tab === tab).length,
    active: activeTab === tab,
    open: expandedGroups[tab],
    onClick: () => toggleResourceGroup(tab),
    onToggle: () => toggleResourceGroup(tab),
    children: expandedGroups[tab] ? resourceTree[tab].map((child) => ({
      id: `resource_category_${tab}_${child.name}`,
      label: child.name,
      count: records.filter((record) => record.tab === tab && record.category === child.name).length || child.count,
      active: activeTab === tab && activeCategory === child.name,
      onClick: () => {
        setActiveTab(tab);
        setActiveCategory(child.name);
        setExpandedGroups((groups) => ({ ...groups, [tab]: true }));
      },
    })) : [],
  }));

  const toggleResourceGroup = (tab: ResourceTab) => {
    const isActive = activeTab === tab;
    const shouldOpen = isActive ? !expandedGroups[tab] : true;
    setActiveTab(tab);
    setExpandedGroups((groups) => ({ ...groups, [tab]: shouldOpen }));
    if (!isActive) setActiveCategory(resourceTree[tab][0].name);
    setAddMode(null);
    setEditingRecord(null);
    const next = records.find((record) => record.tab === tab);
    if (next) setSelectedId(next.id);
  };

  const startAdd = () => {
    setAddMode(activeTab);
    setEditingRecord(null);
    setDraftName(activeTab === '动作' ? 'CLAIM_REWARD' : activeTab === '样式' ? '主按钮样式' : activeTab === '图片' ? 'hero-summer-rebate.png' : activeTab === '数据源' ? 'Activity Realtime API' : 'activity.title');
    setDraftType(activeTab === '动作' ? 'Workflow Event' : activeTab === '样式' ? '颜色' : activeTab === '图片' ? '头图' : activeTab === '数据源' ? 'REST API' : '文本');
    setDraftSource(activeTab === '动作' ? '页面自定义' : activeTab === '样式' ? '页面自定义' : activeTab === '图片' ? '页面上传' : activeTab === '数据源' ? '实时接口' : 'Activity View Model');
    setDraftValue(activeTab === '样式' ? '#245BFF' : activeTab === '数据源' ? '/api/activities/:activityId/view-model' : '');
    if (activeTab === '图片') {
      setUploadQueue([
        { id: 'sample_hero', fileName: 'hero.png', purpose: '头图', recommendedSize: '750 x 360', status: '待保存', sizeLabel: '120 KB' },
        { id: 'sample_card', fileName: 'card-bg.png', purpose: '活动配图', recommendedSize: '690 x 240', status: '尺寸提醒', sizeLabel: '85 KB' },
        { id: 'sample_share', fileName: 'share.png', purpose: '分享图', recommendedSize: '1:1', status: '待保存', sizeLabel: '210 KB' },
      ]);
    }
  };

  const addRecord = () => {
    if (!addMode) return;
    if (addMode === '图片') {
      const nextRecords = uploadQueue.filter((item) => item.status !== '大小错误').map((item, index): ResourceRecord => ({
        id: `image_upload_${Date.now()}_${index}`,
        tab: '图片',
        category: item.purpose === '分享图' ? '活动配图' : item.purpose,
        name: item.fileName,
        identifier: item.fileName,
        type: item.purpose,
        source: '页面上传',
        usage: '0',
        status: item.status === '尺寸提醒' ? '尺寸提醒' : '启用',
        updatedAt: '刚刚',
        description: `${item.purpose}素材`,
        size: item.recommendedSize,
        fileSize: item.sizeLabel,
        thumbnail: '🖼️',
        tags: [item.purpose, '图片'],
      }));
      if (!nextRecords.length) return;
      onRecordsChange((items) => [...nextRecords, ...items]);
      setSelectedId(nextRecords[0].id);
      setAddMode(null);
      return;
    }
    const tab = addMode;
    const next: ResourceRecord = {
      id: `resource_${Date.now()}`,
      tab,
      category: activeCategory,
      name: draftName,
      identifier: tab === '样式' ? '--campaign-button-primary' : draftName.toLowerCase().replace(/\s+/g, '.'),
      type: draftType,
      source: draftSource,
      usage: tab === '数据源' ? '字段 0 个' : tab === '动作' ? '3 个组件' : tab === '样式' ? '4 个组件' : '3 个组件',
      status: '启用',
      updatedAt: '刚刚',
      description: tab === '数据源' ? '页面运行时数据接口，支持实时拉取和字段映射。' : tab === '动作' ? '用户领取奖励的工作流事件' : tab === '样式' ? '按钮主色样式 Token' : '可绑定到页面组件的数据字段',
      value: draftValue,
      endpoint: tab === '数据源' ? draftValue : undefined,
      method: tab === '数据源' ? 'GET' : undefined,
      refresh: tab === '数据源' ? '页面加载 + 30s 轮询' : undefined,
      latency: tab === '数据源' ? '--' : undefined,
      tags: tab === '数据源' ? ['activity.title', 'reward.amount'] : tab === '动作' ? ['按钮', '图片', '卡片'] : tab === '样式' ? ['按钮', '卡片', '标题'] : ['头图', '标题', '分享卡片'],
    };
    onRecordsChange((items) => [next, ...items]);
    setSelectedId(next.id);
    setAddMode(null);
    setEditingRecord(null);
  };

  const updateEditingRecord = (patch: Partial<ResourceRecord>) => {
    setEditingRecord((record) => record ? { ...record, ...patch } : record);
  };

  const saveEditingRecord = () => {
    if (!editingRecord) return;
    const nextRecord = { ...editingRecord, updatedAt: '刚刚' };
    onRecordsChange((items) => items.map((item) => item.id === nextRecord.id ? nextRecord : item));
    setSelectedId(nextRecord.id);
    setEditingRecord(null);
  };

  const duplicateRecord = (record: ResourceRecord) => {
    const nextRecord: ResourceRecord = {
      ...record,
      id: `resource_copy_${Date.now()}`,
      name: `${record.name} 副本`,
      identifier: `${record.identifier}.copy`,
      updatedAt: '刚刚',
    };
    onRecordsChange((items) => [nextRecord, ...items]);
    setActiveTab(nextRecord.tab);
    setActiveCategory(nextRecord.category);
    setExpandedGroups((groups) => ({ ...groups, [nextRecord.tab]: true }));
    setSelectedId(nextRecord.id);
    setAddMode(null);
    setEditingRecord(null);
  };

  const deleteRecord = (record: ResourceRecord) => {
    const remaining = records.filter((item) => item.id !== record.id);
    onRecordsChange(remaining);
    const nextRecord = remaining.find((item) => item.tab === record.tab && item.category === record.category)
      ?? remaining.find((item) => item.tab === record.tab)
      ?? remaining[0];
    if (nextRecord) {
      setActiveTab(nextRecord.tab);
      setActiveCategory(nextRecord.category);
      setExpandedGroups((groups) => ({ ...groups, [nextRecord.tab]: true }));
      setSelectedId(nextRecord.id);
    }
    setAddMode(null);
    setEditingRecord(null);
  };

  const onUploadFiles = (files: FileList | null) => {
    if (!files) return;
    const next = Array.from(files).map((file, index): UploadQueueItem => {
      const purpose = index === 0 ? '头图' : index === 1 ? '活动配图' : '分享图';
      return {
        id: `file_${file.name}_${Date.now()}_${index}`,
        fileName: file.name,
        purpose,
        recommendedSize: recommendedImageSize(purpose),
        status: file.size > 5 * 1024 * 1024 ? '大小错误' : '待保存',
        sizeLabel: `${Math.max(1, Math.round(file.size / 1024))} KB`,
      };
    });
    setUploadQueue(next);
  };

  return (
    <section className="campaign-resource-workspace" data-testid="resource-workspace">
      <Toolbar title="资源" className="campaign-resource-bar">
        <label className="campaign-resource-search">
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索资源（名称、标识符...）" />
        </label>
        <select aria-label="来源" value={sourceFilter} onChange={(event) => setSourceFilter(event.target.value)}>
          {sourceOptions.map((source) => <option key={source}>{source}</option>)}
        </select>
        <select aria-label="状态" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          {['全部', '启用', '停用', '待保存', '尺寸提醒'].map((status) => <option key={status}>{status}</option>)}
        </select>
        <Button variant="primary" icon={<Plus size={16} />} onClick={startAdd}>添加</Button>
      </Toolbar>

      <div className="campaign-resource-layout">
        <aside className="campaign-resource-tree" data-testid="resource-type-tree">
          <div className="campaign-resource-tree-head"><strong>资源类型</strong><button type="button" aria-label="添加资源类型"><Plus size={16} /></button></div>
          <Tree nodes={treeNodes} className="campaign-resource-tree-list" />
          <button className="campaign-resource-tree-footer" type="button"><ListChecks size={16} />常用组件<ChevronDown size={14} /></button>
        </aside>

        <section className="campaign-resource-table-wrap" data-testid="resource-table">
          <ResourceTable
            activeTab={activeTab}
            records={filteredRecords}
            selectedId={selectedId}
            statusClass={resourceStatusClass}
            draftRow={addMode && addMode !== '图片' ? {
              mode: addMode,
              draftType,
              draftSource,
              draftValue,
              onDraftType: setDraftType,
              onDraftSource: setDraftSource,
              onSave: addRecord,
              onCancel: () => setAddMode(null),
            } : undefined}
            onSelect={(record) => {
              setSelectedId(record.id);
              setAddMode(null);
              setEditingRecord(null);
            }}
          />
          <footer className="campaign-resource-pagination"><span>共 {filteredRecords.length} 条</span><button type="button">‹</button><b>1</b><button type="button">›</button><select aria-label="每页条数"><option>10 条/页</option><option>20 条/页</option></select></footer>
        </section>

        <aside className="campaign-resource-panel" data-testid="resource-detail-panel">
          {addMode ? (
            <ResourceAddPanel
              mode={addMode}
              draftName={draftName}
              draftType={draftType}
              draftSource={draftSource}
              draftValue={draftValue}
              uploadQueue={uploadQueue}
              onDraftName={setDraftName}
              onDraftType={setDraftType}
              onDraftSource={setDraftSource}
              onDraftValue={setDraftValue}
              onUploadFiles={onUploadFiles}
              onUpdateUpload={(id, purpose) => setUploadQueue((items) => items.map((item) => item.id === id ? { ...item, purpose, recommendedSize: recommendedImageSize(purpose) } : item))}
              onRemoveUpload={(id) => setUploadQueue((items) => items.filter((item) => item.id !== id))}
              onCancel={() => setAddMode(null)}
              onSave={addRecord}
            />
          ) : editingRecord ? (
            <ResourceEditPanel
              record={editingRecord}
              onChange={updateEditingRecord}
              onCancel={() => setEditingRecord(null)}
              onSave={saveEditingRecord}
            />
          ) : (
            <ResourceDetailPanel
              record={selectedRecord}
              onEdit={() => setEditingRecord({ ...selectedRecord })}
              onDuplicate={() => duplicateRecord(selectedRecord)}
              onDelete={() => deleteRecord(selectedRecord)}
            />
          )}
        </aside>
      </div>
    </section>
  );
}

function ResourceDetailPanel({
  record,
  onEdit,
  onDuplicate,
  onDelete,
}: {
  record: ResourceRecord;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
}) {
  const [preview, setPreview] = useState<{ ok: boolean; latency: string; body: unknown } | null>(null);

  useEffect(() => {
    let active = true;
    if (record.tab !== '数据源') {
      setPreview(null);
      return;
    }
    resourceStorageApi.previewDataSource(record).then((result) => {
      if (active) setPreview(result);
    });
    return () => {
      active = false;
    };
  }, [record]);

  return (
    <Panel title="资源详情" bodyClassName="campaign-resource-detail-body">
        <h3>{record.name}</h3>
        <p>{record.description}</p>
        <dl>
          <div><dt>类型</dt><dd>{record.type}</dd></div>
          <div><dt>来源</dt><dd>{record.source}</dd></div>
          {record.tab === '数据源' ? <div><dt>接口地址</dt><dd>{record.endpoint}</dd></div> : null}
          {record.tab === '数据源' ? <div><dt>请求方式</dt><dd>{record.method}</dd></div> : null}
          {record.tab === '数据源' ? <div><dt>刷新策略</dt><dd>{record.refresh}</dd></div> : null}
          {record.tab === '数据源' ? <div><dt>最近延迟</dt><dd>{record.latency}</dd></div> : null}
          <div><dt>状态</dt><dd><span className={`campaign-resource-status ${resourceStatusClass(record.status)}`}>{record.status}</span></dd></div>
          <div><dt>引用位置</dt><dd>{record.tags.join('、')}</dd></div>
        </dl>
        {record.tab === '数据源' ? (
          <div className="campaign-source-preview">
            <strong>实时拉取预览</strong>
            <code>{JSON.stringify(preview?.body ?? { loading: true }, null, 2)}</code>
            <span>{preview ? `${preview.ok ? '拉取成功' : 'Mock 回退'} · ${preview.latency}` : '拉取中...'}</span>
          </div>
        ) : null}
        <div className="campaign-resource-usage"><strong>使用方式</strong><p>{record.tab === '数据源' ? '数据源负责从接口或实时通道拉取原始数据，再映射为页面可绑定字段。' : record.tab === '图片' ? '拖入图片或头图组件后绑定素材资源。' : record.tab === '样式' ? '组件样式从主题 token 读取。' : record.tab === '动作' ? '点击事件只能绑定白名单 workflow event。' : '字段通过 Activity View Model 表达式绑定到页面组件。'}</p></div>
        <div className="campaign-resource-panel-actions">
          <Button onClick={onEdit}>编辑</Button>
          <Button onClick={onDuplicate}>复制</Button>
          <Button variant="danger" onClick={onDelete}>删除</Button>
        </div>
    </Panel>
  );
}

function ResourceEditPanel({
  record,
  onChange,
  onCancel,
  onSave,
}: {
  record: ResourceRecord;
  onChange: (patch: Partial<ResourceRecord>) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  return (
    <Panel
      title="编辑资源"
      onClose={onCancel}
      bodyClassName="campaign-resource-form"
      footer={<><Button onClick={onCancel}>取消</Button><Button variant="primary" onClick={onSave}>保存</Button></>}
    >
        <Field label="名称" required><input value={record.name} onChange={(event) => onChange({ name: event.target.value })} /></Field>
        <Field label="标识符" required><input value={record.identifier} onChange={(event) => onChange({ identifier: event.target.value })} /></Field>
        <Field label="类型" required><select value={record.type} onChange={(event) => onChange({ type: event.target.value })}><option>文本</option><option>数值</option><option>时间</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Mock JSON</option><option>Workflow Event</option><option>Page Event</option><option>颜色</option><option>圆角</option><option>头图</option><option>活动配图</option><option>图标</option><option>背景图</option></select></Field>
        <Field label="来源" required><select value={record.source} onChange={(event) => onChange({ source: event.target.value })}><option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option><option>第三方接口</option><option>页面上传</option><option>素材库</option></select></Field>
        <Field label="状态" required><select value={record.status} onChange={(event) => onChange({ status: event.target.value as ResourceStatus })}><option>启用</option><option>停用</option><option>待保存</option><option>尺寸提醒</option><option>大小错误</option></select></Field>
        {record.tab === '数据源' ? <Field label="接口地址" required><input value={record.endpoint ?? ''} onChange={(event) => onChange({ endpoint: event.target.value })} /></Field> : null}
        {record.tab === '数据源' ? <Field label="请求方式" required><select value={record.method ?? 'GET'} onChange={(event) => onChange({ method: event.target.value })}><option>GET</option><option>POST</option><option>SUBSCRIBE</option><option>LOCAL</option></select></Field> : null}
        {record.tab === '数据源' ? <Field label="刷新策略" required><select value={record.refresh ?? '页面加载 + 30s 轮询'} onChange={(event) => onChange({ refresh: event.target.value })}><option>页面加载 + 30s 轮询</option><option>页面加载</option><option>手动刷新</option><option>WebSocket 实时</option></select></Field> : null}
        {record.tab === '样式' ? <Field label="Token 值" required><input value={record.value ?? ''} onChange={(event) => onChange({ value: event.target.value })} /></Field> : null}
        {record.tab === '图片' ? <Field label="尺寸"><input value={record.size ?? ''} onChange={(event) => onChange({ size: event.target.value })} /></Field> : null}
        {record.tab === '图片' ? <Field label="大小"><input value={record.fileSize ?? ''} onChange={(event) => onChange({ fileSize: event.target.value })} /></Field> : null}
        <Field label="引用 / 使用"><input value={record.usage} onChange={(event) => onChange({ usage: event.target.value })} /></Field>
        <Field label="说明"><textarea value={record.description} onChange={(event) => onChange({ description: event.target.value })} /></Field>
        <div className="campaign-expression-preview">
          <span>保存后会更新当前资源行</span>
          <code>{record.identifier}</code>
        </div>
    </Panel>
  );
}

function ResourceAddPanel({
  mode,
  draftName,
  draftType,
  draftSource,
  draftValue,
  uploadQueue,
  onDraftName,
  onDraftType,
  onDraftSource,
  onDraftValue,
  onUploadFiles,
  onUpdateUpload,
  onRemoveUpload,
  onCancel,
  onSave,
}: {
  mode: ResourceTab;
  draftName: string;
  draftType: string;
  draftSource: string;
  draftValue: string;
  uploadQueue: UploadQueueItem[];
  onDraftName: (value: string) => void;
  onDraftType: (value: string) => void;
  onDraftSource: (value: string) => void;
  onDraftValue: (value: string) => void;
  onUploadFiles: (files: FileList | null) => void;
  onUpdateUpload: (id: string, purpose: string) => void;
  onRemoveUpload: (id: string) => void;
  onCancel: () => void;
  onSave: () => void;
}) {
  const invalidUploads = uploadQueue.filter((item) => item.status === '大小错误').length;
  const warningUploads = uploadQueue.filter((item) => item.status === '尺寸提醒').length;
  const totalSize = uploadQueue.reduce((sum, item) => sum + Number(item.sizeLabel.replace(/[^0-9]/g, '') || 0), 0);
  if (mode === '图片') {
    return (
      <Panel
        title="批量上传图片"
        onClose={onCancel}
        bodyClassName="campaign-resource-form"
        footer={<><Button onClick={onCancel}>取消</Button><Button variant="primary" disabled={uploadQueue.length === 0 || invalidUploads > 0} onClick={onSave}>全部保存</Button></>}
      >
          <p>支持 PNG / JPG / WEBP / SVG，单张不超过 5MB</p>
          <label className="campaign-upload-zone">
            <input type="file" multiple accept="image/png,image/jpeg,image/webp,image/svg+xml" onChange={(event) => onUploadFiles(event.target.files)} />
            <span>拖入图片到这里，或点击选择文件</span>
          </label>
          <strong className="campaign-upload-title">上传队列</strong>
          <div className="campaign-upload-table">
            <div><b>文件名</b><b>用途</b><b>推荐尺寸</b><b>状态</b></div>
            {uploadQueue.map((item) => (
              <div key={item.id}>
                <span>{item.fileName}<small>{item.sizeLabel}</small></span>
                <select value={item.purpose} onChange={(event) => onUpdateUpload(item.id, event.target.value)}><option>头图</option><option>活动配图</option><option>图标</option><option>背景图</option><option>分享图</option></select>
                <span>{item.recommendedSize}</span>
                <span className="campaign-upload-state">
                  <span className={`campaign-resource-status ${resourceStatusClass(item.status)}`}>{item.status}</span>
                  <Button size="sm" variant="danger" onClick={() => onRemoveUpload(item.id)}>删除</Button>
                </span>
              </div>
            ))}
          </div>
          <div className="campaign-upload-summary"><span>已选择 {uploadQueue.length} 张</span><span>总大小 {totalSize} KB</span><span>{warningUploads + invalidUploads} 张需处理</span></div>
      </Panel>
    );
  }
  return (
    <Panel
      title={mode === '数据' ? '新增数据字段' : mode === '数据源' ? '新增数据源' : mode === '动作' ? '新增动作事件' : '新增样式 Token'}
      onClose={onCancel}
      bodyClassName="campaign-resource-form"
      footer={<><Button onClick={onCancel}>取消</Button><Button variant="primary" onClick={onSave}>保存</Button></>}
    >
        <Field label={mode === '数据' ? '字段名称' : mode === '数据源' ? '数据源名称' : mode === '动作' ? '动作名称' : '样式名称'} required><input value={draftName} onChange={(event) => onDraftName(event.target.value)} /></Field>
        {mode === '样式' ? <Field label="Token 名称" required><input value="--campaign-button-primary" readOnly /></Field> : null}
        <Field label={mode === '数据' ? '字段类型' : mode === '数据源' ? '数据源类型' : mode === '动作' ? '动作类型' : '样式类型'} required><select value={draftType} onChange={(event) => onDraftType(event.target.value)}><option>文本</option><option>数值</option><option>时间</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Mock JSON</option><option>Workflow Event</option><option>Page Event</option><option>颜色</option><option>圆角</option></select></Field>
        {mode === '数据源' ? <Field label="接口地址" required><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} placeholder="/api/activities/:activityId/view-model" /></Field> : null}
        {mode === '数据源' ? <Field label="请求方式" required><select><option>GET</option><option>POST</option><option>SUBSCRIBE</option><option>LOCAL</option></select></Field> : null}
        <Field label={mode === '数据' ? '数据来源' : mode === '数据源' ? '来源类型' : '来源'} required><select value={draftSource} onChange={(event) => onDraftSource(event.target.value)}><option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option><option>第三方接口</option></select></Field>
        {mode === '数据源' ? <Field label="刷新策略" required><select><option>页面加载 + 30s 轮询</option><option>页面加载</option><option>手动刷新</option><option>WebSocket 实时</option></select></Field> : null}
        {mode === '数据源' ? <Field label="字段映射"><select><option>映射到 Activity View Model</option><option>映射到页面数据字段</option><option>仅用于预览</option></select></Field> : null}
        {mode === '动作' ? <Field label="触发组件" required><select><option>按钮 / 图片 / 卡片 / 页面曝光</option></select></Field> : null}
        {mode === '动作' ? <Field label="Payload Schema" required><textarea value={'{\n  "activityId": "{{ activity.id }}",\n  "rewardId": "{{ reward.id }}"\n}'} readOnly /></Field> : null}
        {mode === '样式' ? <Field label="默认值" required><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} /></Field> : mode !== '数据源' ? <Field label="默认值"><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} placeholder="请输入默认值（可选）" /></Field> : null}
        <Field label={mode === '样式' ? '适用组件' : mode === '数据源' ? '绑定字段' : '绑定组件'}><select><option>头图 / 标题 / 分享卡片</option><option>按钮 / 卡片 / 标题</option><option>activity.title / reward.amount</option></select></Field>
        <Field label="说明"><textarea placeholder="请输入说明（可选）" /></Field>
        {mode === '数据' ? <div className="campaign-expression-preview"><span>表达式预览</span><code>{`{{ ${draftName || 'activity.title'} }}`}</code></div> : null}
        {mode === '数据源' ? <div className="campaign-source-preview"><span>实时拉取预览</span><code>{`GET ${draftValue || '/api/activities/:activityId/view-model'}\n200 OK · 86 ms\n{ "activity": { "title": "夏日充值返利活动" } }`}</code></div> : null}
        {mode === '动作' ? <div className="campaign-expression-preview"><span>验证 Payload</span><b>验证通过</b></div> : null}
        {mode === '样式' ? <div className="campaign-style-preview"><span>样式预览</span><button style={{ background: draftValue || 'var(--theme-accent)' }} type="button">立即领取</button><code>--campaign-button-primary</code></div> : null}
    </Panel>
  );
}
