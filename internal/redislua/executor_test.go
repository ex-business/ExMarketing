package redislua

import (
	"context"
	"errors"
	"testing"
)

type fakeClient struct {
	loads   int
	evals   int
	sha     string
	result  string
	evalErr error
	keys    []string
	args    []string
}

func (c *fakeClient) ScriptLoad(ctx context.Context, body string) (string, error) {
	c.loads++
	if body == "" {
		return "", errors.New("empty script")
	}
	if c.sha == "" {
		c.sha = "sha-1"
	}
	return c.sha, nil
}

func (c *fakeClient) EvalSHA(ctx context.Context, sha string, keys []string, args ...string) (string, error) {
	c.evals++
	c.keys = append([]string(nil), keys...)
	c.args = append([]string(nil), args...)
	if c.evalErr != nil {
		err := c.evalErr
		c.evalErr = nil
		return "", err
	}
	return c.result, nil
}

func TestReserveQuotaBuildsScriptRequest(t *testing.T) {
	client := &fakeClient{result: `{"ok":true,"code":"OK","total":"1000000","used":"0","frozen":"25000","version":2}`}
	executor := NewExecutor(client)

	result, err := ReserveQuota(context.Background(), executor, QuotaCommand{
		Key:          "exm:quota:act_1:BUDGET:ACTIVITY:TOTAL",
		AmountScaled: "25000",
		NowUnixMS:    "1770000000000",
		RequestID:    "req_1",
	})
	if err != nil {
		t.Fatalf("ReserveQuota returned error: %v", err)
	}
	if !result.OK || result.Code != "OK" || result.Frozen != "25000" || result.Version != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if client.loads != 1 || client.evals != 1 {
		t.Fatalf("expected one load and one eval, got loads=%d evals=%d", client.loads, client.evals)
	}
	if len(client.keys) != 1 || client.keys[0] != "exm:quota:act_1:BUDGET:ACTIVITY:TOTAL" {
		t.Fatalf("unexpected keys: %#v", client.keys)
	}
	if got := client.args; len(got) != 3 || got[0] != "25000" || got[1] != "1770000000000" || got[2] != "req_1" {
		t.Fatalf("unexpected args: %#v", got)
	}
}

func TestExecutorReloadsOnNoScriptOnce(t *testing.T) {
	client := &fakeClient{
		result:  `{"ok":true,"code":"OK","total":"10","used":"1","frozen":"0","version":3}`,
		evalErr: errors.New("NOSCRIPT No matching script. Please use EVAL."),
	}
	executor := NewExecutor(client)

	_, err := ConfirmQuota(context.Background(), executor, QuotaCommand{
		Key:          "quota:1",
		AmountScaled: "1",
		NowUnixMS:    "1770000000000",
		RequestID:    "req_2",
	})
	if err != nil {
		t.Fatalf("ConfirmQuota returned error: %v", err)
	}
	if client.loads != 2 || client.evals != 2 {
		t.Fatalf("expected reload after NOSCRIPT, got loads=%d evals=%d", client.loads, client.evals)
	}
}

func TestDefaultScriptsContainQuotaOperations(t *testing.T) {
	scripts := DefaultScripts()
	for _, id := range []ScriptID{ScriptQuotaReserve, ScriptQuotaConfirm, ScriptQuotaRelease} {
		if scripts[id] == "" {
			t.Fatalf("missing script %s", id)
		}
	}
}
