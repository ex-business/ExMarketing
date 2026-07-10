import { useState, type Dispatch, type SetStateAction } from 'react';
import type { CampaignPageSchema, CampaignPageVersion, CampaignValidationIssue, ResourceRecord, WorkspaceItem, WorkspaceSection } from './types';
import { CampaignResourceWorkspace } from './ResourceWorkspace';
import { Button, DataTable, Field, Panel, Tree, type DataTableColumn, type TreeNode } from './ui';

type SecondarySection = Exclude<WorkspaceSection, 'page'>;

const workspaceConfig: Record<SecondarySection, { groups: string[]; items: WorkspaceItem[]; detailTitle: string }> = {
  resources: { groups: [], items: [], detailTitle: '资源详情' },
  settings: {
    groups: ['基础信息', '发布', '分享', '权限'],
    detailTitle: '发布预览',
    items: [
      { title: '页面名称', meta: '活动页面名称', detail: '展示在编辑器、页面列表和发布版本中。', group: '基础信息', tags: ['Page'] },
      { title: '页面路径', meta: '唯一访问路径', detail: '发布后按 slug 查找对应页面快照。', group: '基础信息', tags: ['Route'] },
      { title: '发布环境', meta: 'Production', detail: '发布前校验 schema、绑定、动作权限和移动端溢出。', group: '发布', tags: ['DRAFT', 'PUBLISHED'] },
      { title: '分享卡片', meta: '标题与封面', detail: '用于活动链接的社交分享预览。', group: '分享', tags: ['Share'] },
      { title: '编辑权限', meta: '运营团队', detail: '发布操作需要对应页面权限。', group: '权限', tags: ['Owner', 'Editor'] },
    ],
  },
  templates: {
    groups: ['全部', '充值返利', '签到任务', '邀请裂变', '优惠券', '新手活动', '排行榜'],
    detailTitle: '模板详情',
    items: [
      { title: '夏日充值返利', meta: '充值返利 · 5 字段 · 1 动作', detail: '包含头图、奖励卡、倒计时、进度、领取按钮和规则。', group: '充值返利', tags: ['activity.title', 'reward.amount', 'CLAIM_REWARD'] },
      { title: '每日签到领豪礼', meta: '签到任务 · 4 字段 · 2 动作', detail: '适合连续签到和每日奖励。', group: '签到任务', tags: ['OPEN_LOGIN', 'TRACK_EVENT'] },
      { title: '邀请好友奖励', meta: '邀请裂变 · 6 字段 · 2 动作', detail: '包含邀请码、奖励进度和邀请按钮。', group: '邀请裂变', tags: ['NAVIGATE'] },
      { title: '优惠券领取', meta: '优惠券 · 4 字段 · 1 动作', detail: '适合券包领取和权益展示。', group: '优惠券', tags: ['CLAIM_REWARD'] },
      { title: '新手任务礼包', meta: '新手活动 · 7 字段 · 2 动作', detail: '包含任务列表和阶段奖励。', group: '新手活动', tags: ['TRACK_EVENT'] },
      { title: '排行榜冲刺', meta: '排行榜 · 5 字段 · 1 动作', detail: '适合活动榜单冲刺页。', group: '排行榜', tags: ['progress.rank'] },
    ],
  },
};

function sectionTitle(section: SecondarySection) {
  if (section === 'settings') return ['设置', '配置路径、发布、分享和版本信息'];
  if (section === 'templates') return ['模板库', '从标准活动模板创建页面草稿'];
  return ['资源', '集中管理数据、数据源、动作、样式和图片'];
}

export function CampaignWorkspace({
  section,
  page,
  versions,
  validationIssues,
  resourceRecords,
  onResourceRecordsChange,
  onPagePatch,
  onCreateFromTemplate,
  onRollbackVersion,
}: {
  section: SecondarySection;
  page: CampaignPageSchema;
  versions: CampaignPageVersion[];
  validationIssues: CampaignValidationIssue[];
  resourceRecords: ResourceRecord[];
  onResourceRecordsChange: Dispatch<SetStateAction<ResourceRecord[]>>;
  onPagePatch: (patch: Partial<CampaignPageSchema>) => void;
  onCreateFromTemplate: (templateName: string) => void;
  onRollbackVersion: (version: CampaignPageVersion) => void;
}) {
  if (section === 'resources') return <CampaignResourceWorkspace records={resourceRecords} onRecordsChange={onResourceRecordsChange} />;
  return <SecondaryWorkspace section={section} page={page} versions={versions} validationIssues={validationIssues} onPagePatch={onPagePatch} onCreateFromTemplate={onCreateFromTemplate} onRollbackVersion={onRollbackVersion} />;
}

