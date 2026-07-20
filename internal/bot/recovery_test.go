package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
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

func openTestDeliveryLedger(t *testing.T, path string, limit int) *deliveryLedger {
	t.Helper()
	ledger, err := openDeliveryLedger(path, limit)
	if err != nil {
		t.Fatalf("openDeliveryLedger: %v", err)
	}
	t.Cleanup(ledger.close)
	return ledger
}

func TestDeliveryLedgerCommitsCursorOnlyAfterDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot", "delivery-ledger.json")
	ledger := openTestDeliveryLedger(t, path, 16)
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
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
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
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
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
	first := openTestDeliveryLedger(t, path, 16)
	if claimed, err := first.claim(msg); err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}

	first.close()
	restarted := openTestDeliveryLedger(t, path, 16)
	if claimed, err := restarted.claim(msg); err != nil || !claimed {
		t.Fatalf("claim after cold-start recovery = %v, %v; want retry", claimed, err)
	}
	key := deliveryRecordKey(msg.Session(), msg.MessageID)
	if got := restarted.state.Records[key].Attempts; got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestDeliveryLedgerRetryPreservesCursorAndRejectsIdentityConflict(t *testing.T) {
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
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
	ledger := openTestDeliveryLedger(t, path, 16)
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

func TestDeliveryLedgerMigratesVersionOneAndReleasesExclusiveLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	legacy := deliveryLedgerState{
		Version:     legacyDeliveryLedgerVersion,
		Records:     map[string]deliveryRecord{},
		Checkpoints: map[string]RecoveryCheckpoint{},
		Sequences:   map[string]int64{},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	first := openTestDeliveryLedger(t, path, 16)
	if first.state.Version != deliveryLedgerVersion || first.state.Obligations == nil {
		t.Fatalf("migrated state = %+v", first.state)
	}
	if _, err := openDeliveryLedger(path, 16); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("second writer error = %v, want exclusive-lock failure", err)
	}
	first.close()
	reopened := openTestDeliveryLedger(t, path, 16)
	if reopened.state.Version != deliveryLedgerVersion {
		t.Fatalf("reopened version = %d", reopened.state.Version)
	}
}

type obligationTestAdapter struct {
	*fakeAdapter
	beforeSend func(OutboundMessage)
	sendErr    error
}

func (a *obligationTestAdapter) Send(ctx context.Context, msg OutboundMessage) (SendResult, error) {
	if a.beforeSend != nil {
		a.beforeSend(msg)
	}
	result, _ := a.fakeAdapter.Send(ctx, msg)
	if a.sendErr != nil {
		return SendResult{}, a.sendErr
	}
	return result, nil
}

func obligationMessages(msg InboundMessage, texts ...string) []OutboundMessage {
	messages := make([]OutboundMessage, 0, len(texts))
	for _, text := range texts {
		messages = append(messages, OutboundMessage{
			ConnectionID: msg.ConnectionID,
			Domain:       msg.Domain,
			ChatID:       msg.ChatID,
			ChatType:     msg.ChatType,
			Text:         text,
			ReplyToMsgID: inboundReplyMessageID(msg),
		})
	}
	return messages
}

func obligationGateway(ledger *deliveryLedger, msg InboundMessage, adapter Adapter, logger *slog.Logger) *BotGateway {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	gw := NewGateway(GatewayConfig{}, nil, logger)
	gw.recovery = ledger
	gw.adapters = []AdapterBinding{{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: adapter}}
	return gw
}

