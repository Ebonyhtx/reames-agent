// Package bot 实现 Reames Agent 多渠道 IM bot 消息网关，支持 QQ、飞书、微信和 Telegram。
// 架构参考 Hermes 项目的 gateway/adapter/session 模式。
package bot

import (
	"context"
	"sync"
	"time"
)

// Platform 标识 IM 平台。
type Platform string

const (
	PlatformQQ       Platform = "qq"
	PlatformFeishu   Platform = "feishu"
	PlatformWeixin   Platform = "weixin"
	PlatformTelegram Platform = "telegram"
)

// ChatType 标识会话类型。
type ChatType string

const (
	ChatDM     ChatType = "dm"
	ChatGroup  ChatType = "group"
	ChatGuild  ChatType = "guild"
	ChatDirect ChatType = "direct"
	ChatThread ChatType = "thread"
)

// SessionSource 是会话的复合标识，用于生成稳定的 session key。
type SessionSource struct {
	Platform     Platform `json:"platform"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	ChatType     ChatType `json:"chat_type"`
	ChatID       string   `json:"chat_id"`
	UserID       string   `json:"user_id"`
	ThreadID     string   `json:"thread_id,omitempty"`
}

// InboundMessage 是从任一平台收到的入站消息。
type InboundMessage struct {
	Platform     Platform `json:"platform"`
	ConnectionID string   `json:"connection_id,omitempty"`
	Domain       string   `json:"domain,omitempty"`
	ChatType     ChatType `json:"chat_type"`
	ChatID       string   `json:"chat_id"`
	UserID       string   `json:"user_id"`
	UserName     string   `json:"user_name"`
	// OperatorID, when set, is the authenticated actor gated by the allowlist; UserID stays routing-only.
	OperatorID string `json:"operator_id,omitempty"`
	Text       string `json:"text"`
	MessageID  string `json:"message_id"`
	// ReplyToMessageID is the platform-native message identifier used for
	// threaded replies when the durable delivery identity differs. Telegram,
	// for example, uses the global update_id for recovery and message_id for
	// reply_to_message_id. It is runtime-only and never enters provider prompts.
	ReplyToMessageID string `json:"-"`
	// RecoveryCursor is an opaque adapter-owned position used only for missed
	// message recovery. It is committed after final outbound delivery, never on
	// receipt. Recovered identifies messages returned by RecoveryAdapter.
	RecoveryCursor string   `json:"-"`
	Recovered      bool     `json:"-"`
	ThreadID       string   `json:"thread_id,omitempty"`
	MediaURLs      []string `json:"media_urls,omitempty"`
	Raw            any      `json:"-"`
	// deliveryClaims carries already-claimed messages that the queue merged,
	// summarized, or intentionally superseded into this message. It is runtime
	// only and lets one final response settle every durable claim it represents.
	deliveryClaims []deliveryClaim
}

// Session derives the SessionSource from this message.
func (m InboundMessage) Session() SessionSource {
	return SessionSource{
		Platform:     m.Platform,
		ConnectionID: m.ConnectionID,
		Domain:       m.Domain,
		ChatType:     m.ChatType,
		ChatID:       m.ChatID,
		UserID:       m.UserID,
		ThreadID:     m.ThreadID,
	}
}

// OutboundMessage 是发送到平台的消息。
type OutboundMessage struct {
	ConnectionID string           `json:"connection_id,omitempty"`
	Domain       string           `json:"domain,omitempty"`
	ChatID       string           `json:"chat_id"`
	ChatType     ChatType         `json:"chat_type,omitempty"`
	Text         string           `json:"text,omitempty"`
	MediaURLs    []string         `json:"media_urls,omitempty"`
	ReplyToMsgID string           `json:"reply_to_msg_id,omitempty"`
	Keyboard     *InlineKeyboard  `json:"keyboard,omitempty"`
	Card         *InteractiveCard `json:"card,omitempty"`
}

// InlineKeyboard 是内联键盘（用于 QQ 审批）。
type InlineKeyboard struct {
	Rows []InlineKeyboardRow `json:"rows"`
}

// InlineKeyboardRow 是一行按钮。
type InlineKeyboardRow struct {
	Buttons []InlineKeyboardButton `json:"buttons"`
}

// InlineKeyboardButton 是一个按钮。
type InlineKeyboardButton struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Style      int    `json:"style,omitempty"` // 0=default, 1=primary, 2=danger
	CallbackID string `json:"callback_id,omitempty"`
}

// InteractiveCard 是交互式卡片（用于飞书审批/问答）。
type InteractiveCard struct {
	Header   string                   `json:"header"`
	Elements []InteractiveCardElement `json:"elements"`
}

// InteractiveCardElement 是卡片内元素。
type InteractiveCardElement struct {
	Tag     string         `json:"tag"`
	Content string         `json:"content,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

// SendResult 是发送消息的结果。
type SendResult struct {
	MessageID string `json:"message_id,omitempty"`
	Err       error  `json:"err,omitempty"`
}

// Adapter 是平台适配器接口，每个平台实现一个。
type Adapter interface {
	// Platform 返回平台标识。
	Platform() Platform

	// Start 启动适配器，连接平台 gateway。
	Start(ctx context.Context) error

	// Stop 优雅关闭适配器。
	Stop() error

	// Send 发送一条出站消息。
	Send(ctx context.Context, msg OutboundMessage) (SendResult, error)

	// SendTyping 发送"正在输入"状态。
	SendTyping(ctx context.Context, chatID string) error

	// Messages 返回入站消息通道。
	Messages() <-chan InboundMessage

	// Name 返回适配器实例名（用于日志）。
	Name() string
}

// AdapterConnectionState is a transport-neutral connection lifecycle state.
type AdapterConnectionState string

const (
	AdapterConnecting   AdapterConnectionState = "connecting"
	AdapterRunning      AdapterConnectionState = "running"
	AdapterReconnecting AdapterConnectionState = "reconnecting"
	AdapterClosed       AdapterConnectionState = "closed"
)

// AdapterConnectionEvent carries only bounded host-defined state. Reason must
// be a fixed non-secret identifier, never a raw SDK or transport error.
type AdapterConnectionEvent struct {
	State  AdapterConnectionState
	Reason string
	At     time.Time
}

// AdapterConnectionStateSource is an optional adapter capability consumed by
// BotGateway health, status, metrics, and watchdog projection.
type AdapterConnectionStateSource interface {
	ConnectionEvents() <-chan AdapterConnectionEvent
}

// ConnectionReporter is the shared non-blocking lifecycle emitter embedded by
// first-party adapters. A slow health consumer sees the latest bounded states
// without applying backpressure to a platform connection loop.
type ConnectionReporter struct {
	mu sync.Mutex
	ch chan AdapterConnectionEvent
}

func NewConnectionReporter() *ConnectionReporter {
	return &ConnectionReporter{ch: make(chan AdapterConnectionEvent, 8)}
}

func (r *ConnectionReporter) Events() <-chan AdapterConnectionEvent {
	if r == nil {
		return nil
	}
	return r.ch
}

func (r *ConnectionReporter) Report(state AdapterConnectionState, reason string) {
	if r == nil {
		return
	}
	event := AdapterConnectionEvent{State: state, Reason: reason, At: time.Now().UTC()}
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.ch <- event:
		return
	default:
	}
	select {
	case <-r.ch:
	default:
	}
	r.ch <- event
}

