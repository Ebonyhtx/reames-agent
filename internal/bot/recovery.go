package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/fileutil"
)

const (
	deliveryLedgerVersion            = 1
	defaultDeliveryRecordLimit       = 4096
	defaultRecoveryScanLimit         = 200
	maxDeliveryLedgerBytes     int64 = 4 << 20

	deliveryStatusProcessing  = "processing"
	deliveryStatusInterrupted = "interrupted"
	deliveryStatusFailed      = "failed"
	deliveryStatusDelivered   = "delivered"
)

// RecoveryCheckpoint is an opaque, adapter-owned cursor for one remote
// delivery channel. The gateway persists it only after the final response for the
// corresponding inbound message has been delivered successfully.
type RecoveryCheckpoint struct {
	Source    SessionSource `json:"source"`
	Cursor    string        `json:"cursor"`
	Sequence  int64         `json:"sequence"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// DeliveryRecoverySnapshot exposes counts only; remote identities, cursors and
// the local ledger path never leave the gateway through status endpoints.
type DeliveryRecoverySnapshot struct {
	Enabled     bool `json:"enabled"`
	Records     int  `json:"records"`
	Processing  int  `json:"processing"`
	Interrupted int  `json:"interrupted"`
	Failed      int  `json:"failed"`
	Delivered   int  `json:"delivered"`
	Checkpoints int  `json:"checkpoints"`
}

// RecoveryAdapter is an optional capability implemented by adapters that can
// list messages missed while their live connection was unavailable. Results
// must be strictly after the supplied channel checkpoint, oldest-first, carry
// MessageID and RecoveryCursor, and must not exceed limit. The gateway applies
// its own global cap and durable claim gate as a second line of defense.
type RecoveryAdapter interface {
	RecoverMissed(ctx context.Context, checkpoints []RecoveryCheckpoint, limit int) ([]InboundMessage, error)
}

type deliveryRecord struct {
	Source    SessionSource `json:"source"`
	MessageID string        `json:"message_id"`
	Cursor    string        `json:"cursor,omitempty"`
	Sequence  int64         `json:"sequence,omitempty"`
	Status    string        `json:"status"`
	Attempts  int           `json:"attempts"`
	UpdatedAt time.Time     `json:"updated_at"`
	LastError string        `json:"last_error,omitempty"`
}

type deliveryClaim struct {
	Source    SessionSource
	MessageID string
}

type deliveryLedgerState struct {
	Version     int                           `json:"version"`
	Records     map[string]deliveryRecord     `json:"records"`
	Checkpoints map[string]RecoveryCheckpoint `json:"checkpoints"`
	Sequences   map[string]int64              `json:"sequences"`
}

type deliveryLedger struct {
	mu          sync.Mutex
	path        string
	recordLimit int
	state       deliveryLedgerState
}

func openDeliveryLedger(path string, recordLimit int) (*deliveryLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	if recordLimit <= 0 {
		recordLimit = defaultDeliveryRecordLimit
	}
	ledger := &deliveryLedger{
		path:        path,
		recordLimit: recordLimit,
		state: deliveryLedgerState{
			Version:     deliveryLedgerVersion,
			Records:     make(map[string]deliveryRecord),
			Checkpoints: make(map[string]RecoveryCheckpoint),
			Sequences:   make(map[string]int64),
		},
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	if err != nil {
		return nil, fmt.Errorf("bot recovery ledger stat: %w", err)
	}
	if info.Size() > maxDeliveryLedgerBytes {
		return nil, fmt.Errorf("bot recovery ledger exceeds %d bytes", maxDeliveryLedgerBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bot recovery ledger read: %w", err)
	}
	var state deliveryLedgerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("bot recovery ledger decode: %w", err)
	}
	if err := validateDeliveryLedgerState(state, recordLimit); err != nil {
		return nil, err
	}
	ledger.state = state
	changed := false
	for key, record := range ledger.state.Records {
		if record.Status == deliveryStatusProcessing {
			record.Status = deliveryStatusInterrupted
			record.LastError = "gateway stopped before final delivery was committed"
			record.UpdatedAt = time.Now().UTC()
			ledger.state.Records[key] = record
			changed = true
		}
	}
	if changed {
		if err := ledger.persistLocked(ledger.state); err != nil {
			return nil, fmt.Errorf("bot recovery ledger recover interrupted claims: %w", err)
		}
	}
	return ledger, nil
}

func validateDeliveryLedgerState(state deliveryLedgerState, recordLimit int) error {
	if state.Version != deliveryLedgerVersion {
		return fmt.Errorf("bot recovery ledger version %d is unsupported", state.Version)
	}
	if state.Records == nil || state.Checkpoints == nil || state.Sequences == nil {
		return errors.New("bot recovery ledger is missing required maps")
	}
	if len(state.Records) > recordLimit {
		return fmt.Errorf("bot recovery ledger contains %d records; limit is %d", len(state.Records), recordLimit)
	}
	if len(state.Sequences) > recordLimit || len(state.Checkpoints) > recordLimit {
		return fmt.Errorf("bot recovery ledger contains too many channel cursors; limit is %d", recordLimit)
	}
	seenSequences := make(map[string]map[int64]bool)
	for key, record := range state.Records {
		if strings.TrimSpace(record.MessageID) == "" {
			return fmt.Errorf("bot recovery ledger record %q has no message identity", key)
		}
		if key != deliveryRecordKey(record.Source, record.MessageID) {
			return fmt.Errorf("bot recovery ledger record %q identity mismatch", key)
		}
		if (record.Cursor == "") != (record.Sequence == 0) {
			return fmt.Errorf("bot recovery ledger record %q cursor/sequence mismatch", key)
		}
		if record.Attempts <= 0 || record.UpdatedAt.IsZero() {
			return fmt.Errorf("bot recovery ledger record %q has invalid attempt metadata", key)
		}
		if record.Cursor != "" {
			channelKey := recoveryChannelKey(record.Source)
			if record.Sequence > state.Sequences[channelKey] {
				return fmt.Errorf("bot recovery ledger record %q exceeds its channel sequence", key)
			}
			if seenSequences[channelKey] == nil {
				seenSequences[channelKey] = make(map[int64]bool)
			}
			if seenSequences[channelKey][record.Sequence] {
				return fmt.Errorf("bot recovery ledger channel %q repeats sequence %d", channelKey, record.Sequence)
			}
			seenSequences[channelKey][record.Sequence] = true
		}
		switch record.Status {
		case deliveryStatusProcessing, deliveryStatusInterrupted, deliveryStatusFailed, deliveryStatusDelivered:
		default:
			return fmt.Errorf("bot recovery ledger record %q has invalid status %q", key, record.Status)
		}
	}
	for key, checkpoint := range state.Checkpoints {
		if strings.TrimSpace(checkpoint.Cursor) == "" || checkpoint.Sequence <= 0 || key != recoveryChannelKey(checkpoint.Source) {
			return fmt.Errorf("bot recovery ledger checkpoint %q is invalid", key)
		}
		sequence, ok := state.Sequences[key]
		if !ok || sequence < checkpoint.Sequence {
			return fmt.Errorf("bot recovery ledger checkpoint %q has no matching channel sequence", key)
		}
	}
	for key, sequence := range state.Sequences {
		if strings.TrimSpace(key) == "" || sequence <= 0 {
			return fmt.Errorf("bot recovery ledger channel sequence %q is invalid", key)
		}
		if checkpoint, ok := state.Checkpoints[key]; ok && sequence < checkpoint.Sequence {
			return fmt.Errorf("bot recovery ledger channel sequence %q precedes its checkpoint", key)
		}
	}
	return nil
}

func deliveryRecordKey(source SessionSource, messageID string) string {
	payload, _ := json.Marshal(struct {
		Source    SessionSource `json:"source"`
		MessageID string        `json:"message_id"`
	}{Source: source, MessageID: strings.TrimSpace(messageID)})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func inboundDeliveryClaims(msg InboundMessage) []deliveryClaim {
	claims := make([]deliveryClaim, 0, 1+len(msg.deliveryClaims))
	seen := make(map[string]bool, 1+len(msg.deliveryClaims))
	add := func(claim deliveryClaim) {
		claim.MessageID = strings.TrimSpace(claim.MessageID)
		if claim.MessageID == "" {
			return
		}
		key := deliveryRecordKey(claim.Source, claim.MessageID)
		if seen[key] {
			return
		}
		seen[key] = true
		claims = append(claims, claim)
	}
	add(deliveryClaim{Source: msg.Session(), MessageID: msg.MessageID})
	for _, claim := range msg.deliveryClaims {
		add(claim)
	}
	return claims
}

func mergeInboundDeliveryClaims(dst *InboundMessage, src InboundMessage) {
	if dst == nil {
		return
	}
	existing := make(map[string]bool, 1+len(dst.deliveryClaims))
	for _, claim := range inboundDeliveryClaims(*dst) {
		existing[deliveryRecordKey(claim.Source, claim.MessageID)] = true
	}
	for _, claim := range inboundDeliveryClaims(src) {
		key := deliveryRecordKey(claim.Source, claim.MessageID)
		if existing[key] {
			continue
		}
		existing[key] = true
		dst.deliveryClaims = append(dst.deliveryClaims, claim)
	}
}

func recoveryChannelKey(source SessionSource) string {
	return BuildSessionKey(recoveryChannelSource(source))
}

func recoveryChannelSource(source SessionSource) SessionSource {
	// The recovery cursor belongs to the remote delivery stream, not to the
	// per-user Agent session created inside a group or guild. ChatID (and an
	// optional thread) is therefore authoritative whenever present.
	if strings.TrimSpace(source.ChatID) != "" {
		source.UserID = ""
	}
	return source
}

func (l *deliveryLedger) claim(msg InboundMessage) (bool, error) {
	if l == nil {
		return true, nil
	}
	messageID := strings.TrimSpace(msg.MessageID)
	if messageID == "" {
		return false, errors.New("bot recovery claim requires message_id")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key := deliveryRecordKey(msg.Session(), messageID)
	if existing, ok := l.state.Records[key]; ok {
		if existing.Status == deliveryStatusDelivered || existing.Status == deliveryStatusProcessing {
			return false, nil
		}
	}
	candidate := cloneDeliveryLedgerState(l.state)
	if _, exists := candidate.Records[key]; !exists {
		if err := pruneDeliveryRecords(&candidate, l.recordLimit-1); err != nil {
			return false, err
		}
	}
	now := time.Now().UTC()
	record := candidate.Records[key]
	record.Source = msg.Session()
	record.MessageID = messageID
	incomingCursor := strings.TrimSpace(msg.RecoveryCursor)
	if record.Cursor != "" && incomingCursor != "" && record.Cursor != incomingCursor {
		return false, errors.New("bot recovery claim cursor changed for an existing message identity")
	}
	if incomingCursor != "" {
		record.Cursor = incomingCursor
	}
	if record.Cursor != "" && record.Sequence == 0 {
		channelKey := recoveryChannelKey(record.Source)
		if _, exists := candidate.Sequences[channelKey]; !exists && len(candidate.Sequences) >= l.recordLimit {
			return false, fmt.Errorf("bot recovery ledger channel limit %d reached", l.recordLimit)
		}
		candidate.Sequences[channelKey]++
		record.Sequence = candidate.Sequences[channelKey]
	}
	record.Status = deliveryStatusProcessing
	record.Attempts++
	record.UpdatedAt = now
	record.LastError = ""
	candidate.Records[key] = record
	if err := l.persistLocked(candidate); err != nil {
		return false, fmt.Errorf("bot recovery claim persist: %w", err)
	}
	l.state = candidate
	return true, nil
}

func (l *deliveryLedger) wasDelivered(msg InboundMessage) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	record, ok := l.state.Records[deliveryRecordKey(msg.Session(), strings.TrimSpace(msg.MessageID))]
	return ok && record.Status == deliveryStatusDelivered
}

func (l *deliveryLedger) fail(msg InboundMessage, cause error) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	changed := false
	for _, claim := range inboundDeliveryClaims(msg) {
		key := deliveryRecordKey(claim.Source, claim.MessageID)
		record, ok := candidate.Records[key]
		if !ok || record.Status == deliveryStatusDelivered {
			continue
		}
		record.Status = deliveryStatusFailed
		record.UpdatedAt = now
		record.LastError = "delivery failed; inspect local gateway logs"
		candidate.Records[key] = record
		changed = true
	}
	if !changed {
		return nil
	}
	if err := l.persistLocked(candidate); err != nil {
		return fmt.Errorf("bot recovery failure persist: %w", err)
	}
	l.state = candidate
	return nil
}

func (l *deliveryLedger) delivered(msg InboundMessage) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	channels := make(map[string]bool)
	changed := false
	for _, claim := range inboundDeliveryClaims(msg) {
		key := deliveryRecordKey(claim.Source, claim.MessageID)
		record, ok := candidate.Records[key]
		if !ok {
			return fmt.Errorf("bot recovery delivery has no durable claim for %s", key)
		}
		if record.Status == deliveryStatusDelivered {
			continue
		}
		record.Status = deliveryStatusDelivered
		record.UpdatedAt = now
		record.LastError = ""
		candidate.Records[key] = record
		if record.Cursor != "" {
			channels[recoveryChannelKey(record.Source)] = true
		}
		changed = true
	}
	if !changed {
		return nil
	}
	for channelKey := range channels {
		advanceRecoveryCheckpoint(&candidate, channelKey, now)
	}
	if err := l.persistLocked(candidate); err != nil {
		return fmt.Errorf("bot recovery delivery persist: %w", err)
	}
	l.state = candidate
	return nil
}

func (l *deliveryLedger) deliveredSession(sessionKey string) ([]deliveryClaim, error) {
	if l == nil || strings.TrimSpace(sessionKey) == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	channels := make(map[string]bool)
	changed := false
	var claims []deliveryClaim
	for key, record := range candidate.Records {
		if BuildSessionKey(record.Source) != sessionKey || record.Status == deliveryStatusDelivered {
			continue
		}
		record.Status = deliveryStatusDelivered
		record.UpdatedAt = now
		record.LastError = ""
		candidate.Records[key] = record
		claims = append(claims, deliveryClaim{Source: record.Source, MessageID: record.MessageID})
		if record.Cursor != "" {
			channels[recoveryChannelKey(record.Source)] = true
		}
		changed = true
	}
	if !changed {
		return nil, nil
	}
	for channelKey := range channels {
		advanceRecoveryCheckpoint(&candidate, channelKey, now)
	}
	if err := l.persistLocked(candidate); err != nil {
		return nil, fmt.Errorf("bot recovery canceled session persist: %w", err)
	}
	l.state = candidate
	return claims, nil
}

func (l *deliveryLedger) failSession(sessionKey string) ([]deliveryClaim, error) {
	if l == nil || strings.TrimSpace(sessionKey) == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	changed := false
	var claims []deliveryClaim
	for key, record := range candidate.Records {
		if BuildSessionKey(record.Source) != sessionKey || record.Status == deliveryStatusDelivered {
			continue
		}
		record.Status = deliveryStatusFailed
		record.UpdatedAt = now
		record.LastError = "delivery failed; inspect local gateway logs"
		candidate.Records[key] = record
		claims = append(claims, deliveryClaim{Source: record.Source, MessageID: record.MessageID})
		changed = true
	}
	if !changed {
		return nil, nil
	}
	if err := l.persistLocked(candidate); err != nil {
		return nil, fmt.Errorf("bot recovery canceled session failure persist: %w", err)
	}
	l.state = candidate
	return claims, nil
}

func (l *deliveryLedger) checkpoints(binding AdapterBinding) []RecoveryCheckpoint {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]RecoveryCheckpoint, 0, len(l.state.Checkpoints))
	for _, checkpoint := range l.state.Checkpoints {
		if checkpoint.Source.Platform != binding.Platform || checkpoint.Source.ConnectionID != binding.ID || checkpoint.Source.Domain != binding.Domain {
			continue
		}
		out = append(out, checkpoint)
	}
	sort.Slice(out, func(i, j int) bool {
		return recoveryChannelKey(out[i].Source) < recoveryChannelKey(out[j].Source)
	})
	return out
}

func (l *deliveryLedger) snapshot() DeliveryRecoverySnapshot {
	if l == nil {
		return DeliveryRecoverySnapshot{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snapshot := DeliveryRecoverySnapshot{
		Enabled:     true,
		Records:     len(l.state.Records),
		Checkpoints: len(l.state.Checkpoints),
	}
	for _, record := range l.state.Records {
		switch record.Status {
		case deliveryStatusProcessing:
			snapshot.Processing++
		case deliveryStatusInterrupted:
			snapshot.Interrupted++
		case deliveryStatusFailed:
			snapshot.Failed++
		case deliveryStatusDelivered:
			snapshot.Delivered++
		}
	}
	return snapshot
}

func (l *deliveryLedger) persistLocked(state deliveryLedgerState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	data = append(data, '\n')
	if int64(len(data)) > maxDeliveryLedgerBytes {
		return fmt.Errorf("encoded state exceeds %d bytes", maxDeliveryLedgerBytes)
	}
	return fileutil.AtomicWriteFile(l.path, data, 0o600)
}

func cloneDeliveryLedgerState(state deliveryLedgerState) deliveryLedgerState {
	cloned := deliveryLedgerState{
		Version:     state.Version,
		Records:     make(map[string]deliveryRecord, len(state.Records)),
		Checkpoints: make(map[string]RecoveryCheckpoint, len(state.Checkpoints)),
		Sequences:   make(map[string]int64, len(state.Sequences)),
	}
	for key, record := range state.Records {
		cloned.Records[key] = record
	}
	for key, checkpoint := range state.Checkpoints {
		cloned.Checkpoints[key] = checkpoint
	}
	for key, sequence := range state.Sequences {
		cloned.Sequences[key] = sequence
	}
	return cloned
}

func advanceRecoveryCheckpoint(state *deliveryLedgerState, channelKey string, now time.Time) {
	next := int64(1)
	if checkpoint, ok := state.Checkpoints[channelKey]; ok {
		next = checkpoint.Sequence + 1
	}
	bySequence := make(map[int64]deliveryRecord)
	for _, record := range state.Records {
		if record.Cursor != "" && recoveryChannelKey(record.Source) == channelKey {
			bySequence[record.Sequence] = record
		}
	}
	for {
		record, ok := bySequence[next]
		if !ok || record.Status != deliveryStatusDelivered {
			return
		}
		state.Checkpoints[channelKey] = RecoveryCheckpoint{
			Source:    recoveryChannelSource(record.Source),
			Cursor:    record.Cursor,
			Sequence:  record.Sequence,
			UpdatedAt: now,
		}
		next++
	}
}

func pruneDeliveryRecords(state *deliveryLedgerState, max int) error {
	if len(state.Records) <= max {
		return nil
	}
	type candidate struct {
		key       string
		updatedAt time.Time
	}
	removable := make([]candidate, 0, len(state.Records))
	for key, record := range state.Records {
		if record.Status == deliveryStatusProcessing {
			continue
		}
		if record.Cursor != "" {
			checkpoint := state.Checkpoints[recoveryChannelKey(record.Source)]
			if checkpoint.Sequence < record.Sequence {
				// A failed/interrupted record or a delivered record waiting behind
				// one is required to preserve contiguous cursor advancement.
				continue
			}
		}
		removable = append(removable, candidate{key: key, updatedAt: record.UpdatedAt})
	}
	sort.Slice(removable, func(i, j int) bool {
		if removable[i].updatedAt.Equal(removable[j].updatedAt) {
			return removable[i].key < removable[j].key
		}
		return removable[i].updatedAt.Before(removable[j].updatedAt)
	})
	for _, item := range removable {
		if len(state.Records) <= max {
			break
		}
		delete(state.Records, item.key)
	}
	if len(state.Records) > max {
		return fmt.Errorf("bot recovery ledger is full with %d in-flight claims", len(state.Records))
	}
	return nil
}