func TestOutboundObligationPersistsAttemptBeforePlatformSendAndCommitsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	ledger := openTestDeliveryLedger(t, path, 16)
	first := recoveryTestMessage("message-obligation-1", "cursor-obligation-1")
	second := recoveryTestMessage("message-obligation-2", "cursor-obligation-2")
	for _, msg := range []InboundMessage{first, second} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	merged := first
	mergeInboundDeliveryClaims(&merged, second)
	obligation, err := ledger.prepareOutboundObligation(merged, obligationMessages(merged, "durable final answer"))
	if err != nil {
		t.Fatal(err)
	}
	key := deliveryRecordKey(obligation.Source, obligation.MessageID)
	adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(merged.Platform, merged.ConnectionID)}
	adapter.beforeSend = func(OutboundMessage) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read ledger during send: %v", readErr)
			return
		}
		var state deliveryLedgerState
		if err := json.Unmarshal(data, &state); err != nil {
			t.Errorf("decode ledger during send: %v", err)
			return
		}
		persisted := state.Obligations[key]
		if persisted.Status != obligationStatusAttempting || persisted.Attempts != 1 {
			t.Errorf("persisted obligation during send = %+v", persisted)
		}
	}
	gw := obligationGateway(ledger, merged, adapter, nil)
	if delivered, err := gw.sendOutboundObligation(context.Background(), key); err != nil || !delivered {
		t.Fatalf("sendOutboundObligation = %v, %v", delivered, err)
	}
	snapshot := ledger.snapshot()
	if snapshot.Obligations != 0 || snapshot.Delivered != 2 || snapshot.Checkpoints != 1 {
		t.Fatalf("atomic completion snapshot = %+v", snapshot)
	}
	checkpoints := ledger.checkpoints(AdapterBinding{ID: merged.ConnectionID, Domain: merged.Domain, Platform: merged.Platform})
	if len(checkpoints) != 1 || checkpoints[0].Cursor != second.RecoveryCursor || checkpoints[0].Sequence != 2 {
		t.Fatalf("atomic completion checkpoint = %+v", checkpoints)
	}
}

func TestOutboundObligationRecoveryWarningMatchesSendAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name       string
		makeState  func(*testing.T, *deliveryLedger, string)
		wantMarker bool
	}{
		{name: "pending", makeState: func(*testing.T, *deliveryLedger, string) {}, wantMarker: false},
		{name: "attempting", makeState: func(t *testing.T, ledger *deliveryLedger, key string) {
			if _, err := ledger.beginOutboundAttempt(key); err != nil {
				t.Fatal(err)
			}
		}, wantMarker: true},
		{name: "failed", makeState: func(t *testing.T, ledger *deliveryLedger, key string) {
			if _, err := ledger.beginOutboundAttempt(key); err != nil {
				t.Fatal(err)
			}
			if err := ledger.failOutboundAttempt(key); err != nil {
				t.Fatal(err)
			}
		}, wantMarker: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "delivery-ledger.json")
			first := openTestDeliveryLedger(t, path, 16)
			msg := recoveryTestMessage("message-"+tc.name, "cursor-"+tc.name)
			if claimed, err := first.claim(msg); err != nil || !claimed {
				t.Fatalf("claim = %v, %v", claimed, err)
			}
			obligation, err := first.prepareOutboundObligation(msg, obligationMessages(msg, "original final answer"))
			if err != nil {
				t.Fatal(err)
			}
			key := deliveryRecordKey(obligation.Source, obligation.MessageID)
			tc.makeState(t, first, key)
			first.close()

			restarted := openTestDeliveryLedger(t, path, 16)
			adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID)}
			gw := obligationGateway(restarted, msg, adapter, nil)
			if delivered, err := gw.sendOutboundObligation(context.Background(), key); err != nil || !delivered {
				t.Fatalf("recover obligation = %v, %v", delivered, err)
			}
			sent := adapter.sentMessages()
			if len(sent) != 1 {
				t.Fatalf("sent = %+v", sent)
			}
			hasMarker := strings.HasPrefix(sent[0].Text, recoveredReplyMarker)
			if hasMarker != tc.wantMarker || !strings.HasSuffix(sent[0].Text, "original final answer") {
				t.Fatalf("recovered text = %q, want marker=%v", sent[0].Text, tc.wantMarker)
			}
		})
	}
}

