package httpapi

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type sharePageState struct {
	Token            string
	Available        bool
	Reason           string
	PasswordRequired bool
	Scope            string
	Permission       string
}

func (h *Handler) shareLinkPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	state := sharePageState{Token: token, Available: true}
	links, err := h.engine.ListWorkspaceShareLinks(r.Context(), marketing.WorkspaceShareLinkQuery{Token: token, Limit: 1})
	if err != nil || len(links) == 0 {
		state.Available = false
		state.Reason = "链接不存在"
	} else {
		link := links[0]
		state.Scope = string(link.Scope)
		state.Permission = string(link.Permission)
		state.PasswordRequired = strings.TrimSpace(link.PasswordHash) != ""
		switch {
		case !link.Enabled:
			state.Available = false
			if strings.TrimSpace(link.DisabledReason) != "" {
				state.Reason = "链接已停用：" + link.DisabledReason
			} else {
				state.Reason = "链接已停用"
			}
		case link.ExpiresAt != nil && time.Now().After(*link.ExpiresAt):
			state.Available = false
			state.Reason = "链接已过期"
		case link.MaxAccessCount > 0 && link.AccessCount >= link.MaxAccessCount:
			state.Available = false
			state.Reason = "访问次数已用完"
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sharePageTemplate.Execute(w, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var sharePageTemplate = template.Must(template.New("share").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ExMarketing Share</title>
  <style>
    * { box-sizing: border-box; }
    html { -webkit-text-size-adjust: 100%; }
    body { margin: 0; min-height: 100vh; background: #f4f7fb; color: #172033; font: 13px/1.45 Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    .share-shell { min-height: 100vh; display: grid; grid-template-rows: 56px minmax(0, 1fr); }
    .share-topbar { height: 56px; border-bottom: 1px solid #e4eaf2; background: rgba(255,255,255,.96); display: flex; align-items: center; justify-content: space-between; padding: 0 22px; position: sticky; top: 0; z-index: 2; backdrop-filter: blur(10px); }
    .share-brand { display: flex; align-items: center; gap: 10px; font-weight: 950; font-size: 16px; }
    .share-logo { width: 28px; height: 28px; border-radius: 7px; background: #155eef; color: #fff; display: grid; place-items: center; font-weight: 950; box-shadow: 0 8px 18px rgba(21, 94, 239, .22); }
    .share-badge { border: 1px solid #d8e2f0; border-radius: 999px; color: #475467; padding: 5px 10px; font-size: 12px; font-weight: 850; }
    .share-main { width: min(1080px, calc(100vw - 32px)); margin: 28px auto; }
    .share-panel { border: 1px solid #e4eaf2; border-radius: 8px; background: #fff; box-shadow: 0 16px 40px rgba(16, 24, 40, .08); overflow: hidden; }
    .share-header { border-bottom: 1px solid #edf1f7; padding: 22px 24px; display: flex; align-items: center; justify-content: space-between; gap: 16px; background: linear-gradient(180deg, #ffffff 0%, #fbfcff 100%); }
    .share-header h1 { margin: 0; color: #101828; font-size: 22px; line-height: 1.2; letter-spacing: 0; }
    .share-header p { margin: 6px 0 0; color: #667085; font-size: 13px; font-weight: 700; }
    .share-content { padding: 20px 24px 24px; }
    .share-password { display: flex; gap: 10px; max-width: 520px; align-items: flex-start; }
    .password-field { flex: 1; min-width: 0; }
    input { width: 100%; height: 40px; border: 1px solid #d8dee8; border-radius: 6px; padding: 0 12px; font: inherit; outline: none; background: #fff; }
    input:focus { border-color: #155eef; box-shadow: 0 0 0 3px rgba(21, 94, 239, .12); }
    button { height: 40px; border: 1px solid #155eef; border-radius: 6px; background: #155eef; color: #fff; padding: 0 14px; font: inherit; font-weight: 900; cursor: pointer; white-space: nowrap; }
    button.secondary { border-color: #d8dee8; background: #fff; color: #344054; }
    .share-error { border: 1px solid #fecaca; background: #fff7f7; color: #b42318; border-radius: 8px; padding: 14px; font-weight: 800; }
    .share-error strong { display: block; color: #7a271a; font-size: 15px; margin-bottom: 4px; }
    .share-error p { margin: 0; color: #b42318; font-weight: 750; }
    .share-inline-error { min-height: 18px; margin: 6px 0 0; color: #b42318; font-size: 12px; font-weight: 850; }
    .share-muted { color: #667085; font-weight: 750; }
    .share-stats { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin: 16px 0; }
    .share-stat { border: 1px solid #edf1f7; border-radius: 8px; padding: 12px; background: #f8fafc; }
    .share-stat span { display: block; color: #667085; font-size: 11px; font-weight: 900; text-transform: uppercase; }
    .share-stat b { display: block; margin-top: 4px; color: #101828; font-size: 18px; }
    .share-table { border: 1px solid #e4eaf2; border-radius: 8px; overflow: hidden; margin-top: 16px; }
    .share-row { min-height: 42px; display: grid; grid-template-columns: minmax(180px, 1fr) 110px 100px 140px; align-items: center; gap: 10px; padding: 0 12px; border-bottom: 1px solid #edf1f7; }
    .share-row:last-child { border-bottom: 0; }
    .share-row.head { min-height: 34px; background: #f8fafc; color: #667085; font-size: 11px; font-weight: 950; text-transform: uppercase; }
    .share-row strong { color: #172033; }
    .share-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .share-empty { border: 1px dashed #d8e2f0; border-radius: 8px; background: #fbfcff; color: #667085; padding: 16px; font-weight: 800; }
    .audit-card { margin-top: 18px; border: 1px solid #e4eaf2; border-radius: 8px; background: #fff; overflow: hidden; }
    .audit-card h2 { margin: 0; padding: 12px 14px; border-bottom: 1px solid #edf1f7; color: #101828; font-size: 13px; }
    .audit-row { display: grid; grid-template-columns: 130px 1fr 92px; gap: 10px; padding: 10px 14px; border-bottom: 1px solid #edf1f7; color: #475467; font-weight: 750; }
    .audit-row:last-child { border-bottom: 0; }
    .audit-row b { color: #172033; }
    .audit-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    pre { max-height: 420px; overflow: auto; border: 1px solid #edf1f7; border-radius: 8px; background: #101828; color: #e4eaf2; padding: 14px; font-size: 12px; }
    @media (max-width: 720px) {
      body { background: #fff; }
      .share-shell { grid-template-rows: 52px minmax(0, 1fr); }
      .share-topbar { height: 52px; padding: 0 14px; }
      .share-brand span { display: none; }
      .share-badge { max-width: 62vw; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .share-main { width: 100%; margin: 0; }
      .share-panel { min-height: calc(100vh - 52px); border: 0; border-radius: 0; box-shadow: none; }
      .share-header { align-items: flex-start; flex-direction: column; padding: 18px 16px; }
      .share-header h1 { font-size: 20px; overflow-wrap: anywhere; }
      .share-content { padding: 16px; }
      .share-stats { grid-template-columns: 1fr; }
      .share-row { grid-template-columns: minmax(0, 1fr); align-items: flex-start; padding: 10px 12px; }
      .share-row.head { display: none; }
      .share-row span { white-space: normal; }
      .share-password { flex-direction: column; max-width: none; }
      .share-password button { width: 100%; }
      .password-field { width: 100%; }
      .audit-row { grid-template-columns: minmax(0, 1fr); }
      .audit-row span { white-space: normal; }
    }
  </style>
</head>
<body>
  <div class="share-shell">
    <header class="share-topbar">
      <div class="share-brand"><div class="share-logo">Ex</div><span>ExMarketing</span></div>
      <div class="share-badge">{{if .Scope}}{{.Scope}} · {{.Permission}}{{else}}Share{{end}}</div>
    </header>
    <main class="share-main">
      <section class="share-panel">
        <header class="share-header">
          <div>
            <h1 id="share-title">分享访问</h1>
            <p id="share-subtitle">只读浏览已分享的 workspace、folder 或 workflow。</p>
          </div>
          <button class="secondary" type="button" onclick="location.reload()">刷新</button>
        </header>
        <div class="share-content">
          {{if not .Available}}
            <div class="share-error">
              <strong>{{.Reason}}</strong>
              <p>请联系分享者确认链接状态、有效期或访问权限。</p>
            </div>
          {{else if .PasswordRequired}}
            <form id="password-form" class="share-password">
              <div class="password-field">
                <input id="password" type="password" placeholder="输入访问密码" autocomplete="current-password" />
                <p id="password-error" class="share-inline-error"></p>
              </div>
              <button type="submit">访问</button>
            </form>
            <p id="message" class="share-muted">该链接需要密码。</p>
            <div id="share-data"></div>
          {{else}}
            <p id="message" class="share-muted">正在加载分享内容...</p>
            <div id="share-data"></div>
          {{end}}
        </div>
      </section>
    </main>
  </div>
  <script>
    const token = "{{.Token}}";
    const available = {{if .Available}}true{{else}}false{{end}};
    const passwordRequired = {{if .PasswordRequired}}true{{else}}false{{end}};
    const messageEl = document.getElementById('message');
    const dataEl = document.getElementById('share-data');
    const titleEl = document.getElementById('share-title');
    const subtitleEl = document.getElementById('share-subtitle');
    const passwordErrorEl = document.getElementById('password-error');

    async function resolveShare(password) {
      if (!available) return;
      messageEl.textContent = '正在校验链接...';
      if (passwordErrorEl) passwordErrorEl.textContent = '';
      const response = await fetch('/share/' + encodeURIComponent(token), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ password: password || '' })
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) {
        renderShareError(response.status, body);
        return;
      }
      renderShare(body);
    }

    function renderShareError(status, body) {
      const code = body.code || body.errorCode || '';
      const text = body.message || body.error || '链接不可用';
      if (code === 'SHARE_PASSWORD_INVALID' || code === 'SHARE_PASSWORD_REQUIRED') {
        if (passwordErrorEl) passwordErrorEl.textContent = code === 'SHARE_PASSWORD_INVALID' ? '密码错误，请重新输入。' : '请输入访问密码。';
        messageEl.textContent = '密码校验未通过。';
        return;
      }
      const title = code === 'SHARE_EXPIRED' ? '链接已过期' :
        code === 'SHARE_DISABLED' ? '链接已停用' :
        code === 'SHARE_ACCESS_LIMIT' ? '访问次数已用完' :
        code === 'SHARE_FORBIDDEN' ? '无权限访问' :
        code === 'SHARE_NOT_FOUND' || status === 404 ? '链接不存在' : '链接不可用';
      messageEl.textContent = '';
      titleEl.textContent = title;
      subtitleEl.textContent = text;
      dataEl.innerHTML = '<div class="share-error"><strong>' + escapeHtml(title) + '</strong><p>' + escapeHtml(text) + '</p></div>';
    }

    function renderShare(body) {
      const link = body.link || {};
      const target = body.target || {};
      const items = body.items || [];
      const audits = body.audits || [];
      titleEl.textContent = target.name || target.activityName || target.activityCode || '分享内容';
      subtitleEl.textContent = (link.scope || 'SHARE') + ' · ' + (link.permission || 'VIEW') + ' · 已访问 ' + (link.accessCount || 0) + (link.maxAccessCount ? '/' + link.maxAccessCount : '');
      messageEl.textContent = '';
      const stats = body.stats || {};
      let html = '<div class="share-stats">' +
        '<div class="share-stat"><span>Workflows</span><b>' + (stats.workflows ?? (target.activityCode ? 1 : 0)) + '</b></div>' +
        '<div class="share-stat"><span>Folders</span><b>' + (stats.folders ?? 0) + '</b></div>' +
        '<div class="share-stat"><span>Members</span><b>' + (stats.members ?? 0) + '</b></div>' +
      '</div>';
      if (items.length) {
        html += '<div class="share-table"><div class="share-row head"><span>Name</span><span>Type</span><span>Status</span><span>Visibility</span></div>';
        html += items.map((item) => '<div class="share-row"><strong>' + escapeHtml(item.name || item.activityName || item.activityCode || item.id) + '</strong><span>' + escapeHtml(item.type || item.activityType || '-') + '</span><span>' + escapeHtml(item.status || '-') + '</span><span>' + escapeHtml(item.visibility || '-') + '</span></div>').join('');
        html += '</div>';
      } else {
        html += '<pre>' + escapeHtml(JSON.stringify(target, null, 2)) + '</pre>';
      }
      html += renderAudits(audits);
      dataEl.innerHTML = html;
    }

    function renderAudits(audits) {
      let html = '<section class="audit-card"><h2>访问审计</h2>';
      if (!audits.length) {
        return html + '<div class="share-empty">暂无访问审计记录。</div></section>';
      }
      html += audits.map((audit) => {
        const time = audit.createdAt ? new Date(audit.createdAt).toLocaleString() : '-';
        const actor = audit.actorLabel || 'Anonymous visitor';
        const agent = audit.source || 'Share page';
        return '<div class="audit-row"><span>' + escapeHtml(time) + '</span><b>' + escapeHtml(actor) + '</b><span>' + escapeHtml(agent) + '</span></div>';
      }).join('');
      return html + '</section>';
    }

    function escapeHtml(value) {
      return String(value).replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch]));
    }

    document.getElementById('password-form')?.addEventListener('submit', (event) => {
      event.preventDefault();
      resolveShare(document.getElementById('password').value);
    });
    if (available && !passwordRequired) resolveShare('');
  </script>
</body>
</html>`))
