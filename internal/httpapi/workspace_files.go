package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

const maxWorkspaceFileBytes int64 = 64 << 20

type workspaceObjectStore interface {
	Provider() string
	Put(r *http.Request, key string, reader io.Reader) (int64, string, error)
	Delete(r *http.Request, key string) error
}

func workspaceFileStorageDir() string {
	if value := strings.TrimSpace(os.Getenv("EX_MARKETING_FILE_STORAGE_DIR")); value != "" {
		return value
	}
	return filepath.Join("data", "workspace-files")
}

func workspaceFileStorageProvider() string {
	if value := strings.TrimSpace(os.Getenv("EX_MARKETING_FILE_STORAGE_PROVIDER")); value != "" {
		return strings.ToLower(value)
	}
	return "local"
}

func workspaceFileObjectStore() workspaceObjectStore {
	switch workspaceFileStorageProvider() {
	case "s3", "minio":
		return s3CompatibleWorkspaceObjectStore{
			provider: workspaceFileStorageProvider(),
			endpoint: strings.TrimRight(strings.TrimSpace(os.Getenv("EX_MARKETING_S3_ENDPOINT")), "/"),
			bucket:   strings.Trim(strings.TrimSpace(os.Getenv("EX_MARKETING_S3_BUCKET")), "/"),
			prefix:   strings.Trim(strings.TrimSpace(os.Getenv("EX_MARKETING_S3_PREFIX")), "/"),
		}
	default:
		return localWorkspaceObjectStore{root: workspaceFileStorageDir()}
	}
}

type localWorkspaceObjectStore struct {
	root string
}

func (s localWorkspaceObjectStore) Provider() string {
	return "local"
}

