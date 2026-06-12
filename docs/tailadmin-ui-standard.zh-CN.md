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
