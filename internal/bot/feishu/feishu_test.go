package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reames-agent/internal/bot"
	"reames-agent/internal/config"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

func TestWebSocketReconnectReportsRunningDegradedAndRecovered(t *testing.T) {
	t.Setenv("FEISHU_TEST_SECRET", "secret")
	a := New(config.FeishuBotConfig{
		AppID:        "app",
		AppSecretEnv: "FEISHU_TEST_SECRET",
		Mode:         "websocket",
	}, slog.New(slog.NewTextHandler(io.Discard, nil))).(*adapter)
	a.retry = bot.RetryConfig{InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}
	var attempts atomic.Int32
	a.wsAttempt = func(ctx context.Context, callbacks websocketLifecycleCallbacks) error {
		if attempts.Add(1) == 1 {
			callbacks.ready()
			return errors.New("fixture disconnect")
		}
		callbacks.reconnected()
		<-ctx.Done()
		return nil
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	awaitFeishuConnectionState(t, a.ConnectionEvents(), bot.AdapterConnecting)
	awaitFeishuConnectionState(t, a.ConnectionEvents(), bot.AdapterRunning)
	awaitFeishuConnectionState(t, a.ConnectionEvents(), bot.AdapterReconnecting)
	awaitFeishuConnectionState(t, a.ConnectionEvents(), bot.AdapterRunning)
	if attempts.Load() < 2 {
		t.Fatalf("websocket attempts = %d, want reconnect", attempts.Load())
	}
}

func awaitFeishuConnectionState(t *testing.T, events <-chan bot.AdapterConnectionEvent, want bot.AdapterConnectionState) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.State == want {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for Feishu connection state %q", want)
		}
	}
}

func TestStartReturnsMissingWebSocketSecret(t *testing.T) {
	t.Setenv("FEISHU_TEST_SECRET", "")
	a := New(config.FeishuBotConfig{
		AppID:        "cli-test",
		AppSecretEnv: "FEISHU_TEST_SECRET",
		Mode:         "websocket",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := a.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FEISHU_TEST_SECRET") {
		t.Fatalf("Start error = %v, want missing secret env", err)
	}
}

func TestWebhookStartReturnsBindFailureBeforeReportingRunning(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	a := New(config.FeishuBotConfig{
		Mode:              "webhook",
		WebhookPort:       port,
		VerificationToken: "fixture-token",
	}, slog.New(slog.NewTextHandler(io.Discard, nil))).(*adapter)

	err = a.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "webhook listen") {
		t.Fatalf("Start error = %v, want synchronous bind failure", err)
	}
	select {
	case event := <-a.ConnectionEvents():
		t.Fatalf("bind failure reported connection state: %+v", event)
	default:
	}
}

func TestPublishInboundBackpressuresInsteadOfDroppingEnvelope(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	a.msgCh <- bot.InboundMessage{MessageID: "first"}
	done := make(chan bool, 1)
	go func() {
		done <- a.publishInbound(context.Background(), bot.InboundMessage{MessageID: "second"})
	}()
	select {
	case <-done:
		t.Fatal("publish returned while the gateway queue was still full")
	case <-time.After(30 * time.Millisecond):
	}
	if got := (<-a.msgCh).MessageID; got != "first" {
		t.Fatalf("first queued message = %q", got)
	}
	if ok := <-done; !ok {
		t.Fatal("publish failed after queue capacity became available")
	}
	if got := (<-a.msgCh).MessageID; got != "second" {
		t.Fatalf("second queued message = %q", got)
	}
}

func TestPublishInboundCancellationReleasesBackpressure(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	a.msgCh <- bot.InboundMessage{MessageID: "first"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.publishInbound(ctx, bot.InboundMessage{MessageID: "second"}) {
		t.Fatal("canceled publish succeeded")
	}
	if got := len(a.msgCh); got != 1 {
		t.Fatalf("queue length = %d, want original envelope only", got)
	}
}

func TestVerificationTokenValidRequiresConfiguredToken(t *testing.T) {
	a := &adapter{cfg: config.FeishuBotConfig{VerificationToken: "expected"}}

	if a.verificationTokenValid("") {
		t.Fatal("missing token should be rejected when verification token is configured")
	}
	if a.verificationTokenValid("wrong") {
		t.Fatal("wrong token should be rejected")
	}
	if !a.verificationTokenValid("expected") {
		t.Fatal("matching token should be accepted")
	}

	a.cfg.VerificationToken = ""
	if a.verificationTokenValid("") {
		t.Fatal("unconfigured verification token should deny all callers")
	}
}

func TestMarkSeenConcurrent(t *testing.T) {
	a := &adapter{seen: make(map[string]bool)}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = a.markSeen(fmt.Sprintf("evt-%d", i%5))
		}(i)
	}
	wg.Wait()

	if got := len(a.seen); got != 5 {
		t.Fatalf("seen size = %d, want 5", got)
	}
	if a.markSeen("evt-1") != true {
		t.Fatal("second markSeen call should report duplicate")
	}
	if a.markSeen("") {
		t.Fatal("empty event id should not be treated as duplicate")
	}
}

