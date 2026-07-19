package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reames-agent/internal/event"
)

func recoveryTestMessage(id, cursor string) InboundMessage {
	return InboundMessage{
		Platform:       PlatformWeixin,
		ConnectionID:   "weixin-primary",
		Domain:         "weixin",
		ChatType:       ChatDM,
		ChatID:         "chat-1",
		UserID:         "user-1",
		Text:           "/help",
		MessageID:      id,
		RecoveryCursor: cursor,
	}
}

func TestDeliveryLedgerCommitsCursorOnlyAfterDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot", "delivery-ledger.json")
	ledger, err := openDeliveryLedger(path, 16)
	if err != nil {
		t.Fatalf("openDeliveryLedger: %v", err)
	}
	msg := recoveryTestMessage("message-1", "cursor-1")
	claimed, err := ledger.claim(msg)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v; want true, nil", claimed, err)
	}
	if got := ledger.checkpoints(AdapterBinding{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform}); len(got) != 0 {
		t.Fatalf("checkpoints before delivery = %+v, want none", got)
	}
	if claimed, err := ledger.claim(msg); err != nil || claimed {
		t.Fatalf("duplicate in-flight claim = %v, %v; want false, nil", claimed, err)
	}
	if err := ledger.delivered(msg); err != nil {
		t.Fatalf("delivered: %v", err)
	}
	if claimed, err := ledger.claim(msg); err != nil || claimed {
		t.Fatalf("delivered duplicate claim = %v, %v; want false, nil", claimed, err)
	}
	checkpoints := ledger.checkpoints(AdapterBinding{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform})
	wantSource := recoveryChannelSource(msg.Session())
	if len(checkpoints) != 1 || checkpoints[0].Cursor != msg.RecoveryCursor || checkpoints[0].Source != wantSource || checkpoints[0].Sequence != 1 {
		t.Fatalf("checkpoints = %+v, want committed cursor/source", checkpoints)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat ledger: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("ledger file is empty")
	}
}

func TestDeliveryLedgerAdvancesOnlyContiguousChannelPrefix(t *testing.T) {
	ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	if err != nil {
		t.Fatal(err)
	}
	first := recoveryTestMessage("group-message-1", "group-cursor-1")
	first.ChatType = ChatGroup
	first.UserID = "group-user-1"
	second := recoveryTestMessage("group-message-2", "group-cursor-2")
	second.ChatType = ChatGroup
	second.UserID = "group-user-2"
	for _, msg := range []InboundMessage{first, second} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	if err := ledger.delivered(second); err != nil {
		t.Fatalf("deliver second: %v", err)
	}
	binding := AdapterBinding{ID: first.ConnectionID, Domain: first.Domain, Platform: first.Platform}
	if got := ledger.checkpoints(binding); len(got) != 0 {
		t.Fatalf("checkpoint advanced past failed prefix: %+v", got)
	}
	if err := ledger.delivered(first); err != nil {
		t.Fatalf("deliver first: %v", err)
	}
	checkpoints := ledger.checkpoints(binding)
	if len(checkpoints) != 1 || checkpoints[0].Cursor != second.RecoveryCursor || checkpoints[0].Sequence != 2 {
		t.Fatalf("contiguous checkpoint = %+v, want second cursor at sequence 2", checkpoints)
	}
}

func TestDeliveryLedgerMergedQueueTurnSettlesEveryClaim(t *testing.T) {
	ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	if err != nil {
		t.Fatal(err)
	}
	second := recoveryTestMessage("queued-message-2", "queued-cursor-2")
	third := recoveryTestMessage("queued-message-3", "queued-cursor-3")
	for _, msg := range []InboundMessage{second, third} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	merged := second
	mergeInboundDeliveryClaims(&merged, third)
	if err := ledger.delivered(merged); err != nil {
		t.Fatalf("deliver merged turn: %v", err)
	}
	checkpoint := ledger.checkpoints(AdapterBinding{ID: second.ConnectionID, Domain: second.Domain, Platform: second.Platform})
	if len(checkpoint) != 1 || checkpoint[0].Cursor != third.RecoveryCursor || checkpoint[0].Sequence != 2 {
		t.Fatalf("merged checkpoint = %+v, want third cursor at sequence 2", checkpoint)
	}
	if got := ledger.snapshot(); got.Delivered != 2 || got.Processing != 0 || got.Failed != 0 {
		t.Fatalf("merged snapshot = %+v", got)
	}
}

