package harness

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTextResponse(t *testing.T) {
	srv := New(Script{
		TextChunk("Hello, world!"),
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Hello, world!") {
		t.Fatalf("body missing expected text: %s", body)
	}
	if !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("body missing [DONE] marker: %s", body)
	}
}

func TestAuthError401(t *testing.T) {
	srv := New(Script{
		AuthError401("DEEPSEEK_API_KEY"),
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid_api_key") {
		t.Fatalf("body missing error code: %s", body)
	}
}

func TestRateLimit429(t *testing.T) {
	srv := New(Script{
		RateLimit429(),
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 429 {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
}

func TestServerError503(t *testing.T) {
	srv := New(Script{
		ServerError503(),
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestMultipleSteps(t *testing.T) {
	srv := New(Script{
		TextChunk("first"),
		AuthError401("KEY"),
		TextChunk("third"),
	})
	defer srv.Close()

	// Step 1: 200 + text
	resp, _ := http.Get(srv.URL() + "/chat/completions")
	if resp.StatusCode != 200 {
		t.Fatalf("step 1: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Step 2: 401
	resp, _ = http.Get(srv.URL() + "/chat/completions")
	if resp.StatusCode != 401 {
		t.Fatalf("step 2: status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Step 3: 200 again
	resp, _ = http.Get(srv.URL() + "/chat/completions")
	if resp.StatusCode != 200 {
		t.Fatalf("step 3: status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if len(srv.Requests()) != 3 {
		t.Fatalf("requests = %d, want 3", len(srv.Requests()))
	}
}

func TestDelayBefore(t *testing.T) {
	srv := New(Script{
		{Status: 200, Chunks: []Chunk{{Text: "delayed"}}, DelayBefore: 200 * time.Millisecond},
	})
	defer srv.Close()

	start := time.Now()
	resp, err := http.Get(srv.URL() + "/chat/completions")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 100ms (delay)", elapsed)
	}
}

func TestStreamDisconnect(t *testing.T) {
	srv := New(Script{
		StreamDisconnect("partial"),
	})
	defer srv.Close()

	resp, err := http.Get(srv.URL() + "/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "partial") {
		t.Fatalf("body missing partial text: %s", body)
	}
	// After disconnect, [DONE] should NOT be present.
	if strings.Contains(string(body), "[DONE]") {
		t.Fatalf("body should not contain [DONE] after disconnect: %s", body)
	}
}

func TestRequestsAreRecorded(t *testing.T) {
	srv := New(Script{
		TextChunk("a"),
		TextChunk("b"),
	})
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL()+"/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	http.DefaultClient.Do(req)

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	if reqs[0].Auth != "Bearer test-key" {
		t.Fatalf("auth = %q, want 'Bearer test-key'", reqs[0].Auth)
	}
}

func TestReset(t *testing.T) {
	srv := New(Script{
		TextChunk("first-script"),
	})
	defer srv.Close()

	// Use first script.
	resp, _ := http.Get(srv.URL() + "/chat/completions")
	resp.Body.Close()

	srv.Reset(Script{
		AuthError401("NEW_KEY"),
	})

	resp, _ = http.Get(srv.URL() + "/chat/completions")
	if resp.StatusCode != 401 {
		t.Fatalf("after reset: status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	if len(srv.Requests()) != 1 { // only requests after reset
		t.Fatalf("requests after reset = %d, want 1", len(srv.Requests()))
	}
}
