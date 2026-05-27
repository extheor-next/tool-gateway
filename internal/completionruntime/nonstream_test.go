package completionruntime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	dsclient "tool-gateway/internal/deepseek/client"
	"tool-gateway/internal/promptcompat"
)

type fakeCompletionBackend struct {
	responses []*http.Response
	payloads  []map[string]any
	uploads   []dsclient.UploadFileRequest
}

type currentInputRuntimeConfig struct{}

type externalFakeCompletionBackend struct {
	fakeCompletionBackend
}

func (externalFakeCompletionBackend) ExternalAIAdapter() bool { return true }

func (currentInputRuntimeConfig) CurrentInputFileEnabled() bool        { return true }
func (currentInputRuntimeConfig) CurrentInputFileMinChars() int        { return 0 }
func (currentInputRuntimeConfig) CurrentInputFileMaxKeepMessages() int { return 0 }

func (f *fakeCompletionBackend) CreateSession(context.Context, int) (string, error) {
	return "session-1", nil
}

func (f *fakeCompletionBackend) GetPow(context.Context, int) (string, error) {
	return "pow", nil
}

func (f *fakeCompletionBackend) UploadFile(_ context.Context, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	f.uploads = append(f.uploads, req)
	return &dsclient.UploadFileResult{ID: "file-runtime-1"}, nil
}

func (f *fakeCompletionBackend) CallCompletion(_ context.Context, payload map[string]any, _ string) (*http.Response, error) {
	f.payloads = append(f.payloads, payload)
	if len(f.responses) == 0 {
		return sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"fallback"}`), nil
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

func TestExecuteNonStreamWithRetryBuildsCanonicalTurn(t *testing.T) {
	ds := &fakeCompletionBackend{responses: []*http.Response{sseHTTPResponse(
		http.StatusOK,
		`data: {"response_message_id":42,"p":"response/content","v":"<tool_calls><invoke name=\"Write\"><parameter name=\"content\">{\"x\":1}</parameter></invoke></tool_calls>"}`,
	)}}
	stdReq := promptcompat.StandardRequest{
		Surface:         "test",
		ResponseModel:   "deepseek-v4-flash",
		PromptTokenText: "prompt",
		FinalPrompt:     "final prompt",
		ToolNames:       []string{"Write"},
		ToolsRaw: []any{map[string]any{
			"name": "Write",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string"},
				},
			},
		}},
	}

	result, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, stdReq, Options{})
	if outErr != nil {
		t.Fatalf("unexpected output error: %#v", outErr)
	}
	if result.SessionID != "session-1" {
		t.Fatalf("session mismatch: %q", result.SessionID)
	}
	if got := result.Turn.ResponseMessageID; got != 42 {
		t.Fatalf("response message id mismatch: %d", got)
	}
	if len(result.Turn.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(result.Turn.ToolCalls))
	}
	if _, ok := result.Turn.ToolCalls[0].Input["content"].(string); !ok {
		t.Fatalf("expected schema-normalized string argument, got %#v", result.Turn.ToolCalls[0].Input["content"])
	}
	if result.Turn.Usage.InputTokens == 0 || result.Turn.Usage.TotalTokens == 0 {
		t.Fatalf("expected usage to be populated, got %#v", result.Turn.Usage)
	}
}

func TestExecuteNonStreamWithRetryUsesParentMessageForEmptyRetry(t *testing.T) {
	ds := &fakeCompletionBackend{responses: []*http.Response{
		sseHTTPResponse(http.StatusOK, `data: {"response_message_id":77,"p":"response/thinking_content","v":"plan"}`),
		sseHTTPResponse(http.StatusOK, `data: {"response_message_id":78,"p":"response/content","v":"ok"}`),
	}}
	stdReq := promptcompat.StandardRequest{
		Surface:         "test",
		ResponseModel:   "deepseek-v4-flash",
		PromptTokenText: "prompt",
		FinalPrompt:     "final prompt",
	}

	result, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, stdReq, Options{RetryEnabled: true})
	if outErr != nil {
		t.Fatalf("unexpected output error: %#v", outErr)
	}
	if result.Attempts != 1 {
		t.Fatalf("expected one retry, got %d", result.Attempts)
	}
	if len(ds.payloads) != 2 {
		t.Fatalf("expected two completion calls, got %d", len(ds.payloads))
	}
	if got := ds.payloads[1]["parent_message_id"]; got != 77 {
		t.Fatalf("retry parent_message_id mismatch: %#v", got)
	}
	if result.Turn.Text != "ok" {
		t.Fatalf("retry text mismatch: %q", result.Turn.Text)
	}
}

func TestExecuteNonStreamWithRetryConvertsReferenceMarkers(t *testing.T) {
	ds := &fakeCompletionBackend{responses: []*http.Response{sseHTTPResponse(
		http.StatusOK,
		`data: {"p":"response/content","v":"答案[reference:0]。","citation":{"cite_index":0,"url":"https://example.com/ref"}}`,
	)}}
	stdReq := promptcompat.StandardRequest{
		Surface:         "test",
		ResponseModel:   "deepseek-v4-flash-search",
		PromptTokenText: "prompt",
		FinalPrompt:     "final prompt",
		Search:          true,
	}

	result, outErr := ExecuteNonStreamWithRetry(context.Background(), ds, stdReq, Options{})
	if outErr != nil {
		t.Fatalf("unexpected output error: %#v", outErr)
	}
	want := "答案[0](https://example.com/ref)。"
	if result.Turn.Text != want {
		t.Fatalf("text mismatch: got %q want %q", result.Turn.Text, want)
	}
}

func TestStartCompletionAppliesCurrentInputFileGlobally(t *testing.T) {
	ds := &fakeCompletionBackend{responses: []*http.Response{sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"ok"}`)}}
	stdReq := promptcompat.StandardRequest{
		Surface:         "test_adapter",
		RequestedModel:  "deepseek-v4-flash",
		ResolvedModel:   "deepseek-v4-flash",
		ResponseModel:   "deepseek-v4-flash",
		PromptTokenText: "first user turn",
		FinalPrompt:     "first user turn",
		Messages: []any{
			map[string]any{"role": "user", "content": "first user turn"},
		},
	}

	start, outErr := StartCompletion(context.Background(), ds, stdReq, Options{
		CurrentInputFile: currentInputRuntimeConfig{},
	})
	if outErr != nil {
		t.Fatalf("unexpected output error: %#v", outErr)
	}
	if len(ds.uploads) != 1 {
		t.Fatalf("expected current input upload, got %d", len(ds.uploads))
	}
	if got := ds.uploads[0].Filename; got != "TOOL_GATEWAY_HISTORY.txt" {
		t.Fatalf("upload filename=%q want TOOL_GATEWAY_HISTORY.txt", got)
	}
	if len(ds.payloads) != 1 {
		t.Fatalf("expected one completion payload, got %d", len(ds.payloads))
	}
	refIDs, _ := ds.payloads[0]["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-runtime-1" {
		t.Fatalf("expected uploaded file id in ref_file_ids, got %#v", ds.payloads[0]["ref_file_ids"])
	}
	prompt, _ := ds.payloads[0]["prompt"].(string)
	if !strings.Contains(prompt, "Continue from the latest state in the attached TOOL_GATEWAY_HISTORY.txt context.") {
		t.Fatalf("expected continuation prompt, got %q", prompt)
	}
	if !start.Request.CurrentInputFileApplied || !strings.Contains(start.Request.PromptTokenText, "# TOOL_GATEWAY_HISTORY.txt") {
		t.Fatalf("expected prepared request to carry current input file state, got %#v", start.Request)
	}
}