func TestDeliveryLedgerColdStartRetriesInterruptedClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	msg := recoveryTestMessage("message-interrupted", "cursor-interrupted")
	first, err := openDeliveryLedger(path, 16)
	if err != nil {
		t.Fatalf("open first ledger: %v", err)
	}
	if claimed, err := first.claim(msg); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}

	restarted, err := openDeliveryLedger(path, 16)
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	if claimed, err := restarted.claim(msg); err != nil || !claimed {
		t.Fatalf("claim after cold-start recovery = %v, %v; want retry", claimed, err)
	}
	key := deliveryRecordKey(msg.Session(), msg.MessageID)
	if got := restarted.state.Records[key].Attempts; got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestDeliveryLedgerRetryPreservesCursorAndRejectsIdentityConflict(t *testing.T) {
	ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	if err != nil {
		t.Fatal(err)
	}
	msg := recoveryTestMessage("message-retry", "cursor-original")
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	if err := ledger.fail(msg, errors.New("send failed")); err != nil {
		t.Fatal(err)
	}
	liveRetry := msg
	liveRetry.RecoveryCursor = ""
	if claimed, err := ledger.claim(liveRetry); err != nil || !claimed {
		t.Fatalf("live retry claim = %v, %v", claimed, err)
	}
	record := ledger.state.Records[deliveryRecordKey(msg.Session(), msg.MessageID)]
	if record.Cursor != msg.RecoveryCursor || record.Sequence != 1 {
		t.Fatalf("retry record = %+v, want original cursor/sequence", record)
	}
	if err := ledger.fail(msg, errors.New("send failed again")); err != nil {
		t.Fatal(err)
	}
	conflict := msg
	conflict.RecoveryCursor = "cursor-conflict"
	if claimed, err := ledger.claim(conflict); err == nil || claimed {
		t.Fatalf("conflicting cursor claim = %v, %v; want fail closed", claimed, err)
	}
}

func TestDeliveryLedgerFailureDoesNotAdvanceCursorAndRedactsSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	msg := recoveryTestMessage("message-failed", "cursor-failed")
	ledger, err := openDeliveryLedger(path, 16)
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if err := ledger.fail(msg, errors.New("Authorization: Bearer secret-token-value")); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if got := ledger.checkpoints(AdapterBinding{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform}); len(got) != 0 {
		t.Fatalf("failed message checkpoints = %+v, want none", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-token-value") {
		t.Fatalf("ledger leaked secret: %s", data)
	}
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("failed message retry claim = %v, %v; want true, nil", claimed, err)
	}
}

func TestDeliveryLedgerCorruptionFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"records":{"bad":{}},"checkpoints":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeliveryLedger(path, 16); err == nil {
		t.Fatal("openDeliveryLedger accepted corrupt identity")
	}
}

type recoveryTurnController struct {
	botController
	sink *sessionEventSink
	text string
}

func (c *recoveryTurnController) RunTurn(context.Context, string) error {
	c.sink.Emit(event.Event{Kind: event.TurnStarted})
	c.sink.Emit(event.Event{Kind: event.Text, Text: c.text})
	return nil
}

func (c *recoveryTurnController) SessionPath() string   { return "" }
func (c *recoveryTurnController) WorkspaceRoot() string { return "" }

type failedRecoverySendAdapter struct{ *fakeAdapter }

func (a *failedRecoverySendAdapter) Send(context.Context, OutboundMessage) (SendResult, error) {
	return SendResult{}, errors.New("final delivery unavailable")
}

func TestGatewayRunTurnCommitsOnlySuccessfulFinalSend(t *testing.T) {
	for _, tc := range []struct {
		name       string
		adapter    Adapter
		wantCursor bool
		wantFailed int
	}{
		{name: "delivered", adapter: newFakeAdapter(PlatformWeixin, "delivered"), wantCursor: true},
		{name: "send failed", adapter: &failedRecoverySendAdapter{newFakeAdapter(PlatformWeixin, "failed")}, wantFailed: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
			if err != nil {
				t.Fatal(err)
			}
			gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			gw.recovery = ledger
			msg := recoveryTestMessage("turn-message", "turn-cursor")
			msg.Text = "run a turn"
			if claimed, err := ledger.claim(msg); err != nil || !claimed {
				t.Fatalf("claim = %v, %v", claimed, err)
			}
			key := BuildSessionKey(msg.Session())
			sessionSink := &sessionEventSink{}
			ctrl := &recoveryTurnController{sink: sessionSink, text: "final answer"}
			gw.controllers[key] = &sessionState{
				ctrl:             ctrl,
				sink:             sessionSink,
				platform:         msg.Platform,
				connectionID:     msg.ConnectionID,
				toolApprovalMode: normalizeBotToolApprovalMode(""),
			}
			gw.runTurn(context.Background(), tc.adapter, key, msg, nil)
			checkpoints := ledger.checkpoints(AdapterBinding{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform})
			if got := len(checkpoints) == 1; got != tc.wantCursor {
				t.Fatalf("checkpoint committed = %v, want %v (%+v)", got, tc.wantCursor, checkpoints)
			}
			if got := ledger.snapshot().Failed; got != tc.wantFailed {
				t.Fatalf("failed records = %d, want %d", got, tc.wantFailed)
			}
		})
	}
}

