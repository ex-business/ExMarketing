import type { Page as EditorPage } from './editorSchema';

export type CampaignBlockKind = 'HeroBanner' | 'Title' | 'RewardCard' | 'Countdown' | 'ProgressBlock' | 'ClaimButton' | 'RuleText' | 'FAQBlock';
export type ComponentRegistryKey = 'Title' | 'HeroBanner' | 'RewardCard' | 'Button' | 'Image' | 'HorizontalLayout' | 'VerticalLayout';
export type EditorTab = 'content' | 'binding' | 'action';
export type DeviceWidth = 375 | 393 | 414;
export type WorkspaceSection = 'page' | 'resources' | 'settings' | 'templates';
export type LayoutKind = 'horizontal-layout' | 'vertical-layout' | 'horizontal-center' | 'vertical-center' | 'two-column';
export type FlexDirection = 'row' | 'column';
export type FlexJustify = 'flex-start' | 'center' | 'flex-end' | 'space-between' | 'space-around';
export type FlexAlign = 'flex-start' | 'center' | 'flex-end' | 'stretch';
export type FlexWrap = 'nowrap' | 'wrap';

export type CampaignBlock = {
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

export type FreeDesignElement = {
  id: string;
  name?: string;
  component?: ComponentRegistryKey;
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

export type CampaignPageSchema = {
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
  editor: EditorPage;
  blocks: CampaignBlock[];
  freeElements: FreeDesignElement[];
  status: 'DRAFT' | 'PUBLISHED' | 'OFFLINE';
  version: string;
  updatedAt: string;
};

export type CampaignPageVersion = {
  id: string;
  pageId: string;
  version: string;
  status: 'PUBLISHED';
  schemaSnapshot: CampaignPageSchema;
  publishedBy: string;
  publishedAt: string;
};

export type CampaignPageSummary = {
  id: string;
  name: string;
  status: 'DRAFT' | 'PUBLISHED' | 'OFFLINE';
  version: string;
  path: string;
  updatedAt: string;
  createdBy: string;
};

export type CampaignValidationIssue = {
  code: string;
  message: string;
};

export type WorkspaceItem = { title: string; meta: string; detail: string; group: string; tags: string[] };
export type ResourceTab = '数据' | '数据源' | '动作' | '样式' | '图片';
export type ResourceStatus = '启用' | '停用' | '待保存' | '尺寸提醒' | '大小错误';

export type ResourceRecord = {
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

export type UploadQueueItem = {
  id: string;
  fileName: string;
  purpose: string;
  recommendedSize: string;
  status: ResourceStatus;
  sizeLabel: string;
};

export type PageTreeNode =
  | { id: string; type: 'block'; title: string; meta: string; block: CampaignBlock; children?: PageTreeNode[] }
  | { id: string; type: 'layout'; title: string; meta: string; element: FreeDesignElement; children?: PageTreeNode[] };
