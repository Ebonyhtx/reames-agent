// Package telegram implements the Telegram Bot API adapter used by the shared
// Reames Agent Gateway. It intentionally uses net/http instead of an SDK so the
// long-poll acknowledgement boundary stays under Gateway control.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/bot"
	"reames-agent/internal/config"
)

const (
	defaultAPIBase          = "https://api.telegram.org"
	defaultPollTimeout      = 10 * time.Second
	defaultRequestSlack     = 5 * time.Second
	defaultStopTimeout      = 5 * time.Second
	defaultRetryInitial     = time.Second
	defaultRetryMaximum     = 30 * time.Second
	telegramUpdateBatchSize = 100
	maxTelegramBodyBytes    = 64 << 10
)

type pendingState uint8

const (
	pendingWaiting pendingState = iota
	pendingDelivered
	pendingFailed
)

type pendingUpdate struct {
	id    int64
	state pendingState
}

// New creates a production Telegram adapter.
func New(cfg config.TelegramBotConfig, logger *slog.Logger) bot.Adapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &adapter{
		cfg:            cfg,
		logger:         logger.With("platform", "telegram"),
		client:         &http.Client{},
		pollTimeout:    defaultPollTimeout,
		requestSlack:   defaultRequestSlack,
		stopTimeout:    defaultStopTimeout,
		retryInitial:   defaultRetryInitial,
		retryMaximum:   defaultRetryMaximum,
		settlementWake: make(chan struct{}, 1),
	}
}

type adapter struct {
	cfg    config.TelegramBotConfig
	logger *slog.Logger
	client *http.Client

	lifecycleMu sync.Mutex

	pollTimeout  time.Duration
	requestSlack time.Duration
	stopTimeout  time.Duration
	retryInitial time.Duration
	retryMaximum time.Duration

	msgCh  chan bot.InboundMessage
	cancel context.CancelFunc
	done   chan struct{}

	mu             sync.Mutex
	token          string
	apiBase        string
	nextOffset     int64
	pending        map[int64]*pendingUpdate
	pendingOrder   []int64
	settlementWake chan struct{}
}

func (a *adapter) Platform() bot.Platform { return bot.PlatformTelegram }
func (a *adapter) Name() string           { return "telegram" }

func (a *adapter) Start(ctx context.Context) error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done != nil {
		select {
		case <-done:
			a.mu.Lock()
			a.cancel = nil
			a.done = nil
			a.mu.Unlock()
		default:
			return errors.New("telegram adapter is already started")
		}
	}
	token, err := telegramToken(a.cfg.TokenEnv)
	if err != nil {
		return err
	}
	base, err := normalizeAPIBase(a.cfg.APIBase)
	if err != nil {
		return err
	}
	startupTimeout := a.requestTimeout(5 * time.Second)
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	if _, err := a.getMe(startupCtx, base, token); err != nil {
		return err
	}

	runCtx, runCancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.token = token
	a.apiBase = base
	a.msgCh = make(chan bot.InboundMessage, 64)
	a.cancel = runCancel
	a.done = make(chan struct{})
	a.pending = make(map[int64]*pendingUpdate)
	a.pendingOrder = nil
	a.nextOffset = 0
	done = a.done
	a.mu.Unlock()

	go func() {
		defer close(done)
		a.pollLoop(runCtx)
	}()
	return nil
}

func (a *adapter) Stop() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	a.mu.Lock()
	cancel := a.cancel
	done := a.done
	stopTimeout := a.stopTimeout
	a.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	a.client.CloseIdleConnections()
	if done == nil {
		return nil
	}
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}
	timer := time.NewTimer(stopTimeout)
	defer timer.Stop()
	select {
	case <-done:
		a.mu.Lock()
		if a.done == done {
			a.cancel = nil
			a.done = nil
		}
		a.mu.Unlock()
		return nil
	case <-timer.C:
		return errors.New("telegram stop timed out waiting for polling request cancellation")
	}
}