func TestOutboundObligationResumesAtNextUnconfirmedChunk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	first := openTestDeliveryLedger(t, path, 16)
	msg := recoveryTestMessage("message-multichunk", "cursor-multichunk")
	if claimed, err := first.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	obligation, err := first.prepareOutboundObligation(msg, obligationMessages(msg, "chunk-one", "chunk-two", "chunk-three"))
	if err != nil {
		t.Fatal(err)
	}
	key := deliveryRecordKey(obligation.Source, obligation.MessageID)
	if _, err := first.beginOutboundAttempt(key); err != nil {
		t.Fatal(err)
	}
	if _, complete, _, err := first.acknowledgeOutboundChunk(key); err != nil || complete {
		t.Fatalf("ack first chunk = complete %v, err %v", complete, err)
	}
	if _, err := first.beginOutboundAttempt(key); err != nil {
		t.Fatal(err)
	}
	first.close()

	restarted := openTestDeliveryLedger(t, path, 16)
	adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID)}
	gw := obligationGateway(restarted, msg, adapter, nil)
	if delivered, err := gw.sendOutboundObligation(context.Background(), key); err != nil || !delivered {
		t.Fatalf("resume obligation = %v, %v", delivered, err)
	}
	sent := adapter.sentMessages()
	if len(sent) != 2 || sent[0].Text != recoveredReplyMarker+"chunk-two" || sent[1].Text != "chunk-three" {
		t.Fatalf("resumed chunks = %+v", sent)
	}
}

func TestOutboundObligationSendAndCommitFailuresRemainRetryable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		commitFail bool
	}{
		{name: "platform send", commitFail: false},
		{name: "local commit", commitFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
			msg := recoveryTestMessage("message-failure-"+tc.name, "cursor-failure-"+tc.name)
			if claimed, err := ledger.claim(msg); err != nil || !claimed {
				t.Fatalf("claim = %v, %v", claimed, err)
			}
			obligation, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, "failure-sensitive-answer"))
			if err != nil {
				t.Fatal(err)
			}
			key := deliveryRecordKey(obligation.Source, obligation.MessageID)
			adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID)}
			if tc.commitFail {
				originalWrite := ledger.writeFile
				writes := 0
				ledger.writeFile = func(path string, data []byte, mode os.FileMode) error {
					writes++
					if writes == 2 {
						return errors.New("injected acknowledgement commit failure")
					}
					return originalWrite(path, data, mode)
				}
			} else {
				adapter.sendErr = errors.New("injected platform send failure")
			}
			gw := obligationGateway(ledger, msg, adapter, nil)
			if delivered, err := gw.sendOutboundObligation(context.Background(), key); err == nil || !delivered {
				t.Fatalf("send failure = delivered %v, err %v", delivered, err)
			}
			snapshot := ledger.snapshot()
			if snapshot.Obligations != 1 || snapshot.Checkpoints != 0 || snapshot.Delivered != 0 {
				t.Fatalf("retryable failure snapshot = %+v", snapshot)
			}
			if tc.commitFail && snapshot.ObligationAmbiguous != 1 {
				t.Fatalf("commit ambiguity snapshot = %+v", snapshot)
			}
			if !tc.commitFail && snapshot.Failed != 1 {
				t.Fatalf("send failure snapshot = %+v", snapshot)
			}
		})
	}
}

func TestOutboundObligationBoundsAndContentIdentityFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	ledger := openTestDeliveryLedger(t, path, 1024)
	msg := recoveryTestMessage("message-bounds", "cursor-bounds")
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	tooMany := make([]string, maxOutboundObligationChunks+1)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	if _, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, tooMany...)); err == nil {
		t.Fatal("accepted too many outbound chunks")
	}
	if _, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, strings.Repeat("x", maxOutboundObligationBytes+1))); err == nil {
		t.Fatal("accepted oversized outbound obligation")
	}
	obligation, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, "identity-bound-answer"))
	if err != nil {
		t.Fatal(err)
	}
	key := deliveryRecordKey(obligation.Source, obligation.MessageID)
	ledger.close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state deliveryLedgerState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	corrupt := state.Obligations[key]
	corrupt.Chunks[0] = "tampered-answer"
	state.Obligations[key] = corrupt
	data, err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openDeliveryLedger(path, 1024); err == nil || !strings.Contains(err.Error(), "content identity mismatch") {
		t.Fatalf("corrupt obligation error = %v", err)
	}
}

func TestDuplicateInboundResumesObligationWithoutCreatingController(t *testing.T) {
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	msg := recoveryTestMessage("message-duplicate-obligation", "cursor-duplicate-obligation")
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if _, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, "resume-without-model")); err != nil {
		t.Fatal(err)
	}
	adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID)}
	gw := obligationGateway(ledger, msg, adapter, nil)
	gw.handleMessage(context.Background(), gw.adapters[0], msg)
	if len(gw.controllers) != 0 {
		t.Fatalf("duplicate obligation created controllers: %d", len(gw.controllers))
	}
	if sent := adapter.sentMessages(); len(sent) != 1 || sent[0].Text != "resume-without-model" {
		t.Fatalf("duplicate recovery sends = %+v", sent)
	}
}

