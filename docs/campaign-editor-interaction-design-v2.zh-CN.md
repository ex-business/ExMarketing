# Campaign Editor 交互设计 v2

Date: 2026-07-08

本文档定义 ExMarketing Campaign Editor 下一版交互模型。目标不是继续堆按钮和表单，而是把编辑器收敛成业界成熟页面构建工具常见的稳定工作流：结构优先、容器优先、画布居中、属性面板统一、运行时渲染一致。

## 背景问题

当前编辑器已经具备页面树、组件库、自由拖拽、资源、设置、模板库、Inspector 和运行时预览，但使用时仍然“不顺手”。根因不是单个 UI 样式，而是交互模型混杂：

- 页面树和组件库放在同一个窄面板里，结构管理和添加组件互相挤压。
- 默认进入自由拖拽，用户需要同时理解 `x/y/w/h/z`、层级、容器和普通组件。
- 容器没有成为主要布局心智，导致用户仍在用绝对定位解决大多数排版问题。
- Inspector 只有 `内容 / 绑定 / 动作`，布局、外观、交互、数据绑定被混在一起。
- 页面设计、模块预览、运行时虽然已走同一 renderer 方向，但编辑态的入口和模式命名仍不够清晰。

## 业界参考

成熟编辑器大多遵循类似结构：

- Figma：左侧 Layers，中央 Canvas，右侧 Properties。选中对象后只在右侧编辑该对象属性。
- Webflow：Navigator 表达 DOM/元素层级，Style/Settings panel 管理选中元素，布局主要靠 section/container/flex/grid。
- Retool：组件树和组件设置分离，右侧 Inspector 是最常使用的配置区域。
- Framer：自由画布能力存在，但复用组件、布局容器和右侧属性配置仍是主要工作流。

ExMarketing 不是专业设计软件，也不是通用 Webflow。它更接近“营销活动页构建器”，因此应该采用：

```text
结构树 + 组件添加
-> 容器布局
-> 选中对象 Inspector
-> 统一预览 / 运行时 renderer
```

## 产品定位

Campaign Editor v2 定位为：

> 面向运营的移动端营销活动页构建器。

核心原则：

1. 结构优先：页面首先是树，不是散落在画布上的点。
2. 容器优先：多数排版通过水平布局、垂直布局、区块、间距、对齐完成。
3. 自由拖拽降级：自由定位是高级能力，用于头图叠层、特殊运营视觉，不作为默认编辑心智。
4. Inspector 统一：所有属性编辑都走右侧 Inspector，不在页面树里塞复杂控件。
5. 单一 renderer：页面设计、模块预览、运行时预览都使用同一个 Canvas Renderer，不允许出现“设计居中，预览不居中”。
6. Schema 先行：所有拖拽、树操作、快捷键都走 reducer 和 Editor Schema，不直接修改 React state。

## 顶层信息架构

主导航只保留四个一级入口：

```text
页面
资源
设置
模板库
```

说明：

- `页面`：设计和预览活动页，是默认入口。
- `资源`：统一管理数据字段、数据源、动作事件、样式 Token、图片素材。
- `设置`：页面路径、发布、版本、分享、权限。
- `模板库`：从标准活动模板创建页面。

不再把 `数据源 / 动作 / 样式 / 图片 / 页面库` 做成一级导航。它们属于 `资源` 或 `设置` 内部能力。

## 页面工作区布局

页面工作区固定为三栏：

```text
左侧：结构 / 添加
中间：画布
右侧：Inspector
```

顶部 Header：

```text
活动选择 / 设备尺寸 / 自动保存状态 / 预览 / 导出 Schema / 发布
```

底部状态栏：

```text
选中路径 / 画布尺寸 / schema 状态 / 校验状态
```

底部状态栏示例：

```text
Page > 主内容区 > 奖励列表 > 领取按钮    393px    已保存    0 个阻塞
```

## 左侧面板

左侧面板只做两件事：看结构、加组件。使用顶部文本 tab：

```text
结构 | 添加
```

### 结构 tab

结构 tab 显示页面树。

树模型：