func (a *adapter) Messages() <-chan bot.InboundMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.msgCh
}

// SettleInbound advances Telegram's getUpdates offset only after the shared
// Gateway delivery ledger has durably committed the outcome. Failed messages
// remain unconfirmed and are returned by Telegram again after the current batch
// settles; delivered duplicates are acknowledged without running another turn.
func (a *adapter) SettleInbound(messageID string, delivered bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(messageID), 10, 64)
	if err != nil {
		return
	}
	a.mu.Lock()
	record := a.pending[id]
	if record != nil {
		if delivered {
			record.state = pendingDelivered
		} else {
			record.state = pendingFailed
		}
	}
	a.mu.Unlock()
	if record != nil {
		select {
		case a.settlementWake <- struct{}{}:
		default:
		}
	}
}

func (a *adapter) Send(ctx context.Context, msg bot.OutboundMessage) (bot.SendResult, error) {
	chatID := strings.TrimSpace(msg.ChatID)
	text := strings.TrimSpace(msg.Text)
	if chatID == "" || text == "" {
		return bot.SendResult{}, errors.New("telegram send requires chat_id and text")
	}
	params := url.Values{"chat_id": {chatID}, "text": {text}}
	if reply := strings.TrimSpace(msg.ReplyToMsgID); reply != "" {
		if _, err := strconv.ParseInt(reply, 10, 64); err == nil {
			params.Set("reply_to_message_id", reply)
		}
	}
	var result telegramMessage
	if err := a.call(ctx, "sendMessage", params, &result); err != nil {
		return bot.SendResult{}, err
	}
	return bot.SendResult{MessageID: strconv.FormatInt(result.MessageID, 10)}, nil
}

func (a *adapter) SendTyping(ctx context.Context, chatID string) error {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	return a.call(ctx, "sendChatAction", url.Values{"chat_id": {chatID}, "action": {"typing"}}, nil)
}

// SendText performs an actual bounded Telegram send for Desktop diagnostics.
func SendText(ctx context.Context, cfg config.TelegramBotConfig, chatID, text string) (bot.SendResult, error) {
	a := New(cfg, slog.Default()).(*adapter)
	token, err := telegramToken(cfg.TokenEnv)
	if err != nil {
		return bot.SendResult{}, err
	}
	base, err := normalizeAPIBase(cfg.APIBase)
	if err != nil {
		return bot.SendResult{}, err
	}
	a.token = token
	a.apiBase = base
	return a.Send(ctx, bot.OutboundMessage{ChatID: chatID, Text: text})
}

func (a *adapter) pollLoop(ctx context.Context) {
	delay := a.retryInitial
	if delay <= 0 {
		delay = defaultRetryInitial
	}
	maxDelay := a.retryMaximum
	if maxDelay < delay {
		maxDelay = delay
	}
	for ctx.Err() == nil {
		a.mu.Lock()
		offset := a.nextOffset
		a.mu.Unlock()
		updates, err := a.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Warn("telegram polling request failed", "err", err)
			if !bot.SleepCtx(ctx, delay) {
				return
			}
			delay *= 2
			if delay <= 0 || delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		if len(updates) == 0 {
			delay = a.retryInitial
			if delay <= 0 {
				delay = defaultRetryInitial
			}
			continue
		}
		if !a.publishBatch(ctx, updates) {
			return
		}
		batchFailed, ok := a.waitForBatchSettlement(ctx)
		if !ok {
			return
		}
		if batchFailed {
			if !bot.SleepCtx(ctx, delay) {
				return
			}
			delay *= 2
			if delay <= 0 || delay > maxDelay {
				delay = maxDelay
			}
			continue
		}
		delay = a.retryInitial
		if delay <= 0 {
			delay = defaultRetryInitial
		}
	}
}

