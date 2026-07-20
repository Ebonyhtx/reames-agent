package weixin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"reames-agent/internal/bot"
	"reames-agent/internal/config"
)

func TestStartReturnsMissingToken(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "")
	a := New(config.WeixinBotConfig{
		TokenEnv:  "WEIXIN_TEST_TOKEN",
		AccountID: "missing-account",
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	err := a.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "WEIXIN_TEST_TOKEN") {
		t.Fatalf("Start error = %v, want missing token env", err)
	}
}

func TestSendTextPostsIlinkMessage(t *testing.T) {
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	var gotAuth string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != sendMessagePath {
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":        0,
			"errcode":    0,
			"message_id": "wx-msg-1",
		})
	}))
	defer server.Close()

	result, err := SendText(context.Background(), config.WeixinBotConfig{
		TokenEnv: "WEIXIN_TEST_TOKEN",
		APIBase:  server.URL,
	}, "chat-1", "hello weixin")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if result.MessageID != "wx-msg-1" {
		t.Fatalf("message id = %q, want wx-msg-1", result.MessageID)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q, want Bearer token-1", gotAuth)
	}
	msg, ok := gotPayload["msg"].(map[string]any)
	if !ok {
		t.Fatalf("payload msg = %#v, want object", gotPayload["msg"])
	}
	if msg["to_user_id"] != "chat-1" || msg["message_type"] != float64(weixinMsgTypeBot) || msg["message_state"] != float64(weixinMsgStateDone) {
		t.Fatalf("msg metadata = %#v", msg)
	}
	items, ok := msg["item_list"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("item_list = %#v, want one text item", msg["item_list"])
	}
	item, ok := items[0].(map[string]any)
	if !ok || item["type"] != float64(weixinItemText) {
		t.Fatalf("item = %#v, want text item", items[0])
	}
	textItem, ok := item["text_item"].(map[string]any)
	if !ok || textItem["text"] != "hello weixin" {
		t.Fatalf("text item = %#v, want hello weixin", item["text_item"])
	}
}

func isolateWeixinUserConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
}

func TestAccountOwnedPathsConfineUnsafeAccountIDs(t *testing.T) {
	isolateWeixinUserConfig(t)
	root := config.MemoryUserDir()
	wantDir := filepath.Clean(weixinAccountDir(root))
	unsafe := []string{"../escape", `..\\escape`, `C:\\escape`, "CON", "lpt9.log", "CLOCK$", "trailing. "}
	for _, accountID := range unsafe {
		t.Run(accountID, func(t *testing.T) {
			stem := weixinAccountFileStem(accountID)
			if !strings.HasPrefix(stem, "account-") || strings.Contains(stem, accountID) {
				t.Fatalf("unsafe account stem = %q", stem)
			}
			a := New(config.WeixinBotConfig{AccountID: accountID}, testWeixinLogger()).(*adapter)
			for _, path := range []string{savedAccountPath(accountID), a.tokenStorePath(), a.pollStatePath()} {
				if got := filepath.Clean(filepath.Dir(path)); got != wantDir {
					t.Fatalf("path %q escaped account dir: got %q want %q", path, got, wantDir)
				}
			}
		})
	}
	if got := weixinAccountFileStem("bot-account_1.2"); got != "bot-account_1.2" {
		t.Fatalf("safe account stem = %q", got)
	}
	if got := savedAccountPath("   "); got != "" {
		t.Fatalf("blank account path = %q, want empty", got)
	}
}

func TestLogPollHealthThrottlesEmptyPolls(t *testing.T) {
	a := &adapter{logger: slog.Default().With("platform", "weixin")}
	a.logPollHealth(ilinkResponse{})
	first := a.lastPollLog
	if first.IsZero() {
		t.Fatal("first empty poll should update heartbeat timestamp")
	}
	a.logPollHealth(ilinkResponse{})
	if !a.lastPollLog.Equal(first) {
		t.Fatalf("second empty poll updated heartbeat timestamp: got %v want %v", a.lastPollLog, first)
	}
	stale := time.Now().Add(-6 * time.Minute)
	a.lastPollLog = stale
	a.logPollHealth(ilinkResponse{})
	if !a.lastPollLog.After(stale) {
		t.Fatalf("stale empty poll did not refresh heartbeat timestamp: got %v after %v", a.lastPollLog, stale)
	}
}

func TestLogPollHealthLogsNonEmptyPolls(t *testing.T) {
	a := &adapter{logger: slog.Default().With("platform", "weixin")}
	a.logPollHealth(ilinkResponse{Msgs: []ilinkMessage{{MessageID: "msg-1"}}})
	if a.lastPollLog.IsZero() {
		t.Fatal("non-empty poll should update heartbeat timestamp")
	}
}