```text
Page 夏日充值返利活动
  Section 首屏
    VerticalLayout 主内容
      HeroBanner 头图
      RewardCard 奖励卡
      ClaimButton 领取按钮
  Section 规则
    RuleText 活动规则
    FAQBlock 常见问题
```

行内容：

```text
折叠图标 / 组件名称 / 组件类型 / 状态徽标
```

行操作原则：

- 默认不展示一排显隐/复制/删除图标。
- hover 或右键菜单展示快捷操作。
- 常用操作放右键菜单和顶部小工具条：
  - 重命名
  - 复制
  - 删除
  - 显示 / 隐藏
  - 移入容器
  - 移出容器

这样可以避免树行文字被操作图标挤压。

结构 tab 必须支持：

- 搜索节点。
- 展开 / 收起。
- 拖动节点调整顺序。
- 拖动普通组件到容器内。
- 拖动容器子组件到容器外。
- 拖动时显示插入线和目标容器高亮。
- 选中树节点时同步画布选中。

### 添加 tab

添加 tab 是组件库，不混页面树。

组件分类：

```text
常用
布局
营销
内容
表单
高级
```

常用组件：

```text
标题
头图
奖励卡
领取按钮
倒计时
进度
规则
FAQ
图片
```

布局组件：

```text
区块 Section
垂直布局 VerticalLayout
水平布局 HorizontalLayout
双列布局 TwoColumn
卡片容器 CardContainer
```

添加方式：

- 点击组件：插入到当前选中容器末尾。
- 拖动组件到画布：插入到光标所在容器或插入线位置。
- 拖动组件到树节点：插入到对应容器内。
- 如果当前没有选中容器，插入到 Page 根节点末尾。

## 画布

画布默认是“搭建模式”，不是自由设计模式。

### 搭建模式

搭建模式是默认模式。它使用容器和文档流布局，适合运营日常编辑。

行为：

- 组件按树结构从上到下渲染。
- 容器决定方向、间距、对齐、换行。
- 拖动组件时显示插入线。
- 拖动到容器上时容器高亮。
- 组件选中显示单层蓝色边框。
- 不展示 resize 点，除非组件支持尺寸配置。
- 不直接暴露 `x/y`，只在高级面板显示。

### 自由设计模式

自由设计模式是高级能力。

入口：

```text
画布工具条 > 模式 > 自由定位
```

适用场景：

- 头图内部叠字。
- 运营视觉浮层。
- 特殊角标、贴纸、活动素材。

限制：

- 默认不打开。
- 自由元素仍必须属于某个容器或 Section。
- 自由定位保存到同一 Editor Schema。
- 预览和运行时仍使用同一 Canvas Renderer。

### 模块预览

`模块预览` 改名为 `预览模式`。

预览模式：

- 隐藏编辑辅助线、拖拽手柄、选中边框。
- 仍在编辑器内展示当前草稿。
- 不触发真实 workflow，只模拟动作或使用 mock response。

运行时：

- 完全只读。
- 读取发布 snapshot。
- 不读取当前草稿。

## 右侧 Inspector

Inspector 按属性心智分组，不按技术对象分组。

统一分组：

```text
内容
布局
外观
数据
交互
高级
```

### 内容

编辑组件的业务内容：

- 标题文案
- 按钮文案
- 奖励金额展示
- 图片资源
- FAQ 条目
- 活动规则

### 布局

编辑组件和容器布局：

- 宽度：自适应 / 全宽 / 固定值
- 高度：自动 / 固定值
- 方向：横向 / 纵向
- 主轴对齐
- 交叉轴对齐
- 间距 gap
- 内边距 padding
- 换行 wrap

只有自由设计模式或高级对象才显示：

- X
- Y
- Z 层级

### 外观

编辑视觉样式：

- 样式 Token
- 文字样式
- 背景
- 边框
- 圆角
- 阴影
- 按钮变体

外观字段优先使用主题 token，不鼓励直接输入 `#fff` 这类散落颜色。

### 数据

编辑资源绑定：

- 绑定字段
- 默认值
- 显示条件
- 禁用条件
- 数据源来源
- 字段使用位置

