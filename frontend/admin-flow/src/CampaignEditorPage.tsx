import { useEffect, useMemo, useState, type Dispatch, type PointerEvent as ReactPointerEvent, type SetStateAction } from 'react';
import {
  Bell,
  ChevronDown,
  Copy,
  Database,
  Eye,
  FileText,
  Gift,
  HelpCircle,
  Image,
  ListChecks,
  MonitorSmartphone,
  Plane,
  Plus,
  Rows3,
  Settings,
  Sparkles,
  Trash2,
  Type,
} from 'lucide-react';
import { Render, type Config, type Data } from '@measured/puck';
import '@measured/puck/puck.css';
import './CampaignEditorPage.css';

type CampaignBlockKind = 'HeroBanner' | 'Title' | 'RewardCard' | 'Countdown' | 'ProgressBlock' | 'ClaimButton' | 'RuleText' | 'FAQBlock';
type EditorTab = 'content' | 'binding' | 'action';
type DeviceWidth = 375 | 393 | 414;
type WorkspaceSection = 'page' | 'resources' | 'settings' | 'templates';
type LayoutKind = 'horizontal-layout' | 'vertical-layout' | 'horizontal-center' | 'vertical-center' | 'two-column';
type FlexDirection = 'row' | 'column';
type FlexJustify = 'flex-start' | 'center' | 'flex-end' | 'space-between' | 'space-around';
type FlexAlign = 'flex-start' | 'center' | 'flex-end' | 'stretch';
type FlexWrap = 'nowrap' | 'wrap';

type CampaignBlock = {
  id: string;
  kind: CampaignBlockKind;
  title: string;
  visible: boolean;
  bound?: boolean;
  action?: string;
  text?: string;
  binding?: string;
  visibleWhen?: string;
  payload?: string;
  variant?: 'primary' | 'secondary';
  width?: 'full' | 'content';
  radius?: string;
};

type FreeDesignElement = {
  id: string;
  kind: 'text' | 'button' | 'image' | 'card' | 'layout';
  text: string;
  x: number;
  y: number;
  w: number;
  h: number;
  z: number;
  binding?: string;
  visibleWhen?: string;
  action?: string;
  payload?: string;
  variant?: 'primary' | 'secondary';
  parentId?: string;
  layout?: LayoutKind;
  flexDirection?: FlexDirection;
  justifyContent?: FlexJustify;
  alignItems?: FlexAlign;
  flexWrap?: FlexWrap;
  gap?: number;
  radius?: string;
};

type PuckBlockProps = {
  title?: string;
  text?: string;
  binding?: string;
  visibleWhen?: string;
  action?: string;
  payload?: string;
  variant?: string;
  width?: string;
  radius?: string;
};

type CampaignPageSchema = {
  id: string;
  activityId: string;
  name: string;
  path: string;
  designMode: 'controlled' | 'free';
  device: {
    defaultWidth: DeviceWidth;
    supportedWidths: DeviceWidth[];
  };
  theme: {
    primaryToken: string;
    cardRadius: number;
    contentGap: number;
    fontFamily: string;
  };
  blocks: CampaignBlock[];
  freeElements: FreeDesignElement[];
  status: 'DRAFT' | 'PUBLISHED' | 'OFFLINE';
  version: string;
  updatedAt: string;
};

type CampaignPageVersion = {
  id: string;
  pageId: string;
  version: string;
  status: 'PUBLISHED';
  schemaSnapshot: CampaignPageSchema;
  publishedBy: string;
  publishedAt: string;
};

type CampaignPageSummary = {
  id: string;
  name: string;
  status: 'DRAFT' | 'PUBLISHED' | 'OFFLINE';
  version: string;
  path: string;
  updatedAt: string;
  createdBy: string;
};

type CampaignValidationIssue = {
  code: string;
  message: string;
};

type WorkspaceItem = { title: string; meta: string; detail: string; group: string; tags: string[] };
type ResourceTab = '数据' | '数据源' | '动作' | '样式' | '图片';
type ResourceStatus = '启用' | '停用' | '待保存' | '尺寸提醒' | '大小错误';

type ResourceRecord = {
  id: string;
  tab: ResourceTab;
  category: string;
  name: string;
  identifier: string;
  type: string;
  source: string;
  usage: string;
  status: ResourceStatus;
  updatedAt: string;
  description: string;
  size?: string;
  fileSize?: string;
  thumbnail?: string;
  value?: string;
  endpoint?: string;
  method?: string;
  refresh?: string;
  latency?: string;
  tags: string[];
};

type UploadQueueItem = {
  id: string;
  fileName: string;
  purpose: string;
  recommendedSize: string;
  status: ResourceStatus;
  sizeLabel: string;
};

type PageTreeNode =
  | { id: string; type: 'block'; title: string; meta: string; block: CampaignBlock; children?: PageTreeNode[] }
  | { id: string; type: 'layout'; title: string; meta: string; element: FreeDesignElement; children?: PageTreeNode[] };

const LAYOUT_CHILD_HORIZONTAL_INSET = 26;

function layoutChildWidth(layout: FreeDesignElement) {
  return Math.max(0, layout.w - LAYOUT_CHILD_HORIZONTAL_INSET);
}

const navItems = [
  { id: 'page', label: '页面', icon: FileText },
  { id: 'resources', label: '资源', icon: Database },
  { id: 'settings', label: '设置', icon: Settings },
  { id: 'templates', label: '模板库', icon: Sparkles },
];

const workspaceConfig: Record<Exclude<WorkspaceSection, 'page'>, {
  groups: string[];
  items: WorkspaceItem[];
  detailTitle: string;
}> = {
  resources: {
    groups: ['数据', '动作', '样式', '图片'],
    detailTitle: '资源详情',
    items: [
      { title: 'activity.title', meta: '数据 · 文本 · 示例：夏日充值返利活动', detail: '活动标题，用于头图、标题和分享卡片。', group: '数据', tags: ['Activity View Model', 'HeroBanner', 'Title'] },
      { title: 'reward.amount', meta: '数据 · 金额 · 示例：¥188', detail: '奖励金额，格式化后展示在奖励卡片。', group: '数据', tags: ['RewardCard', '默认值：--'] },
      { title: 'eligibility.canClaim', meta: '数据 · 布尔 · 示例：true', detail: '控制领取按钮禁用、显示条件和领取状态。', group: '数据', tags: ['ClaimButton', '{{eligibility.canClaim}}'] },
      { title: 'progress.percent', meta: '数据 · 数字 · 示例：62', detail: '活动参与进度，可绑定到进度条或任务状态。', group: '数据', tags: ['ProgressBlock'] },
      { title: 'CLAIM_REWARD', meta: '动作 · 触发组件：领取按钮', detail: '点击后调用活动事件 API，触发夏日充值返利流程。', group: '动作', tags: ['rewardId -> reward.id', 'activityId -> activity.id'] },
      { title: 'OPEN_LOGIN', meta: '动作 · 触发组件：登录提示', detail: '未登录用户打开登录入口，成功后刷新页面数据。', group: '动作', tags: ['登录态', '刷新数据'] },
      { title: 'NAVIGATE', meta: '动作 · 页面跳转', detail: '跳转到活动规则、账户页或外部落地页。', group: '动作', tags: ['白名单 URL'] },
      { title: 'TRACK_EVENT', meta: '动作 · 埋点事件', detail: '记录运营点击、曝光和转化链路。', group: '动作', tags: ['claim_reward_click'] },
      { title: '主色', meta: '样式 · Primary · theme accent', detail: '影响 CTA、选中描边和链接，不改变活动素材颜色。', group: '样式', tags: ['--theme-accent'] },
      { title: '页面背景', meta: '样式 · Canvas · 移动端活动页', detail: '用于活动页面底色，跟随主题 token 管理。', group: '样式', tags: ['campaign token'] },
      { title: '主按钮样式', meta: '样式 · 全宽 · 8px 圆角', detail: '领取按钮和关键 CTA 使用统一按钮样式。', group: '样式', tags: ['高度 48px', '点击区域正常'] },
      { title: '奖励卡片', meta: '样式 · 8px 圆角 · 细边框', detail: '用于奖励、进度、规则等信息容器。', group: '样式', tags: ['活动素材色不染色'] },
      { title: 'hero-summer-rebate.png', meta: '图片 · 头图素材 · 750x360', detail: '活动首屏头图，可拖入页面或绑定到头图组件。', group: '图片', tags: ['HeroBanner', '移动端 2x'] },
      { title: 'reward-card-bg.png', meta: '图片 · 奖励卡背景 · 690x240', detail: '奖励卡可复用背景图，用于强化金额和权益展示。', group: '图片', tags: ['RewardCard', '可裁剪'] },
      { title: 'share-cover.png', meta: '图片 · 分享封面 · 1:1', detail: '用于微信、Telegram 或活动链接预览。', group: '图片', tags: ['分享卡片'] },
    ],
  },
  settings: {
    groups: ['基础信息', '发布', '分享', '权限'],
    detailTitle: '发布预览',
    items: [
      { title: '页面名称', meta: '夏日充值返利活动', detail: '活动页名称，展示在编辑器和发布版本里。', group: '基础信息', tags: ['page_20250520_001'] },
      { title: '页面路径', meta: '/campaign/summer-rebate', detail: '发布后的访问路径。', group: '基础信息', tags: ['唯一路径'] },
      { title: '上线时间', meta: '2025-05-20 00:00:00', detail: '发布计划开始时间。', group: '发布', tags: ['Asia/Shanghai'] },
      { title: '发布环境', meta: 'Production · 待发布', detail: '发布前校验 schema、绑定字段、动作权限和移动端溢出。', group: '发布', tags: ['DRAFT'] },
      { title: '分享卡片', meta: '标题 + 封面图', detail: '用于运营验收移动端分享预览。', group: '分享', tags: ['复制链接'] },
      { title: '编辑权限', meta: '运营团队', detail: '没有发布权限时不能发布。', group: '权限', tags: ['Owner / Editor'] },
    ],
  },
  templates: {
    groups: ['全部', '充值返利', '签到任务', '邀请裂变', '优惠券', '新手活动', '排行榜'],
    detailTitle: '模板详情',
    items: [
      { title: '夏日充值返利', meta: '充值返利 · 5 字段 · 1 动作', detail: '包含头图、奖励卡、倒计时、参与进度、领取按钮、规则说明。', group: '充值返利', tags: ['activity.title', 'reward.amount', 'CLAIM_REWARD'] },
      { title: '每日签到领豪礼', meta: '签到任务 · 4 字段 · 2 动作', detail: '适合连续签到和每日奖励。', group: '签到任务', tags: ['OPEN_LOGIN', 'TRACK_EVENT'] },
      { title: '邀请好友奖励', meta: '邀请裂变 · 6 字段 · 2 动作', detail: '包含邀请码、奖励进度和邀请按钮。', group: '邀请裂变', tags: ['NAVIGATE'] },
      { title: '优惠券领取', meta: '优惠券 · 4 字段 · 1 动作', detail: '适合券包领取和权益展示。', group: '优惠券', tags: ['CLAIM_REWARD'] },
      { title: '新手任务礼包', meta: '新手活动 · 7 字段 · 2 动作', detail: '包含任务列表和阶段奖励。', group: '新手活动', tags: ['TRACK_EVENT'] },
      { title: '排行榜冲刺', meta: '排行榜 · 5 字段 · 1 动作', detail: '适合活动榜单冲刺页。', group: '排行榜', tags: ['progress.rank'] },
    ],
  },
};

const blockIcons: Record<CampaignBlockKind, typeof Image> = {
  HeroBanner: Image,
  Title: Type,
  RewardCard: Gift,
  Countdown: MonitorSmartphone,
  ProgressBlock: ListChecks,
  ClaimButton: Plane,
  RuleText: FileText,
  FAQBlock: HelpCircle,
};

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

