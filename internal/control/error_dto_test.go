package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestErrorInfoJSONRoundTrip(t *testing.T) {
	original := NewErrorInfo(ErrProviderAuth, "Invalid API key").
		WithDetail("DEEPSEEK_API_KEY is missing").
		WithHTTPStatus(401)

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored ErrorInfo
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Code != ErrProviderAuth {
		t.Fatalf("Code = %q, want %q", restored.Code, ErrProviderAuth)
	}
	if restored.Category != CatAuth {
		t.Fatalf("Category = %q, want %q", restored.Category, CatAuth)
	}
	if restored.Message != "Invalid API key" {
		t.Fatalf("Message = %q", restored.Message)
	}
	if restored.Detail != "DEEPSEEK_API_KEY is missing" {
		t.Fatalf("Detail = %q", restored.Detail)
	}
	if restored.HTTPStatus != 401 {
		t.Fatalf("HTTPStatus = %d, want 401", restored.HTTPStatus)
	}
	if restored.Retryable {
		t.Fatal("auth errors should not be retryable")
	}
	encoded := string(b)
	if !strings.Contains(encoded, `"httpStatus":401`) || strings.Contains(encoded, "http_status") {
		t.Fatalf("ErrorInfo JSON field naming = %s, want frontend-compatible httpStatus", encoded)
	}
}

func TestErrorInfoIsZero(t *testing.T) {
	var zero ErrorInfo
	if !zero.IsZero() {
		t.Fatal("zero ErrorInfo should report IsZero")
	}
	nonZero := NewErrorInfo(ErrCancelled, "cancelled")
	if nonZero.IsZero() {
		t.Fatal("non-zero ErrorInfo should not report IsZero")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrorCode
	}{
		{"nil", nil, ""},
		{"auth 401", errors.New("HTTP 401: unauthorized"), ErrProviderAuth},
		{"auth invalid key", errors.New("invalid_api_key"), ErrProviderAuth},
		{"rate limit 429", errors.New("HTTP 429: rate limit exceeded"), ErrProviderRateLimit},
		{"server error 503", errors.New("HTTP 503: overloaded"), ErrProviderServerError},
		{"stream interrupt", errors.New("stream interrupted: EOF"), ErrStreamInterrupted},
		{"cancel", errors.New("context canceled"), ErrCancelled},
		{"timeout", errors.New("request timeout"), ErrProviderTimeout},
		{"context deadline", context.DeadlineExceeded, ErrProviderTimeout},
		{"ambiguous generate", errors.New("failed to generate output"), ErrUnknown},
		{"ambiguous author", errors.New("author metadata missing"), ErrUnknown},
		{"unknown", errors.New("something unexpected happened"), ErrUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ei := ClassifyError(tt.err)
			if ei.Code != tt.code {
				t.Fatalf("ClassifyError(%v).Code = %q, want %q", tt.err, ei.Code, tt.code)
			}
			if tt.err == nil && !ei.IsZero() {
				t.Fatal("nil error should produce zero ErrorInfo")
			}
		})
	}
}

func TestErrorInfoRoundTripViaErrorInterface(t *testing.T) {
	original := NewErrorInfo(ErrToolTimeout, "Tool execution timed out").
		WithDetail("bash command exceeded 30s deadline")

	// Round-trip through error interface.
	err := original
	restored := ClassifyError(err)

	if restored.Code != ErrToolTimeout {
		t.Fatalf("round-tripped Code = %q", restored.Code)
	}
	if !strings.Contains(restored.Message, "timed out") {
		t.Fatalf("round-tripped Message = %q", restored.Message)
	}
	ptrRestored := ClassifyError(&original)
	if ptrRestored.Code != ErrToolTimeout {
		t.Fatalf("pointer round-trip Code = %q", ptrRestored.Code)
	}
}

func TestErrorInfoCategories(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		category ErrorCategory
	}{
		{ErrProviderAuth, CatAuth},
		{ErrProviderRateLimit, CatRetryable},
		{ErrProviderServerError, CatRetryable},
		{ErrProviderTimeout, CatRetryable},
		{ErrStreamInterrupted, CatRetryable},
		{ErrProviderUnavailable, CatRetryable},
		{ErrToolTimeout, CatFatal},
		{ErrToolPermission, CatFatal},
		{ErrToolSandbox, CatFatal},
		{ErrToolNotFound, CatFatal},
		{ErrApprovalDenied, CatUser},
		{ErrApprovalTimeout, CatUser},
		{ErrSessionLocked, CatFatal},
		{ErrCancelled, CatCancelled},
		{ErrUnknown, CatFatal},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			ei := NewErrorInfo(tt.code, "test")
			if ei.Category != tt.category {
				t.Fatalf("NewErrorInfo(%q).Category = %q, want %q", tt.code, ei.Category, tt.category)
			}
		})
	}
}

func TestErrorInfoErrorMethod(t *testing.T) {
	ei := NewErrorInfo(ErrProviderAuth, "Invalid API key").
		WithDetail("check DEEPSEEK_API_KEY")
	s := ei.Error()
	if strings.Contains(s, string(ErrProviderAuth)) {
		t.Fatalf("Error() should remain user-readable without a code prefix: %s", s)
	}
	if !strings.Contains(s, "Invalid API key") {
		t.Fatalf("Error() should contain message: %s", s)
	}
	if !strings.Contains(s, "check DEEPSEEK_API_KEY") {
		t.Fatalf("Error() should contain detail: %s", s)
	}
}
