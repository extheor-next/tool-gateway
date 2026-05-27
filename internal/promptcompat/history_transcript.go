package promptcompat

import (
	"fmt"
	"strings"
)

const CurrentInputContextFilename = "TOOL_GATEWAY_HISTORY.txt"

const historyTranscriptTitle = "# TOOL_GATEWAY_HISTORY.txt"
const historyTranscriptSummary = "Prior conversation history and tool progress."
const historyRecentFullEntries = 6
const historyOldEntryMaxRunes = 2000

func BuildOpenAIHistoryTranscript(messages []any) string {
	return buildOpenAIHistoryTranscript(messages)
}

func BuildOpenAICurrentUserInputTranscript(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return buildOpenAIHistoryTranscript([]any{
		map[string]any{"role": "user", "content": text},
	})
}

func BuildOpenAICurrentInputContextTranscript(messages []any) string {
	return buildOpenAIHistoryTranscript(messages)
}

func buildOpenAIHistoryTranscript(messages []any) string {
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(historyTranscriptTitle)
	b.WriteString("\n")
	b.WriteString(historyTranscriptSummary)
	b.WriteString("\n\n")

	entries := make([]historyEntry, 0, len(messages))
	for _, raw := range messages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		role := normalizeOpenAIRoleForPrompt(strings.ToLower(strings.TrimSpace(asString(msg["role"]))))
		content := strings.TrimSpace(buildOpenAIHistoryEntry(role, msg))
		if content == "" {
			continue
		}
		entries = append(entries, historyEntry{role: role, content: content})
	}

	fullFrom := len(entries) - historyRecentFullEntries
	if fullFrom < 0 {
		fullFrom = 0
	}
	for i, entry := range entries {
		content := entry.content
		if i < fullFrom {
			content = compactOldHistoryEntry(content)
		}
		fmt.Fprintf(&b, "=== %d. %s ===\n%s\n\n", i+1, strings.ToUpper(roleLabelForHistory(entry.role)), content)
	}

	transcript := strings.TrimSpace(b.String())
	if transcript == "" {
		return ""
	}
	return transcript + "\n"
}

type historyEntry struct {
	role    string
	content string
}

func compactOldHistoryEntry(content string) string {
	runes := []rune(content)
	if len(runes) <= historyOldEntryMaxRunes {
		return content
	}
	keepHead := historyOldEntryMaxRunes / 2
	keepTail := historyOldEntryMaxRunes - keepHead
	omitted := len(runes) - keepHead - keepTail
	return string(runes[:keepHead]) + fmt.Sprintf("\n[older large entry truncated: omitted %d chars]\n", omitted) + string(runes[len(runes)-keepTail:])
}

func buildOpenAIHistoryEntry(role string, msg map[string]any) string {
	switch role {
	case "assistant":
		return strings.TrimSpace(buildAssistantContentForPrompt(msg))
	case "tool", "function":
		return strings.TrimSpace(buildToolHistoryContent(msg))
	case "system", "user":
		return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	default:
		return strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	}
}

func buildToolHistoryContent(msg map[string]any) string {
	content := strings.TrimSpace(NormalizeOpenAIContentForPrompt(msg["content"]))
	parts := make([]string, 0, 2)
	if name := strings.TrimSpace(asString(msg["name"])); name != "" {
		parts = append(parts, "name="+name)
	}
	if callID := strings.TrimSpace(asString(msg["tool_call_id"])); callID != "" {
		parts = append(parts, "tool_call_id="+callID)
	}
	header := ""
	if len(parts) > 0 {
		header = "[" + strings.Join(parts, " ") + "]"
	}
	switch {
	case header != "" && content != "":
		return header + "\n" + content
	case header != "":
		return header
	default:
		return content
	}
}

func roleLabelForHistory(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case "function":
		return "tool"
	case "":
		return "unknown"
	default:
		return role
	}
}
