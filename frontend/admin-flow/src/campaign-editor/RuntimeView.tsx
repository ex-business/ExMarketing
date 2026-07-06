import type { CampaignPageSchema, FreeDesignElement } from './types';
import { blocksToFreeElements, freeCanvasHeight, freeElementStyle } from './layout';
import { campaignPageApi } from './campaignPageStore';
import { renderFreeElementText } from './renderUtils';

export function CampaignRuntimePage() {
  const page = campaignPageApi.loadRuntimePage(typeof window === 'undefined' ? '/campaign/summer-rebate' : window.location.pathname);
  return (
    <main className="campaign-runtime-page">
      <CampaignRuntimeView page={page} />
    </main>
  );
}

export function CampaignRuntimeView({ page, embedded = false }: { page: CampaignPageSchema; embedded?: boolean }) {
  const runtimeElements = page.freeElements?.length ? page.freeElements : blocksToFreeElements(page.blocks, page.device.defaultWidth);
  const elements = [...runtimeElements].sort((a, b) => a.z - b.z);
  const topLevelElements = elements.filter((element) => !element.parentId);
  const childrenByParent = elements.reduce<Record<string, FreeDesignElement[]>>((groups, element) => {
    if (!element.parentId) return groups;
    return { ...groups, [element.parentId]: [...(groups[element.parentId] ?? []), element] };
  }, {});
  const canvasHeight = embedded ? freeCanvasHeight(elements, 0) : undefined;
  return (
    <section className={embedded ? 'campaign-runtime-canvas embedded-preview' : 'campaign-runtime-canvas'} style={{ width: page.device.defaultWidth, ...(canvasHeight ? { height: canvasHeight, minHeight: canvasHeight } : {}) }}>
      {topLevelElements.map((element) => {
        const childElements = childrenByParent[element.id] ?? [];
        return (
          <div
            key={element.id}
            className={`campaign-runtime-element ${element.kind} ${element.layout ?? ''} ${childElements.length ? 'has-children' : ''} ${element.w === 0 || element.h === 0 ? 'zero-size' : ''} ${element.variant ?? 'primary'}`}
            role="button"
            data-action={element.action ?? ''}
            style={freeElementStyle(element)}
          >
            {childElements.length ? childElements.map((child) => (
              <span key={child.id} className={`campaign-runtime-element nested ${child.kind} ${child.w === 0 || child.h === 0 ? 'zero-size' : ''} ${child.variant ?? 'primary'}`} style={freeElementStyle(child, true)}>
                {renderFreeElementText(child.text)}
              </span>
            )) : renderFreeElementText(element.text)}
          </div>
        );
      })}
    </section>
  );
}
