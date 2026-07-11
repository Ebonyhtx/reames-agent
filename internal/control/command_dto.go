package control

import (
	"fmt"
	"strings"
)

// CommandVersion is the current stable command-envelope version. Transports
// must reject versions they do not understand instead of guessing semantics.
const CommandVersion = 1

// CommandKind identifies one transport-agnostic controller operation.
type CommandKind string

const (
	CommandSubmit   CommandKind = "submit"
	CommandCancel   CommandKind = "cancel"
	CommandApproval CommandKind = "approval"
	CommandStatus   CommandKind = "status"
)

// CommandScope is selected by the transport adapter, never by untrusted wire
// input. It decides which submit surface is available while cancellation,
// approval, and status retain identical semantics across transports.
type CommandScope string

const (
	// CommandScopeRemote disables the trusted !shell shortcut and confines
	// file references to the controller workspace.
	CommandScopeRemote CommandScope = "remote"
	// CommandScopeTrusted enables the interactive desktop/TUI command surface.
	CommandScopeTrusted CommandScope = "trusted"
	// CommandScopeUserTurn submits prompt text without interpreting commands.
	CommandScopeUserTurn CommandScope = "user_turn"
)

// Command is the stable command envelope shared by controller frontends.
// Exactly one payload must match Kind; cancel and status have no payload.
type Command struct {
	Version  int              `json:"version"`
	Kind     CommandKind      `json:"kind"`
	Submit   *SubmitCommand   `json:"submit,omitempty"`
	Approval *ApprovalCommand `json:"approval,omitempty"`
}

// SubmitCommand carries model input and optional local transcript metadata.
// Remote transports may only provide Input; Display and Original are trusted
// UI metadata and are rejected under CommandScopeRemote.
type SubmitCommand struct {
	Input    string `json:"input"`
	Display  string `json:"display,omitempty"`
	Original string `json:"original,omitempty"`
}

// ApprovalCommand resolves one pending approval request.
type ApprovalCommand struct {
	ID      string `json:"id"`
	Allow   bool   `json:"allow"`
	Session bool   `json:"session,omitempty"`
	Persist bool   `json:"persist,omitempty"`
}

// CommandResult is the stable acknowledgement returned by command transports.
// Status is sampled after dispatch; asynchronous submits may still be entering
// their run loop, so Accepted is the authoritative dispatch acknowledgement.
type CommandResult struct {
	Version  int           `json:"version"`
	Kind     CommandKind   `json:"kind"`
	Accepted bool          `json:"accepted"`
	Status   RuntimeStatus `json:"status"`
	Error    *CommandError `json:"error,omitempty"`
}

// CommandErrorCode is a stable protocol-level command failure code.
type CommandErrorCode string

const (
	CommandErrInvalidVersion CommandErrorCode = "invalid_version"
	CommandErrInvalidKind    CommandErrorCode = "invalid_kind"
	CommandErrInvalidPayload CommandErrorCode = "invalid_payload"
	CommandErrForbidden      CommandErrorCode = "forbidden"
	CommandErrBusy           CommandErrorCode = "busy"
)

