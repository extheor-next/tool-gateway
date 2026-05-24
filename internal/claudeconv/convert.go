package claudeconv

import (
	"strings"

	"tool-gateway/internal/config"
)

func ConvertClaudeToDeepSeek(claudeReq map[string]any, aliasProvider config.ModelAliasReader, defaultClaudeModel string) map[string]any {
	messages, _ := claudeReq["messages"].([]any)
	model, _ := claudeReq["model"].(string)
	if model == "" {
		model = defaultClaudeModel
	}

	dsModel, ok := config.ResolveModel(aliasProvider, model)
	if !ok || strings.TrimSpace(dsModel) == "" {
		dsModel = "deepseek-v4-flash"
	}

	convertedMessages := make([]any, 0, len(messages)+1)
	if system, ok := claudeReq["system"].(string); ok && system != "" {
		convertedMessages = append(convertedMessages, map[string]any{"role": "system", "content": system})
	}
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			convertedMessages = append(convertedMessages, msg)
			continue
		}
		convertedMessages = append(convertedMessages, convertClaudeMessage(m))
	}

	out := map[string]any{"model": dsModel, "messages": convertedMessages}
	for _, k := range []string{"temperature", "top_p", "stream"} {
		if v, ok := claudeReq[k]; ok {
			out[k] = v
		}
	}
	if stopSeq, ok := claudeReq["stop_sequences"]; ok {
		out["stop"] = stopSeq
	}
	return out
}

// convertClaudeMessage converts a Claude-format message (which may contain
// content blocks like {"type": "image", "source": {...}}) to an OpenAI-compatible
// format with image_url blocks.
func convertClaudeMessage(msg map[string]any) map[string]any {
	content := msg["content"]
	blocks, ok := content.([]any)
	if !ok {
		return msg
	}

	converted := make([]any, 0, len(blocks))
	for _, block := range blocks {
		b, ok := block.(map[string]any)
		if !ok {
			converted = append(converted, block)
			continue
		}
		blockType := strings.ToLower(strings.TrimSpace(toString(b["type"])))
		switch blockType {
		case "image":
			source, _ := b["source"].(map[string]any)
			if source != nil {
				converted = append(converted, claudeImageToOpenAI(source))
			}
		case "text":
			converted = append(converted, map[string]string{
				"type": "text",
				"text": toString(b["text"]),
			})
		default:
			converted = append(converted, block)
		}
	}

	result := make(map[string]any, len(msg))
	for k, v := range msg {
		if k == "content" {
			result[k] = converted
		} else {
			result[k] = v
		}
	}
	return result
}

// claudeImageToOpenAI converts a Claude image source block to an OpenAI image_url block.
// Claude format: {"type": "base64", "media_type": "image/png", "data": "..."}
// OpenAI format:  {"type": "image_url", "image_url": {"url": "data:image/png;base64,..."}}
func claudeImageToOpenAI(source map[string]any) map[string]any {
	mediaType := toString(source["media_type"])
	data := toString(source["data"])
	url := ""
	if mediaType != "" && data != "" {
		url = "data:" + mediaType + ";base64," + data
	}
	return map[string]any{
		"type": "image_url",
		"image_url": map[string]string{"url": url},
	}
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}
