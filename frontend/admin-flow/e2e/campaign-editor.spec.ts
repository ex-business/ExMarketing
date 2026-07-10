import { expect, test } from '@playwright/test';

const storageKeys = [
  'exmarketing.campaignEditor.page.v1',
  'exmarketing.campaignEditor.publishedPage.v1',
  'exmarketing.campaignEditor.publishedPages.v1',
  'exmarketing.campaignEditor.versions.v1',
  'exmarketing.campaignEditor.pages.v1',
  'exmarketing.campaignEditor.resources.v1',
  'exmarketing.campaignEditor.resourceCategories.v1',
];

test.describe('Campaign Editor page design', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/admin-flow/campaign-editor', { waitUntil: 'networkidle' });
    await page.evaluate((keys) => keys.forEach((key) => localStorage.removeItem(key)), storageKeys);
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('switches workspaces in one editor shell', async ({ page }) => {
    const nav = page.getByLabel('Campaign editor navigation');
    for (const label of ['页面', '资源', '设置', '模板库']) {
      await nav.getByRole('button', { name: label, exact: true }).click();
      await expect(page.locator('.campaign-brand')).toContainText('ExMarketing');
    }
    await nav.getByRole('button', { name: '页面', exact: true }).click();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
  });

  test('persists resource categories, records and real-time source preview', async ({ page }) => {
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '资源', exact: true }).click();
    const tree = page.getByTestId('resource-type-tree');
    await tree.getByRole('button', { name: /^数据字段/ }).click();
    await expect(tree.getByRole('button', { name: '活动信息' })).toHaveCount(0);
    await tree.getByRole('button', { name: /^数据字段/ }).click();
    await tree.getByLabel('添加资源类型').click();
    await tree.getByLabel('资源类型名称').fill('会员信息');
    await tree.getByRole('button', { name: '保存' }).click();
    await tree.getByRole('button', { name: '会员信息' }).click();
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await page.getByTestId('resource-detail-panel').getByLabel('字段名称 *').fill('member.level');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '保存' }).click();
    await page.reload({ waitUntil: 'networkidle' });
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '资源', exact: true }).click();
    await tree.getByRole('button', { name: '会员信息' }).click();
    await expect(page.getByTestId('resource-table')).toContainText('member.level');
    await tree.getByRole('button', { name: /^数据源/ }).click();
    await expect(page.getByTestId('resource-detail-panel')).toContainText(/拉取成功|Mock 回退/);
  });

  test('edits, persists, publishes and renders the published snapshot', async ({ page }) => {
    const claim = page.locator('.campaign-free-element.button');
    await claim.click();
    await page.locator('.campaign-inspector-body textarea').fill('马上领取');
    await expect(claim).toContainText('马上领取');
    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');
    await page.getByRole('button', { name: /发布/ }).click();
    await page.locator('.campaign-free-element.button').click();
    await page.locator('.campaign-inspector-body textarea').fill('草稿按钮');
    await page.goto('/admin-flow/campaign/summer-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).toContainText('马上领取');
    await expect(page.locator('.campaign-runtime-canvas')).not.toContainText('草稿按钮');
  });

  test('uses only edit and preview modes', async ({ page }) => {
    const modes = page.getByLabel('页面编辑模式');
    await expect(modes.getByRole('button')).toHaveCount(2);
    await expect(modes).toContainText('编辑');
    await expect(modes).toContainText('预览');
    await expect(modes).not.toContainText('自由定位');
  });

  test('inserts a component after the selected sibling and reveals it', async ({ page }) => {
    const title = page.locator('[data-node-id="free_hero_title"]');
    await title.click();
    await page.getByLabel('页面左侧面板').getByRole('button', { name: '添加' }).click();
    await page.getByRole('button', { name: '倒计时', exact: true }).click();
    const ids = await page.locator('.campaign-free-canvas [data-node-id]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-node-id')));
    const titleIndex = ids.indexOf('free_hero_title');
    expect(titleIndex).toBeGreaterThanOrEqual(0);
    expect(ids[titleIndex + 1]).toContain('countdown');
    await expect(page.locator('.campaign-free-element.selected')).toContainText('活动倒计时');
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
  });

  test('reorders flow components by dragging directly on the canvas', async ({ page }) => {
    const source = page.locator('[data-node-id="free_claim"]');
    const target = page.locator('[data-node-id="free_hero_title"]');
    await source.dragTo(target, { targetPosition: { x: 40, y: 4 } });
    const ids = await page.locator('.campaign-free-canvas [data-node-id]').evaluateAll((nodes) => nodes.map((node) => node.getAttribute('data-node-id')));
    expect(ids.indexOf('free_claim')).toBeLessThan(ids.indexOf('free_hero_title'));
    await expect(page.locator('.campaign-free-element.selected')).toHaveAttribute('data-node-id', 'free_claim');
  });

  test('keeps node names independent from content and synchronized with the tree', async ({ page }) => {
    await page.locator('.campaign-free-element.button').click();
    await page.getByLabel('节点名称').fill('主按钮节点');
    await page.locator('.campaign-inspector-body textarea').fill('马上领取');
    await expect(page.locator('.campaign-outline-row.active')).toContainText('主按钮节点');
    await expect(page.locator('.campaign-outline-row.active')).not.toContainText('马上领取');
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');
  });

  test('searches and collapses the page tree', async ({ page }) => {
    await page.getByLabel('搜索页面树').fill('FAQ');
    await expect(page.locator('.campaign-outline-row')).toHaveCount(1);
    await page.getByLabel('搜索页面树').fill('');
    await page.locator('.campaign-page-tree-root').click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(0);
    await page.locator('.campaign-page-tree-root').click();
    await expect(page.locator('.campaign-outline-row')).not.toHaveCount(0);
  });

  test('keeps page tree rows compact and moves actions into a menu', async ({ page }) => {
    const row = page.locator('.campaign-outline-row', { hasText: 'HeroBanner' });
    expect(await row.evaluate((element) => element.getBoundingClientRect().height)).toBeLessThanOrEqual(36);
    await row.hover();
    await row.getByLabel(/操作/).click();
    await expect(row.getByRole('button', { name: '复制内容块' })).toBeVisible();
  });

  test('deletes nodes with keyboard and tree action menu', async ({ page }) => {
    const before = await page.locator('.campaign-outline-row').count();
    await page.locator('.campaign-free-element.button').click();
    await page.keyboard.press('Delete');
    await expect(page.locator('.campaign-outline-row')).toHaveCount(before - 1);
    const row = page.locator('.campaign-outline-row', { hasText: 'FAQBlock' });
    await row.hover();
    await row.getByLabel(/操作/).click();
    await row.getByRole('button', { name: '删除内容块' }).click();
    await expect(page.locator('.campaign-outline-row', { hasText: 'FAQBlock' })).toHaveCount(0);
  });

  test('adds a layout and nests a node through the tree', async ({ page }) => {
    await page.getByLabel('页面左侧面板').getByRole('button', { name: '添加' }).click();
    await page.getByRole('button', { name: '垂直布局', exact: true }).click();
    const layout = page.locator('.campaign-free-element.layout.vertical-layout');
    await expect(layout).toBeVisible();
    const layoutRow = page.locator('.campaign-outline-row', { hasText: 'VerticalLayout' });
    const heroRow = page.locator('.campaign-outline-row', { hasText: 'HeroBanner' });
    await heroRow.dragTo(layoutRow);
    await expect(layout.locator('[data-node-id="free_from_hero"]')).toBeVisible();
    await expect(layoutRow.locator('..').locator('.campaign-outline-children')).toContainText('HeroBanner');
  });

  test('keeps an empty container the same size in edit and preview', async ({ page }) => {
    await page.getByLabel('页面左侧面板').getByRole('button', { name: '添加' }).click();
    await page.getByRole('button', { name: '垂直布局', exact: true }).click();
    const editBox = await page.locator('.campaign-free-element.layout.vertical-layout').boundingBox();
    expect(editBox).not.toBeNull();
    await page.getByLabel('页面编辑模式').getByRole('button', { name: '预览' }).click();
    const previewBox = await page.locator('.campaign-runtime-element.layout.vertical-layout').boundingBox();
    expect(previewBox).not.toBeNull();
    expect(Math.round(previewBox!.width)).toBe(Math.round(editBox!.width));
    expect(Math.round(previewBox!.height)).toBe(Math.round(editBox!.height));
  });

  test('uses fill and auto sizing by default and scrolls only when requested', async ({ page }) => {
    await page.locator('.campaign-free-element.card').first().click();
    await page.getByRole('button', { name: '样式', exact: true }).click();
    await expect(page.getByLabel('宽度')).toHaveValue('fill');
    await expect(page.getByLabel('高度')).toHaveValue('auto');
    await page.getByLabel('高度').selectOption('scroll');
    await page.getByRole('spinbutton', { name: '高', exact: true }).fill('36');
    await expect(page.locator('.campaign-free-element.selected')).toHaveCSS('overflow-y', 'auto');
  });

  test('enables free positioning per node and persists drag changes', async ({ page }) => {
    const title = page.locator('[data-node-id="free_hero_title"]');
    await title.click();
    await page.getByLabel('定位方式').selectOption('absolute');
    const before = await title.boundingBox();
    expect(before).not.toBeNull();
    await page.mouse.move(before!.x + 10, before!.y + 10);
    await page.mouse.down();
    await page.mouse.move(before!.x + 54, before!.y + 70, { steps: 4 });
    await page.mouse.up();
    const after = await title.boundingBox();
    expect(after).not.toBeNull();
    expect(Math.round(after!.y - before!.y)).toBeGreaterThan(30);
    const expectedY = Number(await page.getByRole('spinbutton', { name: 'Y', exact: true }).inputValue());
    await expect.poll(async () => page.evaluate(() => JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}').editor.nodes.find((node: { id: string }) => node.id === 'free_hero_title').frame.y)).toBe(expectedY);
    const storedBeforeReload = await page.evaluate(() => JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}').editor.nodes.find((node: { id: string }) => node.id === 'free_hero_title'));
    expect(storedBeforeReload.positioning).toBe('absolute');
    expect(storedBeforeReload.frame.y).toBeGreaterThan(30);
    await page.reload({ waitUntil: 'networkidle' });
    const storedAfterReload = await page.evaluate(() => JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}').editor.nodes.find((node: { id: string }) => node.id === 'free_hero_title'));
    expect(storedAfterReload.frame.y).toBe(storedBeforeReload.frame.y);
  });

  test('renders edit, preview and runtime from the same node order', async ({ page }) => {
    const editGeometry = await page.locator('.campaign-free-canvas [data-node-id]').evaluateAll((nodes) => nodes.map((node) => {
      const rect = node.getBoundingClientRect();
      return { id: node.getAttribute('data-node-id'), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) };
    }));
    await page.getByLabel('页面编辑模式').getByRole('button', { name: '预览' }).click();
    const previewGeometry = await page.locator('.campaign-module-preview-canvas [data-node-id]').evaluateAll((nodes) => nodes.map((node) => {
      const rect = node.getBoundingClientRect();
      return { id: node.getAttribute('data-node-id'), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) };
    }));
    expect(previewGeometry).toEqual(editGeometry);
    await expect(page.locator('.campaign-module-preview-canvas .selected')).toHaveCount(0);
    const metrics = await page.locator('.campaign-module-preview-canvas').evaluate((canvas) => ({ clientWidth: canvas.clientWidth, scrollWidth: canvas.scrollWidth }));
    expect(metrics.scrollWidth - metrics.clientWidth).toBeLessThanOrEqual(1);
  });

  test('binds data and actions through resource pickers', async ({ page }) => {
    await page.locator('.campaign-free-element.button').click();
    await page.getByRole('button', { name: '数据与动作', exact: true }).click();
    await page.getByLabel('内容来源').selectOption({ label: 'claim.button.text · claim.button.text' });
    await page.getByLabel('点击动作').selectOption({ label: 'CompletePurchase · purchase.complete' });
    await expect(page.getByText('{{ claim.button.text }}', { exact: true })).toBeVisible();
    await expect.poll(async () => page.evaluate(() => {
      const stored = JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}');
      const button = stored.editor.nodes.find((node: { id: string }) => node.id === 'free_claim');
      return [button.resourceBindings[0]?.resourceId, button.actionBindings[0]?.actionId];
    })).toEqual([expect.any(String), 'CompletePurchase']);
    await page.getByRole('button', { name: /发布/ }).click();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
    await expect.poll(async () => page.evaluate(() => {
      const published = JSON.parse(localStorage.getItem('exmarketing.campaignEditor.publishedPage.v1') ?? '{}');
      return published.editor?.nodes?.find((node: { id: string }) => node.id === 'free_claim')?.actionBindings?.[0]?.actionId;
    })).toBe('CompletePurchase');
  });

  test('undoes and redoes editor operations from the header', async ({ page }) => {
    await page.locator('.campaign-free-element.button').click();
    await page.locator('.campaign-inspector-body textarea').fill('撤销测试');
    await page.locator('.campaign-editor-header').getByRole('button', { name: '撤销', exact: true }).click();
    await expect(page.locator('.campaign-free-element.button')).not.toContainText('撤销测试');
    await page.locator('.campaign-editor-header').getByRole('button', { name: '重做', exact: true }).click();
    await expect(page.locator('.campaign-free-element.button')).toContainText('撤销测试');
  });

  test('device selector is preview-only and does not mutate the page default', async ({ page }) => {
    const initial = await page.evaluate(() => JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}').device.defaultWidth);
    await page.locator('.campaign-editor-header select').selectOption('414');
    await expect(page.locator('.campaign-free-canvas')).toHaveCSS('width', '414px');
    const stored = await page.evaluate(() => JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}').device.defaultWidth);
    expect(stored).toBe(initial);
  });

  test('keeps the editor usable at a mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-editor-header')).toBeVisible();
    await expect(page.locator('.campaign-free-canvas')).toBeVisible();
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '资源', exact: true }).click();
    await expect(page.getByTestId('resource-type-tree')).toBeVisible();
    await expect(page.getByTestId('resource-table')).toBeVisible();
  });

  test('does not render a stale legacy snapshot for another path', async ({ page }) => {
    await page.evaluate(() => localStorage.setItem('exmarketing.campaignEditor.publishedPage.v1', JSON.stringify({ path: '/campaign/summer-rebate', freeElements: [{ id: 'legacy', kind: 'button', text: '旧按钮', x: 0, y: 0, w: 100, h: 40, z: 1 }] })));
    await page.goto('/admin-flow/campaign/unpublished-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).not.toContainText('旧按钮');
  });

  test('creates a page from a template and returns to editing', async ({ page }) => {
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '模板库', exact: true }).click();
    await page.getByRole('button', { name: /夏日充值返利/ }).first().click();
    await page.getByRole('button', { name: '从模板创建' }).click();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
    await expect(page.getByLabel('页面编辑模式').getByRole('button', { name: '编辑' })).toBeVisible();
  });
});
