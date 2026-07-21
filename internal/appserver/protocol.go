// Package appserver implements the local Codex-class App-Server transport.
// It projects Reames Agent's existing control.Controller over a strict JSONL
// request/event protocol; it does not own an agent loop or persistence format.
package appserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
	ErrOverloaded     = -32001
)

// RPCError is returned by handlers when a stable JSON-RPC error code matters.
// App-Server omits the jsonrpc header but retains JSON-RPC request/error shapes.
type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string { return e.Message }

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type InitializeCapabilities struct {
	ExperimentalAPI           bool     `json:"experimentalApi,omitempty"`
	OptOutNotificationMethods []string `json:"optOutNotificationMethods,omitempty"`
}

type InitializeParams struct {
	ClientInfo   ClientInfo              `json:"clientInfo"`
	Capabilities *InitializeCapabilities `json:"capabilities,omitempty"`
}

type InitializeResponse struct {
	UserAgent      string `json:"userAgent"`
	CodexHome      string `json:"codexHome"`
	PlatformFamily string `json:"platformFamily"`
	PlatformOS     string `json:"platformOs"`
}

// ThreadStartParams intentionally contains only settings Reames can honor
// end-to-end. strictDecode rejects every other official or unknown setting so a
// client cannot mistake an ignored sandbox/model/runtime override for success.
type ThreadStartParams struct {
	Model       string `json:"model,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Ephemeral   bool   `json:"ephemeral,omitempty"`
	HistoryMode string `json:"historyMode,omitempty"`
}

type ThreadResumeParams struct {
	ThreadID string `json:"threadId"`
	Model    string `json:"model,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
}

type ThreadStartResponse struct {
	Thread                Thread        `json:"thread"`
	Model                 string        `json:"model"`
	ModelProvider         string        `json:"modelProvider"`
	Cwd                   string        `json:"cwd"`
	RuntimeWorkspaceRoots []string      `json:"runtimeWorkspaceRoots"`
	InstructionSources    []string      `json:"instructionSources"`
	ApprovalPolicy        string        `json:"approvalPolicy"`
	ApprovalsReviewer     string        `json:"approvalsReviewer"`
	Sandbox               SandboxPolicy `json:"sandbox"`
}

type ThreadResumeResponse = ThreadStartResponse

type SandboxPolicy struct {
	Type                string   `json:"type"`
	WritableRoots       []string `json:"writableRoots,omitempty"`
	NetworkAccess       bool     `json:"networkAccess,omitempty"`
	ExcludeTmpdirEnvVar bool     `json:"excludeTmpdirEnvVar,omitempty"`
	ExcludeSlashTmp     bool     `json:"excludeSlashTmp,omitempty"`
}

type ThreadStatus struct {
	Type        string   `json:"type"`
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

type Thread struct {
	ID                   string       `json:"id"`
	SessionID            string       `json:"sessionId"`
	ForkedFromID         *string      `json:"forkedFromId"`
	ParentThreadID       *string      `json:"parentThreadId"`
	Preview              string       `json:"preview"`
	Ephemeral            bool         `json:"ephemeral"`
	HistoryMode          string       `json:"historyMode"`
	ModelProvider        string       `json:"modelProvider"`
	CreatedAt            int64        `json:"createdAt"`
	UpdatedAt            int64        `json:"updatedAt"`
	RecencyAt            *int64       `json:"recencyAt"`
	Status               ThreadStatus `json:"status"`
	Path                 *string      `json:"path"`
	Cwd                  string       `json:"cwd"`
	CLIVersion           string       `json:"cliVersion"`
	Source               string       `json:"source"`
	CanAcceptDirectInput *bool        `json:"canAcceptDirectInput,omitempty"`
	Name                 *string      `json:"name"`
	Turns                []Turn       `json:"turns"`
}

type Turn struct {
	ID          string       `json:"id"`
	Items       []ThreadItem `json:"items"`
	ItemsView   string       `json:"itemsView"`
	Status      string       `json:"status"`
	Error       *TurnError   `json:"error"`
	StartedAt   *int64       `json:"startedAt"`
	CompletedAt *int64       `json:"completedAt"`
	DurationMs  *int64       `json:"durationMs"`
}

type TurnError struct {
	Message string `json:"message"`
}

type ThreadItem map[string]any

type ThreadListParams struct {
	Cursor        string   `json:"cursor,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	SortKey       string   `json:"sortKey,omitempty"`
	SortDirection string   `json:"sortDirection,omitempty"`
	Cwd           []string `json:"cwd,omitempty"`
	SearchTerm    string   `json:"searchTerm,omitempty"`
}

// UnmarshalJSON accepts Codex's cwd string-or-array filter without weakening
// strict top-level field validation.
func (p *ThreadListParams) UnmarshalJSON(raw []byte) error {
	type alias ThreadListParams
	var wire struct {
		Cursor        string          `json:"cursor,omitempty"`
		Limit         int             `json:"limit,omitempty"`
		SortKey       string          `json:"sortKey,omitempty"`
		SortDirection string          `json:"sortDirection,omitempty"`
		Cwd           json.RawMessage `json:"cwd,omitempty"`
		SearchTerm    string          `json:"searchTerm,omitempty"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	*p = ThreadListParams{Cursor: wire.Cursor, Limit: wire.Limit, SortKey: wire.SortKey, SortDirection: wire.SortDirection, SearchTerm: wire.SearchTerm}
	if len(wire.Cwd) == 0 || string(wire.Cwd) == "null" {
		return nil
	}
	var one string
	if json.Unmarshal(wire.Cwd, &one) == nil {
		p.Cwd = []string{one}
		return nil
	}
	if err := json.Unmarshal(wire.Cwd, &p.Cwd); err != nil {
		return fmt.Errorf("cwd must be a string or array of strings")
	}
	return nil
}

type ThreadListResponse struct {
	Data            []Thread `json:"data"`
	NextCursor      *string  `json:"nextCursor"`
	BackwardsCursor *string  `json:"backwardsCursor"`
}

type ThreadReadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns,omitempty"`
}

