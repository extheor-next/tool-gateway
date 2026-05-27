package history

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"tool-gateway/internal/config"
	dsclient "tool-gateway/internal/deepseek/client"
	"tool-gateway/internal/httpapi/openai/shared"
	"tool-gateway/internal/promptcompat"
)

const (
	currentInputFilename    = promptcompat.CurrentInputContextFilename
	currentToolsFilename    = promptcompat.CurrentToolsContextFilename
	currentInputContentType = "text/plain; charset=utf-8"
	currentInputPurpose     = "assistants"
)

type CurrentInputConfigReader interface {
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
	CurrentInputFileMaxKeepMessages() int
}

type CurrentInputUploader interface {
	UploadFile(ctx context.Context, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
}

type ExternalAIAdapterMarker interface {
	ExternalAIAdapter() bool
}

type Service struct {
	Store   CurrentInputConfigReader
	Backend CurrentInputUploader
}

func (s Service) ApplyCurrentInputFile(ctx context.Context, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if stdReq.CurrentInputFileApplied || s.Backend == nil || s.Store == nil || !s.Store.CurrentInputFileEnabled() {
		return stdReq, nil
	}
	if marker, ok := s.Backend.(ExternalAIAdapterMarker); ok && marker.ExternalAIAdapter() {
		return s.applyTruncationForExternalProvider(stdReq)
	}
	threshold := s.Store.CurrentInputFileMinChars()

	index, text := latestUserInputForFile(stdReq.Messages)
	if index < 0 {
		return stdReq, nil
	}
	if len([]rune(text)) < threshold {
		return stdReq, nil
	}
	fileText := promptcompat.BuildOpenAICurrentInputContextTranscript(stdReq.Messages)
	if strings.TrimSpace(fileText) == "" {
		return stdReq, errors.New("current user input file produced empty transcript")
	}
	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscriptForMessages(stdReq.ToolsRaw, stdReq.ToolChoice, stdReq.Messages)
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	result, err := s.Backend.UploadFile(ctx, dsclient.UploadFileRequest{
		Filename:    currentInputFilename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(fileText),
	}, 3)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	toolFileID := ""
	if strings.TrimSpace(toolsText) != "" {
		result, err := s.Backend.UploadFile(ctx, dsclient.UploadFileRequest{
			Filename:    currentToolsFilename,
			ContentType: currentInputContentType,
			Purpose:     currentInputPurpose,
			ModelType:   modelType,
			Data:        []byte(toolsText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		toolFileID = strings.TrimSpace(result.ID)
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
	}

	messages := []any{
		map[string]any{
			"role":    "user",
			"content": currentInputFilePrompt(toolFileID != ""),
		},
	}

	stdReq.Messages = messages
	stdReq.HistoryText = fileText
	stdReq.ToolsText = toolsText
	stdReq.CurrentInputFileApplied = true
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	stdReq.RefFileIDs = prependUniqueRefFileIDs(stdReq.RefFileIDs, fileID, toolFileID)
	stdReq.FinalPrompt, stdReq.ToolNames = promptcompat.BuildOpenAIPromptWithToolInstructionsOnly(messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
	tokenParts := []string{fileText}
	if strings.TrimSpace(toolsText) != "" {
		tokenParts = append(tokenParts, toolsText)
	}
	tokenParts = append(tokenParts, stdReq.FinalPrompt)
	stdReq.PromptTokenText = strings.Join(tokenParts, "\n")
	return stdReq, nil
}

func (s Service) ReuploadAppliedCurrentInputFile(ctx context.Context, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if !stdReq.CurrentInputFileApplied || s.Backend == nil {
		return stdReq, nil
	}
	if marker, ok := s.Backend.(ExternalAIAdapterMarker); ok && marker.ExternalAIAdapter() {
		return stdReq, nil
	}
	fileText := strings.TrimSpace(stdReq.HistoryText)
	if fileText == "" {
		return stdReq, nil
	}
	modelType := "default"
	if resolvedType, ok := config.GetModelType(stdReq.ResolvedModel); ok {
		modelType = resolvedType
	}
	result, err := s.Backend.UploadFile(ctx, dsclient.UploadFileRequest{
		Filename:    currentInputFilename,
		ContentType: currentInputContentType,
		Purpose:     currentInputPurpose,
		ModelType:   modelType,
		Data:        []byte(stdReq.HistoryText),
	}, 3)
	if err != nil {
		return stdReq, fmt.Errorf("upload current user input file: %w", err)
	}
	fileID := strings.TrimSpace(result.ID)
	if fileID == "" {
		return stdReq, errors.New("upload current user input file returned empty file id")
	}

	toolsText, _ := promptcompat.BuildOpenAIToolsContextTranscriptForMessages(stdReq.ToolsRaw, stdReq.ToolChoice, stdReq.Messages)
	toolFileID := ""
	if strings.TrimSpace(toolsText) != "" {
		result, err := s.Backend.UploadFile(ctx, dsclient.UploadFileRequest{
			Filename:    currentToolsFilename,
			ContentType: currentInputContentType,
			Purpose:     currentInputPurpose,
			ModelType:   modelType,
			Data:        []byte(toolsText),
		}, 3)
		if err != nil {
			return stdReq, fmt.Errorf("upload current tools file: %w", err)
		}
		toolFileID = strings.TrimSpace(result.ID)
		if toolFileID == "" {
			return stdReq, errors.New("upload current tools file returned empty file id")
		}
	}

	stdReq.RefFileIDs = replaceGeneratedCurrentInputRefs(stdReq.RefFileIDs, stdReq.CurrentInputFileID, stdReq.CurrentToolsFileID, fileID, toolFileID)
	stdReq.CurrentInputFileID = fileID
	stdReq.CurrentToolsFileID = toolFileID
	return stdReq, nil
}

func latestUserImageParts(messages []any) []any {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok || strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"]))) != "user" {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		images := make([]any, 0)
		for _, raw := range parts {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if strings.ToLower(strings.TrimSpace(shared.AsString(part["type"]))) == "image_url" {
				images = append(images, part)
			}
		}
		if len(images) > 0 {
			return images
		}
	}
	return nil
}

func latestUserInputForFile(messages []any) (int, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(shared.AsString(msg["role"])))
		if role != "user" {
			continue
		}
		text := promptcompat.NormalizeOpenAIContentForPrompt(msg["content"])
		if strings.TrimSpace(text) == "" {
			return -1, ""
		}
		return i, text
	}
	return -1, ""
}

func currentInputFilePrompt(hasToolsFile bool) string {
	prompt := "Continue from the latest state in the attached TOOL_GATEWAY_HISTORY.txt context. Treat it as the current working state and answer the latest user request directly."
	if hasToolsFile {
		prompt += " Available tool descriptions, parameter schemas, and tool-call format rules are attached in TOOL_GATEWAY_TOOLS.txt; use only those tools and follow the attached tool-call rules exactly."
	}
	return prompt
}

func prependUniqueRefFileIDs(existing []string, fileIDs ...string) []string {
	out := make([]string, 0, len(existing)+len(fileIDs))
	seen := map[string]struct{}{}
	for _, fileID := range fileIDs {
		trimmed := strings.TrimSpace(fileID)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}

func replaceGeneratedCurrentInputRefs(existing []string, oldHistoryID, oldToolsID, newHistoryID, newToolsID string) []string {
	filtered := make([]string, 0, len(existing))
	old := map[string]struct{}{}
	for _, id := range []string{oldHistoryID, oldToolsID} {
		trimmed := strings.ToLower(strings.TrimSpace(id))
		if trimmed != "" {
			old[trimmed] = struct{}{}
		}
	}
	for _, id := range existing {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := old[strings.ToLower(trimmed)]; ok {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return prependUniqueRefFileIDs(filtered, newHistoryID, newToolsID)
}

type fileUploader interface {
	UploadFileToProvider(ctx context.Context, filename string, content []byte) (string, error)
}

// applyTruncationForExternalProvider handles context splitting for external AI providers.
// The external adapter uploads context files when it builds the upstream request.
func (s Service) applyTruncationForExternalProvider(stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	threshold := s.Store.CurrentInputFileMinChars()

	totalChars := 0
	for _, m := range stdReq.Messages {
		if msg, ok := m.(map[string]any); ok {
			switch c := msg["content"].(type) {
			case string:
				totalChars += len([]rune(c))
			case []any:
				for _, part := range c {
					if p, ok := part.(map[string]any); ok {
						if t, _ := p["text"].(string); t != "" {
							totalChars += len([]rune(t))
						}
					}
				}
			}
		}
	}
	if totalChars < threshold {
		return stdReq, nil
	}

	fileText := promptcompat.BuildOpenAICurrentInputContextTranscript(stdReq.Messages)
	if strings.TrimSpace(fileText) == "" {
		return stdReq, errors.New("current user input file produced empty transcript")
	}
	toolsText, toolNames := promptcompat.BuildOpenAIToolsContextTranscriptForMessages(stdReq.ToolsRaw, stdReq.ToolChoice, stdReq.Messages)
	hasToolsFile := strings.TrimSpace(toolsText) != ""
	promptText := currentInputFilePrompt(hasToolsFile)
	messages := []any{
		map[string]any{
			"role":    "user",
			"content": promptText,
		},
	}
	if imageParts := latestUserImageParts(stdReq.Messages); len(imageParts) > 0 {
		content := make([]any, 0, len(imageParts)+1)
		content = append(content, map[string]string{"type": "text", "text": promptText})
		content = append(content, imageParts...)
		messages[0].(map[string]any)["content"] = content
	}
	stdReq.Messages = messages
	stdReq.HistoryText = fileText
	stdReq.ToolsText = toolsText
	stdReq.CurrentInputFileApplied = true
	stdReq.CurrentInputFileTruncated = true
	stdReq.FinalPrompt = promptText
	stdReq.ToolNames = toolNames
	tokenParts := []string{fileText}
	if strings.TrimSpace(toolsText) != "" {
		tokenParts = append(tokenParts, toolsText)
	}
	tokenParts = append(tokenParts, stdReq.FinalPrompt)
	stdReq.PromptTokenText = strings.Join(tokenParts, "\n")
	return stdReq, nil
}
