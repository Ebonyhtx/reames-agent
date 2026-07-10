// Package feishu: webhook signature verification following the Feishu Open
// Platform spec. In production, Feishu signs each event push with an
// HMAC-SHA256 over the timestamp + request body, using the app's signing
// secret as the key. This module provides verification plus replay-attack
// protection via timestamp tolerance.
package feishu

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// MaxTimestampAge is the maximum allowed age of a webhook event timestamp.
// Events older than this are rejected to prevent replay attacks.
const MaxTimestampAge = 5 * time.Minute

// WebhookSignatureError describes why a signature check failed.
type WebhookSignatureError struct {
	Reason string
}

func (e *WebhookSignatureError) Error() string { return "feishu webhook signature: " + e.Reason }

// VerifyWebhookSignature checks the Feishu webhook signature against the
// request body. The signature header value is "timestamp\nsigningSecret".
// Returns nil if the signature is valid.
//
// This follows the Feishu Open Platform documentation:
//
//	signature = base64(hmac-sha256(signingSecret, timestamp + body))
//
// Refs:
//
//	https://open.feishu.cn/document/server-docs/event-subscription/event-subscription
func VerifyWebhookSignature(timestamp, signature, body []byte, signingSecret string) error {
	if signingSecret == "" {
		return &WebhookSignatureError{Reason: "signing secret not configured"}
	}
	if len(timestamp) == 0 {
		return &WebhookSignatureError{Reason: "missing timestamp"}
	}
	if len(signature) == 0 {
		return &WebhookSignatureError{Reason: "missing signature"}
	}

	// Validate timestamp to prevent replay attacks.
	ts, err := strconv.ParseInt(string(timestamp), 10, 64)
	if err != nil {
		return &WebhookSignatureError{Reason: fmt.Sprintf("invalid timestamp %q: %v", string(timestamp), err)}
	}
	eventTime := time.Unix(ts, 0)
	if age := time.Since(eventTime); age < 0 {
		return &WebhookSignatureError{Reason: fmt.Sprintf("timestamp is in the future: %s", eventTime)}
	} else if age > MaxTimestampAge {
		return &WebhookSignatureError{Reason: fmt.Sprintf("timestamp too old: %s (max age %s)", eventTime, MaxTimestampAge)}
	}

	// Compute expected signature: base64(hmac-sha256(secret, timestamp + body))
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write(timestamp)
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Constant-time comparison.
	actual := string(signature)
	if !hmac.Equal([]byte(expected), []byte(actual)) {
		return &WebhookSignatureError{Reason: "signature mismatch"}
	}

	return nil
}

// ComputeWebhookSignature computes the Feishu-compatible HMAC-SHA256 signature
// for a given body and signing secret at the given timestamp. Useful for tests
// and for verifying the algorithm against known vectors.
func ComputeWebhookSignature(timestamp int64, body []byte, signingSecret string) string {
	mac := hmac.New(sha256.New, []byte(signingSecret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// IsWebhookSignatureError reports whether err is a signature verification error.
func IsWebhookSignatureError(err error) bool {
	var sigErr *WebhookSignatureError
	return errors.As(err, &sigErr)
}
