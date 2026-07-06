import { expect, test } from '@playwright/test';
import type { CampaignBlock, FreeDesignElement } from '../src/campaign-editor/types';
import { campaignLayoutReducer, fitLayoutToChildren, initialFreeElements, layoutChildWidth, syncFreeElementsWithBlocks } from '../src/campaign-editor/layout';

const blocks: CampaignBlock[] = [
  { id: 'title', kind: 'Title', title: '活动标题', visible: true, bound: true },
  { id: 'claim', kind: 'ClaimButton', title: '领取按钮', visible: true, bound: true, text: '立即领取' },
];

const layout: FreeDesignElement = {
  id: 'layout_root',
  kind: 'layout',
  text: '布局盒子',
  x: 20,
  y: 80,
  w: 260,
  h: 80,
  z: 1,
  layout: 'vertical-layout',
  flexDirection: 'column',
  justifyContent: 'flex-start',
  alignItems: 'stretch',
  flexWrap: 'nowrap',
  gap: 8,
};

test.describe('campaign layout reducer', () => {
  test('moves an existing root element into a layout and fills the layout width', () => {
    const title: FreeDesignElement = { id: 'free_hero_title', kind: 'text', text: '标题', x: 24, y: 28, w: 120, h: 40, z: 2 };

    const next = campaignLayoutReducer([layout, title], {
      type: 'move-into-layout',
      freeElementId: title.id,
      layoutId: layout.id,
      blocks,
      width: 393,
    });

    const moved = next.find((element) => element.id === title.id);
    expect(moved).toMatchObject({
      parentId: layout.id,
      x: 0,
      w: layoutChildWidth(layout),
    });
  });

  test('creates a block-derived child when dropping a tree block into a layout', () => {
    const next = campaignLayoutReducer([layout], {
      type: 'move-into-layout',
      freeElementId: 'free_from_claim',
      layoutId: layout.id,
      blocks,
      width: 393,
    });

    const child = next.find((element) => element.id === 'free_from_claim');
    expect(child).toMatchObject({
      kind: 'button',
      parentId: layout.id,
      text: '立即领取',
      w: layoutChildWidth(layout),
    });
  });

  test('restores a default block element after hide and show', () => {
    const visibleBlocks: CampaignBlock[] = [
      { id: 'title', kind: 'Title', title: '活动标题', visible: true, bound: true },
      { id: 'reward', kind: 'RewardCard', title: '奖励卡片', visible: true, bound: true },
      { id: 'claim', kind: 'ClaimButton', title: '领取按钮', visible: true, bound: true, text: '立即领取' },
    ];
    const hiddenTitleBlocks = visibleBlocks.map((block) => block.id === 'title' ? { ...block, visible: false } : block);

    const afterHide = syncFreeElementsWithBlocks(hiddenTitleBlocks, 393, initialFreeElements);
    expect(afterHide.some((element) => element.text.includes('夏日充值返利活动'))).toBe(false);

    const afterShow = syncFreeElementsWithBlocks(visibleBlocks, 393, afterHide);
    expect(afterShow.some((element) => element.text.includes('夏日充值返利活动'))).toBe(true);
  });

  test('moves a child back to root and clamps it inside the canvas width', () => {
    const child: FreeDesignElement = {
      id: 'free_hero_title',
      kind: 'text',
      text: '标题',
      parentId: layout.id,
      x: 0,
      y: 0,
      w: layoutChildWidth(layout),
      h: 40,
      z: 2,
    };

    const next = campaignLayoutReducer([layout, child], {
      type: 'move-to-root',
      freeElementId: child.id,
      width: 280,
    });

    const moved = next.find((element) => element.id === child.id);
    expect(moved?.parentId).toBeUndefined();
    expect(moved?.x).toBeLessThanOrEqual(280 - child.w);
    expect(moved?.y).toBe(layout.y + layout.h + 18);
  });

  test('fits a layout to overflowing children without shrinking empty layouts', () => {
    const empty = fitLayoutToChildren(layout, []);
    expect(empty).toEqual(layout);

    const grown = fitLayoutToChildren(layout, [
      { id: 'child_a', kind: 'text', text: 'A', parentId: layout.id, x: 0, y: 0, w: 260, h: 42, z: 2 },
      { id: 'child_b', kind: 'text', text: 'B', parentId: layout.id, x: 0, y: 52, w: 260, h: 58, z: 3 },
    ]);
    expect(grown.h).toBeGreaterThan(layout.h);
    expect(grown.w).toBeGreaterThanOrEqual(layout.w);
  });
});
