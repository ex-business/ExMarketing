import type { ReactNode } from 'react';
import { useState } from 'react';
import { Menu, X } from 'lucide-react';
import { TaButton } from './TailAdminPrimitives';

type WorkspaceHomeLayoutProps = {
  sidebar: ReactNode;
  content: ReactNode;
  detail: ReactNode;
  sidebarCollapsed?: boolean;
};

export function WorkspaceHomeLayout({ sidebar, content, detail, sidebarCollapsed = false }: WorkspaceHomeLayoutProps) {
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [mobileDetailOpen, setMobileDetailOpen] = useState(false);

  return (
    <div className={[
      'exm-workspace-layout',
      mobileSidebarOpen ? 'sidebar-open' : '',
      mobileDetailOpen ? 'detail-open' : '',
      sidebarCollapsed ? 'sidebar-collapsed' : '',
    ].filter(Boolean).join(' ')}>
      <TaButton
        className="exm-mobile-sidebar-toggle"
        size="sm"
        aria-label={mobileSidebarOpen ? '关闭导航' : '打开导航'}
        aria-expanded={mobileSidebarOpen}
        onClick={() => setMobileSidebarOpen((value) => !value)}
      >
        {mobileSidebarOpen ? <X size={18} /> : <Menu size={18} />}
        <span>导航</span>
      </TaButton>
      <TaButton
        className="exm-mobile-detail-toggle"
        size="sm"
        aria-label={mobileDetailOpen ? '关闭详情' : '打开详情'}
        aria-expanded={mobileDetailOpen}
        onClick={() => setMobileDetailOpen((value) => !value)}
      >
        {mobileDetailOpen ? <X size={18} /> : null}
        <span>{mobileDetailOpen ? '关闭' : '详情'}</span>
      </TaButton>
      <button className="exm-mobile-sidebar-backdrop" type="button" aria-label="关闭导航" onClick={() => setMobileSidebarOpen(false)} />
      <button className="exm-mobile-detail-backdrop" type="button" aria-label="关闭详情" onClick={() => setMobileDetailOpen(false)} />
      <aside className="exm-workspace-sidebar" onClick={() => setMobileSidebarOpen(false)}>
        {sidebar}
      </aside>
      <main className="exm-workspace-main">{content}</main>
      <aside className="exm-workspace-detail">{detail}</aside>
    </div>
  );
}
