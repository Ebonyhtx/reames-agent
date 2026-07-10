package control

import (
	"encoding/json"
	"errors"
	"fmt"

	"reames-agent/internal/i18n"
	"reames-agent/internal/provider"
)

// explainError maps a provider failure to a structured ErrorInfo with a stable
// ErrorCode, so frontends can classify errors programmatically instead of
// string-matching on the message. The returned ErrorInfo implements error and
// is backward-compatible with all existing TurnDone.Err checks.
func explainError(err error) error {
	if err == nil {
		return nil
	}

	// Stream interruption.
	if provider.IsStreamInterrupted(err) {
		return NewErrorInfo(ErrStreamInterrupted,
			fmt.Sprintf("model stream interrupted after recovery attempts: %s. The partial response was kept; retry or ask Reames Agent to continue", err.Error())).
			WithDetail(err.Error())
	}
	if provider.IsConnReset(err) {
		return NewErrorInfo(ErrStreamInterrupted,
			fmt.Sprintf("model stream disconnected before completion after retry attempts: %s. Check the provider/proxy connection, then retry or ask Reames Agent to continue", err.Error())).
			WithDetail(err.Error())
	}

	// API errors (429, 503, etc.).
	var apiErr *provider.APIError
	if errors.As(err, &apiErr) {
		msg := i18n.M.ProviderStatusMessage(apiErr.Status)
		if msg == "" {
			return NewErrorInfo(ErrUnknown, err.Error()).WithHTTPStatus(apiErr.Status)
		}
		code := ErrUnknown
		switch {
		case apiErr.Status == 429:
			code = ErrProviderRateLimit
		case apiErr.Status >= 500:
			code = ErrProviderServerError
		case apiErr.Status == 401 || apiErr.Status == 403:
			code = ErrProviderAuth
		}
		ei := NewErrorInfo(code, msg).WithHTTPStatus(apiErr.Status)
		if reason := requestErrorReason(apiErr); reason != "" {
			ei = ei.WithDetail(reason)
		}
		return ei
	}

	// Auth errors.
	var authErr *provider.AuthError
	if errors.As(err, &authErr) {
		msg := i18n.M.ProviderErrAuth
		if authErr.HasKey {
			msg = i18n.M.ProviderErrAuthRejected
		}
		detail := ""
		if authErr.KeyEnv != "" {
			if authErr.KeySource != "" {
				detail = fmt.Sprintf("%s from %s", authErr.KeyEnv, authErr.KeySource)
			} else {
				detail = authErr.KeyEnv
			}
		}
		return NewErrorInfo(ErrProviderAuth, msg).
			WithHTTPStatus(authErr.Status).
			WithDetail(detail)
	}

	// Unknown — wrap in ErrorInfo so frontends always have a Code.
	return NewErrorInfo(ErrUnknown, err.Error())
}

// requestErrorReason returns the provider's verbatim reason for request-shaped
// 4xx (400/422) — the localized line names the category, the body names the
// actual cause (context-length exceeded, unpaired tool_calls). Empty otherwise.
func requestErrorReason(e *provider.APIError) string {
	if e.Status != 400 && e.Status != 422 {
		return ""
	}
	return providerBodyReason(e.Body)
}

// providerBodyReason pulls the human reason from an OpenAI/Anthropic-shaped error
// body ({"error":{"message":…}}), falling back to the trimmed raw body.
func providerBodyReason(body string) string {
	if body == "" {
		return ""
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &parsed) == nil && parsed.Error.Message != "" {
		return clampRunes(parsed.Error.Message, 800)
	}
	return clampRunes(body, 800)
}

func clampRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
