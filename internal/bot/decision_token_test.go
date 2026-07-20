package bot

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"reames-agent/internal/control"
	"reames-agent/internal/event"
)

type remoteDecisionController struct {
	botController
	approvalIDs []string
	answerIDs   []string
}

func (c *remoteDecisionController) ExecuteCommand(command control.Command, _ control.CommandScope) (control.CommandResult, error) {
	if command.Kind == control.CommandApproval && command.Approval != nil {
		c.approvalIDs = append(c.approvalIDs, command.Approval.ID)
	}
	return control.CommandResult{Version: control.CommandVersion, Kind: command.Kind, Accepted: true}, nil
}

func (c *remoteDecisionController) TryAnswerQuestion(id string, _ []event.AskAnswer) bool {
	c.answerIDs = append(c.answerIDs, id)
	return true
}

func TestGatewayRemoteApprovalTokenIsOpaqueAndOneShot(t *testing.T) {
	gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adapter := newFakeAdapter(PlatformTelegram, "telegram")
	ctrl := &remoteDecisionController{}
	state := &sessionState{ctrl: ctrl, sink: &sessionEventSink{}}
	msg := InboundMessage{Platform: PlatformTelegram, ChatType: ChatDM, ChatID: "chat", UserID: "approver"}
	key := BuildSessionKey(msg.Session())
	gw.controllers[key] = state

	remote := gw.registerPendingApproval(state, event.Approval{ID: "controller-7", Tool: "bash"})
	if remote.ID == "" || remote.ID == "controller-7" || !strings.HasPrefix(remote.ID, "approval-") {
		t.Fatalf("remote approval ID = %q, want opaque approval token", remote.ID)
	}
	msg.Text = "/approve " + remote.ID
	if err := gw.handleSlashCommand(context.Background(), adapter, key, msg); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.approvalIDs) != 1 || ctrl.approvalIDs[0] != "controller-7" {
		t.Fatalf("controller approvals = %#v, want internal ID once", ctrl.approvalIDs)
	}

	if err := gw.handleSlashCommand(context.Background(), adapter, key, msg); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.approvalIDs) != 1 {
		t.Fatalf("replayed token resolved again: %#v", ctrl.approvalIDs)
	}
	sent := adapter.sentMessages()
	if len(sent) != 2 || sent[0].Text != "已批准。" || !strings.Contains(sent[1].Text, "已过期或不属于当前会话") {
		t.Fatalf("decision acknowledgements = %#v", sent)
	}
}

func TestGatewayRemoteDecisionTokensDoNotRepeatAcrossGatewayRestart(t *testing.T) {
	first := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	second := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	firstID := first.registerPendingApproval(&sessionState{}, event.Approval{ID: "1"}).ID
	secondID := second.registerPendingApproval(&sessionState{}, event.Approval{ID: "1"}).ID
	if firstID == secondID {
		t.Fatalf("gateway restart reused remote decision token %q", firstID)
	}
}

func TestGatewayRemoteAskTokenMapsToInternalIDAndExpires(t *testing.T) {
	gw := NewGateway(GatewayConfig{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	adapter := newFakeAdapter(PlatformWeixin, "weixin")
	ctrl := &remoteDecisionController{}
	state := &sessionState{ctrl: ctrl, sink: &sessionEventSink{}}
	msg := InboundMessage{Platform: PlatformWeixin, ChatType: ChatDM, ChatID: "chat", UserID: "user"}
	key := BuildSessionKey(msg.Session())
	gw.controllers[key] = state

	remote := gw.registerPendingAsk(state, event.Ask{ID: "controller-ask-9", Questions: []event.AskQuestion{{
		ID: "q1", Prompt: "Continue?", Options: []event.AskOption{{Label: "Yes"}, {Label: "No"}},
	}}})
	msg.Text = "/answer " + remote.ID + " 1"
	if err := gw.handleSlashCommand(context.Background(), adapter, key, msg); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.answerIDs) != 1 || ctrl.answerIDs[0] != "controller-ask-9" {
		t.Fatalf("controller answer IDs = %#v", ctrl.answerIDs)
	}
	gw.clearPendingRemoteDecisions(state)
	if err := gw.handleSlashCommand(context.Background(), adapter, key, msg); err != nil {
		t.Fatal(err)
	}
	if len(ctrl.answerIDs) != 1 {
		t.Fatalf("expired ask token resolved again: %#v", ctrl.answerIDs)
	}
}
