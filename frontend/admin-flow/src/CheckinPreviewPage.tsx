import { useEffect, useMemo, useState } from 'react';
import { BadgeCheck, CalendarDays, Check, Gift, LockKeyhole, RotateCcw, ShieldCheck, Sparkles, WalletCards } from 'lucide-react';

type StepStatus = 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED' | 'SKIPPED';

type CheckinStep = {
  id: string;
  title: string;
  description: string;
  status: StepStatus;
  output: string;
};

type CheckinAPIResult = {
  status: 'IDLE' | 'ACCEPTED' | 'DUPLICATE' | 'REJECTED' | 'FAILED';
  idempotencyKey?: string;
  eventId?: string;
  message: string;
  response?: unknown;
};

type CheckinStatus = {
  tenantId: string;
  activityId: string;
  date: string;
  signed: boolean;
  rewarded: boolean;
  streakDays?: number;
  cycleDays?: number;
  cycleCompletedDays?: number;
  todayIndex?: number;
  participationId?: string;
  participationStatus?: string;
  grantOrderId?: string;
  grantStatus?: string;
  rewardType?: string;
  rewardCode?: string;
  rewardAmount?: string;
  eventId?: string;
  idempotencyKey?: string;
  lastUpdatedAt?: string;
  history?: Array<Record<string, unknown>>;
  records?: Array<Record<string, unknown>>;
};

const tenantId = 'site_001';
const defaultActivityCode = 'DAILY_CHECKIN_REWARD_DEV_FINAL';
const subject = { type: 'USER', id: 'user_checkin_demo_001' };

function checkinActivityCodeFromLocation() {
  if (typeof window === 'undefined') return defaultActivityCode;
  const params = new URLSearchParams(window.location.search);
  const requestedCode = params.get('activityCode')?.trim();
  if (requestedCode) return requestedCode;
  return defaultActivityCode;
}

const rewardItems = [
  { day: 1, label: '1 USDT', code: 'DAILY_CHECKIN_1_USDT', active: true },
  { day: 2, label: '100 积分', code: 'CHECKIN_100_POINTS', active: false },
  { day: 3, label: '1 次抽奖', code: 'CHECKIN_DRAW_CHANCE', active: false },
  { day: 4, label: '5 USDT 券', code: 'CHECKIN_COUPON_5', active: false },
  { day: 5, label: '随机现金', code: 'CHECKIN_RANDOM_USDT', active: false },
  { day: 6, label: 'VIP 7 天', code: 'CHECKIN_VIP_7D', active: false },
  { day: 7, label: '实物礼品', code: 'CHECKIN_GIFT', active: false },
];

const initialSteps: CheckinStep[] = [
  { id: 'facts-normalize', title: '上下文标准化', description: '读取用户、KYC、风控和签到上下文。', status: 'PENDING', output: 'standard_context' },
  { id: 'user-level-check', title: '用户等级判断', description: '确认用户存在，等级和 VIP 满足门槛。', status: 'PENDING', output: 'PASS' },
  { id: 'quota-check', title: '每日签到频控', description: '按 userId + activityId 检查今日是否已签到。', status: 'PENDING', output: 'quota_check' },
  { id: 'candidate-reward', title: '候选奖励', description: '从 checkin_reward_config_ref 读取 reward_items。', status: 'PENDING', output: 'DAILY_CHECKIN_1_USDT' },
  { id: 'budget-reserve', title: '预算库存锁定', description: '通过 checkin_reward_budget_ref 原子锁定预算。', status: 'PENDING', output: 'quota_hold' },
  { id: 'grant-order', title: '发奖订单', description: '生成发奖单并投递到钱包发奖通道。', status: 'PENDING', output: 'grant_order' },
];

