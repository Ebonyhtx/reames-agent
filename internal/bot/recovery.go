package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/fileutil"
)

const (
	deliveryLedgerVersion             = 2
	legacyDeliveryLedgerVersion       = 1
	defaultDeliveryRecordLimit        = 4096
	defaultRecoveryScanLimit          = 200
	maxDeliveryLedgerBytes      int64 = 4 << 20
	maxOutboundObligationBytes        = 1 << 20
	maxOutboundObligationChunks       = 512

	deliveryStatusProcessing  = "processing"
	deliveryStatusInterrupted = "interrupted"
	deliveryStatusFailed      = "failed"
	deliveryStatusDelivered   = "delivered"

	obligationStatusPending    = "pending"
	obligationStatusAttempting = "attempting"
	obligationStatusFailed     = "failed"
)

const recoveredReplyMarker = "♻️ 这是网关重启后恢复的答复；上次发送结果未能确认，因此可能与已收到的内容重复。\n\n"

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
	Enabled             bool `json:"enabled"`
	Records             int  `json:"records"`
	Processing          int  `json:"processing"`
	Interrupted         int  `json:"interrupted"`
	Failed              int  `json:"failed"`
	Delivered           int  `json:"delivered"`
	Checkpoints         int  `json:"checkpoints"`
	Obligations         int  `json:"obligations"`
	ObligationPending   int  `json:"obligation_pending"`
	ObligationAmbiguous int  `json:"obligation_ambiguous"`
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
	Source    SessionSource `json:"source"`
	MessageID string        `json:"message_id"`
}

type outboundTarget struct {
	Platform     Platform `json:"platform"`
	ConnectionID string   `json:"connection_id"`
	Domain       string   `json:"domain,omitempty"`
	ChatID       string   `json:"chat_id"`
	ChatType     ChatType `json:"chat_type,omitempty"`
	ReplyToMsgID string   `json:"reply_to_msg_id,omitempty"`
}