// CommandError describes a command rejected before it reached the runtime.
type CommandError struct {
	Code    CommandErrorCode `json:"code"`
	Message string           `json:"message"`
	Field   string           `json:"field,omitempty"`
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewSubmitCommand builds a current-version submit command.
func NewSubmitCommand(input, display, original string) Command {
	return Command{
		Version: CommandVersion,
		Kind:    CommandSubmit,
		Submit:  &SubmitCommand{Input: input, Display: display, Original: original},
	}
}

// NewCancelCommand builds a current-version cancellation command.
func NewCancelCommand() Command {
	return Command{Version: CommandVersion, Kind: CommandCancel}
}

// NewApprovalCommand builds a current-version approval decision command.
func NewApprovalCommand(id string, allow, session, persist bool) Command {
	return Command{
		Version:  CommandVersion,
		Kind:     CommandApproval,
		Approval: &ApprovalCommand{ID: id, Allow: allow, Session: session, Persist: persist},
	}
}

// NewStatusCommand builds a current-version runtime-status query.
func NewStatusCommand() Command {
	return Command{Version: CommandVersion, Kind: CommandStatus}
}

// CommandControl is the stable command port used by transport adapters.
type CommandControl interface {
	ExecuteCommand(Command, CommandScope) (CommandResult, error)
}

type commandTarget interface {
	Submit(string)
	SubmitDisplay(string, string)
	SubmitEditedDisplay(string, string, string)
	SubmitHTTP(string)
	SubmitUserTurn(string, string)
	Cancel()
	Approve(string, bool, bool, bool)
	RuntimeStatus() RuntimeStatus
}

// ExecuteCommand validates and dispatches one stable command. The scope is a
// server-side capability decision and deliberately does not appear in Command's
// JSON representation.
func (c *Controller) ExecuteCommand(command Command, scope CommandScope) (CommandResult, error) {
	c.commandMu.Lock()
	defer c.commandMu.Unlock()

	if err := ValidateCommand(command, scope); err != nil {
		return rejectedRuntimeCommand(command.Kind, RuntimeStatus{}, err), err
	}
	if command.Kind == CommandSubmit {
		c.mu.Lock()
		busy := c.running || c.rotating
		c.mu.Unlock()
		if busy {
			err := commandError(CommandErrBusy, "kind", "controller already has active work")
			return rejectedRuntimeCommand(command.Kind, c.RuntimeStatus(), err), err
		}
	}
	return executeCommand(c, command, scope)
}

func rejectedRuntimeCommand(kind CommandKind, status RuntimeStatus, err *CommandError) CommandResult {
	return CommandResult{Version: CommandVersion, Kind: kind, Status: status, Error: err}
}

func executeCommand(target commandTarget, command Command, scope CommandScope) (CommandResult, error) {
	result := CommandResult{Version: CommandVersion, Kind: command.Kind}
	if err := ValidateCommand(command, scope); err != nil {
		result.Error = err
		return result, err
	}

	switch command.Kind {
	case CommandSubmit:
		payload := command.Submit
		switch scope {
		case CommandScopeRemote:
			target.SubmitHTTP(payload.Input)
		case CommandScopeUserTurn:
			target.SubmitUserTurn(payload.Input, payload.Display)
		case CommandScopeTrusted:
			switch {
			case strings.TrimSpace(payload.Original) != "":
				target.SubmitEditedDisplay(payload.Display, payload.Input, payload.Original)
			case payload.Display != "":
				target.SubmitDisplay(payload.Display, payload.Input)
			default:
				target.Submit(payload.Input)
			}
		}
	case CommandCancel:
		target.Cancel()
	case CommandApproval:
		payload := command.Approval
		target.Approve(payload.ID, payload.Allow, payload.Session, payload.Persist)
	case CommandStatus:
		// Status is sampled below with every other command result.
	}

	result.Accepted = true
	result.Status = target.RuntimeStatus()
	return result, nil
}

// ValidateCommand validates envelope shape and the transport-selected scope
// without causing runtime side effects.
func ValidateCommand(command Command, scope CommandScope) *CommandError {
	if command.Version != CommandVersion {
		return commandError(CommandErrInvalidVersion, "version", fmt.Sprintf("unsupported command version %d", command.Version))
	}
	if scope != CommandScopeRemote && scope != CommandScopeTrusted && scope != CommandScopeUserTurn {
		return commandError(CommandErrForbidden, "scope", "command scope is not permitted")
	}

	switch command.Kind {
	case CommandSubmit:
		if command.Submit == nil || command.Approval != nil {
			return commandError(CommandErrInvalidPayload, "submit", "submit command requires only a submit payload")
		}
		if strings.TrimSpace(command.Submit.Input) == "" {
			return commandError(CommandErrInvalidPayload, "submit.input", "submit input must not be blank")
		}
		if scope == CommandScopeRemote && (command.Submit.Display != "" || command.Submit.Original != "") {
			return commandError(CommandErrForbidden, "submit", "remote submit cannot set local transcript metadata")
		}
		if scope == CommandScopeUserTurn && command.Submit.Original != "" {
			return commandError(CommandErrForbidden, "submit.original", "user-turn submit cannot set edited-prompt metadata")
		}
	case CommandApproval:
		if command.Approval == nil || command.Submit != nil {
			return commandError(CommandErrInvalidPayload, "approval", "approval command requires only an approval payload")
		}
		if strings.TrimSpace(command.Approval.ID) == "" {
			return commandError(CommandErrInvalidPayload, "approval.id", "approval id must not be blank")
		}
	case CommandCancel, CommandStatus:
		if command.Submit != nil || command.Approval != nil {
			return commandError(CommandErrInvalidPayload, "kind", fmt.Sprintf("%s command does not accept a payload", command.Kind))
		}
	default:
		return commandError(CommandErrInvalidKind, "kind", fmt.Sprintf("unknown command kind %q", command.Kind))
	}
	return nil
}

func commandError(code CommandErrorCode, field, message string) *CommandError {
	return &CommandError{Code: code, Field: field, Message: message}
}
