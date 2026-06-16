package httpapi

import (
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type invitePageState struct {
	Token       string
	Available   bool
	Reason      string
	Workspace   marketing.Workspace
	Member      marketing.WorkspaceMember
	ExpiresText string
}

func (h *Handler) workspaceInvitePage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	state := invitePageState{Token: token, Available: true}
	member, err := h.engine.FindWorkspaceMemberByInviteToken(r.Context(), token)
	if err != nil {
		state.Available = false
		state.Reason = "邀请不存在"
	} else {
		state.Member = member
		workspace, workspaceErr := h.engine.GetWorkspace(r.Context(), member.WorkspaceID)
		if workspaceErr == nil {
			state.Workspace = workspace
		}
		switch {
		case member.Status != marketing.WorkspaceMemberInvited:
			state.Available = false
			state.Reason = "邀请已被接受或已失效"
		case member.InviteExpiresAt != nil && time.Now().After(*member.InviteExpiresAt):
			state.Available = false
			state.Reason = "邀请已过期"
		}
		if member.InviteExpiresAt != nil {
			state.ExpiresText = member.InviteExpiresAt.Local().Format("2006-01-02 15:04")
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := invitePageTemplate.Execute(w, state); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) acceptWorkspaceInviteByToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID      string `json:"userId"`
		DisplayName string `json:"displayName"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeStrictJSON(r.Body, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}
	member, err := h.engine.AcceptWorkspaceMemberInviteByToken(r.Context(), r.PathValue("token"), body.UserID, body.DisplayName)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

var invitePageTemplate = template.Must(template.New("invite").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ExMarketing Invite</title>
  <style>
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; background: #f6f8fb; color: #172033; font: 13px/1.45 Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    .invite-shell { min-height: 100vh; display: grid; grid-template-rows: 56px minmax(0, 1fr); }
    .invite-topbar { height: 56px; border-bottom: 1px solid #e4eaf2; background: rgba(255,255,255,.96); display: flex; align-items: center; justify-content: space-between; padding: 0 22px; }
    .invite-brand { display: flex; align-items: center; gap: 10px; font-weight: 950; font-size: 16px; }
    .invite-logo { width: 28px; height: 28px; border-radius: 7px; background: #155eef; color: #fff; display: grid; place-items: center; font-weight: 950; }
    .invite-main { width: min(620px, calc(100vw - 32px)); margin: 42px auto; }
    .invite-panel { border: 1px solid #e4eaf2; border-radius: 8px; background: #fff; box-shadow: 0 16px 40px rgba(16, 24, 40, .08); overflow: hidden; }
    .invite-header { border-bottom: 1px solid #edf1f7; padding: 24px; }
    .invite-header h1 { margin: 0; color: #101828; font-size: 24px; line-height: 1.2; letter-spacing: 0; }
    .invite-header p { margin: 8px 0 0; color: #667085; font-size: 13px; font-weight: 750; }
    .invite-content { padding: 22px 24px 24px; }
    .invite-meta { border: 1px solid #edf1f7; border-radius: 8px; background: #f8fafc; padding: 14px; display: grid; gap: 10px; margin-bottom: 18px; }
    .invite-meta div { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
    .invite-meta span { color: #667085; font-weight: 850; }
    .invite-meta strong { color: #101828; text-align: right; overflow-wrap: anywhere; }
    .invite-form { display: grid; gap: 10px; }
    input { height: 38px; border: 1px solid #d8dee8; border-radius: 6px; padding: 0 12px; font: inherit; }
    button { height: 40px; border: 1px solid #155eef; border-radius: 6px; background: #155eef; color: #fff; padding: 0 14px; font: inherit; font-weight: 900; cursor: pointer; }
    button:disabled { opacity: .6; cursor: default; }
    .invite-error { border: 1px solid #fecaca; background: #fff7f7; color: #b42318; border-radius: 8px; padding: 14px; font-weight: 850; }
    .invite-success { border: 1px solid #bbf7d0; background: #f0fdf4; color: #027a48; border-radius: 8px; padding: 14px; font-weight: 850; display: none; }
    .invite-muted { color: #667085; font-size: 12px; font-weight: 750; margin: 10px 0 0; }
  </style>
</head>
<body>
  <div class="invite-shell">
    <header class="invite-topbar">
      <div class="invite-brand"><div class="invite-logo">Ex</div><span>ExMarketing</span></div>
      <strong>Workspace Invite</strong>
    </header>
    <main class="invite-main">
      <section class="invite-panel">
        <header class="invite-header">
          <h1>{{if .Workspace.Name}}加入 {{.Workspace.Name}}{{else}}加入 ExMarketing workspace{{end}}</h1>
          <p>接受邀请后，你会以指定角色进入该 workspace。</p>
        </header>
        <div class="invite-content">
          {{if not .Available}}
            <div class="invite-error">{{.Reason}}</div>
          {{else}}
            <div class="invite-meta">
              <div><span>Workspace</span><strong>{{.Workspace.Name}}</strong></div>
              <div><span>邀请邮箱</span><strong>{{.Member.Email}}</strong></div>
              <div><span>角色</span><strong>{{.Member.Role}}</strong></div>
              {{if .ExpiresText}}<div><span>过期时间</span><strong>{{.ExpiresText}}</strong></div>{{end}}
            </div>
            <form id="invite-form" class="invite-form">
              <input id="display-name" type="text" placeholder="你的显示名称" value="{{.Member.DisplayName}}" autocomplete="name" />
              <input id="user-id" type="text" placeholder="用户 ID" value="{{if .Member.UserID}}{{.Member.UserID}}{{else}}{{.Member.Email}}{{end}}" autocomplete="username" />
              <button id="accept-button" type="submit">接受邀请</button>
            </form>
            <p id="invite-message" class="invite-muted">邀请会绑定到填写的用户 ID，未填写时使用受邀邮箱。</p>
            <div id="invite-success" class="invite-success">邀请已接受，可以返回 ExMarketing 工作台。</div>
          {{end}}
        </div>
      </section>
    </main>
  </div>
  <script>
    const token = "{{.Token}}";
    const form = document.getElementById('invite-form');
    const button = document.getElementById('accept-button');
    const message = document.getElementById('invite-message');
    const success = document.getElementById('invite-success');
    form?.addEventListener('submit', async (event) => {
      event.preventDefault();
      button.disabled = true;
      message.textContent = '正在接受邀请...';
      const response = await fetch('/invite/' + encodeURIComponent(token), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          displayName: document.getElementById('display-name').value,
          userId: document.getElementById('user-id').value
        })
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok) {
        message.textContent = body.error || '邀请不可用';
        button.disabled = false;
        return;
      }
      message.textContent = '';
      success.style.display = 'block';
      button.textContent = '已接受';
    });
  </script>
</body>
</html>`))
