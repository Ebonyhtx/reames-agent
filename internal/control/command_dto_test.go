package control

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type commandTargetSpy struct {
	calls  []string
	values []any
	status RuntimeStatus
}

func (s *commandTargetSpy) Submit(input string) {
	s.calls = append(s.calls, "submit")
	s.values = append(s.values, input)
}
func (s *commandTargetSpy) SubmitDisplay(display, input string) {
	s.calls = append(s.calls, "display")
	s.values = append(s.values, []string{display, input})
}
func (s *commandTargetSpy) SubmitEditedDisplay(display, input, original string) {
	s.calls = append(s.calls, "edited")
	s.values = append(s.values, []string{display, input, original})
}
func (s *commandTargetSpy) SubmitHTTP(input string) {
	s.calls = append(s.calls, "http")
	s.values = append(s.values, input)
}
func (s *commandTargetSpy) SubmitUserTurn(input, display string) {
	s.calls = append(s.calls, "user_turn")
	s.values = append(s.values, []string{input, display})
}
func (s *commandTargetSpy) Cancel() { s.calls = append(s.calls, "cancel") }
func (s *commandTargetSpy) Approve(id string, allow, session, persist bool) {
	s.calls = append(s.calls, "approval")
	s.values = append(s.values, []any{id, allow, session, persist})
}
func (s *commandTargetSpy) RuntimeStatus() RuntimeStatus { return s.status }

func TestCommandJSONContract(t *testing.T) {
	command := NewApprovalCommand("approval-7", true, true, false)
	raw, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"kind":"approval","approval":{"id":"approval-7","allow":true,"session":true}}`
	if string(raw) != want {
		t.Fatalf("command JSON = %s, want %s", raw, want)
	}

	result := CommandResult{
		Version:  CommandVersion,
		Kind:     CommandStatus,
		Accepted: true,
		Status:   RuntimeStatus{Running: true, PendingPrompt: true, BackgroundJobs: 2, Cancellable: true},
	}
	raw, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	want = `{"version":1,"kind":"status","accepted":true,"status":{"running":true,"pendingPrompt":true,"backgroundJobs":2,"cancelRequested":false,"cancellable":true}}`
	if string(raw) != want {
		t.Fatalf("result JSON = %s, want %s", raw, want)
	}
}

func TestExecuteCommandRoutesStableOperations(t *testing.T) {
	status := RuntimeStatus{Running: true, Cancellable: true}
	tests := []struct {
		name    string
		command Command
		scope   CommandScope
		call    string
		value   any
	}{
		{"remote submit", NewSubmitCommand("hello", "", ""), CommandScopeRemote, "http", "hello"},
		{"trusted submit", NewSubmitCommand("hello", "", ""), CommandScopeTrusted, "submit", "hello"},
		{"display submit", NewSubmitCommand("expanded", "visible", ""), CommandScopeTrusted, "display", []string{"visible", "expanded"}},
		{"edited submit", NewSubmitCommand("edited", "edited", "original"), CommandScopeTrusted, "edited", []string{"edited", "edited", "original"}},
		{"user turn", NewSubmitCommand("prompt", "visible", ""), CommandScopeUserTurn, "user_turn", []string{"prompt", "visible"}},
		{"cancel", NewCancelCommand(), CommandScopeRemote, "cancel", nil},
		{"approval", NewApprovalCommand("a1", false, true, true), CommandScopeRemote, "approval", []any{"a1", false, true, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &commandTargetSpy{status: status}
			result, err := executeCommand(spy, tt.command, tt.scope)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Accepted || result.Version != CommandVersion || result.Kind != tt.command.Kind || result.Status != status {
				t.Fatalf("result = %+v", result)
			}
			if !reflect.DeepEqual(spy.calls, []string{tt.call}) {
				t.Fatalf("calls = %#v, want %q", spy.calls, tt.call)
			}
			if tt.value != nil && !reflect.DeepEqual(spy.values, []any{tt.value}) {
				t.Fatalf("values = %#v, want %#v", spy.values, tt.value)
			}
		})
	}

	spy := &commandTargetSpy{status: status}
	result, err := executeCommand(spy, NewStatusCommand(), CommandScopeRemote)
	if err != nil || !result.Accepted || result.Status != status || len(spy.calls) != 0 {
		t.Fatalf("status result = %+v, err = %v, calls = %#v", result, err, spy.calls)
	}
}

func TestExecuteCommandRejectsInvalidCommandsWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name    string
		command Command
		scope   CommandScope
		code    CommandErrorCode
	}{
		{"version", Command{Version: 2, Kind: CommandCancel}, CommandScopeRemote, CommandErrInvalidVersion},
		{"kind", Command{Version: CommandVersion, Kind: "launch"}, CommandScopeRemote, CommandErrInvalidKind},
		{"blank submit", NewSubmitCommand("  ", "", ""), CommandScopeRemote, CommandErrInvalidPayload},
		{"missing submit", Command{Version: CommandVersion, Kind: CommandSubmit}, CommandScopeRemote, CommandErrInvalidPayload},
		{"mixed payload", Command{Version: CommandVersion, Kind: CommandCancel, Submit: &SubmitCommand{Input: "x"}}, CommandScopeRemote, CommandErrInvalidPayload},
		{"blank approval", NewApprovalCommand(" ", true, false, false), CommandScopeRemote, CommandErrInvalidPayload},
		{"remote metadata", NewSubmitCommand("x", "display", ""), CommandScopeRemote, CommandErrForbidden},
		{"user original", NewSubmitCommand("x", "display", "original"), CommandScopeUserTurn, CommandErrForbidden},
		{"scope", NewCancelCommand(), CommandScope("client-selected"), CommandErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &commandTargetSpy{}
			result, err := executeCommand(spy, tt.command, tt.scope)
			if err == nil || result.Error == nil || result.Error.Code != tt.code || result.Accepted {
				t.Fatalf("result = %+v, err = %v, want code %s", result, err, tt.code)
			}
			if len(spy.calls) != 0 {
				t.Fatalf("invalid command caused calls: %#v", spy.calls)
			}
		})
	}
}

type blockingCommandRunner struct {
	started chan string
	stopped chan struct{}
}

func (r blockingCommandRunner) Run(ctx context.Context, input string) error {
	r.started <- input
	<-ctx.Done()
	r.stopped <- struct{}{}
	return ctx.Err()
}

func TestControllerExecuteCommandRejectsConcurrentSubmit(t *testing.T) {
	runner := blockingCommandRunner{started: make(chan string, 2), stopped: make(chan struct{}, 1)}
	c := New(Options{Runner: runner})

	first, err := c.ExecuteCommand(NewSubmitCommand("first", "", ""), CommandScopeTrusted)
	if err != nil || !first.Accepted || !first.Status.Running {
		t.Fatalf("first submit = %+v, err = %v", first, err)
	}
	select {
	case input := <-runner.started:
		if input != "first" {
			t.Fatalf("runner input = %q", input)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first submit did not start")
	}

	second, err := c.ExecuteCommand(NewSubmitCommand("second", "", ""), CommandScopeTrusted)
	if err == nil || second.Accepted || second.Error == nil || second.Error.Code != CommandErrBusy {
		t.Fatalf("second submit = %+v, err = %v, want busy", second, err)
	}
	select {
	case input := <-runner.started:
		t.Fatalf("busy submit reached runner: %q", input)
	default:
	}

	if _, err := c.ExecuteCommand(NewCancelCommand(), CommandScopeTrusted); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop first submit")
	}
}
