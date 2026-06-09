package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type InviteEmailConfig struct {
	BaseURL            string
	Provider           string
	From               string
	Host               string
	Port               int
	Username           string
	Password           string
	TLS                bool
	SendGridAPIKey     string
	SendGridEndpoint   string
	SESRegion          string
	SESAccessKeyID     string
	SESSecretAccessKey string
	SESSessionToken    string
	SESEndpoint        string
	RetryAttempts      int
	RetryDelay         time.Duration
	Enabled            bool
}

type inviteEmailMessage struct {
	Workspace marketing.Workspace
	Member    marketing.WorkspaceMember
	InviteURL string
}

type inviteEmailDelivery struct {
	Status    string `json:"status"`
	Provider  string `json:"provider,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Message   string `json:"message,omitempty"`
	Attempts  int    `json:"attempts,omitempty"`
}

type inviteEmailSender interface {
	SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery
}

type noopInviteEmailSender struct {
	reason string
}

func (s noopInviteEmailSender) SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery {
	return inviteEmailDelivery{
		Status:    "skipped",
		Provider:  "none",
		Recipient: message.Member.Email,
		Message:   s.reason,
	}
}

type smtpInviteEmailSender struct {
	cfg InviteEmailConfig
}

type sendGridInviteEmailSender struct {
	cfg    InviteEmailConfig
	client *http.Client
}

type sesInviteEmailSender struct {
	cfg    InviteEmailConfig
	client *http.Client
}

type retryInviteEmailSender struct {
	inner    inviteEmailSender
	attempts int
	delay    time.Duration
}

func LoadInviteEmailConfigFromEnv() InviteEmailConfig {
	port := 587
	if raw := strings.TrimSpace(os.Getenv("EX_MARKETING_SMTP_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			port = parsed
		}
	}
	cfg := InviteEmailConfig{
		BaseURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("EX_MARKETING_PUBLIC_BASE_URL")), "/"),
		Provider:           strings.ToLower(strings.TrimSpace(os.Getenv("EX_MARKETING_INVITE_EMAIL_PROVIDER"))),
		From:               strings.TrimSpace(os.Getenv("EX_MARKETING_INVITE_EMAIL_FROM")),
		Host:               strings.TrimSpace(os.Getenv("EX_MARKETING_SMTP_HOST")),
		Port:               port,
		Username:           strings.TrimSpace(os.Getenv("EX_MARKETING_SMTP_USERNAME")),
		Password:           os.Getenv("EX_MARKETING_SMTP_PASSWORD"),
		TLS:                !strings.EqualFold(strings.TrimSpace(os.Getenv("EX_MARKETING_SMTP_TLS")), "false"),
		SendGridAPIKey:     strings.TrimSpace(os.Getenv("EX_MARKETING_SENDGRID_API_KEY")),
		SendGridEndpoint:   strings.TrimSpace(os.Getenv("EX_MARKETING_SENDGRID_ENDPOINT")),
		SESRegion:          strings.TrimSpace(os.Getenv("EX_MARKETING_SES_REGION")),
		SESAccessKeyID:     strings.TrimSpace(os.Getenv("EX_MARKETING_SES_ACCESS_KEY_ID")),
		SESSecretAccessKey: os.Getenv("EX_MARKETING_SES_SECRET_ACCESS_KEY"),
		SESSessionToken:    strings.TrimSpace(os.Getenv("EX_MARKETING_SES_SESSION_TOKEN")),
		SESEndpoint:        strings.TrimSpace(os.Getenv("EX_MARKETING_SES_ENDPOINT")),
		RetryAttempts:      3,
		RetryDelay:         time.Second,
	}
	if cfg.SendGridEndpoint == "" {
		cfg.SendGridEndpoint = "https://api.sendgrid.com/v3/mail/send"
	}
	if cfg.SESRegion == "" {
		cfg.SESRegion = "us-east-1"
	}
	if raw := strings.TrimSpace(os.Getenv("EX_MARKETING_INVITE_EMAIL_RETRY_ATTEMPTS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			cfg.RetryAttempts = parsed
		}
	}
	if raw := strings.TrimSpace(os.Getenv("EX_MARKETING_INVITE_EMAIL_RETRY_DELAY_MS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			cfg.RetryDelay = time.Duration(parsed) * time.Millisecond
		}
	}
	if cfg.Provider == "" {
		switch {
		case cfg.SendGridAPIKey != "":
			cfg.Provider = "sendgrid"
		case cfg.SESAccessKeyID != "" || cfg.SESSecretAccessKey != "":
			cfg.Provider = "ses"
		default:
			cfg.Provider = "smtp"
		}
	}
	switch cfg.Provider {
	case "sendgrid":
		cfg.Enabled = cfg.From != "" && cfg.SendGridAPIKey != ""
	case "ses":
		cfg.Enabled = cfg.From != "" && cfg.SESAccessKeyID != "" && cfg.SESSecretAccessKey != "" && cfg.SESRegion != ""
	default:
		cfg.Provider = "smtp"
		cfg.Enabled = cfg.From != "" && cfg.Host != ""
	}
	return cfg
}

func newInviteEmailSender(cfg InviteEmailConfig) inviteEmailSender {
	if !cfg.Enabled {
		return noopInviteEmailSender{reason: "invite email provider not configured; set EX_MARKETING_INVITE_EMAIL_PROVIDER with SMTP, SendGrid, or SES credentials"}
	}
	var sender inviteEmailSender
	switch cfg.Provider {
	case "sendgrid":
		sender = sendGridInviteEmailSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
	case "ses":
		sender = sesInviteEmailSender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
	default:
		sender = smtpInviteEmailSender{cfg: cfg}
	}
	return retryInviteEmailSender{inner: sender, attempts: cfg.RetryAttempts, delay: cfg.RetryDelay}
}

func (s retryInviteEmailSender) SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery {
	attempts := s.attempts
	if attempts <= 0 {
		attempts = 1
	}
	var last inviteEmailDelivery
	for attempt := 1; attempt <= attempts; attempt++ {
		last = s.inner.SendWorkspaceInvite(ctx, message)
		last.Attempts = attempt
		if last.Status != "failed" {
			return last
		}
		if attempt == attempts {
			return last
		}
		if s.delay > 0 {
			timer := time.NewTimer(s.delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				last.Message = ctx.Err().Error()
				return last
			case <-timer.C:
			}
		}
	}
	return last
}

func (s smtpInviteEmailSender) SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery {
	if strings.TrimSpace(message.Member.Email) == "" {
		return inviteEmailDelivery{Status: "skipped", Provider: "smtp", Message: "member email is empty"}
	}
	subject := fmt.Sprintf("你被邀请加入 ExMarketing workspace：%s", message.Workspace.Name)
	htmlBody, textBody, err := renderInviteEmailBody(message)
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "smtp", Recipient: message.Member.Email, Message: err.Error()}
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.send(message.Member.Email, subject, htmlBody, textBody)
	}()
	select {
	case <-ctx.Done():
		return inviteEmailDelivery{Status: "failed", Provider: "smtp", Recipient: message.Member.Email, Message: ctx.Err().Error()}
	case err := <-errCh:
		if err != nil {
			return inviteEmailDelivery{Status: "failed", Provider: "smtp", Recipient: message.Member.Email, Message: err.Error()}
		}
		return inviteEmailDelivery{Status: "sent", Provider: "smtp", Recipient: message.Member.Email}
	case <-time.After(10 * time.Second):
		return inviteEmailDelivery{Status: "failed", Provider: "smtp", Recipient: message.Member.Email, Message: "SMTP send timeout"}
	}
}

func (s sendGridInviteEmailSender) SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery {
	if strings.TrimSpace(message.Member.Email) == "" {
		return inviteEmailDelivery{Status: "skipped", Provider: "sendgrid", Message: "member email is empty"}
	}
	subject := fmt.Sprintf("你被邀请加入 ExMarketing workspace：%s", message.Workspace.Name)
	htmlBody, textBody, err := renderInviteEmailBody(message)
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "sendgrid", Recipient: message.Member.Email, Message: err.Error()}
	}
	payload := map[string]any{
		"personalizations": []map[string]any{{
			"to":      []map[string]string{{"email": message.Member.Email}},
			"subject": subject,
		}},
		"from": map[string]string{"email": s.cfg.From, "name": "ExMarketing"},
		"content": []map[string]string{
			{"type": "text/plain", "value": textBody},
			{"type": "text/html", "value": htmlBody},
		},
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimSpace(s.cfg.SendGridEndpoint)
	if endpoint == "" {
		endpoint = "https://api.sendgrid.com/v3/mail/send"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "sendgrid", Recipient: message.Member.Email, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.SendGridAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "sendgrid", Recipient: message.Member.Email, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return inviteEmailDelivery{Status: "failed", Provider: "sendgrid", Recipient: message.Member.Email, Message: fmt.Sprintf("SendGrid status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))}
	}
	return inviteEmailDelivery{Status: "sent", Provider: "sendgrid", Recipient: message.Member.Email}
}

func (s sesInviteEmailSender) SendWorkspaceInvite(ctx context.Context, message inviteEmailMessage) inviteEmailDelivery {
	if strings.TrimSpace(message.Member.Email) == "" {
		return inviteEmailDelivery{Status: "skipped", Provider: "ses", Message: "member email is empty"}
	}
	subject := fmt.Sprintf("你被邀请加入 ExMarketing workspace：%s", message.Workspace.Name)
	htmlBody, textBody, err := renderInviteEmailBody(message)
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "ses", Recipient: message.Member.Email, Message: err.Error()}
	}
	payload := map[string]any{
		"FromEmailAddress": s.cfg.From,
		"Destination":      map[string]any{"ToAddresses": []string{message.Member.Email}},
		"Content": map[string]any{"Simple": map[string]any{
			"Subject": map[string]string{"Data": subject, "Charset": "UTF-8"},
			"Body": map[string]any{
				"Text": map[string]string{"Data": textBody, "Charset": "UTF-8"},
				"Html": map[string]string{"Data": htmlBody, "Charset": "UTF-8"},
			},
		}},
	}
	body, _ := json.Marshal(payload)
	endpoint := strings.TrimSpace(s.cfg.SESEndpoint)
	if endpoint == "" {
		endpoint = "https://email." + s.cfg.SESRegion + ".amazonaws.com/v2/email/outbound-emails"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "ses", Recipient: message.Member.Email, Message: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if err := signSESRequest(req, body, s.cfg); err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "ses", Recipient: message.Member.Email, Message: err.Error()}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return inviteEmailDelivery{Status: "failed", Provider: "ses", Recipient: message.Member.Email, Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return inviteEmailDelivery{Status: "failed", Provider: "ses", Recipient: message.Member.Email, Message: fmt.Sprintf("SES status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))}
	}
	return inviteEmailDelivery{Status: "sent", Provider: "ses", Recipient: message.Member.Email}
}

func (s smtpInviteEmailSender) send(to string, subject string, htmlBody string, textBody string) error {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}
	var body bytes.Buffer
	boundary := "exmarketing-invite-boundary"
	body.WriteString("From: " + s.cfg.From + "\r\n")
	body.WriteString("To: " + to + "\r\n")
	body.WriteString("Subject: " + subject + "\r\n")
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	body.WriteString(textBody + "\r\n\r\n")
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	body.WriteString(htmlBody + "\r\n\r\n")
	body.WriteString("--" + boundary + "--\r\n")
	if !s.cfg.TLS {
		return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, body.Bytes())
	}
	var client *smtp.Client
	if s.cfg.Port == 465 {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		defer conn.Close()
		client, err = smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			return err
		}
	} else {
		var err error
		client, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = client.Close()
			return err
		}
	}
	defer client.Close()
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body.Bytes()); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func signSESRequest(req *http.Request, payload []byte, cfg InviteEmailConfig) error {
	if strings.TrimSpace(cfg.SESAccessKeyID) == "" || strings.TrimSpace(cfg.SESSecretAccessKey) == "" {
		return fmt.Errorf("SES credentials are required")
	}
	region := strings.TrimSpace(cfg.SESRegion)
	if region == "" {
		region = "us-east-1"
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	if cfg.SESSessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", cfg.SESSessionToken)
	}
	payloadHash := inviteSHA256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	host := req.URL.Host
	req.Header.Set("Host", host)
	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req.URL)
	canonicalHeaders := "content-type:" + strings.TrimSpace(req.Header.Get("Content-Type")) + "\n" +
		"host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"
	if cfg.SESSessionToken != "" {
		canonicalHeaders += "x-amz-security-token:" + cfg.SESSessionToken + "\n"
		signedHeaders += ";x-amz-security-token"
	}
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	credentialScope := dateStamp + "/" + region + "/ses/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + inviteSHA256Hex([]byte(canonicalRequest))
	signature := hex.EncodeToString(inviteHMACSHA256(inviteAWSSigningKey(cfg.SESSecretAccessKey, dateStamp, region, "ses"), []byte(stringToSign)))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+cfg.SESAccessKeyID+"/"+credentialScope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return nil
}

func canonicalQueryString(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Query().Encode()
}

func inviteSHA256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func inviteHMACSHA256(key []byte, value []byte) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(value)
	return mac.Sum(nil)
}

func inviteAWSSigningKey(secret string, date string, region string, service string) []byte {
	kDate := inviteHMACSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := inviteHMACSHA256(kDate, []byte(region))
	kService := inviteHMACSHA256(kRegion, []byte(service))
	return inviteHMACSHA256(kService, []byte("aws4_request"))
}

func renderInviteEmailBody(message inviteEmailMessage) (string, string, error) {
	var htmlBody bytes.Buffer
	if err := inviteEmailTemplate.Execute(&htmlBody, message); err != nil {
		return "", "", err
	}
	expiresText := ""
	if message.Member.InviteExpiresAt != nil {
		expiresText = "\n有效期至：" + message.Member.InviteExpiresAt.Local().Format("2006-01-02 15:04")
	}
	textBody := fmt.Sprintf("你被邀请加入 ExMarketing workspace：%s\n角色：%s%s\n访问邀请：%s\n如果你不认识邀请者，可以忽略此邮件。\n", message.Workspace.Name, message.Member.Role, expiresText, message.InviteURL)
	return htmlBody.String(), textBody, nil
}

var inviteEmailTemplate = template.Must(template.New("invite-email").Parse(`<!doctype html>
<html lang="zh-CN">
<body style="margin:0;background:#f6f8fb;font-family:Inter,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#172033;">
  <div style="max-width:560px;margin:0 auto;padding:28px 16px;">
    <div style="background:#fff;border:1px solid #e4eaf2;border-radius:8px;overflow:hidden;">
      <div style="padding:20px 22px;border-bottom:1px solid #edf1f7;">
        <strong style="font-size:18px;">ExMarketing</strong>
      </div>
      <div style="padding:22px;">
        <h1 style="margin:0 0 10px;font-size:22px;line-height:1.25;">加入 {{.Workspace.Name}}</h1>
        <p style="margin:0 0 16px;color:#667085;">你被邀请成为 <strong>{{.Member.Role}}</strong>。</p>
        {{if .Member.InviteExpiresAt}}<p style="margin:0 0 16px;color:#667085;">邀请有效期至 {{.Member.InviteExpiresAt.Local.Format "2006-01-02 15:04"}}。</p>{{end}}
        <a href="{{.InviteURL}}" style="display:inline-block;background:#155eef;color:#fff;text-decoration:none;font-weight:800;border-radius:6px;padding:10px 14px;">打开邀请</a>
        <p style="margin:18px 0 0;color:#667085;font-size:12px;">如果按钮不可用，请复制此链接：{{.InviteURL}}</p>
        <p style="margin:12px 0 0;color:#98a2b3;font-size:12px;">如果你不认识邀请者，可以忽略此邮件。</p>
      </div>
    </div>
  </div>
</body>
</html>`))
