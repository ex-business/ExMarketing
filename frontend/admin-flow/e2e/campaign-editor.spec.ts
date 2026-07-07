import { expect, test } from '@playwright/test';

test.describe('Campaign Editor page design', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/admin-flow/campaign-editor', { waitUntil: 'networkidle' });
    await page.evaluate(() => {
      localStorage.removeItem('exmarketing.campaignEditor.page.v1');
      localStorage.removeItem('exmarketing.campaignEditor.publishedPage.v1');
      localStorage.removeItem('exmarketing.campaignEditor.publishedPages.v1');
      localStorage.removeItem('exmarketing.campaignEditor.versions.v1');
      localStorage.removeItem('exmarketing.campaignEditor.pages.v1');
      localStorage.removeItem('exmarketing.campaignEditor.resources.v1');
    });
    await page.reload({ waitUntil: 'networkidle' });
  });

  test('switches all workspaces inside the unified editor shell', async ({ page }) => {
    const nav = page.getByLabel('Campaign editor navigation');
    for (const label of ['页面', '资源', '设置', '模板库']) {
      await nav.getByRole('button', { name: label, exact: true }).click();
      if (label === '页面') {
        await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
      } else if (label === '资源') {
        await expect(page.locator('.campaign-resource-workspace')).toBeVisible();
        await expect(page.locator('.campaign-resource-bar')).toContainText('资源');
      } else {
        await expect(page.getByRole('heading', { name: label }).first()).toBeVisible();
        await expect(page.locator('.campaign-secondary-workspace')).toBeVisible();
      }
      await expect(page.locator('.campaign-brand')).toContainText('ExMarketing');
    }
    await nav.getByRole('button', { name: '资源', exact: true }).click();
    await expect(page.locator('.campaign-resource-title span')).toHaveCount(0);
    await expect(page.locator('.campaign-resource-tabs')).toHaveCount(0);
    const resourceTree = page.getByTestId('resource-type-tree');
    for (const group of ['数据字段', '数据源', '动作事件', '样式 Token', '图片素材']) {
      await expect(resourceTree.getByRole('button', { name: new RegExp(`^${group}`) })).toBeVisible();
    }
    await expect(resourceTree.getByRole('button', { name: '活动信息' })).toBeVisible();
    await resourceTree.getByRole('button', { name: /^数据字段/ }).click();
    await expect(resourceTree.getByRole('button', { name: '活动信息' })).toHaveCount(0);
    await resourceTree.getByRole('button', { name: /^数据字段/ }).click();
    await expect(resourceTree.getByRole('button', { name: '活动信息' })).toBeVisible();
    await expect(page.locator('.campaign-resource-search input')).toBeVisible();
    await expect(page.getByTestId('resource-type-tree')).toBeVisible();
    await expect(page.getByTestId('resource-table')).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('资源详情');
    await expect(page.locator('.campaign-resource-grid')).toHaveCount(0);
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'activity.title' })).toBeVisible();
  });

  test('manages resources with table, add forms and image batch upload', async ({ page }) => {
    const nav = page.getByLabel('Campaign editor navigation');
    await nav.getByRole('button', { name: '资源', exact: true }).click();

    await page.locator('.campaign-resource-search input').fill('activity.title');
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'activity.title' })).toBeVisible();
    await expect(page.getByTestId('resource-table')).not.toContainText('reward.amount');
    await page.locator('.campaign-resource-search input').fill('');

    await page.getByRole('button', { name: '添加', exact: true }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: '新增数据字段' })).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('新增数据字段');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('字段名称');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '取消' }).click();

    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^数据源/ }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'Activity View Model API' })).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('接口地址');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('实时拉取预览');
    await expect(page.getByTestId('resource-detail-panel')).toContainText(/拉取成功|Mock 回退/);
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '编辑' }).click();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('编辑资源');
    await page.getByTestId('resource-detail-panel').getByLabel('名称 *').fill('Activity View Model API 已编辑');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '保存' }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'Activity View Model API 已编辑' })).toBeVisible();
    await nav.getByRole('button', { name: '设置', exact: true }).click();
    await nav.getByRole('button', { name: '资源', exact: true }).click();
    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^数据源/ }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'Activity View Model API 已编辑' })).toBeVisible();
    await page.reload({ waitUntil: 'networkidle' });
    await nav.getByRole('button', { name: '资源', exact: true }).click();
    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^数据源/ }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'Activity View Model API 已编辑' })).toBeVisible();
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '复制' }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: 'Activity View Model API 已编辑 副本' })).toBeVisible();
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '删除' }).click();
    await expect(page.getByTestId('resource-table')).not.toContainText('Activity View Model API 已编辑 副本');
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: '新增数据源' })).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('新增数据源');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('数据源类型');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('刷新策略');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('字段映射');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '取消' }).click();

    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^动作事件/ }).click();
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: '新增动作事件' })).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('Payload Schema');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('验证 Payload');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '取消' }).click();

    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^样式 Token/ }).click();
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await expect(page.getByTestId('resource-table').getByRole('row').filter({ hasText: '新增样式 Token' })).toBeVisible();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('样式预览');
    await page.getByTestId('resource-detail-panel').getByRole('button', { name: '取消' }).click();

    await page.getByTestId('resource-type-tree').getByRole('button', { name: /^图片素材/ }).click();
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await expect(page.getByTestId('resource-detail-panel')).toContainText('批量上传图片');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('上传队列');
    await expect(page.getByTestId('resource-detail-panel')).toContainText('全部保存');
    await expect(page.locator('.campaign-upload-table')).toContainText('尺寸提醒');
  });

  test('edits blocks, persists schema, publishes and previews', async ({ page }) => {
    await page.locator('.campaign-free-element.button').first().click();
    await page.locator('.campaign-inspector-body textarea').first().fill('马上领取');
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');

    await page.locator('.campaign-inspector nav').getByRole('button', { name: '动作' }).click();
    await page.locator('.campaign-inspector-body select').selectOption('TRACK_EVENT');

    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');

    await page.getByRole('button', { name: /发布/ }).click();
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '设置', exact: true }).click();
    await expect(page.locator('.campaign-secondary-detail')).toContainText('最近发布');

    await page.locator('.campaign-editor-header').getByRole('button', { name: '预览', exact: true }).click();
    await expect(page.getByRole('dialog', { name: '移动端预览' })).toBeVisible();
    await expect(page.locator('.campaign-phone-screen')).toContainText('马上领取');
    await page.getByRole('button', { name: '关闭', exact: true }).click();

    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '页面', exact: true }).click();
    await page.locator('.campaign-outline-row', { hasText: 'ClaimButton' }).click();
    await page.locator('.campaign-inspector-body textarea').first().fill('草稿按钮');
    await page.goto('/admin-flow/campaign/summer-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).toContainText('马上领取');
    await expect(page.locator('.campaign-runtime-canvas')).not.toContainText('草稿按钮');

    await page.goto('/admin-flow/campaign-editor', { waitUntil: 'networkidle' });
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '设置', exact: true }).click();
    await page.locator('.campaign-secondary-form label').filter({ hasText: '页面路径' }).locator('input').fill('/campaign/custom-rebate');
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '页面', exact: true }).click();
    await page.locator('.campaign-outline-row', { hasText: 'ClaimButton' }).click();
    await page.locator('.campaign-inspector-body textarea').first().fill('自定义路径按钮');
    await page.getByRole('button', { name: /发布/ }).click();
    await page.goto('/admin-flow/campaign/custom-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).toContainText('自定义路径按钮');

    await page.goto('/admin-flow/campaign-editor', { waitUntil: 'networkidle' });
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '设置', exact: true }).click();
    await page.locator('.campaign-version-list button').last().click();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');
    await expect(page.locator('.campaign-free-element.button')).not.toContainText('自定义路径按钮');
  });

  test('does not render a stale legacy publish snapshot for another runtime path', async ({ page }) => {
    await page.evaluate(() => {
      localStorage.setItem('exmarketing.campaignEditor.publishedPage.v1', JSON.stringify({
        id: 'legacy_page',
        activityId: 'legacy_activity',
        name: '旧发布页面',
        path: '/campaign/summer-rebate',
        designMode: 'free',
        device: { defaultWidth: 393, supportedWidths: [375, 393, 414] },
        theme: { primaryToken: '--theme-accent', cardRadius: 8, contentGap: 12, fontFamily: 'System UI' },
        blocks: [],
        freeElements: [{ id: 'legacy_button', kind: 'button', text: '旧发布按钮', x: 18, y: 20, w: 345, h: 52, z: 1 }],
        status: 'PUBLISHED',
        version: 'legacy',
        updatedAt: new Date().toISOString(),
      }));
    });

    await page.goto('/admin-flow/campaign/unpublished-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).toHaveCount(1);
    await expect(page.locator('.campaign-runtime-canvas')).not.toContainText('旧发布按钮');
    await expect(page.locator('.campaign-runtime-element')).toHaveCount(0);
  });

  test('keeps click preview width fixed and scrolls vertically only when content is taller', async ({ page }) => {
    await page.setViewportSize({ width: 1198, height: 969 });
    await page.reload({ waitUntil: 'networkidle' });
    await page.locator('.campaign-editor-header').getByRole('button', { name: '预览', exact: true }).click();
    await expect(page.getByRole('dialog', { name: '移动端预览' })).toBeVisible();
    const screenMetrics = await page.locator('.campaign-phone-screen').evaluate((screen) => ({
      clientHeight: screen.clientHeight,
      scrollHeight: screen.scrollHeight,
      clientWidth: screen.clientWidth,
      scrollWidth: screen.scrollWidth,
    }));
    expect(screenMetrics.scrollHeight).toBeGreaterThanOrEqual(screenMetrics.clientHeight);
    expect(screenMetrics.scrollWidth - screenMetrics.clientWidth).toBeLessThanOrEqual(1);
  });

  test('keeps the editor usable in a mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-editor-header')).toBeVisible();
    await expect(page.getByLabel('Campaign editor navigation')).toBeVisible();
    await expect(page.locator('.campaign-free-canvas')).toBeVisible();
    const editorMetrics = await page.locator('.campaign-editor').evaluate((editor) => ({
      clientWidth: editor.clientWidth,
      scrollWidth: editor.scrollWidth,
      scrollLeftBefore: editor.scrollLeft,
    }));
    expect(editorMetrics.clientWidth).toBeLessThanOrEqual(390);
    expect(editorMetrics.scrollWidth).toBeGreaterThan(editorMetrics.clientWidth);
    const scrolled = await page.locator('.campaign-editor').evaluate((editor) => {
      editor.scrollLeft = 360;
      return editor.scrollLeft;
    });
    expect(scrolled).toBeGreaterThan(editorMetrics.scrollLeftBefore);
    await page.getByLabel('Campaign editor navigation').getByRole('button', { name: '资源', exact: true }).click();
    await expect(page.getByTestId('resource-type-tree')).toBeVisible();
    await expect(page.getByTestId('resource-table')).toBeVisible();
  });

  test('keeps page authoring as two clear modes', async ({ page }) => {
    await expect(page.locator('.campaign-toolbar')).toContainText('预览');
    await expect(page.locator('.campaign-toolbar')).toContainText('自由定位');
    await expect(page.locator('.campaign-toolbar')).toContainText('页面设计');
    await expect(page.locator('.campaign-toolbar button').first()).toHaveText('页面设计');
    await expect(page.locator('.campaign-toolbar button').nth(1)).toHaveText('自由定位');
    await expect(page.locator('.campaign-toolbar button').nth(2)).toHaveText('预览');
    await expect(page.locator('.campaign-toolbar button').first()).toHaveClass(/active/);
    await expect(page.locator('.campaign-free-canvas')).toBeVisible();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await expect(page.getByRole('heading', { name: '添加组件' })).toBeVisible();
    await expect(page.locator('.campaign-toolbar')).not.toContainText('Puck');
    await expect(page.locator('.campaign-toolbar')).not.toContainText('标题');
    await expect(page.locator('.campaign-toolbar')).not.toContainText('头图');
    await expect(page.locator('.campaign-free-canvas')).toContainText('活动倒计时');
    await expect(page.locator('.campaign-free-canvas')).toContainText('参与进度');
    await expect(page.locator('.campaign-free-canvas')).toContainText('活动规则');
    await expect(page.locator('.campaign-free-canvas')).toContainText('FAQ');

    await page.locator('.campaign-block-palette').getByRole('button', { name: '垂直布局' }).click();
    await expect(page.locator('.campaign-free-element.layout.vertical-layout')).toBeVisible();
    await page.getByLabel('页面编辑模式').getByRole('button', { name: '预览' }).click();
    await page.getByRole('button', { name: '页面设计' }).click();
    await expect(page.locator('.campaign-free-element.layout.vertical-layout')).toBeVisible();
  });

  test('adds modules from the component library by click and drag', async ({ page }) => {
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '标题' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(9);
    await expect(page.locator('.campaign-outline-list')).toContainText('标题');
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '标题' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(10);
    await expect(page.locator('.campaign-outline-list')).toContainText('标题 2');

    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: 'FAQ' }).dragTo(page.locator('.campaign-free-canvas'));
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-outline-list')).toContainText('FAQ 2');
  });

  test('keeps inspector node name aligned with the page tree', async ({ page }) => {
    await page.locator('.campaign-free-element.button').first().click();
    await page.locator('.campaign-inspector-body label').filter({ hasText: '节点名称' }).locator('input').fill('主按钮节点');
    await expect(page.locator('.campaign-inspector header')).toContainText('主按钮节点');
    await expect(page.locator('.campaign-outline-row.active')).toContainText('主按钮节点');

    await page.locator('.campaign-inspector-body label').filter({ hasText: '元素文案' }).locator('textarea').fill('马上领取');
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');
    await expect(page.locator('.campaign-outline-row.active')).toContainText('主按钮节点');
    await expect(page.locator('.campaign-outline-row.active')).not.toContainText('马上领取');
  });

  test('falls back to a valid page tree name when clearing the node name', async ({ page }) => {
    await page.locator('.campaign-free-element.button').first().click();
    await page.locator('.campaign-inspector-body label').filter({ hasText: '节点名称' }).locator('input').fill('');
    await expect(page.locator('.campaign-outline-row.active')).toContainText('领取按钮');
    await expect(page.locator('.campaign-outline-row.active')).toContainText('ClaimButton');
  });

  test('searches the page tree', async ({ page }) => {
    await page.getByRole('textbox', { name: '搜索页面树' }).fill('领取');
    await expect(page.locator('.campaign-outline-row')).toHaveCount(1);
    await expect(page.locator('.campaign-outline-list')).toContainText('领取按钮');

    await page.getByRole('textbox', { name: '搜索页面树' }).fill('不存在');
    await expect(page.locator('.campaign-outline-row')).toHaveCount(0);
    await expect(page.locator('.campaign-tree-empty')).toBeVisible();
  });

  test('keeps page tree rows compact', async ({ page }) => {
    const rowHeight = await page.locator('.campaign-outline-row', { hasText: 'HeroBanner' }).first().evaluate((element) => element.getBoundingClientRect().height);
    expect(rowHeight).toBeLessThanOrEqual(26);
  });

  test('collapses and expands the page tree and component library', async ({ page }) => {
    await page.locator('.campaign-page-tree-root').click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(0);
    await page.locator('.campaign-page-tree-root').click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);

    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await expect(page.locator('.campaign-block-palette button')).toHaveCount(11);
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-block-palette')).toHaveCount(0);
  });

	  test('keeps add component actions read-only in preview mode', async ({ page }) => {
	    await page.getByLabel('页面编辑模式').getByRole('button', { name: '预览' }).click();
	    await expect(page.locator('.campaign-outline-row').first()).toHaveAttribute('draggable', 'false');
	    await expect(page.getByLabel('隐藏内容块').first()).toBeDisabled();
	    await expect(page.getByLabel('复制内容块').first()).toBeDisabled();
	    await expect(page.getByLabel('删除内容块').first()).toBeDisabled();
	    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
	    await expect(page.locator('.campaign-panel-hint')).toContainText('预览模式为只读');
	    await expect(page.locator('.campaign-block-palette').getByRole('button', { name: '标题' })).toBeDisabled();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '标题' }).click({ force: true });
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);
  });

  test('keeps module editing and page design on the same page content', async ({ page }) => {
    await page.locator('.campaign-editor-header select').selectOption('375');
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '水平布局' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await expect(page.locator('.campaign-free-element.layout.horizontal-layout')).toBeVisible();
    const pageDesignCanvasBox = await page.locator('.campaign-free-canvas').boundingBox();
    const pageDesignLayoutBox = await page.locator('.campaign-free-element.layout.horizontal-layout').boundingBox();
    expect(pageDesignCanvasBox).not.toBeNull();
    expect(pageDesignLayoutBox).not.toBeNull();
    await page.locator('.campaign-editor-header').getByRole('button', { name: '预览', exact: true }).click();
    await expect(page.getByRole('dialog', { name: '移动端预览' })).toBeVisible();
    const phoneFrameBox = await page.locator('.campaign-phone-frame').boundingBox();
    const phoneScreenBox = await page.locator('.campaign-phone-screen').boundingBox();
    const freeModePreviewCanvasBox = await page.locator('.campaign-phone-screen .campaign-runtime-canvas').boundingBox();
    const freeModePreviewLayoutBox = await page.locator('.campaign-phone-screen .campaign-runtime-element.layout.horizontal-layout').boundingBox();
    expect(phoneFrameBox).not.toBeNull();
    expect(phoneScreenBox).not.toBeNull();
    expect(Math.round(phoneScreenBox!.width)).toBe(375);
    expect(Math.abs((phoneScreenBox!.x + phoneScreenBox!.width / 2) - (phoneFrameBox!.x + phoneFrameBox!.width / 2))).toBeLessThanOrEqual(1);
    expect(freeModePreviewCanvasBox).not.toBeNull();
    expect(freeModePreviewLayoutBox).not.toBeNull();
    expect(Math.abs(Math.round(freeModePreviewLayoutBox!.x - freeModePreviewCanvasBox!.x) - Math.round(pageDesignLayoutBox!.x - pageDesignCanvasBox!.x))).toBeLessThanOrEqual(1);
    expect(Math.abs(Math.round(freeModePreviewLayoutBox!.y - freeModePreviewCanvasBox!.y) - Math.round(pageDesignLayoutBox!.y - pageDesignCanvasBox!.y))).toBeLessThanOrEqual(1);
    await page.getByRole('button', { name: '关闭', exact: true }).click();
    await page.getByLabel('页面编辑模式').getByRole('button', { name: '预览' }).click();
    const modulePreviewLayout = page.locator('.campaign-runtime-element.layout.horizontal-layout');
    await expect(modulePreviewLayout).toBeVisible();
    const modulePreviewWorkspaceBox = await page.locator('.campaign-canvas-workspace').boundingBox();
    const modulePreviewCanvasBox = await page.locator('.campaign-module-preview-canvas').boundingBox();
    const modulePreviewLayoutBox = await modulePreviewLayout.boundingBox();
    expect(modulePreviewWorkspaceBox).not.toBeNull();
    expect(modulePreviewCanvasBox).not.toBeNull();
    expect(modulePreviewLayoutBox).not.toBeNull();
    expect(Math.abs((modulePreviewCanvasBox!.x + modulePreviewCanvasBox!.width / 2) - (modulePreviewWorkspaceBox!.x + modulePreviewWorkspaceBox!.width / 2))).toBeLessThanOrEqual(1);
    expect(Math.round(modulePreviewLayoutBox!.x)).toBe(Math.round(pageDesignLayoutBox!.x));
    expect(Math.round(modulePreviewLayoutBox!.y)).toBe(Math.round(pageDesignLayoutBox!.y));
    await page.locator('.campaign-editor-header').getByRole('button', { name: '预览', exact: true }).click();
    await expect(page.getByRole('dialog', { name: '移动端预览' })).toBeVisible();
    const moduleModePreviewCanvasBox = await page.locator('.campaign-phone-screen .campaign-runtime-canvas').boundingBox();
    const moduleModePreviewLayoutBox = await page.locator('.campaign-phone-screen .campaign-runtime-element.layout.horizontal-layout').boundingBox();
    expect(moduleModePreviewCanvasBox).not.toBeNull();
    expect(moduleModePreviewLayoutBox).not.toBeNull();
    expect(Math.abs(Math.round(moduleModePreviewLayoutBox!.x - moduleModePreviewCanvasBox!.x) - Math.round(pageDesignLayoutBox!.x - pageDesignCanvasBox!.x))).toBeLessThanOrEqual(1);
    expect(Math.abs(Math.round(moduleModePreviewLayoutBox!.y - moduleModePreviewCanvasBox!.y) - Math.round(pageDesignLayoutBox!.y - pageDesignCanvasBox!.y))).toBeLessThanOrEqual(1);
    await page.getByRole('button', { name: '关闭', exact: true }).click();
    await page.getByRole('button', { name: '页面设计' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    await page.locator('.campaign-outline-row', { hasText: '领取按钮' }).click();
    await page.locator('.campaign-inspector-body textarea').first().fill('马上领取');
    await expect(page.locator('.campaign-free-element.button')).toContainText('马上领取');

    await expect(page.locator('.campaign-free-canvas')).toContainText('马上领取');
    await expect(page.locator('.campaign-outline-list')).toContainText('ClaimButton');
    await expect(page.locator('.campaign-free-element.layout.horizontal-layout')).toBeVisible();
  });

  test('deletes the selected page design element with the keyboard', async ({ page }) => {
    await expect(page.locator('.campaign-free-element')).toHaveCount(8);
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);

    await page.locator('.campaign-free-element.button').first().click();
    await expect(page.locator('.campaign-free-element.selected')).toContainText('立即领取');
    await page.keyboard.press('Delete');

    await expect(page.locator('.campaign-free-element')).toHaveCount(7);
    await expect(page.locator('.campaign-outline-row')).toHaveCount(7);
    await expect(page.locator('.campaign-free-canvas')).not.toContainText('立即领取');
    await expect(page.locator('.campaign-outline-list')).not.toContainText('领取按钮');
  });

  test('deletes the last page tree node', async ({ page }) => {
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);
    for (let remaining = 8; remaining > 0; remaining -= 1) {
      const firstRow = page.locator('.campaign-outline-row').first();
      await firstRow.hover();
      await firstRow.getByRole('button', { name: '删除内容块' }).click();
      await expect(page.locator('.campaign-outline-row')).toHaveCount(remaining - 1);
    }
    await expect(page.locator('.campaign-tree-empty')).toBeVisible();
    await expect(page.locator('.campaign-free-element')).toHaveCount(0);
  });

  test('adds layout boxes to the page design canvas', async ({ page }) => {
    await page.getByRole('button', { name: '自由定位' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '水平布局' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    const layout = page.locator('.campaign-free-element.layout.horizontal-layout');
    await expect(layout).toBeVisible();
    await expect(layout).toContainText('水平布局');
    await expect(layout).toHaveCSS('display', 'grid');
    await expect(page.locator('.campaign-outline-list')).toContainText('水平布局');
    await expect(page.locator('.campaign-outline-row')).toHaveCount(9);
    const layoutBeforeDrag = await layout.boundingBox();
    expect(layoutBeforeDrag).not.toBeNull();
    await page.mouse.move(layoutBeforeDrag!.x + 12, layoutBeforeDrag!.y + 12);
    await page.mouse.down();
    await page.mouse.move(layoutBeforeDrag!.x + 72, layoutBeforeDrag!.y + 48);
    await page.mouse.up();
    const layoutAfterDrag = await layout.boundingBox();
    expect(layoutAfterDrag).not.toBeNull();
    expect(Math.round(layoutAfterDrag!.x - layoutBeforeDrag!.x)).toBeGreaterThan(10);
    expect(Math.round(layoutAfterDrag!.y - layoutBeforeDrag!.y)).toBeGreaterThan(20);
    const canvasBoxAfterDrag = await page.locator('.campaign-free-canvas').boundingBox();
    expect(canvasBoxAfterDrag).not.toBeNull();
    expect(Math.round(layoutAfterDrag!.x + layoutAfterDrag!.width)).toBeLessThanOrEqual(Math.round(canvasBoxAfterDrag!.x + canvasBoxAfterDrag!.width) + 1);

    await page.locator('.campaign-inspector-body label').filter({ hasText: '布局方式' }).locator('select').selectOption('vertical-layout');
    const vertical = page.locator('.campaign-free-element.layout.vertical-layout');
    await expect(vertical).toBeVisible();
    await expect(vertical).toHaveCSS('display', 'grid');
    await expect(vertical).toHaveCSS('overflow-x', 'hidden');
    await expect(vertical).toHaveCSS('overflow-y', 'auto');
    await page.locator('.campaign-inspector-body textarea').fill('123');
    await expect(vertical).toContainText('123');
    const singleLineLayoutCanScroll = await vertical.evaluate((element) => element.scrollHeight > element.clientHeight);
    expect(singleLineLayoutCanScroll).toBe(false);
    await page.locator('.campaign-inspector-body textarea').fill('');
    await expect(vertical).not.toContainText('123');
    await page.locator('.campaign-inspector-body textarea').fill('123\n123\n1223\n21233\n23\n2');
    const emptyLayoutCanScroll = await vertical.evaluate((element) => element.scrollHeight > element.clientHeight);
    expect(emptyLayoutCanScroll).toBe(true);

    await page.locator('.campaign-inspector-body label').filter({ hasText: '主轴' }).locator('select').selectOption('center');
    await page.locator('.campaign-inspector-body label').filter({ hasText: '间距' }).locator('input').fill('18');
    await expect(vertical).toHaveCSS('justify-content', 'center');
    await expect(vertical).toHaveCSS('gap', '18px');
    const initialLayoutBox = await vertical.boundingBox();
    expect(initialLayoutBox).not.toBeNull();

    const existingTitle = page.locator('.campaign-free-canvas > .campaign-free-element.text').first();
    const titleBox = await existingTitle.boundingBox();
    const layoutBox = await vertical.boundingBox();
    expect(titleBox).not.toBeNull();
    expect(layoutBox).not.toBeNull();
    await page.mouse.move(titleBox!.x + 10, titleBox!.y + 10);
    await page.mouse.down();
    await page.mouse.move(layoutBox!.x + 80, layoutBox!.y + 64);
    await page.mouse.up();
    await expect(vertical.locator('.campaign-free-element.nested.text')).toContainText('夏日充值返利活动');
    await expect(vertical).toHaveCSS('display', 'flex');
    await expect(vertical).toHaveCSS('flex-direction', 'column');
    await expect(vertical).toHaveCSS('overflow-x', 'hidden');
    await expect(vertical).toHaveCSS('overflow-y', 'auto');
    const nestedExistingTitle = vertical.locator('.campaign-free-element.nested.text', { hasText: '夏日充值返利活动' });
    const nestedExistingTitleBox = await nestedExistingTitle.boundingBox();
    const verticalBoxAfterExistingDrop = await vertical.boundingBox();
    expect(nestedExistingTitleBox).not.toBeNull();
    expect(verticalBoxAfterExistingDrop).not.toBeNull();
    const verticalContentWidth = await vertical.evaluate((element) => {
      const style = getComputedStyle(element);
      return element.clientWidth - Number.parseFloat(style.paddingLeft) - Number.parseFloat(style.paddingRight);
    });
    expect(Math.round(nestedExistingTitleBox!.width)).toBe(Math.round(verticalContentWidth));
    await page.locator('.campaign-outline-row', { hasText: '水平布局' }).first().click();
    await page.locator('.campaign-inspector-body label').filter({ hasText: '宽' }).locator('input').fill('320');
    const resizedVerticalBox = await vertical.boundingBox();
    const resizedNestedTitleBox = await nestedExistingTitle.boundingBox();
    expect(resizedVerticalBox).not.toBeNull();
    expect(resizedNestedTitleBox).not.toBeNull();
    const resizedVerticalContentWidth = await vertical.evaluate((element) => {
      const style = getComputedStyle(element);
      return element.clientWidth - Number.parseFloat(style.paddingLeft) - Number.parseFloat(style.paddingRight);
    });
    expect(Math.round(resizedNestedTitleBox!.width)).toBe(Math.round(resizedVerticalContentWidth));

    await page.locator('.campaign-outline-row', { hasText: 'HeroBanner' }).dragTo(page.locator('.campaign-outline-row', { hasText: '水平布局' }).first());
    const treeNestedHero = page.locator('.campaign-outline-children .campaign-outline-row', { hasText: 'HeroBanner' });
    await expect(treeNestedHero).toBeVisible();
    const nestedHero = vertical.locator('.campaign-free-element.nested.card', { hasText: '夏日充值返利活动' });
    await expect(nestedHero).toBeVisible();
    const nestedHeroBox = await nestedHero.boundingBox();
    const verticalBoxAfterTreeDrop = await vertical.boundingBox();
    expect(nestedHeroBox).not.toBeNull();
    expect(verticalBoxAfterTreeDrop).not.toBeNull();
    const verticalContentWidthAfterTreeDrop = await vertical.evaluate((element) => {
      const style = getComputedStyle(element);
      return element.clientWidth - Number.parseFloat(style.paddingLeft) - Number.parseFloat(style.paddingRight);
    });
    expect(Math.round(nestedHeroBox!.width)).toBe(Math.round(verticalContentWidthAfterTreeDrop));

    await treeNestedHero.dragTo(page.locator('.campaign-page-tree-root'));
    await expect(page.locator('.campaign-outline-children .campaign-outline-row', { hasText: 'HeroBanner' })).toHaveCount(0);
    await expect(page.locator('.campaign-outline-list > div > .campaign-outline-row', { hasText: 'HeroBanner' })).toBeVisible();
    await expect(vertical.locator('.campaign-free-element.nested.card', { hasText: '夏日充值返利活动' })).toHaveCount(0);
    await expect(page.locator('.campaign-free-canvas > .campaign-free-element.card', { hasText: '夏日充值返利活动' })).toBeVisible();

    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    await page.locator('.campaign-block-palette').getByRole('button', { name: '标题' }).dragTo(vertical, {
      targetPosition: { x: 40, y: 40 },
    });
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '结构' }).click();
    const nestedTitle = vertical.locator('.campaign-free-element.nested.text', { hasText: '标题' });
    await expect(nestedTitle).toBeVisible();
    await expect(nestedTitle).toContainText('标题');
    await expect(nestedTitle).toHaveCSS('position', 'relative');
    const nestedTitleBox = await nestedTitle.boundingBox();
    const verticalBoxAfterPaletteDrop = await vertical.boundingBox();
    expect(nestedTitleBox).not.toBeNull();
    expect(verticalBoxAfterPaletteDrop).not.toBeNull();
    const verticalContentWidthAfterPaletteDrop = await vertical.evaluate((element) => {
      const style = getComputedStyle(element);
      return element.clientWidth - Number.parseFloat(style.paddingLeft) - Number.parseFloat(style.paddingRight);
    });
    expect(Math.round(nestedTitleBox!.width)).toBe(Math.round(verticalContentWidthAfterPaletteDrop));
    await page.locator('.campaign-outline-row', { hasText: '水平布局' }).first().click();
    await expect(vertical).toHaveClass(/selected/);
    await nestedTitle.click();
    await expect(nestedTitle).toHaveClass(/selected/);
    await expect(nestedTitle).toHaveCSS('border-top-width', '2px');
    const selectedNestedTitleBox = await nestedTitle.boundingBox();
    expect(selectedNestedTitleBox).not.toBeNull();
    await page.mouse.move(selectedNestedTitleBox!.x + 12, selectedNestedTitleBox!.y + 12);
    await page.mouse.down();
    await page.mouse.move(selectedNestedTitleBox!.x + 42, selectedNestedTitleBox!.y + 28);
    const nestedGuideBox = await page.locator('.campaign-guide.vertical').boundingBox();
    expect(nestedGuideBox).not.toBeNull();
    expect(nestedGuideBox!.x).toBeGreaterThanOrEqual(verticalBoxAfterPaletteDrop!.x);
    expect(nestedGuideBox!.x).toBeLessThanOrEqual(verticalBoxAfterPaletteDrop!.x + verticalBoxAfterPaletteDrop!.width);
    await page.mouse.up();
    await expect(page.locator('.campaign-outline-children .campaign-outline-row')).toHaveCount(2);
    await expect(page.locator('.campaign-outline-children')).toContainText('标题');
    const grownLayoutBox = await vertical.boundingBox();
    expect(grownLayoutBox).not.toBeNull();
    expect(Math.round(grownLayoutBox!.height - initialLayoutBox!.height)).toBeGreaterThan(20);
    await page.locator('.campaign-outline-row', { hasText: '水平布局' }).first().click();
    const canvasBoxAfterChildrenDrag = await page.locator('.campaign-free-canvas').boundingBox();
    expect(canvasBoxAfterChildrenDrag).not.toBeNull();
    expect(Math.round(grownLayoutBox!.x + grownLayoutBox!.width)).toBeLessThanOrEqual(Math.round(canvasBoxAfterChildrenDrag!.x + canvasBoxAfterChildrenDrag!.width) + 1);
    await page.locator('.campaign-inspector-body label').filter({ hasText: 'X' }).locator('input').fill('123');
    const layoutAfterManualX = await vertical.boundingBox();
    const canvasAfterManualX = await page.locator('.campaign-free-canvas').boundingBox();
    expect(layoutAfterManualX).not.toBeNull();
    expect(canvasAfterManualX).not.toBeNull();
    expect(Math.round(layoutAfterManualX!.x + layoutAfterManualX!.width)).toBeLessThanOrEqual(Math.round(canvasAfterManualX!.x + canvasAfterManualX!.width) + 1);
    await page.locator('.campaign-inspector-body label').filter({ hasText: '高' }).locator('input').fill('64');
    await expect(vertical).toHaveCSS('overflow-x', 'hidden');
    await expect(vertical).toHaveCSS('overflow-y', 'auto');

    const layoutTreeRow = page.locator('.campaign-outline-row', { hasText: '水平布局' }).first();
    await layoutTreeRow.hover();
    await layoutTreeRow.getByRole('button', { name: '删除内容块' }).click();
    await expect(page.locator('.campaign-free-element.layout')).toHaveCount(0);
  });

  test('supports free design canvas drag and persistence', async ({ page }) => {
    await page.getByRole('button', { name: '自由定位' }).click();
    await expect(page.locator('.campaign-free-canvas')).toBeVisible();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();

    const freeElements = page.locator('.campaign-free-element');
    await expect(freeElements).toHaveCount(8);
    await expect(page.locator('.campaign-outline-row')).toHaveCount(8);

    const target = page.locator('.campaign-free-element.text').first();
    await target.click();
    await expect(target).toHaveClass(/selected/);
    await expect(target).toHaveCSS('border-top-width', '2px');
    const selectedTitleBorderColor = await target.evaluate((element) => getComputedStyle(element).borderTopColor);
    expect(selectedTitleBorderColor).not.toBe('rgba(0, 0, 0, 0)');
    const before = await target.boundingBox();
    expect(before).not.toBeNull();
    await page.mouse.move(before!.x + 8, before!.y + 8);
    await page.mouse.down();
    await page.mouse.move(before!.x + 72, before!.y + 54);
    await expect(page.locator('.campaign-guide').first()).toBeVisible();
    await page.mouse.up();

    const after = await target.boundingBox();
    expect(after).not.toBeNull();
    expect(Math.round(after!.x - before!.x)).toBeGreaterThan(30);
    expect(Math.round(after!.y - before!.y)).toBeGreaterThan(20);

    const resizeHandle = page.locator('.campaign-free-resize');
    await expect(resizeHandle).toBeVisible();
    const selectedElement = page.locator('.campaign-free-element.selected');
    const sizeBefore = await selectedElement.boundingBox();
    expect(sizeBefore).not.toBeNull();
    const handleBox = await resizeHandle.boundingBox();
    expect(handleBox).not.toBeNull();
    await page.mouse.move(handleBox!.x + 4, handleBox!.y + 4);
    await page.mouse.down();
    await page.mouse.move(handleBox!.x + 56, handleBox!.y + 36);
    await page.mouse.up();
    const sizeAfter = await selectedElement.boundingBox();
    expect(sizeAfter).not.toBeNull();
    expect(Math.round(sizeAfter!.width - sizeBefore!.width)).toBeGreaterThan(20);
    expect(Math.round(sizeAfter!.height - sizeBefore!.height)).toBeGreaterThan(15);
    const storedBeforeReload = await page.evaluate(() => {
      const pageSchema = JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}');
      return pageSchema.freeElements?.find((element: { id: string }) => element.id === 'free_hero_title');
    });
    expect(storedBeforeReload).toMatchObject({ id: 'free_hero_title' });

    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-free-element')).toHaveCount(8);
    const targetAfterReload = page.locator('.campaign-free-element.text').first();
    const persisted = await targetAfterReload.boundingBox();
    expect(persisted).not.toBeNull();
    expect(Math.abs(Math.round(persisted!.x - after!.x))).toBeLessThanOrEqual(2);
    expect(Math.abs(Math.round(persisted!.y - after!.y))).toBeLessThanOrEqual(2);
    const storedAfterReload = await page.evaluate(() => {
      const pageSchema = JSON.parse(localStorage.getItem('exmarketing.campaignEditor.page.v1') ?? '{}');
      return pageSchema.freeElements?.find((element: { id: string }) => element.id === 'free_hero_title');
    });
    expect(storedAfterReload).toMatchObject({
      x: storedBeforeReload.x,
      y: storedBeforeReload.y,
      w: storedBeforeReload.w,
      h: storedBeforeReload.h,
    });

    await page.locator('.campaign-inspector-body textarea').fill('自由设计标题');
    await page.locator('.campaign-free-grid input').nth(2).fill('620');
    await page.locator('.campaign-free-grid input').nth(3).fill('64');
    const fixedCanvasWidth = await page.locator('.campaign-free-canvas').evaluate((element) => element.getBoundingClientRect().width);
    expect(Math.round(fixedCanvasWidth)).toBe(393);
    await page.locator('.campaign-inspector-body label').filter({ hasText: '层级' }).locator('input').fill('9');
    await page.locator('.campaign-inspector nav').getByRole('button', { name: '绑定' }).click();
    await page.locator('.campaign-inspector-body input').first().fill('{{ activity.title }}');
    await page.locator('.campaign-inspector nav').getByRole('button', { name: '动作' }).click();
    await page.locator('.campaign-inspector-body select').selectOption('TRACK_EVENT');

    await page.locator('.campaign-editor-header').getByRole('button', { name: '预览', exact: true }).click();
    await expect(page.getByRole('dialog', { name: '移动端预览' })).toBeVisible();
    await expect(page.locator('.campaign-runtime-canvas')).toContainText('自由设计标题');
    await page.getByRole('button', { name: '关闭', exact: true }).click();

    await page.getByRole('button', { name: /发布/ }).click();
    await page.goto('/admin-flow/campaign/summer-rebate', { waitUntil: 'networkidle' });
    await expect(page.locator('.campaign-runtime-canvas')).toContainText('自由设计标题');
    const runtimeTitle = page.locator('.campaign-runtime-element', { hasText: '自由设计标题' });
    await expect(runtimeTitle).toHaveAttribute('data-action', 'TRACK_EVENT');
    await expect(runtimeTitle).toHaveCSS('width', '620px');
    await expect(runtimeTitle).toHaveCSS('height', '64px');
    await expect(runtimeTitle).toHaveCSS('z-index', '9');
  });

  test('scrolls the page design canvas vertically', async ({ page }) => {
    await expect(page.locator('.campaign-free-canvas')).toBeVisible();
    const scrolled = await page.locator('.campaign-canvas-workspace').evaluate((element) => {
      element.scrollTop = 320;
      return element.scrollTop;
    });
    expect(scrolled).toBeGreaterThan(0);
  });

  test('auto-scrolls the page design canvas while dragging near the bottom edge', async ({ page }) => {
    await page.getByRole('button', { name: '自由定位' }).click();
    const target = page.locator('.campaign-free-element', { hasText: '立即领取' });
    await target.click();
    const yInput = page.locator('.campaign-inspector-body label').filter({ hasText: 'Y' }).locator('input');
    const beforeY = Number(await yInput.inputValue());
    const box = await target.boundingBox();
    const workspaceBox = await page.locator('.campaign-canvas-workspace').boundingBox();
    expect(box).not.toBeNull();
    expect(workspaceBox).not.toBeNull();

    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await page.mouse.down();
    for (let step = 0; step < 8; step += 1) {
      await page.mouse.move(box!.x + box!.width / 2, workspaceBox!.y + workspaceBox!.height - 12, { steps: 2 });
    }
    await page.mouse.up();

    const afterY = Number(await yInput.inputValue());
    const scrollTop = await page.locator('.campaign-canvas-workspace').evaluate((element) => element.scrollTop);
    expect(afterY).toBeGreaterThan(beforeY + 120);
    expect(scrollTop).toBeGreaterThan(0);
  });

  test('does not duplicate resolution controls inside page design canvas', async ({ page }) => {
    await expect(page.locator('.campaign-free-workspace .campaign-canvas-ruler')).toHaveCount(0);
    const canvasWidth = await page.locator('.campaign-free-canvas').evaluate((element) => element.getBoundingClientRect().width);
    expect(canvasWidth).toBeGreaterThanOrEqual(393);
  });

  test('drops a component into a specific page design canvas position', async ({ page }) => {
    await page.getByRole('button', { name: '自由定位' }).click();
    await page.locator('.campaign-outline .exm-tabs').getByRole('button', { name: '添加' }).click();
    const canvas = page.locator('.campaign-free-canvas');
    await page.locator('.campaign-block-palette').getByRole('button', { name: '标题' }).dragTo(canvas, {
      targetPosition: { x: 80, y: 520 },
    });
    const dropped = page.locator('.campaign-free-element', { hasText: '标题' });
    await expect(dropped).toBeVisible();
    const canvasBox = await canvas.boundingBox();
    const droppedBox = await dropped.boundingBox();
    expect(canvasBox).not.toBeNull();
    expect(droppedBox).not.toBeNull();
    expect(Math.round(droppedBox!.x - canvasBox!.x)).toBeGreaterThan(60);
    expect(Math.round(droppedBox!.y - canvasBox!.y)).toBeGreaterThan(500);
  });

  test('allows zero width and height for free design elements', async ({ page }) => {
    await page.getByRole('button', { name: '自由定位' }).click();
    const target = page.locator('.campaign-free-element.text').first();
    await target.click();
    await page.locator('.campaign-inspector-body label').filter({ hasText: '宽' }).locator('input').fill('0');
    await page.locator('.campaign-inspector-body label').filter({ hasText: '高' }).locator('input').fill('0');
    await expect(target).toHaveCSS('width', '0px');
    await expect(target).toHaveCSS('height', '0px');
  });

  test('creates from template and returns to page design', async ({ page }) => {
    const nav = page.getByLabel('Campaign editor navigation');
    await nav.getByRole('button', { name: '模板库', exact: true }).click();
    await page.getByRole('button', { name: /夏日充值返利/ }).first().click();
    await page.getByRole('button', { name: '从模板创建' }).click();
    await expect(page.getByRole('heading', { name: '页面结构' })).toBeVisible();
    await expect(page.locator('.campaign-select')).toContainText('页面草稿');
  });
});
