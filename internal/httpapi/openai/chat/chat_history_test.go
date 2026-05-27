package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"tool-gateway/internal/chathistory"
	"tool-gateway/internal/config"
	"tool-gateway/internal/llm"
	"tool-gateway/internal/promptcompat"
)

func asTestString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func newTestChatHistoryStore(t *testing.T) *chathistory.Store {
	t.Helper()
	store := chathistory.New(filepath.Join(t.TempDir(), "chat_history.json"))
	if err := store.Err(); err != nil {
		t.Fatalf("chat history store unavailable: %v", err)
	}
	return store
}

func blockChatHistoryDetailDir(t *testing.T, detailDir string) func() {
	t.Helper()
	blockedDir := detailDir + ".blocked"
	if err := os.RemoveAll(blockedDir); err != nil {
		t.Fatalf("remove blocked detail dir failed: %v", err)
	}
	if err := os.Rename(detailDir, blockedDir); err != nil {
		t.Fatalf("move detail dir aside failed: %v", err)
	}
	if err := os.RemoveAll(detailDir); err != nil {
		t.Fatalf("remove blocked detail path failed: %v", err)
	}
	if err := os.WriteFile(detailDir, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("write blocked detail path failed: %v", err)
	}
	var once sync.Once
	return func() {
		t.Helper()
		once.Do(func() {
			if err := os.RemoveAll(detailDir); err != nil {
				t.Fatalf("remove blocking detail path failed: %v", err)
			}
			if err := os.Rename(blockedDir, detailDir); err != nil {
				t.Fatalf("restore detail dir failed: %v", err)
			}
		})
	}
}

func TestChatCompletionsNonStreamPersistsHistory(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		Backend:     streamStatusBackendStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"system","content":"be precise"},{"role":"user","content":"hi there"},{"role":"assistant","content":"previous answer"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.Status != "success" || item.UserInput != "hi there" {
		t.Fatalf("unexpected persisted history summary: %#v", item)
	}
	full, err := historyStore.Get(item.ID)
	if err != nil {
		t.Fatalf("expected detail item, got %v", err)
	}
	if full.Content != "hello world" {
		t.Fatalf("expected detail content persisted, got %#v", full)
	}
	if len(full.Messages) != 3 {
		t.Fatalf("expected all request messages persisted, got %#v", full.Messages)
	}
	if full.FinalPrompt == "" {
		t.Fatalf("expected final prompt to be persisted")
	}
	if item.Surface != "openai.chat_completions" {
		t.Fatalf("expected surface openai.chat_completions, got %q", item.Surface)
	}
}

func TestChatHistoryNonStreamArchivesRawToolCallMarkup(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		UserInput: "call tool",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now().Add(-time.Second),
		finalPrompt: "call tool",
	}
	rawToolCall := `<tool_calls><invoke name="search"><parameter name="q">golang</parameter></invoke></tool_calls>`

	h := &Handler{}
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":`+strconv.Quote(rawToolCall)+`}`, `data: [DONE]`)
	h.handleNonStream(rec, resp, "cid-tool-history", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, session)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	full, err := historyStore.Get(entry.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Content != rawToolCall {
		t.Fatalf("expected raw tool markup archived, got %q", full.Content)
	}
	if full.FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %#v", full.FinishReason)
	}
}

func TestChatHistoryStreamArchivesRawToolCallMarkup(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		Stream:    true,
		UserInput: "call tool",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now().Add(-time.Second),
		finalPrompt: "call tool",
	}
	rawToolCall := `<tool_calls><invoke name="search"><parameter name="q">golang</parameter></invoke></tool_calls>`

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":`+strconv.Quote(rawToolCall)+`}`, `data: [DONE]`)
	h.handleStream(rec, req, resp, "cid-stream-tool-history", "deepseek-v4-flash", "prompt", 0, false, false, []string{"search"}, nil, session)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	full, err := historyStore.Get(entry.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Content != rawToolCall {
		t.Fatalf("expected raw streamed tool markup archived, got %q", full.Content)
	}
	if full.FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %#v", full.FinishReason)
	}
}