func TestGatewayStopCommitsCanceledInboundBeforeCommandCursor(t *testing.T) {
	ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	if err != nil {
		t.Fatal(err)
	}
	gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gw.recovery = ledger
	original := recoveryTestMessage("message-running", "cursor-running")
	original.Text = "long task"
	stop := recoveryTestMessage("message-stop", "cursor-stop")
	stop.Text = "/stop"
	pending := recoveryTestMessage("message-pending", "cursor-pending")
	pending.Text = "queued follow-up"
	for _, msg := range []InboundMessage{original, pending, stop} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	key := BuildSessionKey(original.Session())
	canceled := false
	gw.controllers[key] = &sessionState{
		activeInbound: &original,
		cancel:        func() { canceled = true },
	}
	adapter := newFakeAdapter(PlatformWeixin, "stop")
	if err := gw.handleSlashCommand(context.Background(), adapter, key, stop); err != nil {
		t.Fatalf("handle /stop: %v", err)
	}
	gw.settleInbound(stop, nil)
	if !canceled {
		t.Fatal("active turn was not canceled")
	}
	checkpoints := ledger.checkpoints(AdapterBinding{ID: stop.ConnectionID, Domain: stop.Domain, Platform: stop.Platform})
	if len(checkpoints) != 1 || checkpoints[0].Cursor != stop.RecoveryCursor || checkpoints[0].Sequence != 3 {
		t.Fatalf("stop checkpoint = %+v, want contiguous command cursor", checkpoints)
	}
}

