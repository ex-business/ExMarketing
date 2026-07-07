import type {
  CampaignBlock,
  CampaignBlockKind,
  DeviceWidth,
  EditorTab,
  FlexAlign,
  FlexDirection,
  FlexJustify,
  FlexWrap,
  FreeDesignElement,
  LayoutKind,
} from './types';
import { flexPreset, normalizeLayoutKind } from './layout';
import { Button, Field, InspectorSection } from './ui';

function blockDescription(kind: CampaignBlockKind) {
  if (kind === 'ClaimButton') return '按钮文案、禁用条件、点击动作';
  if (kind === 'HeroBanner') return '活动主视觉、标题、副标题、时间';
  if (kind === 'RewardCard') return '奖励金额、领取条件、奖品图';
  if (kind === 'ProgressBlock') return '参与人数、目标人数、进度比例';
  return '内容、绑定字段和显示状态';
}

export function FreeElementInspector({
  element,
  tab,
  width,
  onChange,
  onLayerMove,
}: {
  element?: FreeDesignElement;
  tab: EditorTab;
  width: DeviceWidth;
  onChange: (patch: Partial<FreeDesignElement>) => void;
  onLayerMove: (direction: -1 | 1) => void;
}) {
  if (!element) return <div className="campaign-inspector-body"><p>选择一个自由元素后编辑属性。</p></div>;
  if (tab === 'binding') {
    return (
      <div className="campaign-inspector-body">
        <InspectorSection description="自由元素绑定同样使用 Activity View Model 白名单表达式。">
          <Field label="绑定字段"><input value={element.binding ?? ''} placeholder="{{ activity.title }}" onChange={(event) => onChange({ binding: event.target.value })} /></Field>
          <Field label="展示条件"><input value={element.visibleWhen ?? ''} placeholder="{{ user.loggedIn }}" onChange={(event) => onChange({ visibleWhen: event.target.value })} /></Field>
        </InspectorSection>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <InspectorSection description="自由设计动作在发布 schema 中保存，运行时按 workflow event 触发。">
          <Field label="点击动作"><select value={element.action || 'NONE'} onChange={(event) => onChange({ action: event.target.value === 'NONE' ? '' : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></Field>
          <Field label="Payload"><input value={element.payload ?? ''} placeholder="{{ activity.id, reward.code, user.id }}" onChange={(event) => onChange({ payload: event.target.value })} /></Field>
        </InspectorSection>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <InspectorSection>
        <Field label="节点名称"><input value={element.name ?? ''} placeholder="节点名称" onChange={(event) => onChange({ name: event.target.value })} /></Field>
        <Field label="元素文案"><textarea value={element.text.split('\\n').join('\n')} onChange={(event) => onChange({ text: event.target.value.split('\n').join('\\n') })} /></Field>
        <div className="campaign-free-grid">
          <Field label="X"><input type="number" value={element.x} onChange={(event) => onChange({ x: Math.max(0, Number(event.target.value)) })} /></Field>
          <Field label="Y"><input type="number" value={element.y} onChange={(event) => onChange({ y: Math.max(0, Number(event.target.value)) })} /></Field>
          <Field label="宽"><input type="number" value={element.w} onChange={(event) => onChange({ w: Math.max(0, Number(event.target.value)) })} /></Field>
          <Field label="高"><input type="number" value={element.h} onChange={(event) => onChange({ h: Math.max(0, Number(event.target.value)) })} /></Field>
        </div>
        <Field label="层级"><input type="number" value={element.z} onChange={(event) => onChange({ z: Number(event.target.value) })} /></Field>
        <div className="campaign-inspector-actions">
          <Button onClick={() => onLayerMove(1)}>上移一层</Button>
          <Button onClick={() => onLayerMove(-1)}>下移一层</Button>
        </div>
        <Field label="圆角"><input value={element.radius ?? '8'} onChange={(event) => onChange({ radius: event.target.value })} /></Field>
      </InspectorSection>
      {element.kind === 'layout' ? (
        <InspectorSection title="布局">
          <Field label="布局方式">
            <select value={normalizeLayoutKind(element.layout)} onChange={(event) => {
              const layout = event.target.value as LayoutKind;
              onChange({ layout, ...flexPreset(layout) });
            }}>
              <option value="horizontal-layout">水平布局</option>
              <option value="vertical-layout">垂直布局</option>
            </select>
          </Field>
          <div className="campaign-free-grid">
            <Field label="方向"><select value={element.flexDirection ?? 'row'} onChange={(event) => onChange({ flexDirection: event.target.value as FlexDirection })}><option value="row">横向</option><option value="column">纵向</option></select></Field>
            <Field label="换行"><select value={element.flexWrap ?? 'nowrap'} onChange={(event) => onChange({ flexWrap: event.target.value as FlexWrap })}><option value="nowrap">不换行</option><option value="wrap">换行</option></select></Field>
            <Field label="主轴"><select value={element.justifyContent ?? 'center'} onChange={(event) => onChange({ justifyContent: event.target.value as FlexJustify })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="space-between">两端</option><option value="space-around">环绕</option></select></Field>
            <Field label="交叉轴"><select value={element.alignItems ?? 'center'} onChange={(event) => onChange({ alignItems: event.target.value as FlexAlign })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="stretch">拉伸</option></select></Field>
            <Field label="间距"><input type="number" value={element.gap ?? 8} onChange={(event) => onChange({ gap: Math.max(0, Number(event.target.value)) })} /></Field>
          </div>
        </InspectorSection>
      ) : null}
      <Field label="样式"><select value={element.variant ?? 'primary'} onChange={(event) => onChange({ variant: event.target.value as FreeDesignElement['variant'] })}><option value="primary">主要样式</option><option value="secondary">次要样式</option></select></Field>
      <p>{width}px 自由画布 · 文案、位置、尺寸、层级、样式会进入发布 schema。</p>
    </div>
  );
}

export function InspectorContent({
  block,
  tab,
  deviceWidth,
  onBlockChange,
  onMove,
}: {
  block: CampaignBlock;
  tab: EditorTab;
  deviceWidth: DeviceWidth;
  onBlockChange: (patch: Partial<CampaignBlock>) => void;
  onMove: (direction: -1 | 1) => void;
}) {
  if (tab === 'binding') {
    return (
      <div className="campaign-inspector-body">
        <InspectorSection description="绑定表达式只支持 Activity View Model 白名单字段，不暴露 workflow 内部节点。">
          <Field label="禁用条件 / 绑定字段"><input value={block.binding ?? (block.kind === 'ClaimButton' ? '{{ !eligibility.canClaim }}' : '{{ activity.title }}')} onChange={(event) => onBlockChange({ binding: event.target.value, bound: true })} /></Field>
          <Field label="展示条件"><input value={block.visibleWhen ?? '{{ user.loggedIn }}'} onChange={(event) => onBlockChange({ visibleWhen: event.target.value })} /></Field>
        </InspectorSection>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <InspectorSection description="动作绑定到 ExMarketing workflow event，不允许任意脚本。">
          <Field label="点击动作"><select value={block.action || 'NONE'} onChange={(event) => onBlockChange({ action: event.target.value === 'NONE' ? '' : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></Field>
          <Field label="Payload"><input value={block.payload ?? '{{ activity.id, reward.code, user.id }}'} onChange={(event) => onBlockChange({ payload: event.target.value })} /></Field>
        </InspectorSection>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <InspectorSection>
        <Field label="节点名称"><input value={block.title} onChange={(event) => onBlockChange({ title: event.target.value })} /></Field>
        {block.kind === 'ClaimButton' ? (
          <Field label="按钮文案"><input value={block.text ?? '立即领取'} onChange={(event) => onBlockChange({ text: event.target.value })} /></Field>
        ) : null}
        <div className="campaign-button-preview">{block.kind === 'ClaimButton' ? block.text ?? '立即领取' : block.title}</div>
        <Field label="按钮样式"><select value={block.variant ?? 'primary'} onChange={(event) => onBlockChange({ variant: event.target.value as CampaignBlock['variant'] })}><option value="primary">主要按钮（蓝色）</option><option value="secondary">次要按钮</option></select></Field>
        <Field label="宽度"><select value={block.width ?? 'full'} onChange={(event) => onBlockChange({ width: event.target.value as CampaignBlock['width'] })}><option value="full">全宽</option><option value="content">内容宽度</option></select></Field>
        <Field label="圆角"><input value={block.radius ?? '8'} onChange={(event) => onBlockChange({ radius: event.target.value })} /></Field>
        <div className="campaign-inspector-actions">
          <Button onClick={() => onMove(-1)}>上移</Button>
          <Button onClick={() => onMove(1)}>下移</Button>
        </div>
      </InspectorSection>
      <p>{deviceWidth}px 画布 · {blockDescription(block.kind)}</p>
    </div>
  );
}