func (a *adapter) publishBatch(ctx context.Context, updates []telegramUpdate) bool {
	sort.Slice(updates, func(i, j int) bool { return updates[i].UpdateID < updates[j].UpdateID })
	var inbound []bot.InboundMessage
	a.mu.Lock()
	if len(a.pendingOrder) != 0 {
		a.mu.Unlock()
		a.logger.Error("telegram internal settlement batch overlap")
		return false
	}
	seen := make(map[int64]bool, len(updates))
	for _, update := range updates {
		if update.UpdateID < a.nextOffset || seen[update.UpdateID] {
			continue
		}
		seen[update.UpdateID] = true
		record := &pendingUpdate{id: update.UpdateID, state: pendingDelivered}
		if msg, ok := inboundMessage(update); ok {
			record.state = pendingWaiting
			inbound = append(inbound, msg)
		}
		a.pending[update.UpdateID] = record
		a.pendingOrder = append(a.pendingOrder, update.UpdateID)
	}
	a.mu.Unlock()
	for _, msg := range inbound {
		select {
		case <-ctx.Done():
			return false
		case a.msgCh <- msg:
		}
	}
	return true
}

func (a *adapter) waitForBatchSettlement(ctx context.Context) (batchFailed bool, ok bool) {
	for {
		a.mu.Lock()
		allSettled := true
		batchFailed = false
		for _, id := range a.pendingOrder {
			record := a.pending[id]
			if record != nil && record.state == pendingWaiting {
				allSettled = false
				break
			}
			if record != nil && record.state == pendingFailed {
				batchFailed = true
			}
		}
		if allSettled {
			next := a.nextOffset
			for _, id := range a.pendingOrder {
				record := a.pending[id]
				if record == nil || record.state == pendingFailed {
					break
				}
				if id >= next {
					next = id + 1
				}
			}
			for _, id := range a.pendingOrder {
				delete(a.pending, id)
			}
			a.pendingOrder = nil
			a.nextOffset = next
			a.mu.Unlock()
			return batchFailed, true
		}
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return false, false
		case <-a.settlementWake:
		}
	}
}

func (a *adapter) getMe(ctx context.Context, base, token string) (telegramUser, error) {
	var user telegramUser
	if err := a.callWithCredential(ctx, base, token, "getMe", nil, &user); err != nil {
		return telegramUser{}, err
	}
	if user.ID == 0 || !user.IsBot {
		return telegramUser{}, errors.New("telegram getMe returned an invalid bot identity")
	}
	return user, nil
}

func (a *adapter) getUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	timeout := a.pollTimeout
	if timeout <= 0 {
		timeout = defaultPollTimeout
	}
	params := url.Values{
		"offset":          {strconv.FormatInt(offset, 10)},
		"timeout":         {strconv.Itoa(max(1, int(timeout/time.Second)))},
		"limit":           {strconv.Itoa(telegramUpdateBatchSize)},
		"allowed_updates": {`["message"]`},
	}
	var updates []telegramUpdate
	if err := a.call(ctx, "getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (a *adapter) call(ctx context.Context, method string, params url.Values, result any) error {
	a.mu.Lock()
	base, token := a.apiBase, a.token
	a.mu.Unlock()
	if base == "" || token == "" {
		return errors.New("telegram adapter is not started")
	}
	return a.callWithCredential(ctx, base, token, method, params, result)
}

func (a *adapter) callWithCredential(ctx context.Context, base, token, method string, params url.Values, result any) error {
	if params == nil {
		params = url.Values{}
	}
	timeout := a.requestTimeout(30 * time.Second)
	if method == "getUpdates" {
		timeout = a.requestTimeout(a.pollTimeout)
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := telegramEndpoint(base, token, method)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return fmt.Errorf("telegram %s request creation failed", method)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		return telegramTransportError(method, requestCtx, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramBodyBytes+1))
	if err != nil {
		return fmt.Errorf("telegram %s response read failed", method)
	}
	if len(body) > maxTelegramBodyBytes {
		return fmt.Errorf("telegram %s response exceeded %d bytes", method, maxTelegramBodyBytes)
	}
	var envelope telegramEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("telegram %s response was not valid JSON", method)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.OK {
		description := sanitizeDescription(envelope.Description, base, token)
		if description == "" {
			description = "request rejected"
		}
		return fmt.Errorf("telegram %s rejected: HTTP %d code=%d %s", method, resp.StatusCode, envelope.ErrorCode, description)
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("telegram %s result decode failed", method)
	}
	return nil
}

