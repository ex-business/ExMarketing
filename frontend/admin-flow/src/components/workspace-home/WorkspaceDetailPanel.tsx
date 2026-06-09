import { useMemo, useState } from 'react';
import {
  Archive,
  Copy,
  Folder,
  Link,
  Lock,
  MoreHorizontal,
  Move,
  Pencil,
  Plus,
  Star,
  Trash2,
  Users,
  Workflow,
  X,
} from 'lucide-react';

export type WorkspaceDetailTarget = {
  id: string;
  type: 'Folder' | 'Workflow' | 'Workspace';
  name: string;
  visibility: 'Private' | 'Workspace' | 'Public';
  workflowCount: number;
  folderCount: number;
  memberCount: number;
  updatedLabel?: string;
  owner?: string;
  version?: string;
  status?: string;
};

export type WorkspaceDetailMember = {
  id: string;
  displayName?: string;
  userId?: string;
  email?: string;
  role?: string;
  status?: string;
};

export type WorkspaceDetailAudit = {
  id: string;
  actorId?: string;
  action: string;
  targetType: string;
  createdAt: string;
};

export type WorkspaceActivityGrant = {
  id: string;
  principalType: 'USER' | 'MEMBER' | 'ROLE';
  principalId: string;
  role: 'ADMIN' | 'EDITOR' | 'VIEWER' | 'GUEST';
  createdBy?: string;
  createdAt: string;
};

export type WorkspaceDetailDataSource = {
  label: string;
  value: string;
  tone?: 'good' | 'info' | 'warn';
};

export type WorkspaceDetailCapabilities = {
  role: string;
  canEditWorkspace: boolean;
  canManageFolders: boolean;
  canManageMembers: boolean;
  reason: string;
};

type WorkspaceDetailPanelProps = {
  target: WorkspaceDetailTarget;
  members: WorkspaceDetailMember[];
  audits: WorkspaceDetailAudit[];
  activityGrants: WorkspaceActivityGrant[];
  shareUrl: string;
  dataSources?: WorkspaceDetailDataSource[];
  capabilities: WorkspaceDetailCapabilities;
  errorMessage?: string;
  onVisibilityChange: (visibility: WorkspaceDetailTarget['visibility']) => void;
  onCreateActivityGrant: (input: { principalType: WorkspaceActivityGrant['principalType']; principalId: string; role: WorkspaceActivityGrant['role'] }) => Promise<unknown>;
  onDeleteActivityGrant: (grantId: string) => Promise<unknown>;
  onCopyShare: () => void;
  onManageMembers: () => void;
  onManageShare: () => void;
  onViewAllAudit: () => void;
  onRename: () => void;
  onMove: () => void;
  onArchive: () => void;
  onClose: () => void;
  isLoading?: boolean;
};

function memberInitial(member: WorkspaceDetailMember): string {
  return (member.displayName || member.userId || member.email || 'U').slice(0, 1).toUpperCase();
}

function auditTime(value: string): string {
  const time = new Date(value).getTime();
  if (!Number.isFinite(time)) return '';
  const diff = Date.now() - time;
  const hour = 60 * 60 * 1000;
  const day = 24 * hour;
  if (diff < hour) return '刚刚';
  if (diff < day) return `${Math.max(1, Math.floor(diff / hour))} 小时前`;
  return `${Math.floor(diff / day)} 天前`;
}

