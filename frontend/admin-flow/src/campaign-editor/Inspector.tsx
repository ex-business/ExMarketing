import type { Node as EditorNode } from './editorSchema';
import type { EditorNodePatch } from './editorEngine';
import type { DeviceWidth, EditorTab, FlexAlign, FlexDirection, FlexJustify, FlexWrap, LayoutKind, ResourceRecord } from './types';
import { flexPreset, normalizeLayoutKind } from './layout';
import { Button, Field, InspectorSection } from './ui';

function numericValue(value: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, parsed) : 0;
}

function expressionFor(record?: ResourceRecord) {
  return record ? `{{ ${record.identifier} }}` : '';
}

export function NodeInspector({
  node,
  tab,
  width,
  resources,
  onChange,
  onReorder,
}: {
  node?: EditorNode;
  tab: EditorTab;
  width: DeviceWidth;
  resources: ResourceRecord[];
  onChange: (patch: EditorNodePatch) => void;
  onReorder: (direction: -1 | 1) => void;
}) {
  if (!node) return <div className="campaign-inspector-body"><p>在页面结构或画布中选择一个节点。</p></div>;

  if (tab === 'data') {
    const bindableResources = resources.filter((record) => (record.tab === '数据' || record.tab === '图片') && record.status === '启用');
    const actions = resources.filter((record) => record.tab === '动作' && record.status === '启用');
    const currentResource = node.resourceBindings[0];
    const currentAction = node.actionBindings[0];
    const currentResourceRecord = bindableResources.find((record) => record.id === currentResource?.resourceId || expressionFor(record) === currentResource?.expression);
    const currentActionRecord = actions.find((record) => record.name === currentAction?.actionId || record.identifier === currentAction?.actionId);
    return (
      <div className="campaign-inspector-body">
        <InspectorSection title="数据绑定" description="从资源中心选择字段。表达式由编辑器生成，避免手工输入错误。">
          <Field label="内容来源">
            <select
              value={currentResourceRecord?.id ?? ''}
              onChange={(event) => {
                const record = bindableResources.find((item) => item.id === event.target.value);
                onChange({ resourceBindings: record ? [{ id: `${node.id}_resource`, resourceId: record.id, expression: expressionFor(record) }] : [] });
              }}
            >
              <option value="">不绑定</option>
              {bindableResources.map((record) => <option key={record.id} value={record.id}>{record.name} · {record.identifier}</option>)}
            </select>
          </Field>
          {currentResource ? <div className="campaign-binding-preview">{currentResource.expression}</div> : null}
          <Field label="显示条件">
            <select value={node.visibilityExpression ?? ''} onChange={(event) => onChange({ visibilityExpression: event.target.value })}>
              <option value="">始终显示</option>
              <option value="{{ user.loggedIn }}">用户已登录</option>
              <option value="{{ eligibility.canClaim }}">可领取奖励</option>
              <option value="{{ activity.active }}">活动进行中</option>
            </select>
          </Field>
        </InspectorSection>
        <InspectorSection title="动作" description="动作由资源中心统一管理，组件只保存引用。">
          <Field label="点击动作">
            <select
              value={currentActionRecord?.id ?? ''}
              onChange={(event) => {
                const record = actions.find((item) => item.id === event.target.value);
                onChange({ actionBindings: record ? [{ id: `${node.id}_action`, actionId: record.name, trigger: 'click' }] : [] });
              }}
            >
              <option value="">无动作</option>
              {actions.map((record) => <option key={record.id} value={record.id}>{record.name} · {record.identifier}</option>)}
            </select>
          </Field>
          {currentAction ? <div className="campaign-binding-preview">{currentAction.actionId}</div> : null}
        </InspectorSection>
      </div>
    );
  }

  if (tab === 'style') {
    return (
      <div className="campaign-inspector-body">
        <InspectorSection title="尺寸策略">
          <div className="campaign-free-grid">
            <Field label="宽度">
              <select value={node.widthMode} onChange={(event) => onChange({ widthMode: event.target.value as EditorNode['widthMode'] })}>
                <option value="fill">填满容器</option>
                <option value="hug">适应内容</option>
                <option value="fixed">固定宽度</option>
              </select>
            </Field>
            <Field label="高度">
              <select value={node.heightMode} onChange={(event) => onChange({ heightMode: event.target.value as EditorNode['heightMode'] })}>
                <option value="auto">适应内容</option>
                <option value="fixed">固定高度</option>
                <option value="scroll">固定并滚动</option>
              </select>
            </Field>
          </div>
          {node.widthMode === 'fixed' || node.heightMode !== 'auto' ? (
            <div className="campaign-free-grid">
              {node.widthMode === 'fixed' ? <Field label="宽"><input type="number" min="0" value={node.frame.width} onChange={(event) => onChange({ frame: { width: numericValue(event.target.value) } })} /></Field> : null}
              {node.heightMode !== 'auto' ? <Field label="高"><input type="number" min="0" value={node.frame.height} onChange={(event) => onChange({ frame: { height: numericValue(event.target.value) } })} /></Field> : null}
            </div>
          ) : null}
          <Field label="圆角"><input type="number" min="0" value={node.radius ?? '8'} onChange={(event) => onChange({ radius: event.target.value })} /></Field>
          {node.type === 'component' ? (
            <Field label="样式变体"><select value={node.content.variant ?? 'primary'} onChange={(event) => onChange({ content: { variant: event.target.value as 'primary' | 'secondary' } })}><option value="primary">主要</option><option value="secondary">次要</option></select></Field>
          ) : null}
        </InspectorSection>
        {node.type === 'container' ? (
          <InspectorSection title="容器布局">
            <Field label="布局方式">
              <select value={normalizeLayoutKind(node.layout)} onChange={(event) => {
                const layout = event.target.value as LayoutKind;
                onChange({ layout, ...flexPreset(layout) });
              }}>
                <option value="horizontal-layout">水平布局</option>
                <option value="vertical-layout">垂直布局</option>
              </select>
            </Field>
            <div className="campaign-free-grid">
              <Field label="方向"><select value={node.flexDirection} onChange={(event) => onChange({ flexDirection: event.target.value as FlexDirection })}><option value="row">横向</option><option value="column">纵向</option></select></Field>
              <Field label="换行"><select value={node.flexWrap} onChange={(event) => onChange({ flexWrap: event.target.value as FlexWrap })}><option value="nowrap">不换行</option><option value="wrap">换行</option></select></Field>
              <Field label="主轴"><select value={node.justifyContent} onChange={(event) => onChange({ justifyContent: event.target.value as FlexJustify })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="space-between">两端</option><option value="space-around">环绕</option></select></Field>
              <Field label="交叉轴"><select value={node.alignItems} onChange={(event) => onChange({ alignItems: event.target.value as FlexAlign })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="stretch">拉伸</option></select></Field>
              <Field label="间距"><input type="number" min="0" value={node.gap} onChange={(event) => onChange({ gap: numericValue(event.target.value) })} /></Field>
            </div>
          </InspectorSection>
        ) : null}
      </div>
    );
  }

  return (
    <div className="campaign-inspector-body">
      <InspectorSection>
        <Field label="节点名称"><input value={node.name} onChange={(event) => onChange({ name: event.target.value })} /></Field>
        <Field label="内容"><textarea value={node.content.text.split('\\n').join('\n')} onChange={(event) => onChange({ content: { text: event.target.value.split('\n').join('\\n') } })} /></Field>
        <Field label="定位方式">
          <select value={node.positioning} onChange={(event) => onChange({ positioning: event.target.value as EditorNode['positioning'] })}>
            <option value="flow">自动布局</option>
            <option value="absolute">自由定位</option>
          </select>
        </Field>
        {node.positioning === 'absolute' ? (
          <details className="campaign-inspector-advanced" open>
            <summary>位置与层级</summary>
            <div className="campaign-free-grid">
              <Field label="X"><input type="number" min="0" value={node.frame.x} onChange={(event) => onChange({ frame: { x: numericValue(event.target.value) } })} /></Field>
              <Field label="Y"><input type="number" min="0" value={node.frame.y} onChange={(event) => onChange({ frame: { y: numericValue(event.target.value) } })} /></Field>
            </div>
            <Field label="层级"><input type="number" value={node.frame.z} onChange={(event) => onChange({ frame: { z: Number(event.target.value) || 0 } })} /></Field>
            <div className="campaign-inspector-actions"><Button onClick={() => onReorder(1)}>上移</Button><Button onClick={() => onReorder(-1)}>下移</Button></div>
          </details>
        ) : null}
        <Field label="状态"><select value={node.visible ? 'visible' : 'hidden'} onChange={(event) => onChange({ visible: event.target.value === 'visible' })}><option value="visible">显示</option><option value="hidden">隐藏</option></select></Field>
      </InspectorSection>
      <p>{width}px 预览宽度 · 默认使用自动布局，需要覆盖时再启用自由定位。</p>
    </div>
  );
}