func TestStartChatHistoryRecoversFromTransientWriteFailure(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	restore := blockChatHistoryDetailDir(t, historyStore.DetailDir())
	t.Cleanup(restore)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	stdReq := promptcompat.StandardRequest{
		ResponseModel: "deepseek-v4-flash",
		Stream:        true,
		Messages: []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		FinalPrompt: "hello",
	}

	session := startChatHistory(historyStore, req, stdReq)
	if session == nil {
		t.Fatalf("expected session even when initial persistence fails")
		return
	}
	if session.disabled {
		t.Fatalf("expected session to remain active after transient start failure")
	}
	if session.entryID == "" {
		t.Fatalf("expected session entry id to be retained")
	}
	if err := historyStore.Err(); err != nil {
		t.Fatalf("transient start failure should not latch store error: %v", err)
	}

	session.lastPersist = time.Now().Add(-time.Second)
	session.progress("thinking", "partial")
	if session.disabled {
		t.Fatalf("expected session to remain active after transient update failure")
	}
	if session.entryID == "" {
		t.Fatalf("expected session entry id to remain set after update failure")
	}
	if err := historyStore.Err(); err != nil {
		t.Fatalf("transient update failure should not latch store error: %v", err)
	}

	restore()

	session.success(http.StatusOK, "thinking", "final answer", "stop", map[string]any{"total_tokens": 7})
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed after restore: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one persisted item after restore, got %#v", snapshot.Items)
	}
	full, err := historyStore.Get(session.entryID)
	if err != nil {
		t.Fatalf("get restored entry failed: %v", err)
	}
	if full.Status != "success" || full.Content != "final answer" {
		t.Fatalf("expected restored entry to persist final success, got %#v", full)
	}
}

func TestHandleStreamContextCancelledMarksHistoryStopped(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	entry, err := historyStore.Start(chathistory.StartParams{
		CallerID:  "caller:test",
		Model:     "deepseek-v4-flash",
		Stream:    true,
		UserInput: "hello",
	})
	if err != nil {
		t.Fatalf("start history failed: %v", err)
	}
	session := &chatHistorySession{
		store:       historyStore,
		entryID:     entry.ID,
		startedAt:   time.Now(),
		lastPersist: time.Now(),
		finalPrompt: "hello",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	resp := makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello"}`, `data: [DONE]`)

	h.handleStream(rec, req, resp, "cid-stop", "deepseek-v4-flash", "prompt", 0, false, false, nil, nil, session)

	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	full, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	if full.Status != "stopped" {
		t.Fatalf("expected stopped status, got %#v", full)
	}
}

func TestChatCompletionsRecordsAdminWebUISource(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		Backend:     streamStatusBackendStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi there"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tool-Gateway-Source", "admin-webui-api-tester")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected admin webui source to be recorded, got %#v", snapshot.Items)
	}
}

func TestChatCompletionsSkipsHistoryWhenDisabled(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	if _, err := historyStore.SetLimit(chathistory.DisabledLimit); err != nil {
		t.Fatalf("disable history store failed: %v", err)
	}
	h := &Handler{
		Store:       mockOpenAIConfig{},
		Auth:        streamStatusAuthStub{},
		Backend:     streamStatusBackendStub{resp: makeOpenAISSEHTTPResponse(`data: {"p":"response/content","v":"hello world"}`, `data: [DONE]`)},
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi there"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 0 {
		t.Fatalf("expected disabled history to stay empty, got %#v", snapshot.Items)
	}
}

