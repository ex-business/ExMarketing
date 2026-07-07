import type {
  CampaignBlock,
  CampaignBlockKind,
  DeviceWidth,
  FlexAlign,
  FlexDirection,
  FlexJustify,
  FlexWrap,
  FreeDesignElement,
  LayoutKind,
} from './types';
import { componentKeyForBlockKind, componentKeyForLayoutKind, componentRegistry, defaultElementFromComponent } from './componentRegistry';

export const LAYOUT_CHILD_HORIZONTAL_INSET = 26;

export function layoutChildWidth(layout: FreeDesignElement) {
  return Math.max(0, layout.w - LAYOUT_CHILD_HORIZONTAL_INSET);
}

export const blockPalette: Array<{ kind: CampaignBlockKind; label: string; title: string }> = [
  { kind: 'HeroBanner', label: '头图', title: '新头图' },
  { kind: 'Title', label: '标题', title: '新标题' },
  { kind: 'RewardCard', label: '奖励卡', title: '新奖励卡片' },
  { kind: 'Countdown', label: '倒计时', title: '新倒计时' },
  { kind: 'ProgressBlock', label: '进度', title: '新参与进度' },
  { kind: 'ClaimButton', label: '按钮', title: '新领取按钮' },
  { kind: 'RuleText', label: '规则', title: '新活动规则' },
  { kind: 'FAQBlock', label: 'FAQ', title: '新 FAQ' },
];

export const layoutPalette: Array<{ kind: LayoutKind; label: string; title: string }> = [
  { kind: 'horizontal-layout', label: '水平布局', title: '水平布局盒子' },
  { kind: 'vertical-layout', label: '垂直布局', title: '垂直布局盒子' },
];

export const ALLOWED_BLOCK_KINDS = new Set<CampaignBlockKind>(['HeroBanner', 'Title', 'RewardCard', 'Countdown', 'ProgressBlock', 'ClaimButton', 'RuleText', 'FAQBlock']);
export const ALLOWED_LAYOUT_KINDS = new Set<LayoutKind>(['horizontal-layout', 'vertical-layout']);

export function flexPreset(kind: LayoutKind): Pick<FreeDesignElement, 'flexDirection' | 'justifyContent' | 'alignItems' | 'flexWrap' | 'gap'> {
  const definition = componentRegistry[componentKeyForLayoutKind(kind)];
  if (definition.elementKind === 'layout') {
    return {
      flexDirection: definition.defaultStyle.flexDirection ?? 'row',
      justifyContent: definition.defaultStyle.justifyContent ?? 'flex-start',
      alignItems: definition.defaultStyle.alignItems ?? 'center',
      flexWrap: definition.defaultStyle.flexWrap ?? 'nowrap',
      gap: definition.defaultStyle.gap ?? 8,
    };
  }
  if (kind === 'two-column') return { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'stretch', flexWrap: 'nowrap', gap: 10 };
  return { flexDirection: 'row', justifyContent: 'flex-start', alignItems: 'center', flexWrap: 'nowrap', gap: 8 };
}

export function normalizeLayoutKind(kind?: LayoutKind): 'horizontal-layout' | 'vertical-layout' {
  return kind === 'vertical-layout' || kind === 'vertical-center' ? 'vertical-layout' : 'horizontal-layout';
}

export const initialBlocks: CampaignBlock[] = [
  { id: 'hero', kind: 'HeroBanner', title: '头图', visible: true, bound: true },
  { id: 'title', kind: 'Title', title: '活动标题', visible: true, bound: true },
  { id: 'reward', kind: 'RewardCard', title: '奖励卡片', visible: true, bound: true },
  { id: 'countdown', kind: 'Countdown', title: '倒计时', visible: true, bound: true },
  { id: 'progress', kind: 'ProgressBlock', title: '参与进度', visible: true, bound: true },
  { id: 'claim', kind: 'ClaimButton', title: '领取按钮', visible: true, bound: true, action: 'CLAIM_REWARD', text: '立即领取', binding: '{{ !eligibility.canClaim }}', visibleWhen: '{{ user.loggedIn }}', payload: '{{ activity.id, reward.code, user.id }}', variant: 'primary', width: 'full', radius: '8' },
  { id: 'rules', kind: 'RuleText', title: '活动规则', visible: true, bound: true },
  { id: 'faq', kind: 'FAQBlock', title: 'FAQ', visible: true, bound: true },
];

