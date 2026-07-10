import type { ReactNode } from 'react';
import type { Node as EditorNode } from './editorSchema';
import type {
  CampaignBlock,
  CampaignBlockKind,
  ComponentRegistryKey,
  DeviceWidth,
  FreeDesignElement,
  LayoutKind,
} from './types';
import { renderFreeElementText } from './renderUtils';

type EditableProperty = {
  key: keyof FreeDesignElement | 'resourceBindings' | 'actionBindings';
  label: string;
  tab: 'content' | 'binding' | 'action' | 'style';
  control: 'text' | 'textarea' | 'select' | 'number' | 'resource' | 'action';
};

type BindingCapability = {
  slot: string;
  label: string;
  defaultExpression?: string;
};

type ActionCapability = {
  trigger: 'click' | 'exposure' | 'submit' | 'timer';
  label: string;
  defaultActionId?: string;
};

export type ComponentDefinition = {
  key: ComponentRegistryKey;
  label: string;
  paletteLabel: string;
  defaultTitle: string;
  elementKind: FreeDesignElement['kind'];
  layoutKind?: LayoutKind;
  defaultSize: (deviceWidth: DeviceWidth) => Pick<FreeDesignElement, 'w' | 'h'>;
  defaultText: (block?: CampaignBlock) => string;
  editableProperties: EditableProperty[];
  defaultStyle: Partial<Pick<FreeDesignElement, 'variant' | 'radius' | 'flexDirection' | 'justifyContent' | 'alignItems' | 'flexWrap' | 'gap'>>;
  resourceBindings: BindingCapability[];
  actionBindings: ActionCapability[];
  runtimeRender: (node: EditorNode, children: ReactNode) => ReactNode;
};

const commonContentProps: EditableProperty[] = [
  { key: 'text', label: '文案', tab: 'content', control: 'textarea' },
  { key: 'binding', label: '资源绑定', tab: 'binding', control: 'resource' },
  { key: 'radius', label: '圆角', tab: 'style', control: 'number' },
];

const layoutProps: EditableProperty[] = [
  { key: 'text', label: '占位文案', tab: 'content', control: 'textarea' },
  { key: 'flexDirection', label: '方向', tab: 'style', control: 'select' },
  { key: 'justifyContent', label: '主轴', tab: 'style', control: 'select' },
  { key: 'alignItems', label: '交叉轴', tab: 'style', control: 'select' },
  { key: 'flexWrap', label: '换行', tab: 'style', control: 'select' },
  { key: 'gap', label: '间距', tab: 'style', control: 'number' },
];