const defaultResourceRecords: ResourceRecord[] = [
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

const blockPalette: Array<{ kind: CampaignBlockKind; label: string; title: string }> = [
  { kind: 'HeroBanner', label: '头图', title: '新头图' },
  { kind: 'Title', label: '标题', title: '新标题' },
  { kind: 'RewardCard', label: '奖励卡', title: '新奖励卡片' },
  { kind: 'Countdown', label: '倒计时', title: '新倒计时' },
  { kind: 'ProgressBlock', label: '进度', title: '新参与进度' },
  { kind: 'ClaimButton', label: '按钮', title: '新领取按钮' },
  { kind: 'RuleText', label: '规则', title: '新活动规则' },
  { kind: 'FAQBlock', label: 'FAQ', title: '新 FAQ' },
];

const layoutPalette: Array<{ kind: LayoutKind; label: string; title: string; icon: typeof Rows3 }> = [
  { kind: 'horizontal-layout', label: '水平布局', title: '水平布局盒子', icon: Rows3 },
  { kind: 'vertical-layout', label: '垂直布局', title: '垂直布局盒子', icon: Rows3 },
];

function flexPreset(kind: LayoutKind): Pick<FreeDesignElement, 'flexDirection' | 'justifyContent' | 'alignItems' | 'flexWrap' | 'gap'> {
  if (kind === 'vertical-layout' || kind === 'vertical-center') return { flexDirection: 'column', justifyContent: 'flex-start', alignItems: 'stretch', flexWrap: 'nowrap', gap: 8 };
  if (kind === 'two-column') return { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'stretch', flexWrap: 'nowrap', gap: 10 };
  return { flexDirection: 'row', justifyContent: 'flex-start', alignItems: 'center', flexWrap: 'nowrap', gap: 8 };
}

function normalizeLayoutKind(kind?: LayoutKind): 'horizontal-layout' | 'vertical-layout' {
  return kind === 'vertical-layout' || kind === 'vertical-center' ? 'vertical-layout' : 'horizontal-layout';
}

const puckConfig: Config<Record<CampaignBlockKind, PuckBlockProps>> = {
  components: {
    HeroBanner: {
      fields: {
        title: { type: 'text' },
        text: { type: 'text' },
      },
      render: ({ title, text }) => <div className="campaign-puck-block hero"><strong>{title || '夏日充值返利活动'}</strong><span>{text || '充值越多，返利越高'}</span></div>,
    },
    Title: {
      fields: {
        title: { type: 'text' },
      },
      render: ({ title }) => <h3 className="campaign-puck-block title">{title || '活动标题'}</h3>,
    },
    RewardCard: {
      fields: {
        title: { type: 'text' },
        text: { type: 'text' },
      },
      render: ({ title, text }) => <div className="campaign-puck-block card"><strong>{title || '奖励卡片'}</strong><span>{text || '￥188 · 单笔充值满 500 元可领'}</span></div>,
    },
    Countdown: {
      fields: {
        title: { type: 'text' },
      },
      render: ({ title }) => <div className="campaign-puck-block card"><strong>{title || '倒计时'}</strong><span>06 天 18 时 32 分 45 秒</span></div>,
    },
    ProgressBlock: {
      fields: {
        title: { type: 'text' },
      },
      render: ({ title }) => <div className="campaign-puck-block card"><strong>{title || '参与进度'}</strong><span>已参与 1,258 人 · 62%</span></div>,
    },
    ClaimButton: {
      fields: {
        title: { type: 'text' },
        text: { type: 'text' },
        binding: { type: 'text' },
        visibleWhen: { type: 'text' },
        action: {
          type: 'select',
          options: [
            { label: 'CLAIM_REWARD', value: 'CLAIM_REWARD' },
            { label: 'OPEN_LOGIN', value: 'OPEN_LOGIN' },
            { label: 'NAVIGATE', value: 'NAVIGATE' },
            { label: 'TRACK_EVENT', value: 'TRACK_EVENT' },
          ],
        },
      },
      render: ({ text }) => <button className="campaign-puck-claim" type="button">{text || '立即领取'}</button>,
    },
    RuleText: {
      fields: {
        text: { type: 'textarea' },
      },
      render: ({ text }) => <div className="campaign-puck-block card"><strong>活动规则</strong><span>{text || '活动期间，单笔充值满 500 元可领取奖励。'}</span></div>,
    },
    FAQBlock: {
      fields: {
        text: { type: 'textarea' },
      },
      render: ({ text }) => <div className="campaign-puck-block card"><strong>FAQ</strong><span>{text || '奖励到账时间、参与资格和领取问题。'}</span></div>,
    },
  },
};

function campaignToPuckData(page: CampaignPageSchema): Data {
  return {
    root: {},
    content: page.blocks.map((block) => ({
      type: block.kind,
      props: {
        id: block.id,
        title: block.title,
        text: block.text,
        binding: block.binding,
        visibleWhen: block.visibleWhen,
        action: block.action,
        payload: block.payload,
        variant: block.variant,
        width: block.width,
        radius: block.radius,
      },
    })),
  };
}

const initialBlocks: CampaignBlock[] = [
  { id: 'hero', kind: 'HeroBanner', title: '头图', visible: true, bound: true },
  { id: 'title', kind: 'Title', title: '活动标题', visible: true, bound: true },
  { id: 'reward', kind: 'RewardCard', title: '奖励卡片', visible: true, bound: true },
  { id: 'countdown', kind: 'Countdown', title: '倒计时', visible: true, bound: true },
  { id: 'progress', kind: 'ProgressBlock', title: '参与进度', visible: true, bound: true },
  { id: 'claim', kind: 'ClaimButton', title: '领取按钮', visible: true, bound: true, action: 'CLAIM_REWARD', text: '立即领取', binding: '{{ !eligibility.canClaim }}', visibleWhen: '{{ user.loggedIn }}', payload: '{{ activity.id, reward.code, user.id }}', variant: 'primary', width: 'full', radius: '8' },
  { id: 'rules', kind: 'RuleText', title: '活动规则', visible: true, bound: true },
  { id: 'faq', kind: 'FAQBlock', title: 'FAQ', visible: false, bound: false },
];

const initialFreeElements: FreeDesignElement[] = [
  { id: 'free_hero_title', kind: 'text', text: '夏日充值返利活动', x: 28, y: 36, w: 238, h: 58, z: 1, binding: '{{ activity.title }}' },
  { id: 'free_reward_card', kind: 'card', text: '奖励金额 ￥188\\n单笔充值满 500 元可领', x: 18, y: 132, w: 345, h: 116, z: 2, binding: '{{ reward.amount }}', radius: '8' },
  { id: 'free_claim', kind: 'button', text: '立即领取', x: 18, y: 282, w: 345, h: 52, z: 3, binding: '{{ !eligibility.canClaim }}', visibleWhen: '{{ user.loggedIn }}', action: 'CLAIM_REWARD', payload: '{{ activity.id, reward.code, user.id }}', variant: 'primary', radius: '8' },
];

function blockToFreeText(block: CampaignBlock) {
  if (block.kind === 'HeroBanner') return '夏日充值返利活动\\n充值越多，返利越高';
  if (block.kind === 'Title') return block.title === '活动标题' ? '夏日充值返利活动' : block.title;
  if (block.kind === 'RewardCard') return '奖励金额 ￥188\\n单笔充值满 500 元可领';
  if (block.kind === 'Countdown') return '活动倒计时\\n06 天 18 时 32 分';
  if (block.kind === 'ProgressBlock') return '参与进度\\n已参与 1,258 人 · 62%';
  if (block.kind === 'ClaimButton') return block.text ?? block.title;
  if (block.kind === 'RuleText') return '活动规则\\n1. 单笔充值满 500 元可领取奖励\\n2. 每个用户仅可领取一次';
  return 'FAQ\\n奖励到账时间、参与资格和领取问题。';
}

function blockToFreeKind(kind: CampaignBlockKind): FreeDesignElement['kind'] {
  if (kind === 'ClaimButton') return 'button';
  if (kind === 'HeroBanner' || kind === 'RewardCard' || kind === 'Countdown' || kind === 'ProgressBlock' || kind === 'RuleText' || kind === 'FAQBlock') return 'card';
  return 'text';
}

function legacyFreeElementIdForBlock(block: CampaignBlock) {
  if (block.id === 'title') return 'free_hero_title';
  if (block.id === 'reward') return 'free_reward_card';
  if (block.id === 'claim') return 'free_claim';
  return undefined;
}

function blockIdForFreeElementId(id: string) {
  if (id.startsWith('free_from_')) return id.replace('free_from_', '');
  if (id === 'free_hero_title') return 'title';
  if (id === 'free_reward_card') return 'reward';
  if (id === 'free_claim') return 'claim';
  return '';
}

function currentFreeElementIdForBlock(blockId: string, elements: FreeDesignElement[]) {
  return elements.find((element) => blockIdForFreeElementId(element.id) === blockId)?.id;
}

function blocksToFreeElements(blocks: CampaignBlock[], width: DeviceWidth): FreeDesignElement[] {
  let cursorY = 28;
  return blocks.filter((block) => block.visible).map((block, index) => {
    const kind = blockToFreeKind(block.kind);
    const h = kind === 'button' ? 52 : block.kind === 'Title' ? 46 : block.kind === 'HeroBanner' ? 120 : block.kind === 'RuleText' ? 132 : 94;
    const w = kind === 'text' ? Math.min(260, width - 56) : width - 36;
    const element: FreeDesignElement = {
      id: `free_from_${block.id}`,
      kind,
      text: blockToFreeText(block),
      x: kind === 'text' ? 28 : 18,
      y: cursorY,
      w,
      h,
      z: index + 1,
      binding: block.binding,
      visibleWhen: block.visibleWhen,
      action: block.action,
      payload: block.payload,
      variant: block.variant,
      radius: block.radius ?? '8',
    };
    cursorY += h + 18;
    return element;
  });
}

function syncFreeElementsWithBlocks(blocks: CampaignBlock[], width: DeviceWidth, currentFreeElements: FreeDesignElement[]) {
  const visibleBlocks = blocks.filter((block) => block.visible);
  const visibleBlockById = new Map(visibleBlocks.map((block) => [block.id, block]));
  const existingById = new Map(currentFreeElements.map((element) => [element.id, element]));
  const visibleFreeIds = new Set(visibleBlocks.map((block) => `free_from_${block.id}`));
  const nonBlockElements = currentFreeElements.filter((element) => !element.id.startsWith('free_from_'));
  const legacySyncedElements = nonBlockElements.flatMap((element) => {
    const blockId = blockIdForFreeElementId(element.id);
    const block = blockId ? visibleBlockById.get(blockId) : undefined;
    if (blockId && !block) return [];
    if (!block) return [element];
    const fresh = blockToFreeElement(block, width, element.z);
    const parent = element.parentId ? currentFreeElements.find((item) => item.id === element.parentId && item.kind === 'layout') : undefined;
    return [{
      ...fresh,
      ...element,
      text: fresh.text,
      binding: fresh.binding,
      visibleWhen: fresh.visibleWhen,
      action: fresh.action,
      payload: fresh.payload,
      variant: fresh.variant,
      radius: fresh.radius,
      w: parent ? layoutChildWidth(parent) : element.w,
    }];
  });
  const legacyBlockIds = new Set(legacySyncedElements.map((element) => blockIdForFreeElementId(element.id)).filter(Boolean));
  const defaultBlockIds = new Set(initialBlocks.map((block) => block.id));
  const blockElements = visibleBlocks.flatMap((block, index) => {
    if (legacyBlockIds.has(block.id)) return [];
    const fresh = blockToFreeElement(block, width, index + 1);
    const existing = existingById.get(fresh.id);
    if (!existing && defaultBlockIds.has(block.id)) return [];
    if (!existing) return [fresh];
    const parent = existing.parentId ? currentFreeElements.find((element) => element.id === existing.parentId && element.kind === 'layout') : undefined;
    return [{
      ...fresh,
      ...existing,
      text: fresh.text,
      binding: fresh.binding,
      visibleWhen: fresh.visibleWhen,
      action: fresh.action,
      payload: fresh.payload,
      variant: fresh.variant,
      radius: fresh.radius,
      w: parent ? layoutChildWidth(parent) : existing.w,
    }];
  });
  return fitLayouts([
    ...legacySyncedElements.filter((element) => !element.parentId || !element.parentId.startsWith('free_from_')),
    ...blockElements.filter((element) => visibleFreeIds.has(element.id)),
  ]);
}

function blockToFreeElement(block: CampaignBlock, width: DeviceWidth, z: number): FreeDesignElement {
  const kind = blockToFreeKind(block.kind);
  const h = kind === 'button' ? 52 : block.kind === 'Title' ? 46 : block.kind === 'HeroBanner' ? 120 : block.kind === 'RuleText' ? 132 : 94;
  const w = kind === 'text' ? Math.min(260, width - 56) : width - 36;
  return {
    id: `free_from_${block.id}`,
    kind,
    text: blockToFreeText(block),
    x: kind === 'text' ? 28 : 18,
    y: 28,
    w,
    h,
    z,
    binding: block.binding,
    visibleWhen: block.visibleWhen,
    action: block.action,
    payload: block.payload,
    variant: block.variant,
    radius: block.radius ?? '8',
  };
}

function findContainingLayout(elements: FreeDesignElement[], x: number, y: number) {
  return [...elements]
    .filter((element) => !element.parentId && element.kind === 'layout' && x >= element.x && x <= element.x + element.w && y >= element.y && y <= element.y + element.h)
    .sort((a, b) => b.z - a.z)[0];
}

function fitLayoutToChildren(layout: FreeDesignElement, children: FreeDesignElement[]) {
  if (layout.kind !== 'layout' || children.length === 0) return layout;
  const gap = layout.gap ?? 8;
  const padding = 24;
  const direction = layout.flexDirection ?? (normalizeLayoutKind(layout.layout) === 'vertical-layout' ? 'column' : 'row');
  const width = direction === 'column'
    ? Math.max(...children.map((child) => child.w))
    : children.reduce((sum, child) => sum + child.w, 0) + gap * Math.max(0, children.length - 1) + padding;
  const height = direction === 'column'
    ? children.reduce((sum, child) => sum + child.h, 0) + gap * Math.max(0, children.length - 1) + padding
    : Math.max(...children.map((child) => child.h)) + padding;
  return { ...layout, w: Math.max(layout.w, Math.ceil(width)), h: Math.max(layout.h, Math.ceil(height)) };
}

function fitLayouts(elements: FreeDesignElement[]) {
  return elements.map((element) => element.kind === 'layout'
    ? fitLayoutToChildren(element, elements.filter((child) => child.parentId === element.id))
    : element);
}

function freeElementStyle(element: FreeDesignElement, nested = false) {
  return {
    left: element.x,
    top: element.y,
    width: element.w,
    height: element.h,
    ...(nested ? {} : { zIndex: element.z }),
    borderRadius: Number(element.radius ?? 8),
    ...(element.kind === 'layout' ? {
      flexDirection: element.flexDirection ?? 'row',
      justifyContent: element.justifyContent ?? 'center',
      alignItems: element.alignItems ?? 'center',
      flexWrap: element.flexWrap ?? 'nowrap',
      gap: element.gap ?? 8,
    } : {}),
  };
}

function freeCanvasHeight(elements: FreeDesignElement[], minHeight = 760) {
  const topLevelElements = elements.filter((element) => !element.parentId);
  return Math.max(minHeight, ...topLevelElements.map((element) => element.y + element.h + 96));
}

function renderFreeElementText(text: string) {
  return text.split('\\n').map((line, index) => <span key={`${index}:${line}`}>{line}</span>);
}

const CAMPAIGN_PAGE_STORAGE_KEY = 'exmarketing.campaignEditor.page.v1';
const CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY = 'exmarketing.campaignEditor.publishedPage.v1';
const CAMPAIGN_VERSION_STORAGE_KEY = 'exmarketing.campaignEditor.versions.v1';
const CAMPAIGN_PAGE_LIST_STORAGE_KEY = 'exmarketing.campaignEditor.pages.v1';
const ALLOWED_ACTIONS = new Set(['CLAIM_REWARD', 'OPEN_LOGIN', 'NAVIGATE', 'TRACK_EVENT']);
const ALLOWED_BLOCK_KINDS = new Set<CampaignBlockKind>(['HeroBanner', 'Title', 'RewardCard', 'Countdown', 'ProgressBlock', 'ClaimButton', 'RuleText', 'FAQBlock']);
const ALLOWED_LAYOUT_KINDS = new Set<LayoutKind>(['horizontal-layout', 'vertical-layout']);

function nowISO() {
  return new Date().toISOString();
}

function defaultCampaignPage(): CampaignPageSchema {
  return {
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
  };
}

function readJSON<T>(key: string, fallback: T): T {
  if (typeof window === 'undefined') return fallback;
  try {
    const value = window.localStorage.getItem(key);
    return value ? JSON.parse(value) as T : fallback;
  } catch {
    return fallback;
  }
}

function writeJSON(key: string, value: unknown) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(key, JSON.stringify(value));
}

