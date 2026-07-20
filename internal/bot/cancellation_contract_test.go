package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

type cancellationSettlement struct {
	messageID string
	delivered bool
}

type cancellationContractAdapter struct {
	*fakeAdapter
	sendErr     error
	settlements chan cancellationSettlement
}

func newCancellationContractAdapter(platform Platform, sendErr error) *cancellationContractAdapter {
	return &cancellationContractAdapter{
		fakeAdapter: newFakeAdapter(platform, "cancel-contract-"+string(platform)),
		sendErr:     sendErr,
		settlements: make(chan cancellationSettlement, 8),
	}
}

func (a *cancellationContractAdapter) Send(ctx context.Context, msg OutboundMessage) (SendResult, error) {
	if a.sendErr != nil {
		return SendResult{}, a.sendErr
	}
	return a.fakeAdapter.Send(ctx, msg)
}

func (a *cancellationContractAdapter) SettleInbound(messageID string, delivered bool) {
	a.settlements <- cancellationSettlement{messageID: messageID, delivered: delivered}
}

func TestGatewayStopCancellationContractAcrossPlatforms(t *testing.T) {
	platforms := []struct {
		platform Platform
		id       string
		domain   string
	}{
		{platform: PlatformFeishu, id: "feishu-main", domain: "feishu"},
		{platform: PlatformQQ, id: "qq-main", domain: "qq"},
		{platform: PlatformWeixin, id: "weixin-main", domain: "weixin"},
		{platform: PlatformTelegram, id: "telegram-main", domain: "telegram"},
	}
	ackOutcomes := []struct {
		name    string
		sendErr error
	}{
		{name: "ack_succeeds"},
		{name: "ack_fails", sendErr: errors.New("fixture acknowledgement failure")},
	}

	for _, platform := range platforms {
		for _, ack := range ackOutcomes {
			t.Run(string(platform.platform)+"/"+ack.name, func(t *testing.T) {
				adapter := newCancellationContractAdapter(platform.platform, ack.sendErr)
				binding := AdapterBinding{
					ID:       platform.id,
					Domain:   platform.domain,
					Platform: platform.platform,
					Adapter:  adapter,
				}
				gw := NewGatewayWithAdapterBindings(GatewayConfig{
					Enabled:      map[Platform]bool{platform.platform: true},
					Allowlist:    AllowlistConfig{AllowAll: true},
					RecoveryPath: filepath.Join(t.TempDir(), "delivery-ledger.json"),
				}, []AdapterBinding{binding}, slog.New(slog.NewTextHandler(io.Discard, nil)))
				ctx, cancel := context.WithCancel(context.Background())
				if err := gw.Start(ctx); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					cancel()
					gw.Stop()
				})

				message := func(suffix, text string, sequence int) InboundMessage {
					return InboundMessage{
						Platform:       platform.platform,
						ConnectionID:   platform.id,
						Domain:         platform.domain,
						ChatType:       ChatDM,
						ChatID:         "chat",
						UserID:         "user",
						Text:           text,
						MessageID:      string(platform.platform) + "-" + suffix,
						RecoveryCursor: string(platform.platform) + "-cursor-" + strconv.Itoa(sequence),
					}
				}
				active := message("active", "active turn", 1)
				queued := message("queued", "queued follow-up", 2)
				stop := message("stop", "/stop", 3)
				key := BuildSessionKey(active.Session())

				for _, msg := range []InboundMessage{active, queued} {
					claimed, err := gw.recovery.claim(msg)
					if err != nil || !claimed {
						t.Fatalf("claim %q = %t, %v", msg.MessageID, claimed, err)
					}
				}
				if result := gw.sessions.TryAcquireWithQueue(key, active, QueueOptions{Mode: QueueModeFollowup}); !result.Acquired {
					t.Fatalf("active acquire = %+v", result)
				}
				if result := gw.sessions.TryAcquireWithQueue(key, queued, QueueOptions{Mode: QueueModeFollowup}); !result.Queued {
					t.Fatalf("queued acquire = %+v", result)
				}
				canceled := make(chan struct{})
				var cancelOnce sync.Once
				gw.mu.Lock()
				gw.controllers[key] = &sessionState{
					cancel:        func() { cancelOnce.Do(func() { close(canceled) }) },
					activeInbound: &active,
				}
				gw.mu.Unlock()

				adapter.msgCh <- stop
				select {
				case <-canceled:
				case <-time.After(2 * time.Second):
					t.Fatal("/stop did not cancel the active turn")
				}

				wantDelivered := ack.sendErr == nil
				settled := make(map[string]int)
				for range 3 {
					select {
					case outcome := <-adapter.settlements:
						if outcome.delivered != wantDelivered {
							t.Fatalf("settlement %q delivered = %t, want %t", outcome.messageID, outcome.delivered, wantDelivered)
						}
						settled[outcome.messageID]++
					case <-time.After(2 * time.Second):
						t.Fatalf("settlements = %#v, want active, queued, and stop", settled)
					}
				}
				for _, msg := range []InboundMessage{active, queued, stop} {
					if settled[msg.MessageID] != 1 {
						t.Fatalf("settlement count for %q = %d, want 1", msg.MessageID, settled[msg.MessageID])
					}
				}
				select {
				case duplicate := <-adapter.settlements:
					t.Fatalf("duplicate settlement = %+v", duplicate)
				case <-time.After(20 * time.Millisecond):
				}
				if gw.sessions.IsActive(key) || gw.sessions.PendingCount(key) != 0 {
					t.Fatalf("session remained active or queued after /stop: active=%t pending=%d", gw.sessions.IsActive(key), gw.sessions.PendingCount(key))
				}

				if wantDelivered {
					checkpoints := gw.recovery.checkpoints(binding)
					if len(checkpoints) != 1 || checkpoints[0].Cursor != stop.RecoveryCursor || checkpoints[0].Sequence != 3 {
						t.Fatalf("checkpoints = %+v, want contiguous stop cursor at sequence 3", checkpoints)
					}
					if snapshot := gw.recovery.snapshot(); snapshot.Delivered != 3 || snapshot.Failed != 0 {
						t.Fatalf("recovery snapshot = %+v, want three delivered claims", snapshot)
					}
					return
				}

				if checkpoints := gw.recovery.checkpoints(binding); len(checkpoints) != 0 {
					t.Fatalf("failed acknowledgement advanced checkpoints: %+v", checkpoints)
				}
				if snapshot := gw.recovery.snapshot(); snapshot.Failed != 3 || snapshot.Delivered != 0 {
					t.Fatalf("recovery snapshot = %+v, want three retryable failures", snapshot)
				}
				for _, msg := range []InboundMessage{active, queued, stop} {
					claimed, err := gw.recovery.claim(msg)
					if err != nil || !claimed {
						t.Fatalf("retry claim %q = %t, %v", msg.MessageID, claimed, err)
					}
				}
			})
		}
	}
}