func TestGatewayStopAcknowledgementFailureLeavesCanceledClaimsRetryable(t *testing.T) {
	ledger, err := openDeliveryLedger(filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	if err != nil {
		t.Fatal(err)
	}
	gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gw.recovery = ledger
	original := recoveryTestMessage("message-running-failed-ack", "cursor-running-failed-ack")
	original.Text = "long task"
	stop := recoveryTestMessage("message-stop-failed-ack", "cursor-stop-failed-ack")
	stop.Text = "/stop"
	for _, msg := range []InboundMessage{original, stop} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	key := BuildSessionKey(original.Session())
	gw.controllers[key] = &sessionState{activeInbound: &original, cancel: func() {}}
	adapter := &failedRecoverySendAdapter{newFakeAdapter(PlatformWeixin, "stop-failed")}
	if err := gw.handleSlashCommand(context.Background(), adapter, key, stop); err == nil {
		t.Fatal("/stop accepted a failed acknowledgement")
	}
	if got := ledger.snapshot(); got.Failed != 2 || got.Delivered != 0 || got.Checkpoints != 0 {
		t.Fatalf("failed acknowledgement snapshot = %+v", got)
	}
	if claimed, err := ledger.claim(original); err != nil || !claimed {
		t.Fatalf("canceled original retry claim = %v, %v", claimed, err)
	}
}

type fakeRecoveryAdapter struct {
	*fakeAdapter
	muRecovery  sync.Mutex
	recovered   []InboundMessage
	checkpoints []RecoveryCheckpoint
	limit       int
	err         error
}

func (f *fakeRecoveryAdapter) RecoverMissed(_ context.Context, checkpoints []RecoveryCheckpoint, limit int) ([]InboundMessage, error) {
	f.muRecovery.Lock()
	defer f.muRecovery.Unlock()
	f.checkpoints = append([]RecoveryCheckpoint(nil), checkpoints...)
	f.limit = limit
	return append([]InboundMessage(nil), f.recovered...), f.err
}

func (f *fakeRecoveryAdapter) recoveryCall() ([]RecoveryCheckpoint, int) {
	f.muRecovery.Lock()
	defer f.muRecovery.Unlock()
	return append([]RecoveryCheckpoint(nil), f.checkpoints...), f.limit
}

func TestGatewayRecoveryScansThenDeduplicatesAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	msg := recoveryTestMessage("message-recovered", "cursor-recovered")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	firstAdapter := &fakeRecoveryAdapter{fakeAdapter: newFakeAdapter(PlatformWeixin, "weixin-primary"), recovered: []InboundMessage{msg}}
	first := NewGatewayWithAdapterBindings(GatewayConfig{
		Enabled:           map[Platform]bool{PlatformWeixin: true},
		Allowlist:         AllowlistConfig{AllowAll: true},
		RecoveryPath:      path,
		RecoveryScanLimit: 5,
	}, []AdapterBinding{{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: firstAdapter}}, logger)
	ctx, cancel := context.WithCancel(context.Background())
	if err := first.Start(ctx); err != nil {
		cancel()
		t.Fatalf("first Start: %v", err)
	}
	if got := len(firstAdapter.sentMessages()); got != 1 {
		cancel()
		first.Stop()
		t.Fatalf("first recovered sends = %d, want 1", got)
	}
	if checkpoints, limit := firstAdapter.recoveryCall(); len(checkpoints) != 0 || limit != 5 {
		cancel()
		first.Stop()
		t.Fatalf("first recovery call checkpoints=%+v limit=%d", checkpoints, limit)
	}
	cancel()
	first.Stop()

	secondAdapter := &fakeRecoveryAdapter{fakeAdapter: newFakeAdapter(PlatformWeixin, "weixin-primary"), recovered: []InboundMessage{msg}}
	second := NewGatewayWithAdapterBindings(GatewayConfig{
		Enabled:           map[Platform]bool{PlatformWeixin: true},
		Allowlist:         AllowlistConfig{AllowAll: true},
		RecoveryPath:      path,
		RecoveryScanLimit: 5,
	}, []AdapterBinding{{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: secondAdapter}}, logger)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := second.Start(ctx2); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	defer second.Stop()
	if got := len(secondAdapter.sentMessages()); got != 0 {
		t.Fatalf("duplicate recovered sends = %d, want 0", got)
	}
	checkpoints, limit := secondAdapter.recoveryCall()
	if len(checkpoints) != 1 || checkpoints[0].Cursor != msg.RecoveryCursor || limit != 5 {
		t.Fatalf("second recovery call checkpoints=%+v limit=%d", checkpoints, limit)
	}
}

func TestGatewayRecoveryEnforcesGlobalScanLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	adapter := &fakeRecoveryAdapter{fakeAdapter: newFakeAdapter(PlatformWeixin, "weixin-primary")}
	for i := 0; i < 3; i++ {
		adapter.recovered = append(adapter.recovered, recoveryTestMessage("message-"+string(rune('a'+i)), "cursor-"+string(rune('a'+i))))
	}
	gw := NewGatewayWithAdapterBindings(GatewayConfig{
		Enabled:           map[Platform]bool{PlatformWeixin: true},
		Allowlist:         AllowlistConfig{AllowAll: true},
		RecoveryPath:      path,
		RecoveryScanLimit: 2,
	}, []AdapterBinding{{ID: "weixin-primary", Domain: "weixin", Platform: PlatformWeixin, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer gw.Stop()
	if got := len(adapter.sentMessages()); got != 2 {
		t.Fatalf("recovered sends = %d, want global cap 2", got)
	}
	if errs := gw.StartErrors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "global remaining limit") {
		t.Fatalf("StartErrors = %v, want scan-limit diagnostic", errs)
	}
}

func TestRecoveryAdapterErrorDegradesWithoutBlockingLiveGateway(t *testing.T) {
	adapter := &fakeRecoveryAdapter{fakeAdapter: newFakeAdapter(PlatformWeixin, "weixin-primary"), err: errors.New("history unavailable")}
	gw := NewGatewayWithAdapterBindings(GatewayConfig{
		Enabled:      map[Platform]bool{PlatformWeixin: true},
		RecoveryPath: filepath.Join(t.TempDir(), "delivery-ledger.json"),
	}, []AdapterBinding{{ID: "weixin-primary", Domain: "weixin", Platform: PlatformWeixin, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err != nil {
		t.Fatalf("Start should keep live adapter available: %v", err)
	}
	defer gw.Stop()
	if errs := gw.StartErrors(); len(errs) != 1 || !strings.Contains(errs[0].Error(), "history unavailable") {
		t.Fatalf("StartErrors = %v", errs)
	}
}