function wait(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

function statusText(status: StepStatus) {
  if (status === 'RUNNING') return '处理中';
  if (status === 'SUCCESS') return '通过';
  if (status === 'FAILED') return '失败';
  if (status === 'SKIPPED') return '跳过';
  return '等待';
}

function compactAPIValue(value: unknown): string {
  if (!value || typeof value !== 'object') return '';
  const body = value as Record<string, unknown>;
  const grants = Array.isArray(body.grantOrders) ? body.grantOrders.length : 0;
  const outbox = Array.isArray(body.outboxMessages) ? body.outboxMessages.length : 0;
  const rules = Array.isArray(body.ruleResults) ? body.ruleResults.length : 0;
  return `grantOrders=${grants} · outbox=${outbox} · ruleResults=${rules}`;
}

async function postCheckinEvent(activityCode: string) {
  const now = new Date();
  const suffix = `${now.getTime()}`;
  const event = {
    eventId: `evt_checkin_${suffix}`,
    eventType: 'CHECKIN_SUCCESS',
    sourceSystem: 'app',
    tenantId,
    subject,
    biz: { bizType: 'CHECKIN', bizId: `checkin_${suffix}` },
    occurredAt: now.toISOString(),
    attributes: {
      eventType: 'DAILY_CHECKIN',
      checkin: true,
      userLevel: 4,
      vipLevel: 2,
      channel: 'APP',
      activityScene: 'daily_checkin',
      activityCode,
    },
  };
  const idempotencyKey = `${event.tenantId}:${event.sourceSystem}:${event.eventType}:${event.biz.bizType}:${event.biz.bizId}`;
  const response = await fetch('/openapi/v1/events', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Idempotency-Key': idempotencyKey,
    },
    body: JSON.stringify(event),
  });
  const body = await response.json().catch(() => ({}));
  return { response, body, event, idempotencyKey };
}

async function fetchCheckinStatus(activityCode: string) {
  const date = new Date().toISOString().slice(0, 10);
  const query = new URLSearchParams({
    tenantId,
    activityCode,
    subjectType: subject.type,
    subjectId: subject.id,
    date,
  });
  const response = await fetch(`/openapi/v1/checkin/status?${query.toString()}`);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(String((body as Record<string, unknown>).message ?? (body as Record<string, unknown>).error ?? '状态查询失败'));
  }
  return body as CheckinStatus;
}