export const componentRegistry: Record<ComponentRegistryKey, ComponentDefinition> = {
  Title: {
    key: 'Title',
    label: '标题',
    paletteLabel: '标题',
    defaultTitle: '新标题',
    elementKind: 'text',
    defaultSize: (width) => ({ w: Math.min(260, width - 56), h: 46 }),
    defaultText: (block) => block?.title === '活动标题' ? '夏日充值返利活动' : block?.title ?? '新标题',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '0' },
    resourceBindings: [{ slot: 'text', label: '标题文案', defaultExpression: '{{ activity.title }}' }],
    actionBindings: [],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  HeroBanner: {
    key: 'HeroBanner',
    label: '头图',
    paletteLabel: '头图',
    defaultTitle: '新头图',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 120 }),
    defaultText: () => '夏日充值返利活动\n充值越多，返利越高',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8', variant: 'secondary' },
    resourceBindings: [
      { slot: 'title', label: '主标题', defaultExpression: '{{ activity.title }}' },
      { slot: 'image', label: '头图图片', defaultExpression: '{{ hero.banner }}' },
    ],
    actionBindings: [{ trigger: 'exposure', label: '曝光事件', defaultActionId: 'TRACK_EVENT' }],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  RewardCard: {
    key: 'RewardCard',
    label: '奖励卡',
    paletteLabel: '奖励卡',
    defaultTitle: '新奖励卡片',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 94 }),
    defaultText: () => '奖励金额 ￥188\n单笔充值满 500 元可领',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8' },
    resourceBindings: [
      { slot: 'amount', label: '奖励金额', defaultExpression: '{{ reward.amount }}' },
      { slot: 'condition', label: '领取条件', defaultExpression: '{{ reward.condition }}' },
    ],
    actionBindings: [{ trigger: 'exposure', label: '奖励曝光', defaultActionId: 'TRACK_EVENT' }],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  Countdown: {
    key: 'Countdown',
    label: '倒计时',
    paletteLabel: '倒计时',
    defaultTitle: '新倒计时',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 82 }),
    defaultText: () => '活动倒计时\n06天 18时 32分 45秒',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8', variant: 'secondary' },
    resourceBindings: [{ slot: 'endTime', label: '结束时间', defaultExpression: '{{ countdown.endTime }}' }],
    actionBindings: [{ trigger: 'timer', label: '倒计时结束事件' }],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  ProgressBlock: {
    key: 'ProgressBlock',
    label: '参与进度',
    paletteLabel: '进度',
    defaultTitle: '新参与进度',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 88 }),
    defaultText: () => '参与进度\n已参与 1,258 人 · 62%',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8' },
    resourceBindings: [{ slot: 'percent', label: '进度值', defaultExpression: '{{ progress.percent }}' }],
    actionBindings: [],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  Button: {
    key: 'Button',
    label: '按钮',
    paletteLabel: '按钮',
    defaultTitle: '新领取按钮',
    elementKind: 'button',
    defaultSize: (width) => ({ w: width - 36, h: 52 }),
    defaultText: (block) => block?.text ?? block?.title ?? '立即领取',
    editableProperties: [
      ...commonContentProps,
      { key: 'action', label: '点击动作', tab: 'action', control: 'action' },
      { key: 'payload', label: 'Payload', tab: 'action', control: 'textarea' },
    ],
    defaultStyle: { radius: '8', variant: 'primary' },
    resourceBindings: [{ slot: 'disabled', label: '禁用条件', defaultExpression: '{{ !eligibility.canClaim }}' }],
    actionBindings: [{ trigger: 'click', label: '点击动作', defaultActionId: 'CLAIM_REWARD' }],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  RuleText: {
    key: 'RuleText',
    label: '活动规则',
    paletteLabel: '规则',
    defaultTitle: '新活动规则',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 94 }),
    defaultText: () => '活动规则\n1. 单笔充值满 500 元可领取奖励\n2. 每个用户仅可领取一次',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8', variant: 'secondary' },
    resourceBindings: [{ slot: 'rules', label: '规则文案', defaultExpression: '{{ activity.rules }}' }],
    actionBindings: [],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  FAQBlock: {
    key: 'FAQBlock',
    label: 'FAQ',
    paletteLabel: 'FAQ',
    defaultTitle: '新 FAQ',
    elementKind: 'card',
    defaultSize: (width) => ({ w: width - 36, h: 94 }),
    defaultText: () => 'FAQ\n奖励到账时间、参与资格和领取问题。',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8', variant: 'secondary' },
    resourceBindings: [{ slot: 'items', label: 'FAQ 数据', defaultExpression: '{{ faq.block }}' }],
    actionBindings: [],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  Image: {
    key: 'Image',
    label: '图片',
    paletteLabel: '图片',
    defaultTitle: '新图片',
    elementKind: 'image',
    defaultSize: (width) => ({ w: width - 36, h: 140 }),
    defaultText: () => '图片占位',
    editableProperties: commonContentProps,
    defaultStyle: { radius: '8' },
    resourceBindings: [
      { slot: 'src', label: '图片资源', defaultExpression: '{{ image.url }}' },
      { slot: 'alt', label: 'Alt 文案', defaultExpression: '{{ image.alt }}' },
    ],
    actionBindings: [{ trigger: 'click', label: '图片点击', defaultActionId: 'NAVIGATE' }],
    runtimeRender: (node) => renderFreeElementText(node.content.text),
  },
  HorizontalLayout: {
    key: 'HorizontalLayout',
    label: '水平布局',
    paletteLabel: '水平布局',
    defaultTitle: '水平布局盒子',
    elementKind: 'layout',
    layoutKind: 'horizontal-layout',
    defaultSize: () => ({ w: 345, h: 96 }),
    defaultText: () => '水平布局盒子\n拖入组件到盒子内',
    editableProperties: layoutProps,
    defaultStyle: { radius: '8', flexDirection: 'row', justifyContent: 'flex-start', alignItems: 'center', flexWrap: 'nowrap', gap: 8 },
    resourceBindings: [],
    actionBindings: [],
    runtimeRender: (node, children) => children || renderFreeElementText(node.content.text),
  },
  VerticalLayout: {
    key: 'VerticalLayout',
    label: '垂直布局',
    paletteLabel: '垂直布局',
    defaultTitle: '垂直布局盒子',
    elementKind: 'layout',
    layoutKind: 'vertical-layout',
    defaultSize: () => ({ w: 345, h: 144 }),
    defaultText: () => '垂直布局盒子\n拖入组件到盒子内',
    editableProperties: layoutProps,
    defaultStyle: { radius: '8', flexDirection: 'column', justifyContent: 'flex-start', alignItems: 'stretch', flexWrap: 'nowrap', gap: 8 },
    resourceBindings: [],
    actionBindings: [],
    runtimeRender: (node, children) => children || renderFreeElementText(node.content.text),
  },
};

