package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"reames-agent/internal/bot"
	"reames-agent/internal/config"
)

const testToken = "123456:telegram_test_secret"

func TestStartRequiresCredentialEnvironment(t *testing.T) {
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_MISSING"}, testLogger()).(*adapter)
	err := a.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_TEST_MISSING") {
		t.Fatalf("Start error = %v, want missing env name", err)
	}
	if strings.Contains(fmt.Sprint(err), "telegram_test_secret") {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func TestNormalizeAPIBaseRejectsInsecureRemoteAndCredentials(t *testing.T) {
	for _, raw := range []string{
		"http://example.com",
		"https://user:pass@example.com",
		"https://example.com?token=secret",
	} {
		if _, err := normalizeAPIBase(raw); err == nil {
			t.Fatalf("normalizeAPIBase(%q) succeeded", raw)
		}
	}
	for _, raw := range []string{"https://api.telegram.org", "http://127.0.0.1:8080", "http://localhost:8080"} {
		if _, err := normalizeAPIBase(raw); err != nil {
			t.Fatalf("normalizeAPIBase(%q): %v", raw, err)
		}
	}
}

func TestPollingWaitsForDurableSettlementBeforeAdvancingOffset(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	offsets := make(chan string, 8)
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		switch method {
		case "getMe":
			writeResult(t, w, telegramUser{ID: 99, IsBot: true, Username: "reames_test_bot"})
		case "getUpdates":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse getUpdates form: %v", err)
				return
			}
			offset := r.Form.Get("offset")
			offsets <- offset
			if offset == "0" {
				writeResult(t, w, []telegramUpdate{testUpdate(42, 7, 1001, 2002, "hello")})
				return
			}
			waitForRequestEnd(r)
		case "sendMessage", "sendChatAction":
			writeResult(t, w, telegramMessage{MessageID: 88})
		default:
			t.Errorf("unexpected method %q", method)
		}
	})

	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = 100 * time.Millisecond
	a.requestSlack = 100 * time.Millisecond
	a.retryInitial = time.Millisecond
	a.retryMaximum = 2 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	msg := awaitMessage(t, a.Messages())
	if msg.Platform != bot.PlatformTelegram || msg.ChatType != bot.ChatGroup || msg.ChatID != "1001" || msg.UserID != "2002" {
		t.Fatalf("inbound route = %+v", msg)
	}
	if msg.MessageID != "42" || msg.ReplyToMessageID != "7" || msg.RecoveryCursor != "42" || msg.Text != "hello" {
		t.Fatalf("inbound identity = %+v", msg)
	}
	if got := awaitString(t, offsets); got != "0" {
		t.Fatalf("first offset = %q, want 0", got)
	}
	select {
	case got := <-offsets:
		t.Fatalf("poll advanced before durable delivery: offset=%q", got)
	case <-time.After(50 * time.Millisecond):
	}
	a.SettleInbound(msg.MessageID, true)
	if got := awaitString(t, offsets); got != "43" {
		t.Fatalf("confirmed offset = %q, want 43", got)
	}
}

func TestFailedSettlementReplaysSameUpdateWithoutAcknowledgingIt(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	offsets := make(chan string, 8)
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		switch method {
		case "getMe":
			writeResult(t, w, telegramUser{ID: 99, IsBot: true})
		case "getUpdates":
			_ = r.ParseForm()
			offset := r.Form.Get("offset")
			offsets <- offset
			if offset == "0" {
				writeResult(t, w, []telegramUpdate{testUpdate(9, 3, 10, 11, "retry me")})
				return
			}
			waitForRequestEnd(r)
		default:
			t.Errorf("unexpected method %q", method)
		}
	})
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = 100 * time.Millisecond
	a.requestSlack = 100 * time.Millisecond
	a.retryInitial = time.Millisecond
	a.retryMaximum = 2 * time.Millisecond
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	first := awaitMessage(t, a.Messages())
	if got := awaitString(t, offsets); got != "0" {
		t.Fatalf("first offset = %q", got)
	}
	a.SettleInbound(first.MessageID, false)
	second := awaitMessage(t, a.Messages())
	if second.MessageID != first.MessageID {
		t.Fatalf("replayed id = %q, want %q", second.MessageID, first.MessageID)
	}
	if got := awaitString(t, offsets); got != "0" {
		t.Fatalf("failed delivery acknowledged remotely: offset=%q", got)
	}
	a.SettleInbound(second.MessageID, true)
	if got := awaitString(t, offsets); got != "10" {
		t.Fatalf("delivered retry offset = %q, want 10", got)
	}
}

