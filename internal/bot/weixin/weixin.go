// Package weixin 实现微信 iLink Bot 适配器。
// 参考 Hermes 项目的 weixin adapter 机制：
// - getupdates 长轮询
// - sendmessage / sendtyping
// - context_token 持久化
// - 二维码登录
// - DM allowlist（默认只对 allowlist 内用户开放 DM；群聊默认关闭）
package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/bot"
	"reames-agent/internal/config"
	"reames-agent/internal/fileutil"
)

const (
	defaultWeixinAPI = "https://ilinkai.weixin.qq.com"
	getUpdatesPath   = "/ilink/bot/getupdates"
	sendMessagePath  = "/ilink/bot/sendmessage"
	sendTypingPath   = "/ilink/bot/sendtyping"
	uploadMediaPath  = "/ilink/bot/getuploadurl"
	getBotQRPath     = "/ilink/bot/get_bot_qrcode"
	getQRStatusPath  = "/ilink/bot/get_qrcode_status"

	ilinkAppID          = "bot"
	ilinkClientVersion  = (2 << 16) | (2 << 8)
	ilinkChannelVersion = "2.2.0"
	weixinItemText      = 1
	weixinMsgTypeBot    = 2
	weixinMsgStateDone  = 2

	weixinHTTPTimeout = 30 * time.Second

	weixinPollStateVersion = 1
	weixinStartupDelay     = 2 * time.Second
	weixinRetryDelay       = 5 * time.Second
	weixinIdleDelay        = 500 * time.Millisecond
	weixinStopTimeout      = 5 * time.Second
)

var weixinHTTPClient = &http.Client{Timeout: weixinHTTPTimeout}

// ilinkUpdate 微信 iLink getupdates 返回的更新消息。
type ilinkUpdate struct {
	UpdateID   int64  `json:"update_id"`
	UpdateType string `json:"update_type"`
	Message    struct {
		MessageID ilinkString `json:"message_id"`
		ChatID    string      `json:"chat_id"`
		ChatType  string      `json:"chat_type"`
		From      struct {
			UserID   string `json:"user_id"`
			UserName string `json:"user_name"`
		} `json:"from"`
		Text      string `json:"text"`
		Timestamp int64  `json:"timestamp"`
	} `json:"message"`
}

type ilinkMessage struct {
	MessageID    ilinkString `json:"message_id"`
	FromUserID   string      `json:"from_user_id"`
	ToUserID     string      `json:"to_user_id"`
	RoomID       string      `json:"room_id"`
	ChatRoomID   string      `json:"chat_room_id"`
	ContextToken string      `json:"context_token"`
	MsgType      int         `json:"msg_type"`
	ItemList     []struct {
		Type     int `json:"type"`
		TextItem struct {
			Text string `json:"text"`
		} `json:"text_item"`
	} `json:"item_list"`
}

type ilinkResponse struct {
	Ret                  int            `json:"ret"`
	Errcode              int            `json:"errcode"`
	Errmsg               string         `json:"errmsg"`
	Updates              []ilinkUpdate  `json:"updates"`
	Msgs                 []ilinkMessage `json:"msgs"`
	HasMore              bool           `json:"has_more"`
	ContextToken         string         `json:"context_token"`
	GetUpdatesBuf        string         `json:"get_updates_buf"`
	LongpollingTimeoutMs int            `json:"longpolling_timeout_ms"`
}

type ilinkString string

