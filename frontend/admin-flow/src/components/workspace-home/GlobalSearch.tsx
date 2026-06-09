import { Search } from 'lucide-react';
import type { ReactNode } from 'react';

export type GlobalSearchResult = {
  id: string;
  type: 'workspace' | 'workflow' | 'folder' | 'member';
  title: string;
  subtitle: string;
  workspaceId?: string;
  folderId?: string;
  highlights?: {
    title?: string;
    subtitle?: string;
  };
  permission?: string;
  permissionReason?: string;
  permissionSources?: string[];
  score?: number;
};

type GlobalSearchProps = {
  value: string;
  workflowCount: number;
  folderCount: number;
  memberCount: number;
  results?: GlobalSearchResult[];
  onChange: (value: string) => void;
  onResultSelect?: (result: GlobalSearchResult) => void;
};

function renderHighlighted(value: string, html?: string) {
  if (!html) return value;
  const parts = html.split(/(<mark>|<\/mark>)/g).filter(Boolean);
  const nodes: ReactNode[] = [];
  let marked = false;
  parts.forEach((part, index) => {
    if (part === '<mark>') {
      marked = true;
      return;
    }
    if (part === '</mark>') {
      marked = false;
      return;
    }
    const text = part
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'");
    nodes.push(marked ? <mark key={index}>{text}</mark> : <span key={index}>{text}</span>);
  });
  return nodes.length ? nodes : value;
}

export function GlobalSearch({ value, workflowCount, folderCount, memberCount, results = [], onChange, onResultSelect }: GlobalSearchProps) {
  const keyword = value.trim();
  return (
    <div className="exm-global-search-wrap">
      <label className="exm-global-search">
        <Search size={16} strokeWidth={2.1} />
        <input
          value={value}
          placeholder="搜索 workflow、folder、member"
          onChange={(event) => onChange(event.target.value)}
        />
        <span>{workflowCount} W</span>
        <span>{folderCount} F</span>
        <span>{memberCount} M</span>
        <kbd>⌘ K</kbd>
      </label>
      {keyword && results.length ? (
        <div className="exm-global-search-results">
          {results.map((result) => (
            <button key={`${result.type}-${result.id}`} type="button" onMouseDown={(event) => {
              event.preventDefault();
              onResultSelect?.(result);
            }}>
              <strong>{result.type}</strong>
              <span className="exm-search-result-body">
                <b>{renderHighlighted(result.title, result.highlights?.title)}</b>
                <em>{renderHighlighted(result.subtitle, result.highlights?.subtitle)}</em>
                {(result.permissionSources?.length || result.permissionReason) ? (
                  <small>
                    <i>{result.permission || 'VIEW'}</i>
                    {(result.permissionSources?.length ? result.permissionSources : [result.permissionReason]).filter(Boolean).map((source) => (
                      <i key={source}>{source}</i>
                    ))}
                  </small>
                ) : null}
              </span>
            </button>
          ))}
        </div>
      ) : null}
      {keyword && !results.length ? (
        <div className="exm-global-search-results exm-global-search-empty">
          <b>没有找到匹配结果</b>
          <span>可以尝试更短关键词，或切换到 Workflow / Folder / Member 维度搜索。</span>
          <span>也可以新建名为 “{keyword}” 的 Workflow 或 Folder。</span>
        </div>
      ) : null}
    </div>
  );
}
