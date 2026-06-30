import { createRoot } from 'react-dom/client';
import '@xyflow/react/dist/style.css';
import { App } from './App';
import { CampaignEditorPage, CampaignRuntimePage } from './CampaignEditorPage';
import { CheckinPreviewPage } from './CheckinPreviewPage';
import { SevenDayCheckinPage } from './SevenDayCheckinPage';
import { applyDocumentTheme, initialExmTheme } from './theme';
import './styles.css';

const root = document.getElementById('root');
if (!root) {
  throw new Error('Root element #root not found');
}

applyDocumentTheme(initialExmTheme());

const pathname = window.location.pathname;

createRoot(root).render(
  pathname.startsWith('/admin-flow/campaign-editor') || new URLSearchParams(window.location.search).has('pageDesign')
    ? <CampaignEditorPage />
    : pathname.startsWith('/campaign/') || pathname.startsWith('/admin-flow/campaign/')
    ? <CampaignRuntimePage />
    : pathname.startsWith('/admin-flow/checkin-streak')
    ? <SevenDayCheckinPage />
    : pathname.startsWith('/admin-flow/checkin')
      ? <CheckinPreviewPage />
      : <App />,
);
