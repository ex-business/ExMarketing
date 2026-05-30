package fact

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/metrics"
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
	response := client.Query(context.Background(), provider, factRequest())
	if response.Success || response.ErrorCode != CodeMappingFailed || response.Reason != ReasonMappingTypeMismatch {
		t.Fatalf("response=%+v", response)
	}
}

func TestQueryRedactsProviderFailureDetails(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("Authorization: Bearer raw-provider-token password=provider-secret")
	})})
	response := client.Query(context.Background(), factProvider(), factRequest())
	if response.Success || response.ErrorCode != CodeProviderHTTPFailed {
		t.Fatalf("expected provider failure, got %+v", response)
	}
	joined := response.Error + " " + response.ProviderStatus + " " + response.Reason
	for _, secret := range []string{"raw-provider-token", "provider-secret"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("provider failure leaked %q in %+v", secret, response)
		}
	}
}

func TestQueryMapsArrayIndexAndSkipsOptionalMissingField(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"tags":["vip","sports"]}}`)),
			Header:     http.Header{},
		}, nil
	})})
	requiredFalse := false
	provider := marketing.FactProvider{
		TenantID:     "site_001",
		FactCode:     "USER_PROFILE",
		ProviderType: "HTTP",
		Endpoint:     "https://example.com/profile",
		Method:       "POST",
		Mapping: map[string]marketing.FactMapping{
			"firstTag": {Source: "$.data.tags[0]", Type: "string"},
			"risk":     {Source: "$.data.risk", Type: "object", Required: &requiredFalse},
		},
		Enabled:   true,
		UpdatedAt: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	}

	response := client.Query(context.Background(), provider, factRequest())
	if !response.Success {
		t.Fatalf("response=%+v", response)
	}
	profile, ok := response.Facts["USER_PROFILE"].(map[string]any)
	if !ok || profile["firstTag"] != "vip" {
		t.Fatalf("mapped facts=%#v", response.Facts)
	}
	if _, ok := profile["risk"]; ok {
		t.Fatalf("optional missing field should be omitted, got %#v", profile)
	}
}

func TestCacheKeyIncludesStandardContextAndProviderVersionButNotTraceID(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	var calls int32
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read provider request: %v", err)
		}
		text := string(body)
		if strings.Contains(text, "traceId") || !strings.Contains(text, "eventType") || !strings.Contains(text, "sourceSystem") || !strings.Contains(text, "occurredAt") {
			t.Fatalf("provider input should include standard context and exclude traceId, got %s", text)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})
	provider := factProvider()
	request := factRequest()
	request.EventID = "evt_1"
	request.EventType = "DEPOSIT_SUCCESS"
	request.SourceSystem = "payment"
	request.OccurredAt = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	request.TraceID = "trace_1"

	first := client.Query(context.Background(), provider, request)
	if !first.Success || first.Cached {
		t.Fatalf("first response=%+v", first)
	}
	sameWithDifferentTrace := request
	sameWithDifferentTrace.TraceID = "trace_2"
	second := client.Query(context.Background(), provider, sameWithDifferentTrace)
	if !second.Success || !second.Cached {
		t.Fatalf("trace-only change should hit cache, response=%+v", second)
	}
	differentEventType := request
	differentEventType.TraceID = "trace_3"
	differentEventType.EventType = "WITHDRAW_SUCCESS"
	third := client.Query(context.Background(), provider, differentEventType)
	if !third.Success || third.Cached {
		t.Fatalf("eventType change should miss cache, response=%+v", third)
	}
	newVersion := provider
	newVersion.UpdatedAt = provider.UpdatedAt.Add(time.Second)
	fourth := client.Query(context.Background(), newVersion, request)
	if !fourth.Success || fourth.Cached {
		t.Fatalf("provider version change should miss cache, response=%+v", fourth)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("provider calls=%d, want 3", calls)
	}
}

func TestQueryDoesNotCacheWhenTTLDisabled(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	var calls int32
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})
	provider := factProvider()
	provider.CacheTTLSeconds = 0

	first := client.Query(context.Background(), provider, factRequest())
	second := client.Query(context.Background(), provider, factRequest())
	if !first.Success || !second.Success || first.Cached || second.Cached {
		t.Fatalf("responses=%+v %+v", first, second)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("provider calls=%d, want 2", calls)
	}
}

func TestQueryMappingFailureReasons(t *testing.T) {
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})
	base := factProvider()

	missing := base
	missing.Mapping = map[string]marketing.FactMapping{"score": {Source: "$.data.score", Type: "number"}}
	if response := client.Query(context.Background(), missing, factRequest()); response.Success || response.Reason != ReasonMappingRequiredMissing {
		t.Fatalf("missing response=%+v", response)
	}

	badKey := base
	badKey.Mapping = map[string]marketing.FactMapping{"facts.USER_PROFILE.vipLevel": {Source: "$.data.vipLevel", Type: "string"}}
	if response := client.Query(context.Background(), badKey, factRequest()); response.Success || response.Reason != ReasonMappingKeyInvalid {
		t.Fatalf("bad key response=%+v", response)
	}

	badSource := base
	badSource.Mapping = map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.*", Type: "string"}}
	if response := client.Query(context.Background(), badSource, factRequest()); response.Success || response.Reason != ReasonMappingSourceInvalid {
		t.Fatalf("bad source response=%+v", response)
	}

	badConfig := base
	badConfig.Mapping = map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "scalar"}}
	if response := client.Query(context.Background(), badConfig, factRequest()); response.Success || response.Reason != ReasonMappingConfigInvalid {
		t.Fatalf("bad config response=%+v", response)
	}
}

func TestQueryRequestValidationAndEndpointCodes(t *testing.T) {
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("provider should not be called")
		return nil, nil
	})})
	provider := factProvider()
	request := factRequest()
	request.Attributes = nil
	if response := client.Query(context.Background(), provider, request); response.Success || response.ErrorCode != CodeInvalidFactQuery {
		t.Fatalf("invalid request response=%+v", response)
	}

	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "allowed.example.com")
	if response := client.Query(context.Background(), provider, factRequest()); response.Success || response.ErrorCode != CodeProviderHostBlocked {
		t.Fatalf("host blocked response=%+v", response)
	}
}

func TestQueryRecordsPrometheusMetrics(t *testing.T) {
	previousMetrics := metrics.Default
	t.Cleanup(func() { metrics.Default = previousMetrics })
	metrics.Default = metrics.NewRegistry()
	t.Setenv("EX_MARKETING_FACT_ALLOWLIST", "example.com")
	client := NewClientWithHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"data":{"vipLevel":"VIP3"}}`)),
			Header:     http.Header{},
		}, nil
	})})
	provider := factProvider()

	first := client.Query(context.Background(), provider, factRequest())
	second := client.Query(context.Background(), provider, factRequest())
	if !first.Success || !second.Success || !second.Cached {
		t.Fatalf("responses=%+v %+v", first, second)
	}

	rendered := string(metrics.Default.RenderPrometheus())
	for _, want := range []string{
		`exmarketing_fact_query_total{fact_code="USER_PROFILE",reason="ok",result="success",tenant_id="site_001"} 1`,
		`exmarketing_fact_query_total{fact_code="USER_PROFILE",reason="cache_hit",result="cache_hit",tenant_id="site_001"} 1`,
		`exmarketing_fact_query_latency_seconds_count{fact_code="USER_PROFILE",result="success",tenant_id="site_001"} 1`,
		`exmarketing_fact_query_latency_seconds_count{fact_code="USER_PROFILE",result="cache_hit",tenant_id="site_001"} 1`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("metrics missing %q\n%s", want, rendered)
		}
	}
}

func factProvider() marketing.FactProvider {
	return marketing.FactProvider{
		TenantID:        "site_001",
		FactCode:        "USER_PROFILE",
		ProviderType:    "HTTP",
		Endpoint:        "https://example.com/profile",
		Method:          "POST",
		Mapping:         map[string]marketing.FactMapping{"vipLevel": {Source: "$.data.vipLevel", Type: "string"}},
		CacheTTLSeconds: 300,
		Enabled:         true,
		TimeoutMS:       5000,
		UpdatedAt:       time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
	}
}

func factRequest() marketing.FactQueryRequest {
	return marketing.FactQueryRequest{
		TenantID:   "site_001",
		FactCode:   "USER_PROFILE",
		Subject:    marketing.EventSubject{Type: "USER", ID: "10001"},
		Biz:        marketing.EventBiz{BizType: "DEPOSIT", BizID: "pay_1"},
		Attributes: map[string]any{"amount": "100"},
	}
}