func TestCanceledInboundUnmarksEventForPlatformRetry(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	a.msgCh <- bot.InboundMessage{MessageID: "full"}
	if a.markSeen("evt-retry") {
		t.Fatal("first event mark reported duplicate")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.handleMessage(ctx, feishuMsgEvent{
		MessageID: "msg-retry",
		ChatID:    "chat-retry",
		ChatType:  "p2p",
		MsgType:   "text",
		Content:   `{"text":"retry me"}`,
		Sender: feishuSender{SenderID: struct {
			UserID  string `json:"user_id"`
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
		}{OpenID: "open-user"}},
	}, "evt-retry")
	if a.markSeen("evt-retry") {
		t.Fatal("canceled publish left event marked as delivered")
	}
}

func TestHandleCardActionUsesChatType(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	raw := []byte(`{
		"event": {
			"operator": {
				"operator_id": {"open_id": "open-user"}
			},
			"context": {
				"open_message_id": "msg-1",
				"open_chat_id": "chat-1"
			},
			"action": {
				"value": {
					"command": "/approve approval-1",
					"chat_type": "dm"
				}
			}
		}
	}`)

	if !a.handleCardAction(context.Background(), raw) {
		t.Fatal("handleCardAction returned false")
	}

	msg := <-a.msgCh
	if msg.ChatType != bot.ChatDM {
		t.Fatalf("chat type = %q, want %q", msg.ChatType, bot.ChatDM)
	}
	if msg.Text != "/approve approval-1" {
		t.Fatalf("text = %q, want /approve approval-1", msg.Text)
	}
}

func TestHandleCardActionEnqueuesAskAnswerCommand(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	raw := []byte(`{
		"event": {
			"operator": {
				"operator_id": {"open_id": "open-user"}
			},
			"context": {
				"open_message_id": "msg-ask",
				"open_chat_id": "chat-ask"
			},
			"action": {
				"value": {
					"command": "/answer ask-1 2",
					"chat_type": "dm",
					"user_id": "allowed-user"
				}
			}
		}
	}`)

	if !a.handleCardAction(context.Background(), raw) {
		t.Fatal("handleCardAction returned false")
	}

	msg := <-a.msgCh
	if msg.Text != "/answer ask-1 2" {
		t.Fatalf("text = %q, want /answer ask-1 2", msg.Text)
	}
	if msg.UserID != "allowed-user" {
		t.Fatalf("user id = %q, want allowed-user", msg.UserID)
	}
	if msg.OperatorID != "open-user" {
		t.Fatalf("operator id = %q, want open-user (the actual clicker, not the card requester)", msg.OperatorID)
	}
	if msg.ChatID != "chat-ask" || msg.MessageID != "msg-ask" {
		t.Fatalf("message routing = chat %q msg %q, want chat-ask/msg-ask", msg.ChatID, msg.MessageID)
	}
}

func TestHandleCardActionAcceptsDirectOperatorID(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	raw := []byte(`{
		"event": {
			"operator": {
				"open_id": "open-user-direct"
			},
			"context": {
				"open_message_id": "msg-1",
				"open_chat_id": "chat-1"
			},
			"action": {
				"value": {
					"command": "/approve approval-1",
					"chat_type": "dm"
				}
			}
		}
	}`)

	if !a.handleCardAction(context.Background(), raw) {
		t.Fatal("handleCardAction returned false")
	}

	msg := <-a.msgCh
	if msg.UserID != "open-user-direct" {
		t.Fatalf("user id = %q, want open-user-direct", msg.UserID)
	}
	if msg.OperatorID != "open-user-direct" {
		t.Fatalf("operator id = %q, want open-user-direct", msg.OperatorID)
	}
}

func TestHandleCardActionDoesNotTrustCardRequesterAsOperator(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	raw := []byte(`{
		"event": {
			"operator": {
				"operator_id": {"open_id": "clicker"}
			},
			"context": {
				"open_message_id": "msg-1",
				"open_chat_id": "chat-1"
			},
			"action": {
				"value": {
					"command": "/approve approval-1",
					"chat_type": "group",
					"user_id": "requester"
				}
			}
		}
	}`)

	if !a.handleCardAction(context.Background(), raw) {
		t.Fatal("handleCardAction returned false")
	}

	msg := <-a.msgCh
	if msg.UserID != "requester" {
		t.Fatalf("user id = %q, want requester (routing follows the card value)", msg.UserID)
	}
	if msg.OperatorID != "clicker" {
		t.Fatalf("operator id = %q, want clicker (gate follows the real button presser)", msg.OperatorID)
	}
}

func TestHandleMessageTreatsTopicGroupAsGroup(t *testing.T) {
	a := &adapter{
		cfg:    config.FeishuBotConfig{RequireMention: true},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	a.handleMessage(context.Background(), feishuMsgEvent{
		MessageID: "msg-topic",
		ChatID:    "chat-topic",
		ChatType:  "topic_group",
		MsgType:   "text",
		Content:   `{"text":"hello"}`,
		Sender: feishuSender{SenderID: struct {
			UserID  string `json:"user_id"`
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
		}{OpenID: "open-user"}},
		Mentions: []feishuMention{{Key: "@_user_1"}},
	})

	msg := <-a.msgCh
	if msg.ChatType != bot.ChatGroup {
		t.Fatalf("chat type = %q, want group", msg.ChatType)
	}
	if msg.ChatID != "chat-topic" || msg.UserID != "open-user" {
		t.Fatalf("message = %+v, want topic group routing", msg)
	}
}

func TestHandleMessageRequiresMentionInTopicGroup(t *testing.T) {
	a := &adapter{
		cfg:    config.FeishuBotConfig{RequireMention: true},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	a.handleMessage(context.Background(), feishuMsgEvent{
		MessageID: "msg-topic",
		ChatID:    "chat-topic",
		ChatType:  "topic_group",
		MsgType:   "text",
		Content:   `{"text":"hello"}`,
		Sender: feishuSender{SenderID: struct {
			UserID  string `json:"user_id"`
			OpenID  string `json:"open_id"`
			UnionID string `json:"union_id"`
		}{OpenID: "open-user"}},
	})

	select {
	case msg := <-a.msgCh:
		t.Fatalf("message without mention was queued: %+v", msg)
	default:
	}
}

func TestWebSocketDispatcherHandlesCardActionTrigger(t *testing.T) {
	a := &adapter{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		msgCh:  make(chan bot.InboundMessage, 1),
	}
	raw := []byte(`{
		"schema": "2.0",
		"header": {
			"event_id": "evt-card-1",
			"event_type": "card.action.trigger",
			"token": ""
		},
		"event": {
			"operator": {
				"operator_id": {
					"open_id": "open-user",
					"union_id": "union-user"
				}
			},
			"context": {
				"open_message_id": "msg-card-1",
				"open_chat_id": "chat-card-1"
			},
			"action": {
				"value": {
					"command": "/approve approval-2",
					"chat_type": "dm",
					"user_id": "allowed-user"
				}
			}
		}
	}`)

	resp, err := a.newEventDispatcher().Do(context.Background(), raw)
	if err != nil {
		t.Fatalf("dispatcher.Do returned error: %v", err)
	}
	toast, ok := resp.(*callback.CardActionTriggerResponse)
	if !ok {
		t.Fatalf("response = %T, want *callback.CardActionTriggerResponse", resp)
	}
	if toast.Toast == nil || toast.Toast.Type != "success" {
		t.Fatalf("toast = %#v, want success toast", toast.Toast)
	}

	msg := <-a.msgCh
	if msg.Text != "/approve approval-2" {
		t.Fatalf("text = %q, want /approve approval-2", msg.Text)
	}
	if msg.ChatID != "chat-card-1" {
		t.Fatalf("chat id = %q, want chat-card-1", msg.ChatID)
	}
	if msg.UserID != "allowed-user" {
		t.Fatalf("user id = %q, want allowed-user", msg.UserID)
	}

	_, err = a.newEventDispatcher().Do(context.Background(), raw)
	if err != nil {
		t.Fatalf("duplicate dispatcher.Do returned error: %v", err)
	}
	select {
	case duplicate := <-a.msgCh:
		t.Fatalf("duplicate card action enqueued message: %#v", duplicate)
	default:
	}
}

func TestBuildMarkdownCard(t *testing.T) {
	content, err := buildMarkdownCard("hello [docs](https://example.com)")
	if err != nil {
		t.Fatalf("buildMarkdownCard: %v", err)
	}
	var payload struct {
		Schema string `json:"schema"`
		Body   struct {
			Elements []struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			} `json:"elements"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("card content should be valid json: %v", err)
	}
	if payload.Schema != "2.0" {
		t.Fatalf("schema = %q, want 2.0", payload.Schema)
	}
	if len(payload.Body.Elements) != 1 || payload.Body.Elements[0].Tag != "markdown" {
		t.Fatalf("elements = %#v, want one markdown element", payload.Body.Elements)
	}
	if payload.Body.Elements[0].Content != "hello [docs](https://example.com)" {
		t.Fatalf("content = %q, want original markdown", payload.Body.Elements[0].Content)
	}
}
