package pluginregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScopedHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/same-origin" {
			http.Redirect(w, r, "/ok", http.StatusFound)
			return
		}
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()

	client := scopedHTTPClient(source.Client())
	resp, err := client.Get(source.URL + "/cross-origin")
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "redirect changed origin") {
		t.Fatalf("cross-origin redirect err = %v", err)
	}
	resp, err = client.Get(source.URL + "/same-origin")
	if err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	_ = resp.Body.Close()
}

func TestContextFetcherEnforcesLengthAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			_, _ = w.Write([]byte("0123456789"))
		case "/blocked":
			<-r.Context().Done()
		}
	}))
	defer server.Close()

	fetcher := &contextFetcher{ctx: context.Background(), client: scopedHTTPClient(server.Client())}
	if _, err := fetcher.DownloadFile(server.URL+"/large", 5, time.Second); err == nil ||
		(!strings.Contains(err.Error(), "exceeded") && !strings.Contains(err.Error(), "maximum")) {
		t.Fatalf("oversized response err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetcher = &contextFetcher{ctx: ctx, client: scopedHTTPClient(server.Client())}
	if _, err := fetcher.DownloadFile(server.URL+"/blocked", 10, time.Second); err == nil || !strings.Contains(strings.ToLower(err.Error()), "canceled") {
		t.Fatalf("canceled fetch err = %v", err)
	}
}
