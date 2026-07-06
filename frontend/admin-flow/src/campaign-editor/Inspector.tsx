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
        <label><span>绑定字段</span><input value={element.binding ?? ''} placeholder="{{ activity.title }}" onChange={(event) => onChange({ binding: event.target.value })} /></label>
        <label><span>展示条件</span><input value={element.visibleWhen ?? ''} placeholder="{{ user.loggedIn }}" onChange={(event) => onChange({ visibleWhen: event.target.value })} /></label>
        <p>自由元素绑定同样使用 Activity View Model 白名单表达式。</p>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <label><span>点击动作</span><select value={element.action || 'NONE'} onChange={(event) => onChange({ action: event.target.value === 'NONE' ? undefined : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></label>
        <label><span>Payload</span><input value={element.payload ?? ''} placeholder="{{ activity.id, reward.code, user.id }}" onChange={(event) => onChange({ payload: event.target.value })} /></label>
        <p>自由设计动作在发布 schema 中保存，运行时按 workflow event 触发。</p>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <label><span>元素文案</span><textarea value={element.text.split('\\n').join('\n')} onChange={(event) => onChange({ text: event.target.value.split('\n').join('\\n') })} /></label>
      <div className="campaign-free-grid">
        <label><span>X</span><input type="number" value={element.x} onChange={(event) => onChange({ x: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>Y</span><input type="number" value={element.y} onChange={(event) => onChange({ y: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>宽</span><input type="number" value={element.w} onChange={(event) => onChange({ w: Math.max(0, Number(event.target.value)) })} /></label>
        <label><span>高</span><input type="number" value={element.h} onChange={(event) => onChange({ h: Math.max(0, Number(event.target.value)) })} /></label>
      </div>
      <label><span>层级</span><input type="number" value={element.z} onChange={(event) => onChange({ z: Number(event.target.value) })} /></label>
      <div className="campaign-inspector-actions">
        <button type="button" onClick={() => onLayerMove(1)}>上移一层</button>
        <button type="button" onClick={() => onLayerMove(-1)}>下移一层</button>
      </div>
      <label><span>圆角</span><input value={element.radius ?? '8'} onChange={(event) => onChange({ radius: event.target.value })} /></label>
      {element.kind === 'layout' ? (
        <>
          <label>
            <span>布局方式</span>
            <select value={normalizeLayoutKind(element.layout)} onChange={(event) => {
              const layout = event.target.value as LayoutKind;
              onChange({ layout, ...flexPreset(layout) });
            }}>
              <option value="horizontal-layout">水平布局</option>
              <option value="vertical-layout">垂直布局</option>
            </select>
          </label>
          <div className="campaign-free-grid">
            <label><span>方向</span><select value={element.flexDirection ?? 'row'} onChange={(event) => onChange({ flexDirection: event.target.value as FlexDirection })}><option value="row">横向</option><option value="column">纵向</option></select></label>
            <label><span>换行</span><select value={element.flexWrap ?? 'nowrap'} onChange={(event) => onChange({ flexWrap: event.target.value as FlexWrap })}><option value="nowrap">不换行</option><option value="wrap">换行</option></select></label>
            <label><span>主轴</span><select value={element.justifyContent ?? 'center'} onChange={(event) => onChange({ justifyContent: event.target.value as FlexJustify })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="space-between">两端</option><option value="space-around">环绕</option></select></label>
            <label><span>交叉轴</span><select value={element.alignItems ?? 'center'} onChange={(event) => onChange({ alignItems: event.target.value as FlexAlign })}><option value="flex-start">开始</option><option value="center">居中</option><option value="flex-end">结束</option><option value="stretch">拉伸</option></select></label>
            <label><span>间距</span><input type="number" value={element.gap ?? 8} onChange={(event) => onChange({ gap: Math.max(0, Number(event.target.value)) })} /></label>
          </div>
        </>
      ) : null}
      <label><span>样式</span><select value={element.variant ?? 'primary'} onChange={(event) => onChange({ variant: event.target.value as FreeDesignElement['variant'] })}><option value="primary">主要样式</option><option value="secondary">次要样式</option></select></label>
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
        <label><span>禁用条件 / 绑定字段</span><input value={block.binding ?? (block.kind === 'ClaimButton' ? '{{ !eligibility.canClaim }}' : '{{ activity.title }}')} onChange={(event) => onBlockChange({ binding: event.target.value, bound: true })} /></label>
        <label><span>展示条件</span><input value={block.visibleWhen ?? '{{ user.loggedIn }}'} onChange={(event) => onBlockChange({ visibleWhen: event.target.value })} /></label>
        <p>绑定表达式只支持 Activity View Model 白名单字段，不暴露 workflow 内部节点。</p>
      </div>
    );
  }
  if (tab === 'action') {
    return (
      <div className="campaign-inspector-body">
        <label><span>点击动作</span><select value={block.action || 'NONE'} onChange={(event) => onBlockChange({ action: event.target.value === 'NONE' ? undefined : event.target.value })}><option>CLAIM_REWARD</option><option>OPEN_LOGIN</option><option>NAVIGATE</option><option>TRACK_EVENT</option><option>NONE</option></select></label>
        <label><span>Payload</span><input value={block.payload ?? '{{ activity.id, reward.code, user.id }}'} onChange={(event) => onBlockChange({ payload: event.target.value })} /></label>
        <p>动作绑定到 ExMarketing workflow event，不允许任意脚本。</p>
      </div>
    );
  }
  return (
    <div className="campaign-inspector-body">
      <label><span>{block.kind === 'ClaimButton' ? '按钮文案' : '模块标题'}</span><input value={block.kind === 'ClaimButton' ? block.text ?? '立即领取' : block.title} onChange={(event) => onBlockChange(block.kind === 'ClaimButton' ? { text: event.target.value, title: event.target.value || '领取按钮' } : { title: event.target.value })} /></label>
      <div className="campaign-button-preview">{block.kind === 'ClaimButton' ? block.text ?? '立即领取' : block.title}</div>
      <label><span>按钮样式</span><select value={block.variant ?? 'primary'} onChange={(event) => onBlockChange({ variant: event.target.value as CampaignBlock['variant'] })}><option value="primary">主要按钮（蓝色）</option><option value="secondary">次要按钮</option></select></label>
      <label><span>宽度</span><select value={block.width ?? 'full'} onChange={(event) => onBlockChange({ width: event.target.value as CampaignBlock['width'] })}><option value="full">全宽</option><option value="content">内容宽度</option></select></label>
      <label><span>圆角</span><input value={block.radius ?? '8'} onChange={(event) => onBlockChange({ radius: event.target.value })} /></label>
      <div className="campaign-inspector-actions">
        <button type="button" onClick={() => onMove(-1)}>上移</button>
        <button type="button" onClick={() => onMove(1)}>下移</button>
      </div>
      <p>{deviceWidth}px 画布 · {blockDescription(block.kind)}</p>
    </div>
  );
}
