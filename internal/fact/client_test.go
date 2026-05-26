package fact

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestQueryMapsAndCachesProviderResponse(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	var calls int32
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		if req.Method != http.MethodPost {
			t.Fatalf("method=%s, want POST", req.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3","stats":{"depositCount30d":5}}}`)),
			Header:     http.Header{},
		}, nil
	})})
	provider := marketing.FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}, "depositCount30d": {Source: "$.data.stats.depositCount30d", Type: "number"}},
		CacheTTLSeconds: 300,
		Enabled:         true,
		TimeoutMS:       5000,
		UpdatedAt:       time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	}
	request := marketing.FactQueryRequest{TenantID: "site_001", FactCode: "USER_PROFILE", Subject: marketing.EventSubject{Type: "USER", ID: "10001"}, Biz: marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_1"}, Attributes: map[string]any{"amount": "100"}}

	first := client.Query(context.Background(), provider, request)
	if !first.Success || first.Cached {
		t.Fatalf("first response=%+v", first)
	}
	profile, ok := first.Facts["USER_PROFILE"].(map[string]any)
	if !ok || profile["vipLevel"] != "VIP3" {
		t.Fatalf("mapped facts=%#v", first.Facts)
	}
	second := client.Query(context.Background(), provider, request)
	if !second.Success || !second.Cached {
		t.Fatalf("second response=%+v", second)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("provider calls=%d, want 1", calls)
	}
}

func TestQueryReturnsMappingFailureReason(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":3}}`)),
			Header:     http.Header{},
		}, nil
	})})
	provider := marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping:      map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		Enabled:      true,
		UpdatedAt:    time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	}
	response := client.Query(context.Background(), provider, marketing.FactQueryRequest{TenantID: "site_001", FactCode: "USER_PROFILE"})
	if response.Success || response.ErrorCode != CodeMappingFailed || !strings.Contains(response.Error, "FACT_TYPE_MISMATCH") {
		t.Fatalf("response=%+v", response)
	}
}