// DeliverySettlementAdapter is an optional adapter contract for transports
// whose inbound acknowledgement must wait until Reames has durably recorded
// the final delivery outcome. SettleInbound must be non-blocking and safe for
// concurrent calls. A delivered=false outcome keeps the remote message
// eligible for retry; delivered=true may advance the transport acknowledgement.
type DeliverySettlementAdapter interface {
	SettleInbound(messageID string, delivered bool)
}

// MessageHandler 是 BotGateway 处理入站消息的回调。
type MessageHandler func(ctx context.Context, msg InboundMessage)

// PlatformAdapter 是第三方平台适配器的接口。实现此接口即可将新 IM 平台接入
// Reames Agent Gateway，无需修改核心代码。
type PlatformAdapter interface {
	// Name 返回平台标识（如 "telegram"、"discord"）。
	Name() Platform

	// Start 启动平台连接，开始接收消息。ctx 取消时停止。
	Start(ctx context.Context, handler MessageHandler) error

	// Send 向指定会话发送消息。
	Send(ctx context.Context, target SessionSource, text string, mediaURLs []string) error

	// Capabilities 返回平台支持的能力。
	Capabilities() PlatformCapabilities
}

// PlatformCapabilities 描述平台支持的能力。
type PlatformCapabilities struct {
	RichText    bool // 富文本（Markdown）
	Cards       bool // 交互卡片（审批按钮等）
	MediaUpload bool // 媒体文件上传
	Reactions   bool // 表情反应
	Threads     bool // 消息线程
	Voice       bool // 语音消息
}

// PlatformRegistry 管理已注册的平台适配器。
type PlatformRegistry struct {
	adapters map[Platform]PlatformAdapter
}

// NewPlatformRegistry 创建空注册表。
func NewPlatformRegistry() *PlatformRegistry {
	return &PlatformRegistry{adapters: make(map[Platform]PlatformAdapter)}
}

// Register 注册一个平台适配器。
func (r *PlatformRegistry) Register(a PlatformAdapter) {
	r.adapters[a.Name()] = a
}

// Get 获取已注册的平台适配器。
func (r *PlatformRegistry) Get(name Platform) (PlatformAdapter, bool) {
	a, ok := r.adapters[name]
	return a, ok
}

// List 列出所有已注册平台。
func (r *PlatformRegistry) List() []Platform {
	names := make([]Platform, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}
