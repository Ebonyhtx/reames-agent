package main

import (
	"reames-agent/internal/agent"
	"reames-agent/internal/control"
	"reames-agent/internal/event"
	"reames-agent/internal/provider"
)

func providerTranscriptForTest(messages []provider.Message) []control.TranscriptMessage {
	session := agent.NewSession("")
	session.Replace(append([]provider.Message(nil), messages...))
	executor := agent.New(nil, nil, session, agent.Options{}, event.Discard)
	controller := control.New(control.Options{Runner: executor, Executor: executor, Sink: event.Discard})
	return controller.Transcript()
}

func historyMessagesFromProviderForTest(messages []provider.Message, resolve func(string) string) []HistoryMessage {
	return historyMessages(providerTranscriptForTest(messages), transcriptResolverForTest(resolve))
}

func historyMessagesWithPlannerDisplaysFromProviderForTest(messages []provider.Message, resolve func(string) string, plannerTurns []plannerDisplayTurn, checkpointTurns map[int]int) []HistoryMessage {
	return historyMessagesWithPlannerDisplays(providerTranscriptForTest(messages), transcriptResolverForTest(resolve), plannerTurns, checkpointTurns)
}

func historyPageFromProviderMessages(messages []provider.Message, resolve func(string) string, plannerTurns []plannerDisplayTurn, checkpointTurns map[int]int, beforeTurn, limit int) HistoryPage {
	return historyPageFromTranscript(providerTranscriptForTest(messages), transcriptResolverForTest(resolve), plannerTurns, checkpointTurns, beforeTurn, limit)
}

func transcriptResolverForTest(resolve func(string) string) func(control.TranscriptMessage) string {
	return func(message control.TranscriptMessage) string {
		if resolve == nil {
			return message.Content
		}
		return resolve(message.Content)
	}
}
