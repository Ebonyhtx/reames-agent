package bot

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSessionKey(t *testing.T) {
	tests := []struct {
		name string
		src  SessionSource
		// DM 同 chat 不同 user 应返回相同 key
		wantSame bool
		src2     SessionSource
	}{
		{
			name:     "dm same chat different user",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "user123", UserID: "a"},
			src2:     SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "user123", UserID: "b"},
			wantSame: true,
		},
		{
			name:     "dm different chat",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "user123", UserID: "a"},
			src2:     SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "user456", UserID: "a"},
			wantSame: false,
		},
		{
			name:     "direct same chat different user",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatDirect, ChatID: "guild123", UserID: "a"},
			src2:     SessionSource{Platform: PlatformQQ, ChatType: ChatDirect, ChatID: "guild123", UserID: "b"},
			wantSame: true,
		},
		{
			name:     "direct distinct from dm",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatDirect, ChatID: "shared", UserID: "a"},
			src2:     SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "shared", UserID: "a"},
			wantSame: false,
		},
		{
			name:     "group same chat different user",
			src:      SessionSource{Platform: PlatformFeishu, ChatType: ChatGroup, ChatID: "group1", UserID: "a"},
			src2:     SessionSource{Platform: PlatformFeishu, ChatType: ChatGroup, ChatID: "group1", UserID: "b"},
			wantSame: false,
		},
		{
			name:     "group same user different chat",
			src:      SessionSource{Platform: PlatformFeishu, ChatType: ChatGroup, ChatID: "group1", UserID: "a"},
			src2:     SessionSource{Platform: PlatformFeishu, ChatType: ChatGroup, ChatID: "group2", UserID: "a"},
			wantSame: false,
		},
		{
			name:     "thread shared",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatThread, ChatID: "ch1", ThreadID: "th1", UserID: "a"},
			src2:     SessionSource{Platform: PlatformQQ, ChatType: ChatThread, ChatID: "ch1", ThreadID: "th1", UserID: "b"},
			wantSame: true,
		},
		{
			name:     "different platform same ids",
			src:      SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "123", UserID: "u1"},
			src2:     SessionSource{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "123", UserID: "u1"},
			wantSame: false,
		},
		{
			name:     "same platform different connection",
			src:      SessionSource{Platform: PlatformFeishu, ConnectionID: "feishu-feishu", ChatType: ChatDM, ChatID: "123", UserID: "u1"},
			src2:     SessionSource{Platform: PlatformFeishu, ConnectionID: "feishu-lark", ChatType: ChatDM, ChatID: "123", UserID: "u1"},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k1 := BuildSessionKey(tt.src)
			k2 := BuildSessionKey(tt.src2)
			if tt.wantSame && k1 != k2 {
				t.Errorf("want same key, got %s != %s", k1, k2)
			}
			if !tt.wantSame && k1 == k2 {
				t.Errorf("want different keys, got %s == %s", k1, k2)
			}
		})
	}
}

func TestIsSlashBypass(t *testing.T) {
	tests := []struct {
		text   string
		bypass bool
	}{
		{"/stop", true},
		{"/stop  extra args", true},
		{"/stopwatch", false},
		{"/new", true},
		{"/reset", true},
		{"/current", true},
		{"/approve", true},
		{"/deny", true},
		{"/yolo", true},
		{"/yolo on", true},
		{"/mode yolo", true},
		{"/status", true},
		{"/statusx", false},
		{"/help", true},
		{"hello", false},
		{"/unknown", false},
		{"", false},
		{" /stop", false}, // leading space means not a slash command
	}

	for _, tt := range tests {
		got := IsSlashBypass(tt.text)
		if got != tt.bypass {
			t.Errorf("IsSlashBypass(%q) = %v, want %v", tt.text, got, tt.bypass)
		}
	}
}