func (s *ilinkString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = ilinkString(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err == nil {
		*s = ilinkString(num.String())
		return nil
	}
	return fmt.Errorf("ilink string: expected string or number, got %s", string(data))
}

type pollSettlementState uint8

const (
	pollWaiting pollSettlementState = iota
	pollDelivered
	pollFailed
)

type weixinPollState struct {
	Version      int    `json:"version"`
	SyncBuf      string `json:"sync_buf,omitempty"`
	LastUpdateID int64  `json:"last_update_id,omitempty"`
}

// adapter 微信适配器实现。
type adapter struct {
	cfg    config.WeixinBotConfig
	logger *slog.Logger
	msgCh  chan bot.InboundMessage
	cancel context.CancelFunc
	done   chan struct{}

	lifecycleMu sync.Mutex

	mu             sync.Mutex
	contextTokens  map[string]string
	syncBuf        string
	lastUpdateID   int64
	pending        map[string]pollSettlementState
	pendingOrder   []string
	pendingNext    weixinPollState
	settlementWake chan struct{}
	pollReadyOnce  sync.Once
	lastPollLog    time.Time

	startupDelay time.Duration
	retryDelay   time.Duration
	idleDelay    time.Duration
	writeState   func(string, []byte, os.FileMode) error
}

// New 创建微信 Bot 适配器。
func New(cfg config.WeixinBotConfig, logger *slog.Logger) bot.Adapter {
	return &adapter{
		cfg:            cfg,
		logger:         logger.With("platform", "weixin"),
		contextTokens:  make(map[string]string),
		settlementWake: make(chan struct{}, 1),
		startupDelay:   weixinStartupDelay,
		retryDelay:     weixinRetryDelay,
		idleDelay:      weixinIdleDelay,
	}
}

func (a *adapter) Platform() bot.Platform { return bot.PlatformWeixin }
func (a *adapter) Name() string           { return "weixin" }

func (a *adapter) Start(ctx context.Context) error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.done != nil {
		select {
		case <-a.done:
			a.cancel = nil
			a.done = nil
		default:
			return fmt.Errorf("weixin adapter is already started")
		}
	}
	if a.token() == "" {
		return a.tokenMissingError()
	}
	if err := a.loadPollState(); err != nil {
		return err
	}
	a.loadContextTokens()
	ctx, a.cancel = context.WithCancel(ctx)
	done := make(chan struct{})
	a.done = done
	a.mu.Lock()
	a.msgCh = make(chan bot.InboundMessage, 64)
	a.pending = make(map[string]pollSettlementState)
	a.pendingOrder = nil
	a.pendingNext = weixinPollState{}
	if a.settlementWake == nil {
		a.settlementWake = make(chan struct{}, 1)
	}
	a.mu.Unlock()

	a.logger.Info("weixin polling started", "account", logHash(a.accountID()), "api_base", a.apiBase())
	go func() {
		defer close(done)
		a.pollLoop(ctx)
	}()
	return nil
}

func (a *adapter) Stop() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	cancel := a.cancel
	done := a.done
	if cancel == nil {
		return nil
	}
	cancel()
	if done == nil {
		a.cancel = nil
		return nil
	}
	timer := time.NewTimer(weixinStopTimeout)
	defer timer.Stop()
	select {
	case <-done:
		if a.done == done {
			a.cancel = nil
			a.done = nil
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("weixin stop timed out waiting for polling cancellation")
	}
}

func (a *adapter) Send(ctx context.Context, msg bot.OutboundMessage) (bot.SendResult, error) {
	return a.sendMessage(ctx, msg)
}

func (a *adapter) SendTyping(ctx context.Context, chatID string) error {
	return a.sendTyping(ctx, chatID)
}

func (a *adapter) Messages() <-chan bot.InboundMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.msgCh
}

// SettleInbound advances the opaque iLink get_updates_buf only after the
// shared Gateway ledger has durably committed final outbound delivery. A
// failed settlement keeps the previously committed buffer, so the same remote
// batch remains eligible for replay instead of disappearing after receipt.
func (a *adapter) SettleInbound(messageID string, delivered bool) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return
	}
	a.mu.Lock()
	state, ok := a.pending[messageID]
	if ok && state == pollWaiting {
		if delivered {
			a.pending[messageID] = pollDelivered
		} else {
			a.pending[messageID] = pollFailed
		}
	}
	wake := a.settlementWake
	a.mu.Unlock()
	if ok && wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

