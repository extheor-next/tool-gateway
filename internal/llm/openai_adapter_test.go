package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tool-gateway/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestChatCompletionsURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "root", base: "https://example.com", want: "https://example.com/v1/chat/completions"},
		{name: "v1", base: "https://example.com/v1", want: "https://example.com/v1/chat/completions"},
		{name: "existing", base: "https://example.com/v1/chat/completions", want: "https://example.com/v1/chat/completions"},
		{name: "custom path", base: "https://example.com/proxy", want: "https://example.com/proxy/v1/chat/completions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chatCompletionsURL(tc.base)
			if err != nil {
				t.Fatalf("chatCompletionsURL() error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestOpenAIAdapterCallCompletionBuildsRequest(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"external_ai":{
			"base_url":"https://upstream.example/custom/v1",
			"api_key":"store-key",
			"model":"gpt-test",
			"headers":{"X-Test-Header":"yes"}
		}
	}`)
	store := config.LoadStore()
	var seenReq *http.Request
	var seenBody map[string]any
	adapter := NewOpenAIAdapter(store)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenReq = req
		if err := json.NewDecoder(req.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"plan\",\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")),
		}, nil
	})}

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{
		"prompt":      "hello",
		"temperature": 0.2,
	}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()

	if seenReq == nil {
		t.Fatal("expected upstream request")
	}
	if got := seenReq.URL.String(); got != "https://upstream.example/custom/v1/chat/completions" {
		t.Fatalf("unexpected URL: %s", got)
	}
	if got := seenReq.Header.Get("Authorization"); got != "Bearer store-key" {
		t.Fatalf("unexpected auth header: %q", got)
	}
	if got := seenReq.Header.Get("X-Test-Header"); got != "yes" {
		t.Fatalf("unexpected custom header: %q", got)
	}
	if seenBody["model"] != "gpt-test" || seenBody["stream"] != true || seenBody["temperature"] != 0.2 {
		t.Fatalf("unexpected request body: %#v", seenBody)
	}
	messages, _ := seenBody["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected one message, got %#v", seenBody["messages"])
	}
	msg, _ := messages[0].(map[string]any)
	if msg["role"] != "user" || msg["content"] != "hello" {
		t.Fatalf("unexpected message: %#v", msg)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read converted stream: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"p":"response/thinking_content"`) || !strings.Contains(text, `"v":"plan"`) {
		t.Fatalf("expected reasoning delta in converted stream, got %s", text)
	}
	if !strings.Contains(text, `"p":"response/content"`) || !strings.Contains(text, `"v":"ok"`) {
		t.Fatalf("expected content delta in converted stream, got %s", text)
	}
}

func TestOpenAIAdapterUploadFileToProviderSendsPurposeAndContentType(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"external_ai":{
			"base_url":"https://upstream.example/custom/v1",
			"api_key":"store-key",
			"headers":{"X-Test-Header":"yes"}
		}
	}`)
	store := config.LoadStore()
	var seenReq *http.Request
	adapter := NewOpenAIAdapter(store)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenReq = req
		if got := req.URL.String(); got != "https://upstream.example/custom/v1/files" {
			t.Fatalf("unexpected URL: %s", got)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer store-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := req.Header.Get("X-Test-Header"); got != "yes" {
			t.Fatalf("unexpected custom header: %q", got)
		}
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parse multipart request: %v", err)
		}
		if got := req.FormValue("purpose"); got != "vision" {
			t.Fatalf("unexpected purpose: %q", got)
		}
		file, header, err := req.FormFile("file")
		if err != nil {
			t.Fatalf("expected file part: %v", err)
		}
		defer file.Close()
		if got := header.Filename; got != "image.png" {
			t.Fatalf("unexpected filename: %q", got)
		}
		if got := header.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("unexpected file content type: %q", got)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file part: %v", err)
		}
		if string(data) != "png-data" {
			t.Fatalf("unexpected file data: %q", string(data))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"id":"file-provider-1"}`)),
		}, nil
	})}

	fileID, err := adapter.UploadFileToProviderWithPurpose(context.Background(), "image.png", []byte("png-data"), "vision", "image/png")
	if err != nil {
		t.Fatalf("UploadFileToProviderWithPurpose() error: %v", err)
	}
	if seenReq == nil {
		t.Fatal("expected upstream upload request")
	}
	if fileID != "file-provider-1" {
		t.Fatalf("unexpected file id: %q", fileID)
	}
}