const campaignPageApi = {
  loadPage: () => {
    const fallback = defaultCampaignPage();
    const stored = readJSON<CampaignPageSchema>(CAMPAIGN_PAGE_STORAGE_KEY, fallback);
    return {
      ...fallback,
      ...stored,
      designMode: stored.designMode ?? 'controlled',
      freeElements: stored.freeElements ?? initialFreeElements,
    };
  },
  loadPublishedPage: () => readJSON<CampaignPageSchema | null>(CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY, null),
  saveDraft: (page: CampaignPageSchema) => writeJSON(CAMPAIGN_PAGE_STORAGE_KEY, page),
  savePublishedPage: (page: CampaignPageSchema) => writeJSON(CAMPAIGN_PUBLISHED_PAGE_STORAGE_KEY, page),
  savePageList: (items: CampaignPageSummary[]) => writeJSON(CAMPAIGN_PAGE_LIST_STORAGE_KEY, items),
  saveVersions: (items: CampaignPageVersion[]) => writeJSON(CAMPAIGN_VERSION_STORAGE_KEY, items),
  loadVersions: () => readJSON<CampaignPageVersion[]>(CAMPAIGN_VERSION_STORAGE_KEY, []),
  publish: (page: CampaignPageSchema, versions: CampaignPageVersion[]) => {
    const versionLabel = nextVersionLabel(versions);
    const publishedPage: CampaignPageSchema = { ...page, status: 'PUBLISHED', version: versionLabel, updatedAt: nowISO() };
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
};

function pageSummary(page: CampaignPageSchema): CampaignPageSummary {
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

function nextVersionLabel(versions: CampaignPageVersion[]) {
  return `v1.0.${versions.length + 1}`;
}

function validateBindingExpression(value: string | undefined): boolean {
  if (!value) return true;
  return /^\{\{\s*!?[a-z]+(?:\.[A-Za-z0-9_]+)+\s*\}\}$/.test(value.trim());
}

function validateCampaignPage(page: CampaignPageSchema): CampaignValidationIssue[] {
  const issues: CampaignValidationIssue[] = [];
  if (!page.name.trim()) issues.push({ code: 'PAGE_NAME_REQUIRED', message: '页面名称不能为空。' });
  if (!page.path.startsWith('/campaign/')) issues.push({ code: 'PAGE_PATH_INVALID', message: '页面路径必须以 /campaign/ 开头。' });
  if (!page.blocks.length) issues.push({ code: 'PAGE_BLOCKS_REQUIRED', message: '页面至少需要一个内容块。' });
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

function blockDescription(kind: CampaignBlockKind) {
  if (kind === 'ClaimButton') return '按钮文案、禁用条件、点击动作';
  if (kind === 'HeroBanner') return '活动主视觉、标题、副标题、时间';
  if (kind === 'RewardCard') return '奖励金额、领取条件、奖品图';
  if (kind === 'ProgressBlock') return '参与人数、目标人数、进度比例';
  return '内容、绑定字段和显示状态';
}

function moveBlock(blocks: CampaignBlock[], id: string, direction: -1 | 1) {
  const index = blocks.findIndex((block) => block.id === id);
  const nextIndex = index + direction;
  if (index < 0 || nextIndex < 0 || nextIndex >= blocks.length) return blocks;
  const next = [...blocks];
  [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
  return next;
}

export function CampaignEditorPage() {
  const [pageSchema, setPageSchema] = useState<CampaignPageSchema>(() => {
    const loadedPage = campaignPageApi.loadPage();
    return {
      ...loadedPage,
      designMode: 'free',
      freeElements: loadedPage.freeElements?.length
        ? loadedPage.freeElements
        : blocksToFreeElements(loadedPage.blocks, loadedPage.device.defaultWidth),
    };
  });
  const [resourceRecords, setResourceRecords] = useState<ResourceRecord[]>(defaultResourceRecords);
  const [versions, setVersions] = useState<CampaignPageVersion[]>(() => campaignPageApi.loadVersions());
  const [pages, setPages] = useState<CampaignPageSummary[]>(() => readJSON(CAMPAIGN_PAGE_LIST_STORAGE_KEY, [pageSummary(readJSON(CAMPAIGN_PAGE_STORAGE_KEY, defaultCampaignPage()))]));
  const [selectedId, setSelectedId] = useState('claim');
  const [inspectorTab, setInspectorTab] = useState<EditorTab>('content');
  const [deviceWidth, setDeviceWidth] = useState<DeviceWidth>(pageSchema.device.defaultWidth);
  const [activeSection, setActiveSection] = useState<WorkspaceSection>('page');
  const [saveMessage, setSaveMessage] = useState('已自动保存');
  const [previewOpen, setPreviewOpen] = useState(false);
  const [validationIssues, setValidationIssues] = useState<CampaignValidationIssue[]>([]);
  const [pageEditorMode, setPageEditorMode] = useState<'controlled' | 'free'>('free');
  const [selectedFreeId, setSelectedFreeId] = useState(() => pageSchema.freeElements?.[0]?.id ?? 'free_from_hero');
  const [draggingBlockKind, setDraggingBlockKind] = useState<CampaignBlockKind | null>(null);
  const [draggingLayoutKind, setDraggingLayoutKind] = useState<LayoutKind | null>(null);
  const [treeQuery, setTreeQuery] = useState('');
  const [treeOpen, setTreeOpen] = useState(true);
  const [libraryOpen, setLibraryOpen] = useState(true);
  const blocks = pageSchema.blocks;
  const freeElements = pageSchema.freeElements ?? initialFreeElements;
  const selectedBlock = blocks.find((block) => block.id === selectedId) ?? blocks[0];
  const selectedFreeElement = freeElements.find((element) => element.id === selectedFreeId) ?? freeElements[0];
  const visibleBlocks = useMemo(() => blocks.filter((block) => block.visible), [blocks]);
  const treeNodes = useMemo<PageTreeNode[]>(() => {
    const keyword = treeQuery.trim().toLowerCase();
    const freeByBlockId = new Map<string, FreeDesignElement>(freeElements.flatMap((element): Array<[string, FreeDesignElement]> => {
      const blockId = blockIdForFreeElementId(element.id);
      return blockId ? [[blockId, element]] : [];
    }));
    const blockNodes: PageTreeNode[] = blocks.map((block) => ({ id: block.id, type: 'block' as const, title: block.title, meta: block.kind, block }));
    const blockNodeById = new Map(blockNodes.map((node) => [node.id, node]));
    const blockNodesByFreeId = new Map<string, PageTreeNode>();
    for (const block of blocks) {
      const node = blockNodeById.get(block.id);
      if (!node) continue;
      blockNodesByFreeId.set(`free_from_${block.id}`, node);
      const legacyId = legacyFreeElementIdForBlock(block);
      if (legacyId) blockNodesByFreeId.set(legacyId, node);
    }
    const layouts = freeElements
      .filter((element) => element.kind === 'layout')
      .map((element) => ({
        id: element.id,
        type: 'layout' as const,
        title: element.text.split('\\n')[0] || '布局盒子',
        meta: normalizeLayoutKind(element.layout) === 'vertical-layout' ? 'VerticalLayout' : 'HorizontalLayout',
        element,
        children: freeElements
          .filter((child) => child.parentId === element.id)
          .map((child) => blockNodesByFreeId.get(child.id))
          .filter((node): node is PageTreeNode => Boolean(node)),
      }));
    const parentedBlockIds = new Set(freeElements.filter((element) => element.parentId).map((element) => blockIdForFreeElementId(element.id)).filter(Boolean));
    const nodes: PageTreeNode[] = [
      ...blockNodes.filter((node) => !parentedBlockIds.has(node.id) && !freeByBlockId.get(node.id)?.parentId),
      ...layouts,
    ];
    const matches = (node: PageTreeNode): boolean => `${node.title} ${node.meta}`.toLowerCase().includes(keyword);
    if (!keyword) return nodes;
    return nodes
      .map((node) => node.children?.length ? { ...node, children: node.children.filter((child) => matches(child)) } : node)
      .filter((node) => matches(node) || Boolean(node.children?.length));
  }, [blocks, freeElements, treeQuery]);
  const SelectedIcon = selectedBlock ? blockIcons[selectedBlock.kind] : Type;

  const updatePage = (patch: Partial<CampaignPageSchema> | ((current: CampaignPageSchema) => CampaignPageSchema)) => {
    setPageSchema((current) => {
      const next = typeof patch === 'function' ? patch(current) : { ...current, ...patch };
      const nextPage = { ...next, updatedAt: nowISO() };
      campaignPageApi.saveDraft(nextPage);
      return nextPage;
    });
  };

  const updateBlocks = (updater: (current: CampaignBlock[]) => CampaignBlock[]) => {
    updatePage((current) => {
      const blocks = updater(current.blocks);
      return {
        ...current,
        blocks,
        freeElements: syncFreeElementsWithBlocks(blocks, current.device.defaultWidth, current.freeElements ?? initialFreeElements),
        designMode: current.designMode,
        status: 'DRAFT',
      };
    });
  };

  const updateFreeElements = (updater: (current: FreeDesignElement[]) => FreeDesignElement[]) => {
    updatePage((current) => ({ ...current, freeElements: updater(current.freeElements ?? initialFreeElements), designMode: 'free', status: 'DRAFT' }));
  };

  const updateSelectedFreeElement = (patch: Partial<FreeDesignElement>) => {
    updateFreeElements((items) => {
      const selected = items.find((element) => element.id === selectedFreeId);
      return fitLayouts(items.map((element) => {
        if (element.id === selectedFreeId) {
          const next = { ...element, ...patch };
          const parent = next.parentId ? items.find((item) => item.id === next.parentId) : undefined;
          const maxX = Math.max(0, (parent?.w ?? pageSchema.device.defaultWidth) - next.w);
          return {
            ...next,
            x: Math.min(maxX, Math.max(0, next.x)),
            y: Math.max(0, next.y),
          };
        }
        if (selected?.kind === 'layout' && element.parentId === selected.id && patch.w !== undefined) {
          return { ...element, x: 0, w: layoutChildWidth({ ...selected, w: Math.max(0, Number(patch.w)) }) };
        }
        return element;
      }));
    });
  };

  const moveFreeElementIntoLayout = (freeElementId: string, layoutId: string) => {
    updateFreeElements((items) => {
      const parent = items.find((item) => item.id === layoutId && item.kind === 'layout');
      const existingChild = items.find((item) => item.id === freeElementId && item.kind !== 'layout');
      const blockId = blockIdForFreeElementId(freeElementId);
      const sourceBlock = blockId ? blocks.find((block) => block.id === blockId) : undefined;
      if (!parent || (!existingChild && !sourceBlock)) return items;
      const child = existingChild ?? blockToFreeElement(sourceBlock!, pageSchema.device.defaultWidth, Math.max(0, ...items.map((item) => item.z)) + 1);
      const sourceItems = existingChild ? items : [...items, child];
      const next = sourceItems.map((item) => item.id === freeElementId ? {
        ...item,
        parentId: parent.id,
        x: 0,
        y: Math.max(0, item.parentId === parent.id ? item.y : 0),
        w: layoutChildWidth(parent),
      } : item);
      return fitLayouts(next);
    });
    setSelectedFreeId(freeElementId);
  };

  const moveFreeElementToRoot = (freeElementId: string) => {
    updateFreeElements((items) => {
      const child = items.find((item) => item.id === freeElementId && item.kind !== 'layout');
      const parent = child?.parentId ? items.find((item) => item.id === child.parentId && item.kind === 'layout') : undefined;
      if (!child || !parent) return items;
      const maxX = Math.max(0, pageSchema.device.defaultWidth - child.w);
      const next = items.map((item) => item.id === freeElementId ? {
        ...item,
        parentId: undefined,
        x: Math.min(maxX, Math.max(0, parent.x + item.x)),
        y: Math.max(0, parent.y + parent.h + 18),
      } : item);
      return fitLayouts(next);
    });
    setSelectedFreeId(freeElementId);
  };

  useEffect(() => {
    campaignPageApi.saveDraft(pageSchema);
    const nextPages = [pageSummary(pageSchema), ...pages.filter((page) => page.id !== pageSchema.id)];
    setPages(nextPages);
    campaignPageApi.savePageList(nextPages);
    setSaveMessage(`已自动保存 ${new Date(pageSchema.updatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageSchema]);

  useEffect(() => {
    campaignPageApi.saveVersions(versions);
  }, [versions]);

  const duplicateBlock = () => {
    const source = selectedBlock;
    if (!source) return;
    const copyBlock = { ...source, id: `${source.id}_${Date.now().toString(36)}`, title: `${source.title} 副本` };
    const index = blocks.findIndex((block) => block.id === source.id);
    const next = [...blocks];
    next.splice(index + 1, 0, copyBlock);
    updateBlocks(() => next);
    setSelectedId(copyBlock.id);
  };

  const duplicateBlockById = (blockId: string) => {
    const source = blocks.find((block) => block.id === blockId);
    if (!source) return;
    const copyBlock = { ...source, id: `${source.id}_${Date.now().toString(36)}`, title: `${source.title} 副本` };
    const index = blocks.findIndex((block) => block.id === source.id);
    updateBlocks((items) => {
      const next = [...items];
      next.splice(index + 1, 0, copyBlock);
      return next;
    });
    setSelectedId(copyBlock.id);
  };

  const duplicateFreeElement = () => {
    if (!selectedFreeElement) return;
    const copyElement: FreeDesignElement = {
      ...selectedFreeElement,
      id: `${selectedFreeElement.id}_${Date.now().toString(36)}`,
      text: `${selectedFreeElement.text} 副本`,
      x: selectedFreeElement.x + 18,
      y: selectedFreeElement.y + 18,
      z: Math.max(0, ...freeElements.map((element) => element.z)) + 1,
    };
    updateFreeElements((items) => [...items, copyElement]);
    setSelectedFreeId(copyElement.id);
  };

  const deleteFreeElementById = (targetId: string) => {
    if (!targetId) return;
    const sourceBlockId = blockIdForFreeElementId(targetId);
    if (sourceBlockId) {
      const selectedIndex = blocks.findIndex((block) => block.id === sourceBlockId);
      const nextBlocks = blocks.filter((block) => block.id !== sourceBlockId);
      const nextSelectedBlock = nextBlocks[Math.max(0, selectedIndex - 1)] ?? nextBlocks[0];
      updatePage((current) => ({
        ...current,
        blocks: nextBlocks,
        freeElements: (current.freeElements ?? []).filter((element) => blockIdForFreeElementId(element.id) !== sourceBlockId),
        designMode: 'free',
        status: 'DRAFT',
      }));
      setSelectedId(nextSelectedBlock?.id ?? '');
      setSelectedFreeId(nextSelectedBlock ? (currentFreeElementIdForBlock(nextSelectedBlock.id, freeElements) ?? `free_from_${nextSelectedBlock.id}`) : '');
      return;
    }
    const next = freeElements.filter((element) => element.id !== targetId && element.parentId !== targetId);
    updateFreeElements(() => next);
    setSelectedFreeId(next[0]?.id ?? '');
  };

  const deleteFreeElement = () => {
    deleteFreeElementById(selectedFreeId);
  };

  const moveFreeLayer = (direction: -1 | 1) => {
    if (!selectedFreeElement) return;
    const nextZ = selectedFreeElement.z + direction;
    updateSelectedFreeElement({ z: Math.max(1, nextZ) });
  };

  const deleteBlock = () => {
    if (!selectedBlock) return;
    const next = blocks.filter((block) => block.id !== selectedBlock.id);
    updatePage((current) => ({
      ...current,
      blocks: next,
      freeElements: (current.freeElements ?? []).filter((element) => blockIdForFreeElementId(element.id) !== selectedBlock.id),
      status: 'DRAFT',
    }));
    const nextSelectedBlock = next[Math.max(0, blocks.findIndex((block) => block.id === selectedBlock.id) - 1)] ?? next[0];
    setSelectedId(nextSelectedBlock?.id ?? '');
    setSelectedFreeId(nextSelectedBlock ? (currentFreeElementIdForBlock(nextSelectedBlock.id, freeElements) ?? `free_from_${nextSelectedBlock.id}`) : '');
  };

  const deleteBlockById = (blockId: string) => {
    const next = blocks.filter((block) => block.id !== blockId);
    updatePage((current) => ({
      ...current,
      blocks: next,
      freeElements: (current.freeElements ?? []).filter((element) => blockIdForFreeElementId(element.id) !== blockId),
      status: 'DRAFT',
    }));
    if (selectedId === blockId || blockIdForFreeElementId(selectedFreeId) === blockId) {
      setSelectedId(next[0]?.id ?? '');
      setSelectedFreeId(next[0] ? (currentFreeElementIdForBlock(next[0].id, freeElements) ?? `free_from_${next[0].id}`) : '');
    }
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'Delete' && event.key !== 'Backspace') return;
      const target = event.target as HTMLElement | null;
      if (target?.closest('input, textarea, select') || target?.isContentEditable) return;
      if (activeSection !== 'page') return;
      event.preventDefault();
      if (pageEditorMode === 'free') {
        deleteFreeElement();
        return;
      }
      deleteBlock();
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [activeSection, pageEditorMode, selectedFreeId, freeElements, blocks, selectedBlock]);

  const createBlock = (kind: CampaignBlockKind = 'RewardCard'): CampaignBlock => {
    const preset = blockPalette.find((item) => item.kind === kind) ?? blockPalette[2];
    return {
      id: `block_${kind.toLowerCase()}_${Date.now().toString(36)}`,
      kind,
      title: preset.title,
      visible: true,
      bound: kind !== 'FAQBlock',
      text: kind === 'ClaimButton' ? '立即领取' : undefined,
      action: kind === 'ClaimButton' ? 'CLAIM_REWARD' : undefined,
      variant: kind === 'ClaimButton' ? 'primary' : undefined,
      width: kind === 'ClaimButton' ? 'full' : undefined,
      radius: kind === 'ClaimButton' ? '8' : undefined,
    };
  };

  const addBlock = (kind: CampaignBlockKind = 'RewardCard') => {
    const nextBlock = createBlock(kind);
    updateBlocks((items) => [...items, nextBlock]);
    setSelectedId(nextBlock.id);
    if (pageEditorMode === 'free') setSelectedFreeId(`free_from_${nextBlock.id}`);
  };

  const addBlockAtCanvasPosition = (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => {
    const nextBlock = createBlock(kind);
    updatePage((current) => {
      const nextBlocks = [...current.blocks, nextBlock];
      const z = Math.max(0, ...(current.freeElements ?? []).map((item) => item.z)) + 1;
      const nextFreeElement = blockToFreeElement(nextBlock, current.device.defaultWidth, z);
      const parent = parentId ? (current.freeElements ?? []).find((element) => element.id === parentId && element.kind === 'layout') : undefined;
      const positionedElement = parent ? {
        ...nextFreeElement,
        parentId: parent.id,
        x: 0,
        y: 0,
        w: layoutChildWidth(parent),
      } : {
        ...nextFreeElement,
        x: Math.min(Math.max(0, current.device.defaultWidth - nextFreeElement.w), Math.max(0, Math.round(x))),
        y: Math.max(0, Math.round(y)),
      };
      const nextFreeElements = fitLayouts([...(current.freeElements ?? []), positionedElement]);
      return { ...current, blocks: nextBlocks, freeElements: nextFreeElements, designMode: 'free', status: 'DRAFT' };
    });
    setSelectedId(nextBlock.id);
    setSelectedFreeId(`free_from_${nextBlock.id}`);
  };

  const createLayoutElement = (kind: LayoutKind, x = 18, y = 360): FreeDesignElement => {
    const preset = layoutPalette.find((item) => item.kind === kind) ?? layoutPalette[0];
    return {
      id: `layout_${kind}_${Date.now().toString(36)}`,
      kind: 'layout',
      text: `${preset.title}\\n拖入组件到盒子内`,
      x: Math.min(Math.max(0, pageSchema.device.defaultWidth - 345), Math.max(0, Math.round(x))),
      y: Math.max(0, Math.round(y)),
      w: 345,
      h: kind === 'vertical-layout' ? 144 : 96,
      z: Math.max(0, ...freeElements.map((element) => element.z)) + 1,
      variant: 'secondary',
      layout: kind,
      ...flexPreset(kind),
      radius: '8',
    };
  };

  const addLayoutAtCanvasPosition = (kind: LayoutKind, x = 18, y = 360) => {
    const nextLayout = createLayoutElement(kind, x, y);
    updateFreeElements((items) => [...items, nextLayout]);
    setSelectedFreeId(nextLayout.id);
  };

  const addFreeElement = (kind: FreeDesignElement['kind']) => {
    const next: FreeDesignElement = {
      id: `free_${kind}_${Date.now().toString(36)}`,
      kind,
      text: kind === 'button' ? '按钮' : kind === 'image' ? '图片占位' : kind === 'card' ? '自由卡片' : '自由文本',
      x: 32,
      y: 360,
      w: kind === 'text' ? 180 : 260,
      h: kind === 'text' ? 44 : 88,
      z: Math.max(0, ...freeElements.map((element) => element.z)) + 1,
      variant: kind === 'button' ? 'primary' : undefined,
      action: kind === 'button' ? 'CLAIM_REWARD' : undefined,
      radius: kind === 'text' ? '0' : '8',
    };
    updateFreeElements((items) => [...items, next]);
    setSelectedFreeId(next.id);
  };

  const updateSelectedBlock = (patch: Partial<CampaignBlock>) => {
    if (!selectedBlock) return;
    updateBlocks((items) => items.map((block) => block.id === selectedBlock.id ? { ...block, ...patch } : block));
  };

  const setEditorMode = (mode: 'controlled' | 'free') => {
    setPageEditorMode(mode);
    if (mode === 'controlled') updatePage((current) => ({ ...current, designMode: 'controlled', status: 'DRAFT' }));
    if (mode === 'free') {
      const nextFreeElements = pageSchema.freeElements?.length
        ? pageSchema.freeElements
        : blocksToFreeElements(pageSchema.blocks, pageSchema.device.defaultWidth);
      setSelectedFreeId((current) => nextFreeElements.some((element) => element.id === current) ? current : nextFreeElements[0]?.id ?? '');
      updatePage({ designMode: 'free', freeElements: nextFreeElements, status: 'DRAFT' });
    }
  };

  const toggleBlockVisible = (blockId: string) => {
    updateBlocks((items) => items.map((block) => block.id === blockId ? { ...block, visible: !block.visible } : block));
  };

  const publishPage = () => {
    const issues = validateCampaignPage(pageSchema);
    setValidationIssues(issues);
    if (issues.length) {
      setActiveSection('settings');
      return;
    }
    const { publishedPage, version } = campaignPageApi.publish(pageSchema, versions);
    setVersions((items) => [version, ...items]);
    setPageSchema(publishedPage);
  };

  const createFromTemplate = (templateName: string) => {
    const nextPage = defaultCampaignPage();
    nextPage.id = `page_${Date.now().toString(36)}`;
    nextPage.name = `${templateName} 页面草稿`;
    nextPage.path = `/campaign/${templateName.toLowerCase().replace(/[^a-z0-9]+/g, '-') || 'template-page'}`;
    nextPage.updatedAt = nowISO();
    setPageSchema(nextPage);
    setSelectedId(nextPage.blocks[0]?.id ?? '');
    setSelectedFreeId(nextPage.freeElements[0]?.id ?? '');
    setPageEditorMode('free');
    setActiveSection('page');
  };

  const exportSchema = () => {
    const blob = new Blob([JSON.stringify(pageSchema, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${pageSchema.id}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const renderTreeNode = (node: PageTreeNode, depth = 0) => {
    const isLayoutNode = node.type === 'layout';
    const block = node.type === 'block' ? node.block : undefined;
    const freeElementId = isLayoutNode ? node.id : currentFreeElementIdForBlock(node.id, freeElements) ?? `free_from_${node.id}`;
    const active = pageEditorMode === 'free' ? selectedFreeId === freeElementId : node.id === selectedId;
    return (
      <div key={`${node.type}_${node.id}`} className={depth > 0 ? 'campaign-outline-branch' : undefined}>
        <div
          className={active ? 'campaign-outline-row active' : 'campaign-outline-row'}
          role="button"
          tabIndex={0}
          draggable={Boolean(block)}
          onDragStart={(event) => {
            if (!block) return;
            event.dataTransfer.setData('application/x-campaign-tree-free-id', freeElementId);
            event.dataTransfer.effectAllowed = 'move';
          }}
          onDragOver={(event) => {
            if (!isLayoutNode) return;
            if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
            event.preventDefault();
            event.dataTransfer.dropEffect = 'move';
          }}
          onDrop={(event) => {
            if (!isLayoutNode) return;
            const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
            if (!draggedFreeId) return;
            event.preventDefault();
            event.stopPropagation();
            moveFreeElementIntoLayout(draggedFreeId, node.id);
          }}
          onClick={() => {
          if (block) setSelectedId(block.id);
          if (pageEditorMode === 'free') setSelectedFreeId(freeElementId);
        }} onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            if (block) setSelectedId(block.id);
            if (pageEditorMode === 'free') setSelectedFreeId(freeElementId);
          }
        }}>
          <span><strong>{node.title}</strong><small>{node.meta}</small></span>
          <button type="button" aria-label={block?.visible ? '隐藏内容块' : '显示内容块'} disabled={!block} onClick={(event) => {
            event.stopPropagation();
            if (block) toggleBlockVisible(block.id);
          }}><Eye size={16} /></button>
          <button type="button" aria-label="复制内容块" disabled={!block} onClick={(event) => {
            event.stopPropagation();
            if (block) duplicateBlockById(block.id);
          }}><Copy size={16} /></button>
          <button type="button" aria-label="删除内容块" onClick={(event) => {
            event.stopPropagation();
            if (block) deleteBlockById(block.id);
            if (isLayoutNode) deleteFreeElementById(node.id);
          }}><Trash2 size={16} /></button>
        </div>
        {node.children?.length ? (
          <div className="campaign-outline-children">
            {node.children.map((child) => renderTreeNode(child, depth + 1))}
          </div>
        ) : null}
      </div>
    );
  };
  const setEditorDeviceWidth = (width: DeviceWidth) => {
    setDeviceWidth(width);
    updatePage((current) => ({ ...current, device: { ...current.device, defaultWidth: width }, status: 'DRAFT' }));
  };
  const previewPage = { ...pageSchema, device: { ...pageSchema.device, defaultWidth: deviceWidth } };

  return (
    <main className="campaign-editor">
      <header className="campaign-editor-header">
        <div className="campaign-brand">
          <span className="campaign-brand-mark">X</span>
          <div><b>ExMarketing</b><small>Campaign Editor</small></div>
        </div>
        <button className="campaign-select" type="button">{pageSchema.name} <ChevronDown size={16} /></button>
        <select value={deviceWidth} onChange={(event) => setEditorDeviceWidth(Number(event.target.value) as DeviceWidth)}>
          <option value={375}>375px</option>
          <option value={393}>393px</option>
          <option value={414}>414px</option>
        </select>
        <span className="campaign-save-state">● {saveMessage}</span>
        <div className="campaign-header-spacer" />
        <button type="button" onClick={() => setPreviewOpen(true)}><Eye size={17} />预览</button>
        <button type="button" onClick={exportSchema}><FileText size={17} />导出 Schema</button>
        <button className="primary" type="button" onClick={publishPage}><Plane size={17} />发布 <ChevronDown size={15} /></button>
        <button className="icon" type="button" aria-label="帮助"><HelpCircle size={18} /></button>
        <button className="icon" type="button" aria-label="通知"><Bell size={18} /></button>
        <button className="campaign-user" type="button"><span>运</span>运营团队 <ChevronDown size={14} /></button>
      </header>

      <section className="campaign-editor-body">
        <aside className="campaign-left-nav" aria-label="Campaign editor navigation">
          {navItems.map(({ id, label, icon: Icon }) => (
            <button key={id} className={id === activeSection ? 'active' : ''} type="button" onClick={() => setActiveSection(id as WorkspaceSection)}>
              <Icon size={22} />
              <span>{label}</span>
            </button>
          ))}
        </aside>

        {activeSection === 'page' ? (
          <>
              <aside className="campaign-outline">
                <h2>页面树</h2>
                <label className="campaign-tree-search">
                  <input value={treeQuery} onChange={(event) => setTreeQuery(event.target.value)} placeholder="搜索" aria-label="搜索页面树" />
                </label>
                <button
                  className="campaign-page-tree-root"
                  type="button"
                  aria-expanded={treeOpen}
                  onDragOver={(event) => {
                    if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
                    event.preventDefault();
                    event.dataTransfer.dropEffect = 'move';
                  }}
                  onDrop={(event) => {
                    const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
                    if (!draggedFreeId) return;
                    event.preventDefault();
                    event.stopPropagation();
                    moveFreeElementToRoot(draggedFreeId);
                  }}
                  onClick={() => setTreeOpen((open) => !open)}
                >
                  <ChevronDown size={15} className={treeOpen ? 'open' : ''} />
                  <FileText size={18} />
                  <span><strong>{pageSchema.name}</strong></span>
                </button>
                {treeOpen ? (
                  <div
                    className="campaign-outline-list"
                    onDragOver={(event) => {
                      if (!event.dataTransfer.types.includes('application/x-campaign-tree-free-id')) return;
                      event.preventDefault();
                      event.dataTransfer.dropEffect = 'move';
                    }}
                    onDrop={(event) => {
                      const draggedFreeId = event.dataTransfer.getData('application/x-campaign-tree-free-id');
                      if (!draggedFreeId) return;
                      event.preventDefault();
                      moveFreeElementToRoot(draggedFreeId);
                    }}
                  >
                    {treeNodes.map((node) => renderTreeNode(node))}
                    {treeNodes.length === 0 ? <p className="campaign-tree-empty">没有匹配节点</p> : null}
                  </div>
                ) : null}
                <button className="campaign-library-title" type="button" aria-expanded={libraryOpen} onClick={() => setLibraryOpen((open) => !open)}>
                  <ChevronDown size={14} className={libraryOpen ? 'open' : ''} />
                  常用组件
                </button>
                {libraryOpen ? (
                  <div className="campaign-block-palette" aria-label="组件库">
                    {blockPalette.map((item) => {
                      const Icon = blockIcons[item.kind];
                      return (
                        <button
                          key={item.kind}
                          type="button"
                          draggable
                          onClick={() => pageEditorMode === 'free' ? addBlockAtCanvasPosition(item.kind, 32, 360) : addBlock(item.kind)}
                          onDragStart={(event) => {
                            setDraggingBlockKind(item.kind);
                            event.dataTransfer.setData('application/x-campaign-block', item.kind);
                            event.dataTransfer.effectAllowed = 'copy';
                          }}
                          onDragEnd={() => setDraggingBlockKind(null)}
                        >
                          <Icon size={16} />
                          {item.label}
                        </button>
                      );
                    })}
                    {layoutPalette.map((item) => {
                      const Icon = item.icon;
                      return (
                        <button
                          key={item.kind}
                          type="button"
                          draggable
                          onClick={() => addLayoutAtCanvasPosition(item.kind, 24, 380)}
                          onDragStart={(event) => {
                            setDraggingLayoutKind(item.kind);
                            event.dataTransfer.setData('application/x-campaign-layout', item.kind);
                            event.dataTransfer.effectAllowed = 'copy';
                          }}
                          onDragEnd={() => setDraggingLayoutKind(null)}
                        >
                          <Icon size={16} />
                          {item.label}
                        </button>
                      );
                    })}
                  </div>
                ) : null}
              </aside>

            <section className="campaign-canvas-workspace">
              <div className="campaign-toolbar">
                <button className={pageEditorMode === 'free' ? 'active' : ''} type="button" onClick={() => setEditorMode('free')}>页面设计</button>
                <button className={pageEditorMode === 'controlled' ? 'active' : ''} type="button" onClick={() => setEditorMode('controlled')}>模块预览</button>
              </div>
              {pageEditorMode === 'free' ? (
                <FreeDesignCanvas
                  width={deviceWidth}
                  elements={freeElements}
                  selectedId={selectedFreeId}
                  onSelect={setSelectedFreeId}
                  onChange={updateFreeElements}
                  draggingBlockKind={draggingBlockKind}
                  draggingLayoutKind={draggingLayoutKind}
                  onDropBlock={(kind, x, y, parentId) => addBlockAtCanvasPosition(kind, x, y, parentId)}
                  onDropLayout={(kind, x, y) => addLayoutAtCanvasPosition(kind, x, y)}
                  onDragDone={() => setDraggingBlockKind(null)}
                  onLayoutDragDone={() => setDraggingLayoutKind(null)}
                />
              ) : (
                <CampaignModulePreview
                  page={pageSchema}
                  selectedId={selectedId}
                  onSelect={setSelectedId}
                  draggingBlockKind={draggingBlockKind}
                  onDropBlock={(kind) => addBlock(kind)}
                  onDragDone={() => setDraggingBlockKind(null)}
                />
              )}
            </section>

            <aside className="campaign-inspector">
              {pageEditorMode === 'free' ? (
                <>
                  <header>
                    <div><Type size={20} /><h2>{selectedFreeElement?.text || '自由元素'}</h2></div>
                    <span>{selectedFreeElement?.binding ? '已绑定' : '未绑定'}</span>
                    <button type="button" aria-label="复制自由元素" onClick={duplicateFreeElement}><Copy size={16} /></button>
                    <button type="button" aria-label="删除自由元素" onClick={deleteFreeElement}><Trash2 size={16} /></button>
                  </header>
                  <nav>
                    {(['content', 'binding', 'action'] as EditorTab[]).map((tab) => (
                      <button key={tab} className={inspectorTab === tab ? 'active' : ''} type="button" onClick={() => setInspectorTab(tab)}>
                        {tab === 'content' ? '内容' : tab === 'binding' ? '绑定' : '动作'}
                      </button>
                    ))}
                  </nav>
                  <FreeElementInspector element={selectedFreeElement} tab={inspectorTab} width={deviceWidth} onChange={updateSelectedFreeElement} onLayerMove={moveFreeLayer} />
                </>
              ) : selectedBlock ? (
                <>
                  <header>
                    <div><SelectedIcon size={20} /><h2>{selectedBlock.title}</h2></div>
                    <span>{selectedBlock.bound ? '已绑定' : '未绑定'}</span>
                    <Copy size={16} />
                    <Trash2 size={16} />
                  </header>
                  <nav>
                    {(['content', 'binding', 'action'] as EditorTab[]).map((tab) => (
                      <button key={tab} className={inspectorTab === tab ? 'active' : ''} type="button" onClick={() => setInspectorTab(tab)}>
                        {tab === 'content' ? '内容' : tab === 'binding' ? '绑定' : '动作'}
                      </button>
                    ))}
                  </nav>
                  <InspectorContent block={selectedBlock} tab={inspectorTab} deviceWidth={deviceWidth} onBlockChange={updateSelectedBlock} onMove={(direction) => updateBlocks((items) => moveBlock(items, selectedBlock.id, direction))} />
                </>
              ) : (
                <div className="campaign-inspector-body"><p>当前页面没有内容块，可以从左侧常用组件继续添加。</p></div>
              )}
            </aside>
          </>
        ) : (
          <CampaignWorkspace
            section={activeSection}
            page={pageSchema}
            versions={versions}
            validationIssues={validationIssues}
            resourceRecords={resourceRecords}
            onResourceRecordsChange={setResourceRecords}
            onPagePatch={(patch) => updatePage(patch)}
            onCreateFromTemplate={createFromTemplate}
          />
        )}
      </section>
      {previewOpen ? (
        <div className="campaign-preview-modal" role="dialog" aria-modal="true" aria-label="移动端预览">
          <button className="campaign-preview-backdrop" type="button" aria-label="关闭预览" onClick={() => setPreviewOpen(false)} />
          <section className="campaign-phone-frame">
            <header><span />移动端预览<button type="button" onClick={() => setPreviewOpen(false)}>关闭</button></header>
            <div className="campaign-phone-screen" style={{ width: deviceWidth }}>
              <CampaignRuntimeView page={previewPage} embedded />
            </div>
          </section>
        </div>
      ) : null}
    </main>
  );
}

function sectionTitle(section: Exclude<WorkspaceSection, 'page'>) {
  if (section === 'resources') return ['资源', '集中管理数据、动作、样式和图片素材'];
  if (section === 'settings') return ['设置', '配置发布路径、环境、分享预览和版本信息'];
  if (section === 'templates') return ['模板库', '从标准活动模板快速创建页面草稿'];
  return ['资源', '集中管理数据、动作、样式和图片素材'];
}

function CampaignWorkspace({
  section,
  page,
  versions,
  validationIssues,
  resourceRecords,
  onResourceRecordsChange,
  onPagePatch,
  onCreateFromTemplate,
}: {
  section: Exclude<WorkspaceSection, 'page'>;
  page: CampaignPageSchema;
  versions: CampaignPageVersion[];
  validationIssues: CampaignValidationIssue[];
  resourceRecords: ResourceRecord[];
  onResourceRecordsChange: Dispatch<SetStateAction<ResourceRecord[]>>;
  onPagePatch: (patch: Partial<CampaignPageSchema>) => void;
  onCreateFromTemplate: (templateName: string) => void;
}) {
  const config = workspaceConfig[section];
  const [activeGroup, setActiveGroup] = useState(config.groups[0]);
  const [selected, setSelected] = useState(0);
  const [title, description] = sectionTitle(section);
  const cards = config.items.filter((item) => activeGroup === '全部' || item.group === activeGroup);
  const current = cards[selected] ?? cards[0] ?? config.items[0];
  if (section === 'resources') return <CampaignResourceWorkspace records={resourceRecords} onRecordsChange={onResourceRecordsChange} />;
  return (
    <section className="campaign-secondary-workspace">
      <aside className="campaign-secondary-groups">
        <h2>{title}</h2>
        <p>{description}</p>
        {config.groups.map((group) => (
          <button
            key={group}
            className={activeGroup === group ? 'active' : ''}
            type="button"
            onClick={() => {
              setActiveGroup(group);
              setSelected(0);
            }}
          >
            {group}
            <span>{config.items.filter((item) => group === '全部' || item.group === group).length}</span>
          </button>
        ))}
      </aside>
      <div className="campaign-secondary-list">
        <header>
          <h2>{activeGroup}</h2>
          <p>{config.detailTitle === '模板详情' ? '选择模板后可从模板创建页面草稿。' : description}</p>
        </header>
        <div>
          {cards.map((card, index) => (
            <button key={card.title} className={selected === index ? 'active' : ''} type="button" onClick={() => setSelected(index)}>
              <strong>{card.title}</strong>
              <span>{card.meta}</span>
              <small>{card.detail}</small>
            </button>
          ))}
        </div>
      </div>
      <div className="campaign-secondary-detail">
        <span>{config.detailTitle}</span>
        <h2>{current.title}</h2>
        <p>{current.detail}</p>
        <div className="campaign-secondary-tags">
          {current.tags.map((tag) => <b key={tag}>{tag}</b>)}
        </div>
        <dl>
          <div><dt>状态</dt><dd>{current.meta}</dd></div>
          <div><dt>{section === 'templates' ? '创建流程' : '安全边界'}</dt><dd>{section === 'templates' ? '从模板创建 -> 检查缺失字段和动作 -> 进入页面设计。' : '发布前校验 schema、路径、绑定字段和动作权限。'}</dd></div>
          <div><dt>发布校验</dt><dd>{versions.length ? `最近发布 ${versions[0].version} · ${new Date(versions[0].publishedAt).toLocaleString()}` : '缺失绑定、动作权限和移动端溢出会阻止发布。'}</dd></div>
        </dl>
        {section === 'settings' ? (
          <div className={validationIssues.length ? 'campaign-validation-list blocking' : 'campaign-validation-list'}>
            <h3>发布校验</h3>
            {validationIssues.length ? validationIssues.map((issue) => (
              <p key={issue.code}><strong>{issue.code}</strong>{issue.message}</p>
            )) : <p><strong>PASS</strong>schema、绑定字段、动作白名单和路径格式通过。</p>}
          </div>
        ) : null}
        {section === 'settings' ? (
          <div className="campaign-secondary-form">
            <label><span>页面名称</span><input value={page.name} onChange={(event) => onPagePatch({ name: event.target.value })} /></label>
            <label><span>页面路径</span><input value={page.path} onChange={(event) => onPagePatch({ path: event.target.value })} /></label>
            <label><span>活动 ID</span><input value={page.activityId} onChange={(event) => onPagePatch({ activityId: event.target.value })} /></label>
          </div>
        ) : null}
        {section === 'templates' ? (
          <button className="campaign-secondary-primary" type="button" onClick={() => onCreateFromTemplate(current.title)}>从模板创建</button>
        ) : null}
      </div>
    </section>
  );
}

function CampaignResourceWorkspace({
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
      <header className="campaign-resource-bar">
        <div className="campaign-resource-title"><h2>资源</h2></div>
        <label className="campaign-resource-search">
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索资源（名称、标识符...）" />
        </label>
        <select aria-label="来源" value={sourceFilter} onChange={(event) => setSourceFilter(event.target.value)}>
          {sourceOptions.map((source) => <option key={source}>{source}</option>)}
        </select>
        <select aria-label="状态" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          {['全部', '启用', '停用', '待保存', '尺寸提醒'].map((status) => <option key={status}>{status}</option>)}
        </select>
        <button className="campaign-resource-add" type="button" onClick={startAdd}><Plus size={16} />添加</button>
      </header>

      <div className="campaign-resource-layout">
        <aside className="campaign-resource-tree" data-testid="resource-type-tree">
          <div className="campaign-resource-tree-head"><strong>资源类型</strong><button type="button" aria-label="添加资源类型"><Plus size={16} /></button></div>
          {resourceTabs.map((tab) => (
            <div key={tab} className="campaign-resource-tree-group">
              <button type="button" className={`${expandedGroups[tab] ? 'open' : ''} ${activeTab === tab ? 'active' : ''}`} onClick={() => toggleResourceGroup(tab)}>
                <ChevronDown size={14} />
                <span>{resourceGroupLabels[tab]}</span>
                <b>{records.filter((record) => record.tab === tab).length}</b>
              </button>
              {expandedGroups[tab] ? (
                <div>
                  {resourceTree[tab].map((child) => (
                    <button key={child.name} type="button" className={activeCategory === child.name ? 'active' : ''} onClick={() => setActiveCategory(child.name)}>
                      <span>{child.name}</span>
                      <b>{records.filter((record) => record.tab === tab && record.category === child.name).length || child.count}</b>
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
          ))}
          <button className="campaign-resource-tree-footer" type="button"><ListChecks size={16} />常用组件<ChevronDown size={14} /></button>
        </aside>

        <section className="campaign-resource-table-wrap" data-testid="resource-table">
          <table className="campaign-resource-table">
            <thead>
              <tr>
                <th>{activeTab === '图片' ? '名称' : activeTab === '数据源' ? '数据源 / 标识符' : '名称 / 标识符'}</th>
                <th>{activeTab === '图片' ? '用途' : '类型'}</th>
                <th>{activeTab === '数据源' ? '接口地址' : '来源'}</th>
                {activeTab === '图片' ? <th>尺寸</th> : activeTab === '数据源' ? <th>刷新策略</th> : <th>引用</th>}
                {activeTab === '图片' ? <th>大小</th> : <th>状态</th>}
                <th>更新时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {addMode && addMode !== '图片' ? (
                <tr className="adding">
                  <td><b>{addMode === '数据' ? '新增数据字段' : addMode === '数据源' ? '新增数据源' : addMode === '动作' ? '新增动作事件' : '新增样式 Token'}</b></td>
                  <td><select value={draftType} onChange={(event) => setDraftType(event.target.value)}><option>文本</option><option>数值</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Workflow Event</option><option>颜色</option><option>圆角</option></select></td>
                  <td>{addMode === '数据源' ? draftValue : <select value={draftSource} onChange={(event) => setDraftSource(event.target.value)}><option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option></select>}</td>
                  <td>{addMode === '数据源' ? '页面加载 + 30s 轮询' : <button type="button">选择组件</button>}</td>
                  <td><span className="campaign-resource-status enabled">启用</span></td>
                  <td>--</td>
                  <td><button type="button" onClick={addRecord}>保存</button><button type="button" onClick={() => setAddMode(null)}>取消</button></td>
                </tr>
              ) : null}
              {filteredRecords.map((record) => (
                <tr key={record.id} className={selectedId === record.id ? 'selected' : ''} onClick={() => {
                  setSelectedId(record.id);
                  setAddMode(null);
                  setEditingRecord(null);
                }}>
                  <td>
                    <span className="campaign-resource-name">
                      {record.tab === '图片' ? <i>{record.thumbnail ?? '🖼️'}</i> : record.tab === '数据源' ? <i>API</i> : record.tab === '样式' ? <i style={{ background: record.value ?? 'var(--theme-accent)' }} /> : null}
                      <span><strong>{record.name}</strong><small>{record.identifier}</small></span>
                    </span>
                  </td>
                  <td>{record.type}</td>
                  <td>{record.tab === '数据源' ? record.endpoint : record.source}</td>
                  <td>{record.tab === '图片' ? record.size : record.tab === '数据源' ? record.refresh : record.usage}</td>
                  <td>{record.tab === '图片' ? record.fileSize : <span className={`campaign-resource-status ${resourceStatusClass(record.status)}`}>{record.status}</span>}</td>
                  <td>{record.updatedAt}</td>
                  <td><button type="button" aria-label={`更多 ${record.name}`} onClick={(event) => event.stopPropagation()}>...</button></td>
                </tr>
              ))}
              {filteredRecords.length === 0 && !addMode ? <tr><td colSpan={7} className="campaign-resource-empty">没有匹配资源</td></tr> : null}
            </tbody>
          </table>
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
  return (
    <>
      <header className="campaign-resource-panel-head"><h2>资源详情</h2><button type="button" aria-label="关闭详情">×</button></header>
      <div className="campaign-resource-detail-body">
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
        {record.tab === '数据源' ? <div className="campaign-source-preview"><strong>实时拉取预览</strong><code>{'{ "activity": { "title": "夏日充值返利活动" }, "reward": { "amount": 188 } }'}</code></div> : null}
        <div className="campaign-resource-usage"><strong>使用方式</strong><p>{record.tab === '数据源' ? '数据源负责从接口或实时通道拉取原始数据，再映射为页面可绑定字段。' : record.tab === '图片' ? '拖入图片或头图组件后绑定素材资源。' : record.tab === '样式' ? '组件样式从主题 token 读取。' : record.tab === '动作' ? '点击事件只能绑定白名单 workflow event。' : '字段通过 Activity View Model 表达式绑定到页面组件。'}</p></div>
        <div className="campaign-resource-panel-actions">
          <button type="button" onClick={onEdit}>编辑</button>
          <button type="button" onClick={onDuplicate}>复制</button>
          <button className="danger" type="button" onClick={onDelete}>删除</button>
        </div>
      </div>
    </>
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
    <>
      <header className="campaign-resource-panel-head"><h2>编辑资源</h2><button type="button" aria-label="关闭编辑" onClick={onCancel}>×</button></header>
      <div className="campaign-resource-form">
        <label><span>名称 *</span><input value={record.name} onChange={(event) => onChange({ name: event.target.value })} /></label>
        <label><span>标识符 *</span><input value={record.identifier} onChange={(event) => onChange({ identifier: event.target.value })} /></label>
        <label><span>类型 *</span><select value={record.type} onChange={(event) => onChange({ type: event.target.value })}><option>文本</option><option>数值</option><option>时间</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Mock JSON</option><option>Workflow Event</option><option>Page Event</option><option>颜色</option><option>圆角</option><option>头图</option><option>活动配图</option><option>图标</option><option>背景图</option></select></label>
        <label><span>来源 *</span><select value={record.source} onChange={(event) => onChange({ source: event.target.value })}><option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option><option>第三方接口</option><option>页面上传</option><option>素材库</option></select></label>
        <label><span>状态 *</span><select value={record.status} onChange={(event) => onChange({ status: event.target.value as ResourceStatus })}><option>启用</option><option>停用</option><option>待保存</option><option>尺寸提醒</option><option>大小错误</option></select></label>
        {record.tab === '数据源' ? <label><span>接口地址 *</span><input value={record.endpoint ?? ''} onChange={(event) => onChange({ endpoint: event.target.value })} /></label> : null}
        {record.tab === '数据源' ? <label><span>请求方式 *</span><select value={record.method ?? 'GET'} onChange={(event) => onChange({ method: event.target.value })}><option>GET</option><option>POST</option><option>SUBSCRIBE</option><option>LOCAL</option></select></label> : null}
        {record.tab === '数据源' ? <label><span>刷新策略 *</span><select value={record.refresh ?? '页面加载 + 30s 轮询'} onChange={(event) => onChange({ refresh: event.target.value })}><option>页面加载 + 30s 轮询</option><option>页面加载</option><option>手动刷新</option><option>WebSocket 实时</option></select></label> : null}
        {record.tab === '样式' ? <label><span>Token 值 *</span><input value={record.value ?? ''} onChange={(event) => onChange({ value: event.target.value })} /></label> : null}
        {record.tab === '图片' ? <label><span>尺寸</span><input value={record.size ?? ''} onChange={(event) => onChange({ size: event.target.value })} /></label> : null}
        {record.tab === '图片' ? <label><span>大小</span><input value={record.fileSize ?? ''} onChange={(event) => onChange({ fileSize: event.target.value })} /></label> : null}
        <label><span>引用 / 使用</span><input value={record.usage} onChange={(event) => onChange({ usage: event.target.value })} /></label>
        <label><span>说明</span><textarea value={record.description} onChange={(event) => onChange({ description: event.target.value })} /></label>
        <div className="campaign-expression-preview">
          <span>保存后会更新当前资源行</span>
          <code>{record.identifier}</code>
        </div>
      </div>
      <footer className="campaign-resource-panel-footer"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={onSave}>保存</button></footer>
    </>
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
      <>
        <header className="campaign-resource-panel-head"><h2>批量上传图片</h2><button type="button" aria-label="关闭新增" onClick={onCancel}>×</button></header>
        <div className="campaign-resource-form">
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
                  <button type="button" onClick={() => onRemoveUpload(item.id)}>删除</button>
                </span>
              </div>
            ))}
          </div>
          <div className="campaign-upload-summary"><span>已选择 {uploadQueue.length} 张</span><span>总大小 {totalSize} KB</span><span>{warningUploads + invalidUploads} 张需处理</span></div>
        </div>
        <footer className="campaign-resource-panel-footer"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" disabled={uploadQueue.length === 0 || invalidUploads > 0} onClick={onSave}>全部保存</button></footer>
      </>
    );
  }
  return (
    <>
      <header className="campaign-resource-panel-head"><h2>{mode === '数据' ? '新增数据字段' : mode === '数据源' ? '新增数据源' : mode === '动作' ? '新增动作事件' : '新增样式 Token'}</h2><button type="button" aria-label="关闭新增" onClick={onCancel}>×</button></header>
      <div className="campaign-resource-form">
        <label><span>{mode === '数据' ? '字段名称' : mode === '数据源' ? '数据源名称' : mode === '动作' ? '动作名称' : '样式名称'} *</span><input value={draftName} onChange={(event) => onDraftName(event.target.value)} /></label>
        {mode === '样式' ? <label><span>Token 名称 *</span><input value="--campaign-button-primary" readOnly /></label> : null}
        <label><span>{mode === '数据' ? '字段类型' : mode === '数据源' ? '数据源类型' : mode === '动作' ? '动作类型' : '样式类型'} *</span><select value={draftType} onChange={(event) => onDraftType(event.target.value)}><option>文本</option><option>数值</option><option>时间</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Mock JSON</option><option>Workflow Event</option><option>Page Event</option><option>颜色</option><option>圆角</option></select></label>
        {mode === '数据源' ? <label><span>接口地址 *</span><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} placeholder="/api/activities/:activityId/view-model" /></label> : null}
        {mode === '数据源' ? <label><span>请求方式 *</span><select><option>GET</option><option>POST</option><option>SUBSCRIBE</option><option>LOCAL</option></select></label> : null}
        <label><span>{mode === '数据' ? '数据来源' : mode === '数据源' ? '来源类型' : '来源'} *</span><select value={draftSource} onChange={(event) => onDraftSource(event.target.value)}><option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option><option>第三方接口</option></select></label>
        {mode === '数据源' ? <label><span>刷新策略 *</span><select><option>页面加载 + 30s 轮询</option><option>页面加载</option><option>手动刷新</option><option>WebSocket 实时</option></select></label> : null}
        {mode === '数据源' ? <label><span>字段映射</span><select><option>映射到 Activity View Model</option><option>映射到页面数据字段</option><option>仅用于预览</option></select></label> : null}
        {mode === '动作' ? <label><span>触发组件 *</span><select><option>按钮 / 图片 / 卡片 / 页面曝光</option></select></label> : null}
        {mode === '动作' ? <label><span>Payload Schema *</span><textarea value={'{\n  "activityId": "{{ activity.id }}",\n  "rewardId": "{{ reward.id }}"\n}'} readOnly /></label> : null}
        {mode === '样式' ? <label><span>默认值 *</span><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} /></label> : mode !== '数据源' ? <label><span>默认值</span><input value={draftValue} onChange={(event) => onDraftValue(event.target.value)} placeholder="请输入默认值（可选）" /></label> : null}
        <label><span>{mode === '样式' ? '适用组件' : mode === '数据源' ? '绑定字段' : '绑定组件'}</span><select><option>头图 / 标题 / 分享卡片</option><option>按钮 / 卡片 / 标题</option><option>activity.title / reward.amount</option></select></label>
        <label><span>说明</span><textarea placeholder="请输入说明（可选）" /></label>
        {mode === '数据' ? <div className="campaign-expression-preview"><span>表达式预览</span><code>{`{{ ${draftName || 'activity.title'} }}`}</code></div> : null}
        {mode === '数据源' ? <div className="campaign-source-preview"><span>实时拉取预览</span><code>{`GET ${draftValue || '/api/activities/:activityId/view-model'}\n200 OK · 86 ms\n{ "activity": { "title": "夏日充值返利活动" } }`}</code></div> : null}
        {mode === '动作' ? <div className="campaign-expression-preview"><span>验证 Payload</span><b>验证通过</b></div> : null}
        {mode === '样式' ? <div className="campaign-style-preview"><span>样式预览</span><button style={{ background: draftValue || 'var(--theme-accent)' }} type="button">立即领取</button><code>--campaign-button-primary</code></div> : null}
      </div>
      <footer className="campaign-resource-panel-footer"><button type="button" onClick={onCancel}>取消</button><button className="primary" type="button" onClick={onSave}>保存</button></footer>
    </>
  );
}

function AddBetween({ onClick }: { onClick: () => void }) {
  return <button className="campaign-between-add" type="button" onClick={onClick}><Plus size={14} />添加内容块</button>;
}

function FreeLayerPanel({
  elements,
  selectedId,
  onSelect,
  onAdd,
  onDuplicate,
  onDelete,
}: {
  elements: FreeDesignElement[];
  selectedId: string;
  onSelect: (id: string) => void;
  onAdd: (kind: FreeDesignElement['kind']) => void;
  onDuplicate: () => void;
  onDelete: () => void;
}) {
  const sortedElements = [...elements].sort((a, b) => b.z - a.z);
  return (
    <aside className="campaign-outline campaign-free-layers">
      <h2>自由图层</h2>
      <p>点击图层选中元素，在画布拖拽移动，拖右下角调整尺寸</p>
      <div className="campaign-free-layer-actions">
        <button type="button" onClick={() => onAdd('text')}><Type size={16} />文本</button>
        <button type="button" onClick={() => onAdd('button')}><Plane size={16} />按钮</button>
        <button type="button" onClick={() => onAdd('card')}><Gift size={16} />卡片</button>
        <button type="button" onClick={() => onAdd('image')}><Image size={16} />图片</button>
      </div>
      <div className="campaign-free-layer-list">
        {sortedElements.map((element) => (
          <button key={element.id} className={selectedId === element.id ? 'active' : ''} type="button" onClick={() => onSelect(element.id)}>
            <span>{element.kind === 'text' ? <Type size={18} /> : element.kind === 'button' ? <Plane size={18} /> : element.kind === 'image' ? <Image size={18} /> : <Gift size={18} />}</span>
            <strong>{element.text.split('\\n')[0]}</strong>
            <small>{element.w} x {element.h} · z{element.z}</small>
          </button>
        ))}
      </div>
      <div className="campaign-free-layer-footer">
        <button type="button" onClick={onDuplicate}><Copy size={16} />复制</button>
        <button type="button" onClick={onDelete}><Trash2 size={16} />删除</button>
      </div>
    </aside>
  );
}

function FreeDesignCanvas({
  width,
  elements,
  selectedId,
  onSelect,
  onChange,
  draggingBlockKind,
  draggingLayoutKind,
  onDropBlock,
  onDropLayout,
  onDragDone,
  onLayoutDragDone,
}: {
  width: DeviceWidth;
  elements: FreeDesignElement[];
  selectedId: string;
  onSelect: (id: string) => void;
  onChange: (updater: (current: FreeDesignElement[]) => FreeDesignElement[]) => void;
  draggingBlockKind: CampaignBlockKind | null;
  draggingLayoutKind: LayoutKind | null;
  onDropBlock: (kind: CampaignBlockKind, x: number, y: number, parentId?: string) => void;
  onDropLayout: (kind: LayoutKind, x: number, y: number) => void;
  onDragDone: () => void;
  onLayoutDragDone: () => void;
}) {
  const [guides, setGuides] = useState<{ x?: number; y?: number }>({});
  const nearestGuide = (value: number, targets: number[]) => targets.reduce((nearest, target) => Math.abs(target - value) < Math.abs(nearest - value) ? target : nearest, targets[0]);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  const canvasWidth = width;
  const canvasHeight = freeCanvasHeight(elements);

  const startDrag = (event: ReactPointerEvent<HTMLElement>, element: FreeDesignElement) => {
    const startX = event.clientX;
    const startY = event.clientY;
    const original = { x: element.x, y: element.y };
    let latest = original;
    event.currentTarget.setPointerCapture(event.pointerId);
    onSelect(element.id);
    const move = (moveEvent: PointerEvent) => {
      const parent = element.parentId ? elements.find((item) => item.id === element.parentId) : undefined;
      const maxX = Math.max(0, (parent?.w ?? width) - element.w);
      const nextX = Math.min(maxX, Math.max(0, original.x + moveEvent.clientX - startX));
      const nextY = Math.max(0, original.y + moveEvent.clientY - startY);
      latest = { x: Math.round(nextX), y: Math.round(nextY) };
      const verticalGuide = nearestGuide(nextX, [0, Math.round(maxX / 2), maxX]);
      const maxY = parent ? Math.max(0, parent.h - element.h) : 320;
      const horizontalGuide = nearestGuide(nextY, [0, Math.round(maxY / 2), maxY]);
      setGuides({
        x: (parent?.x ?? 0) + verticalGuide + element.w / 2,
        y: (parent?.y ?? 0) + horizontalGuide,
      });
      onChange((items) => items.map((item) => item.id === element.id ? { ...item, ...latest } : item));
    };
    const end = () => {
      setGuides({});
      if (element.kind !== 'layout') {
        const centerX = latest.x + element.w / 2;
        const centerY = latest.y + element.h / 2;
        const parent = findContainingLayout(elements.filter((item) => item.id !== element.id), centerX, centerY);
        if (parent) {
          const localX = Math.max(0, Math.min(Math.max(0, parent.w - element.w), Math.round(latest.x - parent.x)));
          onChange((items) => fitLayouts(items.map((item) => item.id === element.id ? {
            ...item,
            parentId: parent.id,
            x: localX,
            y: Math.max(0, Math.round(latest.y - parent.y)),
            w: layoutChildWidth(parent),
          } : item)));
        }
      }
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  const startResize = (event: ReactPointerEvent<HTMLSpanElement>, element: FreeDesignElement) => {
    event.stopPropagation();
    const startX = event.clientX;
    const startY = event.clientY;
    const original = { w: element.w, h: element.h };
    event.currentTarget.setPointerCapture(event.pointerId);
    onSelect(element.id);
    const move = (moveEvent: PointerEvent) => {
      const nextW = Math.max(0, original.w + moveEvent.clientX - startX);
      const nextH = Math.max(0, original.h + moveEvent.clientY - startY);
      onChange((items) => items.map((item) => item.id === element.id ? { ...item, w: Math.round(nextW), h: Math.round(nextH) } : item));
    };
    const end = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
  };

  return (
    <section className="campaign-free-workspace">
      <div
        className="campaign-free-canvas"
        style={{ width: canvasWidth, height: canvasHeight }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          const rect = event.currentTarget.getBoundingClientRect();
          const blockKind = (event.dataTransfer.getData('application/x-campaign-block') || draggingBlockKind) as CampaignBlockKind;
          const layoutKind = (event.dataTransfer.getData('application/x-campaign-layout') || draggingLayoutKind) as LayoutKind;
          if (ALLOWED_BLOCK_KINDS.has(blockKind)) {
            const x = event.clientX - rect.left;
            const y = event.clientY - rect.top;
            const parent = findContainingLayout(elements, x, y);
            onDropBlock(blockKind, x, y, parent?.id);
          }
          if (ALLOWED_LAYOUT_KINDS.has(layoutKind)) {
            onDropLayout(layoutKind, event.clientX - rect.left, event.clientY - rect.top);
          }
          onDragDone();
          onLayoutDragDone();
        }}
      >
        {guides.x !== undefined ? <span className="campaign-guide vertical" style={{ left: guides.x }} /> : null}
        {guides.y !== undefined ? <span className="campaign-guide horizontal" style={{ top: guides.y }} /> : null}
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          return (
          <div
            key={element.id}
            className={`campaign-free-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${selectedId === element.id ? 'selected' : ''}`}
            role="button"
            tabIndex={0}
            style={freeElementStyle(element)}
            onPointerDown={(event) => startDrag(event, element)}
          >
            {childElements.length ? childElements.map((child) => (
              <span
                key={child.id}
                className={`campaign-free-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${selectedId === child.id ? 'selected' : ''}`}
                style={freeElementStyle(child, true)}
                onPointerDown={(event) => {
                  event.stopPropagation();
                  if (selectedId === child.id) {
                    startDrag(event, child);
                    return;
                  }
                  onSelect(child.id);
                }}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelect(child.id);
                }}
              >
                {renderFreeElementText(child.text)}
              </span>
            )) : renderFreeElementText(element.text)}
            {selectedId === element.id ? <span className="campaign-free-resize" aria-label="调整尺寸" onPointerDown={(event) => startResize(event, element)} /> : null}
          </div>
          );
        })}
      </div>
      <div className="campaign-mobile-check">自由设计：元素可拖拽定位，保存到 Campaign Page schema。</div>
    </section>
  );
}

function FreeElementInspector({
  element,
  tab,
  width,
  onChange,
  onLayerMove,
}: {
  element?: FreeDesignElement;
  tab: EditorTab;
  width: DeviceWidth;
  onChange: (patch: Partial<FreeDesignElement>) => void;
  onLayerMove: (direction: -1 | 1) => void;
}) {
  if (!element) return <div className="campaign-inspector-body"><p>选择一个自由元素后编辑属性。</p></div>;
  if (tab === 'binding') {
    return (
      <div className="campaign-inspector-body">
        <label><span>绑定字段</span><input value={element.binding ?? ''} placeholder="{{ activity.title }}" onChange={(event) => onChange({ binding: event.target.value })} /></label>
        <label><span>展示条件</span><input value={element.visibleWhen ?? ''} placeholder="{{ user.loggedIn }}" onChange={(event) => onChange({ visibleWhen: event.target.value })} /></label>
        <p>自由元素绑定同样使用 Activity View Model 白名单表达式。</p>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <label><span>点击动作</span><select value={element.action || 'NONE'} onChange={(event) => onChange({ action: event.target.value === 'NONE' ? undefined : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></label>
        <label><span>Payload</span><input value={element.payload ?? ''} placeholder="{{ activity.id, reward.code, user.id }}" onChange={(event) => onChange({ payload: event.target.value })} /></label>
        <p>自由设计动作在发布 schema 中保存，运行时按 workflow event 触发。</p>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <label><span>元素文案</span><textarea value={element.text.split('\\n').join('\n')} onChange={(event) => onChange({ text: event.target.value.split('\n').join('\\n') })} /></label>
      <div className="campaign-free-grid">
        <label><span>X</span><input type="number" value={element.x} onChange={(event) => onChange({ x: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>Y</span><input type="number" value={element.y} onChange={(event) => onChange({ y: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>宽</span><input type="number" value={element.w} onChange={(event) => onChange({ w: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>高</span><input type="number" value={element.h} onChange={(event) => onChange({ h: Math.max(0, Number(event.target.value)) })} /></label>
      </div>
      <label><span>层级</span><input type="number" value={element.z} onChange={(event) => onChange({ z: Number(event.target.value) })} /></label>
      <div className="campaign-inspector-actions">
        <button type="button" onClick={() => onLayerMove(1)}>上移一层</button>
        <button type="button" onClick={() => onLayerMove(-1)}>下移一层</button>
      </div>
      <label><span>圆角</span><input value={element.radius ?? '8'} onChange={(event) => onChange({ radius: event.target.value })} /></label>
      {element.kind === 'layout' ? (
        <>
          <label>
            <span>布局方式</span>
            <select value={normalizeLayoutKind(element.layout)} onChange={(event) => {
              const layout = event.target.value as LayoutKind;
              onChange({ layout, ...flexPreset(layout) });
            }}>
              <option value="horizontal-layout">水平布局</option>
              <option value="vertical-layout">垂直布局</option>
            </select>
          </label>
          <div className="campaign-free-grid">
            <label><span>方向</span><select value={element.flexDirection ?? 'row'} onChange={(event) => onChange({ flexDirection: event.target.value as FlexDirection })}><option value="row">横向</option><option value="column">纵向</option></select></label>
            <label><span>换行</span><select value={element.flexWrap ?? 'nowrap'} onChange={(event) => onChange({ flexWrap: event.target.value as FlexWrap })}><option value="nowrap">不换行</option><option value="wrap">换行</option></select></label>
            <label><span>主轴</span><select value={element.justifyContent ?? 'center'} onChange={(event) => onChange({ justifyContent: event.target.value as FlexJustify })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="space-between">两端</option><option value="space-around">环绕</option></select></label>
            <label><span>交叉轴</span><select value={element.alignItems ?? 'center'} onChange={(event) => onChange({ alignItems: event.target.value as FlexAlign })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="stretch">拉伸</option></select></label>
            <label><span>间距</span><input type="number" value={element.gap ?? 8} onChange={(event) => onChange({ gap: Math.max(0, Number(event.target.value)) })} /></label>
          </div>
        </>
      ) : null}
      <label><span>样式</span><select value={element.variant ?? 'primary'} onChange={(event) => onChange({ variant: event.target.value as FreeDesignElement['variant'] })}><option value="primary">主要样式</option><option value="secondary">次要样式</option></select></label>
      <p>{width}px 自由画布 · 文案、位置、尺寸、层级、样式会进入发布 schema。</p>
    </div>
  );
}

export function CampaignRuntimePage() {
  const page = campaignPageApi.loadPublishedPage() ?? campaignPageApi.loadPage();
  return (
    <main className="campaign-runtime-page">
      <CampaignRuntimeView page={page} />
    </main>
  );
}

function CampaignRuntimeView({ page, embedded = false }: { page: CampaignPageSchema; embedded?: boolean }) {
  const elements = [...(page.freeElements ?? [])].sort((a, b) => a.z - b.z);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  if (elements.length) {
    const canvasHeight = embedded ? freeCanvasHeight(elements, 0) : undefined;
    return (
      <section className={embedded ? 'campaign-runtime-canvas embedded-preview' : 'campaign-runtime-canvas'} style={{ width: page.device.defaultWidth, ...(canvasHeight ? { height: canvasHeight, minHeight: canvasHeight } : {}) }}>
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          return (
          <div
            key={element.id}
            className={`campaign-runtime-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${element.variant ?? 'primary'}`}
            role="button"
            data-action={element.action ?? ''}
            style={freeElementStyle(element)}
          >
            {childElements.length ? childElements.map((child) => (
              <span key={child.id} className={`campaign-runtime-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${child.variant ?? 'primary'}`} style={freeElementStyle(child, true)}>
                {renderFreeElementText(child.text)}
              </span>
            )) : renderFreeElementText(element.text)}
          </div>
          );
        })}
      </section>
    );
  }
  return <Render config={puckConfig} data={campaignToPuckData(page)} />;
}

function CampaignModulePreview({
  page,
  selectedId,
  onSelect,
  draggingBlockKind,
  onDropBlock,
  onDragDone,
}: {
  page: CampaignPageSchema;
  selectedId: string;
  onSelect: (id: string) => void;
  draggingBlockKind: CampaignBlockKind | null;
  onDropBlock: (kind: CampaignBlockKind) => void;
  onDragDone: () => void;
}) {
  const elements = [...(page.freeElements ?? [])].sort((a, b) => a.z - b.z);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  const selectBlockFromFreeElement = (id: string) => {
    const blockId = blockIdForFreeElementId(id);
    if (blockId) onSelect(blockId);
  };
  const canvasHeight = freeCanvasHeight(elements);
  return (
    <section className="campaign-free-workspace">
      <div
        className="campaign-free-canvas campaign-mobile-canvas campaign-module-preview-canvas"
        style={{ width: page.device.defaultWidth, height: canvasHeight }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          const kind = (event.dataTransfer.getData('application/x-campaign-block') || draggingBlockKind) as CampaignBlockKind;
          if (ALLOWED_BLOCK_KINDS.has(kind)) onDropBlock(kind);
          onDragDone();
        }}
      >
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          const blockId = blockIdForFreeElementId(element.id);
          return (
            <div
              key={element.id}
              className={`campaign-runtime-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${element.variant ?? 'primary'} ${selectedId === blockId ? 'selected' : ''}`}
              role="button"
              data-action={element.action ?? ''}
              style={freeElementStyle(element)}
              onClick={() => selectBlockFromFreeElement(element.id)}
            >
              {childElements.length ? childElements.map((child) => {
                const childBlockId = blockIdForFreeElementId(child.id);
                return (
                  <span
                    key={child.id}
                    className={`campaign-runtime-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${child.variant ?? 'primary'} ${selectedId === childBlockId ? 'selected' : ''}`}
                    style={freeElementStyle(child, true)}
                    onClick={(event) => {
                      event.stopPropagation();
                      selectBlockFromFreeElement(child.id);
                    }}
                  >
                    {renderFreeElementText(child.text)}
                  </span>
                );
              }) : renderFreeElementText(element.text)}
            </div>
          );
        })}
      </div>
      <div className="campaign-mobile-check">模块预览：使用页面设计同一坐标、盒子和层级。</div>
    </section>
  );
}

function CampaignPreview({ blocks, selectedId, onSelect, onAddBlock }: { blocks: CampaignBlock[]; selectedId: string; onSelect: (id: string) => void; onAddBlock: () => void }) {
  return (
    <>
      {blocks.map((block, index) => (
        <div key={block.id}>
          {index > 0 ? <AddBetween onClick={onAddBlock} /> : null}
          <button className={`campaign-preview-block ${block.kind.toLowerCase()} ${block.id === selectedId ? 'selected' : ''}`} type="button" onClick={() => onSelect(block.id)}>
            {block.kind === 'HeroBanner' ? (
              <div className="campaign-hero-art"><strong>夏日充值<br />返利活动</strong><span>充值越多，返利越高</span><small>活动时间：2025.05.20 00:00:00 - 2025.06.20 23:59:59</small><i>素材</i></div>
            ) : block.kind === 'RewardCard' ? (
              <div className="campaign-reward-card"><span>奖励金额</span><b>￥188</b><em>可领取</em><p>单笔充值满 500 元可领</p><i>素材</i></div>
            ) : block.kind === 'Countdown' ? (
              <div className="campaign-countdown"><b>活动倒计时</b><p><span>06<small>天</small></span>:<span>18<small>时</small></span>:<span>32<small>分</small></span>:<span>45<small>秒</small></span></p></div>
            ) : block.kind === 'ProgressBlock' ? (
              <div className="campaign-progress"><b>参与进度</b><em>已参与 1,258 人</em><i><span /></i><p>目标 2,000 人 <strong>62%</strong></p></div>
            ) : block.kind === 'ClaimButton' ? (
              <span className="campaign-claim-button">{block.text ?? '立即领取'}</span>
            ) : block.kind === 'RuleText' ? (
              <div className="campaign-rules"><b>活动规则</b><p>1. 活动期间，单笔充值满 500 元可领取奖励；</p><p>2. 每个用户仅可领取一次奖励；</p><p>3. 奖励将于 24 小时内发放至账户。</p></div>
            ) : block.kind === 'Title' ? (
              <h3 className="campaign-title-block">夏日充值返利活动</h3>
            ) : (
              <div className="campaign-rules"><b>FAQ</b><p>奖励到账时间、参与资格和领取问题。</p></div>
            )}
          </button>
        </div>
      ))}
    </>
  );
}

function InspectorContent({
  block,
  tab,
  deviceWidth,
  onBlockChange,
  onMove,
}: {
  block: CampaignBlock;
  tab: EditorTab;
  deviceWidth: DeviceWidth;
  onBlockChange: (patch: Partial<CampaignBlock>) => void;
  onMove: (direction: -1 | 1) => void;
}) {
  if (tab === 'binding') {
    return (
      <div className="campaign-inspector-body">
        <label><span>禁用条件 / 绑定字段</span><input value={block.binding ?? (block.kind === 'ClaimButton' ? '{{ !eligibility.canClaim }}' : '{{ activity.title }}')} onChange={(event) => onBlockChange({ binding: event.target.value, bound: true })} /></label>
        <label><span>展示条件</span><input value={block.visibleWhen ?? '{{ user.loggedIn }}'} onChange={(event) => onBlockChange({ visibleWhen: event.target.value })} /></label>
        <p>绑定表达式只支持 Activity View Model 白名单字段，不暴露 workflow 内部节点。</p>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <label><span>点击动作</span><select value={block.action || 'NONE'} onChange={(event) => onBlockChange({ action: event.target.value === 'NONE' ? undefined : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></label>
        <label><span>Payload</span><input value={block.payload ?? '{{ activity.id, reward.code, user.id }}'} onChange={(event) => onBlockChange({ payload: event.target.value })} /></label>
        <p>动作绑定到 ExMarketing workflow event，不允许任意脚本。</p>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <label><span>{block.kind === 'ClaimButton' ? '按钮文案' : '模块标题'}</span><input value={block.kind === 'ClaimButton' ? block.text ?? '立即领取' : block.title} onChange={(event) => onBlockChange(block.kind === 'ClaimButton' ? { text: event.target.value, title: event.target.value || '领取按钮' } : { title: event.target.value })} /></label>
      <div className="campaign-button-preview">{block.kind === 'ClaimButton' ? block.text ?? '立即领取' : block.title}</div>
      <label><span>按钮样式</span><select value={block.variant ?? 'primary'} onChange={(event) => onBlockChange({ variant: event.target.value as CampaignBlock['variant'] })}><option value="primary">主要按钮（蓝色）</option><option value="secondary">次要按钮</option></select></label>
      <label><span>宽度</span><select value={block.width ?? 'full'} onChange={(event) => onBlockChange({ width: event.target.value as CampaignBlock['width'] })}><option value="full">全宽</option><option value="content">内容宽度</option></select></label>
      <label><span>圆角</span><input value={block.radius ?? '8'} onChange={(event) => onBlockChange({ radius: event.target.value })} /></label>
      <div className="campaign-inspector-actions">
        <button type="button" onClick={() => onMove(-1)}>上移</button>
        <button type="button" onClick={() => onMove(1)}>下移</button>
      </div>
      <p>{deviceWidth}px 画布 · {blockDescription(block.kind)}</p>
    </div>
  );
}