export function CheckinPreviewPage() {
  const activityCode = useMemo(() => checkinActivityCodeFromLocation(), []);
  const isStreakActivity = activityCode === defaultActivityCode;
  const [signed, setSigned] = useState(false);
  const [running, setRunning] = useState(false);
  const [completedDays, setCompletedDays] = useState(0);
  const [streak, setStreak] = useState(0);
  const [balance, setBalance] = useState('128.00');
  const [lastResult, setLastResult] = useState('');
  const [apiResult, setAPIResult] = useState<CheckinAPIResult>({ status: 'IDLE', message: '尚未调用接口。' });
  const [steps, setSteps] = useState<CheckinStep[]>(initialSteps);

  useEffect(() => {
    let cancelled = false;
    setAPIResult({ status: 'IDLE', message: '正在读取今日签到状态 ...' });
    fetchCheckinStatus(activityCode)
      .then((status) => {
        if (cancelled) return;
        if (status.signed) {
          setSigned(true);
          const restoredDays = Math.max(1, Math.min(7, status.cycleCompletedDays ?? status.streakDays ?? 1));
          setCompletedDays(restoredDays);
          setStreak(Math.max(1, status.streakDays ?? restoredDays));
          if (status.rewarded) {
            setBalance('129.00');
          }
          setSteps((items) => items.map((item) => ({ ...item, status: 'SUCCESS' })));
          setLastResult(status.rewarded ? '已从后端恢复今日签到和发奖状态。' : '已从后端恢复今日签到状态，发奖仍在处理中。');
          setAPIResult({
            status: 'ACCEPTED',
            idempotencyKey: status.idempotencyKey,
            eventId: status.eventId,
            message: `状态已恢复 · participation=${status.participationStatus || '-'} · grant=${status.grantStatus || '-'}`,
            response: status,
          });
          return;
        }
        const pendingCompletedDays = Math.max(0, Math.min(7, (status.todayIndex ?? 1) - 1));
        setCompletedDays(pendingCompletedDays);
        setStreak(pendingCompletedDays);
        setAPIResult({ status: 'IDLE', message: '今日未签到。' });
      })
      .catch((error) => {
        if (cancelled) return;
        setAPIResult({ status: 'FAILED', message: (error as Error).message });
      });
    return () => {
      cancelled = true;
    };
  }, [activityCode]);

  const signedDays = useMemo(() => new Set(Array.from({ length: completedDays }, (_, index) => index + 1)), [completedDays]);
  const todayReward = rewardItems[Math.max(0, Math.min(6, completedDays || 0))] ?? rewardItems[0];

  const runCheckin = async () => {
    if (running || signed) return;
    setRunning(true);
    setLastResult('');
    setAPIResult({ status: 'IDLE', message: '正在发送 POST /openapi/v1/events ...' });
    setSteps(initialSteps);
    const requestPromise = postCheckinEvent(activityCode);
    for (const step of initialSteps) {
      setSteps((items) => items.map((item) => item.id === step.id ? { ...item, status: 'RUNNING' } : item));
      await wait(180);
    }
    try {
      const { response, body, event, idempotencyKey } = await requestPromise;
      const accepted = response.ok;
      const duplicate = response.status === 409 && Boolean((body as Record<string, unknown>).duplicate);
      const rejected = response.status === 422;
      const grantCount = Array.isArray((body as Record<string, unknown>).grantOrders) ? ((body as Record<string, unknown>).grantOrders as unknown[]).length : 0;
      const outboxCount = Array.isArray((body as Record<string, unknown>).outboxMessages) ? ((body as Record<string, unknown>).outboxMessages as unknown[]).length : 0;
      if (accepted || duplicate) {
        const completedGrant = duplicate || grantCount > 0 || outboxCount > 0;
        setSteps((items) => items.map((item) => ({ ...item, status: completedGrant ? 'SUCCESS' : 'SKIPPED' })));
        setSigned(completedGrant);
        if (completedGrant) {
          try {
            const latestStatus = await fetchCheckinStatus(activityCode);
            const restoredDays = Math.max(1, Math.min(7, latestStatus.cycleCompletedDays ?? latestStatus.streakDays ?? 1));
            setSigned(latestStatus.signed);
            setCompletedDays(restoredDays);
            setStreak(Math.max(1, latestStatus.streakDays ?? restoredDays));
            if (latestStatus.rewarded) {
              setBalance('129.00');
            }
          } catch {
            setCompletedDays((value) => Math.max(1, Math.min(7, value + 1)));
            setStreak((value) => Math.max(1, Math.min(7, value + 1)));
            setBalance('129.00');
          }
        }
        setLastResult(duplicate ? '接口返回重复事件，签到结果已存在。' : completedGrant ? '接口已接收，签到事件已进入工作流并生成发奖结果。' : '接口已接收，但未生成发奖单；需要发布匹配的签到活动。');
        setAPIResult({
          status: duplicate ? 'DUPLICATE' : 'ACCEPTED',
          idempotencyKey,
          eventId: event.eventId,
          message: `${response.status} ${response.statusText || 'OK'} · ${compactAPIValue(body)}`,
          response: body,
        });
      } else {
        const reason = String(
          (body as Record<string, unknown>).message
            ?? (body as Record<string, unknown>).error
            ?? (body as Record<string, unknown>).code
            ?? 'UNKNOWN',
        );
        setSteps((items) => items.map((item) => ({ ...item, status: rejected ? 'SKIPPED' : 'FAILED' })));
        setLastResult(rejected ? `接口已处理，但业务规则拒绝：${reason}` : '接口调用失败。');
        setAPIResult({
          status: rejected ? 'REJECTED' : 'FAILED',
          idempotencyKey,
          eventId: event.eventId,
          message: `${response.status} ${response.statusText || 'ERROR'} · ${reason}`,
          response: body,
        });
      }
    } catch (error) {
      setSteps((items) => items.map((item) => ({ ...item, status: 'FAILED' })));
      setLastResult('接口不可用或网络失败。');
      setAPIResult({ status: 'FAILED', message: (error as Error).message });
    } finally {
      setRunning(false);
    }
  };

  const resetDemo = () => {
    setSigned(false);
    setRunning(false);
    setCompletedDays(0);
    setStreak(0);
    setBalance('128.00');
    setLastResult('');
    setAPIResult({ status: 'IDLE', message: '尚未调用接口。' });
    setSteps(initialSteps);
  };

  return (
    <main className="checkin-page">
      <section className="checkin-shell">
        <header className="checkin-header">
          <div>
            <span>{isStreakActivity ? '7-Day Check-in' : 'Daily Check-in'}</span>
            <h1>{isStreakActivity ? '7 日连续签到奖励' : '每日签到奖励'}</h1>
            <p>当前测试活动：{activityCode}。签到链路使用 Activity State、Quota Check、Candidate Reward、Budget Reserve 和 Grant Order。</p>
          </div>
          <a href={isStreakActivity ? '/admin-flow/?activityId=act_9330403ed9cfe4b86b2523e0' : '/admin-flow/'} aria-label="返回工作流画布">工作流画布</a>
        </header>

        <section className="checkin-overview">
          <article className="checkin-user-panel">
            <div className="checkin-user-main">
              <div className="checkin-avatar">T</div>
              <div>
                <span>USER · user_001</span>
                <b>VIP 2 / Level 4</b>
              </div>
            </div>
            <div className="checkin-metrics">
              <span><b>{streak}</b><em>连续签到</em></span>
              <span><b>{balance}</b><em>USDT 余额</em></span>
              <span><b>{signed ? '已签到' : '待签到'}</b><em>今日状态</em></span>
            </div>
          </article>

          <article className="checkin-reward-panel">
            <div>
              <Gift size={22} />
              <span>
                <b>{todayReward.label}</b>
                <em>{todayReward.code}</em>
              </span>
            </div>
            <p>奖励配置来自 Redis Ref：{isStreakActivity ? 'checkin_streak_7d_reward_config_ref' : 'checkin_reward_config_ref'}，预算锁定来自 {isStreakActivity ? 'checkin_streak_budget_inventory_ref' : 'checkin_reward_budget_ref'}。</p>
          </article>
        </section>

        <section className="checkin-card">
          <div className="checkin-card-head">
            <span>
              <CalendarDays size={20} />
              <b>7 天签到进度</b>
            </span>
            <button type="button" onClick={resetDemo}>
              <RotateCcw size={16} />
              重置
            </button>
          </div>

          <div className="checkin-days">
            {rewardItems.map((item) => {
              const done = signedDays.has(item.day);
              return (
                <div key={item.day} className={done ? 'done' : item.active ? 'active' : ''}>
                  <span>DAY {item.day}</span>
                  <b>{item.label}</b>
                  <em>{done ? <Check size={16} /> : item.code}</em>
                </div>
              );
            })}
          </div>

          <div className="checkin-action-row">
            <button type="button" onClick={runCheckin} disabled={running || signed}>
              <Sparkles size={18} />
              {running ? '签到处理中' : signed ? '今日已签到' : '立即签到'}
            </button>
            <span>{lastResult || '点击后会调用 /openapi/v1/events，并提交 CHECKIN_SUCCESS 标准事件。'}</span>
          </div>
        </section>

        <section className={`checkin-api-result ${apiResult.status.toLowerCase()}`}>
          <div>
            <span>Endpoint</span>
            <b>POST /openapi/v1/events</b>
          </div>
          <div>
            <span>接口调用</span>
            <b>{apiResult.message}</b>
          </div>
          <div>
            <span>Idempotency-Key</span>
            <b>{apiResult.idempotencyKey || '-'}</b>
          </div>
          <div>
            <span>Event ID</span>
            <b>{apiResult.eventId || '-'}</b>
          </div>
          {apiResult.response ? <pre>{JSON.stringify(apiResult.response, null, 2)}</pre> : null}
        </section>

        <section className="checkin-flow">
          {steps.map((step, index) => (
            <article key={step.id} className={step.status.toLowerCase()}>
              <i>{index + 1}</i>
              <div>
                <span>{step.title}</span>
                <b>{step.description}</b>
                <em>{step.output}</em>
              </div>
              <strong>{statusText(step.status)}</strong>
            </article>
          ))}
        </section>

        <section className="checkin-contract">
          <div><ShieldCheck size={18} /><span>资格通过后才生成候选奖励</span></div>
          <div><LockKeyhole size={18} /><span>预算库存锁定只做并发安全预占</span></div>
          <div><WalletCards size={18} /><span>发奖事实由 Grant Order 和 Outbox 承接</span></div>
          <div><BadgeCheck size={18} /><span>失败分支进入 Audit，不影响成功链路</span></div>
        </section>
      </section>
    </main>
  );
}
