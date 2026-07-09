// Package feedback defines the privacy-preserving local feedback ledger used by
// self-hosted Reames Agent nodes. It intentionally stores only sanitized,
// bounded records in the user's Reames Agent home; uploading or issue creation is
// a separate, explicit workflow.
package feedback

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"reames-agent/internal/config"
)

const (
	SchemaVersion = 1

	MaxMessageBytes  = 8 << 10
	MaxFieldBytes    = 512
	MaxMetadataPairs = 32
)

var (
	userPathSegment       = regexp.MustCompile(`(?i)([A-Z]:\\Users\\|/(?:home|Users)/)[^/\\:\s"']+`)
	emailPattern          = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`)
	secretKeyValuePattern = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|id[_-]?token|authorization|secret|password|passwd|pwd|token)\b\s*[:=]\s*(?:Bearer\s+)?['"]?[^'"\s,;]+['"]?`)
	bearerTokenPattern    = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{16,}`)
	explicitKeyPattern    = regexp.MustCompile(`\b(?:sk|rk)-(?:proj-)?[A-Za-z0-9_-]{16,}\b`)
	envIdentifierPattern  = regexp.MustCompile(`\b[A-Z][A-Z0-9_]*(?:API[_-]?KEY|ACCESS[_-]?KEY|PRIVATE[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD|PWD)[A-Z0-9_]*\b`)
	jwtPattern            = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
	longHexPattern        = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)
	longBase64Pattern     = regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	longBase64URLPattern  = regexp.MustCompile(`\b[A-Za-z0-9_-]{48,}\b`)
)

// ReportInput is the accepted feedback payload. Callers may submit richer
// frontend, CLI, Gateway, or worker reports, but all fields are sanitized and
// clipped before they are stored.
type ReportInput struct {
	Kind         string            `json:"kind"`
	Source       string            `json:"source,omitempty"`
	Label        string            `json:"label,omitempty"`
	Version      string            `json:"version,omitempty"`
	OS           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	Channel      string            `json:"channel,omitempty"`
	Message      string            `json:"message,omitempty"`
	ErrorType    string            `json:"errorType,omitempty"`
	ErrorMessage string            `json:"errorMessage,omitempty"`
	TopFrame     string            `json:"topFrame,omitempty"`
	OccurredAt   string            `json:"occurredAt,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Record is the durable JSONL form.
type Record struct {
	SchemaVersion int               `json:"schemaVersion"`
	ID            string            `json:"id"`
	Fingerprint   string            `json:"fingerprint"`
	ReceivedAt    string            `json:"receivedAt"`
	Kind          string            `json:"kind"`
	Source        string            `json:"source,omitempty"`
	Label         string            `json:"label,omitempty"`
	Version       string            `json:"version,omitempty"`
	OS            string            `json:"os,omitempty"`
	Arch          string            `json:"arch,omitempty"`
	Channel       string            `json:"channel,omitempty"`
	Message       string            `json:"message,omitempty"`
	ErrorType     string            `json:"errorType,omitempty"`
	ErrorMessage  string            `json:"errorMessage,omitempty"`
	TopFrame      string            `json:"topFrame,omitempty"`
	OccurredAt    string            `json:"occurredAt,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Store appends records and computes summaries over a JSONL ledger.
type Store struct {
	Path string
	mu   sync.Mutex
	now  func() time.Time
}

// Summary is the aggregate view used by cloud/server diagnostics.
type Summary struct {
	Path   string         `json:"path"`
	Total  int            `json:"total"`
	Groups []SummaryGroup `json:"groups"`
}

// SummaryGroup is a duplicate cluster keyed by Record.Fingerprint.
type SummaryGroup struct {
	Fingerprint string `json:"fingerprint"`
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	Count       int    `json:"count"`
	FirstSeen   string `json:"firstSeen,omitempty"`
	LastSeen    string `json:"lastSeen,omitempty"`
	LatestID    string `json:"latestId,omitempty"`
	TopFrame    string `json:"topFrame,omitempty"`
	ErrorType   string `json:"errorType,omitempty"`
}

// DefaultPath returns the self-hosted local feedback ledger path.
func DefaultPath() string {
	base := config.MemoryUserDir()
	if strings.TrimSpace(base) == "" {
		return ""
	}
	return filepath.Join(base, "feedback", "feedback.jsonl")
}

// NewStore creates a store at path. An empty path falls back to DefaultPath().
func NewStore(path string) *Store {
	if strings.TrimSpace(path) == "" {
		path = DefaultPath()
	}
	return &Store{Path: path, now: time.Now}
}

// Append sanitizes, fingerprints, and appends one feedback record.
func (s *Store) Append(in ReportInput) (Record, error) {
	if strings.TrimSpace(s.Path) == "" {
		return Record{}, errors.New("feedback store path is unavailable")
	}
	rec, err := Normalize(in, s.now())
	if err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return Record{}, err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Record{}, err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Summary reads the ledger and groups duplicate reports by fingerprint.
func (s *Store) Summary(limit int) (Summary, error) {
	return SummarizeFile(s.Path, limit)
}

// Normalize returns the sanitized durable record for in.
func Normalize(in ReportInput, received time.Time) (Record, error) {
	kind, ok := normalizeKind(in.Kind)
	if !ok {
		return Record{}, fmt.Errorf("feedback kind must be one of crash, exception, feedback, performance, bot, metrics")
	}
	message := sanitizeText(in.Message, MaxMessageBytes)
	errMsg := sanitizeText(in.ErrorMessage, MaxMessageBytes)
	if strings.TrimSpace(message) == "" && strings.TrimSpace(errMsg) == "" {
		return Record{}, errors.New("feedback message or errorMessage is required")
	}
	if received.IsZero() {
		received = time.Now()
	}
	rec := Record{
		SchemaVersion: SchemaVersion,
		ReceivedAt:    received.UTC().Format(time.RFC3339Nano),
		Kind:          kind,
		Source:        sanitizeField(in.Source),
		Label:         sanitizeField(in.Label),
		Version:       sanitizeField(in.Version),
		OS:            sanitizeField(in.OS),
		Arch:          sanitizeField(in.Arch),
		Channel:       sanitizeField(in.Channel),
		Message:       message,
		ErrorType:     sanitizeField(in.ErrorType),
		ErrorMessage:  errMsg,
		TopFrame:      sanitizeText(in.TopFrame, MaxFieldBytes),
		OccurredAt:    sanitizeField(in.OccurredAt),
		Metadata:      sanitizeMetadata(in.Metadata),
	}
	rec.Fingerprint = fingerprint(rec)
	rec.ID = recordID(rec)
	return rec, nil
}

// SummarizeFile groups records from path. Missing files are treated as an empty
// ledger so fresh cloud nodes can expose the endpoint before the first report.
func SummarizeFile(path string, limit int) (Summary, error) {
	out := Summary{Path: path}
	if strings.TrimSpace(path) == "" {
		return out, errors.New("feedback store path is unavailable")
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	defer f.Close()
	groups := map[string]*SummaryGroup{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out.Total++
		fp := strings.TrimSpace(rec.Fingerprint)
		if fp == "" {
			fp = fingerprint(rec)
		}
		g := groups[fp]
		if g == nil {
			g = &SummaryGroup{Fingerprint: fp, Kind: rec.Kind, Label: rec.Label, FirstSeen: rec.ReceivedAt}
			groups[fp] = g
		}
		g.Count++
		g.LastSeen = rec.ReceivedAt
		g.LatestID = rec.ID
		if g.TopFrame == "" {
			g.TopFrame = rec.TopFrame
		}
		if g.ErrorType == "" {
			g.ErrorType = rec.ErrorType
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	out.Groups = make([]SummaryGroup, 0, len(groups))
	for _, g := range groups {
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool {
		if out.Groups[i].Count != out.Groups[j].Count {
			return out.Groups[i].Count > out.Groups[j].Count
		}
		return out.Groups[i].LastSeen > out.Groups[j].LastSeen
	})
	if limit > 0 && len(out.Groups) > limit {
		out.Groups = out.Groups[:limit]
	}
	return out, nil
}

func normalizeKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "crash", "exception", "feedback", "performance", "bot", "metrics":
		return strings.ToLower(strings.TrimSpace(kind)), true
	default:
		return "", false
	}
}

func sanitizeField(s string) string {
	return sanitizeText(s, MaxFieldBytes)
}

func sanitizeText(s string, max int) string {
	s = strings.TrimSpace(s)
	s = userPathSegment.ReplaceAllString(s, "${1}_")
	s = emailPattern.ReplaceAllString(s, "[redacted-email]")
	s = bearerTokenPattern.ReplaceAllString(s, "Bearer [redacted]")
	s = secretKeyValuePattern.ReplaceAllString(s, "${1}=[redacted]")
	s = envIdentifierPattern.ReplaceAllString(s, "[redacted-env]")
	s = jwtPattern.ReplaceAllString(s, "[redacted-jwt]")
	s = explicitKeyPattern.ReplaceAllString(s, "[redacted-key]")
	s = longHexPattern.ReplaceAllString(s, "[redacted-hex]")
	s = longBase64Pattern.ReplaceAllString(s, "[redacted-token]")
	s = longBase64URLPattern.ReplaceAllString(s, "[redacted-token]")
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}

func sanitizeMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > MaxMetadataPairs {
		keys = keys[:MaxMetadataPairs]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		key := sanitizeField(k)
		if key == "" {
			continue
		}
		out[key] = sanitizeText(in[k], MaxFieldBytes)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func fingerprint(r Record) string {
	parts := []string{
		r.Kind,
		r.Source,
		r.Label,
		r.ErrorType,
		r.TopFrame,
		clipForFingerprint(r.ErrorMessage),
		clipForFingerprint(r.Message),
	}
	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(parts, "\x00"))))
	return hex.EncodeToString(sum[:12])
}

func recordID(r Record) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		r.Fingerprint,
		r.ReceivedAt,
		r.Kind,
		r.Message,
		r.ErrorMessage,
	}, "\x00")))
	return hex.EncodeToString(sum[:12])
}

func clipForFingerprint(s string) string {
	if len(s) > 512 {
		return s[:512]
	}
	return s
}
