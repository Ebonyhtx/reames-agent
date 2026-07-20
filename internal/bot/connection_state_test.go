package bot

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type connectionStateFakeAdapter struct {
	*fakeAdapter
	reporter *ConnectionReporter
}

func newConnectionStateFakeAdapter() *connectionStateFakeAdapter {
	return &connectionStateFakeAdapter{
		fakeAdapter: newFakeAdapter(PlatformTelegram, "connection-state"),
		reporter:    NewConnectionReporter(),
	}
}

func (a *connectionStateFakeAdapter) ConnectionEvents() <-chan AdapterConnectionEvent {
	return a.reporter.Events()
}

func TestConnectionReporterKeepsLatestBoundedState(t *testing.T) {
	reporter := NewConnectionReporter()
	for i := 0; i < 20; i++ {
		reporter.Report(AdapterReconnecting, "poll_failed")
	}
	reporter.Report(AdapterRunning, "")
	var last AdapterConnectionEvent
	for len(reporter.ch) > 0 {
		last = <-reporter.ch
	}
	if last.State != AdapterRunning || last.Reason != "" || last.At.IsZero() {
		t.Fatalf("latest event = %+v, want running", last)
	}
}

func TestGatewayProjectsReconnectLifecycleIntoHealth(t *testing.T) {
	adapter := newConnectionStateFakeAdapter()
	gw := NewGatewayWithAdapterBindings(
		GatewayConfig{Enabled: map[Platform]bool{PlatformTelegram: true}},
		[]AdapterBinding{{ID: "telegram-main", Domain: "telegram", Platform: PlatformTelegram, Adapter: adapter}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()

	adapter.reporter.Report(AdapterReconnecting, "poll_failed")
	awaitAdapterHealth(t, gw, func(health AdapterHealthSnapshot) bool {
		return health.Status == string(AdapterReconnecting) && health.Closed && health.Reconnects == 1 && health.LastError == "poll_failed"
	})
	adapter.reporter.Report(AdapterRunning, "")
	awaitAdapterHealth(t, gw, func(health AdapterHealthSnapshot) bool {
		return health.Status == string(AdapterRunning) && !health.Closed && health.Reconnects == 1 && health.LastError == ""
	})
	adapter.reporter.Report(AdapterReconnecting, "poll_failed")
	adapter.reporter.Report(AdapterReconnecting, "poll_failed")
	awaitAdapterHealth(t, gw, func(health AdapterHealthSnapshot) bool {
		return health.Status == string(AdapterReconnecting) && health.Reconnects == 2
	})
}

func TestGatewayRejectsDuplicateStartWithoutDuplicatingConnectionWatcher(t *testing.T) {
	adapter := newConnectionStateFakeAdapter()
	gw := NewGatewayWithAdapterBindings(
		GatewayConfig{Enabled: map[Platform]bool{PlatformTelegram: true}},
		[]AdapterBinding{{ID: "telegram-main", Domain: "telegram", Platform: PlatformTelegram, Adapter: adapter}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err := gw.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()
	if err := gw.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("second Start error = %v, want already started", err)
	}

	adapter.reporter.Report(AdapterReconnecting, "poll_failed")
	awaitAdapterHealth(t, gw, func(health AdapterHealthSnapshot) bool {
		return health.Status == string(AdapterReconnecting) && health.Reconnects == 1
	})
	time.Sleep(20 * time.Millisecond)
	if health := gw.AdapterHealth(); len(health) != 1 || health[0].Reconnects != 1 {
		t.Fatalf("health after one reconnect = %+v, want one watcher update", health)
	}
}

func TestConnectionStatusAndMetricsBoundReconnectReason(t *testing.T) {
	adapter := newConnectionStateFakeAdapter()
	binding := AdapterBinding{ID: "telegram-main", Domain: "telegram", Platform: PlatformTelegram, Adapter: adapter}
	gw := NewGatewayWithAdapterBindings(
		GatewayConfig{},
		[]AdapterBinding{binding},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	rawReason := "https://api.example.invalid/poll?token=super-secret"
	gw.applyAdapterConnectionEvent(binding, AdapterConnectionEvent{
		State:  AdapterReconnecting,
		Reason: rawReason,
		At:     time.Now().UTC(),
	})
	if health := gw.AdapterHealth(); len(health) != 1 || health[0].LastError != "connection_error" || health[0].Reconnects != 1 {
		t.Fatalf("bounded health = %+v, want fixed reason and one reconnect", health)
	}

	status := httptest.NewRecorder()
	gw.handleControlStatus(status, httptest.NewRequest(http.MethodGet, "/status", nil))
	metrics := httptest.NewRecorder()
	gw.handleControlMetrics(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	combined := status.Body.String() + metrics.Body.String()
	if strings.Contains(combined, rawReason) || strings.Contains(combined, "super-secret") {
		t.Fatalf("control projection leaked raw reconnect reason: %q", combined)
	}
	if !strings.Contains(status.Body.String(), `"last_error":"connection_error"`) {
		t.Fatalf("status = %q, want bounded connection_error", status.Body.String())
	}
	for _, want := range []string{
		`reamesAgent_bot_adapter_reconnects_total{id="telegram-main",platform="telegram",domain="telegram"} 1`,
		`reamesAgent_bot_adapter_status{id="telegram-main",platform="telegram",domain="telegram",status="reconnecting"} 1`,
	} {
		if !strings.Contains(metrics.Body.String(), want) {
			t.Fatalf("metrics = %q, missing %q", metrics.Body.String(), want)
		}
	}
}

func awaitAdapterHealth(t *testing.T, gw *BotGateway, match func(AdapterHealthSnapshot) bool) AdapterHealthSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		health := gw.AdapterHealth()
		if len(health) == 1 && match(health[0]) {
			return health[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("adapter health did not reach expected state: %+v", gw.AdapterHealth())
	return AdapterHealthSnapshot{}
}