export function WorkspaceDetailPanel({
  target,
  members,
  audits,
  activityGrants,
  shareUrl,
  dataSources = [],
  capabilities,
  errorMessage = '',
  onVisibilityChange,
  onCreateActivityGrant,
  onDeleteActivityGrant,
  onCopyShare,
  onManageMembers,
  onManageShare,
  onViewAllAudit,
  onRename,
  onMove,
  onArchive,
  onClose,
  isLoading = false,
}: WorkspaceDetailPanelProps) {
  const [tab, setTab] = useState<'overview' | 'permission' | 'share' | 'audit'>('overview');
  const [grantPrincipalType, setGrantPrincipalType] = useState<WorkspaceActivityGrant['principalType']>('MEMBER');
  const [grantPrincipalId, setGrantPrincipalId] = useState('');
  const [grantRole, setGrantRole] = useState<WorkspaceActivityGrant['role']>('VIEWER');
  const [grantBusy, setGrantBusy] = useState(false);
  const [grantError, setGrantError] = useState('');
  const visibleMembers = useMemo(() => members.filter((member) => member.status !== 'REMOVED').slice(0, 5), [members]);
  const recentAudits = audits.slice(0, 3);
  const TargetIcon = target.type === 'Workflow' ? Workflow : Folder;
  const canEditTarget = target.type === 'Folder' ? capabilities.canManageFolders : capabilities.canEditWorkspace;
  const disabledReason = `${capabilities.reason}，不能修改 ${target.type}。`;
  const grantMembers = useMemo(() => members.filter((member) => member.status !== 'REMOVED'), [members]);
  const principalLabel = (grant: WorkspaceActivityGrant): string => {
    if (grant.principalType === 'ROLE') return grant.principalId;
    const member = members.find((item) => (
      item.id === grant.principalId || item.userId === grant.principalId || item.email === grant.principalId
    ));
    return member?.displayName || member?.email || member?.userId || grant.principalId;
  };
  const submitGrant = async () => {
    const principalId = grantPrincipalId.trim();
    if (!principalId) {
      setGrantError('请选择或输入授权对象。');
      return;
    }
    setGrantBusy(true);
    setGrantError('');
    try {
      await onCreateActivityGrant({ principalType: grantPrincipalType, principalId, role: grantRole });
      setGrantPrincipalId('');
    } catch (error) {
      setGrantError((error as Error).message);
    } finally {
      setGrantBusy(false);
    }
  };

  if (isLoading) {
    return (
      <section className="exm-detail-panel loading">
        <header className="exm-detail-head">
          <div>
            <span className="exm-skeleton exm-detail-label-skeleton" />
            <h2><i className="exm-skeleton exm-detail-icon-skeleton" /><b className="exm-skeleton exm-detail-title-skeleton" /></h2>
            <p className="exm-skeleton exm-detail-meta-skeleton" />
          </div>
          <button type="button" disabled aria-label="收藏"><Star size={17} /></button>
          <button type="button" disabled aria-label="更多"><MoreHorizontal size={17} /></button>
          <button type="button" disabled aria-label="关闭详情"><X size={17} /></button>
        </header>
        <nav className="exm-detail-tabs skeleton-tabs">
          {Array.from({ length: 4 }).map((_, index) => <span key={index} className="exm-skeleton" />)}
        </nav>
        <div className="exm-detail-body">
          <section className="exm-detail-section">
            <span className="exm-skeleton exm-section-title-skeleton" />
            <div className="exm-visibility-switch skeleton-switch">
              {Array.from({ length: 3 }).map((_, index) => <span key={index} className="exm-skeleton" />)}
            </div>
          </section>
          <section className="exm-detail-stats skeleton-stats">
            {Array.from({ length: 3 }).map((_, index) => (
              <div key={index}><span className="exm-skeleton" /><b className="exm-skeleton" /></div>
            ))}
          </section>
          {Array.from({ length: 3 }).map((_, index) => (
            <section key={index} className="exm-detail-section skeleton-detail-section">
              <span className="exm-skeleton" />
              <p className="exm-skeleton" />
              <p className="exm-skeleton short" />
            </section>
          ))}
        </div>
        <footer className="exm-detail-actions skeleton-actions">
          {Array.from({ length: 3 }).map((_, index) => <span key={index} className="exm-skeleton" />)}
        </footer>
      </section>
    );
  }

  return (
    <section className="exm-detail-panel">
      <header className="exm-detail-head">
        <div>
          <span>{target.type}</span>
          <h2><TargetIcon size={21} fill={target.type === 'Folder' ? 'currentColor' : 'none'} />{target.name}</h2>
          <p>{target.status ? `${target.status} · ` : ''}{target.version ? `${target.version} · ` : ''}{target.updatedLabel ?? 'updated 2h ago'}</p>
        </div>
        <button type="button" aria-label="收藏"><Star size={17} /></button>
        <button type="button" aria-label="更多"><MoreHorizontal size={17} /></button>
        <button type="button" aria-label="关闭详情" onClick={onClose}><X size={17} /></button>
      </header>

      <nav className="exm-detail-tabs">
        <button className={tab === 'overview' ? 'active' : ''} type="button" onClick={() => setTab('overview')}>概览</button>
        <button data-testid="detail-tab-permission" className={tab === 'permission' ? 'active' : ''} type="button" onClick={() => setTab('permission')}>权限</button>
        <button className={tab === 'share' ? 'active' : ''} type="button" onClick={() => setTab('share')}>分享</button>
        <button className={tab === 'audit' ? 'active' : ''} type="button" onClick={() => setTab('audit')}>审计</button>
      </nav>

      <div className="exm-detail-body">
        {errorMessage ? (
          <section className="exm-detail-section exm-detail-error" role="alert">
            <h3>接口状态</h3>
            <p>{errorMessage}</p>
          </section>
        ) : null}
        {!canEditTarget ? (
          <section className="exm-detail-section exm-detail-permission">
            <h3>当前权限</h3>
            <p><b>{capabilities.role}</b> · {capabilities.reason}</p>
          </section>
        ) : null}
        {(tab === 'overview' || tab === 'permission') ? (
          <section className="exm-detail-section">
            <h3>可见性</h3>
            <div className="exm-visibility-switch">
              {(['Private', 'Workspace', 'Public'] as const).map((visibility) => (
                <button
                  key={visibility}
                  className={target.visibility === visibility ? 'active' : ''}
                  type="button"
                  disabled={!canEditTarget}
                  title={!canEditTarget ? disabledReason : visibility}
                  onClick={() => onVisibilityChange(visibility)}
                >
                  <Lock size={14} />{visibility}
                </button>
              ))}
            </div>
          </section>
        ) : null}

        {tab === 'permission' && target.type === 'Workflow' ? (
          <section className="exm-detail-section exm-acl-section">
            <div className="exm-detail-section-head">
              <h3>Workflow 单独授权</h3>
              <span>{activityGrants.length} 条</span>
            </div>
            <div className="exm-acl-form">
              <select data-testid="acl-principal-type" value={grantPrincipalType} onChange={(event) => {
                const nextType = event.target.value as WorkspaceActivityGrant['principalType'];
                setGrantPrincipalType(nextType);
                setGrantPrincipalId('');
              }}>
                <option value="MEMBER">Member</option>
                <option value="USER">User</option>
                <option value="ROLE">Role</option>
              </select>
              {grantPrincipalType === 'ROLE' ? (
                <select data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)}>
                  <option value="">选择角色</option>
                  <option value="ADMIN">ADMIN</option>
                  <option value="EDITOR">EDITOR</option>
                  <option value="VIEWER">VIEWER</option>
                  <option value="GUEST">GUEST</option>
                </select>
              ) : grantPrincipalType === 'MEMBER' ? (
                <select data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)}>
                  <option value="">选择成员</option>
                  {grantMembers.map((member) => (
                    <option key={member.id} value={member.id}>{member.displayName || member.email || member.userId || member.id}</option>
                  ))}
                </select>
              ) : (
                <input data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)} placeholder="输入 userId" />
              )}
              <select data-testid="acl-role" value={grantRole} onChange={(event) => setGrantRole(event.target.value as WorkspaceActivityGrant['role'])}>
                <option value="ADMIN">ADMIN</option>
                <option value="EDITOR">EDITOR</option>
                <option value="VIEWER">VIEWER</option>
                <option value="GUEST">GUEST</option>
              </select>
              <button data-testid="acl-add" type="button" disabled={grantBusy || !capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能添加单独授权。` : '添加授权'} onClick={() => void submitGrant()}><Plus size={14} />添加</button>
            </div>
            {grantError ? <p className="exm-acl-error">{grantError}</p> : null}
            <div className="exm-acl-list">
              {activityGrants.length ? activityGrants.map((grant) => (
                <div key={grant.id} className="exm-acl-row">
                  <div>
                    <strong>{principalLabel(grant)}</strong>
                    <span>{grant.principalType} · {grant.principalId}</span>
                  </div>
                  <b>{grant.role}</b>
                  <button data-testid="acl-delete" type="button" aria-label="删除授权" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能删除授权。` : '删除授权'} onClick={() => void onDeleteActivityGrant(grant.id)}><Trash2 size={14} /></button>
                </div>
              )) : <p className="exm-detail-empty">暂无单独授权，当前仅使用可见性和 workspace 成员权限。</p>}
            </div>
          </section>
        ) : null}

        {tab === 'permission' && target.type !== 'Workflow' ? (
          <section className="exm-detail-section">
            <p className="exm-detail-empty">单独 ACL 只作用于 Workflow。Folder 和 Workspace 使用可见性、成员角色和分享权限控制。</p>
          </section>
        ) : null}

        {tab === 'overview' ? (
          <section className="exm-detail-stats">
            <div><span>Workflows</span><b>{target.workflowCount}</b></div>
            <div><span>Folders</span><b>{target.folderCount}</b></div>
            <div><span>Members</span><b>{target.memberCount}</b></div>
          </section>
        ) : null}

        {tab === 'overview' && dataSources.length ? (
          <section className="exm-detail-section exm-data-source-section">
            <div className="exm-detail-section-head">
              <h3>数据真实性</h3>
            </div>
            <div className="exm-data-source-list">
              {dataSources.map((source) => (
                <div key={source.label}>
                  <span>{source.label}</span>
                  <b className={source.tone ?? 'info'}>{source.value}</b>
                </div>
              ))}
            </div>
          </section>
        ) : null}

        {(tab === 'overview' || tab === 'permission') ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>成员</h3>
              <button type="button" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能管理成员。` : '管理成员'} onClick={onManageMembers}>管理成员</button>
            </div>
            <div className="exm-member-row">
              <div className="exm-member-avatars">
                {visibleMembers.length ? visibleMembers.map((member) => (
                  <span key={member.id} title={member.displayName || member.userId || member.email}>{memberInitial(member)}</span>
                )) : <span>U</span>}
              </div>
              <p>{target.memberCount} 位成员</p>
            </div>
          </section>
        ) : null}

        {(tab === 'overview' || tab === 'share') ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>分享链接</h3>
              <button type="button" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能管理分享。` : '管理分享'} onClick={onManageShare}>管理分享</button>
            </div>
            <label className="exm-share-input">
              <Link size={15} />
              <input value={shareUrl} readOnly />
              <button type="button" aria-label="复制分享链接" onClick={onCopyShare}><Copy size={15} /></button>
            </label>
          </section>
        ) : null}

        {(tab === 'overview' || tab === 'audit') ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>最近审计</h3>
              <button type="button" onClick={onViewAllAudit}>查看全部</button>
            </div>
            <div className="exm-audit-list">
              {recentAudits.length ? recentAudits.map((audit) => (
                <div key={audit.id}>
                  <span title={audit.actorId || 'system'}>{(audit.actorId || 'system').slice(0, 1).toUpperCase()}</span>
                  <p>{audit.action} · {audit.targetType}</p>
                  <em>{auditTime(audit.createdAt)}</em>
                </div>
              )) : <p className="exm-detail-empty">暂无审计记录</p>}
            </div>
          </section>
        ) : null}
      </div>

      <footer className="exm-detail-actions">
        <button type="button" disabled={!canEditTarget} title={!canEditTarget ? disabledReason : '重命名'} onClick={onRename}><Pencil size={15} />重命名</button>
        <button type="button" disabled={!canEditTarget || target.type === 'Workspace'} title={target.type === 'Workspace' ? 'Workspace 不支持移动' : !canEditTarget ? disabledReason : '移动'} onClick={onMove}><Move size={15} />移动</button>
        <button className="danger" type="button" disabled={!canEditTarget} title={!canEditTarget ? disabledReason : '归档'} onClick={onArchive}><Archive size={15} />归档</button>
      </footer>
    </section>
  );
}