function SecondaryWorkspace({
  section,
  page,
  versions,
  validationIssues,
  onPagePatch,
  onCreateFromTemplate,
  onRollbackVersion,
}: {
  section: Exclude<SecondarySection, 'resources'>;
  page: CampaignPageSchema;
  versions: CampaignPageVersion[];
  validationIssues: CampaignValidationIssue[];
  onPagePatch: (patch: Partial<CampaignPageSchema>) => void;
  onCreateFromTemplate: (templateName: string) => void;
  onRollbackVersion: (version: CampaignPageVersion) => void;
}) {
  const config = workspaceConfig[section];
  const [activeGroup, setActiveGroup] = useState(config.groups[0]);
  const [selected, setSelected] = useState(0);
  const [title, description] = sectionTitle(section);
  const items = config.items.filter((item) => activeGroup === '全部' || item.group === activeGroup);
  const current = items[selected] ?? items[0] ?? config.items[0];
  const groupNodes: TreeNode[] = config.groups.map((group) => ({
    id: `workspace_group_${section}_${group}`,
    label: group,
    count: config.items.filter((item) => group === '全部' || item.group === group).length,
    active: activeGroup === group,
    onClick: () => { setActiveGroup(group); setSelected(0); },
  }));
  const columns: Array<DataTableColumn<WorkspaceItem>> = [{
    key: 'item',
    header: activeGroup,
    render: (item) => <span className="campaign-secondary-table-item"><strong>{item.title}</strong><small>{item.meta}</small><em>{item.detail}</em></span>,
  }];
  return (
    <section className="campaign-secondary-workspace">
      <aside className="campaign-secondary-groups"><h2>{title}</h2><p>{description}</p><Tree nodes={groupNodes} className="campaign-secondary-group-tree" /></aside>
      <div className="campaign-secondary-list">
        <header><h2>{activeGroup}</h2><p>{description}</p></header>
        <DataTable className="campaign-secondary-table" columns={columns} rows={items} rowKey={(item) => item.title} selectedKey={current.title} onRowClick={(item) => setSelected(items.findIndex((entry) => entry.title === item.title))} />
      </div>
      <Panel title={config.detailTitle} className="campaign-secondary-detail" bodyClassName="campaign-secondary-detail-body">
        <h2>{current.title}</h2><p>{current.detail}</p>
        <div className="campaign-secondary-tags">{current.tags.map((tag) => <b key={tag}>{tag}</b>)}</div>
        <dl>
          <div><dt>状态</dt><dd>{current.meta}</dd></div>
          <div><dt>发布校验</dt><dd>{versions.length ? `最近发布 ${versions[0].version} · ${new Date(versions[0].publishedAt).toLocaleString()}` : '发布前检查绑定、动作权限和页面路径。'}</dd></div>
        </dl>
        {section === 'settings' ? (
          <>
            <div className={validationIssues.length ? 'campaign-validation-list blocking' : 'campaign-validation-list'}><h3>发布校验</h3>{validationIssues.length ? validationIssues.map((issue) => <p key={issue.code}><strong>{issue.code}</strong>{issue.message}</p>) : <p><strong>PASS</strong>schema、绑定、动作和路径通过。</p>}</div>
            <div className="campaign-secondary-form">
              <Field label="页面名称"><input value={page.name} onChange={(event) => onPagePatch({ name: event.target.value })} /></Field>
              <Field label="页面路径"><input value={page.path} onChange={(event) => onPagePatch({ path: event.target.value })} /></Field>
              <Field label="活动 ID"><input value={page.activityId} onChange={(event) => onPagePatch({ activityId: event.target.value })} /></Field>
            </div>
            {versions.length ? <div className="campaign-version-list"><h3>历史版本</h3>{versions.map((version) => <button key={version.id} type="button" onClick={() => onRollbackVersion(version)}><strong>{version.version}</strong><span>{new Date(version.publishedAt).toLocaleString()}</span><small>回滚为草稿</small></button>)}</div> : null}
          </>
        ) : <Button className="campaign-secondary-primary" variant="primary" onClick={() => onCreateFromTemplate(current.title)}>从模板创建</Button>}
      </Panel>
    </section>
  );
}