func TestGetUpdatesAcceptsNumericIlinkMessageID(t *testing.T) {
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != getUpdatesPath {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":             0,
			"errcode":         0,
			"get_updates_buf": "cursor-1",
			"msgs": []map[string]any{
				{
					"message_id":   123456789,
					"from_user_id": "wx-user-1",
					"to_user_id":   "bot-account",
					"msg_type":     1,
					"item_list": []map[string]any{
						{"type": weixinItemText, "text_item": map[string]string{"text": "hello"}},
					},
				},
			},
		})
	}))
	defer server.Close()

	a := &adapter{
		cfg: config.WeixinBotConfig{
			TokenEnv:  "WEIXIN_TEST_TOKEN",
			APIBase:   server.URL,
			AccountID: "bot-account",
		},
		logger:        slog.Default().With("platform", "weixin"),
		msgCh:         make(chan bot.InboundMessage, 1),
		contextTokens: make(map[string]string),
	}
	result, err := a.getUpdates(context.Background())
	if err != nil {
		t.Fatalf("getUpdates: %v", err)
	}
	if len(result.Msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(result.Msgs))
	}
	msg, ok := a.inboundFromIlinkMessage(result.Msgs[0])
	if !ok || msg.MessageID != "123456789" || msg.UserID != "wx-user-1" || msg.Text != "hello" {
		t.Fatalf("message = %+v ok=%v, want numeric id converted and text preserved", msg, ok)
	}
}

func TestPollingCommitsRecoveryBufferOnlyAfterDurableSettlement(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	requested := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Buffer string `json:"get_updates_buf"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode getupdates request: %v", err)
			return
		}
		requested <- body.Buffer
		if body.Buffer == "" {
			writeWeixinPollResponse(t, w, "cursor-1", "wx-msg-1")
			return
		}
		writeWeixinPollResponse(t, w, body.Buffer, "")
	}))
	defer server.Close()

	a := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", APIBase: server.URL, AccountID: "durable"}, testWeixinLogger()).(*adapter)
	a.startupDelay = time.Millisecond
	a.retryDelay = time.Millisecond
	a.idleDelay = time.Hour
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	if got := awaitWeixinCursor(t, requested); got != "" {
		t.Fatalf("first get_updates_buf = %q, want empty", got)
	}
	msg := awaitWeixinMessage(t, a.Messages())
	if msg.MessageID != "wx-msg-1" || msg.Text != "hello" {
		t.Fatalf("inbound = %+v", msg)
	}
	select {
	case got := <-requested:
		t.Fatalf("poll advanced before durable settlement: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := os.Stat(a.pollStatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("poll state existed before settlement: %v", err)
	}

	a.SettleInbound(msg.MessageID, true)
	if got := awaitWeixinCursor(t, requested); got != "cursor-1" {
		t.Fatalf("committed get_updates_buf = %q, want cursor-1", got)
	}
	data, err := os.ReadFile(a.pollStatePath())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"hello", "wx-msg-1", "wx-user-1", "token-1"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("poll state leaked %q: %s", forbidden, data)
		}
	}
	var state weixinPollState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	if state.Version != weixinPollStateVersion || state.SyncBuf != "cursor-1" {
		t.Fatalf("poll state = %+v", state)
	}
}

func TestFailedSettlementReplaysFromPreviouslyCommittedBuffer(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	requested := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Buffer string `json:"get_updates_buf"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requested <- body.Buffer
		if body.Buffer == "" {
			writeWeixinPollResponse(t, w, "cursor-1", "wx-msg-retry")
			return
		}
		writeWeixinPollResponse(t, w, body.Buffer, "")
	}))
	defer server.Close()

	a := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", APIBase: server.URL, AccountID: "retry"}, testWeixinLogger()).(*adapter)
	a.startupDelay = time.Millisecond
	a.retryDelay = time.Millisecond
	a.idleDelay = time.Hour
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	if got := awaitWeixinCursor(t, requested); got != "" {
		t.Fatalf("first cursor = %q", got)
	}
	first := awaitWeixinMessage(t, a.Messages())
	a.SettleInbound(first.MessageID, false)
	if got := awaitWeixinCursor(t, requested); got != "" {
		t.Fatalf("failed delivery advanced cursor to %q", got)
	}
	second := awaitWeixinMessage(t, a.Messages())
	if second.MessageID != first.MessageID {
		t.Fatalf("replayed message = %q, want %q", second.MessageID, first.MessageID)
	}
	a.SettleInbound(second.MessageID, true)
	if got := awaitWeixinCursor(t, requested); got != "cursor-1" {
		t.Fatalf("delivered retry cursor = %q, want cursor-1", got)
	}
}

