import type { ResourceRecord, ResourceStatus, ResourceTab } from '../types';
import { Button } from './Button';

type DraftResourceRow = {
  mode: Exclude<ResourceTab, '图片'>;
  draftType: string;
  draftSource: string;
  draftValue: string;
  onDraftType: (value: string) => void;
  onDraftSource: (value: string) => void;
  onSave: () => void;
  onCancel: () => void;
};

type ResourceTableProps = {
  activeTab: ResourceTab;
  records: ResourceRecord[];
  selectedId: string;
  draftRow?: DraftResourceRow;
  statusClass: (status: ResourceStatus) => string;
  onSelect: (record: ResourceRecord) => void;
};

function resourceLabel(record: ResourceRecord) {
  if (record.tab === '图片') return <i>{record.thumbnail ?? 'IMG'}</i>;
  if (record.tab === '数据源') return <i>API</i>;
  if (record.tab === '样式') return <i style={{ background: record.value ?? 'var(--theme-accent)' }} />;
  return null;
}

export function ResourceTable({ activeTab, records, selectedId, draftRow, statusClass, onSelect }: ResourceTableProps) {
  return (
    <table className="campaign-resource-table">
      <thead>
        <tr>
          <th>{activeTab === '图片' ? '名称' : activeTab === '数据源' ? '数据源 / 标识符' : '名称 / 标识符'}</th>
          <th>{activeTab === '图片' ? '用途' : '类型'}</th>
          <th>{activeTab === '数据源' ? '接口地址' : '来源'}</th>
          {activeTab === '图片' ? <th>尺寸</th> : activeTab === '数据源' ? <th>刷新策略</th> : <th>引用</th>}
          {activeTab === '图片' ? <th>大小</th> : <th>状态</th>}
          <th>更新时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        {draftRow ? (
          <tr className="adding">
            <td><b>{draftRow.mode === '数据' ? '新增数据字段' : draftRow.mode === '数据源' ? '新增数据源' : draftRow.mode === '动作' ? '新增动作事件' : '新增样式 Token'}</b></td>
            <td>
              <select value={draftRow.draftType} onChange={(event) => draftRow.onDraftType(event.target.value)}>
                <option>文本</option><option>数值</option><option>REST API</option><option>GraphQL</option><option>WebSocket</option><option>Workflow Event</option><option>颜色</option><option>圆角</option>
              </select>
            </td>
            <td>{draftRow.mode === '数据源' ? draftRow.draftValue : (
              <select value={draftRow.draftSource} onChange={(event) => draftRow.onDraftSource(event.target.value)}>
                <option>Activity View Model</option><option>页面自定义</option><option>系统内置</option><option>实时接口</option>
              </select>
            )}</td>
            <td>{draftRow.mode === '数据源' ? '页面加载 + 30s 轮询' : <Button size="sm">选择组件</Button>}</td>
            <td><span className="campaign-resource-status enabled">启用</span></td>
            <td>--</td>
            <td className="campaign-resource-row-actions">
              <Button size="sm" variant="primary" onClick={draftRow.onSave}>保存</Button>
              <Button size="sm" onClick={draftRow.onCancel}>取消</Button>
            </td>
          </tr>
        ) : null}
        {records.map((record) => (
          <tr key={record.id} className={selectedId === record.id ? 'selected' : ''} onClick={() => onSelect(record)}>
            <td>
              <span className="campaign-resource-name">
                {resourceLabel(record)}
                <span><strong>{record.name}</strong><small>{record.identifier}</small></span>
              </span>
            </td>
            <td>{record.type}</td>
            <td>{record.tab === '数据源' ? record.endpoint : record.source}</td>
            <td>{record.tab === '图片' ? record.size : record.tab === '数据源' ? record.refresh : record.usage}</td>
            <td>{record.tab === '图片' ? record.fileSize : <span className={`campaign-resource-status ${statusClass(record.status)}`}>{record.status}</span>}</td>
            <td>{record.updatedAt}</td>
            <td><Button size="sm" variant="ghost" aria-label={`更多 ${record.name}`} onClick={(event) => event.stopPropagation()}>...</Button></td>
          </tr>
        ))}
        {records.length === 0 && !draftRow ? <tr><td colSpan={7} className="campaign-resource-empty">没有匹配资源</td></tr> : null}
      </tbody>
    </table>
  );
}
