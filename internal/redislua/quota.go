package redislua

import (
	"context"
	"encoding/json"
	"errors"
)

type QuotaResult struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Total   string `json:"total"`
	Used    string `json:"used"`
	Frozen  string `json:"frozen"`
	Version int64  `json:"version"`
}

type QuotaCommand struct {
	Key          string
	AmountScaled string
	NowUnixMS    string
	RequestID    string
}

func (c QuotaCommand) validate() error {
	if c.Key == "" {
		return errors.New("quota redis key is required")
	}
	if c.AmountScaled == "" {
		return errors.New("quota amount is required")
	}
	if c.NowUnixMS == "" {
		return errors.New("quota nowUnixMs is required")
	}
	return nil
}

func ReserveQuota(ctx context.Context, executor *Executor, command QuotaCommand) (QuotaResult, error) {
	return evalQuota(ctx, executor, ScriptQuotaReserve, command)
}

func ConfirmQuota(ctx context.Context, executor *Executor, command QuotaCommand) (QuotaResult, error) {
	return evalQuota(ctx, executor, ScriptQuotaConfirm, command)
}

func ReleaseQuota(ctx context.Context, executor *Executor, command QuotaCommand) (QuotaResult, error) {
	return evalQuota(ctx, executor, ScriptQuotaRelease, command)
}

func evalQuota(ctx context.Context, executor *Executor, scriptID ScriptID, command QuotaCommand) (QuotaResult, error) {
	if err := command.validate(); err != nil {
		return QuotaResult{}, err
	}
	raw, err := executor.Eval(ctx, scriptID, []string{command.Key}, command.AmountScaled, command.NowUnixMS, command.RequestID)
	if err != nil {
		return QuotaResult{}, err
	}
	var result QuotaResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return QuotaResult{}, err
	}
	return result, nil
}

const quotaReserveLua = `
local cjson = cjson

local key = KEYS[1]
local amount = tonumber(ARGV[1] or "0")
local now_ms = ARGV[2] or ""
local request_id = ARGV[3] or ""

local function result(ok, code, message, total, used, frozen, version)
  return cjson.encode({
    ok = ok,
    code = code,
    message = message or "",
    total = tostring(total or "0"),
    used = tostring(used or "0"),
    frozen = tostring(frozen or "0"),
    version = tonumber(version or "0")
  })
end

if key == nil or key == "" then
  return result(false, "BAD_REQUEST", "quota key is required")
end
if amount == nil or amount <= 0 then
  return result(false, "BAD_REQUEST", "amount must be positive")
end
if redis.call("EXISTS", key) == 0 then
  return result(false, "NOT_FOUND", "quota not found")
end

local total = tonumber(redis.call("HGET", key, "total") or "0")
local used = tonumber(redis.call("HGET", key, "used") or "0")
local frozen = tonumber(redis.call("HGET", key, "frozen") or "0")
local version = tonumber(redis.call("HGET", key, "version") or "0")

if total - used - frozen < amount then
  return result(false, "QUOTA_EXCEEDED", "quota exceeded", total, used, frozen, version)
end

frozen = frozen + amount
version = version + 1
redis.call("HSET", key,
  "frozen", tostring(frozen),
  "version", tostring(version),
  "updatedAt", now_ms,
  "lastRequestId", request_id
)
return result(true, "OK", "", total, used, frozen, version)
`

const quotaConfirmLua = `
local cjson = cjson

local key = KEYS[1]
local amount = tonumber(ARGV[1] or "0")
local now_ms = ARGV[2] or ""
local request_id = ARGV[3] or ""

local function result(ok, code, message, total, used, frozen, version)
  return cjson.encode({
    ok = ok,
    code = code,
    message = message or "",
    total = tostring(total or "0"),
    used = tostring(used or "0"),
    frozen = tostring(frozen or "0"),
    version = tonumber(version or "0")
  })
end

if key == nil or key == "" then
  return result(false, "BAD_REQUEST", "quota key is required")
end
if amount == nil or amount <= 0 then
  return result(false, "BAD_REQUEST", "amount must be positive")
end
if redis.call("EXISTS", key) == 0 then
  return result(false, "NOT_FOUND", "quota not found")
end

local total = tonumber(redis.call("HGET", key, "total") or "0")
local used = tonumber(redis.call("HGET", key, "used") or "0")
local frozen = tonumber(redis.call("HGET", key, "frozen") or "0")
local version = tonumber(redis.call("HGET", key, "version") or "0")

if frozen < amount then
  return result(false, "RESERVATION_CONFLICT", "frozen quota is insufficient", total, used, frozen, version)
end

frozen = frozen - amount
used = used + amount
version = version + 1
redis.call("HSET", key,
  "used", tostring(used),
  "frozen", tostring(frozen),
  "version", tostring(version),
  "updatedAt", now_ms,
  "lastRequestId", request_id
)
return result(true, "OK", "", total, used, frozen, version)
`

const quotaReleaseLua = `
local cjson = cjson

local key = KEYS[1]
local amount = tonumber(ARGV[1] or "0")
local now_ms = ARGV[2] or ""
local request_id = ARGV[3] or ""

local function result(ok, code, message, total, used, frozen, version)
  return cjson.encode({
    ok = ok,
    code = code,
    message = message or "",
    total = tostring(total or "0"),
    used = tostring(used or "0"),
    frozen = tostring(frozen or "0"),
    version = tonumber(version or "0")
  })
end

if key == nil or key == "" then
  return result(false, "BAD_REQUEST", "quota key is required")
end
if amount == nil or amount <= 0 then
  return result(false, "BAD_REQUEST", "amount must be positive")
end
if redis.call("EXISTS", key) == 0 then
  return result(false, "NOT_FOUND", "quota not found")
end

local total = tonumber(redis.call("HGET", key, "total") or "0")
local used = tonumber(redis.call("HGET", key, "used") or "0")
local frozen = tonumber(redis.call("HGET", key, "frozen") or "0")
local version = tonumber(redis.call("HGET", key, "version") or "0")

if frozen < amount then
  return result(false, "RESERVATION_CONFLICT", "frozen quota is insufficient", total, used, frozen, version)
end

frozen = frozen - amount
version = version + 1
redis.call("HSET", key,
  "frozen", tostring(frozen),
  "version", tostring(version),
  "updatedAt", now_ms,
  "lastRequestId", request_id
)
return result(true, "OK", "", total, used, frozen, version)
`