func (s localWorkspaceObjectStore) Put(_ *http.Request, key string, reader io.Reader) (int64, string, error) {
	absolutePath := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return 0, "", err
	}
	out, err := os.Create(absolutePath)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(reader, maxWorkspaceFileBytes+1))
	if err != nil {
		return 0, "", err
	}
	if written > maxWorkspaceFileBytes {
		_ = os.Remove(absolutePath)
		return 0, "", marketing.ErrInvalidWorkspace
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s localWorkspaceObjectStore) Delete(_ *http.Request, key string) error {
	absolutePath := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.Remove(absolutePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type s3CompatibleWorkspaceObjectStore struct {
	provider string
	endpoint string
	bucket   string
	prefix   string
}

func (s s3CompatibleWorkspaceObjectStore) Provider() string {
	return s.provider
}

func (s s3CompatibleWorkspaceObjectStore) objectURL(key string) (string, error) {
	if s.endpoint == "" || s.bucket == "" {
		return "", fmt.Errorf("S3-compatible storage requires EX_MARKETING_S3_ENDPOINT and EX_MARKETING_S3_BUCKET")
	}
	escapedParts := make([]string, 0, 4)
	escapedParts = append(escapedParts, url.PathEscape(s.bucket))
	if s.prefix != "" {
		for _, part := range strings.Split(s.prefix, "/") {
			if part != "" {
				escapedParts = append(escapedParts, url.PathEscape(part))
			}
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(key), "/") {
		if part != "" {
			escapedParts = append(escapedParts, url.PathEscape(part))
		}
	}
	return s.endpoint + "/" + strings.Join(escapedParts, "/"), nil
}

func (s s3CompatibleWorkspaceObjectStore) Put(r *http.Request, key string, reader io.Reader) (int64, string, error) {
	hasher := sha256.New()
	limited := io.LimitReader(reader, maxWorkspaceFileBytes+1)
	body, err := io.ReadAll(io.TeeReader(limited, hasher))
	if err != nil {
		return 0, "", err
	}
	if int64(len(body)) > maxWorkspaceFileBytes {
		return 0, "", marketing.ErrInvalidWorkspace
	}
	target, err := s.objectURL(key)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", fmt.Errorf("S3-compatible PUT failed: %s", resp.Status)
	}
	return int64(len(body)), hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s s3CompatibleWorkspaceObjectStore) Delete(r *http.Request, key string) error {
	target, err := s.objectURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("S3-compatible DELETE failed: %s", resp.Status)
	}
	return nil
}

func (h *Handler) listWorkspaceFileObjects(w http.ResponseWriter, r *http.Request) {
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	if !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionView) {
		return
	}
	query := marketing.WorkspaceFileObjectQuery{
		WorkspaceID:       workspaceID,
		FolderID:          strings.TrimSpace(r.URL.Query().Get("folderId")),
		ActivityID:        strings.TrimSpace(r.URL.Query().Get("activityId")),
		WorkflowVersionID: strings.TrimSpace(r.URL.Query().Get("workflowVersionId")),
		ExecutionID:       strings.TrimSpace(r.URL.Query().Get("executionId")),
		Kind:              marketing.WorkspaceFileObjectKind(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("kind")))),
		Limit:             intQuery(r, "limit"),
	}
	items, err := h.engine.ListWorkspaceFileObjects(r.Context(), query)
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) uploadWorkspaceFileObject(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxWorkspaceFileBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart body")
		return
	}
	workspaceID := strings.TrimSpace(r.FormValue("workspaceId"))
	if workspaceID == "" || !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionEdit) {
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	created, err := h.storeWorkspaceFileFromReader(r, workspaceID, header.Filename, header.Header.Get("Content-Type"), file, workspaceFileMetaFromForm(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) deleteWorkspaceFileObject(w http.ResponseWriter, r *http.Request) {
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if workspaceID == "" || !h.requireWorkspacePermission(w, r, workspaceID, workspacePermissionAdmin) {
		return
	}
	deleted, err := h.engine.DeleteWorkspaceFileObject(r.Context(), workspaceID, r.PathValue("fileId"), actorFromRequest(r))
	if err != nil {
		workspaceError(w, err)
		return
	}
	if err := workspaceFileObjectStore().Delete(r, deleted.StorageKey); err != nil {
		workspaceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

type workspaceFileMeta struct {
	FolderID          string
	ActivityID        string
	WorkflowVersionID string
	ExecutionID       string
	Kind              marketing.WorkspaceFileObjectKind
	CreatedBy         string
}

func workspaceFileMetaFromForm(r *http.Request) workspaceFileMeta {
	kind := marketing.WorkspaceFileObjectKind(strings.ToUpper(strings.TrimSpace(r.FormValue("kind"))))
	if kind == "" {
		kind = marketing.WorkspaceFileImportWorkflow
	}
	return workspaceFileMeta{
		FolderID:          strings.TrimSpace(r.FormValue("folderId")),
		ActivityID:        strings.TrimSpace(r.FormValue("activityId")),
		WorkflowVersionID: strings.TrimSpace(r.FormValue("workflowVersionId")),
		ExecutionID:       strings.TrimSpace(r.FormValue("executionId")),
		Kind:              kind,
		CreatedBy:         actorFromRequest(r),
	}
}

func (h *Handler) storeWorkspaceJSONFile(r *http.Request, workspaceID string, fileName string, payload any, meta workspaceFileMeta) (marketing.WorkspaceFileObject, error) {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	return h.storeWorkspaceFileFromReader(r, workspaceID, fileName, "application/json", bytes.NewReader(raw), meta)
}

func (h *Handler) storeWorkspaceFileFromMultipart(r *http.Request, workspaceID string, header *multipart.FileHeader, meta workspaceFileMeta) (marketing.WorkspaceFileObject, error) {
	file, err := header.Open()
	if err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	defer file.Close()
	return h.storeWorkspaceFileFromReader(r, workspaceID, header.Filename, header.Header.Get("Content-Type"), file, meta)
}

func (h *Handler) storeWorkspaceFileFromReader(r *http.Request, workspaceID string, fileName string, contentType string, reader io.Reader, meta workspaceFileMeta) (marketing.WorkspaceFileObject, error) {
	if meta.Kind == "" {
		meta.Kind = marketing.WorkspaceFileImportWorkflow
	}
	if meta.CreatedBy == "" {
		meta.CreatedBy = actorFromRequest(r)
	}
	fileID := marketing.NewID("wsfile")
	safeName := strings.TrimSpace(filepath.Base(fileName))
	if safeName == "" || safeName == "." {
		safeName = fileID + ".bin"
	}
	now := time.Now().UTC()
	relativeKey := filepath.Join(workspaceID, now.Format("20060102"), fileID+"-"+safeName)
	store := workspaceFileObjectStore()
	written, checksum, err := store.Put(r, filepath.ToSlash(relativeKey), reader)
	if err != nil {
		return marketing.WorkspaceFileObject{}, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return h.engine.CreateWorkspaceFileObject(r.Context(), marketing.WorkspaceFileObject{
		ID:                fileID,
		WorkspaceID:       workspaceID,
		FolderID:          meta.FolderID,
		ActivityID:        meta.ActivityID,
		WorkflowVersionID: meta.WorkflowVersionID,
		ExecutionID:       meta.ExecutionID,
		Kind:              meta.Kind,
		FileName:          safeName,
		ContentType:       contentType,
		SizeBytes:         written,
		StorageProvider:   store.Provider(),
		StorageKey:        filepath.ToSlash(relativeKey),
		ChecksumSHA256:    checksum,
		CreatedBy:         meta.CreatedBy,
		CreatedAt:         now,
	})
}