func TestChatCompletionsExternalKimiUploadsContextAndForwardsImage(t *testing.T) {
	uploadNames := []string{}
	var seenChat map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/files":
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("parse upload: %v", err)
			}
			_, header, err := req.FormFile("file")
			if err != nil {
				t.Fatalf("expected uploaded file: %v", err)
			}
			uploadNames = append(uploadNames, header.Filename)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"file-` + strings.TrimSuffix(header.Filename, ".txt") + `"}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(req.Body).Decode(&seenChat); err != nil {
				t.Fatalf("decode chat: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
	}))
	defer server.Close()
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{"external_ai":{"base_url":"`+server.URL+`/v1","api_key":"local-key","model":"kimi-k2.5"}}`)
	store := config.LoadStore()
	backend := llm.NewOpenAIAdapter(store)
	historyStore := newTestChatHistoryStore(t)
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth:        streamStatusAuthStub{},
		Backend:     backend,
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"kimi-k2.5","messages":[{"role":"user","content":[{"type":"text","text":"what is this image?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="}}]}],"tools":[{"type":"function","function":{"name":"search","description":"Search docs","parameters":{"type":"object"}}}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(uploadNames) != 2 || uploadNames[0] != "TOOL_GATEWAY_HISTORY.txt" || uploadNames[1] != "image.png" {
		t.Fatalf("expected history and image file uploads without unrelated tools file, got %#v", uploadNames)
	}
	fileIDs, _ := seenChat["file_ids"].([]any)
	if len(fileIDs) != 2 || fileIDs[0] != "file-TOOL_GATEWAY_HISTORY" || fileIDs[1] != "file-image.png" {
		t.Fatalf("expected history and image file IDs in chat request, got %#v", seenChat["file_ids"])
	}
	messages := seenChat["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	continuationCount := 0
	for _, raw := range content {
		part, _ := raw.(map[string]any)
		if part["type"] == "image_url" {
			t.Fatalf("did not expect image_url in chat request, got %#v", messages)
		}
		if part["type"] == "text" && strings.Contains(asTestString(part["text"]), "Continue from the latest state") {
			continuationCount++
		}
	}
	if continuationCount != 1 {
		t.Fatalf("expected exactly one continuation prompt in upstream chat content, got %d content=%#v", continuationCount, content)
	}
}

func TestChatCompletionsCurrentInputFilePersistsNeutralPrompt(t *testing.T) {
	historyStore := newTestChatHistoryStore(t)
	ds := &inlineUploadBackendStub{}
	h := &Handler{
		Store: mockOpenAIConfig{
			currentInputEnabled: true,
		},
		Auth:        streamStatusAuthStub{},
		Backend:     ds,
		ChatHistory: historyStore,
	}

	reqBody := `{"model":"deepseek-v4-flash","messages":[{"role":"system","content":"system instructions"},{"role":"user","content":"first user turn"},{"role":"assistant","content":"","reasoning_content":"hidden reasoning","tool_calls":[{"name":"search","arguments":{"query":"docs"}}]},{"role":"tool","name":"search","tool_call_id":"call-1","content":"tool result"},{"role":"user","content":"latest user turn"}],"stream":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer direct-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ChatCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	snapshot, err := historyStore.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(snapshot.Items))
	}
	full, err := historyStore.Get(snapshot.Items[0].ID)
	if err != nil {
		t.Fatalf("expected detail item, got %v", err)
	}
	if len(ds.uploadCalls) != 1 {
		t.Fatalf("expected current input upload to happen, got %d", len(ds.uploadCalls))
	}
	if ds.uploadCalls[0].Filename != "TOOL_GATEWAY_HISTORY.txt" {
		t.Fatalf("expected TOOL_GATEWAY_HISTORY.txt upload, got %q", ds.uploadCalls[0].Filename)
	}
	if full.HistoryText != string(ds.uploadCalls[0].Data) {
		t.Fatalf("expected uploaded current input file to be persisted in history text")
	}
	if len(full.Messages) != 1 {
		t.Fatalf("expected continuation prompt to be the only persisted message, got %#v", full.Messages)
	}
	if !strings.Contains(full.Messages[0].Content, "Continue from the latest state in the attached TOOL_GATEWAY_HISTORY.txt context.") {
		t.Fatalf("expected continuation prompt to be persisted, got %#v", full.Messages[0])
	}
}