func TestOpenAIAdapterKimiDoesNotForwardSamplingParams(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"external_ai":{
			"base_url":"https://kimi.local/v1",
			"api_key":"store-key",
			"model":"kimi-k2.5"
		}
	}`)
	store := config.LoadStore()
	var seenBody map[string]any
	adapter := NewOpenAIAdapter(store)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(req.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")),
		}, nil
	})}

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{
		"prompt":      "hello",
		"temperature": 0.2,
		"top_p":       0.9,
	}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()
	if _, ok := seenBody["temperature"]; ok {
		t.Fatalf("did not expect temperature for Kimi upstream, got %#v", seenBody)
	}
	if _, ok := seenBody["top_p"]; ok {
		t.Fatalf("did not expect top_p for Kimi upstream, got %#v", seenBody)
	}
}

func TestOpenAIAdapterCallCompletionUploadsContextFiles(t *testing.T) {
	t.Setenv("TOOL_GATEWAY_CONFIG_JSON", `{
		"external_ai":{
			"base_url":"https://upstream.example/custom/v1",
			"api_key":"store-key",
			"model":"gpt-test"
		}
	}`)
	store := config.LoadStore()
	uploadNames := []string{}
	var seenBody map[string]any
	adapter := NewOpenAIAdapter(store)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/custom/v1/files":
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("parse multipart upload: %v", err)
			}
			_, header, err := req.FormFile("file")
			if err != nil {
				t.Fatalf("expected uploaded file: %v", err)
			}
			uploadNames = append(uploadNames, header.Filename)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"file-` + strings.TrimSuffix(header.Filename, ".txt") + `"}`)),
			}, nil
		case "/custom/v1/chat/completions":
			if err := json.NewDecoder(req.Body).Decode(&seenBody); err != nil {
				t.Fatalf("decode upstream request: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")),
			}, nil
		default:
			t.Fatalf("unexpected upstream path: %s", req.URL.Path)
			return nil, nil
		}
	})}

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{
		"prompt":       "Continue from attached context.",
		"ref_file_ids": []any{"file-existing"},
		"history_text": "# TOOL_GATEWAY_HISTORY.txt\nprevious context",
		"tools_text":   "# TOOL_GATEWAY_TOOLS.txt\nTool: search",
		"request_messages": []any{map[string]any{
			"role":    "user",
			"content": "Continue from attached context.",
		}},
	}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()

	if len(uploadNames) != 2 || uploadNames[0] != "TOOL_GATEWAY_HISTORY.txt" || uploadNames[1] != "TOOL_GATEWAY_TOOLS.txt" {
		t.Fatalf("expected history and tools uploads, got %#v", uploadNames)
	}
	fileIDs, _ := seenBody["file_ids"].([]any)
	if len(fileIDs) != 3 || fileIDs[0] != "file-TOOL_GATEWAY_HISTORY" || fileIDs[1] != "file-TOOL_GATEWAY_TOOLS" || fileIDs[2] != "file-existing" {
		t.Fatalf("expected uploaded context file IDs first, got %#v", seenBody["file_ids"])
	}
}

func TestOpenAIAdapterKimiUploadsImageURLAsFileID(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"id":"file-` + strings.TrimSuffix(header.Filename, ".jpg") + `"}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(req.Body).Decode(&seenChat); err != nil {
				t.Fatalf("decode chat request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("EXTERNAL_AI_BASE_URL", server.URL+"/v1")
	t.Setenv("EXTERNAL_AI_API_KEY", "local-key")
	t.Setenv("EXTERNAL_AI_MODEL", "kimi-k2.6")
	adapter := NewOpenAIAdapter(nil)

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{
		"prompt": "what is this image?",
		"request_messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "what is this image?"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64,aW1hZ2U="}},
			},
		}},
	}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()
	if len(uploadNames) != 1 || uploadNames[0] != "image.jpg" {
		t.Fatalf("expected image upload, got %#v", uploadNames)
	}
	fileIDs, _ := seenChat["file_ids"].([]any)
	if len(fileIDs) != 1 || fileIDs[0] != "file-image" {
		t.Fatalf("expected uploaded image file id in chat request, got %#v", seenChat["file_ids"])
	}
	messages := seenChat["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	for _, raw := range content {
		part, _ := raw.(map[string]any)
		if part["type"] == "image_url" {
			t.Fatalf("did not expect image_url forwarded to Kimi provider, got %#v", messages)
		}
	}
}