var _ bot.DeliverySettlementAdapter = (*adapter)(nil)

// SendText sends one plain text message to a saved Weixin iLink conversation.
// It is used by desktop settings as an actual connection test.
func SendText(ctx context.Context, cfg config.WeixinBotConfig, chatID, text string) (bot.SendResult, error) {
	a := &adapter{cfg: cfg, logger: slog.Default().With("platform", "weixin"), contextTokens: make(map[string]string)}
	return a.sendMessage(ctx, bot.OutboundMessage{ChatID: chatID, Text: text})
}

// token 从环境变量获取微信 token。
func (a *adapter) token() string {
	if token := os.Getenv(a.cfg.TokenEnv); token != "" {
		return token
	}
	account, _ := loadSavedAccount(a.accountID())
	if account.Token != "" {
		return account.Token
	}
	if a.cfg.AccountID == "" {
		account, _ = loadAnySavedAccount()
		return account.Token
	}
	return ""
}

func (a *adapter) tokenMissingError() error {
	if strings.TrimSpace(a.cfg.TokenEnv) == "" {
		return fmt.Errorf("weixin token is not configured and no saved weixin account is available")
	}
	return fmt.Errorf("%s not set and no saved weixin account is available", a.cfg.TokenEnv)
}

// apiBase 返回 API base URL。
func (a *adapter) apiBase() string {
	if a.cfg.APIBase != "" {
		return a.cfg.APIBase
	}
	account, _ := loadSavedAccount(a.accountID())
	if account.BaseURL != "" {
		return strings.TrimRight(account.BaseURL, "/")
	}
	return defaultWeixinAPI
}

func (a *adapter) accountID() string {
	if a.cfg.AccountID != "" {
		return a.cfg.AccountID
	}
	return "default"
}

func (a *adapter) contextToken(chatID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.contextTokens[chatID]
}

func (a *adapter) setContextToken(chatID, token string) {
	a.mu.Lock()
	if token == "" {
		delete(a.contextTokens, chatID)
	} else {
		a.contextTokens[chatID] = token
	}
	a.mu.Unlock()
	a.saveContextTokens()
}

func (a *adapter) tokenStorePath() string {
	root := config.MemoryUserDir()
	stem := weixinAccountFileStem(a.accountID())
	if root == "" || stem == "" {
		return ""
	}
	return filepath.Join(weixinAccountDir(root), stem+".context-tokens.json")
}

func (a *adapter) loadContextTokens() {
	path := a.tokenStorePath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var tokens map[string]string
	if err := json.Unmarshal(data, &tokens); err != nil {
		a.logger.Warn("failed to load weixin context tokens", "err", err)
		return
	}
	a.mu.Lock()
	a.contextTokens = tokens
	a.mu.Unlock()
}

func (a *adapter) saveContextTokens() {
	path := a.tokenStorePath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		a.logger.Warn("failed to create weixin token dir", "err", err)
		return
	}
	a.mu.Lock()
	data, err := json.MarshalIndent(a.contextTokens, "", "  ")
	a.mu.Unlock()
	if err != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		a.logger.Warn("failed to save weixin context tokens", "err", err)
	}
}

func (a *adapter) pollStatePath() string {
	root := config.MemoryUserDir()
	stem := weixinAccountFileStem(a.accountID())
	if root == "" || stem == "" {
		return ""
	}
	return filepath.Join(weixinAccountDir(root), stem+".poll-state.json")
}

