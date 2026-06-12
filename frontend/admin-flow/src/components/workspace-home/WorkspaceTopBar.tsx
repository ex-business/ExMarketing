import { ChevronDown, Download, Plus, Settings } from 'lucide-react';
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
  onSearchChange: (value: string) => void;
  onSearchResultSelect?: (result: GlobalSearchResult) => void;
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
  onSearchChange,
  onSearchResultSelect,
  onNewWorkflow,
  onImport,
  onSettings,
}: WorkspaceTopBarProps) {
  return (
    <header className="exm-workspace-topbar">
      <div className="exm-topbar-left">
        <BrandMark />
        <nav className="exm-breadcrumb" aria-label="Breadcrumb">
          {isLoading ? (
            <>
              <span className="exm-skeleton exm-topbar-crumb-skeleton" />
              <i>/</i>
              <span className="exm-skeleton exm-topbar-crumb-skeleton short" />
            </>
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
        <div className="exm-topbar-search-skeleton">
          <span className="exm-skeleton" />
          <b className="exm-skeleton" />
          <i className="exm-skeleton" />
          <i className="exm-skeleton" />
          <i className="exm-skeleton" />
        </div>
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