func TestOpenAIAdapterLocalKimi2APIFlowUploadsContextAndForwardsImage(t *testing.T) {
	uploadNames := []string{}
	var seenChat map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/files":
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("parse kimi2api upload: %v", err)
			}
			_, header, err := req.FormFile("file")
			if err != nil {
				t.Fatalf("expected kimi2api file upload: %v", err)
			}
			uploadNames = append(uploadNames, header.Filename)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"kimi-file-` + strings.TrimSuffix(header.Filename, ".txt") + `"}`))
		case "/v1/chat/completions":
			if err := json.NewDecoder(req.Body).Decode(&seenChat); err != nil {
				t.Fatalf("decode kimi2api chat request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected kimi2api path: %s", req.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("EXTERNAL_AI_BASE_URL", server.URL+"/v1")
	t.Setenv("EXTERNAL_AI_API_KEY", "local-key")
	t.Setenv("EXTERNAL_AI_MODEL", "kimi-k2.5")
	adapter := NewOpenAIAdapter(nil)

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{
		"prompt":       "Continue from attached context.",
		"history_text": "# TOOL_GATEWAY_HISTORY.txt\nprevious context",
		"tools_text":   "# TOOL_GATEWAY_TOOLS.txt\nTool: search",
		"request_messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "Continue from attached context."},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,aW1hZ2U="}},
			},
		}},
	}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()

	if len(uploadNames) != 3 || uploadNames[0] != "TOOL_GATEWAY_HISTORY.txt" || uploadNames[1] != "TOOL_GATEWAY_TOOLS.txt" || uploadNames[2] != "image.png" {
		t.Fatalf("expected context and image file uploads to local kimi2api, got %#v", uploadNames)
	}
	fileIDs, _ := seenChat["file_ids"].([]any)
	if len(fileIDs) != 3 || fileIDs[0] != "kimi-file-TOOL_GATEWAY_HISTORY" || fileIDs[1] != "kimi-file-TOOL_GATEWAY_TOOLS" || fileIDs[2] != "kimi-file-image.png" {
		t.Fatalf("expected uploaded file IDs in chat request, got %#v", seenChat["file_ids"])
	}
	messages := seenChat["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	promptCount := 0
	for _, raw := range content {
		part, _ := raw.(map[string]any)
		if part["type"] == "image_url" {
			t.Fatalf("did not expect image_url to reach local kimi2api, got %#v", messages)
		}
		if part["type"] == "text" && strings.Contains(part["text"].(string), "Continue from") {
			promptCount++
		}
	}
	if promptCount != 1 {
		t.Fatalf("expected one continuation prompt in upstream content, got %d content=%#v", promptCount, content)
	}
}

func TestOpenAIAdapterCallCompletionReturnsUpstreamErrorResponse(t *testing.T) {
	t.Setenv("EXTERNAL_AI_BASE_URL", "https://upstream.example")
	t.Setenv("EXTERNAL_AI_API_KEY", "env-key")
	adapter := NewOpenAIAdapter(nil)
	adapter.Client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer env-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})}

	resp, err := adapter.CallCompletion(context.Background(), map[string]any{"prompt": "hello"}, "")
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected upstream status, got %d", resp.StatusCode)
	}
}

func TestOpenAIAdapterCallCompletionRequiresConfig(t *testing.T) {
	t.Setenv("EXTERNAL_AI_BASE_URL", "")
	t.Setenv("EXTERNAL_AI_API_KEY", "")
	adapter := NewOpenAIAdapter(nil)
	_, err := adapter.CallCompletion(context.Background(), map[string]any{"prompt": "hello"}, "")
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("expected base_url error, got %v", err)
	}
}
