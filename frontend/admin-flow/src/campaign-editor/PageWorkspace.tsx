import type { CampaignBlock, CampaignBlockKind, CampaignPageSchema, FreeDesignElement } from './types';
import { AddBetween } from './FreeCanvas';
import { ALLOWED_BLOCK_KINDS, blockIdForFreeElementId, freeCanvasHeight, freeElementStyle } from './layout';
import { renderFreeElementText } from './renderUtils';

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
  const elements = [...(page.freeElements ?? [])].sort((a, b) => a.z - b.z);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  const selectBlockFromFreeElement = (id: string) => {
    const blockId = blockIdForFreeElementId(id);
    if (blockId) onSelect(blockId);
  };
  const previewBlockIds = elements.map((element) => blockIdForFreeElementId(element.id)).filter(Boolean);
  const activeBlockId = previewBlockIds.includes(selectedId) ? selectedId : previewBlockIds[0] ?? selectedId;
  const canvasHeight = freeCanvasHeight(elements);
  return (
    <section className="campaign-free-workspace">
      <div
        className="campaign-free-canvas campaign-mobile-canvas campaign-module-preview-canvas"
        style={{ width: page.device.defaultWidth, height: canvasHeight }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          const kind = (event.dataTransfer.getData('application/x-campaign-block') || draggingBlockKind) as CampaignBlockKind;
          if (ALLOWED_BLOCK_KINDS.has(kind)) onDropBlock(kind);
          onDragDone();
        }}
      >
        {topLevelElements.map((element) => {
          const childElements = childrenByParent[element.id] ?? [];
          const blockId = blockIdForFreeElementId(element.id);
          return (
            <div
              key={element.id}
              className={`campaign-runtime-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${element.variant ?? 'primary'} ${activeBlockId === blockId ? 'selected' : ''}`}
              role="button"
              data-action={element.action ?? ''}
              style={freeElementStyle(element)}
              onClick={() => selectBlockFromFreeElement(element.id)}
            >
              {childElements.length ? childElements.map((child) => {
                const childBlockId = blockIdForFreeElementId(child.id);
                return (
                  <span
                    key={child.id}
                    className={`campaign-runtime-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${child.variant ?? 'primary'} ${activeBlockId === childBlockId ? 'selected' : ''}`}
                    style={freeElementStyle(child, true)}
                    onClick={(event) => {
                      event.stopPropagation();
                      selectBlockFromFreeElement(child.id);
                    }}
                  >
                    {renderFreeElementText(child.text)}
                  </span>
                );
              }) : renderFreeElementText(element.text)}
            </div>
          );
        })}
      </div>
    </section>
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