func TestParseSlashCommand(t *testing.T) {
	cmd, ok := ParseSlashCommand("/CURRENT now")
	if !ok {
		t.Fatal("/CURRENT should parse as a known slash command")
	}
	if cmd.Verb != "/current" {
		t.Fatalf("verb = %q, want /current", cmd.Verb)
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "now" || cmd.RawArgs != "now" {
		t.Fatalf("command = %+v, want one arg and raw args", cmd)
	}
	if _, ok := ParseSlashCommand(" /current"); ok {
		t.Fatal("leading whitespace should keep text as a normal message")
	}
	if _, ok := ParseSlashCommand("/currently"); ok {
		t.Fatal("prefix-only matches must not be parsed as commands")
	}
}

func TestSessionManager_TryAcquire(t *testing.T) {
	sm := NewSessionManager(100 * time.Millisecond)

	msg := InboundMessage{Text: "hello", Platform: PlatformQQ, ChatType: ChatDM, ChatID: "c1", UserID: "u1"}
	key := BuildSessionKey(msg.Session())

	// 第一次获取成功
	acquired, merged := sm.TryAcquire(key, msg)
	if !acquired || merged {
		t.Error("first acquire should succeed")
	}

	// 第二次获取应该排队
	acquired, merged = sm.TryAcquire(key, InboundMessage{Text: "world"})
	if acquired || !merged {
		t.Error("second acquire should merge into queue")
	}

	// slash bypass 命令应绕过
	acquired, merged = sm.TryAcquire(key, InboundMessage{Text: "/stop"})
	if !acquired || merged {
		t.Error("slash bypass should acquire immediately")
	}

	// 第一次 Release 返回排队消息
	next := sm.Release(key)
	if next == nil {
		t.Fatal("expected queued message after first release")
	}
	if next.Text != "world" {
		t.Errorf("merged text = %q, want %q", next.Text, "world")
	}
}

func TestSessionManager_Debounce(t *testing.T) {
	sm := NewSessionManager(200 * time.Millisecond)

	msg := InboundMessage{Text: "first", Platform: PlatformQQ, ChatType: ChatDM, ChatID: "c1", UserID: "u1"}
	key := BuildSessionKey(msg.Session())

	acquired, _ := sm.TryAcquire(key, msg)
	if !acquired {
		t.Fatal("first acquire should succeed")
	}

	// 同 session 消息应合并
	sm.TryAcquire(key, InboundMessage{Text: "second"})
	// 在 debounce 窗口内发第三条
	sm.TryAcquire(key, InboundMessage{Text: "third"})

	next := sm.Release(key)
	if next == nil {
		t.Fatal("expected queued message after release")
	}
	// "second" 和 "third" 合并在队列里（"first" 已作为 active 被处理）
	if next.Text != "second\nthird" {
		t.Errorf("merged = %q, want %q", next.Text, "second\nthird")
	}
}

func TestSessionManagerCollectCarriesMergedDeliveryClaimsAndMedia(t *testing.T) {
	sm := NewSessionManager(time.Second)
	base := InboundMessage{Platform: PlatformWeixin, ConnectionID: "primary", Domain: "weixin", ChatType: ChatDM, ChatID: "chat", UserID: "user"}
	active := base
	active.MessageID = "active"
	active.Text = "active"
	key := BuildSessionKey(active.Session())
	if result := sm.TryAcquireWithQueue(key, active, QueueOptions{Mode: QueueModeCollect}); !result.Acquired {
		t.Fatalf("active result = %+v", result)
	}
	second := base
	second.MessageID = "second"
	second.Text = "second"
	second.MediaURLs = []string{"https://example.test/second.png"}
	third := base
	third.MessageID = "third"
	third.Text = "third"
	third.MediaURLs = []string{"https://example.test/third.png"}
	sm.TryAcquireWithQueue(key, second, QueueOptions{Mode: QueueModeCollect})
	sm.TryAcquireWithQueue(key, third, QueueOptions{Mode: QueueModeCollect})

	merged := sm.Release(key)
	if merged == nil || merged.Text != "second\nthird" {
		t.Fatalf("merged = %+v", merged)
	}
	claims := inboundDeliveryClaims(*merged)
	if len(claims) != 2 || claims[0].MessageID != second.MessageID || claims[1].MessageID != third.MessageID {
		t.Fatalf("merged claims = %+v", claims)
	}
	if len(merged.MediaURLs) != 2 || merged.MediaURLs[0] != second.MediaURLs[0] || merged.MediaURLs[1] != third.MediaURLs[0] {
		t.Fatalf("merged media = %+v", merged.MediaURLs)
	}
}

func TestSessionManagerQueueCapCarriesDroppedClaimIntoNextTurn(t *testing.T) {
	sm := NewSessionManager(time.Second)
	base := InboundMessage{Platform: PlatformWeixin, ConnectionID: "primary", ChatType: ChatDM, ChatID: "chat", UserID: "user"}
	active := base
	active.MessageID = "active"
	key := BuildSessionKey(active.Session())
	sm.TryAcquireWithQueue(key, active, QueueOptions{Mode: QueueModeFollowup, Cap: 1})
	old := base
	old.MessageID = "old"
	old.Text = "old text"
	sm.TryAcquireWithQueue(key, old, QueueOptions{Mode: QueueModeFollowup, Cap: 1, Drop: QueueDropSummarize})
	newest := base
	newest.MessageID = "newest"
	newest.Text = "new text"
	result := sm.TryAcquireWithQueue(key, newest, QueueOptions{Mode: QueueModeFollowup, Cap: 1, Drop: QueueDropSummarize})
	if !result.Queued || !result.Dropped {
		t.Fatalf("queue result = %+v", result)
	}
	merged := sm.Release(key)
	if merged == nil || !strings.Contains(merged.Text, "old text") {
		t.Fatalf("released message = %+v", merged)
	}
	claims := inboundDeliveryClaims(*merged)
	if len(claims) != 2 || claims[0].MessageID != newest.MessageID || claims[1].MessageID != old.MessageID {
		t.Fatalf("queue-cap claims = %+v", claims)
	}
}

func TestSessionManagerInterruptReportsSupersededQueueClaims(t *testing.T) {
	sm := NewSessionManager(time.Second)
	base := InboundMessage{Platform: PlatformWeixin, ConnectionID: "primary", ChatType: ChatDM, ChatID: "chat", UserID: "user"}
	active := base
	active.MessageID = "active"
	key := BuildSessionKey(active.Session())
	sm.TryAcquireWithQueue(key, active, QueueOptions{Mode: QueueModeFollowup})
	queued := base
	queued.MessageID = "queued"
	sm.TryAcquireWithQueue(key, queued, QueueOptions{Mode: QueueModeFollowup})
	interrupt := base
	interrupt.MessageID = "interrupt"
	result := sm.ReplacePending(key, interrupt)
	if len(result.Discarded) != 1 || result.Discarded[0].MessageID != queued.MessageID {
		t.Fatalf("interrupt result = %+v", result)
	}
}

func TestSessionManager_ForceRelease(t *testing.T) {
	sm := NewSessionManager(100 * time.Millisecond)

	msg := InboundMessage{Text: "test", Platform: PlatformQQ, ChatType: ChatDM, ChatID: "c1", UserID: "u1"}
	key := BuildSessionKey(msg.Session())

	sm.TryAcquire(key, msg)
	if !sm.IsActive(key) {
		t.Error("should be active")
	}

	sm.ForceRelease(key)
	if sm.IsActive(key) {
		t.Error("should not be active after force release")
	}
}

func TestHashID(t *testing.T) {
	h1 := hashID("user_12345")
	h2 := hashID("user_12345")
	h3 := hashID("user_67890")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different inputs should produce different hashes")
	}
	if hashID("") != "" {
		t.Error("empty input should produce empty hash")
	}
}

func TestInboundMessage_Session(t *testing.T) {
	msg := InboundMessage{
		Platform:     PlatformQQ,
		ConnectionID: "qq-main",
		Domain:       "qq",
		ChatType:     ChatDM,
		ChatID:       "chat1",
		UserID:       "user1",
		ThreadID:     "thread1",
	}

	src := msg.Session()
	if src.Platform != PlatformQQ || src.ConnectionID != "qq-main" || src.Domain != "qq" || src.ChatType != ChatDM || src.ChatID != "chat1" || src.UserID != "user1" || src.ThreadID != "thread1" {
		t.Error("Session() should copy all fields")
	}
}
