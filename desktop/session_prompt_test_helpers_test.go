package main

import "reames-agent/internal/provider"

func systemPromptFrom(messages []provider.Message) string {
	for _, message := range messages {
		if message.Role == provider.RoleSystem {
			return message.Content
		}
	}
	return ""
}
