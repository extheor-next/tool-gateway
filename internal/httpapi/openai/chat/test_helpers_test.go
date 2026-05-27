package chat

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"tool-gateway/internal/auth"
	dsclient "tool-gateway/internal/deepseek/client"
)

type mockOpenAIConfig struct {
	aliases             map[string]string
	autoDeleteMode      string
	toolMode            string
	earlyEmit           string
	responsesTTL        int
	embedProv           string
	currentInputEnabled bool
	currentInputMin     int
	currentInputMaxKeep int
	thinkingInjection   *bool
	thinkingPrompt      string
}

func (m mockOpenAIConfig) ModelAliases() map[string]string     { return m.aliases }
func (m mockOpenAIConfig) ToolcallMode() string                { return m.toolMode }
func (m mockOpenAIConfig) ToolcallEarlyEmitConfidence() string { return m.earlyEmit }
func (m mockOpenAIConfig) ResponsesStoreTTLSeconds() int       { return m.responsesTTL }
func (m mockOpenAIConfig) EmbeddingsProvider() string          { return m.embedProv }
func (m mockOpenAIConfig) AutoDeleteMode() string {
	if m.autoDeleteMode == "" {
		return "none"
	}
	return m.autoDeleteMode
}
func (m mockOpenAIConfig) AutoDeleteSessions() bool      { return false }
func (m mockOpenAIConfig) CurrentInputFileEnabled() bool { return m.currentInputEnabled }
func (m mockOpenAIConfig) CurrentInputFileMinChars() int {
	return m.currentInputMin
}
func (m mockOpenAIConfig) CurrentInputFileMaxKeepMessages() int {
	return m.currentInputMaxKeep
}
func (m mockOpenAIConfig) ThinkingInjectionEnabled() bool {
	if m.thinkingInjection == nil {
		return false
	}
	return *m.thinkingInjection
}
func (m mockOpenAIConfig) ThinkingInjectionPrompt() string { return m.thinkingPrompt }

type streamStatusAuthStub struct{}

func (streamStatusAuthStub) Determine(_ *http.Request) (*auth.RequestAuth, error) {
	return &auth.RequestAuth{CallerID: "caller:test"}, nil
}

func (streamStatusAuthStub) DetermineCaller(_ *http.Request) (*auth.RequestAuth, error) {
	return (&streamStatusAuthStub{}).Determine(nil)
}

type streamStatusBackendStub struct {
	resp *http.Response
}

func (m streamStatusBackendStub) CreateSession(_ context.Context, _ int) (string, error) {
	return "session-id", nil
}

func (m streamStatusBackendStub) GetPow(_ context.Context, _ int) (string, error) {
	return "pow", nil
}

func (m streamStatusBackendStub) UploadFile(_ context.Context, _ dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	return &dsclient.UploadFileResult{ID: "file-id", Filename: "file.txt", Bytes: 1, Status: "uploaded"}, nil
}

func (m streamStatusBackendStub) CallCompletion(_ context.Context, _ map[string]any, _ string) (*http.Response, error) {
	return m.resp, nil
}

func (m streamStatusBackendStub) DeleteSession(_ context.Context, _ string, _ int) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (m streamStatusBackendStub) DeleteAllSessions(_ context.Context) error {
	return nil
}

func makeOpenAISSEHTTPResponse(lines ...string) *http.Response {
	body := strings.Join(lines, "\n")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type inlineUploadBackendStub struct {
	uploadCalls    []dsclient.UploadFileRequest
	lastCtx        context.Context
	completionReq  map[string]any
	createSession  string
	uploadErr      error
	completionResp *http.Response
}

func (m *inlineUploadBackendStub) CreateSession(_ context.Context, _ int) (string, error) {
	if strings.TrimSpace(m.createSession) == "" {
		return "session-id", nil
	}
	return m.createSession, nil
}

func (m *inlineUploadBackendStub) GetPow(_ context.Context, _ int) (string, error) {
	return "pow", nil
}

func (m *inlineUploadBackendStub) UploadFile(ctx context.Context, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	m.lastCtx = ctx
	m.uploadCalls = append(m.uploadCalls, req)
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	id := "file-inline-1"
	if len(m.uploadCalls) > 1 {
		id = "file-inline-" + fmt.Sprint(len(m.uploadCalls))
	}
	return &dsclient.UploadFileResult{
		ID:       id,
		Filename: req.Filename,
		Bytes:    int64(len(req.Data)),
		Status:   "uploaded",
		Purpose:  req.Purpose,
	}, nil
}

func (m *inlineUploadBackendStub) CallCompletion(_ context.Context, payload map[string]any, _ string) (*http.Response, error) {
	m.completionReq = payload
	if m.completionResp != nil {
		return m.completionResp, nil
	}
	return makeOpenAISSEHTTPResponse(
		`data: {"p":"response/content","v":"ok"}`,
		`data: [DONE]`,
	), nil
}

func (m *inlineUploadBackendStub) DeleteSession(_ context.Context, _ string, _ int) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (m *inlineUploadBackendStub) DeleteAllSessions(_ context.Context) error {
	return nil
}

func (m *inlineUploadBackendStub) ExternalAIAdapter() bool { return false }

func historySplitTestMessages() []any {
	toolCalls := []any{
		map[string]any{
			"name":      "search",
			"arguments": map[string]any{"query": "docs"},
		},
	}
	return []any{
		map[string]any{"role": "system", "content": "system instructions"},
		map[string]any{"role": "user", "content": "first user turn"},
		map[string]any{
			"role":              "assistant",
			"content":           "",
			"reasoning_content": "hidden reasoning",
			"tool_calls":        toolCalls,
		},
		map[string]any{
			"role":         "tool",
			"name":         "search",
			"tool_call_id": "call-1",
			"content":      "tool result",
		},
		map[string]any{"role": "user", "content": "latest user turn"},
	}
}
