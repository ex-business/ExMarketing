package redislua

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type RateLimitResult struct {
	OK           bool   `json:"ok"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	Count        int    `json:"count"`
	Limit        int    `json:"limit"`
	WindowUnixMS int64  `json:"windowUnixMs"`
	RetryAfterMS int64  `json:"retryAfterMs"`
}

type RateLimitCommand struct {
	Key    string
	Limit  int
	Window time.Duration
	Now    time.Time
}

func (c RateLimitCommand) validate() error {
	if c.Key == "" {
		return errors.New("rate limit redis key is required")
	}
	if c.Limit < 0 {
		return errors.New("rate limit must be non-negative")
	}
	if c.Window <= 0 {
		return errors.New("rate limit window must be positive")
	}
	return nil
}

func ConsumeRateLimit(ctx context.Context, executor *Executor, command RateLimitCommand) (RateLimitResult, error) {
	if err := command.validate(); err != nil {
		return RateLimitResult{}, err
	}
	now := command.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	raw, err := executor.Eval(ctx, ScriptRateLimit, []string{command.Key},
		strconv.Itoa(command.Limit),
		strconv.FormatInt(command.Window.Milliseconds(), 10),
		strconv.FormatInt(now.UnixMilli(), 10),
	)
	if err != nil {
		return RateLimitResult{}, err
	}
	var result RateLimitResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return RateLimitResult{}, err
	}
	return result, nil
}

const rateLimitLua = `
local cjson = cjson

local key = KEYS[1]
local limit = tonumber(ARGV[1] or "0")
local window_ms = tonumber(ARGV[2] or "0")
local now_ms = tonumber(ARGV[3] or "0")

local function result(ok, code, message, count, retry_after_ms)
  local ttl = tonumber(retry_after_ms or "0")
  local window_start = now_ms
  if ttl > 0 and window_ms > ttl then
    window_start = now_ms - (window_ms - ttl)
  end
  return cjson.encode({
    ok = ok,
    code = code,
    message = message or "",
    count = tonumber(count or "0"),
    limit = tonumber(limit or "0"),
    windowUnixMs = tonumber(window_start or "0"),
    retryAfterMs = tonumber(ttl or "0")
  })
end

if key == nil or key == "" then
  return result(false, "BAD_REQUEST", "rate limit key is required", 0, 0)
end
if limit == nil or limit < 0 then
  return result(false, "BAD_REQUEST", "limit must be non-negative", 0, 0)
end
if window_ms == nil or window_ms <= 0 then
  return result(false, "BAD_REQUEST", "window must be positive", 0, 0)
end
if limit == 0 then
  return result(true, "OK", "", 0, 0)
end

local current_raw = redis.call("GET", key)
if current_raw == false then
  redis.call("PSETEX", key, window_ms, "1")
  return result(true, "OK", "", 1, window_ms)
end

local ttl = tonumber(redis.call("PTTL", key) or "-2")
if ttl == nil or ttl <= 0 then
  redis.call("PSETEX", key, window_ms, "1")
  return result(true, "OK", "", 1, window_ms)
end

local current = tonumber(current_raw or "0") or 0
if current >= limit then
  return result(false, "RATE_LIMITED", "rate limit exceeded", current, ttl)
end

current = tonumber(redis.call("INCR", key) or "0") or current + 1
return result(true, "OK", "", current, ttl)
`
