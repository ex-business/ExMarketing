import { Search } from 'lucide-react';
import type { ReactNode } from 'react';
import { TaBadge, TaInput, TaMenu, TaMenuItem } from './TailAdminPrimitives';

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

function resultTypeLabel(type: GlobalSearchResult['type']): string {
  if (type === 'workflow') return '流程';
  if (type === 'folder') return '文件夹';
  if (type === 'member') return '成员';
  return '工作区';
}

export function GlobalSearch({ value, workflowCount, folderCount, memberCount, results = [], onChange, onResultSelect }: GlobalSearchProps) {
  const keyword = value.trim();
  return (
    <div className="exm-global-search-wrap">
      <label className="exm-global-search">
        <Search size={16} strokeWidth={2.1} />
        <TaInput
          value={value}
          placeholder="搜索流程、文件夹、成员"
          onChange={(event) => onChange(event.target.value)}
        />
        <TaBadge>{workflowCount} 流程</TaBadge>
        <TaBadge>{folderCount} 文件夹</TaBadge>
        <TaBadge>{memberCount} 成员</TaBadge>
        <kbd>⌘ K</kbd>
      </label>
      {keyword && results.length ? (
        <TaMenu className="exm-global-search-results">
          {results.map((result) => (
            <TaMenuItem key={`${result.type}-${result.id}`} onMouseDown={(event) => {
              event.preventDefault();
              onResultSelect?.(result);
            }}>
              <strong>{resultTypeLabel(result.type)}</strong>
              <span className="exm-search-result-body">
                <b>{renderHighlighted(result.title, result.highlights?.title)}</b>
                <em>{renderHighlighted(result.subtitle, result.highlights?.subtitle)}</em>
              </span>
            </TaMenuItem>
          ))}
        </TaMenu>
      ) : null}
      {keyword && !results.length ? (
        <div className="exm-global-search-results exm-global-search-empty">
          <b>没有找到匹配结果</b>
          <span>可以尝试更短关键词，或切换到流程、文件夹、成员维度搜索。</span>
          <span>也可以新建名为 “{keyword}” 的流程或文件夹。</span>
        </div>
      ) : null}
    </div>
  );
}