export const initialFreeElements: FreeDesignElement[] = [
  { id: 'free_hero_title', kind: 'text', text: '夏日充值返利活动', x: 28, y: 36, w: 238, h: 58, z: 1, binding: '{{ activity.title }}' },
  { id: 'free_reward_card', kind: 'card', text: '奖励金额 ￥188\\n单笔充值满 500 元可领', x: 18, y: 132, w: 345, h: 116, z: 2, binding: '{{ reward.amount }}', radius: '8' },
  { id: 'free_claim', kind: 'button', text: '立即领取', x: 18, y: 282, w: 345, h: 52, z: 3, binding: '{{ !eligibility.canClaim }}', visibleWhen: '{{ user.loggedIn }}', action: 'CLAIM_REWARD', payload: '{{ activity.id, reward.code, user.id }}', variant: 'primary', radius: '8' },
];

export function blockToFreeText(block: CampaignBlock) {
  if (block.kind === 'HeroBanner' || block.kind === 'Title' || block.kind === 'RewardCard' || block.kind === 'ClaimButton') {
    return componentRegistry[componentKeyForBlockKind(block.kind)].defaultText(block);
  }
  if (block.kind === 'Countdown') return '活动倒计时\\n06 天 18 时 32 分';
  if (block.kind === 'ProgressBlock') return '参与进度\\n已参与 1,258 人 · 62%';
  if (block.kind === 'RuleText') return '活动规则\\n1. 单笔充值满 500 元可领取奖励\\n2. 每个用户仅可领取一次';
  return 'FAQ\\n奖励到账时间、参与资格和领取问题。';
}

const semanticBlockTextPrefix: Partial<Record<CampaignBlockKind, string>> = {
  Countdown: '活动倒计时',
  ProgressBlock: '参与进度',
  RuleText: '活动规则',
  FAQBlock: 'FAQ',
};

export function syncedBlockText(block: CampaignBlock, existingText: string | undefined, freshText: string) {
  if (!existingText) return freshText;
  const expectedPrefix = semanticBlockTextPrefix[block.kind];
  if (!expectedPrefix) return existingText;
  const firstLine = existingText.split('\\n')[0]?.trim();
  return firstLine === expectedPrefix ? existingText : freshText;
}

export function blockToFreeKind(kind: CampaignBlockKind): FreeDesignElement['kind'] {
  if (kind === 'HeroBanner' || kind === 'Title' || kind === 'RewardCard' || kind === 'ClaimButton') return componentRegistry[componentKeyForBlockKind(kind)].elementKind;
  if (kind === 'Countdown' || kind === 'ProgressBlock' || kind === 'RuleText' || kind === 'FAQBlock') return 'card';
  return 'text';
}

export function legacyFreeElementIdForBlock(block: CampaignBlock) {
  if (block.id === 'title') return 'free_hero_title';
  if (block.id === 'reward') return 'free_reward_card';
  if (block.id === 'claim') return 'free_claim';
  return undefined;
}

export function blockIdForFreeElementId(id: string) {
  if (id.startsWith('free_from_')) return id.replace('free_from_', '');
  if (id === 'free_hero_title') return 'title';
  if (id === 'free_reward_card') return 'reward';
  if (id === 'free_claim') return 'claim';
  return '';
}

export function currentFreeElementIdForBlock(blockId: string, elements: FreeDesignElement[]) {
  return elements.find((element) => blockIdForFreeElementId(element.id) === blockId)?.id;
}