export const registeredComponentKeys = Object.keys(componentRegistry) as ComponentRegistryKey[];

export function componentKeyForBlockKind(kind: CampaignBlockKind): ComponentRegistryKey {
  if (kind === 'Title') return 'Title';
  if (kind === 'HeroBanner') return 'HeroBanner';
  if (kind === 'RewardCard') return 'RewardCard';
  if (kind === 'Countdown') return 'Countdown';
  if (kind === 'ProgressBlock') return 'ProgressBlock';
  if (kind === 'ClaimButton') return 'Button';
  if (kind === 'RuleText') return 'RuleText';
  if (kind === 'FAQBlock') return 'FAQBlock';
  return 'RewardCard';
}

export function componentKeyForLayoutKind(kind?: LayoutKind): ComponentRegistryKey {
  return kind === 'vertical-layout' || kind === 'vertical-center' ? 'VerticalLayout' : 'HorizontalLayout';
}

export function componentDefinitionForElement(element: FreeDesignElement): ComponentDefinition {
  if (element.component) return componentRegistry[element.component];
  if (element.kind === 'layout') return componentRegistry[componentKeyForLayoutKind(element.layout)];
  if (element.kind === 'text') return componentRegistry.Title;
  if (element.kind === 'button') return componentRegistry.Button;
  if (element.kind === 'image') return componentRegistry.Image;
  return componentRegistry.RewardCard;
}

export function componentKeyForNode(node: EditorNode): ComponentRegistryKey {
  if (node.type === 'container') return componentKeyForLayoutKind(node.layout);
  if (node.component === 'ClaimButton' || node.component === 'Button') return 'Button';
  if (node.component === 'Text') return 'Title';
  if (node.component === 'Card') return 'RewardCard';
  return node.component as ComponentRegistryKey;
}

export function componentDefinitionForNode(node: EditorNode): ComponentDefinition {
  return componentRegistry[componentKeyForNode(node)];
}

export function defaultElementFromComponent({
  componentKey,
  id,
  width,
  z,
  x,
  y,
  block,
}: {
  componentKey: ComponentRegistryKey;
  id: string;
  width: DeviceWidth;
  z: number;
  x?: number;
  y?: number;
  block?: CampaignBlock;
}): FreeDesignElement {
  const definition = componentRegistry[componentKey];
  const size = definition.defaultSize(width);
  return {
    id,
    name: block?.title ?? definition.defaultTitle,
    component: componentKey,
    kind: definition.elementKind,
    text: definition.defaultText(block),
    x: x ?? (definition.elementKind === 'text' ? 28 : 18),
    y: y ?? 28,
    w: size.w,
    h: size.h,
    z,
    binding: block?.binding ?? definition.resourceBindings[0]?.defaultExpression,
    visibleWhen: block?.visibleWhen,
    action: block ? block.action : definition.actionBindings.find((binding) => binding.trigger === 'click')?.defaultActionId,
    payload: block?.payload,
    variant: block?.variant ?? definition.defaultStyle.variant,
    layout: definition.layoutKind,
    flexDirection: definition.defaultStyle.flexDirection,
    justifyContent: definition.defaultStyle.justifyContent,
    alignItems: definition.defaultStyle.alignItems,
    flexWrap: definition.defaultStyle.flexWrap,
    gap: definition.defaultStyle.gap,
    radius: block?.radius ?? definition.defaultStyle.radius,
  };
}

export function renderRuntimeNode(node: EditorNode, children: ReactNode) {
  return componentDefinitionForNode(node).runtimeRender(node, children);
}