func TestOutboundObligationBodyStaysOutOfLogsStatusAndMetrics(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
	msg := recoveryTestMessage("message-private-obligation", "cursor-private-obligation")
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	const secretBody = "UNIQUE_FINAL_BODY_MUST_NOT_LEAK_74291"
	obligation, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, secretBody))
	if err != nil {
		t.Fatal(err)
	}
	adapter := &obligationTestAdapter{fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID), sendErr: errors.New("platform unavailable")}
	gw := obligationGateway(ledger, msg, adapter, logger)
	key := deliveryRecordKey(obligation.Source, obligation.MessageID)
	_, _ = gw.sendOutboundObligation(context.Background(), key)

	status := httptest.NewRecorder()
	gw.handleControlStatus(status, httptest.NewRequest("GET", "/status", nil))
	metrics := httptest.NewRecorder()
	gw.handleControlMetrics(metrics, httptest.NewRequest("GET", "/metrics", nil))
	combined := logs.String() + status.Body.String() + metrics.Body.String()
	if strings.Contains(combined, secretBody) {
		t.Fatalf("outbound body leaked through diagnostics: %s", combined)
	}
	if !strings.Contains(status.Body.String(), `"obligations":1`) || !strings.Contains(metrics.Body.String(), "reamesAgent_bot_outbound_obligations 1") {
		t.Fatalf("privacy-safe obligation counts missing: status=%s metrics=%s", status.Body.String(), metrics.Body.String())
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
			ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
			gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			gw.recovery = ledger
			msg := recoveryTestMessage("turn-message", "turn-cursor")
			msg.Text = "run a turn"
			gw.adapters = []AdapterBinding{{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: tc.adapter}}
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
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
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
	ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
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
	err := gw.handleSlashCommand(context.Background(), adapter, key, stop)
	if err == nil {
		t.Fatal("/stop accepted a failed acknowledgement")
	}
	gw.failInbound(stop, err)
	if got := ledger.snapshot(); got.Failed != 2 || got.Delivered != 0 || got.Checkpoints != 0 {
		t.Fatalf("failed acknowledgement snapshot = %+v", got)
	}
	if claimed, err := ledger.claim(original); err != nil || !claimed {
		t.Fatalf("canceled original retry claim = %v, %v", claimed, err)
	}
}

func TestGatewayStopSettlementWriteFailureKeepsCommandRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	ledger := openTestDeliveryLedger(t, path, 16)
	gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	gw.recovery = ledger
	original := recoveryTestMessage("message-running-settlement-failure", "cursor-running-settlement-failure")
	original.Text = "long task"
	stop := recoveryTestMessage("message-stop-settlement-failure", "cursor-stop-settlement-failure")
	stop.Text = "/stop"
	for _, msg := range []InboundMessage{original, stop} {
		if claimed, err := ledger.claim(msg); err != nil || !claimed {
			t.Fatalf("claim %s = %v, %v", msg.MessageID, claimed, err)
		}
	}
	key := BuildSessionKey(original.Session())
	gw.controllers[key] = &sessionState{activeInbound: &original, cancel: func() {}}
	originalWrite := ledger.writeFile
	failNextWrite := true
	ledger.writeFile = func(path string, data []byte, mode os.FileMode) error {
		if failNextWrite {
			failNextWrite = false
			return errors.New("injected canceled-session commit failure")
		}
		return originalWrite(path, data, mode)
	}

	adapter := newFakeAdapter(PlatformWeixin, "stop-settlement-failure")
	err := gw.handleSlashCommand(context.Background(), adapter, key, stop)
	if err == nil || !strings.Contains(err.Error(), "canceled-session commit failure") {
		t.Fatalf("handle /stop error = %v, want durable settlement failure", err)
	}
	gw.failInbound(stop, err)
	if got := ledger.snapshot(); got.Processing != 1 || got.Failed != 1 || got.Delivered != 0 || got.Checkpoints != 0 {
		t.Fatalf("settlement write failure snapshot = %+v", got)
	}

	ledger.close()
	restarted, err := openDeliveryLedger(path, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.close()
	for _, msg := range []InboundMessage{original, stop} {
		claimed, claimErr := restarted.claim(msg)
		if claimErr != nil || !claimed {
			t.Fatalf("retry claim %q = %t, %v", msg.MessageID, claimed, claimErr)
		}
	}
}