export function blocksToFreeElements(blocks: CampaignBlock[], width: DeviceWidth): FreeDesignElement[] {
  let cursorY = 28;
  return blocks.filter((block) => block.visible).map((block, index) => {
    const element: FreeDesignElement = {
      ...blockToFreeElement(block, width, index + 1),
      y: cursorY,
    };
    cursorY += element.h + 18;
    return element;
  });
}

export function blockToFreeElement(block: CampaignBlock, width: DeviceWidth, z: number): FreeDesignElement {
  const componentKey = componentKeyForBlockKind(block.kind);
  const element = defaultElementFromComponent({
    componentKey,
    id: `free_from_${block.id}`,
    width,
    z,
    block,
  });
  return {
    ...element,
    text: blockToFreeText(block),
    kind: blockToFreeKind(block.kind),
    component: componentKey,
    binding: block.binding,
    visibleWhen: block.visibleWhen,
    action: block.action,
    payload: block.payload,
    variant: block.variant ?? element.variant,
    radius: block.radius ?? element.radius ?? '8',
  };
}

export function syncFreeElementsWithBlocks(blocks: CampaignBlock[], width: DeviceWidth, currentFreeElements: FreeDesignElement[]) {
  const visibleBlocks = blocks.filter((block) => block.visible);
  const visibleBlockById = new Map(visibleBlocks.map((block) => [block.id, block]));
  const defaultElementsByBlockId = new Map(
    blocksToFreeElements(visibleBlocks, width).map((element) => [blockIdForFreeElementId(element.id), element]),
  );
  const existingById = new Map(currentFreeElements.map((element) => [element.id, element]));
  const visibleFreeIds = new Set(visibleBlocks.map((block) => `free_from_${block.id}`));
  const currentBlockIds = new Set(currentFreeElements.map((element) => blockIdForFreeElementId(element.id)).filter(Boolean));
  const missingDefaultBlock = visibleBlocks.some((block) => !currentBlockIds.has(block.id));
  const legacyInitialIds = new Set(['free_hero_title', 'free_reward_card', 'free_claim']);
  const legacyOnlySnapshot = missingDefaultBlock && currentFreeElements.length > 0 && currentFreeElements.every((element) => legacyInitialIds.has(element.id));
  if (legacyOnlySnapshot) {
    const legacyByBlockId = new Map(currentFreeElements.map((element) => [blockIdForFreeElementId(element.id), element]));
    return fitLayouts(visibleBlocks.map((block, index) => {
      const fresh = defaultElementsByBlockId.get(block.id) ?? blockToFreeElement(block, width, index + 1);
      const legacy = legacyByBlockId.get(block.id);
      if (!legacy) return fresh;
      return {
        ...fresh,
        id: legacy.id,
        name: legacy.name ?? fresh.name,
        text: syncedBlockText(block, legacy.text, fresh.text),
        binding: legacy.binding,
        visibleWhen: legacy.visibleWhen,
        action: legacy.action,
        payload: legacy.payload,
        variant: legacy.variant ?? fresh.variant,
        radius: legacy.radius ?? fresh.radius,
      };
    }));
  }
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
      name: element.name ?? fresh.name,
      text: syncedBlockText(block, element.text, fresh.text),
      binding: element.binding,
      visibleWhen: element.visibleWhen,
      action: element.action,
      payload: element.payload,
      variant: element.variant ?? fresh.variant,
      radius: element.radius ?? fresh.radius,
      w: parent ? layoutChildWidth(parent) : element.w,
    }];
  });
  const legacyBlockIds = new Set(legacySyncedElements.map((element) => blockIdForFreeElementId(element.id)).filter(Boolean));
  const defaultBlockIds = new Set(initialBlocks.map((block) => block.id));
  const blockElements = visibleBlocks.flatMap((block, index) => {
    if (legacyBlockIds.has(block.id)) return [];
    const fresh = defaultElementsByBlockId.get(block.id) ?? blockToFreeElement(block, width, index + 1);
    const existing = existingById.get(fresh.id);
    if (!existing) return [fresh];
    const parent = existing.parentId ? currentFreeElements.find((element) => element.id === existing.parentId && element.kind === 'layout') : undefined;
    return [{
      ...fresh,
      ...existing,
      name: existing.name ?? fresh.name,
      text: syncedBlockText(block, existing.text, fresh.text),
      binding: existing.binding,
      visibleWhen: existing.visibleWhen,
      action: existing.action,
      payload: existing.payload,
      variant: existing.variant ?? fresh.variant,
      radius: existing.radius ?? fresh.radius,
      w: parent ? layoutChildWidth(parent) : existing.w,
    }];
  });
  return fitLayouts([
    ...legacySyncedElements.filter((element) => !element.parentId || !element.parentId.startsWith('free_from_')),
    ...blockElements.filter((element) => visibleFreeIds.has(element.id)),
  ]);
}

