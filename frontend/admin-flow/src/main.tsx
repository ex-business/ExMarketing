import { createRoot } from 'react-dom/client';
import { lazy, Suspense } from 'react';
import '@xyflow/react/dist/style.css';
import { applyDocumentTheme, initialExmTheme } from './theme';
import './styles.css';

const App = lazy(() => import('./App').then((module) => ({ default: module.App })));
const CampaignEditorPage = lazy(() => import('./CampaignEditorPage').then((module) => ({ default: module.CampaignEditorPage })));
const CampaignRuntimePage = lazy(() => import('./campaign-editor/RuntimeView').then((module) => ({ default: module.CampaignRuntimePage })));
const CheckinPreviewPage = lazy(() => import('./CheckinPreviewPage').then((module) => ({ default: module.CheckinPreviewPage })));
const SevenDayCheckinPage = lazy(() => import('./SevenDayCheckinPage').then((module) => ({ default: module.SevenDayCheckinPage })));

const root = document.getElementById('root');
if (!root) {
  throw new Error('Root element #root not found');
}

applyDocumentTheme(initialExmTheme());

const pathname = window.location.pathname;

const routeElement = pathname.startsWith('/admin-flow/campaign-editor') || new URLSearchParams(window.location.search).has('pageDesign')
    ? <CampaignEditorPage />
    : pathname.startsWith('/campaign/') || pathname.startsWith('/admin-flow/campaign/')
    ? <CampaignRuntimePage />
    : pathname.startsWith('/admin-flow/checkin-streak')
    ? <SevenDayCheckinPage />
    : pathname.startsWith('/admin-flow/checkin')
      ? <CheckinPreviewPage />
      : <App />;

createRoot(root).render(
  <Suspense fallback={null}>
    {routeElement}
  </Suspense>,
);
