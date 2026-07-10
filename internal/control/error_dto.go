// Package control defines transport-agnostic DTOs for errors surfaced to
// frontends. Frontends MUST use Code and Category for classification, not
// string-matching on Message or Detail.
package control

import (
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
	ErrSessionLocked   ErrorCode = "session_locked"   // another process holds the lease
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

// ErrorInfo is the structured error DTO surfaced to transport layers.
// Frontends classify errors via Code/Category, not by string-matching Message
// or Detail. The Message field is safe to display to users; Detail is optional
// and may contain technical context.
type ErrorInfo struct {
	Code     ErrorCode     `json:"code"`
	Category ErrorCategory `json:"category"`
	Message  string        `json:"message"`           // user-facing summary
	Detail   string        `json:"detail,omitempty"`  // optional technical context
	Retryable bool         `json:"retryable"`         // derived from Category
	HTTPStatus int         `json:"http_status,omitempty"` // original HTTP status, if applicable
}

// IsZero reports whether e is the zero value (no error).
func (e ErrorInfo) IsZero() bool { return e.Code == "" }

// Error implements the error interface so ErrorInfo can be passed where
// errors are expected.
func (e ErrorInfo) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
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
	var ei ErrorInfo
	if errors.As(err, &ei) {
		return ei
	}

	msg := err.Error()

	// Auth errors.
	if strings.Contains(msg, "401") || strings.Contains(msg, "auth") ||
		strings.Contains(msg, "invalid_api_key") || strings.Contains(msg, "Incorrect API key") {
		return NewErrorInfo(ErrProviderAuth, "Authentication failed. Check your API key.")
	}

	// Rate limit.
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate") {
		return NewErrorInfo(ErrProviderRateLimit, "Rate limit reached. Retry after a short wait.")
	}

	// Server errors.
	if strings.Contains(msg, "503") || strings.Contains(msg, "500") ||
		strings.Contains(msg, "overloaded") || strings.Contains(msg, "server error") {
		return NewErrorInfo(ErrProviderServerError, "Provider server error. The service may be temporarily unavailable.")
	}

	// Stream interruption.
	if strings.Contains(msg, "stream") || strings.Contains(msg, "interrupt") ||
		strings.Contains(msg, "EOF") || strings.Contains(msg, "connection") {
		return NewErrorInfo(ErrStreamInterrupted, "Connection to provider was interrupted.")
	}

	// Cancellation.
	if strings.Contains(msg, "cancel") || strings.Contains(msg, "context") {
		return NewErrorInfo(ErrCancelled, "Operation was cancelled.")
	}

	// Timeout.
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return NewErrorInfo(ErrProviderTimeout, "Request timed out. The provider may be slow or unreachable.")
	}

	// Default: unknown.
	return NewErrorInfo(ErrUnknown, msg)
}