绑定字段只从 `资源 > 数据字段` 选择，第一版不开放任意表达式编辑。

### 交互

编辑动作：

- 点击动作
- 跳转目标
- Workflow event
- Payload 映射
- 成功后刷新数据
- 失败提示
- 防重复点击
- 埋点事件

动作必须来自白名单，不允许任意脚本。

### 高级

面向少数高级场景：

- 节点 ID
- schema 片段
- 自定义属性
- 可访问性标签
- 调试信息

高级默认折叠。

## Editor Schema

Schema 只保留一个页面结构。

```text
Page
Node
ContainerNode
ComponentNode
ResourceBinding
ActionBinding
StyleToken
```

### Page

```json
{
  "schemaVersion": 1,
  "id": "page_20250520_001",
  "activityId": "act_20250520_001",
  "name": "夏日充值返利活动",
  "path": "/campaign/summer-rebate",
  "device": {
    "defaultWidth": 393,
    "supportedWidths": [375, 393, 414]
  },
  "rootNodeIds": ["section_hero"],
  "nodes": [],
  "resourceBindings": [],
  "actionBindings": [],
  "styleTokens": []
}
```

### ContainerNode

```json
{
  "id": "section_hero",
  "type": "container",
  "name": "首屏",
  "layout": "vertical-layout",
  "children": ["hero", "reward", "claim"],
  "frame": { "x": 0, "y": 0, "width": 393, "height": 0, "z": 1 },
  "flexDirection": "column",
  "justifyContent": "flex-start",
  "alignItems": "stretch",
  "flexWrap": "nowrap",
  "gap": 12,
  "resourceBindings": [],
  "actionBindings": [],
  "styleTokens": []
}
```

### ComponentNode

```json
{
  "id": "claim",
  "type": "component",
  "component": "ClaimButton",
  "name": "领取按钮",
  "parentId": "section_hero",
  "visible": true,
  "frame": { "x": 0, "y": 0, "width": 345, "height": 52, "z": 3 },
  "content": { "text": "立即领取", "variant": "primary" },
  "resourceBindings": [
    { "id": "claim_disabled", "expression": "{{ !eligibility.canClaim }}" }
  ],
  "actionBindings": [
    { "id": "claim_action", "actionId": "CLAIM_REWARD", "trigger": "click" }
  ],
  "styleTokens": [
    { "id": "claim_radius", "token": "--campaign-radius-button", "value": "8px" }
  ]
}
```

## Editor Engine

所有编辑操作必须走 reducer。

```text
selectNode
moveNode
resizeNode
moveIntoContainer
moveOutOfContainer
deleteNode
duplicateNode
reorderNode
undo
redo
```

规则：

- 树操作、画布拖拽、快捷键、Inspector 修改都派发 action。
- reducer 返回新的 Page schema。
- React state 只承载 reducer 结果，不直接修改节点。
- undo / redo 以“用户意图”为单位，不以每个 pointer move 为单位。
- 拖拽中可以 transient preview，pointer up 后提交一个历史记录。

## Canvas Renderer

页面设计、预览模式、运行时必须共用一个 renderer。

```text
CanvasRenderer(mode="design")
CanvasRenderer(mode="preview")
CanvasRenderer(mode="runtime")
```

模式差异：

| 模式 | 数据源 | 编辑辅助 | 动作行为 |
| --- | --- | --- | --- |
| design | 当前草稿 | 选中框、插入线、容器高亮、拖拽手柄 | 不触发真实动作 |
| preview | 当前草稿 | 隐藏编辑辅助，只读 | mock / dry-run |
| runtime | 发布 snapshot | 完全隐藏 | 真实 workflow event |

验收标准：

- 设计态居中，预览态也必须居中。
- 设计态宽度，预览态和运行时必须一致。
- 容器内对齐线以容器为基准，不以页面为基准。
- 同一组件在三个模式下使用同一 runtime render 定义。

## Component System

组件必须统一注册。

第一批组件：

```text
Title
HeroBanner
RewardCard
Button
Image
HorizontalLayout
VerticalLayout
```