func (a *adapter) loadPollState() error {
	path := a.pollStatePath()
	if path == "" {
		return fmt.Errorf("weixin poll state path is unavailable")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		a.mu.Lock()
		a.syncBuf = ""
		a.lastUpdateID = 0
		a.mu.Unlock()
		return nil
	}
	if err != nil {
		return fmt.Errorf("read weixin poll state: %w", err)
	}
	var state weixinPollState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode weixin poll state: %w", err)
	}
	if state.Version != weixinPollStateVersion || state.LastUpdateID < 0 {
		return fmt.Errorf("weixin poll state is invalid or unsupported")
	}
	a.mu.Lock()
	a.syncBuf = state.SyncBuf
	a.lastUpdateID = state.LastUpdateID
	a.mu.Unlock()
	return nil
}

func (a *adapter) persistPollState(state weixinPollState) error {
	path := a.pollStatePath()
	if path == "" {
		return fmt.Errorf("weixin poll state path is unavailable")
	}
	state.Version = weixinPollStateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode weixin poll state: %w", err)
	}
	data = append(data, '\n')
	write := a.writeState
	if write == nil {
		write = fileutil.AtomicWriteFile
	}
	if err := write(path, data, 0o600); err != nil {
		return fmt.Errorf("persist weixin poll state: %w", err)
	}
	return nil
}

func ilinkGET(ctx context.Context, baseURL, endpoint string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(baseURL, "/")+"/"+strings.TrimLeft(endpoint, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-Id", ilinkAppID)
	req.Header.Set("iLink-App-ClientVersion", fmt.Sprintf("%d", ilinkClientVersion))
	resp, err := weixinHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		if len(data) > 200 {
			data = data[:200]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// pollLoop 长轮询获取更新。
func (a *adapter) pollLoop(ctx context.Context) {
	// 启动时短暂等待让登录完成
	startupDelay := a.startupDelay
	if startupDelay <= 0 {
		startupDelay = weixinStartupDelay
	}
	if !bot.SleepCtx(ctx, startupDelay) {
		return
	}
	retryDelay := a.retryDelay
	if retryDelay <= 0 {
		retryDelay = weixinRetryDelay
	}
	idleDelay := a.idleDelay
	if idleDelay <= 0 {
		idleDelay = weixinIdleDelay
	}

	for {
		if ctx.Err() != nil {
			return
		}

		result, err := a.getUpdates(ctx)
		if err != nil {
			a.logger.Error("getupdates failed", "err", err)
			if !bot.SleepCtx(ctx, retryDelay) {
				return
			}
			continue
		}
		if err := a.publishPollBatch(ctx, result); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("weixin poll batch failed", "err", err)
			if !bot.SleepCtx(ctx, retryDelay) {
				return
			}
			continue
		}

		// 没有更新时短暂等待
		if !result.HasMore && len(result.Updates) == 0 && len(result.Msgs) == 0 {
			if !bot.SleepCtx(ctx, idleDelay) {
				return
			}
		}
	}
}

// getUpdates 调用微信 iLink getupdates API。
func (a *adapter) getUpdates(ctx context.Context) (ilinkResponse, error) {
	tok := a.token()
	if tok == "" {
		return ilinkResponse{}, a.tokenMissingError()
	}

	url := a.apiBase() + getUpdatesPath

	a.mu.Lock()
	payload := map[string]interface{}{
		"get_updates_buf": a.syncBuf,
		"base_info": map[string]string{
			"channel_version": ilinkChannelVersion,
		},
	}
	a.mu.Unlock()

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return ilinkResponse{}, err
	}
	setIlinkHeaders(req, tok, body)

	resp, err := weixinHTTPClient.Do(req)
	if err != nil {
		return ilinkResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return ilinkResponse{}, fmt.Errorf("weixin getupdates HTTP %d", resp.StatusCode)
	}

	var result ilinkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ilinkResponse{}, err
	}
	if result.Ret != 0 || result.Errcode != 0 {
		return ilinkResponse{}, fmt.Errorf("getupdates error ret=%d errcode=%d: %s", result.Ret, result.Errcode, result.Errmsg)
	}
	a.pollReadyOnce.Do(func() {
		a.logger.Info("weixin getupdates ready", "account", logHash(a.accountID()), "api_base", a.apiBase())
	})
	a.logPollHealth(result)
	if result.GetUpdatesBuf == "" && (result.HasMore || len(result.Updates) > 0 || len(result.Msgs) > 0) {
		return ilinkResponse{}, fmt.Errorf("weixin getupdates returned messages without a recovery buffer")
	}
	return result, nil
}

