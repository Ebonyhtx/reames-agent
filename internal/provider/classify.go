package provider

import "strings"

// FailoverReason describes why a provider request failed and what action to take.
type FailoverReason int

const (
	FailNone            FailoverReason = iota
	FailAuth                           // bad API key, revoke/rotate
	FailBilling                        // quota exceeded, top up
	FailRateLimit                      // transient rate limit, retry with backoff
	FailOverloaded                     // server overloaded, retry later
	FailContextOverflow                // prompt too long, compact context
	FailModelNotFound                  // model name wrong or deprecated
	FailBadRequest                     // malformed request, don't retry
	FailNetwork                        // connection error, retry
	FailTimeout                        // request timeout, retry
	FailServerError                    // 5xx, retry
	FailUnknown                        // unrecognized, retry once
)

// ClassifiedError annotates an error with a failover reason and action flags.
type ClassifiedError struct {
	Err               error
	Reason            FailoverReason
	Retryable         bool
	ShouldCompact     bool // reduce context window before retry
	ShouldRotateCreds bool // try another API key
	Message           string
}

// ClassifyError examines an error from a provider API call and returns a
// structured classification with recommended actions. Based on Hermes
// error_classifier.py pattern matching.
func ClassifyError(err error, statusCode int, body string) ClassifiedError {
	ce := ClassifiedError{Err: err, Retryable: true}

	// --- Status code classification ---
	switch statusCode {
	case 401, 403:
		ce.Reason = FailAuth
		ce.Retryable = false
		ce.Message = "authentication failed — check your API key"
		return ce
	case 402:
		ce.Reason = FailBilling
		ce.Retryable = false
		ce.Message = "billing/quota exceeded — check your account balance"
		return ce
	case 429:
		ce.Reason = FailRateLimit
		ce.Message = "rate limited — backing off"
		return ce
	}

	// --- Body pattern matching ---
	lower := strings.ToLower(body)
	errStr := strings.ToLower(err.Error())

	switch {
	// Billing / quota
	case containsAny(lower, "insufficient_quota", "quota exceeded", "billing", "insufficient balance",
		"account balance", "credit limit", "payment required", "out of credits", "充值", "余额不足"):
		ce.Reason = FailBilling
		ce.Retryable = false
		ce.Message = "billing/quota issue detected"

	// Rate limiting
	case containsAny(lower, "rate_limit", "rate limit", "too many requests", "request limit",
		"tpm", "rpm", "tokens per minute", "并发限制", "频率限制"):
		ce.Reason = FailRateLimit
		ce.Message = "rate limit — backing off"

	// Context overflow
	case containsAny(lower, "context_length_exceeded", "maximum context length", "context window",
		"token limit", "too long", "reduce the length", "max_tokens", "上下文长度", "超过最大长度"):
		ce.Reason = FailContextOverflow
		ce.ShouldCompact = true
		ce.Message = "context window exceeded — compact before retry"

	// Model not found
	case containsAny(lower, "model_not_found", "no such model", "model does not exist",
		"deprecated", "invalid model", "模型不存在"):
		ce.Reason = FailModelNotFound
		ce.Retryable = false
		ce.Message = "model not found — check model name"

	// Server overloaded
	case containsAny(lower, "overloaded", "server error", "internal error", "service unavailable",
		"temporarily unavailable", "busy", "high load", "过载", "繁忙"):
		ce.Reason = FailOverloaded
		ce.Message = "server overloaded — retry with backoff"

	// Bad request
	case containsAny(lower, "invalid_request_error", "bad request", "invalid parameter"):
		ce.Reason = FailBadRequest
		ce.Retryable = false
		ce.Message = "bad request — check parameters"

	// Network errors
	case containsAny(errStr, "connection refused", "connection reset", "no such host",
		"network", "eof", "broken pipe", "timeout", "deadline exceeded", "tls"):
		ce.Reason = FailNetwork
		ce.Message = "network error — retry"

	// 5xx server errors
	case statusCode >= 500:
		ce.Reason = FailServerError
		ce.Message = "server error — retry"

	default:
		ce.Reason = FailUnknown
		ce.Message = "unknown error — retry once"
	}

	return ce
}

func containsAny(s string, patterns ...string) bool {
	for _, p := range patterns {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}
