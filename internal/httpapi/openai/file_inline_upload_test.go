package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/auth"
	dsclient "tool-gateway/internal/deepseek/client"
)

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

func TestPreprocessInlineFileInputsReplacesDataURLAndCollectsRefFileIDs(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Backend: ds}
	req := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="},
					},
				},
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := h.preprocessInlineFileInputs(ctx, &auth.RequestAuth{CallerID: "caller:test"}, req); err != nil {
		t.Fatalf("preprocess failed: %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].ModelType != "default" {
		t.Fatalf("expected default model type when request omits model, got %q", ds.uploadCalls[0].ModelType)
	}
	if ds.lastCtx != ctx {
		t.Fatalf("expected upload to use request context")
	}
	if ds.uploadCalls[0].ContentType != "image/png" {
		t.Fatalf("expected image/png, got %q", ds.uploadCalls[0].ContentType)
	}
	if ds.uploadCalls[0].Filename != "image.png" {
		t.Fatalf("expected inferred filename image.png, got %q", ds.uploadCalls[0].Filename)
	}
	messages, _ := req["messages"].([]any)
	first, _ := messages[0].(map[string]any)
	content, _ := first["content"].([]any)
	block, _ := content[0].(map[string]any)
	if block["type"] != "input_image" {
		t.Fatalf("expected input_image replacement, got %#v", block)
	}
	if block["file_id"] != "file-inline-1" {
		t.Fatalf("expected file-inline-1 replacement id, got %#v", block)
	}
	refIDs, _ := req["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-inline-1" {
		t.Fatalf("unexpected ref_file_ids: %#v", req["ref_file_ids"])
	}
}

func TestPreprocessInlineFileInputsDeduplicatesIdenticalPayloads(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Backend: ds}
	req := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="}},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="}},
				},
			},
		},
	}

	if err := h.preprocessInlineFileInputs(context.Background(), &auth.RequestAuth{CallerID: "caller:test"}, req); err != nil {
		t.Fatalf("preprocess failed: %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected deduplicated single upload, got %d", len(ds.uploadCalls))
	}
	refIDs, _ := req["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-inline-1" {
		t.Fatalf("unexpected ref_file_ids after dedupe: %#v", req["ref_file_ids"])
	}
}

type inlinePassthroughBackendStub struct {
	inlineUploadBackendStub
}

func (m *inlinePassthroughBackendStub) PreserveInlineFileInputs() bool {
	return true
}

func TestChatCompletionsPreservesImageURLForInlinePassthroughBackend(t *testing.T) {
	ds := &inlinePassthroughBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	reqBody := `{"model":"kimi-k2.5","messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 0 {
		t.Fatalf("expected image_url passthrough without upload, got %d upload calls", len(ds.uploadCalls))
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	messages, _ := ds.completionReq["request_messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected one request message, got %#v", ds.completionReq["request_messages"])
	}
	message, _ := messages[0].(map[string]any)
	content, _ := message["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("expected two content parts, got %#v", message["content"])
	}
	image, _ := content[1].(map[string]any)
	if image["type"] != "image_url" {
		t.Fatalf("expected image_url part to be preserved, got %#v", image)
	}
}

func TestChatCompletionsUploadsInlineFilesBeforeCompletion(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	reqBody := `{"model":"deepseek-v4-vision","messages":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].ModelType != "vision" {
		t.Fatalf("expected vision model type for vision request, got %q", ds.uploadCalls[0].ModelType)
	}
	if ds.completionReq == nil {
		t.Fatal("expected completion payload to be captured")
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-inline-1" {
		t.Fatalf("unexpected completion ref_file_ids: %#v", ds.completionReq["ref_file_ids"])
	}
}

func TestResponsesUploadsInlineFilesBeforeCompletion(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody := `{"model":"deepseek-v4-pro","input":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].ModelType != "expert" {
		t.Fatalf("expected expert model type for pro request, got %q", ds.uploadCalls[0].ModelType)
	}
	refIDs, _ := ds.completionReq["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-inline-1" {
		t.Fatalf("unexpected completion ref_file_ids: %#v", ds.completionReq["ref_file_ids"])
	}
}

func TestChatCompletionsInlineUploadFailureReturnsBadRequest(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,%%%"}}]}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.completionReq != nil {
		t.Fatalf("did not expect completion call on upload decode error")
	}
}

func TestChatCompletionsInlineUploadLimitReturnsBadRequest(t *testing.T) {
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	content := []any{map[string]any{"type": "input_text", "text": "hi"}}
	for i := 0; i < 51; i++ {
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": "data:image/png;base64,QUJDRA=="},
		})
	}
	body, err := json.Marshal(map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []any{map[string]any{
			"role":    "user",
			"content": content,
		}},
		"stream": false,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "exceeded maximum of 50 inline files per request") {
		t.Fatalf("expected inline file limit error, got body=%s", rec.Body.String())
	}
	if ds.completionReq != nil {
		t.Fatalf("did not expect completion call after inline file limit error")
	}
}

func TestResponsesInlineUploadFailureReturnsInternalServerError(t *testing.T) {
	ds := &inlineUploadBackendStub{uploadErr: errors.New("boom")}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody := `{"model":"deepseek-v4-flash","input":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ds.completionReq != nil {
		t.Fatalf("did not expect completion call after upload failure")
	}
}

func TestVercelPrepareUploadsInlineFilesBeforeLeasePayload(t *testing.T) {
	t.Setenv("VERCEL", "1")
	t.Setenv("TOOL_GATEWAY_VERCEL_INTERNAL_SECRET", "stream-secret")
	ds := &inlineUploadBackendStub{}
	h := &openAITestSurface{Store: mockOpenAIConfig{}, Auth: streamStatusAuthStub{}, Backend: ds}
	r := chi.NewRouter()
	registerOpenAITestRoutes(r, h)
	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QUJDRA=="}}]}],"stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?__stream_prepare=1", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("X-Tool-Gateway-Internal-Token", "stream-secret")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected 1 upload call, got %d", len(ds.uploadCalls))
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	payload, _ := out["payload"].(map[string]any)
	if payload == nil {
		t.Fatalf("expected payload in prepare response, got %#v", out)
	}
	refIDs, _ := payload["ref_file_ids"].([]any)
	if len(refIDs) != 1 || refIDs[0] != "file-inline-1" {
		t.Fatalf("unexpected payload ref_file_ids: %#v", payload["ref_file_ids"])
	}
}