func (a *adapter) requestTimeout(operation time.Duration) time.Duration {
	if operation <= 0 {
		operation = defaultPollTimeout
	}
	slack := a.requestSlack
	if slack <= 0 {
		slack = defaultRequestSlack
	}
	return operation + slack
}

func inboundMessage(update telegramUpdate) (bot.InboundMessage, bool) {
	msg := update.Message
	if msg == nil || strings.TrimSpace(msg.Text) == "" || msg.Chat.ID == 0 || msg.From == nil || msg.From.ID == 0 {
		return bot.InboundMessage{}, false
	}
	chatType := bot.ChatDM
	switch strings.ToLower(strings.TrimSpace(msg.Chat.Type)) {
	case "group", "supergroup":
		chatType = bot.ChatGroup
	case "channel":
		chatType = bot.ChatGuild
	}
	threadID := ""
	if msg.MessageThreadID != 0 {
		chatType = bot.ChatThread
		threadID = strconv.FormatInt(msg.MessageThreadID, 10)
	}
	updateID := strconv.FormatInt(update.UpdateID, 10)
	return bot.InboundMessage{
		Platform:         bot.PlatformTelegram,
		Domain:           "telegram",
		ChatType:         chatType,
		ChatID:           strconv.FormatInt(msg.Chat.ID, 10),
		UserID:           strconv.FormatInt(msg.From.ID, 10),
		UserName:         firstNonEmpty(strings.TrimSpace(msg.From.Username), strings.TrimSpace(msg.From.FirstName)),
		Text:             msg.Text,
		MessageID:        updateID,
		ReplyToMessageID: strconv.FormatInt(msg.MessageID, 10),
		RecoveryCursor:   updateID,
		ThreadID:         threadID,
	}, true
}

func telegramToken(envName string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return "", errors.New("telegram token_env is not configured")
	}
	token := strings.TrimSpace(os.Getenv(envName))
	if token == "" {
		return "", fmt.Errorf("telegram credential environment variable %s is not set", envName)
	}
	if strings.ContainsAny(token, "\r\n/\\?#") {
		return "", fmt.Errorf("telegram credential environment variable %s contains invalid characters", envName)
	}
	return token, nil
}

func normalizeAPIBase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultAPIBase
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("telegram api_base must be an absolute URL without credentials, query, or fragment")
	}
	if u.Scheme != "https" {
		host := strings.TrimSpace(u.Hostname())
		ip := net.ParseIP(host)
		if u.Scheme != "http" || !(strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())) {
			return "", errors.New("telegram api_base requires HTTPS; HTTP is allowed only for loopback fixtures")
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func telegramEndpoint(base, token, method string) string {
	return strings.TrimRight(base, "/") + "/bot" + url.PathEscape(token) + "/" + url.PathEscape(method)
}

func telegramTransportError(method string, ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("telegram %s transport timed out", method)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return fmt.Errorf("telegram %s transport failed (timeout=%t temporary=%t)", method, netErr.Timeout(), netErr.Temporary())
	}
	return fmt.Errorf("telegram %s transport failed (%T)", method, err)
}

func sanitizeDescription(description, base, token string) string {
	description = strings.ReplaceAll(description, token, "<redacted>")
	description = strings.ReplaceAll(description, base, "<telegram-api>")
	description = strings.Join(strings.Fields(description), " ")
	if len(description) > 200 {
		description = description[:200]
	}
	return description
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type telegramEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID       int64         `json:"message_id"`
	MessageThreadID int64         `json:"message_thread_id"`
	Text            string        `json:"text"`
	Chat            telegramChat  `json:"chat"`
	From            *telegramUser `json:"from"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}
