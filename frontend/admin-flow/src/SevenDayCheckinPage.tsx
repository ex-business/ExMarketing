import { useEffect, useMemo, useState } from 'react';
import { ArrowLeft, BadgeCheck, CalendarCheck2, Check, CircleDollarSign, Clock3, Gift, Loader2, ShieldCheck, Sparkles, Trophy, WalletCards } from 'lucide-react';

type APIStatus = 'IDLE' | 'LOADING' | 'ACCEPTED' | 'DUPLICATE' | 'REJECTED' | 'FAILED';

type CheckinStatus = {
  tenantId: string;
  activityCode: string;
  date: string;
  signed: boolean;
  rewarded: boolean;
  streakDays?: number;
  cycleDays?: number;
  cycleCompletedDays?: number;
  todayIndex?: number;
  participationStatus?: string;
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

type APIResult = {
  status: APIStatus;
  message: string;
  eventId?: string;
  idempotencyKey?: string;
  response?: unknown;
};

const tenantId = 'site_001';
const activityCode = 'DAILY_CHECKIN_STREAK_REWARD';
const activityId = 'act_9330403ed9cfe4b86b2523e0';
const subject = { type: 'USER', id: 'user_checkin_demo_001' };

const rewards = [
  { day: 1, title: '1 USDT', code: 'DAILY_CHECKIN_DAY1_1_USDT', tone: 'cash' },
  { day: 2, title: '100 积分', code: 'CHECKIN_DAY2_POINTS_100', tone: 'points' },
  { day: 3, title: '1 次抽奖', code: 'CHECKIN_DAY3_DRAW', tone: 'draw' },
  { day: 4, title: '5 USDT 券', code: 'CHECKIN_DAY4_COUPON_5', tone: 'coupon' },
  { day: 5, title: '随机现金', code: 'CHECKIN_DAY5_RANDOM_CASH', tone: 'cash' },
  { day: 6, title: 'VIP 7 天', code: 'CHECKIN_DAY6_VIP_7D', tone: 'vip' },
  { day: 7, title: '实物礼品', code: 'CHECKIN_DAY7_GIFT', tone: 'gift' },
];

const pipeline = [
  ['签到事件入口', '接收 CHECKIN_SUCCESS 标准事件'],
  ['获取连续签到状态', '读取昨日/今日状态并计算连续天数'],
  ['今日签到频控', '同一用户同一活动每天只允许一次'],
  ['7日候选奖励', '按 todayIndex 匹配奖励配置 Ref'],
  ['预算库存锁定', '通过 provider 原子锁定预算或库存'],
  ['签到发奖订单', '生成 Grant Order 并进入 Outbox'],
];

function todayISO() {
  return new Date().toISOString().slice(0, 10);
}

function compactResult(value: unknown) {
  if (!value || typeof value !== 'object') return '';
  const body = value as Record<string, unknown>;
  const grants = Array.isArray(body.grantOrders) ? body.grantOrders.length : 0;
  const outbox = Array.isArray(body.outboxMessages) ? body.outboxMessages.length : 0;
  const rules = Array.isArray(body.ruleResults) ? body.ruleResults.length : 0;
  return `grantOrders=${grants} / outbox=${outbox} / ruleResults=${rules}`;
}

async function fetchSevenDayStatus() {
  const query = new URLSearchParams({
    tenantId,
    activityCode,
    subjectType: subject.type,
    subjectId: subject.id,
    date: todayISO(),
    cycleDays: '7',
  });
  const response = await fetch(`/openapi/v1/checkin/status?${query.toString()}`);
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(String((body as Record<string, unknown>).message ?? (body as Record<string, unknown>).error ?? '状态查询失败'));
  }
  return body as CheckinStatus;
}