type ThreadReadResponse struct {
	Thread Thread `json:"thread"`
}

type ThreadLoadedListParams struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type ThreadLoadedListResponse struct {
	Data       []string `json:"data"`
	NextCursor *string  `json:"nextCursor"`
}

type ThreadNameSetParams struct {
	ThreadID string `json:"threadId"`
	Name     string `json:"name"`
}

type ThreadUnsubscribeParams struct {
	ThreadID string `json:"threadId"`
}

type ThreadUnsubscribeResponse struct {
	Status string `json:"status"`
}

type UserInput struct {
	Type         string            `json:"type"`
	Text         string            `json:"text,omitempty"`
	TextElements []json.RawMessage `json:"textElements,omitempty"`
	URL          string            `json:"url,omitempty"`
	Path         string            `json:"path,omitempty"`
	Name         string            `json:"name,omitempty"`
}

type TurnStartParams struct {
	ThreadID            string      `json:"threadId"`
	ClientUserMessageID string      `json:"clientUserMessageId,omitempty"`
	Input               []UserInput `json:"input"`
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

type TurnSteerParams struct {
	ThreadID            string      `json:"threadId"`
	ExpectedTurnID      string      `json:"expectedTurnId"`
	ClientUserMessageID string      `json:"clientUserMessageId,omitempty"`
	Input               []UserInput `json:"input"`
}

type TurnSteerResponse struct {
	TurnID string `json:"turnId"`
}

type TurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

type TurnInterruptResponse struct{}

type ApprovalRequestParams struct {
	ThreadID           string         `json:"threadId"`
	TurnID             string         `json:"turnId"`
	ItemID             string         `json:"itemId"`
	Reason             string         `json:"reason,omitempty"`
	Command            string         `json:"command,omitempty"`
	Cwd                string         `json:"cwd,omitempty"`
	AvailableDecisions []string       `json:"availableDecisions"`
	ReamesPlan         map[string]any `json:"reamesPlan,omitempty"`
}

type ApprovalResponse struct {
	Decision string `json:"decision"`
}

type RequestUserInputParams struct {
	ThreadID  string                     `json:"threadId"`
	TurnID    string                     `json:"turnId"`
	ItemID    string                     `json:"itemId"`
	Questions []RequestUserInputQuestion `json:"questions"`
}

type RequestUserInputQuestion struct {
	ID       string                   `json:"id"`
	Header   string                   `json:"header"`
	Question string                   `json:"question"`
	IsOther  bool                     `json:"isOther"`
	IsSecret bool                     `json:"isSecret"`
	Options  []RequestUserInputOption `json:"options,omitempty"`
}

type RequestUserInputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type RequestUserInputAnswer struct {
	Answers []string `json:"answers"`
}

type RequestUserInputResponse struct {
	Answers map[string]RequestUserInputAnswer `json:"answers"`
}

func strictDecode(raw json.RawMessage, dst any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateHistoryMode(mode string) error {
	switch strings.TrimSpace(mode) {
	case "", "legacy":
		return nil
	case "paginated":
		return fmt.Errorf("historyMode %q is not supported by this App-Server version", mode)
	default:
		return fmt.Errorf("unknown historyMode %q", mode)
	}
}

func flattenTextInput(input []UserInput) (string, error) {
	parts := make([]string, 0, len(input))
	for i, item := range input {
		if item.Type != "text" {
			return "", fmt.Errorf("input[%d] type %q is not supported; this server currently accepts text only", i, item.Type)
		}
		if len(item.TextElements) > 0 {
			return "", fmt.Errorf("input[%d].textElements is not supported", i)
		}
		if strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	text := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if text == "" {
		return "", fmt.Errorf("input must contain non-empty text")
	}
	return text, nil
}