func TestStartCompletionExternalCurrentInputCarriesContextAndImages(t *testing.T) {
	ds := &externalFakeCompletionBackend{fakeCompletionBackend: fakeCompletionBackend{responses: []*http.Response{sseHTTPResponse(http.StatusOK, `data: {"p":"response/content","v":"ok"}`)}}}
	stdReq := promptcompat.StandardRequest{
		Surface:         "test_adapter",
		RequestedModel:  "kimi-k2.5",
		ResolvedModel:   "kimi-k2.5",
		ResponseModel:   "kimi-k2.5",
		PromptTokenText: "what is this image?",
		FinalPrompt:     "what is this image?",
		Messages: []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "uploaded image"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aW1hZ2U="}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "read this file and explain the image"},
			}},
		},
		ToolsRaw: []any{map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "Read",
				"description": "Read file contents from disk",
				"parameters":  map[string]any{"type": "object"},
			},
		}},
	}

	start, outErr := StartCompletion(context.Background(), ds, stdReq, Options{
		CurrentInputFile: currentInputRuntimeConfig{},
	})
	if outErr != nil {
		t.Fatalf("unexpected output error: %#v", outErr)
	}
	if len(ds.uploads) != 0 {
		t.Fatalf("external adapter should upload context later, got runtime uploads %#v", ds.uploads)
	}
	payload := ds.payloads[0]
	if !strings.Contains(payload["history_text"].(string), "# TOOL_GATEWAY_HISTORY.txt") {
		t.Fatalf("expected history_text in payload, got %#v", payload["history_text"])
	}
	toolsText := payload["tools_text"].(string)
	if !strings.Contains(toolsText, "# TOOL_GATEWAY_TOOLS.txt") {
		t.Fatalf("expected tools_text in payload, got %#v", payload["tools_text"])
	}
	if !strings.Contains(toolsText, "TOOL CALL FORMAT") || !strings.Contains(toolsText, "<|DSML|tool_calls>") {
		t.Fatalf("expected tool-call rules in tools_text, got %#v", payload["tools_text"])
	}
	prompt := payload["prompt"].(string)
	if strings.Contains(prompt, "TOOL CALL FORMAT") || strings.Contains(prompt, "<|DSML|tool_calls>") {
		t.Fatalf("expected short prompt without inline tool-call rules, got %q", prompt)
	}
	if strings.Contains(prompt, "Output integrity guard") || strings.Contains(prompt, "<|begin▁of▁sentence|>") || strings.Contains(prompt, "<|User|><|Assistant|>") {
		t.Fatalf("expected external current-input prompt to skip full prompt template, got %q", prompt)
	}
	messages := payload["request_messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	foundImage := false
	for _, raw := range content {
		part, _ := raw.(map[string]any)
		if part["type"] == "image_url" {
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatalf("expected image_url part to remain in request_messages, got %#v", messages)
	}
	if !start.Request.CurrentInputFileApplied || start.Request.HistoryText == "" {
		t.Fatalf("expected prepared external request to carry current input state, got %#v", start.Request)
	}
}

func sseHTTPResponse(status int, lines ...string) *http.Response {
	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