func (a *adapter) logPollHealth(result ilinkResponse) {
	shouldLog := len(result.Updates) > 0 || len(result.Msgs) > 0
	a.mu.Lock()
	if !shouldLog && time.Since(a.lastPollLog) >= 5*time.Minute {
		shouldLog = true
	}
	if shouldLog {
		a.lastPollLog = time.Now()
	}
	a.mu.Unlock()
	if !shouldLog {
		return
	}
	a.logger.Info("weixin getupdates heartbeat",
		"updates", len(result.Updates),
		"msgs", len(result.Msgs),
		"has_more", result.HasMore,
		"timeout_ms", result.LongpollingTimeoutMs)
}

func (a *adapter) publishPollBatch(ctx context.Context, result ilinkResponse) error {
	a.mu.Lock()
	if len(a.pendingOrder) != 0 {
		a.mu.Unlock()
		return fmt.Errorf("weixin settlement batch overlap")
	}
	current := weixinPollState{
		Version:      weixinPollStateVersion,
		SyncBuf:      a.syncBuf,
		LastUpdateID: a.lastUpdateID,
	}
	a.mu.Unlock()

	sort.SliceStable(result.Updates, func(i, j int) bool {
		return result.Updates[i].UpdateID < result.Updates[j].UpdateID
	})
	next := current
	if result.GetUpdatesBuf != "" {
		next.SyncBuf = result.GetUpdatesBuf
	}
	for _, upd := range result.Updates {
		if upd.UpdateID > next.LastUpdateID {
			next.LastUpdateID = upd.UpdateID
		}
	}

	seen := make(map[string]bool, len(result.Updates)+len(result.Msgs))
	messages := make([]bot.InboundMessage, 0, len(result.Updates)+len(result.Msgs))
	for _, upd := range result.Updates {
		msg, ok := a.inboundFromUpdate(upd)
		if !ok || seen[msg.MessageID] {
			continue
		}
		seen[msg.MessageID] = true
		messages = append(messages, msg)
	}
	for _, raw := range result.Msgs {
		msg, ok := a.inboundFromIlinkMessage(raw)
		if !ok || seen[msg.MessageID] {
			continue
		}
		seen[msg.MessageID] = true
		messages = append(messages, msg)
	}

	a.mu.Lock()
	a.pendingNext = next
	for _, msg := range messages {
		a.pending[msg.MessageID] = pollWaiting
		a.pendingOrder = append(a.pendingOrder, msg.MessageID)
	}
	a.mu.Unlock()

	for _, msg := range messages {
		select {
		case <-ctx.Done():
			a.clearPendingBatch()
			return ctx.Err()
		case a.msgCh <- msg:
			a.logger.Info("weixin inbound queued", "chat_type", msg.ChatType, "chat", logHash(msg.ChatID), "user", logHash(msg.UserID), "message", logHash(msg.MessageID), "text_chars", len([]rune(msg.Text)))
		}
	}

	delivered, err := a.waitForBatchSettlement(ctx)
	if err != nil {
		return err
	}
	if !delivered {
		return fmt.Errorf("weixin poll batch delivery failed; recovery buffer was not advanced")
	}
	return nil
}