async function submitSevenDayCheckin() {
  const now = new Date();
  const suffix = `${now.getTime()}`;
  const event = {
    eventId: `evt_streak_checkin_${suffix}`,
    eventType: 'CHECKIN_SUCCESS',
    sourceSystem: 'app',
    tenantId,
    subject,
    biz: { bizType: 'CHECKIN', bizId: `streak_checkin_${suffix}` },
    occurredAt: now.toISOString(),
    attributes: {
      checkin: true,
      userLevel: 4,
      vipLevel: 2,
      channel: 'APP',
      activityScene: 'seven_day_checkin',
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

export function SevenDayCheckinPage() {
  const [status, setStatus] = useState<CheckinStatus | null>(null);
  const [apiResult, setAPIResult] = useState<APIResult>({ status: 'IDLE', message: '等待读取签到状态。' });
  const [running, setRunning] = useState(false);

  const completedDays = Math.max(0, Math.min(7, status?.cycleCompletedDays ?? (status?.signed ? status?.streakDays ?? 1 : (status?.todayIndex ?? 1) - 1)));
  const todayIndex = Math.max(1, Math.min(7, status?.todayIndex ?? completedDays + 1));
  const currentReward = rewards[Math.max(0, todayIndex - 1)] ?? rewards[0];
  const historyDates = useMemo(() => new Set((status?.history ?? []).filter((item) => item.signed).map((item) => String(item.date))), [status?.history]);

  const refreshStatus = async () => {
    setAPIResult((result) => ({ ...result, status: result.status === 'IDLE' ? 'LOADING' : result.status, message: result.status === 'IDLE' ? '正在读取 7 日签到状态。' : result.message }));
    const next = await fetchSevenDayStatus();
    setStatus(next);
    setAPIResult({
      status: 'IDLE',
      message: next.signed ? `今日已签到 · participation=${next.participationStatus || '-'} · grant=${next.grantStatus || '-'}` : '今日还未签到，可以发起测试。',
      eventId: next.eventId,
      idempotencyKey: next.idempotencyKey,
      response: next,
    });
  };

  useEffect(() => {
    let cancelled = false;
    fetchSevenDayStatus()
      .then((next) => {
        if (cancelled) return;
        setStatus(next);
        setAPIResult({
          status: 'IDLE',
          message: next.signed ? `今日已签到 · 连续 ${next.streakDays ?? 0} 天` : '今日还未签到，可以发起测试。',
          eventId: next.eventId,
          idempotencyKey: next.idempotencyKey,
          response: next,
        });
      })
      .catch((error) => {
        if (!cancelled) setAPIResult({ status: 'FAILED', message: (error as Error).message });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const runCheckin = async () => {
    if (running || status?.signed) return;
    setRunning(true);
    setAPIResult({ status: 'LOADING', message: '正在提交 CHECKIN_SUCCESS 到 /openapi/v1/events。' });
    try {
      const { response, body, event, idempotencyKey } = await submitSevenDayCheckin();
      const duplicate = response.status === 409 && Boolean((body as Record<string, unknown>).duplicate);
      const accepted = response.ok || duplicate;
      if (!accepted) {
        setAPIResult({
          status: response.status === 422 ? 'REJECTED' : 'FAILED',
          message: `${response.status} ${response.statusText || 'ERROR'} · ${String((body as Record<string, unknown>).message ?? (body as Record<string, unknown>).error ?? 'UNKNOWN')}`,
          eventId: event.eventId,
          idempotencyKey,
          response: body,
        });
        return;
      }
      await refreshStatus();
      setAPIResult({
        status: duplicate ? 'DUPLICATE' : 'ACCEPTED',
        message: duplicate ? '重复事件，已返回已有签到结果。' : `事件已接收 · ${compactResult(body)}`,
        eventId: event.eventId,
        idempotencyKey,
        response: body,
      });
    } catch (error) {
      setAPIResult({ status: 'FAILED', message: (error as Error).message });
    } finally {
      setRunning(false);
    }
  };

  return (
    <main className="streak-page">
      <section className="streak-shell">
        <header className="streak-hero">
          <nav>
            <a href={`/admin-flow/?activityId=${activityId}`}><ArrowLeft size={16} />流程画布</a>
            <button type="button" onClick={() => void refreshStatus()}><Clock3 size={16} />刷新状态</button>
          </nav>
          <div className="streak-hero-grid">
            <div>
              <span>7-Day Check-in Campaign</span>
              <h1>7 日连续签到</h1>
              <p>独立测试页，绑定活动 <b>{activityCode}</b>，用于验证连续天数、今日频控、奖励候选和发奖订单。</p>
            </div>
            <div className="streak-hero-card">
              <CalendarCheck2 size={28} />
              <strong>DAY {todayIndex}</strong>
              <span>{status?.signed ? '今日已完成' : '今日待签到'}</span>
            </div>
          </div>
        </header>

        <section className="streak-summary">
          <article>
            <Sparkles size={20} />
            <span>连续天数</span>
            <b>{status?.streakDays ?? completedDays}</b>
          </article>
          <article>
            <Trophy size={20} />
            <span>周期进度</span>
            <b>{completedDays}/7</b>
          </article>
          <article>
            <Gift size={20} />
            <span>今日奖励</span>
            <b>{currentReward.title}</b>
          </article>
          <article>
            <WalletCards size={20} />
            <span>发奖状态</span>
            <b>{status?.grantStatus || '-'}</b>
          </article>
        </section>

        <section className="streak-board">
          <div className="streak-board-head">
            <div>
              <span>Reward Calendar</span>
              <h2>7 天奖励日历</h2>
            </div>
            <button type="button" onClick={runCheckin} disabled={running || Boolean(status?.signed)}>
              {running ? <Loader2 className="spin" size={18} /> : <BadgeCheck size={18} />}
              {running ? '签到处理中' : status?.signed ? '今日已签到' : '立即签到'}
            </button>
          </div>
          <div className="streak-days">
            {rewards.map((reward) => {
              const done = reward.day <= completedDays;
              const active = reward.day === todayIndex && !status?.signed;
              return (
                <article key={reward.day} className={`${reward.tone} ${done ? 'done' : ''} ${active ? 'active' : ''}`}>
                  <span>DAY {reward.day}</span>
                  <b>{reward.title}</b>
                  <small>{reward.code}</small>
                  <i>{done || historyDates.size > 0 && historyDates.has(todayISO()) && reward.day <= completedDays ? <Check size={16} /> : reward.day}</i>
                </article>
              );
            })}
          </div>
        </section>

        <section className="streak-grid">
          <article className={`streak-result ${apiResult.status.toLowerCase()}`}>
            <div className="streak-section-title">
              <CircleDollarSign size={18} />
              <span>接口结果</span>
            </div>
            <dl>
              <div><dt>Status</dt><dd>{apiResult.status}</dd></div>
              <div><dt>Message</dt><dd>{apiResult.message}</dd></div>
              <div><dt>Event ID</dt><dd>{apiResult.eventId || '-'}</dd></div>
              <div><dt>Idempotency-Key</dt><dd>{apiResult.idempotencyKey || '-'}</dd></div>
            </dl>
            {apiResult.response ? <pre>{JSON.stringify(apiResult.response, null, 2)}</pre> : null}
          </article>

          <article className="streak-pipeline">
            <div className="streak-section-title">
              <ShieldCheck size={18} />
              <span>测试链路</span>
            </div>
            {pipeline.map(([title, description], index) => (
              <div key={title} className={index < completedDays || status?.signed ? 'done' : ''}>
                <i>{index + 1}</i>
                <span>{title}</span>
                <small>{description}</small>
              </div>
            ))}
          </article>
        </section>
      </section>
    </main>
  );
}
