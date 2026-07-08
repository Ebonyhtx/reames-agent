package provider

import (
	"errors"
	"testing"
)

func TestClassifyAuth(t *testing.T) {
	ce := ClassifyError(errors.New("unauthorized"), 401, "")
	if ce.Reason != FailAuth || ce.Retryable {
		t.Fatalf("auth: reason=%d retryable=%v", ce.Reason, ce.Retryable)
	}
}

func TestClassifyBilling(t *testing.T) {
	ce := ClassifyError(errors.New("quota"), 402, "")
	if ce.Reason != FailBilling || ce.Retryable {
		t.Fatalf("billing: reason=%d", ce.Reason)
	}
}

func TestClassifyRateLimit(t *testing.T) {
	ce := ClassifyError(errors.New("slow down"), 429, "")
	if ce.Reason != FailRateLimit {
		t.Fatalf("rate limit: reason=%d", ce.Reason)
	}
}

func TestClassifyContextOverflow(t *testing.T) {
	ce := ClassifyError(errors.New("too long"), 400, `{"error":{"message":"context_length_exceeded"}}`)
	if ce.Reason != FailContextOverflow || !ce.ShouldCompact {
		t.Fatalf("context: reason=%d compact=%v", ce.Reason, ce.ShouldCompact)
	}
}

func TestClassifyModelNotFound(t *testing.T) {
	ce := ClassifyError(errors.New("not found"), 404, `{"error":{"message":"model_not_found"}}`)
	if ce.Reason != FailModelNotFound || ce.Retryable {
		t.Fatalf("model: reason=%d", ce.Reason)
	}
}

func TestClassifyOverloaded(t *testing.T) {
	ce := ClassifyError(errors.New("busy"), 503, "server overloaded")
	if ce.Reason != FailOverloaded {
		t.Fatalf("overloaded: reason=%d", ce.Reason)
	}
}

func TestClassifyNetwork(t *testing.T) {
	ce := ClassifyError(errors.New("connection refused"), 0, "")
	if ce.Reason != FailNetwork {
		t.Fatalf("network: reason=%d", ce.Reason)
	}
}

func TestClassifyBadRequest(t *testing.T) {
	ce := ClassifyError(errors.New("bad"), 400, `{"error":{"message":"invalid_request_error"}}`)
	if ce.Reason != FailBadRequest || ce.Retryable {
		t.Fatalf("bad request: reason=%d retryable=%v", ce.Reason, ce.Retryable)
	}
}

func TestClassifyServerError(t *testing.T) {
	ce := ClassifyError(errors.New("crash"), 500, "")
	if ce.Reason != FailServerError {
		t.Fatalf("server error: reason=%d", ce.Reason)
	}
}

func TestClassifyUnknown(t *testing.T) {
	ce := ClassifyError(errors.New("mystery"), 418, "unexpected teapot")
	if ce.Reason != FailUnknown {
		t.Fatalf("unknown: reason=%d", ce.Reason)
	}
}

func TestClassifyChineseBilling(t *testing.T) {
	ce := ClassifyError(errors.New("fail"), 200, `{"error":{"message":"账户余额不足，请充值"}}`)
	if ce.Reason != FailBilling {
		t.Fatalf("chinese billing: reason=%d", ce.Reason)
	}
}