func (a *adapter) waitForBatchSettlement(ctx context.Context) (delivered bool, err error) {
	for {
		a.mu.Lock()
		allSettled := true
		delivered = true
		for _, id := range a.pendingOrder {
			switch a.pending[id] {
			case pollWaiting:
				allSettled = false
			case pollFailed:
				delivered = false
			}
		}
		if allSettled {
			next := a.pendingNext
			current := weixinPollState{
				Version:      weixinPollStateVersion,
				SyncBuf:      a.syncBuf,
				LastUpdateID: a.lastUpdateID,
			}
			a.mu.Unlock()
			if delivered && next != current {
				if err := a.persistPollState(next); err != nil {
					a.clearPendingBatch()
					return false, err
				}
				a.mu.Lock()
				a.syncBuf = next.SyncBuf
				a.lastUpdateID = next.LastUpdateID
				a.mu.Unlock()
			}
			a.clearPendingBatch()
			return delivered, nil
		}
		wake := a.settlementWake
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			a.clearPendingBatch()
			return false, ctx.Err()
		case <-wake:
		}
	}
}

func (a *adapter) clearPendingBatch() {
	a.mu.Lock()
	for _, id := range a.pendingOrder {
		delete(a.pending, id)
	}
	a.pendingOrder = nil
	a.pendingNext = weixinPollState{}
	a.mu.Unlock()
}

// handleUpdate 处理单条微信更新消息。
func (a *adapter) inboundFromUpdate(upd ilinkUpdate) (bot.InboundMessage, bool) {
	if upd.UpdateType != "message" {
		a.logger.Info("weixin update ignored", "reason", "non_message", "update_type", upd.UpdateType)
		return bot.InboundMessage{}, false
	}

	m := upd.Message
	if strings.TrimSpace(string(m.MessageID)) == "" || strings.TrimSpace(m.From.UserID) == "" || strings.TrimSpace(m.Text) == "" {
		a.logger.Info("weixin update ignored", "reason", "missing_identity_or_text", "message", logHash(string(m.MessageID)))
		return bot.InboundMessage{}, false
	}
	chatType := bot.ChatDM
	if m.ChatType == "group" {
		chatType = bot.ChatGroup
	}

	return bot.InboundMessage{
		Platform:  bot.PlatformWeixin,
		ChatType:  chatType,
		ChatID:    m.ChatID,
		UserID:    m.From.UserID,
		UserName:  m.From.UserName,
		Text:      m.Text,
		MessageID: string(m.MessageID),
	}, true
}

func (a *adapter) inboundFromIlinkMessage(m ilinkMessage) (bot.InboundMessage, bool) {
	if m.FromUserID == "" || m.FromUserID == a.accountID() {
		a.logger.Info("weixin message ignored", "reason", "self_or_missing_sender", "from", logHash(m.FromUserID), "message", logHash(string(m.MessageID)))
		return bot.InboundMessage{}, false
	}
	text := extractIlinkText(m.ItemList)
	if text == "" {
		a.logger.Info("weixin message ignored", "reason", "empty_text", "from", logHash(m.FromUserID), "message", logHash(string(m.MessageID)))
		return bot.InboundMessage{}, false
	}
	chatType, chatID := guessIlinkChat(m, a.accountID())
	if chatID == "" {
		a.logger.Info("weixin message ignored", "reason", "missing_chat", "from", logHash(m.FromUserID), "message", logHash(string(m.MessageID)))
		return bot.InboundMessage{}, false
	}
	if strings.TrimSpace(string(m.MessageID)) == "" {
		a.logger.Info("weixin message ignored", "reason", "missing_message_id", "from", logHash(m.FromUserID))
		return bot.InboundMessage{}, false
	}
	if m.ContextToken != "" {
		a.setContextToken(chatID, m.ContextToken)
	}
	return bot.InboundMessage{
		Platform:  bot.PlatformWeixin,
		ChatType:  chatType,
		ChatID:    chatID,
		UserID:    m.FromUserID,
		UserName:  m.FromUserID,
		Text:      text,
		MessageID: string(m.MessageID),
	}, true
}

func logHash(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])[:12]
}