export function findContainingLayout(elements: FreeDesignElement[], x: number, y: number) {
  return [...elements]
    .filter((element) => !element.parentId && element.kind === 'layout' && x >= element.x && x <= element.x + element.w && y >= element.y && y <= element.y + element.h)
    .sort((a, b) => b.z - a.z)[0];
}

export function fitLayoutToChildren(layout: FreeDesignElement, children: FreeDesignElement[]) {
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

export function fitLayouts(elements: FreeDesignElement[]) {
  return elements.map((element) => element.kind === 'layout'
    ? fitLayoutToChildren(element, elements.filter((child) => child.parentId === element.id))
    : element);
}

export function freeElementStyle(element: FreeDesignElement, nested = false) {
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

export function freeCanvasHeight(elements: FreeDesignElement[], minHeight = 760) {
  const topLevelElements = elements.filter((element) => !element.parentId);
  return Math.max(minHeight, ...topLevelElements.map((element) => element.y + element.h + 96));
}

export function moveBlock(blocks: CampaignBlock[], id: string, direction: -1 | 1) {
  const index = blocks.findIndex((block) => block.id === id);
  const nextIndex = index + direction;
  if (index < 0 || nextIndex < 0 || nextIndex >= blocks.length) return blocks;
  const next = [...blocks];
  [next[index], next[nextIndex]] = [next[nextIndex], next[index]];
  return next;
}

export type LayoutAction =
  | { type: 'fit' }
  | { type: 'move-into-layout'; freeElementId: string; layoutId: string; blocks: CampaignBlock[]; width: DeviceWidth }
  | { type: 'move-to-root'; freeElementId: string; width: DeviceWidth };

export function campaignLayoutReducer(elements: FreeDesignElement[], action: LayoutAction) {
  if (action.type === 'fit') return fitLayouts(elements);
  if (action.type === 'move-into-layout') {
    const parent = elements.find((item) => item.id === action.layoutId && item.kind === 'layout');
    const existingChild = elements.find((item) => item.id === action.freeElementId && item.kind !== 'layout');
    const blockId = blockIdForFreeElementId(action.freeElementId);
    const sourceBlock = blockId ? action.blocks.find((block) => block.id === blockId) : undefined;
    if (!parent || (!existingChild && !sourceBlock)) return elements;
    const child = existingChild ?? blockToFreeElement(sourceBlock!, action.width, Math.max(0, ...elements.map((item) => item.z)) + 1);
    const sourceItems = existingChild ? elements : [...elements, child];
    return fitLayouts(sourceItems.map((item) => item.id === action.freeElementId ? {
      ...item,
      parentId: parent.id,
      x: 0,
      y: Math.max(0, item.parentId === parent.id ? item.y : 0),
      w: layoutChildWidth(parent),
    } : item));
  }
  const child = elements.find((item) => item.id === action.freeElementId && item.kind !== 'layout');
  const parent = child?.parentId ? elements.find((item) => item.id === child.parentId && item.kind === 'layout') : undefined;
  if (!child || !parent) return elements;
  const maxX = Math.max(0, action.width - child.w);
  return fitLayouts(elements.map((item) => item.id === action.freeElementId ? {
    ...item,
    parentId: undefined,
    x: Math.min(maxX, Math.max(0, parent.x + item.x)),
    y: Math.max(0, parent.y + parent.h + 18),
  } : item));
}
