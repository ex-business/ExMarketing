import type { CampaignBlock, CampaignBlockKind, CampaignPageSchema } from './types';
import { AddBetween } from './FreeCanvas';
import { CanvasRenderer, pageElementsForRenderer } from './CanvasRenderer';
import { blockIdForFreeElementId } from './layout';

export function CampaignModulePreview({
  page,
  selectedId,
  onSelect,
  draggingBlockKind,
  onDropBlock,
  onDragDone,
}: {
  page: CampaignPageSchema;
  selectedId: string;
  onSelect: (id: string) => void;
  draggingBlockKind: CampaignBlockKind | null;
  onDropBlock: (kind: CampaignBlockKind) => void;
  onDragDone: () => void;
}) {
  const previewBlockIds = pageElementsForRenderer(page)
    .map((element) => blockIdForFreeElementId(element.id))
    .filter(Boolean);
  const activeBlockId = previewBlockIds.includes(selectedId) ? selectedId : previewBlockIds[0] ?? selectedId;
  return (
    <CanvasRenderer
      page={page}
      mode="preview"
      selectedNodeId={activeBlockId}
      draggingBlockKind={draggingBlockKind}
      onDropBlock={onDropBlock}
      onDragDone={onDragDone}
      onSelectNode={onSelect}
    />
  );
}

export function CampaignPreview({ blocks, selectedId, onSelect, onAddBlock }: { blocks: CampaignBlock[]; selectedId: string; onSelect: (id: string) => void; onAddBlock: () => void }) {
  return (
    <>
      {blocks.map((block, index) => (
        <div key={block.id}>
          {index > 0 ? <AddBetween onClick={onAddBlock} /> : null}
          <button className={`campaign-preview-block ${block.kind.toLowerCase()} ${block.id === selectedId ? 'selected' : ''}`} type="button" onClick={() => onSelect(block.id)}>
            {block.kind === 'HeroBanner' ? (
              <div className="campaign-hero-art"><strong>夏日充值<br />返利活动</strong><span>充值越多，返利越高</span><small>活动时间：2025.05.20 00:00:00 - 2025.06.20 23:59:59</small><i>素材</i></div>
            ) : block.kind === 'RewardCard' ? (
              <div className="campaign-reward-card"><span>奖励金额</span><b>￥188</b><em>可领取</em><p>单笔充值满 500 元可领</p><i>素材</i></div>
            ) : block.kind === 'Countdown' ? (
              <div className="campaign-countdown"><b>活动倒计时</b><p><span>06<small>天</small></span>:<span>18<small>时</small></span>:<span>32<small>分</small></span>:<span>45<small>秒</small></span></p></div>
            ) : block.kind === 'ProgressBlock' ? (
              <div className="campaign-progress"><b>参与进度</b><em>已参与 1,258 人</em><i><span /></i><p>目标 2,000 人 <strong>62%</strong></p></div>
            ) : block.kind === 'ClaimButton' ? (
              <span className="campaign-claim-button">{block.text ?? '立即领取'}</span>
            ) : block.kind === 'RuleText' ? (
              <div className="campaign-rules"><b>活动规则</b><p>1. 活动期间，单笔充值满 500 元可领取奖励；</p><p>2. 每个用户仅可领取一次奖励；</p><p>3. 奖励将于 24 小时内发放至账户。</p></div>
            ) : block.kind === 'Title' ? (
              <h3 className="campaign-title-block">夏日充值返利活动</h3>
            ) : (
              <div className="campaign-rules"><b>FAQ</b><p>奖励到账时间、参与资格和领取问题。</p></div>
            )}
          </button>
        </div>
      ))}
    </>
  );
}
