// Package telegram implements a Telegram Bot API adapter for the Reames Agent
// gateway. Uses only net/http — no external SDK dependency.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// Bot is a Telegram bot adapter.
type Bot struct {
	token   string
	client  *http.Client
	handler func(Incoming)
	offset  int64
}

// Incoming is a received Telegram message.
type Incoming struct {
	ChatID   int64
	UserID   int64
	UserName string
	Text     string
	MsgID    int64
}

// Outgoing is a message to send to Telegram.
type Outgoing struct {
	ChatID    int64
	Text      string
	ParseMode string // "HTML" or "MarkdownV2"
}

// New creates a Telegram bot adapter.
func New(token string, handler func(Incoming)) *Bot {
	return &Bot{
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
		handler: handler,
	}
}

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (b *Bot) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := b.getUpdates()
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= b.offset {
				b.offset = u.UpdateID + 1
			}
			if u.Message == nil || u.Message.Text == "" {
				continue
			}
			b.handler(Incoming{
				ChatID:   u.Message.Chat.ID,
				UserID:   u.Message.From.ID,
				UserName: u.Message.From.FirstName,
				Text:     u.Message.Text,
				MsgID:    u.Message.MessageID,
			})
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// SendText sends a text message to a Telegram chat.
func (b *Bot) SendText(ctx context.Context, chatID int64, text string) error {
	return b.apiCall(ctx, "sendMessage", map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
		"text":    text,
	})
}

func (b *Bot) apiCall(ctx context.Context, method string, params map[string]string) error {
	data := url.Values{}
	for k, v := range params {
		data.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, "POST",
		apiBase+b.token+"/"+method, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram api %s: %d %s", method, resp.StatusCode, string(body))
	}
	return nil
}

type updateResponse struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

type update struct {
	UpdateID int64    `json:"update_id"`
	Message  *message `json:"message"`
}

type message struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      chat   `json:"chat"`
	From      user   `json:"from"`
}

type chat struct {
	ID int64 `json:"id"`
}

type user struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

func (b *Bot) getUpdates() ([]update, error) {
	u := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=10", apiBase, b.token, b.offset)
	resp, err := b.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r updateResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return r.Result, nil
}

// buildReply composes a formatted reply.
func buildReply(text string) string {
	var b bytes.Buffer
	b.WriteString(text)
	return b.String()
}