func TestStartRejectsConcurrentRunAndAllowsRestartAfterStop(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		switch method {
		case "getMe":
			writeResult(t, w, telegramUser{ID: 99, IsBot: true})
		case "getUpdates":
			waitForRequestEnd(r)
		default:
			t.Errorf("unexpected method %q", method)
		}
	})
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = 50 * time.Millisecond
	a.requestSlack = 10 * time.Millisecond
	a.stopTimeout = time.Second
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("second Start error = %v, want already started", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestPollingRequestDeadlineReentersReconnectLadder(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	var polls atomic.Int32
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		if method == "getMe" {
			writeResult(t, w, telegramUser{ID: 99, IsBot: true})
			return
		}
		if method != "getUpdates" {
			t.Errorf("unexpected method %q", method)
			return
		}
		polls.Add(1)
		waitForRequestEnd(r)
	})
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = 10 * time.Millisecond
	a.requestSlack = 10 * time.Millisecond
	a.retryInitial = time.Millisecond
	a.retryMaximum = 2 * time.Millisecond
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	deadline := time.Now().Add(time.Second)
	for polls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := polls.Load(); got < 2 {
		t.Fatalf("poll attempts = %d, want reconnect after bounded timeout", got)
	}
}

func TestTransportFailureDoesNotLeakTokenOrURL(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN"}, testLogger()).(*adapter)
	a.client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s with %s", req.URL.String(), testToken)
	})
	err := a.Start(context.Background())
	if err == nil {
		t.Fatal("Start succeeded")
	}
	for _, forbidden := range []string{testToken, "/bot" + testToken, "api.telegram.org"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("transport error leaked %q: %v", forbidden, err)
		}
	}
}

func TestSendUsesNativeReplyMessageID(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	form := make(chan map[string]string, 1)
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		if method == "getMe" {
			writeResult(t, w, telegramUser{ID: 99, IsBot: true})
			return
		}
		if method == "getUpdates" {
			waitForRequestEnd(r)
			return
		}
		if method != "sendMessage" {
			t.Errorf("method = %q", method)
			return
		}
		_ = r.ParseForm()
		form <- map[string]string{
			"chat_id":             r.Form.Get("chat_id"),
			"text":                r.Form.Get("text"),
			"reply_to_message_id": r.Form.Get("reply_to_message_id"),
		}
		writeResult(t, w, telegramMessage{MessageID: 77})
	})
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = time.Second
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	result, err := a.Send(context.Background(), bot.OutboundMessage{ChatID: "1001", Text: "hello", ReplyToMsgID: "7"})
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID != "77" {
		t.Fatalf("message id = %q", result.MessageID)
	}
	got := <-form
	if got["chat_id"] != "1001" || got["text"] != "hello" || got["reply_to_message_id"] != "7" {
		t.Fatalf("send form = %+v", got)
	}
}

