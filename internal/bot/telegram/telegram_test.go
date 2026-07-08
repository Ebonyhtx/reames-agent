package telegram

import (
	"testing"
)

func TestNew(t *testing.T) {
	b := New("test-token", func(Incoming) {})
	if b == nil {
		t.Fatal("nil bot")
	}
	if b.token != "test-token" {
		t.Fatalf("token mismatch: %q", b.token)
	}
}

func TestBuildReply(t *testing.T) {
	r := buildReply("hello world")
	if r != "hello world" {
		t.Fatalf("unexpected reply: %q", r)
	}
}

func TestOffsetInitial(t *testing.T) {
	b := New("token", func(Incoming) {})
	if b.offset != 0 {
		t.Fatalf("initial offset should be 0, got %d", b.offset)
	}
}
