package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"ds2api/internal/auth"
	"ds2api/internal/config"
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

	resp, err := adapter.CallCompletion(context.Background(), &auth.RequestAuth{}, map[string]any{
		"prompt":      "hello",
		"temperature": 0.2,
	}, "", 1)
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

	resp, err := adapter.CallCompletion(context.Background(), nil, map[string]any{"prompt": "hello"}, "", 1)
	if err != nil {
		t.Fatalf("CallCompletion() error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected upstream status, got %d", resp.StatusCode)
	}
}

func TestOpenAIAdapterCallCompletionRequiresConfig(t *testing.T) {
	adapter := NewOpenAIAdapter(nil)
	_, err := adapter.CallCompletion(context.Background(), nil, map[string]any{"prompt": "hello"}, "", 1)
	if err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("expected base_url error, got %v", err)
	}
}
