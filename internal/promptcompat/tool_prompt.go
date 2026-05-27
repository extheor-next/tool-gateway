package promptcompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"tool-gateway/internal/toolcall"
)

const CurrentToolsContextFilename = "TOOL_GATEWAY_TOOLS.txt"

const toolsTranscriptTitle = "# TOOL_GATEWAY_TOOLS.txt"
const toolsTranscriptSummary = "Available tool descriptions and parameter schemas for this request."

type toolPromptParts struct {
	Descriptions string
	Instructions string
	Names        []string
}

func injectToolPrompt(messages []map[string]any, tools []any, policy ToolChoicePolicy) ([]map[string]any, []string) {
	return injectToolPromptWithDescriptions(messages, tools, policy, true)
}

func injectToolPromptInstructionsOnly(messages []map[string]any, tools []any, policy ToolChoicePolicy) ([]map[string]any, []string) {
	return injectToolPromptWithDescriptions(messages, tools, policy, false)
}

func injectToolPromptWithDescriptions(messages []map[string]any, tools []any, policy ToolChoicePolicy, includeDescriptions bool) ([]map[string]any, []string) {
	if policy.IsNone() {
		return messages, nil
	}
	parts := buildToolPromptParts(tools, policy)
	if parts.Instructions == "" {
		return messages, parts.Names
	}
	toolPrompt := parts.Instructions
	if includeDescriptions && parts.Descriptions != "" {
		toolPrompt = parts.Descriptions + "\n\n" + toolPrompt
	} else if !includeDescriptions && parts.Descriptions != "" {
		toolPrompt = "Available tool descriptions, parameter schemas, and tool-call format rules are attached in TOOL_GATEWAY_TOOLS.txt. Treat TOOL_GATEWAY_TOOLS.txt as the authoritative list of callable tools and schemas; use only tools and parameters listed there. If you call tools, follow the tool-call format rules from TOOL_GATEWAY_TOOLS.txt exactly."
	}

	for i := range messages {
		if messages[i]["role"] == "system" {
			old, _ := messages[i]["content"].(string)
			messages[i]["content"] = strings.TrimSpace(old + "\n\n" + toolPrompt)
			return messages, parts.Names
		}
	}
	messages = append([]map[string]any{{"role": "system", "content": toolPrompt}}, messages...)
	return messages, parts.Names
}

func buildToolPromptParts(tools []any, policy ToolChoicePolicy) toolPromptParts {
	toolSchemas := make([]string, 0, len(tools))
	names := make([]string, 0, len(tools))
	isAllowed := func(name string) bool {
		if strings.TrimSpace(name) == "" {
			return false
		}
		if len(policy.Allowed) == 0 {
			return true
		}
		_, ok := policy.Allowed[name]
		return ok
	}

	for _, t := range tools {
		tool, ok := t.(map[string]any)
		if !ok {
			continue
		}
		name, desc, schema := toolcall.ExtractToolMeta(tool)
		name = strings.TrimSpace(name)
		if !isAllowed(name) {
			continue
		}
		names = append(names, name)
		if desc == "" {
			desc = "No description available"
		}
		b, _ := json.Marshal(schema)
		toolSchemas = append(toolSchemas, fmt.Sprintf("Tool: %s\nDescription: %s\nParameters: %s", name, desc, string(b)))
	}
	if len(toolSchemas) == 0 {
		return toolPromptParts{Names: names}
	}
	descriptions := "You have access to these tools:\n\n" + strings.Join(toolSchemas, "\n\n")
	instructions := toolcall.BuildToolCallInstructions(names)
	if hasReadLikeTool(names) {
		instructions += "\n\nRead-tool cache guard: If a Read/read_file-style tool result says the file is unchanged, already available in history, should be referenced from previous context, or otherwise provides no file body, treat that result as missing content. Do not repeatedly call the same read request for that missing body. Request a full-content read if the tool supports it, or tell the user that the file contents need to be provided again."
	}
	if policy.Mode == ToolChoiceRequired {
		instructions += "\n7) For this response, you MUST call at least one tool from the allowed list."
	}
	if policy.Mode == ToolChoiceForced && strings.TrimSpace(policy.ForcedName) != "" {
		instructions += "\n7) For this response, you MUST call exactly this tool name: " + strings.TrimSpace(policy.ForcedName)
		instructions += "\n8) Do not call any other tool."
	}
	return toolPromptParts{
		Descriptions: descriptions,
		Instructions: instructions,
		Names:        names,
	}
}

