# TailAdmin UI 标准

本项目后台界面以后统一参考 TailAdmin React demo：

- 标准参考：`https://react-demo.tailadmin.com/`
- 组件参考：`https://tailadmin.com/react-components`
- 开源参考：`https://github.com/TailAdmin/free-react-tailwind-admin-dashboard`

## 执行规则

1. 有对应 TailAdmin 组件时，优先按 TailAdmin 的组件结构、尺寸、状态和交互实现。
2. 没有对应组件时，不新增另一套视觉语言，按 TailAdmin 的字体、颜色、圆角、阴影、间距和表单规范扩展。
3. 新增后台组件默认使用 `Outfit` 字体、TailAdmin brand blue、gray palette、8/12/16px radius、44px 表单控件高度。
4. 表格、筛选、弹窗、抽屉、按钮、输入框、select、tabs、badge、empty state、pagination 都必须优先走本地 TailAdmin primitive。
5. 工作流 canvas 和节点图形属于专业编辑器区域，可以保留领域化样式，但外层面板、弹窗、表单和操作区仍按 TailAdmin 标准。

## ExMarketing 主题体系

ExMarketing 后台统一使用基于 Azure Symphony 的蓝色主题，并支持白天和黑夜两种模式。所有新增页面必须优先使用 CSS token，不允许在业务组件里直接写死主色、背景色、边框色和文本色。

基础色板：

```text
Azure 300  #a3c3ff
Azure 400  #7aa1ff
Azure 500  #5c85ff
Azure 600  #3f6bff
Azure 700  #2a53e0
Azure 800  #1b3fb3
Azure 900  #0f2c82
```

### Light Mode

白天模式用于默认后台和运营编辑场景，整体保持白底、浅灰分割、蓝色强调。

```text
page            #f6f8ff
canvas          #f3f7ff
surface         #ffffff
surface raised  #f8faff
surface hover   #eff5ff
border          #dfe8fb
border strong   #b9cafa
text strong     #0b1736
text body       #1d2a4d
text muted      #627092
text soft       #9aa7c2
primary         #1746ff
primary hover   #1b3fb3
primary soft    rgba(163, 195, 255, 0.24)
primary border  rgba(63, 107, 255, 0.34)
```

### Dark Mode

黑夜模式不是纯黑主题，而是深蓝后台主题。目标是减少长时间编辑疲劳，同时保留 ExMarketing 的蓝色品牌识别。

```text
page            #081229
canvas          #0d1b3f
surface         #101d42
surface raised  #142451
surface hover   #1a2e63
border          #273d73
border strong   #3f5fad
text strong     #f5f8ff
text body       #dce7ff
text muted      #a4b7e8
text soft       #7287bd
primary         #7aa1ff
primary hover   #a3c3ff
primary soft    rgba(122, 161, 255, 0.16)
primary border  rgba(122, 161, 255, 0.44)
```

### CSS Token

前端统一从 `frontend/admin-flow/src/styles.css` 的主题变量读取颜色：

```text
--theme-page
--theme-canvas
--theme-surface
--theme-surface-raised
--theme-surface-hover
--theme-control
--theme-control-hover
--theme-border
--theme-border-strong
--theme-text
--theme-text-body
--theme-text-muted
--theme-text-soft
--theme-accent
--theme-accent-strong
--theme-accent-soft
--theme-accent-border
--theme-on-accent
```

业务组件推荐使用语义别名：

```text
--app-bg
--panel-bg
--panel-border
--text-strong
--text-main
--text-muted
--brand-blue
--brand-blue-soft
--btn-primary-bg
--btn-primary-hover-bg
--btn-active-bg
--btn-active-text
```

### 使用规则

1. 主按钮、发布按钮、选中态、链接和关键操作使用 `--theme-accent`。
2. hover / pressed 状态使用 `--theme-accent-strong` 或组件内对应 hover token。
3. 面板背景使用 `--theme-surface`，页面背景使用 `--theme-page`。
4. Canvas、编辑区域、流程画布背景使用 `--theme-canvas`。
5. 文本颜色按层级使用 `--theme-text`、`--theme-text-body`、`--theme-text-muted`、`--theme-text-soft`。
6. 边框统一使用 `--theme-border`，强调边框使用 `--theme-border-strong` 或 `--theme-accent-border`。
7. 新页面必须同时检查白天和黑夜模式，不能只在 light mode 下可读。
8. 活动素材、营销 banner、奖品图不跟随后台系统主题强制染色。

### 主题切换

当前前端通过 `data-exm-theme` 控制主题：

```text
<html data-exm-theme="light">
<html data-exm-theme="dark">
```

主题状态由 `frontend/admin-flow/src/theme.ts` 管理，优先读取 `localStorage`，否则跟随系统 `prefers-color-scheme`。

## 本地组件入口

当前 workspace home 已建立本地 primitive：

`frontend/admin-flow/src/components/workspace-home/TailAdminPrimitives.tsx`

新增同类后台 UI 时，优先扩展这里的 primitive，而不是在业务组件里直接写一套新的 button/input/select/modal 样式。

## Modal 标准

- Dialog 宽度：小表单约 `520px`，复杂表单约 `560px+`
- Dialog 圆角：`16px`
- 表单控件高度：`44px`
- 主按钮高度：`44px`
- 标题：20px / 30px，半粗
- 说明：14px / 20px，gray-500
- 错误提示：浅红背景 + 红色边框 + 14px 文本

## 文案标准

后台产品界面统一中文业务文案。避免混用 `Folder / Workspace / workflow / Code / Tenant`，默认使用：

- Workspace -> 工作区
- Folder -> 文件夹
- Workflow -> 流程
- Code -> 流程编码
- Tenant -> 租户