每个组件定义：

```text
默认尺寸
默认标题
可编辑属性
默认样式
可绑定资源
可绑定动作
runtime 渲染方式
```

示例：

```ts
type ComponentDefinition = {
  key: ComponentRegistryKey;
  label: string;
  defaultSize: (width: DeviceWidth) => { w: number; h: number };
  editableProps: Array<"content" | "layout" | "appearance" | "data" | "interaction">;
  resourceBindings: ResourceBindingDefinition[];
  actionBindings: ActionBindingDefinition[];
  runtimeRender: (element, children) => ReactNode;
};
```

## 资源工作区

资源工作区保持表格型管理台，不回到卡片式。

内部资源类型：

```text
数据字段
数据源
动作事件
样式 Token
图片素材
```

布局：

```text
左侧资源类型树
中间资源表格
右侧详情 / 新增 / 编辑面板
```

图片必须支持批量上传队列。

数据源必须支持：

- REST API
- GraphQL
- WebSocket
- Mock JSON
- 刷新策略
- 实时拉取预览
- 字段映射

## 设计系统

编辑器 UI 不再散落 class。

基础组件：

```text
Button
Field
Tabs
Panel
Tree
Toolbar
InspectorSection
DataTable
SegmentedControl
```

使用范围：

- 页面树使用 `Tree`。
- 资源树使用 `Tree`。
- 资源列表使用 `DataTable`。
- Inspector 分组使用 `InspectorSection`。
- 页面设计 / 预览模式使用 `SegmentedControl`。
- 顶部和资源页工具区使用 `Toolbar`。
- 所有操作按钮使用 `Button`。

样式规则：

- 所有颜色进入 `tokens.css`。
- 不在业务 CSS 内直接散落 `#fff`、`#245BFF` 等硬编码颜色。
- 字号、行高、间距、圆角、边框也逐步 token 化。
- 组件库、页面树、Inspector、资源页共享同一套按钮和表单样式。

## 推荐交互细节

### 选中态

- 画布选中只显示一层边框。
- 不显示蓝色圆点编号。
- 树选中只高亮整行。
- 树 hover 才显示快捷操作。

### 拖拽

- 拖动普通组件时显示插入线。
- 拖动到容器时容器高亮。
- 拖动容器本身时按普通组件处理，不改变尺寸。
- 子组件拖入容器后默认宽度填满容器内容区。
- 子组件拖出容器后恢复为根节点，并保留可见位置。

### 滚动条

- 容器内容未溢出时不显示滚动条。
- 只有实际内容超过容器尺寸时才显示滚动条。
- 运营侧默认不需要手动管理 overflow。

### 宽高

- 宽高最小值为 `0`。
- 普通搭建模式优先使用自动高度。
- 固定宽高属于高级设置。

## 路线图

### P0：交互模型收敛

- 页面左侧拆成 `结构 / 添加`。
- 页面树移除常驻三图标，只保留 hover 菜单。
- 默认进入搭建模式。
- 自由设计改为高级模式。
- Inspector 改为 `内容 / 布局 / 外观 / 数据 / 交互 / 高级`。

### P1：Schema 和 Engine 完整接管

- 所有 Inspector 修改也走 reducer。
- 消除 legacy `blocks/freeElements` 作为保存源。
- undo / redo 以用户意图为单位。
- 发布 snapshot 只读取 Editor Schema。

### P2：容器布局增强

- 增加 Section / CardContainer。
- 容器支持 padding、gap、align、wrap。
- 树拖拽支持插入位置预览。
- 画布拖拽支持容器内插入线。

### P3：资源绑定体验

- 字段绑定从输入框改为资源选择器。
- 动作绑定从 select 改为动作选择器。
- 样式绑定从输入值改为 token picker。
- 图片绑定从文本改为素材选择器。

## 不做

- 不做通用 HTML 编辑器。
- 不开放任意 JS。
- 不把 Puck 作为主编辑器方案。
- 不默认进入 Figma 式自由画布。
- 不在页面树里堆大量按钮。
- 不让资源、动作、样式成为一级导航。
