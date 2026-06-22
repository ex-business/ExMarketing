import { createRoot } from 'react-dom/client';
import '@xyflow/react/dist/style.css';
import { App } from './App';
import { CheckinPreviewPage } from './CheckinPreviewPage';
import { applyDocumentTheme, initialExmTheme } from './theme';
import './styles.css';

const root = document.getElementById('root');
if (!root) {
  throw new Error('Root element #root not found');
}

applyDocumentTheme(initialExmTheme());

createRoot(root).render(window.location.pathname.startsWith('/admin-flow/checkin') ? <CheckinPreviewPage /> : <App />);