type outboundObligation struct {
	ID        string          `json:"id"`
	Source    SessionSource   `json:"source"`
	MessageID string          `json:"message_id"`
	Claims    []deliveryClaim `json:"claims"`
	Target    outboundTarget  `json:"target"`
	Chunks    []string        `json:"chunks"`
	NextChunk int             `json:"next_chunk"`
	Status    string          `json:"status"`
	Attempts  int             `json:"attempts"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	LastError string          `json:"last_error,omitempty"`
}

type outboundObligationItem struct {
	Key        string
	Obligation outboundObligation
}

type deliveryLedgerState struct {
	Version     int                           `json:"version"`
	Records     map[string]deliveryRecord     `json:"records"`
	Checkpoints map[string]RecoveryCheckpoint `json:"checkpoints"`
	Sequences   map[string]int64              `json:"sequences"`
	Obligations map[string]outboundObligation `json:"obligations"`
}

type deliveryLedger struct {
	mu          sync.Mutex
	path        string
	recordLimit int
	state       deliveryLedgerState
	releaseLock func()
	writeFile   func(string, []byte, os.FileMode) error
}

func openDeliveryLedger(path string, recordLimit int) (*deliveryLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	if recordLimit <= 0 {
		recordLimit = defaultDeliveryRecordLimit
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("bot recovery ledger create parent: %w", err)
	}
	releaseLock, err := lockDeliveryLedgerFile(path)
	if err != nil {
		return nil, fmt.Errorf("bot recovery ledger is already in use: %w", err)
	}
	ledger := &deliveryLedger{
		path:        path,
		recordLimit: recordLimit,
		releaseLock: releaseLock,
		writeFile:   fileutil.AtomicWriteFile,
		state: deliveryLedgerState{
			Version:     deliveryLedgerVersion,
			Records:     make(map[string]deliveryRecord),
			Checkpoints: make(map[string]RecoveryCheckpoint),
			Sequences:   make(map[string]int64),
			Obligations: make(map[string]outboundObligation),
		},
	}
	ok := false
	defer func() {
		if !ok {
			ledger.close()
		}
	}()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		ok = true
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
	changed := false
	if state.Version == legacyDeliveryLedgerVersion {
		if len(state.Obligations) != 0 {
			return nil, errors.New("bot recovery legacy ledger unexpectedly contains outbound obligations")
		}
		state.Version = deliveryLedgerVersion
		state.Obligations = make(map[string]outboundObligation)
		changed = true
	}
	if err := validateDeliveryLedgerState(state, recordLimit); err != nil {
		return nil, err
	}
	ledger.state = state
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
	ok = true
	return ledger, nil
}

func (l *deliveryLedger) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	release := l.releaseLock
	l.releaseLock = nil
	l.mu.Unlock()
	if release != nil {
		release()
	}
}

func validateDeliveryLedgerState(state deliveryLedgerState, recordLimit int) error {
	if state.Version != deliveryLedgerVersion {
		return fmt.Errorf("bot recovery ledger version %d is unsupported", state.Version)
	}
	if state.Records == nil || state.Checkpoints == nil || state.Sequences == nil || state.Obligations == nil {
		return errors.New("bot recovery ledger is missing required maps")
	}
	if len(state.Records) > recordLimit {
		return fmt.Errorf("bot recovery ledger contains %d records; limit is %d", len(state.Records), recordLimit)
	}
	if len(state.Sequences) > recordLimit || len(state.Checkpoints) > recordLimit {
		return fmt.Errorf("bot recovery ledger contains too many channel cursors; limit is %d", recordLimit)
	}
	if len(state.Obligations) > recordLimit {
		return fmt.Errorf("bot recovery ledger contains %d outbound obligations; limit is %d", len(state.Obligations), recordLimit)
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
	claimedByObligation := make(map[string]string)
	for key, obligation := range state.Obligations {
		if strings.TrimSpace(obligation.MessageID) == "" || key != deliveryRecordKey(obligation.Source, obligation.MessageID) {
			return fmt.Errorf("bot recovery outbound obligation %q identity mismatch", key)
		}
		if obligation.ID != computeOutboundObligationID(obligation) {
			return fmt.Errorf("bot recovery outbound obligation %q content identity mismatch", key)
		}
		if obligation.Target.Platform != obligation.Source.Platform || obligation.Target.ConnectionID != obligation.Source.ConnectionID || obligation.Target.Domain != obligation.Source.Domain || obligation.Target.ChatID != obligation.Source.ChatID || obligation.Target.ChatType != obligation.Source.ChatType {
			return fmt.Errorf("bot recovery outbound obligation %q target identity mismatch", key)
		}
		if len(obligation.Claims) == 0 || len(obligation.Claims) > recordLimit {
			return fmt.Errorf("bot recovery outbound obligation %q has invalid claim count", key)
		}
		primary := obligation.Claims[0]
		if primary.Source != obligation.Source || primary.MessageID != obligation.MessageID {
			return fmt.Errorf("bot recovery outbound obligation %q primary claim mismatch", key)
		}
		if len(obligation.Chunks) == 0 || len(obligation.Chunks) > maxOutboundObligationChunks || obligation.NextChunk < 0 || obligation.NextChunk >= len(obligation.Chunks) {
			return fmt.Errorf("bot recovery outbound obligation %q has invalid chunk progress", key)
		}
		if outboundObligationBytes(obligation.Chunks) > maxOutboundObligationBytes {
			return fmt.Errorf("bot recovery outbound obligation %q exceeds %d bytes", key, maxOutboundObligationBytes)
		}
		for _, chunk := range obligation.Chunks {
			if strings.TrimSpace(chunk) == "" {
				return fmt.Errorf("bot recovery outbound obligation %q contains an empty chunk", key)
			}
		}
		if obligation.CreatedAt.IsZero() || obligation.UpdatedAt.IsZero() || obligation.UpdatedAt.Before(obligation.CreatedAt) || obligation.Attempts < 0 || len(obligation.LastError) > 256 {
			return fmt.Errorf("bot recovery outbound obligation %q has invalid attempt metadata", key)
		}
		switch obligation.Status {
		case obligationStatusPending:
		case obligationStatusAttempting, obligationStatusFailed:
			if obligation.Attempts == 0 {
				return fmt.Errorf("bot recovery outbound obligation %q has no recorded attempt", key)
			}
		default:
			return fmt.Errorf("bot recovery outbound obligation %q has invalid status %q", key, obligation.Status)
		}
		for _, claim := range obligation.Claims {
			claimKey := deliveryRecordKey(claim.Source, claim.MessageID)
			if owner, exists := claimedByObligation[claimKey]; exists && owner != key {
				return fmt.Errorf("bot recovery claim %q belongs to multiple outbound obligations", claimKey)
			}
			claimedByObligation[claimKey] = key
			record, exists := state.Records[claimKey]
			if !exists || record.Source != claim.Source || record.MessageID != claim.MessageID || record.Status == deliveryStatusDelivered {
				return fmt.Errorf("bot recovery outbound obligation %q references an invalid claim %q", key, claimKey)
			}
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

func computeOutboundObligationID(obligation outboundObligation) string {
	payload, _ := json.Marshal(struct {
		Source    SessionSource   `json:"source"`
		MessageID string          `json:"message_id"`
		Claims    []deliveryClaim `json:"claims"`
		Target    outboundTarget  `json:"target"`
		Chunks    []string        `json:"chunks"`
	}{
		Source:    obligation.Source,
		MessageID: strings.TrimSpace(obligation.MessageID),
		Claims:    obligation.Claims,
		Target:    obligation.Target,
		Chunks:    obligation.Chunks,
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func outboundObligationBytes(chunks []string) int {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	return total
}

func cloneOutboundObligation(obligation outboundObligation) outboundObligation {
	obligation.Claims = append([]deliveryClaim(nil), obligation.Claims...)
	obligation.Chunks = append([]string(nil), obligation.Chunks...)
	return obligation
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
	if _, _, ok := findOutboundObligationForClaim(l.state, msg.Session(), messageID); ok {
		return false, nil
	}
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

func (l *deliveryLedger) prepareOutboundObligation(msg InboundMessage, messages []OutboundMessage) (outboundObligation, error) {
	if l == nil {
		return outboundObligation{}, errors.New("bot recovery ledger is disabled")
	}
	claims := inboundDeliveryClaims(msg)
	if len(claims) == 0 {
		return outboundObligation{}, errors.New("outbound obligation requires at least one durable inbound claim")
	}
	target, chunks, err := normalizeOutboundObligationMessages(msg, messages)
	if err != nil {
		return outboundObligation{}, err
	}
	now := time.Now().UTC()
	obligation := outboundObligation{
		Source:    msg.Session(),
		MessageID: strings.TrimSpace(msg.MessageID),
		Claims:    append([]deliveryClaim(nil), claims...),
		Target:    target,
		Chunks:    chunks,
		Status:    obligationStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	obligation.ID = computeOutboundObligationID(obligation)
	key := deliveryRecordKey(obligation.Source, obligation.MessageID)

	l.mu.Lock()
	defer l.mu.Unlock()
	if existing, ok := l.state.Obligations[key]; ok {
		if existing.ID != obligation.ID {
			return outboundObligation{}, errors.New("outbound obligation content changed for an existing inbound identity")
		}
		return cloneOutboundObligation(existing), nil
	}
	if len(l.state.Obligations) >= l.recordLimit {
		return outboundObligation{}, fmt.Errorf("bot recovery outbound obligation limit %d reached", l.recordLimit)
	}
	for _, claim := range claims {
		claimKey := deliveryRecordKey(claim.Source, claim.MessageID)
		record, ok := l.state.Records[claimKey]
		if !ok || record.Status == deliveryStatusDelivered {
			return outboundObligation{}, fmt.Errorf("outbound obligation has no unsettled durable claim for %s", claimKey)
		}
		if ownerKey, _, exists := findOutboundObligationForClaim(l.state, claim.Source, claim.MessageID); exists && ownerKey != key {
			return outboundObligation{}, fmt.Errorf("outbound obligation claim %s is already owned", claimKey)
		}
	}
	candidate := cloneDeliveryLedgerState(l.state)
	candidate.Obligations[key] = obligation
	if err := validateDeliveryLedgerState(candidate, l.recordLimit); err != nil {
		return outboundObligation{}, err
	}
	if err := l.persistLocked(candidate); err != nil {
		return outboundObligation{}, fmt.Errorf("persist outbound obligation before send: %w", err)
	}
	l.state = candidate
	return cloneOutboundObligation(obligation), nil
}

func normalizeOutboundObligationMessages(msg InboundMessage, messages []OutboundMessage) (outboundTarget, []string, error) {
	if len(messages) == 0 || len(messages) > maxOutboundObligationChunks {
		return outboundTarget{}, nil, fmt.Errorf("outbound obligation chunk count must be between 1 and %d", maxOutboundObligationChunks)
	}
	target := outboundTarget{
		Platform:     msg.Platform,
		ConnectionID: strings.TrimSpace(messages[0].ConnectionID),
		Domain:       strings.TrimSpace(messages[0].Domain),
		ChatID:       strings.TrimSpace(messages[0].ChatID),
		ChatType:     messages[0].ChatType,
		ReplyToMsgID: strings.TrimSpace(messages[0].ReplyToMsgID),
	}
	if target.ConnectionID == "" {
		target.ConnectionID = strings.TrimSpace(msg.ConnectionID)
	}
	if target.Domain == "" {
		target.Domain = strings.TrimSpace(msg.Domain)
	}
	if target.ChatID == "" || target.ChatID != strings.TrimSpace(msg.ChatID) || target.ConnectionID != strings.TrimSpace(msg.ConnectionID) || target.Domain != strings.TrimSpace(msg.Domain) || target.ChatType != msg.ChatType {
		return outboundTarget{}, nil, errors.New("outbound obligation target does not match the durable inbound identity")
	}
	chunks := make([]string, 0, len(messages))
	for _, message := range messages {
		if len(message.MediaURLs) != 0 || message.Keyboard != nil || message.Card != nil {
			return outboundTarget{}, nil, errors.New("outbound obligation supports final text chunks only")
		}
		if strings.TrimSpace(message.ConnectionID) != target.ConnectionID || strings.TrimSpace(message.Domain) != target.Domain || strings.TrimSpace(message.ChatID) != target.ChatID || message.ChatType != target.ChatType || strings.TrimSpace(message.ReplyToMsgID) != target.ReplyToMsgID {
			return outboundTarget{}, nil, errors.New("outbound obligation chunks do not share one target")
		}
		text := strings.TrimSpace(message.Text)
		if text == "" {
			return outboundTarget{}, nil, errors.New("outbound obligation contains an empty final chunk")
		}
		chunks = append(chunks, text)
	}
	if outboundObligationBytes(chunks) > maxOutboundObligationBytes {
		return outboundTarget{}, nil, fmt.Errorf("outbound obligation exceeds %d bytes", maxOutboundObligationBytes)
	}
	return target, chunks, nil
}

func findOutboundObligationForClaim(state deliveryLedgerState, source SessionSource, messageID string) (string, outboundObligation, bool) {
	want := deliveryRecordKey(source, strings.TrimSpace(messageID))
	for key, obligation := range state.Obligations {
		for _, claim := range obligation.Claims {
			if deliveryRecordKey(claim.Source, claim.MessageID) == want {
				return key, obligation, true
			}
		}
	}
	return "", outboundObligation{}, false
}

func (l *deliveryLedger) outboundObligationFor(msg InboundMessage) (string, outboundObligation, bool) {
	if l == nil {
		return "", outboundObligation{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	key, obligation, ok := findOutboundObligationForClaim(l.state, msg.Session(), msg.MessageID)
	return key, cloneOutboundObligation(obligation), ok
}

func (l *deliveryLedger) outboundObligation(key string) (outboundObligation, bool) {
	if l == nil {
		return outboundObligation{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	obligation, ok := l.state.Obligations[strings.TrimSpace(key)]
	return cloneOutboundObligation(obligation), ok
}

func (l *deliveryLedger) pendingOutboundObligations(limit int) []outboundObligationItem {
	if l == nil || limit <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	items := make([]outboundObligationItem, 0, len(l.state.Obligations))
	for key, obligation := range l.state.Obligations {
		items = append(items, outboundObligationItem{Key: key, Obligation: cloneOutboundObligation(obligation)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Obligation.CreatedAt.Equal(items[j].Obligation.CreatedAt) {
			return items[i].Key < items[j].Key
		}
		return items[i].Obligation.CreatedAt.Before(items[j].Obligation.CreatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (l *deliveryLedger) beginOutboundAttempt(key string) (outboundObligation, error) {
	if l == nil {
		return outboundObligation{}, errors.New("bot recovery ledger is disabled")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	obligation, ok := candidate.Obligations[strings.TrimSpace(key)]
	if !ok {
		return outboundObligation{}, errors.New("outbound obligation no longer exists")
	}
	now := time.Now().UTC()
	obligation.Status = obligationStatusAttempting
	obligation.Attempts++
	obligation.UpdatedAt = now
	obligation.LastError = ""
	candidate.Obligations[key] = obligation
	for _, claim := range obligation.Claims {
		claimKey := deliveryRecordKey(claim.Source, claim.MessageID)
		record, exists := candidate.Records[claimKey]
		if !exists || record.Status == deliveryStatusDelivered {
			return outboundObligation{}, fmt.Errorf("outbound obligation claim %s is not retryable", claimKey)
		}
		record.Status = deliveryStatusProcessing
		record.UpdatedAt = now
		record.LastError = ""
		candidate.Records[claimKey] = record
	}
	if err := validateDeliveryLedgerState(candidate, l.recordLimit); err != nil {
		return outboundObligation{}, err
	}
	if err := l.persistLocked(candidate); err != nil {
		return outboundObligation{}, fmt.Errorf("persist outbound attempt before send: %w", err)
	}
	l.state = candidate
	return cloneOutboundObligation(obligation), nil
}

func (l *deliveryLedger) failOutboundAttempt(key string) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	obligation, ok := candidate.Obligations[strings.TrimSpace(key)]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	obligation.Status = obligationStatusFailed
	obligation.UpdatedAt = now
	obligation.LastError = "platform delivery was not confirmed; retry will carry a duplicate warning"
	candidate.Obligations[key] = obligation
	for _, claim := range obligation.Claims {
		claimKey := deliveryRecordKey(claim.Source, claim.MessageID)
		record, exists := candidate.Records[claimKey]
		if !exists || record.Status == deliveryStatusDelivered {
			continue
		}
		record.Status = deliveryStatusFailed
		record.UpdatedAt = now
		record.LastError = "delivery failed; inspect local gateway logs"
		candidate.Records[claimKey] = record
	}
	if err := validateDeliveryLedgerState(candidate, l.recordLimit); err != nil {
		return err
	}
	if err := l.persistLocked(candidate); err != nil {
		return fmt.Errorf("persist outbound delivery failure: %w", err)
	}
	l.state = candidate
	return nil
}

func (l *deliveryLedger) acknowledgeOutboundChunk(key string) (outboundObligation, bool, []deliveryClaim, error) {
	if l == nil {
		return outboundObligation{}, false, nil, errors.New("bot recovery ledger is disabled")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	obligation, ok := candidate.Obligations[strings.TrimSpace(key)]
	if !ok {
		return outboundObligation{}, false, nil, errors.New("outbound obligation no longer exists")
	}
	if obligation.Status != obligationStatusAttempting {
		return outboundObligation{}, false, nil, fmt.Errorf("outbound obligation is %s, not attempting", obligation.Status)
	}
	now := time.Now().UTC()
	if obligation.NextChunk+1 < len(obligation.Chunks) {
		obligation.NextChunk++
		obligation.Status = obligationStatusPending
		obligation.UpdatedAt = now
		obligation.LastError = ""
		candidate.Obligations[key] = obligation
		if err := validateDeliveryLedgerState(candidate, l.recordLimit); err != nil {
			return outboundObligation{}, false, nil, err
		}
		if err := l.persistLocked(candidate); err != nil {
			return outboundObligation{}, false, nil, fmt.Errorf("persist outbound chunk acknowledgement: %w", err)
		}
		l.state = candidate
		return cloneOutboundObligation(obligation), false, nil, nil
	}

	channels := make(map[string]bool)
	claims := append([]deliveryClaim(nil), obligation.Claims...)
	for _, claim := range claims {
		claimKey := deliveryRecordKey(claim.Source, claim.MessageID)
		record, exists := candidate.Records[claimKey]
		if !exists {
			return outboundObligation{}, false, nil, fmt.Errorf("outbound obligation completion has no claim %s", claimKey)
		}
		record.Status = deliveryStatusDelivered
		record.UpdatedAt = now
		record.LastError = ""
		candidate.Records[claimKey] = record
		if record.Cursor != "" {
			channels[recoveryChannelKey(record.Source)] = true
		}
	}
	delete(candidate.Obligations, key)
	for channelKey := range channels {
		advanceRecoveryCheckpoint(&candidate, channelKey, now)
	}
	if err := validateDeliveryLedgerState(candidate, l.recordLimit); err != nil {
		return outboundObligation{}, false, nil, err
	}
	if err := l.persistLocked(candidate); err != nil {
		return outboundObligation{}, false, nil, fmt.Errorf("commit outbound delivery and inbound cursor: %w", err)
	}
	l.state = candidate
	return outboundObligation{}, true, claims, nil
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
		if obligationKey, _, ok := findOutboundObligationForClaim(candidate, claim.Source, claim.MessageID); ok {
			delete(candidate.Obligations, obligationKey)
		}
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

func (l *deliveryLedger) deliveredSession(sessionKey string, excluded []deliveryClaim) ([]deliveryClaim, error) {
	if l == nil || strings.TrimSpace(sessionKey) == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	channels := make(map[string]bool)
	excludedKeys := deliveryClaimKeySet(excluded)
	deliveredKeys := make(map[string]bool)
	changed := false
	var claims []deliveryClaim
	for key, record := range candidate.Records {
		if BuildSessionKey(record.Source) != sessionKey || record.Status == deliveryStatusDelivered || excludedKeys[key] {
			continue
		}
		record.Status = deliveryStatusDelivered
		record.UpdatedAt = now
		record.LastError = ""
		candidate.Records[key] = record
		claims = append(claims, deliveryClaim{Source: record.Source, MessageID: record.MessageID})
		deliveredKeys[key] = true
		if record.Cursor != "" {
			channels[recoveryChannelKey(record.Source)] = true
		}
		changed = true
	}
	for obligationKey, obligation := range candidate.Obligations {
		remove := false
		for _, claim := range obligation.Claims {
			if deliveredKeys[deliveryRecordKey(claim.Source, claim.MessageID)] {
				remove = true
				break
			}
		}
		if remove {
			delete(candidate.Obligations, obligationKey)
			changed = true
		}
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

func (l *deliveryLedger) failSession(sessionKey string, excluded []deliveryClaim) ([]deliveryClaim, error) {
	if l == nil || strings.TrimSpace(sessionKey) == "" {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	candidate := cloneDeliveryLedgerState(l.state)
	now := time.Now().UTC()
	excludedKeys := deliveryClaimKeySet(excluded)
	changed := false
	var claims []deliveryClaim
	for key, record := range candidate.Records {
		if BuildSessionKey(record.Source) != sessionKey || record.Status == deliveryStatusDelivered || excludedKeys[key] {
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

func deliveryClaimKeySet(claims []deliveryClaim) map[string]bool {
	keys := make(map[string]bool, len(claims))
	for _, claim := range claims {
		if strings.TrimSpace(claim.MessageID) == "" {
			continue
		}
		keys[deliveryRecordKey(claim.Source, claim.MessageID)] = true
	}
	return keys
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
		Obligations: len(l.state.Obligations),
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
	for _, obligation := range l.state.Obligations {
		switch obligation.Status {
		case obligationStatusPending:
			snapshot.ObligationPending++
		case obligationStatusAttempting, obligationStatusFailed:
			snapshot.ObligationAmbiguous++
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
	writeFile := l.writeFile
	if writeFile == nil {
		writeFile = fileutil.AtomicWriteFile
	}
	return writeFile(l.path, data, 0o600)
}

func cloneDeliveryLedgerState(state deliveryLedgerState) deliveryLedgerState {
	cloned := deliveryLedgerState{
		Version:     state.Version,
		Records:     make(map[string]deliveryRecord, len(state.Records)),
		Checkpoints: make(map[string]RecoveryCheckpoint, len(state.Checkpoints)),
		Sequences:   make(map[string]int64, len(state.Sequences)),
		Obligations: make(map[string]outboundObligation, len(state.Obligations)),
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
	for key, obligation := range state.Obligations {
		cloned.Obligations[key] = cloneOutboundObligation(obligation)
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
	protected := make(map[string]bool)
	for _, obligation := range state.Obligations {
		for _, claim := range obligation.Claims {
			protected[deliveryRecordKey(claim.Source, claim.MessageID)] = true
		}
	}
	for key, record := range state.Records {
		if record.Status == deliveryStatusProcessing || protected[key] {
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
