package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/engine"
	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/workflow"
)

func createTestWorkspace(t *testing.T, handler *Handler, actor string) marketing.Workspace {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
		"tenantId": "site_001",
		"name": "Storage Test Workspace",
		"slug": "storage-test-`+strconv.FormatInt(time.Now().UnixNano(), 36)+`",
		"visibility": "TEAM",
		"defaultRole": "VIEWER",
		"ownerId": "`+actor+`",
		"createdBy": "`+actor+`"
	}`))
	req.Header.Set("X-Actor-Id", actor)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", rec.Code, rec.Body.String())
	}
	var workspace marketing.Workspace
	if err := json.NewDecoder(rec.Body).Decode(&workspace); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	return workspace
}

func uploadWorkspaceFileForTest(t *testing.T, handler *Handler, actor string, fields map[string]string, filename string, size int) marketing.WorkspaceFileObject {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := io.CopyN(part, bytes.NewReader(bytes.Repeat([]byte("x"), size)), int64(size)); err != nil {
		t.Fatalf("write upload bytes: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/workspace-files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Actor-Id", actor)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload file status=%d body=%s", rec.Code, rec.Body.String())
	}
	var file marketing.WorkspaceFileObject
	if err := json.NewDecoder(rec.Body).Decode(&file); err != nil {
		t.Fatalf("decode file: %v", err)
	}
	return file
}

func TestSharePageStatesAndAccessAudit(t *testing.T) {
	handler := newTestHandler()
	routes := handler.Routes()
	workspace := createTestWorkspace(t, handler, "owner")

	createShare := func(body string) marketing.WorkspaceShareLink {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/share-links", strings.NewReader(body))
		req.Header.Set("X-Actor-Id", "owner")
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create share failed: %d %s", rec.Code, rec.Body.String())
		}
		var link marketing.WorkspaceShareLink
		if err := json.NewDecoder(rec.Body).Decode(&link); err != nil {
			t.Fatalf("decode share link: %v", err)
		}
		return link
	}

	protected := createShare(`{
		"scope": "WORKSPACE",
		"permission": "VIEW",
		"passwordHash": "open-sesame",
		"createdBy": "owner"
	}`)

	page := httptest.NewRecorder()
	routes.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/share/"+protected.Token, nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `id="password-error"`) || !strings.Contains(page.Body.String(), "访问审计") {
		t.Fatalf("share page missing password/audit UI: %d %s", page.Code, page.Body.String())
	}

	wrongPassword := httptest.NewRecorder()
	routes.ServeHTTP(wrongPassword, httptest.NewRequest(http.MethodPost, "/share/"+protected.Token, strings.NewReader(`{"password":"bad"}`)))
	if wrongPassword.Code != http.StatusUnauthorized || !strings.Contains(wrongPassword.Body.String(), `"code":"SHARE_PASSWORD_INVALID"`) {
		t.Fatalf("expected password invalid response, got %d %s", wrongPassword.Code, wrongPassword.Body.String())
	}

	resolve := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/share/"+protected.Token, strings.NewReader(`{"password":"open-sesame"}`))
	req.Header.Set("X-Actor-Id", "viewer")
	req.Header.Set("User-Agent", "ShareE2E/1.0")
	routes.ServeHTTP(resolve, req)
	if resolve.Code != http.StatusOK {
		t.Fatalf("resolve share failed: %d %s", resolve.Code, resolve.Body.String())
	}
	var payload struct {
		Link   map[string]any   `json:"link"`
		Audits []map[string]any `json:"audits"`
	}
	if err := json.NewDecoder(resolve.Body).Decode(&payload); err != nil {
		t.Fatalf("decode resolve payload: %v", err)
	}
	if payload.Link["scope"] != string(marketing.WorkspaceShareWorkspace) {
		t.Fatalf("expected public link scope in resolve payload, got %+v", payload.Link)
	}
	if _, ok := payload.Link["passwordHash"]; ok {
		t.Fatalf("resolve payload leaked passwordHash: %+v", payload.Link)
	}
	if _, ok := payload.Link["token"]; ok {
		t.Fatalf("resolve payload leaked token: %+v", payload.Link)
	}
	if len(payload.Audits) == 0 || payload.Audits[0]["createdAt"] == "" || payload.Audits[0]["actorLabel"] == "" {
		t.Fatalf("expected public access audit summary, got %+v", payload.Audits)
	}
	if _, ok := payload.Audits[0]["actorId"]; ok {
		t.Fatalf("share audit payload leaked actorId: %+v", payload.Audits[0])
	}
	if _, ok := payload.Audits[0]["userAgent"]; ok {
		t.Fatalf("share audit payload leaked userAgent: %+v", payload.Audits[0])
	}
	if _, ok := payload.Audits[0]["ip"]; ok {
		t.Fatalf("share audit payload leaked ip: %+v", payload.Audits[0])
	}

	expiredAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	expired := createShare(`{
		"scope": "WORKSPACE",
		"permission": "VIEW",
		"expiresAt": "` + expiredAt + `",
		"createdBy": "owner"
	}`)
	expiredResolve := httptest.NewRecorder()
	routes.ServeHTTP(expiredResolve, httptest.NewRequest(http.MethodPost, "/share/"+expired.Token, strings.NewReader(`{}`)))
	if expiredResolve.Code != http.StatusGone || !strings.Contains(expiredResolve.Body.String(), `"code":"SHARE_EXPIRED"`) {
		t.Fatalf("expected expired response, got %d %s", expiredResolve.Code, expiredResolve.Body.String())
	}
}

func TestWorkspaceInviteEmailSendGridRetryAndAudit(t *testing.T) {
	attempts := 0
	sendgrid := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer sg-test-key" {
			t.Fatalf("unexpected sendgrid request method/auth: %s %s", r.Method, r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode sendgrid payload: %v", err)
		}
		if !strings.Contains(fmt.Sprint(payload), "invitee@example.com") || !strings.Contains(fmt.Sprint(payload), "/invite/") {
			t.Fatalf("sendgrid payload missing recipient/invite link: %+v", payload)
		}
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary failure token=raw-secret"))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer sendgrid.Close()

	t.Setenv("EX_MARKETING_INVITE_EMAIL_PROVIDER", "sendgrid")
	t.Setenv("EX_MARKETING_INVITE_EMAIL_FROM", "noreply@example.com")
	t.Setenv("EX_MARKETING_SENDGRID_API_KEY", "sg-test-key")
	t.Setenv("EX_MARKETING_SENDGRID_ENDPOINT", sendgrid.URL)
	t.Setenv("EX_MARKETING_INVITE_EMAIL_RETRY_ATTEMPTS", "2")
	t.Setenv("EX_MARKETING_INVITE_EMAIL_RETRY_DELAY_MS", "0")
	t.Setenv("EX_MARKETING_PUBLIC_BASE_URL", "http://127.0.0.1:5173")

	handler := newTestHandler()
	workspace := createTestWorkspace(t, handler, "owner")
	memberReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/members", strings.NewReader(`{
		"email": "invitee@example.com",
		"displayName": "Invitee",
		"role": "EDITOR",
		"status": "INVITED",
		"invitedBy": "owner"
	}`))
	memberReq.Header.Set("X-Actor-Id", "owner")
	memberRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(memberRec, memberReq)
	if memberRec.Code != http.StatusCreated {
		t.Fatalf("create invited member failed: %d %s", memberRec.Code, memberRec.Body.String())
	}
	var response workspaceMemberInviteResponse
	if err := json.NewDecoder(memberRec.Body).Decode(&response); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	if response.EmailDelivery.Provider != "sendgrid" || response.EmailDelivery.Status != "sent" || response.EmailDelivery.Attempts != 2 {
		t.Fatalf("unexpected delivery: %+v", response.EmailDelivery)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/audit-logs?action=SEND_MEMBER_INVITE_EMAIL&limit=10", nil)
	auditReq.Header.Set("X-Actor-Id", "owner")
	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK || !strings.Contains(auditRec.Body.String(), `"SEND_MEMBER_INVITE_EMAIL"`) || !strings.Contains(auditRec.Body.String(), `"sendgrid"`) {
		t.Fatalf("invite send audit missing: %d %s", auditRec.Code, auditRec.Body.String())
	}
}

func TestWorkspaceInvitePageIgnoresClientSuppliedUserID(t *testing.T) {
	handler := newTestHandler()
	workspace := createTestWorkspace(t, handler, "owner")
	memberReq := httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/members", strings.NewReader(`{
		"email": "invitee-bind@example.com",
		"displayName": "",
		"role": "VIEWER",
		"status": "INVITED",
		"invitedBy": "owner"
	}`))
	memberReq.Header.Set("X-Actor-Id", "owner")
	memberRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(memberRec, memberReq)
	if memberRec.Code != http.StatusCreated {
		t.Fatalf("create invited member failed: %d %s", memberRec.Code, memberRec.Body.String())
	}
	var response workspaceMemberInviteResponse
	if err := json.NewDecoder(memberRec.Body).Decode(&response); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/invite/"+response.InviteToken, strings.NewReader(`{
		"userId": "attacker-controlled-user",
		"displayName": "Accepted Invitee"
	}`))
	acceptRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("accept invite failed: %d %s", acceptRec.Code, acceptRec.Body.String())
	}
	var accepted marketing.WorkspaceMember
	if err := json.NewDecoder(acceptRec.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accepted invite: %v", err)
	}
	if accepted.UserID != "invitee-bind@example.com" {
		t.Fatalf("public invite accepted client supplied user id: %+v", accepted)
	}
	if accepted.DisplayName != "Accepted Invitee" {
		t.Fatalf("expected display name update, got %+v", accepted)
	}
}

func TestSESInviteEmailSenderSignsAndSends(t *testing.T) {
	var gotAuth string
	var gotToken string
	var gotPayload string
	ses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotToken = r.Header.Get("X-Amz-Security-Token")
		payload, _ := io.ReadAll(r.Body)
		gotPayload = string(payload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"MessageId":"msg_1"}`))
	}))
	defer ses.Close()

	sender := sesInviteEmailSender{
		cfg: InviteEmailConfig{
			Provider:           "ses",
			From:               "noreply@example.com",
			SESRegion:          "us-east-1",
			SESAccessKeyID:     "AKIATEST",
			SESSecretAccessKey: "secret",
			SESSessionToken:    "session-token",
			SESEndpoint:        ses.URL + "/v2/email/outbound-emails",
		},
		client: ses.Client(),
	}
	delivery := sender.SendWorkspaceInvite(context.Background(), inviteEmailMessage{
		Workspace: marketing.Workspace{Name: "SES Workspace"},
		Member:    marketing.WorkspaceMember{Email: "ses@example.com", Role: marketing.WorkspaceRoleViewer},
		InviteURL: "https://example.com/invite/token",
	})
	if delivery.Status != "sent" || delivery.Provider != "ses" {
		t.Fatalf("unexpected ses delivery: %+v", delivery)
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 ") || gotToken != "session-token" {
		t.Fatalf("SES request was not signed correctly: auth=%q token=%q", gotAuth, gotToken)
	}
	if !strings.Contains(gotPayload, "ses@example.com") || !strings.Contains(gotPayload, "SES Workspace") {
		t.Fatalf("SES payload missing invite details: %s", gotPayload)
	}
}

func newTestHandler() *Handler {
	return NewHandler(engine.NewService(marketing.NewMemoryRepository()))
}

func newTestHandlerWithRepository(repo *marketing.MemoryRepository) *Handler {
	return NewHandler(engine.NewService(repo))
}

func newAuthTestHandler(token string, tenantIDs []string, sourceSystems []string, maxBodyBytes int64) *Handler {
	credential := OpenAPIEventCredential{
		TenantIDs:     map[string]struct{}{},
		SourceSystems: map[string]struct{}{},
	}
	for _, tenantID := range tenantIDs {
		credential.TenantIDs[tenantID] = struct{}{}
	}
	for _, sourceSystem := range sourceSystems {
		credential.SourceSystems[sourceSystem] = struct{}{}
	}
	return NewHandlerWithOpenAPIEventConfig(engine.NewService(marketing.NewMemoryRepository()), OpenAPIEventConfig{
		RequireAuth:  true,
		Credentials:  map[string]OpenAPIEventCredential{token: credential},
		MaxBodyBytes: maxBodyBytes,
	})
}

func newWebhookTestHandler(repo *marketing.MemoryRepository, receivers map[string]WebhookReceiver) *Handler {
	return NewHandlerWithConfigs(engine.NewService(repo), OpenAPIEventConfig{MaxBodyBytes: defaultEventMaxBodyBytes}, WebhookReceiverConfig{
		MaxBodyBytes: defaultEventMaxBodyBytes,
		Receivers:    receivers,
	})
}

func newSignedCallbackTestHandler(repo *marketing.MemoryRepository) *Handler {
	return NewHandlerWithRewardCallbackConfig(engine.NewService(repo), OpenAPIEventConfig{MaxBodyBytes: defaultEventMaxBodyBytes}, WebhookReceiverConfig{}, RewardCallbackConfig{
		Secrets:      map[string]string{"site_001": "callback-secret"},
		MaxSkew:      5 * time.Minute,
		MaxBodyBytes: defaultEventMaxBodyBytes,
	})
}

