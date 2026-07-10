package bot

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMessageFixture_Deduplicator verifies message-ID-based dedup
// prevents replay attacks and correctly expires old entries.
func TestMessageFixture_Deduplicator(t *testing.T) {
	d := newMsgDedup(3, 50*time.Millisecond)
	defer d.Stop()

	if d.IsDuplicate("msg-1") {
		t.Fatal("first msg-1 should not be duplicate")
	}
	if !d.IsDuplicate("msg-1") {
		t.Fatal("second msg-1 should be duplicate (replay detection)")
	}
	if d.IsDuplicate("msg-2") {
		t.Fatal("msg-2 should not be duplicate")
	}
	time.Sleep(60 * time.Millisecond)
	if d.IsDuplicate("msg-1") {
		t.Fatal("msg-1 should not be duplicate after expiry")
	}
}

func TestMessageFixture_ConcurrentDedup(t *testing.T) {
	d := newMsgDedup(1000, 10*time.Second)
	defer d.Stop()
	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			results <- d.IsDuplicate(fmt.Sprintf("msg-%d", id))
		}(i)
	}
	wg.Wait()
	close(results)
	for r := range results {
		if r {
			t.Fatal("concurrent first occurrence should not be duplicate")
		}
	}
}

func TestMessageFixture_DedupBounds(t *testing.T) {
	d := newMsgDedup(10, time.Hour)
	defer d.Stop()
	for i := 0; i < 20; i++ {
		d.IsDuplicate(fmt.Sprintf("msg-%d", i))
	}
	if d.Len() > 10 {
		t.Fatalf("dedup len = %d, want <= 10", d.Len())
	}
}

func TestMessageFixture_TextNormalization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"leading whitespace", "  hello  ", "hello"},
		{"multiline", "line1\nline2\n", "line1\nline2"},
		{"trailing newlines", "text\n\n\n", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeText(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- implementation ---

type msgDedup struct {
	mu       sync.Mutex
	seen     map[string]time.Time
	capacity int
	ttl      time.Duration
	ticker   *time.Ticker
	done     chan struct{}
}

func newMsgDedup(capacity int, ttl time.Duration) *msgDedup {
	d := &msgDedup{
		seen:     make(map[string]time.Time, capacity),
		capacity: capacity,
		ttl:      ttl,
		ticker:   time.NewTicker(ttl / 2),
		done:     make(chan struct{}),
	}
	go d.reapLoop()
	return d
}

func (d *msgDedup) IsDuplicate(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.seen[id]; ok {
		return true
	}
	if len(d.seen) >= d.capacity {
		for k := range d.seen {
			delete(d.seen, k)
			break
		}
	}
	d.seen[id] = time.Now()
	return false
}

func (d *msgDedup) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.seen)
}

func (d *msgDedup) Stop() {
	close(d.done)
	d.ticker.Stop()
}

func (d *msgDedup) reapLoop() {
	for {
		select {
		case <-d.ticker.C:
			d.reap()
		case <-d.done:
			return
		}
	}
}

func (d *msgDedup) reap() {
	d.mu.Lock()
	defer d.mu.Unlock()
	cutoff := time.Now().Add(-d.ttl)
	for id, ts := range d.seen {
		if ts.Before(cutoff) {
			delete(d.seen, id)
		}
	}
}

func normalizeText(s string) string {
	result := make([]byte, 0, len(s))
	skip := true
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\n' {
			if len(result) > 0 && result[len(result)-1] == '\n' {
				continue // collapse multiple newlines at end
			}
			result = append(result, ch)
			skip = true
		} else if ch == ' ' || ch == '\t' {
			if skip {
				continue
			}
			result = append(result, ' ')
			skip = true
		} else {
			result = append(result, ch)
			skip = false
		}
	}
	for len(result) > 0 && (result[len(result)-1] == '\n' || result[len(result)-1] == ' ') {
		result = result[:len(result)-1]
	}
	return string(result)
}