type fakeRecoveryAdapter struct {
	*fakeAdapter
	muRecovery  sync.Mutex
	recovered   []InboundMessage
	checkpoints []RecoveryCheckpoint
	limit       int
	calls       int
	err         error
}

func (f *fakeRecoveryAdapter) RecoverMissed(_ context.Context, checkpoints []RecoveryCheckpoint, limit int) ([]InboundMessage, error) {
	f.muRecovery.Lock()
	defer f.muRecovery.Unlock()
	f.calls++
	f.checkpoints = append([]RecoveryCheckpoint(nil), checkpoints...)
	f.limit = limit
	return append([]InboundMessage(nil), f.recovered...), f.err
}

func (f *fakeRecoveryAdapter) recoveryCall() ([]RecoveryCheckpoint, int) {
	f.muRecovery.Lock()
	defer f.muRecovery.Unlock()
	return append([]RecoveryCheckpoint(nil), f.checkpoints...), f.limit
}

func (f *fakeRecoveryAdapter) recoveryCalls() int {
	f.muRecovery.Lock()
	defer f.muRecovery.Unlock()
	return f.calls
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

func TestStartupObligationsAndHistoryShareOneRecoveryScanBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-ledger.json")
	msg := recoveryTestMessage("message-startup-obligation", "cursor-startup-obligation")
	ledger := openTestDeliveryLedger(t, path, 16)
	if claimed, err := ledger.claim(msg); err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if _, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, "startup obligation")); err != nil {
		t.Fatal(err)
	}
	ledger.close()

	adapter := &fakeRecoveryAdapter{
		fakeAdapter: newFakeAdapter(msg.Platform, msg.ConnectionID),
		recovered:   []InboundMessage{recoveryTestMessage("history-must-wait", "history-cursor")},
	}
	gw := NewGatewayWithAdapterBindings(GatewayConfig{
		Enabled:           map[Platform]bool{msg.Platform: true},
		Allowlist:         AllowlistConfig{AllowAll: true},
		RecoveryPath:      path,
		RecoveryScanLimit: 1,
	}, []AdapterBinding{{ID: msg.ConnectionID, Domain: msg.Domain, Platform: msg.Platform, Adapter: adapter}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()
	if got := adapter.recoveryCalls(); got != 0 {
		t.Fatalf("history recovery calls = %d, want zero after obligation consumes cap", got)
	}
	if sent := adapter.sentMessages(); len(sent) != 1 || sent[0].Text != "startup obligation" {
		t.Fatalf("startup obligation sends = %+v", sent)
	}
}

func TestExplicitSettlementRemovesObligationButFailedAcknowledgementPreservesIt(t *testing.T) {
	for _, tc := range []struct {
		name            string
		settle          func(*deliveryLedger, InboundMessage) error
		wantObligations int
	}{
		{name: "acknowledged cancellation", settle: func(ledger *deliveryLedger, msg InboundMessage) error {
			return ledger.delivered(msg)
		}, wantObligations: 0},
		{name: "failed cancellation acknowledgement", settle: func(ledger *deliveryLedger, msg InboundMessage) error {
			return ledger.fail(msg, errors.New("cancellation acknowledgement failed"))
		}, wantObligations: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := openTestDeliveryLedger(t, filepath.Join(t.TempDir(), "delivery-ledger.json"), 16)
			msg := recoveryTestMessage("message-cancel-"+tc.name, "cursor-cancel-"+tc.name)
			if claimed, err := ledger.claim(msg); err != nil || !claimed {
				t.Fatalf("claim = %v, %v", claimed, err)
			}
			if _, err := ledger.prepareOutboundObligation(msg, obligationMessages(msg, "answer pending cancellation")); err != nil {
				t.Fatal(err)
			}
			if err := tc.settle(ledger, msg); err != nil {
				t.Fatal(err)
			}
			if got := ledger.snapshot().Obligations; got != tc.wantObligations {
				t.Fatalf("obligations after settlement = %d, want %d", got, tc.wantObligations)
			}
		})
	}
}
