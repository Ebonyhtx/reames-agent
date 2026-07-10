// Package control defines transport-agnostic DTOs for errors surfaced to
// frontends. New consumers can classify by Code and Category while legacy
// consumers continue to display the error string.
package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrorCode is a stable, machine-readable code for classifying runtime errors.
// Codes are never removed; deprecated codes remain accepted by frontends but
// are no longer emitted by new controller versions.
type ErrorCode string

const (
	// ErrUnknown is the fallback for unclassified errors.
	ErrUnknown ErrorCode = "unknown"

	// --- Provider / network errors ---
	ErrProviderAuth        ErrorCode = "provider_auth"         // 401, invalid API key
	ErrProviderRateLimit   ErrorCode = "provider_rate_limit"   // 429, quota / rate limit
	ErrProviderServerError ErrorCode = "provider_server_error" // 5xx, upstream unavailable
	ErrProviderTimeout     ErrorCode = "provider_timeout"      // request timeout
	ErrStreamInterrupted   ErrorCode = "stream_interrupted"    // mid-stream disconnect
	ErrProviderUnavailable ErrorCode = "provider_unavailable"  // DNS / connection refused

	// --- Tool errors ---
	ErrToolTimeout    ErrorCode = "tool_timeout"    // tool execution exceeded deadline
	ErrToolPermission ErrorCode = "tool_permission" // denied by permission gate
	ErrToolSandbox    ErrorCode = "tool_sandbox"    // blocked by sandbox policy
	ErrToolNotFound   ErrorCode = "tool_not_found"  // tool name not in registry

	// --- Approval errors ---
	ErrApprovalDenied  ErrorCode = "approval_denied"  // user denied the request
	ErrApprovalTimeout ErrorCode = "approval_timeout" // no response within deadline

	// --- Session / lifecycle errors ---
	ErrSessionLocked   ErrorCode = "session_locked"    // another process holds the lease
	ErrSessionNotFound ErrorCode = "session_not_found" // session path does not exist
	ErrSessionClosed   ErrorCode = "session_closed"    // session was closed mid-operation

	// --- Cancellation ---
	ErrCancelled ErrorCode = "cancelled" // user requested cancellation
)

// ErrorCategory groups error codes for coarse UI routing (banner colour,
// retry affordance, help link).
type ErrorCategory string

const (
	CatAuth      ErrorCategory = "auth"      // credentials — user must fix keys
	CatRetryable ErrorCategory = "retryable" // transient — retry may succeed
	CatFatal     ErrorCategory = "fatal"     // permanent — user must change input
	CatUser      ErrorCategory = "user"      // user action needed (approval, etc.)
	CatCancelled ErrorCategory = "cancelled" // user stopped the operation
)

// categoryFor maps error codes to their stable category.
func categoryFor(code ErrorCode) ErrorCategory {
	switch code {
	case ErrProviderAuth:
		return CatAuth
	case ErrProviderRateLimit, ErrProviderServerError, ErrProviderTimeout,
		ErrStreamInterrupted, ErrProviderUnavailable:
		return CatRetryable
	case ErrToolTimeout, ErrToolPermission, ErrToolSandbox, ErrToolNotFound,
		ErrSessionLocked, ErrSessionNotFound, ErrSessionClosed:
		return CatFatal
	case ErrApprovalDenied, ErrApprovalTimeout:
		return CatUser
	case ErrCancelled:
		return CatCancelled
	default:
		return CatFatal
	}
}

// ErrorInfo is the structured error DTO surfaced to transport layers. New
// frontends can classify errors via Code/Category instead of string-matching
// Message or Detail. Message is safe to display; Detail is technical context.
type ErrorInfo struct {
	Code       ErrorCode     `json:"code"`
	Category   ErrorCategory `json:"category"`
	Message    string        `json:"message"`              // user-facing summary
	Detail     string        `json:"detail,omitempty"`     // optional technical context
	Retryable  bool          `json:"retryable"`            // derived from Category
	HTTPStatus int           `json:"httpStatus,omitempty"` // original HTTP status, if applicable
}

// IsZero reports whether e is the zero value (no error).
func (e ErrorInfo) IsZero() bool { return e.Code == "" }

// Error implements the error interface so ErrorInfo can be passed where
// errors are expected.
func (e ErrorInfo) Error() string {
	if e.Detail != "" && !strings.Contains(e.Message, e.Detail) {
		return fmt.Sprintf("%s\n%s", e.Message, e.Detail)
	}
	return e.Message
}

// NewErrorInfo creates an ErrorInfo from an error code and message.
// Category and Retryable are derived automatically from the code.
func NewErrorInfo(code ErrorCode, message string) ErrorInfo {
	return ErrorInfo{
		Code:      code,
		Category:  categoryFor(code),
		Message:   message,
		Retryable: categoryFor(code) == CatRetryable,
	}
}

// WithDetail returns a copy with the Detail field set.
func (e ErrorInfo) WithDetail(detail string) ErrorInfo {
	e.Detail = detail
	return e
}

// WithHTTPStatus returns a copy with the HTTPStatus field set.
func (e ErrorInfo) WithHTTPStatus(status int) ErrorInfo {
	e.HTTPStatus = status
	return e
}

// ClassifyError converts a Go error into a structured ErrorInfo.
// It inspects the error chain for known provider/tool/approval error types.
func ClassifyError(err error) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}

	// Check for our own ErrorInfo first (round-trip).
	var ptr *ErrorInfo
	if errors.As(err, &ptr) && ptr != nil {
		return *ptr
	}
	var ei ErrorInfo
	if errors.As(err, &ei) {
		return ei
	}

	msg := err.Error()
	lower := strings.ToLower(msg)

	// Auth errors.
	if errorContainsAny(lower, "http 401", "status 401", "invalid_api_key", "incorrect api key", "authentication failed", "unauthorized") {
		return NewErrorInfo(ErrProviderAuth, "Authentication failed. Check your API key.")
	}

	// Rate limit.
	if errorContainsAny(lower, "http 429", "status 429", "rate limit", "rate_limit") {
		return NewErrorInfo(ErrProviderRateLimit, "Rate limit reached. Retry after a short wait.")
	}

	// Server errors.
	if errorContainsAny(lower, "http 500", "status 500", "http 503", "status 503", "overloaded", "server error") {
		return NewErrorInfo(ErrProviderServerError, "Provider server error. The service may be temporarily unavailable.")
	}

	// Timeout must be checked before generic context cancellation.
	if errors.Is(err, context.DeadlineExceeded) || errorContainsAny(lower, "timeout", "timed out", "deadline exceeded") {
		return NewErrorInfo(ErrProviderTimeout, "Request timed out. The provider may be slow or unreachable.")
	}

	// Stream interruption.
	if errorContainsAny(lower, "stream interrupted", "unexpected eof", "connection reset", "broken pipe") {
		return NewErrorInfo(ErrStreamInterrupted, "Connection to provider was interrupted.")
	}

	// Cancellation.
	if errors.Is(err, context.Canceled) || errorContainsAny(lower, "context canceled", "context cancelled", "operation was canceled", "operation was cancelled") {
		return NewErrorInfo(ErrCancelled, "Operation was cancelled.")
	}

	// Default: unknown.
	return NewErrorInfo(ErrUnknown, msg)
}

func errorContainsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
