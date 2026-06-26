package redislua

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type ScriptID string

const (
	ScriptQuotaReserve ScriptID = "quota_reserve_v1"
	ScriptQuotaConfirm ScriptID = "quota_confirm_v1"
	ScriptQuotaRelease ScriptID = "quota_release_v1"
	ScriptRateLimit    ScriptID = "rate_limit_v1"
)

type Client interface {
	ScriptLoad(ctx context.Context, body string) (string, error)
	EvalSHA(ctx context.Context, sha string, keys []string, args ...string) (string, error)
}

type Executor struct {
	client  Client
	scripts map[ScriptID]string
	mu      sync.Mutex
	shas    map[ScriptID]string
}

func NewExecutor(client Client) *Executor {
	return &Executor{
		client:  client,
		scripts: DefaultScripts(),
		shas:    make(map[ScriptID]string),
	}
}

func (e *Executor) Eval(ctx context.Context, id ScriptID, keys []string, args ...string) (string, error) {
	if e == nil || e.client == nil {
		return "", errors.New("redislua executor client is required")
	}
	body, ok := e.scripts[id]
	if !ok {
		return "", errors.New("redislua script is not registered: " + string(id))
	}
	sha, err := e.sha(ctx, id, body)
	if err != nil {
		return "", err
	}
	result, err := e.client.EvalSHA(ctx, sha, keys, args...)
	if err == nil {
		return result, nil
	}
	if !isNoScript(err) {
		return "", err
	}
	sha, err = e.reload(ctx, id, body)
	if err != nil {
		return "", err
	}
	return e.client.EvalSHA(ctx, sha, keys, args...)
}

func (e *Executor) sha(ctx context.Context, id ScriptID, body string) (string, error) {
	e.mu.Lock()
	sha := e.shas[id]
	e.mu.Unlock()
	if sha != "" {
		return sha, nil
	}
	return e.reload(ctx, id, body)
}

func (e *Executor) reload(ctx context.Context, id ScriptID, body string) (string, error) {
	sha, err := e.client.ScriptLoad(ctx, body)
	if err != nil {
		return "", err
	}
	if sha == "" {
		return "", errors.New("redis SCRIPT LOAD returned empty sha")
	}
	e.mu.Lock()
	e.shas[id] = sha
	e.mu.Unlock()
	return sha, nil
}

func isNoScript(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "NOSCRIPT")
}

func DefaultScripts() map[ScriptID]string {
	return map[ScriptID]string{
		ScriptQuotaReserve: quotaReserveLua,
		ScriptQuotaConfirm: quotaConfirmLua,
		ScriptQuotaRelease: quotaReleaseLua,
		ScriptRateLimit:    rateLimitLua,
	}
}