func TestPollStateWriteFailureKeepsBatchReplayable(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	requested := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Buffer string `json:"get_updates_buf"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requested <- body.Buffer
		writeWeixinPollResponse(t, w, "cursor-1", "wx-msg-disk")
	}))
	defer server.Close()

	a := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", APIBase: server.URL, AccountID: "disk"}, testWeixinLogger()).(*adapter)
	a.startupDelay = time.Millisecond
	a.retryDelay = time.Millisecond
	a.writeState = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	if err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()

	if got := awaitWeixinCursor(t, requested); got != "" {
		t.Fatalf("first cursor = %q", got)
	}
	first := awaitWeixinMessage(t, a.Messages())
	a.SettleInbound(first.MessageID, true)
	if got := awaitWeixinCursor(t, requested); got != "" {
		t.Fatalf("state write failure advanced cursor to %q", got)
	}
	second := awaitWeixinMessage(t, a.Messages())
	if second.MessageID != first.MessageID {
		t.Fatalf("replayed message = %q, want %q", second.MessageID, first.MessageID)
	}
}

func TestStartFailsClosedOnCorruptPollState(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	a := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", AccountID: "corrupt"}, testWeixinLogger()).(*adapter)
	if err := os.MkdirAll(filepath.Dir(a.pollStatePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a.pollStatePath(), []byte(`{"version":1,"sync_buf":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "decode weixin poll state") {
		t.Fatalf("Start error = %v, want corrupt poll-state failure", err)
	}
}

func TestStartRejectsConcurrentRunAndAllowsRestartAfterStop(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	a := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", AccountID: "lifecycle"}, testWeixinLogger()).(*adapter)
	a.startupDelay = time.Hour
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

func TestRestartLoadsCommittedRecoveryBuffer(t *testing.T) {
	isolateWeixinUserConfig(t)
	t.Setenv("WEIXIN_TEST_TOKEN", "token-1")
	requested := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Buffer string `json:"get_updates_buf"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requested <- body.Buffer
		writeWeixinPollResponse(t, w, body.Buffer, "")
	}))
	defer server.Close()

	seed := New(config.WeixinBotConfig{TokenEnv: "WEIXIN_TEST_TOKEN", APIBase: server.URL, AccountID: "restart"}, testWeixinLogger()).(*adapter)
	if err := seed.persistPollState(weixinPollState{SyncBuf: "cursor-restored", LastUpdateID: 44}); err != nil {
		t.Fatal(err)
	}
	restarted := New(seed.cfg, testWeixinLogger()).(*adapter)
	restarted.startupDelay = time.Millisecond
	restarted.idleDelay = time.Hour
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer restarted.Stop()
	if got := awaitWeixinCursor(t, requested); got != "cursor-restored" {
		t.Fatalf("restart cursor = %q, want cursor-restored", got)
	}
	restarted.mu.Lock()
	lastUpdateID := restarted.lastUpdateID
	restarted.mu.Unlock()
	if lastUpdateID != 44 {
		t.Fatalf("restart last update id = %d, want 44", lastUpdateID)
	}
}

func TestPollBatchBackpressuresInsteadOfDroppingEnvelope(t *testing.T) {
	isolateWeixinUserConfig(t)
	a := New(config.WeixinBotConfig{AccountID: "backpressure"}, testWeixinLogger()).(*adapter)
	a.msgCh = make(chan bot.InboundMessage, 1)
	a.pending = make(map[string]pollSettlementState)
	a.msgCh <- bot.InboundMessage{MessageID: "first"}
	result := ilinkResponse{
		GetUpdatesBuf: "cursor-1",
		Msgs: []ilinkMessage{{
			MessageID:  "second",
			FromUserID: "wx-user-1",
			ToUserID:   "bot-account",
			ItemList: []struct {
				Type     int `json:"type"`
				TextItem struct {
					Text string `json:"text"`
				} `json:"text_item"`
			}{{Type: weixinItemText, TextItem: struct {
				Text string `json:"text"`
			}{Text: "hello"}}},
		}},
	}
	done := make(chan error, 1)
	go func() { done <- a.publishPollBatch(context.Background(), result) }()
	select {
	case err := <-done:
		t.Fatalf("publish returned while queue was full: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if got := (<-a.msgCh).MessageID; got != "first" {
		t.Fatalf("first queued message = %q", got)
	}
	second := awaitWeixinMessage(t, a.msgCh)
	if second.MessageID != "second" {
		t.Fatalf("second queued message = %q", second.MessageID)
	}
	a.SettleInbound(second.MessageID, true)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("settled poll batch did not finish")
	}
}

func writeWeixinPollResponse(t *testing.T, w http.ResponseWriter, cursor, messageID string) {
	t.Helper()
	response := map[string]any{
		"ret":             0,
		"errcode":         0,
		"get_updates_buf": cursor,
	}
	if messageID != "" {
		response["msgs"] = []map[string]any{{
			"message_id":   messageID,
			"from_user_id": "wx-user-1",
			"to_user_id":   "bot-account",
			"item_list": []map[string]any{{
				"type":      weixinItemText,
				"text_item": map[string]string{"text": "hello"},
			}},
		}}
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Errorf("encode poll response: %v", err)
	}
}

func awaitWeixinCursor(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for weixin poll request")
		return ""
	}
}

func awaitWeixinMessage(t *testing.T, ch <-chan bot.InboundMessage) bot.InboundMessage {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for weixin inbound message")
		return bot.InboundMessage{}
	}
}

func testWeixinLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
