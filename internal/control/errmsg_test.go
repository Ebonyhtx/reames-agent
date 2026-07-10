package control

import (
	"errors"
	"io"
	"strings"
	"testing"

	"reames-agent/internal/i18n"
	"reames-agent/internal/provider"
)

func TestExplainError(t *testing.T) {
	if explainError(nil) != nil {
		t.Error("nil should stay nil")
	}

	// 402 balance error — should have the correct message content.
	bal := explainError(&provider.APIError{Provider: "deepseek", Status: 402, Body: "Insufficient Balance"})
	if bal == nil {
		t.Fatal("402 should produce an error")
	}
	if !strings.Contains(bal.Error(), i18n.M.ProviderErrInsufficientBalance) {
		t.Errorf("402 should contain the insufficient-balance message, got %q", bal.Error())
	}
	// Verify it's an ErrorInfo with the right code.
	var ei ErrorInfo
	if !errors.As(bal, &ei) {
		t.Fatal("402 error should be an ErrorInfo")
	}
	if ei.HTTPStatus != 402 {
		t.Errorf("402 error HTTPStatus = %d, want 402", ei.HTTPStatus)
	}

	// 401 auth error — should name the key env.
	auth := explainError(&provider.AuthError{Provider: "deepseek", KeyEnv: "DEEPSEEK_API_KEY", Status: 401})
	if !strings.Contains(auth.Error(), "DEEPSEEK_API_KEY") {
		t.Errorf("401 should name the key env: %q", auth.Error())
	}
	if !strings.Contains(auth.Error(), i18n.M.ProviderErrAuth) {
		t.Errorf("401 should use the missing-key message: %q", auth.Error())
	}
	if !errors.As(auth, &ei) {
		t.Fatal("401 error should be an ErrorInfo")
	}
	if ei.Code != ErrProviderAuth {
		t.Errorf("401 code = %q, want %q", ei.Code, ErrProviderAuth)
	}

	// 401 with key present — should use server-rejected message.
	rejected := explainError(&provider.AuthError{Provider: "mimo", KeyEnv: "MIMO_API_KEY", Status: 401, HasKey: true})
	if !strings.Contains(rejected.Error(), i18n.M.ProviderErrAuthRejected) {
		t.Errorf("401 with a key present should use the server-rejected message: %q", rejected.Error())
	}
	if !strings.Contains(rejected.Error(), "MIMO_API_KEY") {
		t.Errorf("401 should still name the key env: %q", rejected.Error())
	}

	// 401 with key source.
	sourced := explainError(&provider.AuthError{Provider: "deepseek", KeyEnv: "DEEPSEEK_API_KEY", KeySource: "project .env", Status: 401, HasKey: true})
	if !strings.Contains(sourced.Error(), "DEEPSEEK_API_KEY") && !strings.Contains(sourced.Error(), "project .env") {
		t.Errorf("401 should name the key source: %q", sourced.Error())
	}

	// Status codes should map to localized messages.
	for _, status := range []int{400, 422, 429, 500, 503} {
		got := explainError(&provider.APIError{Provider: "p", Status: status})
		if got.Error() == "" {
			t.Errorf("status %d should map to a localized message, got empty", status)
		}
		// Verify it's an ErrorInfo.
		var ei2 ErrorInfo
		if !errors.As(got, &ei2) {
			t.Errorf("status %d error should be an ErrorInfo", status)
		}
	}

	// 400 with JSON body — should append the provider reason.
	jsonBody := explainError(&provider.APIError{Provider: "deepseek", Status: 400, Body: `{"error":{"message":"This model's maximum context length is 65536 tokens.","type":"invalid_request_error"}}`})
	if !strings.Contains(jsonBody.Error(), i18n.M.ProviderErrBadRequest) || !strings.Contains(jsonBody.Error(), "maximum context length") {
		t.Errorf("400 should append the provider reason from a JSON body, got %q", jsonBody.Error())
	}

	// 422 with raw body.
	rawBody := explainError(&provider.APIError{Provider: "deepseek", Status: 422, Body: "some unparseable detail"})
	if !strings.Contains(rawBody.Error(), "some unparseable detail") {
		t.Errorf("422 should fall back to the raw body, got %q", rawBody.Error())
	}

	// 429 body must not leak into the main message — but it may appear in Detail.
	noLeak := explainError(&provider.APIError{Provider: "deepseek", Status: 429, Body: `{"error":{"message":"slow down"}}`})
	if noLeak == nil {
		t.Fatal("429 should produce an error")
	}
	if !strings.Contains(noLeak.Error(), i18n.M.ProviderErrRateLimited) {
		t.Errorf("429 must contain the rate-limited message, got %q", noLeak.Error())
	}
	// Verify ErrorInfo code.
	if !errors.As(noLeak, &ei) {
		t.Fatal("429 error should be an ErrorInfo")
	}
	if ei.Code != ErrProviderRateLimit {
		t.Errorf("429 code = %q, want %q", ei.Code, ErrProviderRateLimit)
	}

	// Stream interruption.
	interrupted := explainError(&provider.StreamInterruptedError{Err: io.ErrUnexpectedEOF})
	if !strings.Contains(interrupted.Error(), "model stream interrupted") || !strings.Contains(interrupted.Error(), "continue") {
		t.Errorf("stream interruption should be actionable, got %q", interrupted.Error())
	}
	if !errors.As(interrupted, &ei) {
		t.Fatal("stream interruption should be an ErrorInfo")
	}
	if ei.Code != ErrStreamInterrupted {
		t.Errorf("stream interruption code = %q, want %q", ei.Code, ErrStreamInterrupted)
	}

	// Connection reset.
	disconnected := explainError(io.ErrUnexpectedEOF)
	if !strings.Contains(disconnected.Error(), "model stream disconnected") || !strings.Contains(disconnected.Error(), "retry") {
		t.Errorf("connection reset should be actionable, got %q", disconnected.Error())
	}

	// Unknown errors are still wrapped in ErrorInfo but with ErrUnknown code.
	plain := errors.New("some other failure")
	wrapped := explainError(plain)
	if wrapped == nil {
		t.Fatal("unknown errors should still produce an ErrorInfo")
	}
	if !errors.As(wrapped, &ei) {
		t.Fatal("unknown error should be an ErrorInfo")
	}
	if ei.Code != ErrUnknown {
		t.Errorf("unknown error code = %q, want %q", ei.Code, ErrUnknown)
	}
	if !strings.Contains(wrapped.Error(), "some other failure") {
		t.Errorf("unknown error should preserve the original message, got %q", wrapped.Error())
	}
}