func TestHealth(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestWorkspaceFileStorageLargeUploadStatsAndDelete(t *testing.T) {
	t.Setenv("EX_MARKETING_FILE_STORAGE_PROVIDER", "local")
	t.Setenv("EX_MARKETING_FILE_STORAGE_DIR", t.TempDir())
	handler := newTestHandler()
	workspace := createTestWorkspace(t, handler, "owner")

	largeSize := 8 * 1024 * 1024
	versionFile := uploadWorkspaceFileForTest(t, handler, "owner", map[string]string{
		"workspaceId":       workspace.ID,
		"kind":              string(marketing.WorkspaceFileWorkflowVersion),
		"workflowVersionId": "wfv_large",
	}, "workflow-version-large.bin", largeSize)
	if versionFile.SizeBytes != int64(largeSize) || versionFile.StorageProvider != "local" || versionFile.WorkflowVersionID != "wfv_large" {
		t.Fatalf("unexpected large version file: %+v", versionFile)
	}
	traceFile := uploadWorkspaceFileForTest(t, handler, "owner", map[string]string{
		"workspaceId": workspace.ID,
		"kind":        string(marketing.WorkspaceFileExecutionTrace),
		"executionId": "exec_large",
	}, "execution-trace.json", 1024)
	if traceFile.Kind != marketing.WorkspaceFileExecutionTrace || traceFile.ExecutionID != "exec_large" {
		t.Fatalf("unexpected trace file: %+v", traceFile)
	}

	storage := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/v1/workspace-home/storage", nil)
	req.Header.Set("X-Actor-Id", "owner")
	handler.Routes().ServeHTTP(storage, req)
	if storage.Code != http.StatusOK {
		t.Fatalf("storage summary status=%d body=%s", storage.Code, storage.Body.String())
	}
	var summary marketing.WorkspaceStorageUsage
	if err := json.NewDecoder(storage.Body).Decode(&summary); err != nil {
		t.Fatalf("decode storage summary: %v", err)
	}
	if summary.FileObjects != 2 || summary.FileBytes < int64(largeSize+1024) {
		t.Fatalf("expected file object/bytes summary, got %+v", summary)
	}

	listWithoutWorkspace := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/v1/workspace-files", nil)
	req.Header.Set("X-Actor-Id", "owner")
	handler.Routes().ServeHTTP(listWithoutWorkspace, req)
	if listWithoutWorkspace.Code != http.StatusBadRequest {
		t.Fatalf("expected workspaceId requirement, got %d %s", listWithoutWorkspace.Code, listWithoutWorkspace.Body.String())
	}

	listForWorkspace := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/v1/workspace-files?workspaceId="+workspace.ID, nil)
	req.Header.Set("X-Actor-Id", "owner")
	handler.Routes().ServeHTTP(listForWorkspace, req)
	if listForWorkspace.Code != http.StatusOK {
		t.Fatalf("list workspace files status=%d body=%s", listForWorkspace.Code, listForWorkspace.Body.String())
	}
	var listPayload struct {
		Items []marketing.WorkspaceFileObject `json:"items"`
	}
	if err := json.NewDecoder(listForWorkspace.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode file list: %v", err)
	}
	if len(listPayload.Items) != 2 {
		t.Fatalf("expected workspace scoped file list, got %+v", listPayload.Items)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/admin/v1/workspace-files/"+versionFile.ID+"?workspaceId="+workspace.ID, nil)
	deleteReq.Header.Set("X-Actor-Id", "owner")
	deleteRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete file status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	storageAfterDelete := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/admin/v1/workspace-home/storage", nil)
	req.Header.Set("X-Actor-Id", "owner")
	handler.Routes().ServeHTTP(storageAfterDelete, req)
	if storageAfterDelete.Code != http.StatusOK {
		t.Fatalf("storage summary after delete status=%d body=%s", storageAfterDelete.Code, storageAfterDelete.Body.String())
	}
	var after marketing.WorkspaceStorageUsage
	if err := json.NewDecoder(storageAfterDelete.Body).Decode(&after); err != nil {
		t.Fatalf("decode storage summary after delete: %v", err)
	}
	if after.FileObjects != 1 || after.FileBytes != traceFile.SizeBytes {
		t.Fatalf("expected deleted file bytes to be reclaimed, got %+v", after)
	}
}

func TestWorkspaceFileStorageProviderSwitchesToS3Compatible(t *testing.T) {
	var putPath string
	var deletePath string
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putPath = r.URL.EscapedPath()
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			deletePath = r.URL.EscapedPath()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer s3.Close()

	t.Setenv("EX_MARKETING_FILE_STORAGE_PROVIDER", "minio")
	t.Setenv("EX_MARKETING_S3_ENDPOINT", s3.URL)
	t.Setenv("EX_MARKETING_S3_BUCKET", "exmarketing-test")
	t.Setenv("EX_MARKETING_S3_PREFIX", "workspace-files")
	handler := newTestHandler()
	workspace := createTestWorkspace(t, handler, "owner")

	file := uploadWorkspaceFileForTest(t, handler, "owner", map[string]string{
		"workspaceId": workspace.ID,
		"kind":        string(marketing.WorkspaceFileExecutionAttachment),
		"executionId": "exec_minio",
	}, "minio-object.bin", 2048)
	if file.StorageProvider != "minio" || !strings.Contains(putPath, "/exmarketing-test/workspace-files/"+workspace.ID+"/") {
		t.Fatalf("expected minio put path/provider, file=%+v path=%s", file, putPath)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/admin/v1/workspace-files/"+file.ID+"?workspaceId="+workspace.ID, nil)
	deleteReq.Header.Set("X-Actor-Id", "owner")
	deleteRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete minio file status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if !strings.Contains(deletePath, "/exmarketing-test/workspace-files/"+workspace.ID+"/") {
		t.Fatalf("expected minio delete path, got %s", deletePath)
	}
}

func TestWorkspaceAdminHTTPAPI(t *testing.T) {
	handler := newTestHandler()
	withOwner := func(req *http.Request) *http.Request {
		req.Header.Set("X-Actor-Id", "owner")
		return req
	}

	createWorkspace := httptest.NewRecorder()
	handler.Routes().ServeHTTP(createWorkspace, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
		"tenantId": "site_001",
		"name": "Growth Workspace",
		"slug": "growth",
		"visibility": "TEAM",
		"defaultRole": "VIEWER",
		"ownerId": "owner",
		"createdBy": "owner"
	}`))))
	if createWorkspace.Code != http.StatusCreated {
		t.Fatalf("create workspace status=%d body=%s", createWorkspace.Code, createWorkspace.Body.String())
	}
	var workspace marketing.Workspace
	if err := json.NewDecoder(createWorkspace.Body).Decode(&workspace); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if workspace.ID == "" || workspace.Status != marketing.WorkspaceActive {
		t.Fatalf("unexpected workspace: %+v", workspace)
	}

	duplicate := httptest.NewRecorder()
	handler.Routes().ServeHTTP(duplicate, httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
		"tenantId": "site_001",
		"name": "Growth Workspace",
		"slug": "growth"
	}`)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status 409, got %d: %s", duplicate.Code, duplicate.Body.String())
	}

	listWorkspace := httptest.NewRecorder()
	handler.Routes().ServeHTTP(listWorkspace, httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces?tenantId=site_001", nil))
	if listWorkspace.Code != http.StatusOK || !strings.Contains(listWorkspace.Body.String(), `"slug":"growth"`) {
		t.Fatalf("list workspace failed: %d %s", listWorkspace.Code, listWorkspace.Body.String())
	}

	updateWorkspace := httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateWorkspace, withOwner(httptest.NewRequest(http.MethodPut, "/admin/v1/workspaces/"+workspace.ID, strings.NewReader(`{
		"name": "Growth Ops",
		"description": "clear me",
		"icon": "spark",
		"color": "#2563eb",
		"visibility": "PRIVATE",
		"updatedBy": "owner"
	}`))))
	if updateWorkspace.Code != http.StatusOK || !strings.Contains(updateWorkspace.Body.String(), `"visibility":"PRIVATE"`) {
		t.Fatalf("update workspace failed: %d %s", updateWorkspace.Code, updateWorkspace.Body.String())
	}
	clearWorkspace := httptest.NewRecorder()
	handler.Routes().ServeHTTP(clearWorkspace, withOwner(httptest.NewRequest(http.MethodPut, "/admin/v1/workspaces/"+workspace.ID, strings.NewReader(`{
		"description": "",
		"icon": "",
		"color": "",
		"ownerId": "",
		"updatedBy": "owner"
	}`))))
	if clearWorkspace.Code != http.StatusOK {
		t.Fatalf("clear workspace failed: %d %s", clearWorkspace.Code, clearWorkspace.Body.String())
	}
	var clearedWorkspace marketing.Workspace
	if err := json.NewDecoder(clearWorkspace.Body).Decode(&clearedWorkspace); err != nil {
		t.Fatalf("decode cleared workspace: %v", err)
	}
	if clearedWorkspace.Description != "" || clearedWorkspace.Icon != "" || clearedWorkspace.Color != "" || clearedWorkspace.OwnerID != "" {
		t.Fatalf("expected cleared workspace metadata, got %+v", clearedWorkspace)
	}

	createMember := httptest.NewRecorder()
	handler.Routes().ServeHTTP(createMember, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/members", strings.NewReader(`{
		"userId": "editor",
		"email": "editor@example.com",
		"displayName": "Editor",
		"role": "EDITOR",
		"status": "ACTIVE",
		"invitedBy": "owner"
	}`))))
	if createMember.Code != http.StatusCreated {
		t.Fatalf("create member failed: %d %s", createMember.Code, createMember.Body.String())
	}
	var member marketing.WorkspaceMember
	if err := json.NewDecoder(createMember.Body).Decode(&member); err != nil {
		t.Fatalf("decode member: %v", err)
	}

	getMember := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getMember, withOwner(httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/members/"+member.ID, nil)))
	if getMember.Code != http.StatusOK || !strings.Contains(getMember.Body.String(), `"userId":"editor"`) {
		t.Fatalf("get member failed: %d %s", getMember.Code, getMember.Body.String())
	}

	deleteMember := httptest.NewRecorder()
	handler.Routes().ServeHTTP(deleteMember, withOwner(httptest.NewRequest(http.MethodDelete, "/admin/v1/workspaces/"+workspace.ID+"/members/"+member.ID, nil)))
	if deleteMember.Code != http.StatusOK || !strings.Contains(deleteMember.Body.String(), `"status":"REMOVED"`) {
		t.Fatalf("delete member failed: %d %s", deleteMember.Code, deleteMember.Body.String())
	}

	createFolder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(createFolder, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/folders", strings.NewReader(`{
		"name": "VIP 活动",
		"visibility": "TEAM",
		"sortOrder": 10,
		"createdBy": "owner"
	}`))))
	if createFolder.Code != http.StatusCreated {
		t.Fatalf("create folder failed: %d %s", createFolder.Code, createFolder.Body.String())
	}
	var folder marketing.WorkspaceFolder
	if err := json.NewDecoder(createFolder.Body).Decode(&folder); err != nil {
		t.Fatalf("decode folder: %v", err)
	}

	createWorkspaceActivity := httptest.NewRecorder()
	activityBody := strings.Replace(validActivityBody(), `"tenantId": "site_001",`, `"tenantId": "site_001", "workspaceId": "`+workspace.ID+`", "folderId": "`+folder.ID+`",`, 1)
	handler.Routes().ServeHTTP(createWorkspaceActivity, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(activityBody))))
	if createWorkspaceActivity.Code != http.StatusCreated {
		t.Fatalf("create workspace activity failed: %d %s", createWorkspaceActivity.Code, createWorkspaceActivity.Body.String())
	}
	var workspaceActivity marketing.Activity
	if err := json.NewDecoder(createWorkspaceActivity.Body).Decode(&workspaceActivity); err != nil {
		t.Fatalf("decode workspace activity: %v", err)
	}
	if workspaceActivity.WorkspaceID != workspace.ID || workspaceActivity.FolderID != folder.ID {
		t.Fatalf("expected activity workspace/folder binding, got %+v", workspaceActivity)
	}

	createChildFolder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(createChildFolder, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/folders", strings.NewReader(`{
		"parentId": "`+folder.ID+`",
		"name": "Child",
		"description": "clear me",
		"visibility": "TEAM",
		"sortOrder": 9,
		"createdBy": "owner"
	}`))))
	if createChildFolder.Code != http.StatusCreated {
		t.Fatalf("create child folder failed: %d %s", createChildFolder.Code, createChildFolder.Body.String())
	}
	var childFolder marketing.WorkspaceFolder
	if err := json.NewDecoder(createChildFolder.Body).Decode(&childFolder); err != nil {
		t.Fatalf("decode child folder: %v", err)
	}
	clearChildFolder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(clearChildFolder, withOwner(httptest.NewRequest(http.MethodPut, "/admin/v1/workspaces/"+workspace.ID+"/folders/"+childFolder.ID, strings.NewReader(`{
		"parentId": "",
		"description": "",
		"sortOrder": 0,
		"updatedBy": "owner"
	}`))))
	if clearChildFolder.Code != http.StatusOK {
		t.Fatalf("clear child folder failed: %d %s", clearChildFolder.Code, clearChildFolder.Body.String())
	}
	var clearedFolder marketing.WorkspaceFolder
	if err := json.NewDecoder(clearChildFolder.Body).Decode(&clearedFolder); err != nil {
		t.Fatalf("decode cleared folder: %v", err)
	}
	if clearedFolder.ParentID != "" || clearedFolder.Description != "" || clearedFolder.SortOrder != 0 {
		t.Fatalf("expected cleared folder parent/description/sort, got %+v", clearedFolder)
	}

	getFolder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getFolder, withOwner(httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/folders/"+folder.ID, nil)))
	if getFolder.Code != http.StatusOK || !strings.Contains(getFolder.Body.String(), `"name":"VIP 活动"`) {
		t.Fatalf("get folder failed: %d %s", getFolder.Code, getFolder.Body.String())
	}

	createShare := httptest.NewRecorder()
	handler.Routes().ServeHTTP(createShare, withOwner(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/share-links", strings.NewReader(`{
		"folderId": "`+folder.ID+`",
		"scope": "FOLDER",
		"permission": "VIEW",
		"createdBy": "owner"
	}`))))
	if createShare.Code != http.StatusCreated {
		t.Fatalf("create share failed: %d %s", createShare.Code, createShare.Body.String())
	}
	var share marketing.WorkspaceShareLink
	if err := json.NewDecoder(createShare.Body).Decode(&share); err != nil {
		t.Fatalf("decode share: %v", err)
	}

	listShares := httptest.NewRecorder()
	handler.Routes().ServeHTTP(listShares, withOwner(httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/share-links?enabledOnly=true", nil)))
	if listShares.Code != http.StatusOK || !strings.Contains(listShares.Body.String(), share.Token) {
		t.Fatalf("list shares failed: %d %s", listShares.Code, listShares.Body.String())
	}

	getShare := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getShare, withOwner(httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/share-links/"+share.ID, nil)))
	if getShare.Code != http.StatusOK || !strings.Contains(getShare.Body.String(), `"permission":"VIEW"`) {
		t.Fatalf("get share failed: %d %s", getShare.Code, getShare.Body.String())
	}

	updateShare := httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateShare, withOwner(httptest.NewRequest(http.MethodPut, "/admin/v1/workspaces/"+workspace.ID+"/share-links/"+share.ID, strings.NewReader(`{
		"permission": "EDIT",
		"enabled": false,
		"updatedBy": "owner"
	}`))))
	if updateShare.Code != http.StatusOK || !strings.Contains(updateShare.Body.String(), `"permission":"EDIT"`) || !strings.Contains(updateShare.Body.String(), `"enabled":false`) {
		t.Fatalf("update share failed: %d %s", updateShare.Code, updateShare.Body.String())
	}

	deleteShare := httptest.NewRecorder()
	handler.Routes().ServeHTTP(deleteShare, withOwner(httptest.NewRequest(http.MethodDelete, "/admin/v1/workspaces/"+workspace.ID+"/share-links/"+share.ID, nil)))
	if deleteShare.Code != http.StatusOK || !strings.Contains(deleteShare.Body.String(), `"enabled":false`) {
		t.Fatalf("delete share failed: %d %s", deleteShare.Code, deleteShare.Body.String())
	}

	audit := httptest.NewRecorder()
	handler.Routes().ServeHTTP(audit, withOwner(httptest.NewRequest(http.MethodGet, "/admin/v1/workspaces/"+workspace.ID+"/audit-logs?limit=20", nil)))
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"CREATE_WORKSPACE"`) || !strings.Contains(audit.Body.String(), `"CREATE_FOLDER"`) {
		t.Fatalf("audit logs failed: %d %s", audit.Code, audit.Body.String())
	}
}

