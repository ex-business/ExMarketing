import type { CampaignPageSchema } from './types';
import { campaignPageApi } from './campaignPageStore';
import { CanvasRenderer } from './CanvasRenderer';
import './styles/tokens.css';
import './styles/runtime.css';

export function CampaignRuntimePage() {
  const page = campaignPageApi.loadRuntimePage(typeof window === 'undefined' ? '/campaign/summer-rebate' : window.location.pathname);
  return (
    <main className="campaign-runtime-page">
      <CampaignRuntimeView page={page} />
    </main>
  );
}

export function CampaignRuntimeView({ page, embedded = false }: { page: CampaignPageSchema; embedded?: boolean }) {
  return <CanvasRenderer page={page} mode="runtime" embedded={embedded} />;
}
