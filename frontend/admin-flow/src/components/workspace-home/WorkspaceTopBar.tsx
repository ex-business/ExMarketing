import { ChevronDown, ChevronLeft, ChevronRight, Download, Moon, Plus, Settings, Sun } from 'lucide-react';
import { BrandMark } from './BrandMark';
import { GlobalSearch } from './GlobalSearch';
import { TaButton } from './TailAdminPrimitives';
import type { GlobalSearchResult } from './GlobalSearch';

type WorkspaceTopBarProps = {
  workspaceName: string;
  folderName?: string;
  searchValue: string;
  workflowCount: number;
  folderCount: number;
  memberCount: number;
  searchResults?: GlobalSearchResult[];
  actorInitial?: string;
  isLoading?: boolean;
  sidebarCollapsed?: boolean;
  themeMode?: 'light' | 'dark';
  onSearchChange: (value: string) => void;
  onSearchResultSelect?: (result: GlobalSearchResult) => void;
  onSidebarCollapseToggle?: () => void;
  onThemeToggle?: () => void;
  onNewWorkflow: () => void;
  onImport: () => void;
  onSettings: () => void;
};

export function WorkspaceTopBar({
  workspaceName,
  folderName,
  searchValue,
  workflowCount,
  folderCount,
  memberCount,
  searchResults,
  actorInitial = 'D',
  isLoading = false,
  sidebarCollapsed = false,
  themeMode = 'light',
  onSearchChange,
  onSearchResultSelect,
  onSidebarCollapseToggle,
  onThemeToggle,
  onNewWorkflow,
  onImport,
  onSettings,
}: WorkspaceTopBarProps) {
  return (
    <header className="exm-workspace-topbar">
      <div className="exm-topbar-left">
        <BrandMark />
        {onSidebarCollapseToggle ? (
          <TaButton
            className="exm-sidebar-collapse-toggle"
            size="sm"
            aria-label={sidebarCollapsed ? '展开侧栏' : '收起侧栏'}
            aria-expanded={!sidebarCollapsed}
            title={sidebarCollapsed ? '展开侧栏' : '收起侧栏'}
            onClick={onSidebarCollapseToggle}
          >
            {sidebarCollapsed ? <ChevronRight size={17} strokeWidth={2.7} /> : <ChevronLeft size={17} strokeWidth={2.7} />}
          </TaButton>
        ) : null}
        <nav className="exm-breadcrumb" aria-label="Breadcrumb">
          {isLoading ? (
            null
          ) : (
            <>
              <span>Activity Studio</span>
              <i>/</i>
              <span>{workspaceName || 'hello'}</span>
              {folderName ? (
                <>
                  <i>/</i>
                  <b>{folderName}</b>
                </>
              ) : null}
            </>
          )}
        </nav>
      </div>
      {isLoading ? (
        null
      ) : (
        <GlobalSearch
          value={searchValue}
          workflowCount={workflowCount}
          folderCount={folderCount}
          memberCount={memberCount}
          results={searchResults}
          onChange={onSearchChange}
          onResultSelect={onSearchResultSelect}
        />
      )}
      <div className="exm-topbar-actions">
        {onThemeToggle ? (
          <TaButton
            className="exm-theme-toggle"
            aria-label={themeMode === 'dark' ? '切换到白天模式' : '切换到黑夜模式'}
            title={themeMode === 'dark' ? '白天模式' : '黑夜模式'}
            onClick={onThemeToggle}
          >
            {themeMode === 'dark' ? <Sun size={16} strokeWidth={2.2} /> : <Moon size={16} strokeWidth={2.2} />}
          </TaButton>
        ) : null}
        <TaButton className="exm-primary-action" tone="primary" onClick={onNewWorkflow}>
          <Plus size={16} strokeWidth={2.3} />
          <span>新建</span>
          <ChevronDown size={14} strokeWidth={2.3} />
        </TaButton>
        <TaButton onClick={onImport}>
          <Download size={15} strokeWidth={2.2} />
          <span>导入</span>
        </TaButton>
        <TaButton onClick={onSettings}>
          <Settings size={15} strokeWidth={2.2} />
          <span>设置</span>
        </TaButton>
        <TaButton className="exm-user-menu" tone="ghost" aria-label="用户菜单">
          <span>{actorInitial.slice(0, 1).toUpperCase()}</span>
          <ChevronDown size={14} strokeWidth={2.2} />
        </TaButton>
      </div>
    </header>
  );
}