func BuildOpenAIToolsContextTranscript(toolsRaw any, policy ToolChoicePolicy) (string, []string) {
	return BuildOpenAIToolsContextTranscriptForMessages(toolsRaw, policy, nil)
}

func BuildOpenAIToolsContextTranscriptForMessages(toolsRaw any, policy ToolChoicePolicy, messages []any) (string, []string) {
	if policy.IsNone() {
		return "", nil
	}
	tools, ok := toolsRaw.([]any)
	if !ok || len(tools) == 0 {
		return "", nil
	}
	compact := false
	if len(policy.Allowed) == 0 && strings.TrimSpace(latestUserTextForToolSelection(messages)) != "" {
		if !shouldIncludeToolsForUserText(latestUserTextForToolSelection(messages)) {
			return "", nil
		}
		compact = true
	}
	parts := buildToolPromptParts(tools, policy)
	if compact {
		parts.Descriptions = buildCompactToolDescriptions(tools, policy)
	}
	if strings.TrimSpace(parts.Descriptions) == "" {
		return "", parts.Names
	}
	var b strings.Builder
	b.WriteString(toolsTranscriptTitle)
	b.WriteString("\n")
	b.WriteString(toolsTranscriptSummary)
	b.WriteString("\n\n")
	b.WriteString(parts.Descriptions)
	if strings.TrimSpace(parts.Instructions) != "" {
		b.WriteString("\n\n")
		b.WriteString(parts.Instructions)
	}
	b.WriteString("\n")
	return b.String(), parts.Names
}

func latestUserTextForToolSelection(messages []any) string {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok || strings.ToLower(strings.TrimSpace(fmt.Sprint(msg["role"]))) != "user" {
			continue
		}
		switch content := msg["content"].(type) {
		case string:
			return strings.TrimSpace(content)
		case []any:
			parts := make([]string, 0, len(content))
			for _, raw := range content {
				part, ok := raw.(map[string]any)
				if !ok || strings.ToLower(strings.TrimSpace(fmt.Sprint(part["type"]))) != "text" {
					continue
				}
				if text := strings.TrimSpace(fmt.Sprint(part["text"])); text != "" {
					parts = append(parts, text)
				}
			}
			return strings.Join(parts, "\n")
		}
	}
	return ""
}

func shouldIncludeToolsForUserText(userText string) bool {
	query := strings.ToLower(strings.TrimSpace(userText))
	return query != "" && !isPureImageQuestion(query) && likelyNeedsTool(query)
}

func buildCompactToolDescriptions(tools []any, policy ToolChoicePolicy) string {
	lines := make([]string, 0, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _, schema := toolcall.ExtractToolMeta(tool)
		name = strings.TrimSpace(name)
		if name == "" || !policy.Allows(name) {
			continue
		}
		b, _ := json.Marshal(schema)
		lines = append(lines, fmt.Sprintf("Tool: %s\nParameters: %s", name, string(b)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "You have access to these tools:\n\n" + strings.Join(lines, "\n\n")
}

func isPureImageQuestion(query string) bool {
	for _, term := range []string{"这是什么", "what is this", "describe image", "描述图片", "看图", "图片"} {
		if strings.Contains(query, term) {
			return true
		}
	}
	return false
}

func likelyNeedsTool(query string) bool {
	for _, term := range []string{
		"read", "读取", "查看", "打开", "文件", "file",
		"grep", "搜索代码", "查找", "search code", "find",
		"glob", "列出", "匹配", "文件列表",
		"bash", "运行", "执行", "命令", "command",
		"edit", "修改", "编辑", "修复", "代码", "code",
		"write", "写入", "创建", "新建",
		"web", "search", "搜索", "联网", "url", "网页",
		"todo", "任务", "计划",
	} {
		if strings.Contains(query, term) {
			return true
		}
	}
	return false
}

func hasReadLikeTool(names []string) bool {
	for _, name := range names {
		switch normalizeToolNameForGuard(name) {
		case "read", "readfile":
			return true
		}
	}
	return false
}

func normalizeToolNameForGuard(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
