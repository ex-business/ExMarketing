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
import { TaButton, TaIconButton, TaInput, TaSelect, TaTab, TaTabs } from './TailAdminPrimitives';

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
  templateCount: number;
  activeWorkflowCount: number;
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
  onCreateFolder: () => void;
  onCreateWorkflow: () => void;
  onImportWorkflow: () => void;
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

function targetTypeLabel(value: WorkspaceDetailTarget['type']): string {
  if (value === 'Folder') return '文件夹';
  if (value === 'Workspace') return '工作区';
  return '流程';
}

function visibilityLabel(value: WorkspaceDetailTarget['visibility']): string {
  if (value === 'Public') return '公开';
  if (value === 'Workspace') return '工作区';
  return '私有';
}

function statusLabel(value: string): string {
  const normalized = value.toUpperCase();
  if (normalized === 'ACTIVE' || normalized === 'PUBLISHED') return '已启用';
  if (normalized === 'DRAFT') return '草稿';
  if (normalized === 'ARCHIVED') return '已归档';
  return value;
}

export function WorkspaceDetailPanel({
  target,
  members,
  audits,
  activityGrants,
  shareUrl,
  templateCount,
  activeWorkflowCount,
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
  onCreateFolder,
  onCreateWorkflow,
  onImportWorkflow,
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
  const disabledReason = `${capabilities.reason}，不能修改${targetTypeLabel(target.type)}。`;
  const disabledActionReason = (action: string): string => `${capabilities.reason}，不能执行${action}。`;
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
    return null;
  }

  return (
    <section className="exm-detail-panel">
      <header className="exm-detail-head">
        <div className="exm-detail-title-block">
          <span>{targetTypeLabel(target.type)}</span>
          <h2><TargetIcon size={21} fill={target.type === 'Folder' ? 'currentColor' : 'none'} />{target.name}</h2>
          <p>{target.status ? `${statusLabel(target.status)} · ` : ''}{target.version ? `${target.version} · ` : ''}{target.updatedLabel ?? '2 小时前更新'}</p>
        </div>
        <div className="exm-detail-head-actions">
          <TaIconButton aria-label="收藏"><Star size={17} /></TaIconButton>
          <TaIconButton aria-label="更多"><MoreHorizontal size={17} /></TaIconButton>
          <TaIconButton aria-label="关闭详情" onClick={onClose}><X size={17} /></TaIconButton>
        </div>
        {canEditTarget ? (
          <div className="exm-detail-quick-actions">
            <TaButton size="sm" title="重命名" onClick={onRename}><Pencil size={14} />重命名</TaButton>
            <TaButton size="sm" disabled={target.type === 'Workspace'} title={target.type === 'Workspace' ? '工作区不支持移动' : '移动'} onClick={onMove}><Move size={14} />移动</TaButton>
            <TaButton size="sm" className="danger" tone="danger" title="归档" onClick={onArchive}><Archive size={14} />归档</TaButton>
          </div>
        ) : (
          <div className="exm-detail-quick-actions readonly">
            <span title={disabledReason}>{capabilities.role} · 只读浏览</span>
          </div>
        )}
      </header>

      <TaTabs className="exm-detail-tabs">
        <TaTab active={tab === 'overview'} onClick={() => setTab('overview')}>概览</TaTab>
        <TaTab data-testid="detail-tab-permission" active={tab === 'permission'} onClick={() => setTab('permission')}>权限</TaTab>
        <TaTab active={tab === 'share'} onClick={() => setTab('share')}>分享</TaTab>
        <TaTab active={tab === 'audit'} onClick={() => setTab('audit')}>审计</TaTab>
      </TaTabs>

      <div className="exm-detail-body">
        {errorMessage ? (
          <section className="exm-detail-section exm-detail-error" role="alert">
            <h3>接口状态</h3>
            <p>{errorMessage}</p>
          </section>
        ) : null}
        {tab === 'permission' && !canEditTarget ? (
          <section className="exm-detail-section exm-detail-permission">
            <h3>当前权限</h3>
            <p><b>{capabilities.role}</b> · {capabilities.reason}</p>
          </section>
        ) : null}
        {tab === 'permission' ? (
          <section className="exm-detail-section">
            <h3>可见性</h3>
            <div className="exm-visibility-switch">
              {(['Private', 'Workspace', 'Public'] as const).map((visibility) => (
                <TaButton
                  key={visibility}
                  className={target.visibility === visibility ? 'active' : ''}
                  size="sm"
                  disabled={!canEditTarget}
                  title={!canEditTarget ? disabledReason : visibilityLabel(visibility)}
                  onClick={() => onVisibilityChange(visibility)}
                >
                  <Lock size={14} />{visibilityLabel(visibility)}
                </TaButton>
              ))}
            </div>
          </section>
        ) : null}

        {tab === 'permission' && target.type === 'Workflow' ? (
          <section className="exm-detail-section exm-acl-section">
            <div className="exm-detail-section-head">
              <h3>流程单独授权</h3>
              <span>{activityGrants.length} 条</span>
            </div>
            <div className="exm-acl-form">
              <TaSelect data-testid="acl-principal-type" value={grantPrincipalType} onChange={(event) => {
                const nextType = event.target.value as WorkspaceActivityGrant['principalType'];
                setGrantPrincipalType(nextType);
                setGrantPrincipalId('');
              }}>
                <option value="MEMBER">成员</option>
                <option value="USER">用户</option>
                <option value="ROLE">角色</option>
              </TaSelect>
              {grantPrincipalType === 'ROLE' ? (
                <TaSelect data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)}>
                  <option value="">选择角色</option>
                  <option value="ADMIN">ADMIN</option>
                  <option value="EDITOR">EDITOR</option>
                  <option value="VIEWER">VIEWER</option>
                  <option value="GUEST">GUEST</option>
                </TaSelect>
              ) : grantPrincipalType === 'MEMBER' ? (
                <TaSelect data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)}>
                  <option value="">选择成员</option>
                  {grantMembers.map((member) => (
                    <option key={member.id} value={member.id}>{member.displayName || member.email || member.userId || member.id}</option>
                  ))}
                </TaSelect>
              ) : (
                <TaInput data-testid="acl-principal-id" value={grantPrincipalId} onChange={(event) => setGrantPrincipalId(event.target.value)} placeholder="输入 userId" />
              )}
              <TaSelect data-testid="acl-role" value={grantRole} onChange={(event) => setGrantRole(event.target.value as WorkspaceActivityGrant['role'])}>
                <option value="ADMIN">ADMIN</option>
                <option value="EDITOR">EDITOR</option>
                <option value="VIEWER">VIEWER</option>
                <option value="GUEST">GUEST</option>
              </TaSelect>
            <TaButton tone="primary" data-testid="acl-add" disabled={grantBusy || !capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能添加单独授权。` : '添加授权'} onClick={() => void submitGrant()}><Plus size={14} />添加</TaButton>
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
                  <TaIconButton data-testid="acl-delete" aria-label="删除授权" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能删除授权。` : '删除授权'} onClick={() => void onDeleteActivityGrant(grant.id)}><Trash2 size={14} /></TaIconButton>
                </div>
              )) : <p className="exm-detail-empty">暂无单独授权，当前仅使用可见性和工作区成员权限。</p>}
            </div>
          </section>
        ) : null}

        {tab === 'permission' && target.type !== 'Workflow' ? (
          <section className="exm-detail-section exm-permission-note">
            <h3>权限继承</h3>
            <p>文件夹使用可见性和工作区成员角色控制访问。流程级 ACL 只在具体流程上配置。</p>
          </section>
        ) : null}

        {tab === 'overview' ? (
          <section className="exm-detail-stats exm-workspace-stats">
            <div><span>流程</span><b>{target.workflowCount}</b></div>
            <div><span>文件夹</span><b>{target.folderCount}</b></div>
            <div><span>模板</span><b>{templateCount}</b></div>
            <div><span>可用</span><b>{activeWorkflowCount}</b></div>
          </section>
        ) : null}

        {tab === 'overview' ? (
          <section className="exm-detail-section exm-detail-create-actions">
            <h3>创建入口</h3>
            {!capabilities.canEditWorkspace ? <p>{capabilities.role} · 只读浏览</p> : null}
            <div>
              <TaButton
                size="sm"
                data-testid="workspace-new-folder"
                disabled={!capabilities.canManageFolders}
                title={!capabilities.canManageFolders ? disabledActionReason('新建文件夹') : '新建文件夹'}
                onClick={onCreateFolder}
              >
                <Folder size={14} />新建文件夹
              </TaButton>
              <TaButton
                size="sm"
                disabled={!capabilities.canEditWorkspace}
                title={!capabilities.canEditWorkspace ? disabledActionReason('导入流程') : '导入流程'}
                onClick={onImportWorkflow}
              >
                <Copy size={14} />导入流程
              </TaButton>
              <TaButton
                size="sm"
                tone="primary"
                data-testid="workspace-new-workflow"
                disabled={!capabilities.canEditWorkspace}
                title={!capabilities.canEditWorkspace ? disabledActionReason('新建流程') : '新建流程'}
                onClick={onCreateWorkflow}
              >
                <Plus size={14} />新建流程
              </TaButton>
            </div>
          </section>
        ) : null}

        {tab === 'overview' ? (
          <section className="exm-detail-section exm-detail-summary">
            <h3>摘要</h3>
            <div>
              <span>可见性</span>
              <b>{visibilityLabel(target.visibility)}</b>
            </div>
            <div>
              <span>权限</span>
              <b>{capabilities.role}</b>
            </div>
            <div>
              <span>成员</span>
              <b>{target.memberCount}</b>
            </div>
          </section>
        ) : null}

        {tab === 'permission' ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>成员</h3>
              <TaButton size="sm" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能管理成员。` : '管理成员'} onClick={onManageMembers}>管理成员</TaButton>
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

        {tab === 'share' ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>分享链接</h3>
              <TaButton size="sm" disabled={!capabilities.canManageMembers} title={!capabilities.canManageMembers ? `${capabilities.reason}，不能管理分享。` : '管理分享'} onClick={onManageShare}>管理分享</TaButton>
            </div>
            <label className="exm-share-input">
              <Link size={15} />
              <TaInput value={shareUrl} readOnly />
              <TaIconButton aria-label="复制分享链接" onClick={onCopyShare}><Copy size={15} /></TaIconButton>
            </label>
          </section>
        ) : null}

        {tab === 'audit' ? (
          <section className="exm-detail-section">
            <div className="exm-detail-section-head">
              <h3>最近审计</h3>
              <TaButton size="sm" onClick={onViewAllAudit}>查看全部</TaButton>
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

        {tab === 'audit' && dataSources.length ? (
          <section className="exm-detail-section exm-data-source-section">
            <div className="exm-detail-section-head">
              <h3>数据来源</h3>
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
      </div>

    </section>
  );
}
