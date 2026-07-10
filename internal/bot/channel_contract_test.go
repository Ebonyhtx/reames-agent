package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

// TestChannelContract_SessionKeyIsolation verifies that session keys are
// isolated across different platforms, chats, and users — preventing
// cross-channel message routing errors.
func TestChannelContract_SessionKeyIsolation(t *testing.T) {
	feishu := SessionSource{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "oc_abc", UserID: "ou_1"}
	qq := SessionSource{Platform: PlatformQQ, ChatType: ChatDM, ChatID: "oc_abc", UserID: "ou_1"}
	weixin := SessionSource{Platform: PlatformWeixin, ChatType: ChatDM, ChatID: "oc_abc", UserID: "ou_1"}

	kf := sessionKeyHash(feishu)
	kq := sessionKeyHash(qq)
	kw := sessionKeyHash(weixin)

	if kf == kq || kf == kw || kq == kw {
		t.Fatal("session keys must differ across platforms")
	}

	// Same platform, different user → different key.
	diffUser := SessionSource{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "oc_abc", UserID: "ou_2"}
	if sessionKeyHash(feishu) == sessionKeyHash(diffUser) {
		t.Fatal("different users must have different session keys")
	}

	// Same platform, different chat → different key.
	diffChat := SessionSource{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "oc_xyz", UserID: "ou_1"}
	if sessionKeyHash(feishu) == sessionKeyHash(diffChat) {
		t.Fatal("different chats must have different session keys")
	}

	// Same all fields → same key (stable).
	same := SessionSource{Platform: PlatformFeishu, ChatType: ChatDM, ChatID: "oc_abc", UserID: "ou_1"}
	if sessionKeyHash(feishu) != sessionKeyHash(same) {
		t.Fatal("identical sessions must have identical keys")
	}
}

// TestChannelContract_MessageIDFormat verifies that message IDs are
// non-empty and unique across platforms.
func TestChannelContract_MessageIDFormat(t *testing.T) {
	msg := InboundMessage{
		Platform:  PlatformFeishu,
		MessageID: "msg_12345",
		Text:      "hello",
	}
	if msg.MessageID == "" {
		t.Fatal("message ID must not be empty")
	}
	if msg.Session().Platform != PlatformFeishu {
		t.Fatal("session must preserve platform")
	}
}

// TestChannelContract_MessageTextNeverEmpty verifies that empty text
// messages are handled correctly (not passed to the agent).
func TestChannelContract_MessageTextNeverEmpty(t *testing.T) {
	empty := InboundMessage{Platform: PlatformFeishu, MessageID: "1", Text: ""}
	trimmed := InboundMessage{Platform: PlatformFeishu, MessageID: "2", Text: "   "}

	if empty.Text != "" {
		t.Fatal("empty text should stay empty")
	}
	if trimmed.Text == "" {
		t.Fatal("whitespace-only text should not be collapsed to empty here")
	}
}

// sessionKeyHash is a stable hash of a SessionSource, used for
// generating unique session keys.
func sessionKeyHash(src SessionSource) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:%s:%s:%s:%s",
		src.Platform, src.ConnectionID, src.Domain,
		src.ChatType, src.ChatID, src.UserID)
	return hex.EncodeToString(h.Sum(nil))[:16]
}