func TestWorkspaceRolePermissionsHTTPAPI(t *testing.T) {
	handler := newTestHandler()
	routes := handler.Routes()
	withActor := func(req *http.Request, actor string) *http.Request {
		req.Header.Set("X-Actor-Id", actor)
		return req
	}

	createWorkspace := httptest.NewRecorder()
	routes.ServeHTTP(createWorkspace, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
		"tenantId": "site_001",
		"name": "Permission Workspace",
		"slug": "permission-workspace",
		"visibility": "PRIVATE",
		"defaultRole": "VIEWER",
		"ownerId": "owner",
		"createdBy": "owner"
	}`)), "owner"))
	if createWorkspace.Code != http.StatusCreated {
		t.Fatalf("create workspace failed: %d %s", createWorkspace.Code, createWorkspace.Body.String())
	}
	var workspace marketing.Workspace
	if err := json.NewDecoder(createWorkspace.Body).Decode(&workspace); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}

	createFolder := httptest.NewRecorder()
	routes.ServeHTTP(createFolder, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/folders", strings.NewReader(`{
		"name": "Editable Folder",
		"visibility": "TEAM",
		"sortOrder": 10
	}`)), "owner"))
	if createFolder.Code != http.StatusCreated {
		t.Fatalf("create folder failed: %d %s", createFolder.Code, createFolder.Body.String())
	}
	var folder marketing.WorkspaceFolder
	if err := json.NewDecoder(createFolder.Body).Decode(&folder); err != nil {
		t.Fatalf("decode folder: %v", err)
	}

	for _, body := range []string{
		`{"userId":"viewer","role":"VIEWER","status":"ACTIVE"}`,
		`{"userId":"editor","role":"EDITOR","status":"ACTIVE"}`,
		`{"userId":"admin","role":"ADMIN","status":"ACTIVE"}`,
	} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/members", strings.NewReader(body)), "owner"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create member failed: %d %s", rec.Code, rec.Body.String())
		}
	}

	activityBody := strings.Replace(validActivityBody(), `"tenantId": "site_001",`, `"tenantId": "site_001", "workspaceId": "`+workspace.ID+`", "folderId": "`+folder.ID+`",`, 1)
	createActivity := httptest.NewRecorder()
	routes.ServeHTTP(createActivity, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(activityBody)), "editor"))
	if createActivity.Code != http.StatusCreated {
		t.Fatalf("editor should create activity, got %d: %s", createActivity.Code, createActivity.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(createActivity.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}

	draftBody := `{
		"definition": {
			"meta": {"activityId":"` + activity.ID + `","workspaceId":"` + workspace.ID + `","folderId":"` + folder.ID + `"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口","outputs":["raw_event"],"position":{"x":80,"y":120}},
				{"id":"reward","type":"reward","kind":"reward","title":"奖励计算","inputs":["raw_event"],"outputs":["reward"],"position":{"x":410,"y":120}}
			],
			"edges": [{"id":"event-in-reward","source":"event-in","target":"reward"}]
		}
	}`
	viewerSave := httptest.NewRecorder()
	routes.ServeHTTP(viewerSave, withActor(httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID+"/workflow/draft", strings.NewReader(draftBody)), "viewer"))
	if viewerSave.Code != http.StatusForbidden {
		t.Fatalf("viewer should not save draft, got %d: %s", viewerSave.Code, viewerSave.Body.String())
	}

	editorSave := httptest.NewRecorder()
	routes.ServeHTTP(editorSave, withActor(httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID+"/workflow/draft", strings.NewReader(draftBody)), "editor"))
	if editorSave.Code != http.StatusOK {
		t.Fatalf("editor should save draft, got %d: %s", editorSave.Code, editorSave.Body.String())
	}

	editorPublish := httptest.NewRecorder()
	routes.ServeHTTP(editorPublish, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activity.ID+"/workflow/publish", strings.NewReader(`{}`)), "editor"))
	if editorPublish.Code != http.StatusForbidden {
		t.Fatalf("editor should not publish, got %d: %s", editorPublish.Code, editorPublish.Body.String())
	}

	adminPublish := httptest.NewRecorder()
	routes.ServeHTTP(adminPublish, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activity.ID+"/workflow/publish", strings.NewReader(`{}`)), "admin"))
	if adminPublish.Code != http.StatusCreated {
		t.Fatalf("admin should publish workflow, got %d: %s", adminPublish.Code, adminPublish.Body.String())
	}

	debugRun := httptest.NewRecorder()
	routes.ServeHTTP(debugRun, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activity.ID+"/workflow/debug-run", strings.NewReader(`{"operator":"editor","template":"deposit_success"}`)), "editor"))
	if debugRun.Code != http.StatusCreated {
		t.Fatalf("editor should run workflow debug trace, got %d: %s", debugRun.Code, debugRun.Body.String())
	}
	var debugTrace marketing.WorkflowExecutionTrace
	if err := json.NewDecoder(debugRun.Body).Decode(&debugTrace); err != nil {
		t.Fatalf("decode debug trace: %v", err)
	}
	for _, item := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "nodes", method: http.MethodGet, path: "/admin/v1/workflow/executions/" + debugTrace.Execution.ID + "/nodes"},
		{name: "trace", method: http.MethodGet, path: "/admin/v1/workflow/executions/" + debugTrace.Execution.ID + "/trace"},
		{name: "compare", method: http.MethodGet, path: "/admin/v1/workflow/executions/" + debugTrace.Execution.ID + "/compare?baselineExecutionId=" + debugTrace.Execution.ID},
		{name: "cancel", method: http.MethodPost, path: "/admin/v1/workflow/executions/" + debugTrace.Execution.ID + "/cancel", body: `{"operator":"viewer","reason":"test"}`},
	} {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, withActor(httptest.NewRequest(item.method, item.path, strings.NewReader(item.body)), "outsider"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("outsider should not access execution %s endpoint, got %d: %s", item.name, rec.Code, rec.Body.String())
		}
	}
	viewerNodes := httptest.NewRecorder()
	routes.ServeHTTP(viewerNodes, withActor(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+debugTrace.Execution.ID+"/nodes", nil), "viewer"))
	if viewerNodes.Code != http.StatusOK || !strings.Contains(viewerNodes.Body.String(), `"nodeId":"event-in"`) {
		t.Fatalf("viewer should access node execution list, got %d: %s", viewerNodes.Code, viewerNodes.Body.String())
	}
	viewerCancel := httptest.NewRecorder()
	routes.ServeHTTP(viewerCancel, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/executions/"+debugTrace.Execution.ID+"/cancel", strings.NewReader(`{"operator":"viewer","reason":"test"}`)), "viewer"))
	if viewerCancel.Code != http.StatusForbidden {
		t.Fatalf("viewer should not cancel execution, got %d: %s", viewerCancel.Code, viewerCancel.Body.String())
	}

	createHiddenWorkspace := httptest.NewRecorder()
	routes.ServeHTTP(createHiddenWorkspace, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
		"tenantId": "site_001",
		"name": "Hidden Workspace",
		"slug": "hidden-workspace",
		"visibility": "PRIVATE",
		"defaultRole": "VIEWER",
		"ownerId": "other-owner",
		"createdBy": "other-owner"
	}`)), "other-owner"))
	if createHiddenWorkspace.Code != http.StatusCreated {
		t.Fatalf("create hidden workspace failed: %d %s", createHiddenWorkspace.Code, createHiddenWorkspace.Body.String())
	}
	var hiddenWorkspace marketing.Workspace
	if err := json.NewDecoder(createHiddenWorkspace.Body).Decode(&hiddenWorkspace); err != nil {
		t.Fatalf("decode hidden workspace: %v", err)
	}
	createHiddenFolder := httptest.NewRecorder()
	routes.ServeHTTP(createHiddenFolder, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+hiddenWorkspace.ID+"/folders", strings.NewReader(`{"name":"Hidden Folder","visibility":"TEAM"}`)), "other-owner"))
	if createHiddenFolder.Code != http.StatusCreated {
		t.Fatalf("create hidden folder failed: %d %s", createHiddenFolder.Code, createHiddenFolder.Body.String())
	}
	var hiddenFolder marketing.WorkspaceFolder
	if err := json.NewDecoder(createHiddenFolder.Body).Decode(&hiddenFolder); err != nil {
		t.Fatalf("decode hidden folder: %v", err)
	}
	hiddenActivityBody := strings.Replace(validActivityBody(), `"tenantId": "site_001",`, `"tenantId": "site_001", "workspaceId": "`+hiddenWorkspace.ID+`", "folderId": "`+hiddenFolder.ID+`",`, 1)
	hiddenActivityBody = strings.Replace(hiddenActivityBody, `"activityCode": "FIRST_DEPOSIT_BONUS"`, `"activityCode": "HIDDEN_DEPOSIT_BONUS"`, 1)
	hiddenActivityBody = strings.Replace(hiddenActivityBody, `"activityName": "First Deposit Bonus"`, `"activityName": "Hidden Deposit Bonus"`, 1)
	createHiddenActivity := httptest.NewRecorder()
	routes.ServeHTTP(createHiddenActivity, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(hiddenActivityBody)), "other-owner"))
	if createHiddenActivity.Code != http.StatusCreated {
		t.Fatalf("create hidden activity failed: %d %s", createHiddenActivity.Code, createHiddenActivity.Body.String())
	}
	var hiddenActivity marketing.Activity
	if err := json.NewDecoder(createHiddenActivity.Body).Decode(&hiddenActivity); err != nil {
		t.Fatalf("decode hidden activity: %v", err)
	}
	hiddenDraftBody := strings.Replace(draftBody, `"meta": {"activityId":"`+activity.ID+`","workspaceId":"`+workspace.ID+`","folderId":"`+folder.ID+`"}`, `"meta": {"activityId":"`+hiddenActivity.ID+`","workspaceId":"`+hiddenWorkspace.ID+`","folderId":"`+hiddenFolder.ID+`"}`, 1)
	saveHiddenDraft := httptest.NewRecorder()
	routes.ServeHTTP(saveHiddenDraft, withActor(httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+hiddenActivity.ID+"/workflow/draft", strings.NewReader(hiddenDraftBody)), "other-owner"))
	if saveHiddenDraft.Code != http.StatusOK {
		t.Fatalf("save hidden draft failed: %d %s", saveHiddenDraft.Code, saveHiddenDraft.Body.String())
	}
	hiddenDebugRun := httptest.NewRecorder()
	routes.ServeHTTP(hiddenDebugRun, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+hiddenActivity.ID+"/workflow/debug-run", strings.NewReader(`{"operator":"other-owner","template":"deposit_success"}`)), "other-owner"))
	if hiddenDebugRun.Code != http.StatusCreated {
		t.Fatalf("run hidden debug trace failed: %d %s", hiddenDebugRun.Code, hiddenDebugRun.Body.String())
	}
	var hiddenDebugTrace marketing.WorkflowExecutionTrace
	if err := json.NewDecoder(hiddenDebugRun.Body).Decode(&hiddenDebugTrace); err != nil {
		t.Fatalf("decode hidden debug trace: %v", err)
	}
	saveHiddenTemplate := httptest.NewRecorder()
	routes.ServeHTTP(saveHiddenTemplate, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+hiddenActivity.ID+"/workflow/draft-template", strings.NewReader(`{
		"templateId":"hidden-workflow-template",
		"name":"Hidden Workflow Template",
		"operator":"other-owner"
	}`)), "other-owner"))
	if saveHiddenTemplate.Code != http.StatusCreated {
		t.Fatalf("save hidden workflow template failed: %d %s", saveHiddenTemplate.Code, saveHiddenTemplate.Body.String())
	}
	listHiddenTemplates := httptest.NewRecorder()
	routes.ServeHTTP(listHiddenTemplates, withActor(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/draft-templates?source=SAVED", nil), "owner"))
	if listHiddenTemplates.Code != http.StatusOK || strings.Contains(listHiddenTemplates.Body.String(), "hidden-workflow-template") {
		t.Fatalf("owner should not list hidden workflow template, got %d: %s", listHiddenTemplates.Code, listHiddenTemplates.Body.String())
	}
	getHiddenTemplate := httptest.NewRecorder()
	routes.ServeHTTP(getHiddenTemplate, withActor(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/draft-templates/hidden-workflow-template", nil), "owner"))
	if getHiddenTemplate.Code != http.StatusForbidden {
		t.Fatalf("owner should not get hidden workflow template, got %d: %s", getHiddenTemplate.Code, getHiddenTemplate.Body.String())
	}
	hiddenTemplateVersions := httptest.NewRecorder()
	routes.ServeHTTP(hiddenTemplateVersions, withActor(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/draft-templates/hidden-workflow-template/versions", nil), "owner"))
	if hiddenTemplateVersions.Code != http.StatusNotFound {
		t.Fatalf("owner should not list hidden workflow template versions, got %d: %s", hiddenTemplateVersions.Code, hiddenTemplateVersions.Body.String())
	}
	compareHiddenBaseline := httptest.NewRecorder()
	routes.ServeHTTP(compareHiddenBaseline, withActor(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+debugTrace.Execution.ID+"/compare?baselineExecutionId="+hiddenDebugTrace.Execution.ID, nil), "owner"))
	if compareHiddenBaseline.Code != http.StatusForbidden {
		t.Fatalf("owner should not compare against hidden baseline execution, got %d: %s", compareHiddenBaseline.Code, compareHiddenBaseline.Body.String())
	}

	adminFolder := httptest.NewRecorder()
	routes.ServeHTTP(adminFolder, withActor(httptest.NewRequest(http.MethodPut, "/admin/v1/workspaces/"+workspace.ID+"/folders/"+folder.ID, strings.NewReader(`{"visibility":"PRIVATE"}`)), "admin"))
	if adminFolder.Code != http.StatusOK || !strings.Contains(adminFolder.Body.String(), `"visibility":"PRIVATE"`) {
		t.Fatalf("admin should update folder visibility, got %d: %s", adminFolder.Code, adminFolder.Body.String())
	}

	viewerShare := httptest.NewRecorder()
	routes.ServeHTTP(viewerShare, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/share-links", strings.NewReader(`{"scope":"WORKSPACE","permission":"VIEW"}`)), "viewer"))
	if viewerShare.Code != http.StatusForbidden {
		t.Fatalf("viewer should not create share link, got %d: %s", viewerShare.Code, viewerShare.Body.String())
	}

	ownerShare := httptest.NewRecorder()
	routes.ServeHTTP(ownerShare, withActor(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/share-links", strings.NewReader(`{"scope":"WORKSPACE","permission":"VIEW"}`)), "owner"))
	if ownerShare.Code != http.StatusCreated {
		t.Fatalf("owner should create share link, got %d: %s", ownerShare.Code, ownerShare.Body.String())
	}
}

func TestMetricsEndpointRendersEventAndOutboxMetrics(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	now := time.Now().UTC().Add(-2 * time.Minute)
	_, err := repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_metrics_1",
		TenantID:      "site_metrics",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://reward.example.com/callback",
		Payload:       json.RawMessage(`{"commandId":"cmd_metrics"}`),
		SendStatus:    marketing.OutboxPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	badEvent := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(`{"tenantId":`))
	badReq.Header.Set("Content-Type", "application/json")
	handler.Routes().ServeHTTP(badEvent, badReq)
	if badEvent.Code != http.StatusBadRequest {
		t.Fatalf("expected bad event status %d, got %d: %s", http.StatusBadRequest, badEvent.Code, badEvent.Body.String())
	}

	activityRec := postActivity(t, handler, validActivityBody())
	if activityRec.Code != http.StatusCreated {
		t.Fatalf("expected create activity status %d, got %d: %s", http.StatusCreated, activityRec.Code, activityRec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(strings.NewReader(activityRec.Body.String())).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	approveAndActivateCreatedActivity(t, handler, activityRec)
	createQuota(t, handler, activity.ID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1000.00")
	createQuota(t, handler, activity.ID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "100")
	eventRec := postDepositEvent(t, handler, "evt_metrics_ok", "biz_metrics_ok")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event status %d, got %d: %s", http.StatusAccepted, eventRec.Code, eventRec.Body.String())
	}

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`# TYPE exmarketing_event_intake_total counter`,
		`exmarketing_event_intake_total`,
		`reason="invalid_payload"`,
		`exmarketing_rule_execution_total`,
		`exmarketing_rule_stage_latency_seconds_bucket`,
		`exmarketing_reward_execution_total`,
		`exmarketing_reward_stage_latency_seconds_bucket`,
		`exmarketing_quota_attempt_total`,
		`exmarketing_quota_amount_total`,
		`exmarketing_outbox_backlog{send_status="PENDING",target_channel="HTTP",tenant_id="site_metrics"} 1`,
		`exmarketing_outbox_oldest_eligible_age_seconds`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected metrics body to contain %q, got:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{`activity_id=`, `activityId=`, `eventId=`, `subjectId=`, `bizId=`, `commandId=`, `idempotencyKey=`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics body contains forbidden label %q:\n%s", forbidden, body)
		}
	}
}

func TestCreateActivityReturnsUnifiedConfigVersions(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	if activity.Version != 1 || len(activity.Rules) != 1 || activity.Rules[0].Version != 1 || len(activity.Rewards) != 1 || activity.Rewards[0].Version != 1 {
		t.Fatalf("expected activity/rule/reward version 1, got activity=%d rules=%v rewards=%v", activity.Version, activity.Rules, activity.Rewards)
	}
	if !strings.Contains(string(activity.Config), `"entry"`) {
		t.Fatalf("expected activity config to be stored, got: %s", string(activity.Config))
	}
	if !strings.Contains(string(activity.Rewards[0].Config), `"targetRef":"https://wallet.example.com/reward"`) {
		t.Fatalf("expected reward targetRef to be stored, got: %s", string(activity.Rewards[0].Config))
	}
}

func TestActivityWorkflowHTTPAPI(t *testing.T) {
	t.Run("node config schemas are exposed", func(t *testing.T) {
		handler := newTestHandler()
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/node-config-schemas", nil))
		if rec.Code != http.StatusOK ||
			!strings.Contains(rec.Body.String(), `"nodeType":"http-call"`) ||
			!strings.Contains(rec.Body.String(), `"key":"url"`) ||
			!strings.Contains(rec.Body.String(), `"required":true`) ||
			!strings.Contains(rec.Body.String(), `"validationRules"`) ||
			!strings.Contains(rec.Body.String(), `"type":"PATTERN"`) {
			t.Fatalf("expected node config schemas, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("workflow draft templates can apply and save versions", func(t *testing.T) {
		handler := newTestHandler()
		withOps := func(req *http.Request) *http.Request {
			req.Header.Set("X-Actor-Id", "ops")
			return req
		}
		createWorkspace := httptest.NewRecorder()
		handler.Routes().ServeHTTP(createWorkspace, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces", strings.NewReader(`{
			"tenantId": "site_001",
			"name": "Template Workspace",
			"slug": "template-workspace",
			"visibility": "TEAM",
			"defaultRole": "VIEWER",
			"ownerId": "ops",
			"createdBy": "ops"
		}`))))
		if createWorkspace.Code != http.StatusCreated {
			t.Fatalf("create template workspace failed: %d %s", createWorkspace.Code, createWorkspace.Body.String())
		}
		var workspace marketing.Workspace
		if err := json.NewDecoder(createWorkspace.Body).Decode(&workspace); err != nil {
			t.Fatalf("decode template workspace: %v", err)
		}
		createFolder := httptest.NewRecorder()
		handler.Routes().ServeHTTP(createFolder, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/workspaces/"+workspace.ID+"/folders", strings.NewReader(`{"name":"Templates","visibility":"TEAM","createdBy":"ops"}`))))
		if createFolder.Code != http.StatusCreated {
			t.Fatalf("create template folder failed: %d %s", createFolder.Code, createFolder.Body.String())
		}
		var folder marketing.WorkspaceFolder
		if err := json.NewDecoder(createFolder.Body).Decode(&folder); err != nil {
			t.Fatalf("decode template folder: %v", err)
		}
		activityBody := strings.Replace(validActivityBody(), `"tenantId": "site_001",`, `"tenantId": "site_001", "workspaceId": "`+workspace.ID+`", "folderId": "`+folder.ID+`",`, 1)
		createActivity := httptest.NewRecorder()
		handler.Routes().ServeHTTP(createActivity, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(activityBody))))
		if createActivity.Code != http.StatusCreated {
			t.Fatalf("create template activity failed: %d %s", createActivity.Code, createActivity.Body.String())
		}
		var activity marketing.Activity
		if err := json.NewDecoder(createActivity.Body).Decode(&activity); err != nil {
			t.Fatalf("decode template activity: %v", err)
		}
		activityID := activity.ID

		listTemplates := httptest.NewRecorder()
		handler.Routes().ServeHTTP(listTemplates, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/draft-templates", nil))
		if listTemplates.Code != http.StatusOK ||
			!strings.Contains(listTemplates.Body.String(), "complete-activity-chain") ||
			!strings.Contains(listTemplates.Body.String(), "quota-protected-reward-chain") {
			t.Fatalf("expected built-in workflow draft templates, got %d: %s", listTemplates.Code, listTemplates.Body.String())
		}

		listPresets := httptest.NewRecorder()
		handler.Routes().ServeHTTP(listPresets, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/node-presets", nil))
		if listPresets.Code != http.StatusOK || !strings.Contains(listPresets.Body.String(), "reward-fixed-usd") {
			t.Fatalf("expected node presets, got %d: %s", listPresets.Code, listPresets.Body.String())
		}

		applyTemplate := httptest.NewRecorder()
		handler.Routes().ServeHTTP(applyTemplate, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft-templates/simple-reward-chain/apply", strings.NewReader(`{
			"operator":"ops",
			"workspaceId":"`+workspace.ID+`",
			"folderId":"`+folder.ID+`",
			"parameters":{"eventType":"SIGNUP_SUCCESS","rewardAmount":25,"currency":"EUR","quotaCode":"WELCOME_QUOTA","limitMaxCount":2}
		}`))))
		if applyTemplate.Code != http.StatusOK ||
			!strings.Contains(applyTemplate.Body.String(), `"activityId":"`+activityID+`"`) ||
			!strings.Contains(applyTemplate.Body.String(), `"eventTypes":["SIGNUP_SUCCESS"]`) ||
			!strings.Contains(applyTemplate.Body.String(), `"amount":25`) ||
			!strings.Contains(applyTemplate.Body.String(), `"currency":"EUR"`) ||
			!strings.Contains(applyTemplate.Body.String(), `"quotaCode":"WELCOME_QUOTA"`) {
			t.Fatalf("expected template draft apply, got %d: %s", applyTemplate.Code, applyTemplate.Body.String())
		}

		validateTemplateDraft := httptest.NewRecorder()
		handler.Routes().ServeHTTP(validateTemplateDraft, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft/validate", nil)))
		if validateTemplateDraft.Code != http.StatusOK || !strings.Contains(validateTemplateDraft.Body.String(), `"valid":true`) {
			t.Fatalf("expected template draft to be valid, got %d: %s", validateTemplateDraft.Code, validateTemplateDraft.Body.String())
		}

		saveTemplate := httptest.NewRecorder()
		handler.Routes().ServeHTTP(saveTemplate, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft-template", strings.NewReader(`{
			"templateId":"saved-fast-reward",
			"name":"保存的快速奖励模板",
			"category":"团队模板",
			"operator":"ops",
			"workspaceId":"`+workspace.ID+`",
			"folderId":"`+folder.ID+`",
			"visibility":"PUBLIC"
		}`))))
		if saveTemplate.Code != http.StatusCreated ||
			!strings.Contains(saveTemplate.Body.String(), `"version":1`) ||
			!strings.Contains(saveTemplate.Body.String(), `"workspaceId":"`+workspace.ID+`"`) ||
			!strings.Contains(saveTemplate.Body.String(), `"folderId":"`+folder.ID+`"`) ||
			!strings.Contains(saveTemplate.Body.String(), `"visibility":"PUBLIC"`) {
			t.Fatalf("expected saved workflow template, got %d: %s", saveTemplate.Code, saveTemplate.Body.String())
		}

		saveTemplateV2 := httptest.NewRecorder()
		handler.Routes().ServeHTTP(saveTemplateV2, withOps(httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft-template", strings.NewReader(`{
			"templateId":"saved-fast-reward",
			"name":"保存的快速奖励模板",
			"category":"团队模板",
			"operator":"ops"
		}`))))
		if saveTemplateV2.Code != http.StatusCreated || !strings.Contains(saveTemplateV2.Body.String(), `"version":2`) {
			t.Fatalf("expected saved workflow template v2, got %d: %s", saveTemplateV2.Code, saveTemplateV2.Body.String())
		}

		versions := httptest.NewRecorder()
		handler.Routes().ServeHTTP(versions, withOps(httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/draft-templates/saved-fast-reward/versions", nil)))
		if versions.Code != http.StatusOK || !strings.Contains(versions.Body.String(), `"version":2`) || !strings.Contains(versions.Body.String(), `"version":1`) {
			t.Fatalf("expected saved template versions, got %d: %s", versions.Code, versions.Body.String())
		}
	})

	t.Run("publish requires draft", func(t *testing.T) {
		handler := newTestHandler()
		activityID := createActivity(t, handler)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "workflow draft is required") {
			t.Fatalf("expected missing draft status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
		}
	})

	handler := newTestHandler()
	activityID := createActivity(t, handler)

	validateMissingDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(validateMissingDraft, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft/validate", nil))
	if validateMissingDraft.Code != http.StatusNotFound {
		t.Fatalf("expected missing draft validation status %d, got %d: %s", http.StatusNotFound, validateMissingDraft.Code, validateMissingDraft.Body.String())
	}

	invalidDraft := httptest.NewRecorder()
	invalidReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {"meta":{"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"match","type":"match"}],"edges":[]}
	}`))
	handler.Routes().ServeHTTP(invalidDraft, invalidReq)
	if invalidDraft.Code != http.StatusOK {
		t.Fatalf("expected invalid but parseable draft to save, got %d: %s", invalidDraft.Code, invalidDraft.Body.String())
	}

	validateInvalidDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(validateInvalidDraft, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft/validate", nil))
	if validateInvalidDraft.Code != http.StatusOK ||
		!strings.Contains(validateInvalidDraft.Body.String(), `"valid":false`) ||
		!strings.Contains(validateInvalidDraft.Body.String(), `"code":"WORKFLOW_ENTRY_MISSING"`) {
		t.Fatalf("expected saved invalid draft validation issue, got %d: %s", validateInvalidDraft.Code, validateInvalidDraft.Body.String())
	}

	publishInvalidDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(publishInvalidDraft, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`)))
	if publishInvalidDraft.Code != http.StatusBadRequest || !strings.Contains(publishInvalidDraft.Body.String(), "WORKFLOW_ENTRY_MISSING") {
		t.Fatalf("expected saved invalid draft publish to fail, got %d: %s", publishInvalidDraft.Code, publishInvalidDraft.Body.String())
	}

	validateMissingEntry := httptest.NewRecorder()
	validateReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {"meta":{"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"match","type":"match"}],"edges":[]}
	}`))
	handler.Routes().ServeHTTP(validateMissingEntry, validateReq)
	if validateMissingEntry.Code != http.StatusOK ||
		!strings.Contains(validateMissingEntry.Body.String(), `"valid":false`) ||
		!strings.Contains(validateMissingEntry.Body.String(), `"code":"WORKFLOW_ENTRY_MISSING"`) {
		t.Fatalf("expected entry missing validation, got %d: %s", validateMissingEntry.Code, validateMissingEntry.Body.String())
	}

	validateDuplicateNodeID := httptest.NewRecorder()
	validateDuplicateNodeIDReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {"meta":{"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},"nodes":[{"id":"event-in","type":"event-in","kind":"entry"},{"id":"event-in","type":"match"}],"edges":[]}
	}`))
	handler.Routes().ServeHTTP(validateDuplicateNodeID, validateDuplicateNodeIDReq)
	if validateDuplicateNodeID.Code != http.StatusOK ||
		!strings.Contains(validateDuplicateNodeID.Body.String(), `"valid":false`) ||
		!strings.Contains(validateDuplicateNodeID.Body.String(), `"code":"WORKFLOW_NODE_ID_DUPLICATED"`) {
		t.Fatalf("expected duplicate node id validation, got %d: %s", validateDuplicateNodeID.Code, validateDuplicateNodeID.Body.String())
	}

	validateMetadata := httptest.NewRecorder()
	validateMetadataReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateMetadata, validateMetadataReq)
	if validateMetadata.Code != http.StatusOK ||
		!strings.Contains(validateMetadata.Body.String(), `"valid":false`) ||
		!strings.Contains(validateMetadata.Body.String(), `"code":"WORKFLOW_METADATA_ACTIVITY_ID_REQUIRED"`) ||
		!strings.Contains(validateMetadata.Body.String(), `"code":"WORKFLOW_METADATA_WORKSPACE_ID_REQUIRED"`) ||
		!strings.Contains(validateMetadata.Body.String(), `"code":"WORKFLOW_METADATA_FOLDER_ID_REQUIRED"`) {
		t.Fatalf("expected workflow metadata validation, got %d: %s", validateMetadata.Code, validateMetadata.Body.String())
	}

	validateEdgeEndpoints := httptest.NewRecorder()
	validateEdgeEndpointsReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"}
			],
			"edges": [
				{"id":"missing-source","source":"missing","target":"match"},
				{"id":"missing-target","source":"event-in","target":"missing"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateEdgeEndpoints, validateEdgeEndpointsReq)
	if validateEdgeEndpoints.Code != http.StatusOK ||
		!strings.Contains(validateEdgeEndpoints.Body.String(), `"valid":false`) ||
		!strings.Contains(validateEdgeEndpoints.Body.String(), `"code":"WORKFLOW_EDGE_SOURCE_MISSING"`) ||
		!strings.Contains(validateEdgeEndpoints.Body.String(), `"code":"WORKFLOW_EDGE_TARGET_MISSING"`) {
		t.Fatalf("expected missing edge endpoint validation, got %d: %s", validateEdgeEndpoints.Code, validateEdgeEndpoints.Body.String())
	}

	validateReachability := httptest.NewRecorder()
	validateReachabilityReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"},
				{"id":"qualify","type":"qualify"},
				{"id":"reward","type":"reward"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"},
				{"id":"qualify-reward","source":"qualify","target":"reward"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateReachability, validateReachabilityReq)
	if validateReachability.Code != http.StatusOK ||
		!strings.Contains(validateReachability.Body.String(), `"valid":false`) ||
		!strings.Contains(validateReachability.Body.String(), `"code":"WORKFLOW_NODE_UNREACHABLE"`) ||
		!strings.Contains(validateReachability.Body.String(), `"nodeId":"qualify"`) {
		t.Fatalf("expected unreachable node validation, got %d: %s", validateReachability.Code, validateReachability.Body.String())
	}

	validateIsland := httptest.NewRecorder()
	validateIslandReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"},
				{"id":"reward","type":"reward"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateIsland, validateIslandReq)
	if validateIsland.Code != http.StatusOK ||
		!strings.Contains(validateIsland.Body.String(), `"valid":false`) ||
		!strings.Contains(validateIsland.Body.String(), `"code":"WORKFLOW_NODE_ISLAND"`) ||
		!strings.Contains(validateIsland.Body.String(), `"nodeId":"reward"`) {
		t.Fatalf("expected island node validation, got %d: %s", validateIsland.Code, validateIsland.Body.String())
	}

	validateDuplicateOutput := httptest.NewRecorder()
	validateDuplicateOutputReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"},
				{"id":"qualify","type":"qualify"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","sourceHandle":"raw_event","target":"match"},
				{"id":"event-in-qualify","source":"event-in","sourceHandle":"raw_event","target":"qualify"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateDuplicateOutput, validateDuplicateOutputReq)
	if validateDuplicateOutput.Code != http.StatusOK ||
		!strings.Contains(validateDuplicateOutput.Body.String(), `"valid":false`) ||
		!strings.Contains(validateDuplicateOutput.Body.String(), `"code":"WORKFLOW_OUTPUT_DUPLICATED_EDGE"`) ||
		!strings.Contains(validateDuplicateOutput.Body.String(), `"nodeId":"event-in"`) ||
		!strings.Contains(validateDuplicateOutput.Body.String(), `"edgeId":"event-in-qualify"`) {
		t.Fatalf("expected duplicate output edge validation, got %d: %s", validateDuplicateOutput.Code, validateDuplicateOutput.Body.String())
	}

	validateCycle := httptest.NewRecorder()
	validateCycleReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"},
				{"id":"reward","type":"reward"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"},
				{"id":"match-reward","source":"match","target":"reward"},
				{"id":"reward-match","source":"reward","target":"match"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(validateCycle, validateCycleReq)
	if validateCycle.Code != http.StatusOK ||
		!strings.Contains(validateCycle.Body.String(), `"valid":false`) ||
		!strings.Contains(validateCycle.Body.String(), `"code":"WORKFLOW_UNSUPPORTED_CYCLE"`) {
		t.Fatalf("expected unsupported cycle validation, got %d: %s", validateCycle.Code, validateCycle.Body.String())
	}

	saveCycle := httptest.NewRecorder()
	saveCycleReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"},
				{"id":"reward","type":"reward"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"},
				{"id":"match-reward","source":"match","target":"reward"},
				{"id":"reward-match","source":"reward","target":"match"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(saveCycle, saveCycleReq)
	if saveCycle.Code != http.StatusOK {
		t.Fatalf("expected cyclic draft to save before publish validation, got %d: %s", saveCycle.Code, saveCycle.Body.String())
	}

	publishCycle := httptest.NewRecorder()
	publishCycleReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
	handler.Routes().ServeHTTP(publishCycle, publishCycleReq)
	if publishCycle.Code != http.StatusBadRequest || !strings.Contains(publishCycle.Body.String(), "WORKFLOW_UNSUPPORTED_CYCLE") {
		t.Fatalf("expected cyclic draft publish to fail, got %d: %s", publishCycle.Code, publishCycle.Body.String())
	}

	saveMissingNodeConfig := httptest.NewRecorder()
	saveMissingNodeConfigReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"http","type":"http-call","config":{"method":"POST"}}
			],
			"edges": [
				{"id":"event-in-http","source":"event-in","target":"http"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(saveMissingNodeConfig, saveMissingNodeConfigReq)
	if saveMissingNodeConfig.Code != http.StatusOK {
		t.Fatalf("expected missing node config draft to save before publish validation, got %d: %s", saveMissingNodeConfig.Code, saveMissingNodeConfig.Body.String())
	}

	publishMissingNodeConfig := httptest.NewRecorder()
	publishMissingNodeConfigReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
	handler.Routes().ServeHTTP(publishMissingNodeConfig, publishMissingNodeConfigReq)
	if publishMissingNodeConfig.Code != http.StatusBadRequest || !strings.Contains(publishMissingNodeConfig.Body.String(), "WORKFLOW_NODE_CONFIG_REQUIRED") {
		t.Fatalf("expected missing node config publish to fail, got %d: %s", publishMissingNodeConfig.Code, publishMissingNodeConfig.Body.String())
	}

	saveInvalidVariableRef := httptest.NewRecorder()
	saveInvalidVariableRefReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"inbox","type":"inbox"},
				{"id":"match","type":"match","config":{"inputMapping":{"event_context":{"sourceNodeId":"inbox","outputKey":"renamed_event_context","path":"$.user.id"}}}}
			],
			"edges": [
				{"id":"event-in-inbox","source":"event-in","target":"inbox"},
				{"id":"inbox-match","source":"inbox","target":"match"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(saveInvalidVariableRef, saveInvalidVariableRefReq)
	if saveInvalidVariableRef.Code != http.StatusOK {
		t.Fatalf("expected invalid variable ref draft to save before publish validation, got %d: %s", saveInvalidVariableRef.Code, saveInvalidVariableRef.Body.String())
	}

	publishInvalidVariableRef := httptest.NewRecorder()
	publishInvalidVariableRefReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
	handler.Routes().ServeHTTP(publishInvalidVariableRef, publishInvalidVariableRefReq)
	if publishInvalidVariableRef.Code != http.StatusBadRequest || !strings.Contains(publishInvalidVariableRef.Body.String(), "WORKFLOW_NODE_INPUT_MAPPING_OUTPUT_MISSING") {
		t.Fatalf("expected invalid variable ref publish to fail, got %d: %s", publishInvalidVariableRef.Code, publishInvalidVariableRef.Body.String())
	}

	saveInvalidRouteModel := httptest.NewRecorder()
	saveInvalidRouteModelReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry"},
				{"id":"match","type":"match"}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match","route":{"type":"OUTPUT"}}
			]
		}
	}`))
	handler.Routes().ServeHTTP(saveInvalidRouteModel, saveInvalidRouteModelReq)
	if saveInvalidRouteModel.Code != http.StatusOK {
		t.Fatalf("expected invalid route model draft to save before publish validation, got %d: %s", saveInvalidRouteModel.Code, saveInvalidRouteModel.Body.String())
	}

	publishInvalidRouteModel := httptest.NewRecorder()
	publishInvalidRouteModelReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
	handler.Routes().ServeHTTP(publishInvalidRouteModel, publishInvalidRouteModelReq)
	if publishInvalidRouteModel.Code != http.StatusBadRequest || !strings.Contains(publishInvalidRouteModel.Body.String(), "WORKFLOW_EDGE_ROUTE_KEY_REQUIRED") {
		t.Fatalf("expected invalid route model publish to fail, got %d: %s", publishInvalidRouteModel.Code, publishInvalidRouteModel.Body.String())
	}

	saveRec := httptest.NewRecorder()
	saveReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID+"/workflow/draft", strings.NewReader(`{
		"operator": "ops",
		"definition": {
			"meta": {"activityId":"`+activityID+`","workspaceId":"workspace-1","folderId":"folder-1"},
			"nodes": [
				{"id":"event-in","type":"event-in","kind":"entry","title":"事件入口","outputs":["raw_event"],"position":{"x":80,"y":120}},
				{"id":"match","type":"match","kind":"logic","title":"活动匹配","inputs":["raw_event"],"outputs":["matched"],"position":{"x":410,"y":120}},
				{"id":"reward","type":"reward","kind":"reward","title":"奖励计算","inputs":["matched"],"outputs":["reward"],"position":{"x":740,"y":120}}
			],
			"edges": [
				{"id":"event-in-match","source":"event-in","target":"match"},
				{"id":"match-reward","source":"match","target":"reward"}
			]
		}
	}`))
	handler.Routes().ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK || !strings.Contains(saveRec.Body.String(), `"activityId":"`+activityID+`"`) {
		t.Fatalf("expected save draft status %d, got %d: %s", http.StatusOK, saveRec.Code, saveRec.Body.String())
	}

	getDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getDraft, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activityID+"/workflow/draft", nil))
	if getDraft.Code != http.StatusOK || !strings.Contains(getDraft.Body.String(), `"event-in"`) {
		t.Fatalf("expected draft detail, got %d: %s", getDraft.Code, getDraft.Body.String())
	}

	validateCurrentDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(validateCurrentDraft, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/draft/validate", nil))
	if validateCurrentDraft.Code != http.StatusOK || !strings.Contains(validateCurrentDraft.Body.String(), `"valid":true`) {
		t.Fatalf("expected valid current draft validation, got %d: %s", validateCurrentDraft.Code, validateCurrentDraft.Body.String())
	}

	validateDraft := httptest.NewRecorder()
	handler.Routes().ServeHTTP(validateDraft, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/validate", strings.NewReader(`{}`)))
	if validateDraft.Code != http.StatusOK || !strings.Contains(validateDraft.Body.String(), `"valid":true`) {
		t.Fatalf("expected valid draft validation, got %d: %s", validateDraft.Code, validateDraft.Body.String())
	}

	debugRun := httptest.NewRecorder()
	handler.Routes().ServeHTTP(debugRun, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/debug-run", strings.NewReader(`{"operator":"ops","template":"deposit_success"}`)))
	if debugRun.Code != http.StatusCreated || !strings.Contains(debugRun.Body.String(), `"workflowVersionId":"debug-draft"`) || !strings.Contains(debugRun.Body.String(), `"nodeExecutions"`) {
		t.Fatalf("expected debug run trace, got %d: %s", debugRun.Code, debugRun.Body.String())
	}
	var debugTrace marketing.WorkflowExecutionTrace
	if err := json.NewDecoder(debugRun.Body).Decode(&debugTrace); err != nil {
		t.Fatalf("decode debug trace: %v", err)
	}
	exportDebugTrace := httptest.NewRecorder()
	handler.Routes().ServeHTTP(exportDebugTrace, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+debugTrace.Execution.ID+"/trace", nil))
	if exportDebugTrace.Code != http.StatusOK || !strings.Contains(exportDebugTrace.Body.String(), debugTrace.Execution.ID) {
		t.Fatalf("expected exported debug trace, got %d: %s", exportDebugTrace.Code, exportDebugTrace.Body.String())
	}
	rerunDebug := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rerunDebug, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/executions/"+debugTrace.Execution.ID+"/rerun", strings.NewReader(`{"operator":"ops"}`)))
	if rerunDebug.Code != http.StatusCreated || !strings.Contains(rerunDebug.Body.String(), `"sourceExecutionId":"`+debugTrace.Execution.ID+`"`) {
		t.Fatalf("expected rerun debug trace, got %d: %s", rerunDebug.Code, rerunDebug.Body.String())
	}

	publishRec := httptest.NewRecorder()
	publishReq := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`))
	handler.Routes().ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusCreated || !strings.Contains(publishRec.Body.String(), `"version":1`) {
		t.Fatalf("expected publish status %d, got %d: %s", http.StatusCreated, publishRec.Code, publishRec.Body.String())
	}
	var firstVersion marketing.WorkflowVersion
	if err := json.NewDecoder(publishRec.Body).Decode(&firstVersion); err != nil {
		t.Fatalf("decode first version: %v", err)
	}

	versionsRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(versionsRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activityID+"/workflow/versions", nil))
	if versionsRec.Code != http.StatusOK || !strings.Contains(versionsRec.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("expected versions list, got %d: %s", versionsRec.Code, versionsRec.Body.String())
	}

	archiveActive := httptest.NewRecorder()
	handler.Routes().ServeHTTP(archiveActive, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/versions/"+firstVersion.ID+"/archive", strings.NewReader(`{"operator":"ops"}`)))
	if archiveActive.Code != http.StatusBadRequest || !strings.Contains(archiveActive.Body.String(), "active workflow version cannot be archived") {
		t.Fatalf("expected active archive to be blocked, got %d: %s", archiveActive.Code, archiveActive.Body.String())
	}

	publishSecond := httptest.NewRecorder()
	handler.Routes().ServeHTTP(publishSecond, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/publish", strings.NewReader(`{"operator":"ops"}`)))
	if publishSecond.Code != http.StatusCreated || !strings.Contains(publishSecond.Body.String(), `"version":2`) {
		t.Fatalf("expected second publish, got %d: %s", publishSecond.Code, publishSecond.Body.String())
	}

	archiveOld := httptest.NewRecorder()
	handler.Routes().ServeHTTP(archiveOld, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/versions/"+firstVersion.ID+"/archive", strings.NewReader(`{"operator":"ops"}`)))
	if archiveOld.Code != http.StatusOK || !strings.Contains(archiveOld.Body.String(), `"status":"ARCHIVED"`) {
		t.Fatalf("expected old version archive to be idempotent, got %d: %s", archiveOld.Code, archiveOld.Body.String())
	}

	rollbackOld := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rollbackOld, httptest.NewRequest(http.MethodPost, "/admin/v1/activities/"+activityID+"/workflow/versions/"+firstVersion.ID+"/rollback", strings.NewReader(`{"operator":"ops"}`)))
	if rollbackOld.Code != http.StatusCreated || !strings.Contains(rollbackOld.Body.String(), `"version":3`) || !strings.Contains(rollbackOld.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("expected rollback to create active v3, got %d: %s", rollbackOld.Code, rollbackOld.Body.String())
	}

	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1000.00")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "100")
	eventRec := postDepositEvent(t, handler, "evt_workflow_http", "pay_workflow_http")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event status %d, got %d: %s", http.StatusAccepted, eventRec.Code, eventRec.Body.String())
	}

	executionsRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(executionsRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activityID+"/workflow/executions?limit=5", nil))
	if executionsRec.Code != http.StatusOK || !strings.Contains(executionsRec.Body.String(), `"eventId":"evt_workflow_http"`) {
		t.Fatalf("expected executions list, got %d: %s", executionsRec.Code, executionsRec.Body.String())
	}
	var executions struct {
		Items []marketing.WorkflowExecution `json:"items"`
	}
	if err := json.NewDecoder(executionsRec.Body).Decode(&executions); err != nil {
		t.Fatalf("decode executions: %v", err)
	}
	var productionExecution marketing.WorkflowExecution
	for _, item := range executions.Items {
		if item.WorkflowVersionID != "debug-draft" {
			productionExecution = item
			break
		}
	}
	if productionExecution.ID == "" {
		t.Fatalf("expected production workflow execution, got %+v", executions.Items)
	}
	versionFilteredRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(versionFilteredRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activityID+"/workflow/executions?limit=5&workflowVersionId="+productionExecution.WorkflowVersionID, nil))
	if versionFilteredRec.Code != http.StatusOK || !strings.Contains(versionFilteredRec.Body.String(), productionExecution.ID) {
		t.Fatalf("expected version-filtered executions list, got %d: %s", versionFilteredRec.Code, versionFilteredRec.Body.String())
	}
	if !strings.Contains(versionFilteredRec.Body.String(), `"hasMore":`) || !strings.Contains(versionFilteredRec.Body.String(), `"nextOffset":`) {
		t.Fatalf("expected execution pagination metadata, got %d: %s", versionFilteredRec.Code, versionFilteredRec.Body.String())
	}
	oldVersionFilteredRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(oldVersionFilteredRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activityID+"/workflow/executions?limit=5&workflowVersionId="+firstVersion.ID, nil))
	if oldVersionFilteredRec.Code != http.StatusOK || strings.Contains(oldVersionFilteredRec.Body.String(), productionExecution.ID) {
		t.Fatalf("expected old version filter to exclude latest execution, got %d: %s", oldVersionFilteredRec.Code, oldVersionFilteredRec.Body.String())
	}

	nodesRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(nodesRec, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+productionExecution.ID+"/nodes", nil))
	if nodesRec.Code != http.StatusOK || !strings.Contains(nodesRec.Body.String(), `"nodeId":"event-in"`) || !strings.Contains(nodesRec.Body.String(), `"status":"SUCCESS"`) {
		t.Fatalf("expected node execution list, got %d: %s", nodesRec.Code, nodesRec.Body.String())
	}
	summaryNodesRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(summaryNodesRec, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+productionExecution.ID+"/nodes?payload=summary", nil))
	if summaryNodesRec.Code != http.StatusOK || !strings.Contains(summaryNodesRec.Body.String(), `"nodeId":"event-in"`) || strings.Contains(summaryNodesRec.Body.String(), `"input":`) || strings.Contains(summaryNodesRec.Body.String(), `"output":`) {
		t.Fatalf("expected summary node execution list without payload, got %d: %s", summaryNodesRec.Code, summaryNodesRec.Body.String())
	}
	pagedNodesRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(pagedNodesRec, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+productionExecution.ID+"/nodes?payload=summary&limit=1&offset=0", nil))
	if pagedNodesRec.Code != http.StatusOK || !strings.Contains(pagedNodesRec.Body.String(), `"limit":1`) || !strings.Contains(pagedNodesRec.Body.String(), `"hasMore":true`) || !strings.Contains(pagedNodesRec.Body.String(), `"nextOffset":1`) {
		t.Fatalf("expected paged node execution list metadata, got %d: %s", pagedNodesRec.Code, pagedNodesRec.Body.String())
	}
	var nodeList struct {
		Items []marketing.WorkflowNodeExecution `json:"items"`
	}
	if err := json.NewDecoder(nodesRec.Body).Decode(&nodeList); err != nil {
		t.Fatalf("decode node execution list: %v", err)
	}
	if len(nodeList.Items) == 0 {
		t.Fatalf("expected at least one node execution")
	}
	nodeDetailRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(nodeDetailRec, httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/executions/"+productionExecution.ID+"/nodes/"+nodeList.Items[0].ID, nil))
	if nodeDetailRec.Code != http.StatusOK || !strings.Contains(nodeDetailRec.Body.String(), `"nodeId":"event-in"`) || !strings.Contains(nodeDetailRec.Body.String(), `"output":`) {
		t.Fatalf("expected node execution detail with payload, got %d: %s", nodeDetailRec.Code, nodeDetailRec.Body.String())
	}
}

func TestCreateActivityRejectsDuplicateCodeInSameTenant(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	rec = postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
}

func TestGetAndUpdateActivityMainConfig(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var created marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created activity: %v", err)
	}

	getRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+created.ID, nil))
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), `"activityCode":"FIRST_DEPOSIT_BONUS"`) {
		t.Fatalf("expected activity detail, got %d: %s", getRec.Code, getRec.Body.String())
	}

	future := time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	updateBody := strings.ReplaceAll(`{
		"activityName": "Updated Activity",
		"status": "PAUSED",
		"startTime": "2026-05-02T00:00:00Z",
		"endTime": "__FUTURE_TIME__",
		"timezone": "Asia/Shanghai",
		"priority": 9,
		"exclusiveGroup": "updated_group",
		"stackable": true,
		"configJson": {"metadata": {"owner": "retention"}}
	}`, "__FUTURE_TIME__", future)
	updateRec := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+created.ID, strings.NewReader(updateBody))
	handler.Routes().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d: %s", http.StatusOK, updateRec.Code, updateRec.Body.String())
	}
	var updated marketing.Activity
	if err := json.NewDecoder(updateRec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated activity: %v", err)
	}
	if updated.ID != created.ID || updated.TenantID != created.TenantID || updated.Code != created.Code || updated.Type != created.Type {
		t.Fatalf("identity fields drifted: created=%+v updated=%+v", created, updated)
	}
	if updated.Name != "Updated Activity" || updated.Status != marketing.ActivityPaused || updated.Timezone != "Asia/Shanghai" || updated.Priority != 9 || !updated.Stackable || updated.Version != created.Version+1 {
		t.Fatalf("unexpected updated activity: %+v", updated)
	}
	if !strings.Contains(string(updated.Config), `"retention"`) {
		t.Fatalf("expected configJson update, got: %s", string(updated.Config))
	}
}

func TestUpdateActivityRejectsIdentityFieldsAndMissingActivity(t *testing.T) {
	handler := newTestHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/act_missing", strings.NewReader(`{"activityName":"Updated"}`))
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing update status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
	}

	activityID := createActivity(t, handler)
	for _, body := range []string{
		`{"tenantId":"other"}`,
		`{"activityCode":"OTHER"}`,
		`{"activityType":"OTHER"}`,
		`{"id":"other"}`,
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID, strings.NewReader(body))
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected identity update status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
		}
	}
}

func TestActivityApprovalFlowBlocksGoLiveUntilRequiredApprovals(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}

	updateRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID, strings.NewReader(`{"operator":"ops","status":"ACTIVE"}`)))
	if updateRec.Code != http.StatusBadRequest || !strings.Contains(updateRec.Body.String(), "APPROVAL_REQUIRED") {
		t.Fatalf("expected unapproved activation block, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	submitRec := postApprovalAction(t, handler, "/admin/v1/activities/"+activity.ID+"/approval/submit", `{"operator":"ops","expectedVersion":1}`)
	if submitRec.Code != http.StatusOK || !strings.Contains(submitRec.Body.String(), `"status":"PENDING_L1"`) || !strings.Contains(submitRec.Body.String(), `"highRisk":true`) {
		t.Fatalf("expected PENDING_L1 high-risk approval, got %d: %s", submitRec.Code, submitRec.Body.String())
	}
	selfRec := postApprovalAction(t, handler, "/admin/v1/activities/"+activity.ID+"/approval/approve", `{"operator":"ops"}`)
	if selfRec.Code != http.StatusBadRequest || !strings.Contains(selfRec.Body.String(), "APPROVAL_SAME_OPERATOR") {
		t.Fatalf("expected self approval block, got %d: %s", selfRec.Code, selfRec.Body.String())
	}
	l1Rec := postApprovalAction(t, handler, "/admin/v1/activities/"+activity.ID+"/approval/approve", `{"operator":"lead"}`)
	if l1Rec.Code != http.StatusOK || !strings.Contains(l1Rec.Body.String(), `"status":"PENDING_L2"`) {
		t.Fatalf("expected high-risk l1 to require l2, got %d: %s", l1Rec.Code, l1Rec.Body.String())
	}
	updateRec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID, strings.NewReader(`{"operator":"ops","status":"ACTIVE"}`)))
	if updateRec.Code != http.StatusBadRequest || !strings.Contains(updateRec.Body.String(), "APPROVAL_REQUIRED") {
		t.Fatalf("expected l1-only activation block, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
	l2Rec := postApprovalAction(t, handler, "/admin/v1/activities/"+activity.ID+"/approval/approve", `{"operator":"director","reason":"ok"}`)
	if l2Rec.Code != http.StatusOK || !strings.Contains(l2Rec.Body.String(), `"status":"APPROVED"`) {
		t.Fatalf("expected final approval, got %d: %s", l2Rec.Code, l2Rec.Body.String())
	}
	updateRec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID, strings.NewReader(`{"operator":"ops","status":"ACTIVE"}`)))
	if updateRec.Code != http.StatusOK || !strings.Contains(updateRec.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("expected approved activation, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	recordsRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recordsRec, httptest.NewRequest(http.MethodGet, "/admin/v1/activities/"+activity.ID+"/approval-records", nil))
	if recordsRec.Code != http.StatusOK || !strings.Contains(recordsRec.Body.String(), `"action":"APPROVAL_SUBMIT"`) || !strings.Contains(recordsRec.Body.String(), `"action":"APPROVAL_APPROVE"`) {
		t.Fatalf("expected auditable approval records, got %d: %s", recordsRec.Code, recordsRec.Body.String())
	}
}

func TestApprovedScheduledActivityIsNotMatchedByFormalEvent(t *testing.T) {
	handler := newTestHandler()
	rec := postActivity(t, handler, validActivityBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	approveAndActivateActivity(t, handler, activity.ID, activity.Version)
	updateRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(updateRec, httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activity.ID, strings.NewReader(`{"operator":"ops","status":"SCHEDULED"}`)))
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected scheduled status update, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
	eventRec := postRawEvent(t, handler, depositEventJSON("evt_scheduled_approved", "pay_scheduled_approved", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_scheduled_approved")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected accepted event, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	if strings.Contains(eventRec.Body.String(), `"grantOrders":[{`) || strings.Contains(eventRec.Body.String(), `"participations":[{`) {
		t.Fatalf("scheduled activity must not be formally matched, got: %s", eventRec.Body.String())
	}
}

func TestAuditQueryFiltersAndSnapshots(t *testing.T) {
	handler := newTestHandler()
	body := strings.Replace(validActivityBody(), "{", `{"operator":"ops-a",`, 1)
	rec := postActivity(t, handler, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&action=CREATE_ACTIVITY&actor=ops-a&activityId="+activity.ID, nil))
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit status %d, got %d: %s", http.StatusOK, auditRec.Code, auditRec.Body.String())
	}
	for _, want := range []string{`"actor":"ops-a"`, `"action":"CREATE_ACTIVITY"`, `"resource":"ACTIVITY"`, `"resourceId":"` + activity.ID + `"`, `"beforeSnapshot":null`, `"afterSnapshot":{`, `"source":{`} {
		if !strings.Contains(auditRec.Body.String(), want) {
			t.Fatalf("expected audit %s, got: %s", want, auditRec.Body.String())
		}
	}

	otherTenant := httptest.NewRecorder()
	handler.Routes().ServeHTTP(otherTenant, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=other&action=CREATE_ACTIVITY", nil))
	if otherTenant.Code != http.StatusOK || strings.Contains(otherTenant.Body.String(), activity.ID) {
		t.Fatalf("expected tenant-scoped audit filter, got %d: %s", otherTenant.Code, otherTenant.Body.String())
	}
}

func TestAuditQueryTimeTraceAndRequestFilters(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	early := time.Date(2026, 5, 29, 9, 0, 0, 0, time.UTC)
	matching := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	late := time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC)
	for _, record := range []marketing.AuditRecord{
		{
			ID:         "audit_early",
			TenantID:   "site_001",
			Actor:      "ops",
			Action:     "CREATE_ACTIVITY",
			Resource:   "ACTIVITY",
			ResourceID: "act_early",
			Payload:    []byte(`{"source":{"traceId":"trace-old","requestId":"req-old"},"activityId":"act_early"}`),
			CreatedAt:  early,
		},
		{
			ID:         "audit_match",
			TenantID:   "site_001",
			Actor:      "ops",
			Action:     "MANUAL_GRANT_DECISION",
			Resource:   "GRANT_ORDER",
			ResourceID: "grant_match",
			Payload:    []byte(`{"source":{"traceId":"trace-123","requestId":"req-456"},"commandId":"cmd_match"}`),
			CreatedAt:  matching,
		},
		{
			ID:         "audit_late",
			TenantID:   "site_001",
			Actor:      "ops",
			Action:     "MANUAL_GRANT_DECISION",
			Resource:   "GRANT_ORDER",
			ResourceID: "grant_late",
			Payload:    []byte(`{"source":{"traceId":"trace-123","requestId":"req-other"},"commandId":"cmd_late"}`),
			CreatedAt:  late,
		},
	} {
		if err := repo.CreateAudit(context.Background(), record); err != nil {
			t.Fatalf("create audit: %v", err)
		}
	}

	rec := httptest.NewRecorder()
	path := "/admin/v1/audit?tenantId=site_001&startTime=2026-05-29T09:30:00Z&endTime=2026-05-29T10:30:00Z&traceId=trace-123&requestId=req-456"
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected audit OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"audit_match"`) || strings.Contains(rec.Body.String(), "audit_early") || strings.Contains(rec.Body.String(), "audit_late") {
		t.Fatalf("expected service-side time trace/request filter, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&startTime=not-a-time", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid time status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}

func TestCreateActivityRejectsUnsupportedRewardDelivery(t *testing.T) {
	handler := newTestHandler()
	for name, body := range map[string]string{
		"missing targetRef": strings.Replace(validActivityBody(), `,
					"targetRef": "https://wallet.example.com/reward"`, "", 1),
		"mq channel":     strings.Replace(validActivityBody(), `"grantChannel": "HTTP"`, `"grantChannel": "MQ"`, 1),
		"manual mode":    strings.Replace(validActivityBody(), `"grantMode": "AUTO"`, `"grantMode": "MANUAL"`, 1),
		"unknown config": strings.Replace(validActivityBody(), `"targetRef": "https://wallet.example.com/reward"`, `"targetRef": "https://wallet.example.com/reward", "unknown": true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			rec := postActivity(t, handler, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateActivityRejectsInvalidConfigAndRuleSchema(t *testing.T) {
	handler := newTestHandler()
	for name, body := range map[string]string{
		"duplicate trigger": strings.Replace(validActivityBody(), `"triggerEvents": ["DEPOSIT_SUCCESS"]`, `"triggerEvents": ["DEPOSIT_SUCCESS", "DEPOSIT_SUCCESS"]`, 1),
		"unknown config":    strings.Replace(validActivityBody(), `"metadata": {"owner": "growth"}`, `"metadata": {"owner": "growth"}, "quota": {}`, 1),
		"bad rule op":       strings.Replace(validActivityBody(), `"op": ">="`, `"op": "BETWEEN"`, 1),
		"bad version":       strings.Replace(validActivityBody(), `"version": 1,`, `"version": 2,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			rec := postActivity(t, handler, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateActivityValidatesDisplayEntryAudienceConfig(t *testing.T) {
	handler := newTestHandler()
	valid := strings.Replace(validActivityBody(), `"entranceCodes": ["deposit_page"],
				"landingUrl": "https://example.com/promo/first-deposit"`, `"enabled": true,
				"entryCodes": ["HOME_BANNER", "PROMOTION_CENTER"],
				"channels": ["APP", "WEB"],
				"sourceSystems": ["payment"],
				"tenantIds": ["site_001"],
				"audience": {
					"subjectTypes": ["USER"],
					"segments": ["new_user"],
					"riskLevels": ["NORMAL"]
				}`, 1)
	valid = strings.Replace(valid, `"title": "First Deposit Bonus",`, `"visible": false,
				"title": "First Deposit Bonus",
				"description": "activity description",
				"tags": ["new_user", "deposit"],`, 1)

	rec := postActivity(t, handler, valid)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected valid display/entry config, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"visible":false`) || !strings.Contains(rec.Body.String(), `"entryCodes"`) || !strings.Contains(rec.Body.String(), `"subjectTypes"`) {
		t.Fatalf("expected config to round-trip, got: %s", rec.Body.String())
	}

	for name, body := range map[string]string{
		"display object": strings.Replace(validActivityBody(), `"display": {`, `"display": "bad"`, 1),
		"enabled bool":   strings.Replace(validActivityBody(), `"entry": {`, `"entry": {"enabled": "yes",`, 1),
		"entry array":    strings.Replace(validActivityBody(), `"channels": ["APP", "WEB"]`, `"channels": "APP"`, 1),
		"tenant scope":   strings.Replace(validActivityBody(), `"channels": ["APP", "WEB"]`, `"channels": ["APP", "WEB"], "tenantIds": ["site_002"]`, 1),
		"banner url":     strings.Replace(validActivityBody(), `"bannerUrl": "https://example.com/banner.png"`, `"bannerUrl": "ftp://example.com/banner.png"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			rec := postActivity(t, handler, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateActivityValidatesFactProviderReferences(t *testing.T) {
	missing := strings.Replace(validActivityBody(), `"attributes.vipLevel"`, `"facts.USER_PROFILE.vipLevel"`, 1)
	rec := postActivity(t, newTestHandler(), missing)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "USER_PROFILE") {
		t.Fatalf("expected missing fact provider rejection, got %d: %s", rec.Code, rec.Body.String())
	}

	repo := marketing.NewMemoryRepository()
	_, err := repo.SaveFactProvider(context.Background(), marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping:      map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "number"}},
		Enabled:      true,
		TimeoutMS:    5000,
		UpdatedAt:    time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	rec = postActivity(t, newTestHandlerWithRepository(repo), missing)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected activity with configured provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateActivityRejectsDisabledAndMissingFactMapping(t *testing.T) {
	activityBody := strings.Replace(validActivityBody(), `"attributes.vipLevel"`, `"facts.USER_PROFILE.risk.score"`, 1)
	repo := marketing.NewMemoryRepository()
	_, err := repo.SaveFactProvider(context.Background(), marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping:      map[string]marketing.FactMapping{"risk": {Source: "$.risk", Type: "object"}},
		Enabled:      true,
		TimeoutMS:    5000,
		UpdatedAt:    time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	rec := postActivity(t, newTestHandlerWithRepository(repo), activityBody)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "FACT_FIELD_MISSING") {
		t.Fatalf("expected missing mapped field rejection, got %d: %s", rec.Code, rec.Body.String())
	}

	repo = marketing.NewMemoryRepository()
	_, err = repo.SaveFactProvider(context.Background(), marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping:      map[string]marketing.FactMapping{"risk.score": {Source: "$.risk.score", Type: "number"}},
		Enabled:      false,
		TimeoutMS:    5000,
		UpdatedAt:    time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	rec = postActivity(t, newTestHandlerWithRepository(repo), activityBody)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "FACT_PROVIDER_DISABLED") {
		t.Fatalf("expected disabled provider rejection, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFactQueryRejectsInlineProviderAndUsesRegistry(t *testing.T) {
	handler := newTestHandler()
	inlineBody := `{
		"tenantId": "site_001",
		"factCode": "USER_PROFILE",
		"provider": {"endpoint": "https://example.com/profile"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/facts/query", strings.NewReader(inlineBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected inline provider to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}

	body := `{
		"tenantId": "site_001",
		"factCode": "USER_PROFILE",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_1"},
		"attributes": {"amount": "100"},
		"traceId": "trace_1"
	}`
	req = httptest.NewRequest(http.MethodPost, "/openapi/v1/facts/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "FACT_PROVIDER_NOT_CONFIGURED") {
		t.Fatalf("expected registry lookup failure, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFactQueryReturnsDisabledProviderCode(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	_, err := repo.SaveFactProvider(context.Background(), marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping:      map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		Enabled:      false,
		TimeoutMS:    5000,
		UpdatedAt:    time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	body := `{
		"tenantId": "site_001",
		"factCode": "USER_PROFILE",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_1"},
		"attributes": {}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/facts/query", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newTestHandlerWithRepository(repo).Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "FACT_PROVIDER_DISABLED") {
		t.Fatalf("expected disabled provider code, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPrelaunchValidationBlocksMissingFactProvider(t *testing.T) {
	activity := strings.Replace(validActivityBody(), `"attributes.vipLevel"`, `"facts.USER_PROFILE.vipLevel"`, 1)
	body := `{"activity":` + activity + `,"targetAction":"GO_LIVE"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/prelaunch-validation", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newTestHandler().Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status OK, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"conclusion":"BLOCK"`) || !strings.Contains(rec.Body.String(), "FACT_PROVIDER_NOT_CONFIGURED") {
		t.Fatalf("expected prelaunch fact provider block, got: %s", rec.Body.String())
	}
}

func TestActivityEventCreatesGrantCommand(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)

	event := `{
		"eventId": "evt_20260524_000001",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_123"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"channel": "APP",
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"grantOrders"`) {
		t.Fatalf("expected grant orders, got: %s", body)
	}
	if !strings.Contains(body, `"rewardAmount":"59.8000"`) {
		t.Fatalf("expected ratio reward amount, got: %s", body)
	}
	if !strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected outbox message, got: %s", body)
	}

	var result marketing.ProcessEventResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result.Participations) != 1 || len(result.GrantOrders) != 1 {
		t.Fatalf("expected one participation and grant, got participations=%d grants=%d", len(result.Participations), len(result.GrantOrders))
	}
	expectedParticipationKey := "site_001:" + activityID + ":site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_123"
	if result.Participations[0].IdempotencyKey != expectedParticipationKey {
		t.Fatalf("unexpected participation idempotency key: %s", result.Participations[0].IdempotencyKey)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(result.Participations[0].Snapshot, &snapshot); err != nil {
		t.Fatalf("decode participation snapshot: %v", err)
	}
	for _, key := range []string{"activity", "event", "ruleResult", "rewards"} {
		if _, ok := snapshot[key]; !ok {
			t.Fatalf("expected snapshot key %s, got: %s", key, string(result.Participations[0].Snapshot))
		}
	}
	var rewardPayload map[string]any
	if err := json.Unmarshal(result.GrantOrders[0].RewardPayload, &rewardPayload); err != nil {
		t.Fatalf("decode reward payload: %v", err)
	}
	for _, key := range []string{"rewardCode", "rewardType", "calculateType", "currency", "flowMultiple", "targetRef"} {
		if _, ok := rewardPayload[key]; !ok {
			t.Fatalf("expected reward payload key %s, got: %s", key, string(result.GrantOrders[0].RewardPayload))
		}
	}
}

func TestActivityEventOpenAPIAuthRejectsUnauthorizedBeforeInboxWrite(t *testing.T) {
	handler := newAuthTestHandler("secret-token", []string{"site_001"}, []string{"payment"}, defaultEventMaxBodyBytes)
	event := depositEventJSON("evt_auth_missing", "pay_auth_missing", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	key := "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_auth_missing"

	tests := []struct {
		name          string
		authorization string
	}{
		{name: "missing"},
		{name: "basic", authorization: "Basic secret-token"},
		{name: "empty bearer", authorization: "Bearer "},
		{name: "unknown bearer", authorization: "Bearer wrong-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := eventRequest(event, key)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `"errorCode":"UNAUTHORIZED"`) || !strings.Contains(body, `"message":"unauthorized"`) {
				t.Fatalf("expected unauthorized error body, got: %s", body)
			}
			if strings.Contains(body, "secret-token") || strings.Contains(body, "Authorization") {
				t.Fatalf("error body leaked sensitive auth data: %s", body)
			}
		})
	}

	valid := eventRequest(event, key)
	valid.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, valid)
	if rec.Code != http.StatusAccepted || strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected valid request accepted after auth failures without duplicate, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestActivityEventOpenAPIAuthTenantAndSourceAuthorization(t *testing.T) {
	handler := newAuthTestHandler("secret-token", []string{"site_001"}, []string{"payment"}, defaultEventMaxBodyBytes)

	tenantEvent := strings.Replace(depositEventJSON("evt_tenant_forbidden", "pay_tenant_forbidden", `{}`), `"tenantId": "site_001"`, `"tenantId": "site_002"`, 1)
	rec := postAuthorizedRawEvent(t, handler, tenantEvent, "site_002:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_tenant_forbidden", "secret-token")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"errorCode":"TENANT_FORBIDDEN"`) {
		t.Fatalf("expected tenant forbidden, got %d: %s", rec.Code, rec.Body.String())
	}

	sourceEvent := strings.Replace(depositEventJSON("evt_source_forbidden", "pay_source_forbidden", `{}`), `"sourceSystem": "payment"`, `"sourceSystem": "order"`, 1)
	rec = postAuthorizedRawEvent(t, handler, sourceEvent, "site_001:order:DEPOSIT_SUCCESS:DEPOSIT:pay_source_forbidden", "secret-token")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"errorCode":"SOURCE_FORBIDDEN"`) {
		t.Fatalf("expected source forbidden, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestActivityEventContentTypeInvalidJSONAndBodyLimit(t *testing.T) {
	handler := newTestHandler()
	event := depositEventJSON("evt_media", "pay_media", `{}`)
	key := "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_media"

	req := eventRequest(event, key)
	req.Header.Del("Content-Type")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), `"errorCode":"UNSUPPORTED_MEDIA_TYPE"`) {
		t.Fatalf("expected unsupported media type for missing content-type, got %d: %s", rec.Code, rec.Body.String())
	}

	req = eventRequest(event, key)
	req.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType || !strings.Contains(rec.Body.String(), `"errorCode":"UNSUPPORTED_MEDIA_TYPE"`) {
		t.Fatalf("expected unsupported media type for text/plain, got %d: %s", rec.Code, rec.Body.String())
	}

	req = eventRequest(`{"eventId":`, key)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"errorCode":"INVALID_JSON"`) {
		t.Fatalf("expected invalid json, got %d: %s", rec.Code, rec.Body.String())
	}

	limited := newAuthTestHandler("secret-token", []string{"site_001"}, []string{"payment"}, 64)
	req = eventRequest(event, key)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec = httptest.NewRecorder()
	limited.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"errorCode":"REQUEST_TOO_LARGE"`) {
		t.Fatalf("expected request too large, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIEventConfigFromEnv(t *testing.T) {
	t.Setenv("EX_MARKETING_REQUIRE_OPENAPI_AUTH", "true")
	t.Setenv("EX_MARKETING_OPENAPI_TOKENS", "token-a:site_001,site_002:payment,order")
	t.Setenv("EX_MARKETING_EVENT_MAX_BODY_BYTES", "2048")

	cfg, err := LoadOpenAPIEventConfigFromEnv(nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.RequireAuth || cfg.MaxBodyBytes != 2048 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	credential := cfg.Credentials["token-a"]
	if _, ok := credential.TenantIDs["site_002"]; !ok {
		t.Fatalf("expected site_002 tenant authorization: %+v", credential)
	}
	if _, ok := credential.SourceSystems["order"]; !ok {
		t.Fatalf("expected order source authorization: %+v", credential)
	}

	t.Setenv("EX_MARKETING_OPENAPI_TOKENS", "")
	if _, err := LoadOpenAPIEventConfigFromEnv(nil); err == nil {
		t.Fatal("expected require-auth config without tokens to fail")
	}
}

func TestActivityEventRuleRejectionReturnsExplainWithoutGrant(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	rec := postRawEvent(t, handler, depositEventJSON("evt_rule_reject", "pay_rule_reject", `"amount":"299.00","vipLevel":3,"isFirstDeposit":false`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_rule_reject")

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"status":"REJECTED"`,
		`"rejectReason":"CONDITION_NOT_MET: attributes.isFirstDeposit != true"`,
		`"ruleResults"`,
		`"stage":"QUALIFY"`,
		`"ruleCode":"FIRST_DEPOSIT_AND_AMOUNT"`,
		`"reasonCode":"CONDITION_NOT_MET"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in response, got: %s", want, body)
		}
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) {
		t.Fatalf("expected no grant/outbox on rule rejection, got: %s", body)
	}
}

func TestSimulateEventReturnsRuleExplainWithoutPersistence(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	event := depositEventJSON("evt_sim_reject", "pay_sim_reject", `"amount":"50.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ruleResults"`) || !strings.Contains(body, `"failedStage":"QUALIFY"`) || !strings.Contains(body, `"field":"attributes.amount"`) {
		t.Fatalf("expected simulation rule explain, got: %s", body)
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) {
		t.Fatalf("expected no simulated grants after rejection, got: %s", body)
	}
}

func TestEntryConfigFiltersSimulationAndProcessEvent(t *testing.T) {
	handler := newTestHandler()
	disabledBody := strings.Replace(validActivityBody(), `"entry": {`, `"entry": {"enabled": false,`, 1)
	rec := postActivity(t, handler, disabledBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected activity create, got %d: %s", rec.Code, rec.Body.String())
	}
	approveAndActivateCreatedActivity(t, handler, rec)

	event := depositEventJSON("evt_entry_disabled", "pay_entry_disabled", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	simRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(simRec, httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event)))
	if simRec.Code != http.StatusOK {
		t.Fatalf("expected simulation status %d, got %d: %s", http.StatusOK, simRec.Code, simRec.Body.String())
	}
	if !strings.Contains(simRec.Body.String(), `"decisionReasonCode":"ENTRY_DISABLED"`) || strings.Contains(simRec.Body.String(), `"grantOrderPreviews":[{`) {
		t.Fatalf("expected entry disabled rejection without grant preview, got: %s", simRec.Body.String())
	}

	eventRec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_entry_disabled")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event status %d, got %d: %s", http.StatusAccepted, eventRec.Code, eventRec.Body.String())
	}
	for _, want := range []string{
		`"status":"REJECTED"`,
		`"rejectReason":"ENTRY_DISABLED: activity entry is disabled"`,
		`"reasonCode":"ENTRY_DISABLED"`,
		`"snapshot":`,
	} {
		if !strings.Contains(eventRec.Body.String(), want) {
			t.Fatalf("expected %s in process event entry rejection, got: %s", want, eventRec.Body.String())
		}
	}
	if strings.Contains(eventRec.Body.String(), `"grantOrders":[{`) || strings.Contains(eventRec.Body.String(), `"outboxMessages":[{`) {
		t.Fatalf("expected process event entry rejection without grant/outbox, got: %s", eventRec.Body.String())
	}
}

func TestEntryConfigMismatchReasons(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		event  string
		reason string
	}{
		{
			name:   "channel",
			body:   validActivityBody(),
			event:  depositEventJSON("evt_entry_channel", "pay_entry_channel", `"channel":"KIOSK","amount":"299.00","vipLevel":3,"isFirstDeposit":true`),
			reason: "ENTRY_CHANNEL_NOT_ALLOWED",
		},
		{
			name:   "source system",
			body:   strings.Replace(validActivityBody(), `"channels": ["APP", "WEB"]`, `"channels": ["APP", "WEB"], "sourceSystems": ["wallet"]`, 1),
			event:  depositEventJSON("evt_entry_source", "pay_entry_source", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`),
			reason: "ENTRY_SOURCE_SYSTEM_NOT_ALLOWED",
		},
		{
			name:   "subject type",
			body:   strings.Replace(validActivityBody(), `"channels": ["APP", "WEB"]`, `"channels": ["APP", "WEB"], "audience": {"subjectTypes": ["ACCOUNT"]}`, 1),
			event:  depositEventJSON("evt_entry_subject", "pay_entry_subject", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`),
			reason: "AUDIENCE_SUBJECT_TYPE_NOT_ALLOWED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestHandler()
			rec := postActivity(t, handler, tt.body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected activity create, got %d: %s", rec.Code, rec.Body.String())
			}
			approveAndActivateCreatedActivity(t, handler, rec)
			simRec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(simRec, httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(tt.event)))
			if simRec.Code != http.StatusOK {
				t.Fatalf("expected simulation status %d, got %d: %s", http.StatusOK, simRec.Code, simRec.Body.String())
			}
			if !strings.Contains(simRec.Body.String(), `"decisionReasonCode":"`+tt.reason+`"`) || strings.Contains(simRec.Body.String(), `"grantOrderPreviews":[{`) {
				t.Fatalf("expected %s rejection without grant preview, got: %s", tt.reason, simRec.Body.String())
			}
		})
	}
}

func TestDisplayVisibleFalseDoesNotBlockEntryMatching(t *testing.T) {
	handler := newTestHandler()
	body := strings.Replace(validActivityBody(), `"display": {`, `"display": {"visible": false,`, 1)
	rec := postActivity(t, handler, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected activity create, got %d: %s", rec.Code, rec.Body.String())
	}
	approveAndActivateCreatedActivity(t, handler, rec)

	rec = postDepositEvent(t, handler, "evt_display_hidden", "pay_display_hidden")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected event status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"grantOrders":[{`) || strings.Contains(rec.Body.String(), "ENTRY_DISABLED") {
		t.Fatalf("expected hidden display to keep reward path available, got: %s", rec.Body.String())
	}
}

func TestSimulateEventReturnsActivityAndQuotaPreviewsWithoutPersistence(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")
	createQuota(t, handler, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "10001", "1")

	event := depositEventJSON("evt_sim_success", "pay_sim_success", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"matchedActivities":[{`,
		`"rewardPreviews":[{`,
		`"quotaImpactPreviews":[{`,
		`"quotaType":"BUDGET"`,
		`"availableBefore":"100.0000"`,
		`"delta":"59.8000"`,
		`"unlimited":false`,
		`"wouldExceed":false`,
		`"participationPreview"`,
		`"grantOrderPreviews":[{`,
		`"outboxCommandPreviews":[{`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in simulation response, got: %s", want, body)
		}
	}

	accepted := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_sim_success")
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("expected formal event after simulation to be accepted, got %d: %s", accepted.Code, accepted.Body.String())
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&action=SIMULATION_RUN&eventId=evt_sim_success", nil))
	if auditRec.Code != http.StatusOK || !strings.Contains(auditRec.Body.String(), `"action":"SIMULATION_RUN"`) {
		t.Fatalf("expected simulation audit, got %d: %s", auditRec.Code, auditRec.Body.String())
	}
}

func TestSimulateEventQuotaExceededSuppressesGrantPreview(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1")

	event := depositEventJSON("evt_sim_quota", "pay_sim_quota", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"hasQuotaRisk":true`,
		`"decisionReasonCode":"QUOTA_EXCEEDED"`,
		`"wouldExceed":true`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %s in quota risk simulation, got: %s", want, body)
		}
	}
	if strings.Contains(body, `"grantOrders":[{`) || strings.Contains(body, `"outboxMessages":[{`) || strings.Contains(body, `"grantOrderPreviews":[{`) || strings.Contains(body, `"outboxCommandPreviews":[{`) {
		t.Fatalf("expected no grant/outbox previews when quota would exceed, got: %s", body)
	}
}

func TestSimulateEventRejectsInvalidStandardEventSchema(t *testing.T) {
	handler := newTestHandler()

	event := strings.Replace(depositEventJSON("evt_sim_invalid_schema", "pay_sim_invalid_schema", `null`), `"occurredAt": "2026-05-24T12:00:00Z"`, `"occurredAt": "2026-05-24T12:00:00"`, 1)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", strings.NewReader(event))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"INVALID_EVENT"`) || !strings.Contains(body, "occurredAt") {
		t.Fatalf("expected INVALID_EVENT with field reason, got: %s", body)
	}
}

func TestActivityEventRequiresIdempotencyKey(t *testing.T) {
	handler := newTestHandler()

	rec := postRawEvent(t, handler, depositEventJSON("evt_missing_idem", "pay_missing_idem", `"amount":"299.00","vipLevel":3,"isFirstDeposit":true`), "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency error code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsMismatchedIdempotencyKey(t *testing.T) {
	handler := newTestHandler()

	rec := postRawEvent(t, handler, depositEventJSON("evt_bad_idem", "pay_bad_idem", `"amount":"299.00"`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:other_pay")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency error code, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsUnknownFields(t *testing.T) {
	handler := newTestHandler()

	event := `{
		"eventId": "evt_unknown",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_unknown"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {},
		"orderAmount": "299.00"
	}`
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_unknown")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
		t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `orderAmount`) {
		t.Fatalf("expected unknown field path, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsNestedUnknownFields(t *testing.T) {
	handler := newTestHandler()

	event := `{
		"eventId": "evt_nested_unknown",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001", "name": "Alice"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_nested_unknown"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {}
	}`
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_nested_unknown")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
		t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `subject.name`) {
		t.Fatalf("expected nested unknown field path, got: %s", rec.Body.String())
	}
}

func TestActivityEventRejectsMissingRequiredField(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name  string
		event string
		key   string
	}{
		{
			name: "missing occurredAt",
			event: `{
				"eventId": "evt_missing_required",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_required"},
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_required",
		},
		{
			name: "missing eventType",
			event: `{
				"eventId": "evt_missing_event_type",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_event_type"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_event_type",
		},
		{
			name: "missing biz",
			event: `{
				"eventId": "evt_missing_biz",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_biz",
		},
		{
			name: "missing attributes",
			event: `{
				"eventId": "evt_missing_attrs",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_missing_attrs"},
				"occurredAt": "2026-05-24T12:00:00Z"
			}`,
			key: "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_missing_attrs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postRawEvent(t, handler, tt.event, tt.key)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"code":"INVALID_EVENT"`) {
				t.Fatalf("expected invalid event code, got: %s", rec.Body.String())
			}
		})
	}
}

func TestActivityEventDuplicateAndConflict(t *testing.T) {
	handler := newTestHandler()
	createActivity(t, handler)

	event := depositEventJSON("evt_duplicate", "pay_duplicate", `"amount":299.00,"currency":"CNY","vipLevel":3,"isFirstDeposit":true`)
	first := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected first status %d, got %d: %s", http.StatusAccepted, first.Code, first.Body.String())
	}

	reordered := `{
		"tenantId": "site_001",
		"sourceSystem": "payment",
		"eventType": "DEPOSIT_SUCCESS",
		"eventId": "evt_duplicate",
		"subject": {"id": "10001", "type": "USER"},
		"biz": {"bizId": "pay_duplicate", "bizType": "DEPOSIT"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"channel": "APP",
			"isFirstDeposit": true,
			"vipLevel": 3.0,
			"currency": "CNY",
			"amount": 299.000
		}
	}`
	second := postRawEvent(t, handler, reordered, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate 200 response, got %d: %s", second.Code, second.Body.String())
	}
	if strings.Contains(second.Body.String(), `"grantOrders":[{`) || strings.Contains(second.Body.String(), `"outboxMessages":[{`) {
		t.Fatalf("expected duplicate response without new grant/outbox, got: %s", second.Body.String())
	}

	conflict := depositEventJSON("evt_conflict", "pay_duplicate", `"amount":299.00,"currency":"CNY","vipLevel":3,"isFirstDeposit":true`)
	conflictRec := postRawEvent(t, handler, conflict, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_duplicate")
	if conflictRec.Code != http.StatusBadRequest {
		t.Fatalf("expected conflict status %d, got %d: %s", http.StatusBadRequest, conflictRec.Code, conflictRec.Body.String())
	}
	if !strings.Contains(conflictRec.Body.String(), `"code":"INVALID_IDEMPOTENCY_KEY"`) {
		t.Fatalf("expected idempotency conflict code, got: %s", conflictRec.Body.String())
	}
}

func TestActivityEventRejectsInvalidSchemaFields(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name    string
		event   string
		key     string
		message string
	}{
		{
			name:    "null attributes",
			event:   depositEventJSON("evt_null_attrs", "pay_null_attrs", `null`),
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_null_attrs",
			message: "attributes must be object",
		},
		{
			name:    "array attributes",
			event:   depositEventJSON("evt_array_attrs", "pay_array_attrs", `[]`),
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_array_attrs",
			message: "attributes must be object",
		},
		{
			name: "subject is string",
			event: `{
				"eventId": "evt_subject_string",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": "10001",
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_subject_string"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_subject_string",
			message: "subject must be object",
		},
		{
			name: "missing subject id",
			event: `{
				"eventId": "evt_subject_missing_id",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_subject_missing_id"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_subject_missing_id",
			message: "subject.id is required",
		},
		{
			name: "biz unknown field",
			event: `{
				"eventId": "evt_biz_unknown",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_biz_unknown", "amount": "299.00"},
				"occurredAt": "2026-05-24T12:00:00Z",
				"attributes": {}
			}`,
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_biz_unknown",
			message: "biz.amount",
		},
		{
			name: "invalid time",
			event: `{
				"eventId": "evt_bad_time",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_bad_time"},
				"occurredAt": "2026-99-99",
				"attributes": {}
			}`,
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_bad_time",
			message: "occurredAt",
		},
		{
			name: "time missing timezone",
			event: `{
				"eventId": "evt_no_tz",
				"eventType": "DEPOSIT_SUCCESS",
				"sourceSystem": "payment",
				"tenantId": "site_001",
				"subject": {"type": "USER", "id": "10001"},
				"biz": {"bizType": "DEPOSIT", "bizId": "pay_no_tz"},
				"occurredAt": "2026-05-24T12:00:00",
				"attributes": {}
			}`,
			key:     "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_no_tz",
			message: "occurredAt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postRawEvent(t, handler, tt.event, tt.key)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, `"code":"INVALID_EVENT"`) || !strings.Contains(body, tt.message) {
				t.Fatalf("expected INVALID_EVENT containing %q, got: %s", tt.message, body)
			}
		})
	}
}

func TestActivityEventAcceptsEmptyAndNestedAttributes(t *testing.T) {
	handler := newTestHandler()

	event := depositEventJSON("evt_empty_attrs", "pay_empty_attrs", `{}`)
	rec := postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_empty_attrs")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}

	event = depositEventJSON("evt_nested_attrs", "pay_nested_attrs", `{"amount":"299.00","risk":{"tags":["vip","first"]},"items":[{"sku":"A"}]}`)
	rec = postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_nested_attrs")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
}

func TestActivityEventBudgetExceededReturnsConflictAndAudit(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "1")

	event := `{
		"eventId": "evt_20260524_000002",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "pay_124"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": {
			"channel": "APP",
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_124")
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "quota exceeded") || !strings.Contains(body, "BUDGET") {
		t.Fatalf("expected budget quota error, got: %s", body)
	}
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"BUDGET"`) {
		t.Fatalf("expected structured quota error, got: %s", body)
	}
	if strings.Contains(body, `"grantOrders"`) || strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected no grant/outbox response on quota failure, got: %s", body)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001", nil)
	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit status %d, got %d: %s", http.StatusOK, auditRec.Code, auditRec.Body.String())
	}
	if !strings.Contains(auditRec.Body.String(), `"action":"QUOTA_EXCEEDED"`) {
		t.Fatalf("expected quota audit record, got: %s", auditRec.Body.String())
	}
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"BUDGET"`) {
		t.Fatalf("expected structured quota audit payload, got: %s", auditRec.Body.String())
	}
}

func TestActivityEventUserLimitExceededReturnsConflictAndAudit(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "10001", "0")

	rec := postDepositEvent(t, handler, "evt_user_limit_exceeded", "pay_user_limit_exceeded")

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"USER_LIMIT"`) || !strings.Contains(body, `"scopeKey":"10001"`) {
		t.Fatalf("expected user limit quota error, got: %s", body)
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001", nil))
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"USER_LIMIT"`) {
		t.Fatalf("expected user limit audit payload, got: %s", auditRec.Body.String())
	}
}

func TestActivityEventStockExceededReturnsConflictAndNoRewardableResult(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "0")

	rec := postDepositEvent(t, handler, "evt_stock_exceeded", "pay_stock_exceeded")

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"QUOTA_EXCEEDED"`) || !strings.Contains(body, `"quotaType":"STOCK"`) || !strings.Contains(body, `"scopeKey":"FIRST_DEPOSIT_RATIO"`) {
		t.Fatalf("expected stock quota error, got: %s", body)
	}
	if strings.Contains(body, `"status":"REWARDABLE"`) || strings.Contains(body, `"grantOrders"`) || strings.Contains(body, `"outboxMessages"`) {
		t.Fatalf("expected no rewardable/grant/outbox response on stock failure, got: %s", body)
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001", nil))
	if !strings.Contains(auditRec.Body.String(), `"quotaType":"STOCK"`) {
		t.Fatalf("expected stock audit payload, got: %s", auditRec.Body.String())
	}
}

func TestRewardCallbackSuccessConfirmsQuotaAndIsIdempotent(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	eventRec := postDepositEvent(t, handler, "evt_callback_success", "pay_callback_success")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var eventResult marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &eventResult); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := eventResult.GrantOrders[0].BillNo
	markGrantOrderSending(t, repo, commandID)

	rec := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_001",
		"message": "paid",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected callback OK, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"grantStatus":"SUCCESS"`, `"participationStatus":"GRANTED"`, `"reservationStatus":"CONFIRMED"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected %s, got: %s", want, rec.Body.String())
		}
	}

	rec = postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_001",
		"message": "paid again",
		"finishedAt": "2026-05-24T12:06:00Z"
	}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate success callback, got %d: %s", rec.Code, rec.Body.String())
	}

	conflict := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_002",
		"message": "paid under another wallet id",
		"finishedAt": "2026-05-24T12:07:00Z"
	}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"CALLBACK_STATUS_CONFLICT"`) {
		t.Fatalf("expected externalGrantId conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}

	reconcile := httptest.NewRecorder()
	handler.Routes().ServeHTTP(reconcile, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=site_001&commandId="+commandID, nil))
	if reconcile.Code != http.StatusOK {
		t.Fatalf("expected reconciliation OK, got %d: %s", reconcile.Code, reconcile.Body.String())
	}
	for _, want := range []string{`"commandId":"` + commandID + `"`, `"externalGrantId":"wallet_001"`, `"grantStatus":"SUCCESS"`, `"reservationStatus":"CONFIRMED"`} {
		if !strings.Contains(reconcile.Body.String(), want) {
			t.Fatalf("expected reconciliation %s, got: %s", want, reconcile.Body.String())
		}
	}
}

func TestRewardReconciliationSupportsActivitySubjectAndEventQueries(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")
	createQuota(t, handler, activityID, marketing.QuotaUserLimit, marketing.QuotaScopeUser, "10001", "1")

	eventRec := postDepositEvent(t, handler, "evt_reconcile_dims", "pay_reconcile_dims")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var result marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := result.GrantOrders[0].BillNo

	for _, path := range []string{
		"/admin/v1/reward/reconciliation?tenantId=site_001&activityId=" + activityID,
		"/admin/v1/reward/reconciliation?tenantId=site_001&subjectType=USER&subjectId=10001",
		"/admin/v1/reward/reconciliation?tenantId=site_001&eventId=evt_reconcile_dims",
		"/admin/v1/reward/reconciliation?tenantId=site_001&idempotencyKey=site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_reconcile_dims",
	} {
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected reconciliation OK for %s, got %d: %s", path, rec.Code, rec.Body.String())
		}
		for _, want := range []string{`"commandId":"` + commandID + `"`, `"eventInbox":{`, `"eventId":"evt_reconcile_dims"`, `"outboxMessages":[{`, `"quotaReservations":[{`, `"auditTimeline":[`} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("expected reconciliation %s for %s, got: %s", want, path, rec.Body.String())
			}
		}
	}

	otherTenant := httptest.NewRecorder()
	handler.Routes().ServeHTTP(otherTenant, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=other&commandId="+commandID, nil))
	if otherTenant.Code != http.StatusOK || strings.Contains(otherTenant.Body.String(), commandID) {
		t.Fatalf("expected tenant-scoped reconciliation, got %d: %s", otherTenant.Code, otherTenant.Body.String())
	}
}

func TestRewardReconciliationValidatesQueryAndSupportsPartialLinks(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	invalid := httptest.NewRecorder()
	handler.Routes().ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=site_001", nil))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"INVALID_RECONCILIATION_QUERY"`) {
		t.Fatalf("expected reconciliation validation error, got %d: %s", invalid.Code, invalid.Body.String())
	}

	rejectedEvent := postRawEvent(t, handler, depositEventJSON("evt_reconcile_rejected", "pay_reconcile_rejected", `"amount":"299.00","vipLevel":3,"isFirstDeposit":false`), "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_reconcile_rejected")
	if rejectedEvent.Code != http.StatusAccepted {
		t.Fatalf("expected rejected event accepted, got %d: %s", rejectedEvent.Code, rejectedEvent.Body.String())
	}
	var rejectedResult marketing.ProcessEventResult
	if err := json.Unmarshal(rejectedEvent.Body.Bytes(), &rejectedResult); err != nil {
		t.Fatalf("decode rejected event result: %v", err)
	}
	if len(rejectedResult.Participations) != 1 {
		t.Fatalf("expected rejected participation, got: %s", rejectedEvent.Body.String())
	}

	byParticipation := httptest.NewRecorder()
	handler.Routes().ServeHTTP(byParticipation, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=site_001&participationId="+rejectedResult.Participations[0].ID, nil))
	if byParticipation.Code != http.StatusOK {
		t.Fatalf("expected participation reconciliation OK, got %d: %s", byParticipation.Code, byParticipation.Body.String())
	}
	for _, want := range []string{`"participationId":"` + rejectedResult.Participations[0].ID + `"`, `"participationStatus":"REJECTED"`, `"diagnostics":["PARTICIPATION_REJECTED"]`} {
		if !strings.Contains(byParticipation.Body.String(), want) {
			t.Fatalf("expected rejected reconciliation %s, got: %s", want, byParticipation.Body.String())
		}
	}

	successEvent := postDepositEvent(t, handler, "evt_reconcile_outbox", "pay_reconcile_outbox")
	if successEvent.Code != http.StatusAccepted {
		t.Fatalf("expected success event accepted, got %d: %s", successEvent.Code, successEvent.Body.String())
	}
	var successResult marketing.ProcessEventResult
	if err := json.Unmarshal(successEvent.Body.Bytes(), &successResult); err != nil {
		t.Fatalf("decode success event result: %v", err)
	}
	if len(successResult.OutboxMessages) != 1 {
		t.Fatalf("expected outbox message, got: %s", successEvent.Body.String())
	}

	byOutbox := httptest.NewRecorder()
	handler.Routes().ServeHTTP(byOutbox, httptest.NewRequest(http.MethodGet, "/admin/v1/reward/reconciliation?tenantId=site_001&outboxId="+successResult.OutboxMessages[0].ID, nil))
	if byOutbox.Code != http.StatusOK {
		t.Fatalf("expected outbox reconciliation OK, got %d: %s", byOutbox.Code, byOutbox.Body.String())
	}
	for _, want := range []string{`"outboxMessages":[{`, `"id":"` + successResult.OutboxMessages[0].ID + `"`, `"commandId":"` + successResult.GrantOrders[0].BillNo + `"`, `"participation":{`, `"grantOrder":{`} {
		if !strings.Contains(byOutbox.Body.String(), want) {
			t.Fatalf("expected outbox reconciliation %s, got: %s", want, byOutbox.Body.String())
		}
	}
}

func TestRewardCallbackFailedReleasesQuotaAndRejectsConflictAndUnknown(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	eventRec := postDepositEvent(t, handler, "evt_callback_failed", "pay_callback_failed")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var eventResult marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &eventResult); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := eventResult.GrantOrders[0].BillNo
	markGrantOrderSending(t, repo, commandID)

	rec := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "FAILED",
		"message": "wallet rejected",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected failed callback OK, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"grantStatus":"FAILED"`, `"participationStatus":"FAILED"`, `"reservationStatus":"RELEASED"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("expected %s, got: %s", want, rec.Body.String())
		}
	}
	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&action=GRANT_FAILED&commandId="+commandID, nil))
	for _, want := range []string{`"action":"GRANT_FAILED"`, `"resource":"GRANT_ORDER"`, `"commandId":"` + commandID + `"`, `"source":{`} {
		if !strings.Contains(auditRec.Body.String(), want) {
			t.Fatalf("expected grant failed audit %s, got: %s", want, auditRec.Body.String())
		}
	}

	conflict := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_late",
		"message": "late success",
		"finishedAt": "2026-05-24T12:06:00Z"
	}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"CALLBACK_STATUS_CONFLICT"`) {
		t.Fatalf("expected conflict response, got %d: %s", conflict.Code, conflict.Body.String())
	}

	unknown := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "cmd_unknown",
		"status": "FAILED",
		"message": "not found",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if unknown.Code != http.StatusNotFound || !strings.Contains(unknown.Body.String(), `"code":"GRANT_ORDER_NOT_FOUND"`) {
		t.Fatalf("expected unknown command response, got %d: %s", unknown.Code, unknown.Body.String())
	}
}

func TestRewardCallbackRejectsPendingGrantOrder(t *testing.T) {
	handler := newTestHandler()
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "100")
	createQuota(t, handler, activityID, marketing.QuotaStock, marketing.QuotaScopeActivity, "FIRST_DEPOSIT_RATIO", "10")

	eventRec := postDepositEvent(t, handler, "evt_callback_pending", "pay_callback_pending")
	if eventRec.Code != http.StatusAccepted {
		t.Fatalf("expected event accepted, got %d: %s", eventRec.Code, eventRec.Body.String())
	}
	var eventResult marketing.ProcessEventResult
	if err := json.Unmarshal(eventRec.Body.Bytes(), &eventResult); err != nil {
		t.Fatalf("decode event result: %v", err)
	}
	commandID := eventResult.GrantOrders[0].BillNo

	rec := postRewardCallback(t, handler, `{
		"tenantId": "site_001",
		"commandId": "`+commandID+`",
		"status": "SUCCESS",
		"externalGrantId": "wallet_pending",
		"message": "too early",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"CALLBACK_STATUS_CONFLICT"`) {
		t.Fatalf("expected pending callback conflict, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRewardCallbackSignedIdempotencyAndLocatorConflict(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newSignedCallbackTestHandler(repo)
	activityID := createActivity(t, handler)
	createQuota(t, handler, activityID, marketing.QuotaBudget, marketing.QuotaScopeActivity, "TOTAL", "500")

	firstRec := postDepositEvent(t, handler, "evt_callback_signed_a", "pay_callback_signed_a")
	secondRec := postDepositEvent(t, handler, "evt_callback_signed_b", "pay_callback_signed_b")
	if firstRec.Code != http.StatusAccepted || secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected events accepted, got first=%d %s second=%d %s", firstRec.Code, firstRec.Body.String(), secondRec.Code, secondRec.Body.String())
	}
	var first, second marketing.ProcessEventResult
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first event result: %v", err)
	}
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second event result: %v", err)
	}
	firstOrder := first.GrantOrders[0]
	secondOrder := second.GrantOrders[0]
	markGrantOrderSending(t, repo, firstOrder.BillNo)
	markGrantOrderSending(t, repo, secondOrder.BillNo)

	body := `{
		"tenantId": "site_001",
		"idempotency_key": "` + firstOrder.IdempotencyKey + `",
		"status": "SUCCESS",
		"externalGrantId": "wallet_signed_001",
		"message": "paid",
		"finishedAt": "2026-05-24T12:05:00Z"
	}`
	missing := postRewardCallback(t, handler, body)
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), `"errorCode":"CALLBACK_SIGNATURE_MISSING"`) {
		t.Fatalf("expected missing signature response, got %d: %s", missing.Code, missing.Body.String())
	}

	invalid := postSignedRewardCallback(t, handler, body, func(req *http.Request) {
		req.Header.Set("X-ExMarketing-Signature", "bad")
	})
	if invalid.Code != http.StatusUnauthorized || !strings.Contains(invalid.Body.String(), `"errorCode":"CALLBACK_SIGNATURE_INVALID"`) {
		t.Fatalf("expected invalid signature response, got %d: %s", invalid.Code, invalid.Body.String())
	}

	expired := postSignedRewardCallback(t, handler, body, func(req *http.Request) {
		timestamp := strconv.FormatInt(time.Now().UTC().Add(-10*time.Minute).Unix(), 10)
		req.Header.Set("X-ExMarketing-Timestamp", timestamp)
		req.Header.Set("X-ExMarketing-Signature", webhookTestSignature("callback-secret", timestamp, body))
	})
	if expired.Code != http.StatusUnauthorized || !strings.Contains(expired.Body.String(), `"errorCode":"CALLBACK_TIMESTAMP_INVALID"`) {
		t.Fatalf("expected expired signature response, got %d: %s", expired.Code, expired.Body.String())
	}

	ok := postSignedRewardCallback(t, handler, body, nil)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `"grantStatus":"SUCCESS"`) || !strings.Contains(ok.Body.String(), `"idempotencyKey":"`+firstOrder.IdempotencyKey+`"`) {
		t.Fatalf("expected signed idempotency callback OK, got %d: %s", ok.Code, ok.Body.String())
	}

	conflictBody := `{
		"tenantId": "site_001",
		"billNo": "` + secondOrder.BillNo + `",
		"idempotencyKey": "` + firstOrder.IdempotencyKey + `",
		"status": "FAILED",
		"message": "wrong locator",
		"finishedAt": "2026-05-24T12:06:00Z"
	}`
	conflict := postSignedRewardCallback(t, handler, conflictBody, nil)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"errorCode":"CALLBACK_LOCATOR_CONFLICT"`) {
		t.Fatalf("expected locator conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}
}

func TestQuotaTimeoutReleaseEndpointDryRunReleaseAndAudit(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	old := time.Now().UTC().Add(-3 * time.Hour)
	activityID := "act_timeout_release_http"
	if _, err := repo.CreateQuota(context.Background(), marketing.Quota{
		ID:          "quota_timeout_release_http",
		ActivityID:  activityID,
		Type:        marketing.QuotaBudget,
		Scope:       marketing.QuotaScopeActivity,
		ScopeKey:    "TOTAL",
		TotalValue:  "100",
		UsedValue:   "0",
		FrozenValue: "0",
		Version:     1,
		CreatedAt:   old,
		UpdatedAt:   old,
	}); err != nil {
		t.Fatalf("create quota: %v", err)
	}
	_, err := repo.CreateParticipationGrantOutboxesWithQuota(context.Background(), marketing.AtomicGrantCommand{
		Participation: marketing.Participation{
			ID:             "part_timeout_release_http",
			TenantID:       "site_001",
			ActivityID:     activityID,
			EventInboxID:   "inbox_timeout_release_http",
			Subject:        marketing.EventSubject{Type: "USER", ID: "10001"},
			Biz:            marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_timeout_release_http"},
			Status:         marketing.ParticipationRewardable,
			Snapshot:       []byte(`{}`),
			IdempotencyKey: "part_idem_timeout_release_http",
			CreatedAt:      old,
			UpdatedAt:      old,
		},
		BudgetAmount: "10",
		GrantOrders: []marketing.GrantOrder{{
			ID:              "grant_timeout_release_http",
			TenantID:        "site_001",
			ActivityID:      activityID,
			ParticipationID: "part_timeout_release_http",
			Subject:         marketing.EventSubject{Type: "USER", ID: "10001"},
			RewardType:      marketing.RewardBonus,
			RewardAmount:    "10",
			RewardPayload:   []byte(`{}`),
			BillNo:          "cmd_timeout_release_http",
			GrantStatus:     marketing.GrantPending,
			GrantChannel:    marketing.GrantHTTP,
			IdempotencyKey:  "grant_idem_timeout_release_http",
			CreatedAt:       old,
			UpdatedAt:       old,
		}},
		OutboxMessages: []marketing.OutboxMessage{{
			ID:            "outbox_timeout_release_http",
			TenantID:      "site_001",
			MessageType:   "REWARD_COMMAND",
			TargetChannel: marketing.GrantHTTP,
			TargetRef:     "https://wallet.example.com/reward",
			Payload:       []byte(`{}`),
			SendStatus:    marketing.OutboxPending,
			CreatedAt:     old,
			UpdatedAt:     old,
		}},
	})
	if err != nil {
		t.Fatalf("atomic grant: %v", err)
	}

	body := `{"tenantId":"site_001","olderThan":"` + time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339) + `","limit":10,"dryRun":true,"operator":"ops","reason":"provider timeout"}`
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/v1/quota-reservations/timeout-release", strings.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"result":"DRY_RUN"`) || !strings.Contains(rec.Body.String(), `"reservationStatus":"FROZEN"`) {
		t.Fatalf("expected dry-run candidate, got %d: %s", rec.Code, rec.Body.String())
	}

	body = strings.Replace(body, `"dryRun":true,`, "", 1)
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/v1/quota-reservations/timeout-release", strings.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"result":"RELEASED"`) || !strings.Contains(rec.Body.String(), `"grantStatus":"FAILED"`) || !strings.Contains(rec.Body.String(), `"reservationStatus":"RELEASED"`) {
		t.Fatalf("expected released candidate, got %d: %s", rec.Code, rec.Body.String())
	}

	audit := httptest.NewRecorder()
	handler.Routes().ServeHTTP(audit, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&action=QUOTA_TIMEOUT_RELEASE", nil))
	if audit.Code != http.StatusOK || !strings.Contains(audit.Body.String(), `"reason":"provider timeout"`) {
		t.Fatalf("expected timeout release audit, got %d: %s", audit.Code, audit.Body.String())
	}
}

func TestOutboxDLQQueryReplayAndClose(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := NewHandler(engine.NewService(repo))
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	_, err := repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_1",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_1"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		ErrorMessage:  "target returned status 500",
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}
	if _, err := repo.CreateGrantOrder(context.Background(), marketing.GrantOrder{
		ID:          "grant_dlq_1",
		TenantID:    "site_001",
		BillNo:      "cmd_1",
		GrantStatus: marketing.GrantRetry,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create replay grant order: %v", err)
	}
	_, err = repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_other_tenant",
		TenantID:      "site_002",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_2"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create other tenant outbox: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/outbox/dlq?tenantId=site_001", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected DLQ list status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"outbox_dlq_1"`) || strings.Contains(rec.Body.String(), "outbox_dlq_other_tenant") {
		t.Fatalf("expected tenant-scoped DLQ list, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_1/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "retry after wallet recovery"
	}`))
	handler.Routes().ServeHTTP(rec, replayReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected replay status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"sendStatus":"PENDING"`) || !strings.Contains(rec.Body.String(), `"commandId":"cmd_1"`) {
		t.Fatalf("expected replay to keep payload and return PENDING, got: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	replayReq = httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_1/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "retry after wallet recovery"
	}`))
	handler.Routes().ServeHTTP(rec, replayReq)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected second replay conflict, got %d: %s", rec.Code, rec.Body.String())
	}

	_, err = repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_close",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_close"}`),
		SendStatus:    marketing.OutboxDLQ,
		RetryCount:    5,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create close outbox: %v", err)
	}
	if _, err := repo.CreateGrantOrder(context.Background(), marketing.GrantOrder{
		ID:          "grant_dlq_close",
		TenantID:    "site_001",
		BillNo:      "cmd_close",
		GrantStatus: marketing.GrantRetry,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create close grant order: %v", err)
	}
	rec = httptest.NewRecorder()
	closeReq := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_close/close", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "business rejected"
	}`))
	handler.Routes().ServeHTTP(rec, closeReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sendStatus":"DLQ_CLOSED"`) {
		t.Fatalf("expected close to DLQ_CLOSED, got %d: %s", rec.Code, rec.Body.String())
	}

	auditRec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001", nil))
	for _, want := range []string{`"action":"OUTBOX_DLQ_REPLAY"`, `"action":"OUTBOX_DLQ_CLOSE"`, `"actor":"ops"`, `"targetChannel":"HTTP"`, `"retryCount":5`, `"commandId":"cmd_1"`} {
		if !strings.Contains(auditRec.Body.String(), want) {
			t.Fatalf("expected audit %s, got: %s", want, auditRec.Body.String())
		}
	}
}

func TestOutboxDLQOperationRequiresOperatorAndReason(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := NewHandler(engine.NewService(repo))
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	_, err := repo.CreateOutbox(context.Background(), marketing.OutboxMessage{
		ID:            "outbox_dlq_missing_reason",
		TenantID:      "site_001",
		MessageType:   "REWARD_COMMAND",
		TargetChannel: marketing.GrantHTTP,
		TargetRef:     "https://wallet.example.com/reward",
		Payload:       []byte(`{"commandId":"cmd_1"}`),
		SendStatus:    marketing.OutboxDLQ,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/outbox/outbox_dlq_missing_reason/replay", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": ""
	}`))
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected missing reason conflict, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestManualGrantDecisionAndExportWriteStableAuditActions(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newTestHandlerWithRepository(repo)
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	if _, err := repo.CreateParticipation(context.Background(), marketing.Participation{
		ID:             "part_manual_audit",
		TenantID:       "site_001",
		ActivityID:     "act_manual_audit",
		Subject:        marketing.EventSubject{Type: "USER", ID: "10001"},
		Status:         marketing.ParticipationRewardable,
		IdempotencyKey: "part_manual_audit_idem",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("create participation: %v", err)
	}
	if _, err := repo.CreateGrantOrder(context.Background(), marketing.GrantOrder{
		ID:              "grant_manual_audit",
		TenantID:        "site_001",
		ActivityID:      "act_manual_audit",
		ParticipationID: "part_manual_audit",
		Subject:         marketing.EventSubject{Type: "USER", ID: "10001"},
		RewardType:      marketing.RewardBonus,
		RewardAmount:    "10",
		RewardPayload:   []byte(`{"grantMode":"MANUAL"}`),
		BillNo:          "cmd_manual_audit",
		GrantStatus:     marketing.GrantPending,
		GrantChannel:    marketing.GrantManualFile,
		IdempotencyKey:  "grant_manual_audit_idem",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("create manual grant: %v", err)
	}
	decision := httptest.NewRecorder()
	handler.Routes().ServeHTTP(decision, httptest.NewRequest(http.MethodPost, "/admin/v1/manual-grants/decision", strings.NewReader(`{
		"tenantId": "site_001",
		"commandId": "cmd_manual_audit",
		"status": "SUCCESS",
		"externalGrantId": "offline_1",
		"operator": "ops",
		"reason": "offline paid",
		"finishedAt": "2026-05-29T10:01:00Z"
	}`)))
	if decision.Code != http.StatusOK {
		t.Fatalf("expected manual decision OK, got %d: %s", decision.Code, decision.Body.String())
	}

	if _, err := repo.CreateParticipation(context.Background(), marketing.Participation{
		ID:             "part_export_audit",
		TenantID:       "site_001",
		ActivityID:     "act_export_audit",
		Subject:        marketing.EventSubject{Type: "USER", ID: "10002"},
		Status:         marketing.ParticipationRewardable,
		IdempotencyKey: "part_export_audit_idem",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("create export participation: %v", err)
	}
	if _, err := repo.CreateGrantOrder(context.Background(), marketing.GrantOrder{
		ID:              "grant_export_audit",
		TenantID:        "site_001",
		ActivityID:      "act_export_audit",
		ParticipationID: "part_export_audit",
		Subject:         marketing.EventSubject{Type: "USER", ID: "10002"},
		RewardType:      marketing.RewardBonus,
		RewardAmount:    "5",
		RewardPayload:   []byte(`{"grantMode":"MANUAL_EXPORT","rewardCode":"REWARD_EXPORT"}`),
		BillNo:          "cmd_export_audit",
		GrantStatus:     marketing.GrantPending,
		GrantChannel:    marketing.GrantManualFile,
		IdempotencyKey:  "grant_export_audit_idem",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("create export grant: %v", err)
	}
	export := httptest.NewRecorder()
	handler.Routes().ServeHTTP(export, httptest.NewRequest(http.MethodPost, "/admin/v1/manual-grant-exports", strings.NewReader(`{
		"tenantId": "site_001",
		"operator": "ops",
		"reason": "daily export",
		"activityId": "act_export_audit"
	}`)))
	if export.Code != http.StatusCreated {
		t.Fatalf("expected manual export created, got %d: %s", export.Code, export.Body.String())
	}

	audit := httptest.NewRecorder()
	handler.Routes().ServeHTTP(audit, httptest.NewRequest(http.MethodGet, "/admin/v1/audit?tenantId=site_001&actor=ops", nil))
	for _, want := range []string{`"action":"MANUAL_GRANT_DECISION"`, `"action":"MANUAL_GRANT_EXPORT_CREATE"`, `"decisionStatus":"SUCCESS"`, `"checksum":`} {
		if !strings.Contains(audit.Body.String(), want) {
			t.Fatalf("expected manual audit %s, got: %s", want, audit.Body.String())
		}
	}
	if strings.Contains(audit.Body.String(), "MANUAL_GRANT_CONFIRM_SUCCESS") || strings.Contains(audit.Body.String(), "MANUAL_GRANT_EXPORT_READY") {
		t.Fatalf("expected stable manual audit actions only, got: %s", audit.Body.String())
	}
}

func TestListWorkflowRequirements(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify workflow endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/requirements", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"2026-05-24-test"`) {
		t.Fatalf("expected requirement id, got: %s", body)
	}
	if !strings.Contains(body, `"analysis":{"status":"WAITING"`) {
		t.Fatalf("expected analysis agent status, got: %s", body)
	}
}

func TestGetWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify workflow detail endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/requirements/2026-05-24-test", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"task.md"`) {
		t.Fatalf("expected task artifact, got: %s", body)
	}
	if !strings.Contains(body, `"agent-status.json"`) {
		t.Fatalf("expected status artifact, got: %s", body)
	}
}

func TestCreateAndIntakeWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := newTestHandler()

	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements", strings.NewReader(`{
		"title": "agent control",
		"description": "verify admin creates workflow requirements",
		"risk": "低"
	}`))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	reqHTTP = httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/intake", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ANALYSIS_READY"`) {
		t.Fatalf("expected intake status, got: %s", rec.Body.String())
	}
}

func TestListWorkflowTemplates(t *testing.T) {
	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodGet, "/admin/v1/workflow/templates", nil)
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"id":"marketing-platform-22"`) ||
		!strings.Contains(body, `"order":35`) ||
		!strings.Contains(body, "标准事件模型 Schema 校验") ||
		!strings.Contains(body, "管理后台通用配置页面") {
		t.Fatalf("expected marketing platform template, got: %s", body)
	}
}

func TestApplyWorkflowTemplateCreatesIntakeTasks(t *testing.T) {
	t.Chdir(t.TempDir())
	handler := newTestHandler()
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/templates/marketing-platform-22/apply", strings.NewReader(`{"intake":true}`))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var body struct {
		Created []workflow.Requirement `json:"created"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Created) != len(workflow.MarketingPlatformTemplate().Items) {
		t.Fatalf("expected %d created requirements, got %d", len(workflow.MarketingPlatformTemplate().Items), len(body.Created))
	}
	if body.Created[0].Status != workflow.StatusAnalysisReady || body.Created[0].TaskDir == "" {
		t.Fatalf("expected intake-created task, got: %+v", body.Created[0])
	}
	if _, err := os.Stat(body.Created[0].TaskDir + "/agent-prompts.md"); err != nil {
		t.Fatalf("expected task files: %v", err)
	}

	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/templates/marketing-platform-22/apply", strings.NewReader(`{"intake":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected idempotent status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var second struct {
		Created []workflow.Requirement         `json:"created"`
		Skipped []workflow.RequirementTemplate `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if len(second.Created) != 0 || len(second.Skipped) != len(workflow.MarketingPlatformTemplate().Items) {
		t.Fatalf("expected duplicate titles skipped, got created=%d skipped=%d", len(second.Created), len(second.Skipped))
	}
}

func TestFilterAgentLogNoise(t *testing.T) {
	content := strings.Join([]string{
		"before",
		"2026-05-24T12:04:05.228825Z  WARN codex_core_skills::loader: ignoring interface.icon_small: icon path must not contain '..'",
		"2026-05-24T12:04:05.228829Z  WARN codex_core_skills::loader: ignoring interface.icon_large: icon path must not contain '..'",
		"after",
	}, "\n")
	got := filterAgentLogNoise(content)
	if strings.Contains(got, "codex_core_skills::loader") || strings.Contains(got, "interface.icon_large") || strings.Contains(got, "interface.icon_small") {
		t.Fatalf("expected noisy Codex skill icon warnings filtered, got: %s", got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("expected useful log lines preserved, got: %s", got)
	}
}

func TestDeleteWorkflowRequirement(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	backlog := workflow.Backlog{Items: []workflow.Requirement{
		{ID: "req-a", Title: "A", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-b", Title: "B", Description: "keep me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
	}}
	if err := workflow.Save("requirements/backlog.json", backlog); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	reqHTTP := httptest.NewRequest(http.MethodDelete, "/admin/v1/workflow/requirements/req-a", nil)
	rec := httptest.NewRecorder()
	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	got, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "req-b" {
		t.Fatalf("expected only req-b, got: %+v", got.Items)
	}
}

func TestDeleteWorkflowRequirementsBatch(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	backlog := workflow.Backlog{Items: []workflow.Requirement{
		{ID: "req-a", Title: "A", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-b", Title: "B", Description: "delete me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
		{ID: "req-c", Title: "C", Description: "keep me", Status: workflow.StatusNew, CreatedAt: now, UpdatedAt: now},
	}}
	if err := workflow.Save("requirements/backlog.json", backlog); err != nil {
		t.Fatalf("save backlog: %v", err)
	}

	reqHTTP := httptest.NewRequest(http.MethodDelete, "/admin/v1/workflow/requirements", strings.NewReader(`{"ids":["req-a","req-b"]}`))
	rec := httptest.NewRecorder()
	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	got, err := workflow.Load("requirements/backlog.json")
	if err != nil {
		t.Fatalf("load backlog: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "req-c" {
		t.Fatalf("expected only req-c, got: %+v", got.Items)
	}
}

func TestRunWorkflowAgent(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify run agent endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	command := `cat >/dev/null && cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
88
~~~

## Decision

~~~text
PASS
~~~
EOF`
	body := `{"agent":"analysis","command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-agent", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"NextStatus":"REVIEW_READY"`) {
		t.Fatalf("expected review ready, got: %s", rec.Body.String())
	}
}

func TestRunWorkflowPipelineReturnsAnalysisBlockToAnalysis(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify run pipeline endpoint",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	command := `cat >/dev/null && cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
60
~~~

## Decision

~~~text
BLOCK
~~~
EOF`
	body := `{"maxSteps":1,"command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-pipeline", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ANALYSIS_FAIL"`) {
		t.Fatalf("expected analysis fail, got: %s", rec.Body.String())
	}
	report, err := os.ReadFile("tasks/2026-05-24-test/agent-report.md")
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(report), "最终状态: ANALYSIS_FAIL") {
		t.Fatalf("expected report final status, got: %s", string(report))
	}
}

func TestRunWorkflowPipelineRetriesAnalysisAfterBlock(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now().UTC()
	req := workflow.Requirement{
		ID:          "2026-05-24-test",
		Title:       "test requirement",
		Description: "verify analysis retry",
		Status:      workflow.StatusAnalysisReady,
		Risk:        "中",
		TaskDir:     workflow.TaskDirFor("2026-05-24-test"),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := workflow.CreateTaskFiles(req); err != nil {
		t.Fatalf("create task files: %v", err)
	}
	if err := workflow.Save("requirements/backlog.json", workflow.Backlog{Items: []workflow.Requirement{req}}); err != nil {
		t.Fatalf("save backlog: %v", err)
	}
	command := `cat >/dev/null
count_file="$EX_MARKETING_TASK_DIR/.analysis-count"
count=0
if [ -f "$count_file" ]; then count=$(cat "$count_file"); fi
count=$((count + 1))
printf "%s" "$count" > "$count_file"
if [ "$count" -eq 1 ]; then
cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Score

~~~text
60
~~~

## Decision

~~~text
BLOCK
~~~

缺少验收标准。
EOF
else
cat > "$EX_MARKETING_TASK_DIR/analysis.md" <<'EOF'
# Analysis

## Review 反馈处理

已根据上一轮拒绝原因补充验收标准。

## Score

~~~text
88
~~~

## Decision

~~~text
PASS
~~~
EOF
fi`
	body := `{"maxSteps":2,"command":` + quoteJSON(command) + `}`
	reqHTTP := httptest.NewRequest(http.MethodPost, "/admin/v1/workflow/requirements/2026-05-24-test/run-pipeline", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler := newTestHandler()
	handler.Routes().ServeHTTP(rec, reqHTTP)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"REVIEW_READY"`) {
		t.Fatalf("expected retry to reach review ready, got: %s", rec.Body.String())
	}
	count, err := os.ReadFile("tasks/2026-05-24-test/.analysis-count")
	if err != nil {
		t.Fatalf("read retry count: %v", err)
	}
	if string(count) != "2" {
		t.Fatalf("expected analysis to run twice, got %q", string(count))
	}
}

func quoteJSON(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + replacer.Replace(s) + `"`
}

func createActivity(t *testing.T, handler *Handler) string {
	t.Helper()
	body := validActivityBody()
	rec := postActivity(t, handler, body)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var activity marketing.Activity
	if err := json.NewDecoder(rec.Body).Decode(&activity); err != nil {
		t.Fatalf("decode activity: %v", err)
	}
	approveAndActivateActivity(t, handler, activity.ID, activity.Version)
	return activity.ID
}

func approveAndActivateActivity(t *testing.T, handler *Handler, activityID string, version int) {
	t.Helper()
	rec := postApprovalAction(t, handler, "/admin/v1/activities/"+activityID+"/approval/submit", `{"operator":"ops","expectedVersion":`+strconv.Itoa(version)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected submit approval status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	rec = postApprovalAction(t, handler, "/admin/v1/activities/"+activityID+"/approval/approve", `{"operator":"lead"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected l1 approval status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	rec = postApprovalAction(t, handler, "/admin/v1/activities/"+activityID+"/approval/approve", `{"operator":"director"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected l2 approval status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	updateRec := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/admin/v1/activities/"+activityID, strings.NewReader(`{"operator":"ops","status":"ACTIVE"}`))
	handler.Routes().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected activate status %d, got %d: %s", http.StatusOK, updateRec.Code, updateRec.Body.String())
	}
}

func approveAndActivateCreatedActivity(t *testing.T, handler *Handler, rec *httptest.ResponseRecorder) {
	t.Helper()
	var activity marketing.Activity
	if err := json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&activity); err != nil {
		t.Fatalf("decode created activity: %v", err)
	}
	approveAndActivateActivity(t, handler, activity.ID, activity.Version)
}

func postApprovalAction(t *testing.T, handler *Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func TestValidatePrelaunchEndpointReturnsItemizedHighRiskApprovalRequired(t *testing.T) {
	handler := newTestHandler()
	body := `{
		"actor": "operator",
		"targetAction": "GO_LIVE",
		"activity": ` + validActivityBody() + `
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities/prelaunch-validation", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"conclusion":"APPROVAL_REQUIRED"`) {
		t.Fatalf("expected high-risk approval-required result, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"allowGoLive":false`) {
		t.Fatalf("expected go-live blocked, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"items"`) || !strings.Contains(rec.Body.String(), `"highRiskItems"`) {
		t.Fatalf("expected itemized validation result, got: %s", rec.Body.String())
	}
}

func TestWebhookReceiverMapsSignedPayloadIntoEventChain(t *testing.T) {
	repo := marketing.NewMemoryRepository()
	handler := newWebhookTestHandler(repo, map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)})
	body := validWebhookPayload("provider_evt_001", "pay_001", "10001")

	rec := postWebhook(t, handler, "payment-webhook", body, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d: %s", http.StatusAccepted, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"duplicate":false`) || !strings.Contains(rec.Body.String(), `"idempotencyKey":"site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:pay_001"`) {
		t.Fatalf("expected mapped event response, got: %s", rec.Body.String())
	}

	duplicate := postWebhook(t, handler, "payment-webhook", body, "")
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate response, got %d: %s", duplicate.Code, duplicate.Body.String())
	}

	audits, err := repo.ListAudit(context.Background(), marketing.AuditQuery{Resource: "WEBHOOK_RECEIVER", ResourceID: "payment-webhook"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) < 2 {
		t.Fatalf("expected webhook audits, got %+v", audits)
	}
	actions := []string{audits[0].Action, audits[1].Action}
	if !slicesContains(actions, "WEBHOOK_RECEIVED") || !slicesContains(actions, "WEBHOOK_DUPLICATE") {
		t.Fatalf("expected received and duplicate audits, got %+v", actions)
	}
	if strings.Contains(string(audits[0].Payload), "receiver-secret") || strings.Contains(string(audits[0].Payload), body) {
		t.Fatalf("audit leaked secret or raw payload: %s", string(audits[0].Payload))
	}
}

func TestWebhookReceiverRejectsSecurityAndMappingFailures(t *testing.T) {
	body := validWebhookPayload("provider_evt_002", "pay_002", "10001")
	for _, tc := range []struct {
		name       string
		receivers  map[string]WebhookReceiver
		body       string
		mutate     func(*http.Request)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing receiver",
			receivers:  map[string]WebhookReceiver{},
			wantStatus: http.StatusBadRequest,
			wantCode:   "WEBHOOK_RECEIVER_NOT_FOUND",
		},
		{
			name:       "disabled receiver",
			receivers:  map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(false)},
			wantStatus: http.StatusForbidden,
			wantCode:   "WEBHOOK_RECEIVER_DISABLED",
		},
		{
			name:      "missing signature",
			receivers: map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)},
			mutate: func(req *http.Request) {
				req.Header.Del("X-ExMarketing-Signature")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "WEBHOOK_SIGNATURE_MISSING",
		},
		{
			name:      "invalid signature",
			receivers: map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)},
			mutate: func(req *http.Request) {
				req.Header.Set("X-ExMarketing-Signature", strings.Repeat("0", 64))
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "WEBHOOK_SIGNATURE_INVALID",
		},
		{
			name:      "expired timestamp",
			receivers: map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)},
			mutate: func(req *http.Request) {
				timestamp := strconv.FormatInt(time.Now().UTC().Add(-10*time.Minute).Unix(), 10)
				req.Header.Set("X-ExMarketing-Timestamp", timestamp)
				req.Header.Set("X-ExMarketing-Signature", webhookTestSignature("receiver-secret", timestamp, body))
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "WEBHOOK_TIMESTAMP_INVALID",
		},
		{
			name:       "mapping failed",
			receivers:  map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)},
			body:       `{"id":"provider_evt_002","user":{"id":"10001"},"createdAt":"2026-05-24T12:00:00Z"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "WEBHOOK_MAPPING_FAILED",
		},
		{
			name: "source forbidden",
			receivers: map[string]WebhookReceiver{"payment-webhook": func() WebhookReceiver {
				receiver := validWebhookReceiver(true)
				receiver.AllowedTenantIDs = []string{"site_002"}
				return receiver
			}()},
			wantStatus: http.StatusForbidden,
			wantCode:   "WEBHOOK_SOURCE_FORBIDDEN",
		},
		{
			name:      "idempotency mismatch",
			receivers: map[string]WebhookReceiver{"payment-webhook": validWebhookReceiver(true)},
			mutate: func(req *http.Request) {
				req.Header.Set("Idempotency-Key", "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:other")
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_IDEMPOTENCY_KEY",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := marketing.NewMemoryRepository()
			handler := newWebhookTestHandler(repo, tc.receivers)
			payload := body
			if tc.body != "" {
				payload = tc.body
			}
			rec := postWebhookWithMutation(t, handler, "payment-webhook", payload, tc.mutate)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), `"errorCode":"`+tc.wantCode+`"`) {
				t.Fatalf("expected %d %s, got %d: %s", tc.wantStatus, tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
}

func validActivityBody() string {
	future := time.Now().UTC().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	body := `{
		"tenantId": "site_001",
		"activityCode": "FIRST_DEPOSIT_BONUS",
		"activityName": "First Deposit Bonus",
		"activityType": "FIRST_DEPOSIT",
		"status": "DRAFT",
		"triggerEvents": ["DEPOSIT_SUCCESS"],
		"startTime": "2026-05-01T00:00:00Z",
		"endTime": "__FUTURE_TIME__",
		"timezone": "UTC",
		"priority": 100,
		"exclusiveGroup": "deposit_bonus",
		"stackable": false,
		"version": 1,
		"config": {
			"entry": {
				"channels": ["APP", "WEB"],
				"entranceCodes": ["deposit_page"],
				"landingUrl": "https://example.com/promo/first-deposit"
			},
			"display": {
				"title": "First Deposit Bonus",
				"bannerUrl": "https://example.com/banner.png",
				"termsUrl": "https://example.com/terms",
				"visibleFrom": "2026-05-01T00:00:00Z",
				"visibleTo": "__FUTURE_TIME__",
				"sort": 100
			},
			"audience": {
				"segmentCodes": ["new_user"],
				"tags": ["first_deposit"],
				"countries": ["CN"]
			},
			"metadata": {"owner": "growth"}
		},
		"rules": [
			{
				"ruleCode": "FIRST_DEPOSIT_AND_AMOUNT",
				"rulePhase": "QUALIFY",
				"ruleType": "CONDITION",
				"ruleConfig": {
					"logic": "AND",
					"conditions": [
						{"field": "attributes.isFirstDeposit", "op": "=", "value": true},
						{"field": "attributes.amount", "op": ">=", "value": "100.00"},
						{"field": "attributes.vipLevel", "op": "IN", "value": [3, 4, 5]}
					]
				},
				"sort": 1,
				"enabled": true,
				"version": 1
			}
		],
		"rewards": [
			{
				"rewardCode": "FIRST_DEPOSIT_RATIO",
				"rewardType": "BONUS",
				"calculateType": "RATIO",
				"rewardConfig": {
					"baseField": "attributes.amount",
					"rate": "0.20",
					"maxAmount": "500.00",
					"currency": "CNY",
					"flowMultiple": "3",
					"targetRef": "https://wallet.example.com/reward"
				},
				"grantMode": "AUTO",
				"grantChannel": "HTTP",
				"flowMultiple": "3",
				"enabled": true,
				"version": 1
			}
		]
	}`
	return strings.ReplaceAll(body, "__FUTURE_TIME__", future)
}

func postActivity(t *testing.T, handler *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/activities", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func createQuota(t *testing.T, handler *Handler, activityID string, quotaType marketing.QuotaType, scope marketing.QuotaScope, scopeKey string, totalValue string) {
	t.Helper()
	body := `{
		"activityId": "` + activityID + `",
		"quotaType": "` + string(quotaType) + `",
		"scope": "` + string(scope) + `",
		"scopeKey": "` + scopeKey + `",
		"totalValue": "` + totalValue + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/quotas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
}

func postDepositEvent(t *testing.T, handler *Handler, eventID string, bizID string) *httptest.ResponseRecorder {
	t.Helper()
	event := depositEventJSON(eventID, bizID, `{
			"amount": "299.00",
			"currency": "CNY",
			"vipLevel": 3,
			"isFirstDeposit": true
		}`)
	return postRawEvent(t, handler, event, "site_001:payment:DEPOSIT_SUCCESS:DEPOSIT:"+bizID)
}

func depositEventJSON(eventID string, bizID string, attributesJSON string) string {
	trimmedAttributes := strings.TrimSpace(attributesJSON)
	if attributesJSON != "null" && !strings.HasPrefix(trimmedAttributes, "{") && !strings.HasPrefix(trimmedAttributes, "[") {
		attributesJSON = "{" + attributesJSON + "}"
	}
	trimmedAttributes = strings.TrimSpace(attributesJSON)
	if strings.HasPrefix(trimmedAttributes, "{") && !strings.Contains(trimmedAttributes, `"channel"`) {
		if trimmedAttributes == "{}" {
			attributesJSON = `{"channel":"APP"}`
		} else {
			attributesJSON = strings.Replace(attributesJSON, "{", `{"channel":"APP",`, 1)
		}
	}
	return `{
		"eventId": "` + eventID + `",
		"eventType": "DEPOSIT_SUCCESS",
		"sourceSystem": "payment",
		"tenantId": "site_001",
		"subject": {"type": "USER", "id": "10001"},
		"biz": {"bizType": "DEPOSIT", "bizId": "` + bizID + `"},
		"occurredAt": "2026-05-24T12:00:00Z",
		"attributes": ` + attributesJSON + `
	}`
}

func postRawEvent(t *testing.T, handler *Handler, event string, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := eventRequest(event, idempotencyKey)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func postAuthorizedRawEvent(t *testing.T, handler *Handler, event string, idempotencyKey string, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := eventRequest(event, idempotencyKey)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func eventRequest(event string, idempotencyKey string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/events", strings.NewReader(event))
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

func validWebhookReceiver(enabled bool) WebhookReceiver {
	return WebhookReceiver{
		ReceiverCode:         "payment-webhook",
		Enabled:              enabled,
		Secret:               "receiver-secret",
		AllowedTenantIDs:     []string{"site_001"},
		AllowedSourceSystems: []string{"payment"},
		AllowedIPs:           nil,
		Mapping: WebhookMapping{
			EventID:      WebhookMappingRule{Path: "$.id"},
			EventType:    WebhookMappingRule{Const: "DEPOSIT_SUCCESS", HasConst: true},
			SourceSystem: WebhookMappingRule{Const: "payment", HasConst: true},
			TenantID:     WebhookMappingRule{Const: "site_001", HasConst: true},
			SubjectType:  WebhookMappingRule{Const: "USER", HasConst: true},
			SubjectID:    WebhookMappingRule{Path: "$.user.id"},
			BizType:      WebhookMappingRule{Const: "DEPOSIT", HasConst: true},
			BizID:        WebhookMappingRule{Path: "$.payment.id"},
			OccurredAt:   WebhookMappingRule{Path: "$.createdAt"},
			Attributes: map[string]WebhookMappingRule{
				"amount":          {Path: "$.payment.amount"},
				"currency":        {Path: "$.payment.currency"},
				"providerEventId": {Path: "$.id"},
			},
		},
	}
}

func validWebhookPayload(eventID string, paymentID string, userID string) string {
	return `{"id":"` + eventID + `","user":{"id":"` + userID + `"},"payment":{"id":"` + paymentID + `","amount":100,"currency":"USD"},"createdAt":"2026-05-24T12:00:00Z"}`
}

func postWebhook(t *testing.T, handler *Handler, receiverCode string, body string, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	return postWebhookWithMutation(t, handler, receiverCode, body, func(req *http.Request) {
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
	})
}

func postWebhookWithMutation(t *testing.T, handler *Handler, receiverCode string, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/webhooks/"+receiverCode, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ExMarketing-Timestamp", timestamp)
	req.Header.Set("X-ExMarketing-Signature", webhookTestSignature("receiver-secret", timestamp, body))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func webhookTestSignature(secret string, timestamp string, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func postRewardCallback(t *testing.T, handler *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/reward/callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func postSignedRewardCallback(t *testing.T, handler *Handler, body string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/openapi/v1/reward/callback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ExMarketing-Timestamp", timestamp)
	req.Header.Set("X-ExMarketing-Signature", webhookTestSignature("callback-secret", timestamp, body))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	return rec
}

func markGrantOrderSending(t *testing.T, repo *marketing.MemoryRepository, commandID string) {
	t.Helper()
	if err := repo.UpdateGrantOrderByCommandID(context.Background(), "site_001", commandID, marketing.GrantSending, ""); err != nil {
		t.Fatalf("mark grant order sending: %v", err)
	}
}