func TestGatewayFixtureRetriesFailedDeliveryBeforeAcknowledgingUpdate(t *testing.T) {
	t.Setenv("TELEGRAM_TEST_TOKEN", testToken)
	offsets := make(chan string, 16)
	sends := make(chan map[string]string, 4)
	var sendAttempts atomic.Int32
	server := telegramServer(t, func(w http.ResponseWriter, r *http.Request, method string) {
		switch method {
		case "getMe":
			writeResult(t, w, telegramUser{ID: 99, IsBot: true})
		case "getUpdates":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse getUpdates form: %v", err)
				return
			}
			offset := r.Form.Get("offset")
			offsets <- offset
			if offset == "0" {
				writeResult(t, w, []telegramUpdate{testUpdate(42, 7, 1001, 2002, "/help")})
				return
			}
			waitForRequestEnd(r)
		case "sendMessage":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse sendMessage form: %v", err)
				return
			}
			sends <- map[string]string{
				"chat_id":             r.Form.Get("chat_id"),
				"reply_to_message_id": r.Form.Get("reply_to_message_id"),
			}
			if sendAttempts.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"ok":false,"error_code":502,"description":"fixture failure"}`)
				return
			}
			writeResult(t, w, telegramMessage{MessageID: 88})
		case "sendChatAction":
			writeResult(t, w, true)
		default:
			t.Errorf("unexpected method %q", method)
		}
	})
	ledgerPath := filepath.Join(t.TempDir(), "delivery-ledger.json")
	a := New(config.TelegramBotConfig{TokenEnv: "TELEGRAM_TEST_TOKEN", APIBase: server.URL}, testLogger()).(*adapter)
	a.pollTimeout = 100 * time.Millisecond
	a.requestSlack = 100 * time.Millisecond
	a.retryInitial = time.Millisecond
	a.retryMaximum = 2 * time.Millisecond
	gw := bot.NewGatewayWithAdapterBindings(bot.GatewayConfig{
		Enabled:      map[bot.Platform]bool{bot.PlatformTelegram: true},
		Allowlist:    bot.AllowlistConfig{AllowAll: true},
		RecoveryPath: ledgerPath,
	}, []bot.AdapterBinding{{ID: "telegram-main", Domain: "telegram", Platform: bot.PlatformTelegram, Adapter: a}}, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gw.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer gw.Stop()

	firstOffset := awaitString(t, offsets)
	firstSend := <-sends
	secondOffset := awaitString(t, offsets)
	secondSend := <-sends
	confirmedOffset := awaitString(t, offsets)
	if firstOffset != "0" || secondOffset != "0" || confirmedOffset != "43" {
		t.Fatalf("offset sequence = %q, %q, %q; want 0, 0, 43", firstOffset, secondOffset, confirmedOffset)
	}
	for i, sent := range []map[string]string{firstSend, secondSend} {
		if sent["chat_id"] != "1001" || sent["reply_to_message_id"] != "7" {
			t.Fatalf("send %d = %+v", i+1, sent)
		}
	}
	ledger, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ledger), testToken) || strings.Contains(string(ledger), "telegram_test_secret") {
		t.Fatalf("delivery ledger leaked Telegram credential: %s", ledger)
	}
}

func testUpdate(updateID, messageID, chatID, userID int64, text string) telegramUpdate {
	return telegramUpdate{UpdateID: updateID, Message: &telegramMessage{
		MessageID: messageID,
		Text:      text,
		Chat:      telegramChat{ID: chatID, Type: "supergroup"},
		From:      &telegramUser{ID: userID, FirstName: "Test User", Username: "tester"},
	}}
}

func telegramServer(t *testing.T, handler func(http.ResponseWriter, *http.Request, string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/bot" + testToken + "/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("request path did not use expected credential route")
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		handler(w, r, strings.TrimPrefix(r.URL.Path, prefix))
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	return server
}

func writeResult(t *testing.T, w http.ResponseWriter, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result}); err != nil {
		t.Errorf("write Telegram fixture response: %v", err)
	}
}

func waitForRequestEnd(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(500 * time.Millisecond):
	}
}

func awaitMessage(t *testing.T, ch <-chan bot.InboundMessage) bot.InboundMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Telegram message")
		return bot.InboundMessage{}
	}
}

func awaitString(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Telegram fixture request")
		return ""
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
