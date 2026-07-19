package trust

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<script>alert(1)</script>hello", "hello"},
		{"<style>body{}</style>text", "text"},
		{"<b>bold</b> text", "bold text"},
		{"<a href='x'>link</a>", "link"},
		{"normal text", "normal text"},
		{"<script>hack</script><p>safe</p><style>x</style>", "safe"},
	}
	for _, tt := range tests {
		got := SanitizeHTML(tt.in)
		if got != tt.want {
			t.Errorf("SanitizeHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrapUntrusted(t *testing.T) {
	got := WrapUntrusted("hello world", "web_search")
	if !strings.Contains(got, "UNTRUSTED_WEB_SEARCH_OUTPUT") {
		t.Errorf("missing envelope: %s", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("missing content: %s", got)
	}
}

func TestRedactSecrets(t *testing.T) {
	tests := []struct{ in, contains string }{
		{"my key is sk-abc123def45678901234567890123456", "REDACTED"},
		{"token: ghp_abcdefghijklmnopqrstuvwxyz1234567890", "REDACTED"},
		{"normal text without secrets", ""},
	}
	for _, tt := range tests {
		got := RedactSecrets(tt.in)
		if tt.contains == "" {
			if got != tt.in {
				t.Errorf("unexpected redaction: %q -> %q", tt.in, got)
			}
		} else if !strings.Contains(got, tt.contains) {
			t.Errorf("expected %q in %q", tt.contains, got)
		}
	}
}

func TestRedactSecretsMasksCredentialKeyValues(t *testing.T) {
	secret := "real-secret-value-1234567890"
	in := `DEEPSEEK_API_KEY=` + secret + ` Authorization: Bearer ` + secret + ` {"access_token":"` + secret + `"} PWD=/home/user`
	got := RedactSecrets(in)
	if strings.Contains(got, secret) {
		t.Fatalf("credential value leaked: %q", got)
	}
	if !strings.Contains(got, "Authorization: Bearer [REDACTED]") {
		t.Fatalf("authorization scheme was not preserved: %q", got)
	}
	if !strings.Contains(got, "PWD=/home/user") {
		t.Fatalf("working-directory PWD was falsely redacted: %q", got)
	}
	if again := RedactSecrets(got); again != got {
		t.Fatalf("redaction is not idempotent:\nfirst:  %q\nsecond: %q", got, again)
	}
}

func TestRedactSecretsPreservesSafeURLFieldsAndSpecificLabels(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	in := "https://example.com/plugin.zip?token=" + secret + "&version=2#install"
	got := RedactSecrets(in)
	if strings.Contains(got, secret) {
		t.Fatalf("URL credential leaked: %q", got)
	}
	for _, want := range []string{"token=[REDACTED:OpenAI]", "&version=2", "#install"} {
		if !strings.Contains(got, want) {
			t.Fatalf("redacted URL missing %q: %q", want, got)
		}
	}
	if again := RedactSecrets(got); again != got {
		t.Fatalf("URL redaction is not idempotent:\nfirst:  %q\nsecond: %q", got, again)
	}
}

func TestRedactSecretsLongConcurrentTranscript(t *testing.T) {
	const secret = "real-secret-value-1234567890"
	var transcript strings.Builder
	for i := 0; i < 2_000; i++ {
		fmt.Fprintf(&transcript, "message %d payload=%s DEEPSEEK_API_KEY=%s Authorization: Bearer %s\n", i, strings.Repeat("x", i%31), secret, secret)
	}
	input := transcript.String()

	const workers = 12
	const iterations = 10
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				got := RedactSecrets(input)
				if strings.Contains(got, secret) {
					errs <- "long concurrent redaction leaked the test secret"
					return
				}
				if again := RedactSecrets(got); again != got {
					errs <- "long concurrent redaction was not idempotent"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestSanitizeHTMLNested(t *testing.T) {
	in := "<div><script>alert(1)</script><p>safe</p><style>body{}</style></div>"
	got := SanitizeHTML(in)
	if got != "safe" {
		t.Fatalf("nested sanitize: %q", got)
	}
}

func TestSanitizeHTMLImgOnload(t *testing.T) {
	in := `<img src=x onerror="alert(1)">text`
	got := SanitizeHTML(in)
	if got != "text" {
		t.Fatalf("img onerror: %q", got)
	}
}

func TestRedactSecretsChinese(t *testing.T) {
	in := "我的密钥是 sk-abc123def456789012345678901234567890123456789012"
	got := RedactSecrets(in)
	if got == in {
		t.Fatal("Chinese text with secret should be redacted")
	}
	if !strings.Contains(got, "REDACTED") {
		t.Fatalf("no redaction: %q", got)
	}
}

func TestRedactSecretsNoFalsePositive(t *testing.T) {
	in := "normal text without any keys or tokens"
	got := RedactSecrets(in)
	if got != in {
		t.Fatalf("false positive: %q", got)
	}
}

func TestWrapUntrustedEmpty(t *testing.T) {
	got := WrapUntrusted("", "test")
	if got != "" {
		t.Fatalf("empty should return empty: %q", got)
	}
}
