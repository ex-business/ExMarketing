package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ex-stack/ExMarketing/internal/marketing"
	"github.com/ex-stack/ExMarketing/internal/redislua"
)

var ErrServiceRateLimitRedisNotConfigured = errors.New("SERVICE_RATE_LIMIT_REDIS_NOT_CONFIGURED")

type ServiceRateLimiter interface {
	ConsumeServiceProviderRateLimit(ctx context.Context, cmd marketing.ServiceProviderRateLimitConsume) (marketing.ServiceProviderRuntimeState, bool, error)
}

func (s *Service) SetRedisServiceRateLimiter(limiter ServiceRateLimiter) {
	s.redisRateLimiter = limiter
}

type RedisServiceRateLimiter struct {
	executor  *redislua.Executor
	keyPrefix string
}

func NewRedisServiceRateLimiter(client redislua.Client, keyPrefix string) *RedisServiceRateLimiter {
	limiter := &RedisServiceRateLimiter{
		keyPrefix: strings.Trim(strings.TrimSpace(keyPrefix), ":"),
	}
	if client != nil {
		limiter.executor = redislua.NewExecutor(client)
	}
	return limiter
}

func (l *RedisServiceRateLimiter) ConsumeServiceProviderRateLimit(ctx context.Context, cmd marketing.ServiceProviderRateLimitConsume) (marketing.ServiceProviderRuntimeState, bool, error) {
	state := cmd.State
	now := cmd.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if cmd.Window <= 0 {
		cmd.Window = defaultProviderRateLimitWindow
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	if cmd.Limit <= 0 {
		state.UpdatedAt = now
		return state, true, nil
	}
	if l == nil || l.executor == nil {
		return marketing.ServiceProviderRuntimeState{}, false, ErrServiceRateLimitRedisNotConfigured
	}
	result, err := redislua.ConsumeRateLimit(ctx, l.executor, redislua.RateLimitCommand{
		Key:    l.key(cmd),
		Limit:  cmd.Limit,
		Window: cmd.Window,
		Now:    now,
	})
	if err != nil {
		return marketing.ServiceProviderRuntimeState{}, false, fmt.Errorf("SERVICE_RATE_LIMIT_REDIS_CONSUME_FAILED:%w", err)
	}
	state.RateLimitCount = result.Count
	if result.WindowUnixMS > 0 {
		state.RateLimitWindow = time.UnixMilli(result.WindowUnixMS).UTC()
	}
	state.UpdatedAt = now
	return state, result.OK, nil
}

func (l *RedisServiceRateLimiter) key(cmd marketing.ServiceProviderRateLimitConsume) string {
	prefix := l.keyPrefix
	if prefix == "" {
		prefix = "exmarketing:service-rate-limit"
	}
	state := cmd.State
	parts := []string{
		prefix,
		redisRateLimitKeyPart(state.TenantID),
		redisRateLimitKeyPart(state.ServiceRef),
		redisRateLimitKeyPart(state.InterfaceRef),
		redisRateLimitKeyPart(state.ProviderRef),
		redisRateLimitKeyPart(firstNonEmpty(cmd.PolicyRef, "default")),
		redisRateLimitKeyPart(firstNonEmpty(cmd.Scope, "ANONYMOUS")),
		redisRateLimitKeyPart(firstNonEmpty(cmd.SubjectHash, "anonymous")),
	}
	return strings.Join(parts, ":")
}

func redisRateLimitKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