func extractIlinkText(items []struct {
	Type     int `json:"type"`
	TextItem struct {
		Text string `json:"text"`
	} `json:"text_item"`
}) string {
	var out []string
	for _, item := range items {
		if item.Type == weixinItemText && item.TextItem.Text != "" {
			out = append(out, item.TextItem.Text)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func guessIlinkChat(m ilinkMessage, accountID string) (bot.ChatType, string) {
	roomID := firstNonEmptyString(m.RoomID, m.ChatRoomID)
	if roomID != "" {
		return bot.ChatGroup, roomID
	}
	if m.ToUserID != "" && accountID != "" && m.ToUserID != accountID && m.MsgType == 1 {
		return bot.ChatGroup, m.ToUserID
	}
	return bot.ChatDM, m.FromUserID
}

func setIlinkHeaders(req *http.Request, token string, body []byte) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	req.Header.Set("iLink-App-Id", ilinkAppID)
	req.Header.Set("iLink-App-ClientVersion", fmt.Sprintf("%d", ilinkClientVersion))
}

func randomWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]))))
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// sendMessage 使用微信 iLink sendmessage API 发送消息。
func (a *adapter) sendMessage(ctx context.Context, msg bot.OutboundMessage) (bot.SendResult, error) {
	tok := a.token()
	if tok == "" {
		return bot.SendResult{}, a.tokenMissingError()
	}

	url := a.apiBase() + sendMessagePath

	payload := map[string]interface{}{
		"base_info": map[string]string{"channel_version": ilinkChannelVersion},
		"msg": map[string]interface{}{
			"from_user_id":  "",
			"to_user_id":    msg.ChatID,
			"client_id":     fmt.Sprintf("reamesAgent-%d", time.Now().UnixNano()),
			"message_type":  weixinMsgTypeBot,
			"message_state": weixinMsgStateDone,
			"item_list": []map[string]interface{}{
				{"type": weixinItemText, "text_item": map[string]string{"text": msg.Text}},
			},
		},
	}
	if contextToken := a.contextToken(msg.ChatID); contextToken != "" {
		if m, ok := payload["msg"].(map[string]interface{}); ok {
			m["context_token"] = contextToken
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return bot.SendResult{}, err
	}
	setIlinkHeaders(req, tok, body)

	resp, err := weixinHTTPClient.Do(req)
	if err != nil {
		return bot.SendResult{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Ret       int         `json:"ret"`
		Errcode   int         `json:"errcode"`
		Errmsg    string      `json:"errmsg"`
		MessageID ilinkString `json:"message_id"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err != nil {
		return bot.SendResult{}, err
	}
	if result.Ret != 0 || result.Errcode != 0 {
		if a.contextToken(msg.ChatID) != "" {
			a.setContextToken(msg.ChatID, "")
			return a.sendMessage(ctx, msg)
		}
		return bot.SendResult{}, fmt.Errorf("sendmessage error ret=%d errcode=%d: %s", result.Ret, result.Errcode, result.Errmsg)
	}

	return bot.SendResult{MessageID: string(result.MessageID)}, nil
}

// sendTyping 发送"正在输入"状态。
func (a *adapter) sendTyping(ctx context.Context, chatID string) error {
	tok := a.token()
	if tok == "" {
		return a.tokenMissingError()
	}

	url := a.apiBase() + sendTypingPath

	payload := map[string]interface{}{
		"base_info":     map[string]string{"channel_version": ilinkChannelVersion},
		"ilink_user_id": chatID,
		"status":        1,
	}
	if contextToken := a.contextToken(chatID); contextToken != "" {
		payload["context_token"] = contextToken
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	setIlinkHeaders(req, tok, body)

	resp, err := weixinHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Ret     int    `json:"ret"`
		Errcode int    `json:"errcode"`
		Errmsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if result.Ret != 0 || result.Errcode != 0 {
		return fmt.Errorf("sendtyping error ret=%d errcode=%d: %s", result.Ret, result.Errcode, result.Errmsg)
	}

	return nil
}
