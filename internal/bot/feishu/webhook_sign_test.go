package feishu

import (
	"strconv"
	"testing"
	"time"
)

func TestVerifyWebhookSignatureValid(t *testing.T) {
	secret := "test-signing-secret"
	body := []byte(`{"challenge":"test-challenge","token":"test-token","type":"url_verification"}`)
	now := time.Now().Unix()
	timestamp := []byte(strconv.FormatInt(now, 10))

	sig := ComputeWebhookSignature(now, body, secret)

	err := VerifyWebhookSignature(timestamp, []byte(sig), body, secret)
	if err != nil {
		t.Fatalf("valid signature should pass: %v", err)
	}
}

func TestVerifyWebhookSignatureRejectsEmptySecret(t *testing.T) {
	err := VerifyWebhookSignature([]byte("123"), []byte("sig"), []byte("body"), "")
	if err == nil {
		t.Fatal("empty signing secret should be rejected")
	}
	if !IsWebhookSignatureError(err) {
		t.Fatalf("should be WebhookSignatureError, got %T", err)
	}
}

func TestVerifyWebhookSignatureRejectsEmptyTimestamp(t *testing.T) {
	err := VerifyWebhookSignature(nil, []byte("sig"), []byte("body"), "secret")
	if err == nil {
		t.Fatal("empty timestamp should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsEmptySignature(t *testing.T) {
	err := VerifyWebhookSignature([]byte("123"), nil, []byte("body"), "secret")
	if err == nil {
		t.Fatal("empty signature should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsWrongSignature(t *testing.T) {
	secret := "secret"
	body := []byte("payload")
	now := time.Now().Unix()

	err := VerifyWebhookSignature(
		[]byte(strconv.FormatInt(now, 10)),
		[]byte("wrong-signature"),
		body,
		secret,
	)
	if err == nil {
		t.Fatal("wrong signature should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsWrongBody(t *testing.T) {
	secret := "secret"
	now := time.Now().Unix()

	sig := ComputeWebhookSignature(now, []byte("correct body"), secret)

	err := VerifyWebhookSignature(
		[]byte(strconv.FormatInt(now, 10)),
		[]byte(sig),
		[]byte("tampered body"),
		secret,
	)
	if err == nil {
		t.Fatal("tampered body should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsReplayAttack(t *testing.T) {
	secret := "secret"
	body := []byte("payload")

	// Use a timestamp from 10 minutes ago (beyond MaxTimestampAge).
	oldTime := time.Now().Add(-10 * time.Minute).Unix()
	sig := ComputeWebhookSignature(oldTime, body, secret)

	err := VerifyWebhookSignature(
		[]byte(strconv.FormatInt(oldTime, 10)),
		[]byte(sig),
		body,
		secret,
	)
	if err == nil {
		t.Fatal("replay attack (old timestamp) should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsFutureTimestamp(t *testing.T) {
	secret := "secret"
	body := []byte("payload")

	futureTime := time.Now().Add(1 * time.Hour).Unix()
	sig := ComputeWebhookSignature(futureTime, body, secret)

	err := VerifyWebhookSignature(
		[]byte(strconv.FormatInt(futureTime, 10)),
		[]byte(sig),
		body,
		secret,
	)
	if err == nil {
		t.Fatal("future timestamp should be rejected")
	}
}

func TestVerifyWebhookSignatureRejectsInvalidTimestamp(t *testing.T) {
	secret := "secret"

	err := VerifyWebhookSignature(
		[]byte("not-a-number"),
		[]byte("sig"),
		[]byte("body"),
		secret,
	)
	if err == nil {
		t.Fatal("invalid timestamp should be rejected")
	}
}

func TestComputeWebhookSignatureIsDeterministic(t *testing.T) {
	secret := "secret"
	body := []byte("body")
	ts := int64(1234567890)

	sig1 := ComputeWebhookSignature(ts, body, secret)
	sig2 := ComputeWebhookSignature(ts, body, secret)

	if sig1 != sig2 {
		t.Fatalf("signatures should be deterministic: %q vs %q", sig1, sig2)
	}
}

func TestComputeWebhookSignatureChangesWithBody(t *testing.T) {
	secret := "secret"
	ts := int64(1234567890)

	sig1 := ComputeWebhookSignature(ts, []byte("a"), secret)
	sig2 := ComputeWebhookSignature(ts, []byte("b"), secret)

	if sig1 == sig2 {
		t.Fatal("different bodies should produce different signatures")
	}
}

func TestComputeWebhookSignatureChangesWithTimestamp(t *testing.T) {
	secret := "secret"
	body := []byte("body")

	sig1 := ComputeWebhookSignature(123, body, secret)
	sig2 := ComputeWebhookSignature(456, body, secret)

	if sig1 == sig2 {
		t.Fatal("different timestamps should produce different signatures")
	}
}
