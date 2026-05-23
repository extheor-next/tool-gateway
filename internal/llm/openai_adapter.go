package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	dsclient "ds2api/internal/deepseek/client"
)

// OpenAIAdapter is the OpenAI-compatible upstream backend used at runtime.
// It keeps ds2api's existing prompt/tool-call parsing pipeline by converting
// an OpenAI-compatible upstream stream into the small SSE shape that
// the rest of the project already understands.
type OpenAIAdapter struct {
	Store  *config.Store
	Client *http.Client
}

func NewOpenAIAdapter(store *config.Store) *OpenAIAdapter {
	return &OpenAIAdapter{Store: store, Client: &http.Client{Timeout: 0}}
}

func (a *OpenAIAdapter) ExternalAIAdapter() bool { return true }

func (a *OpenAIAdapter) Login(_ context.Context, _ config.Account) (string, error) {
	return "", errors.New("legacy account login has been removed; configure external_ai or EXTERNAL_AI_* instead")
}

func (a *OpenAIAdapter) CreateSession(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return fmt.Sprintf("chatcmpl-ds2api-%d", time.Now().UnixNano()), nil
}

func (a *OpenAIAdapter) GetPow(_ context.Context, _ *auth.RequestAuth, _ int) (string, error) {
	return "", nil
}

func (a *OpenAIAdapter) UploadFile(_ context.Context, user *auth.RequestAuth, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if len(req.Data) == 0 {
		return nil, errors.New("file is required")
	}
	// External OpenAI-compatible chat backends generally do not share DeepSeek's
	// remote file API. Return a stable local file object so generic clients can
	// still use /files for bookkeeping; current-input-file integration is skipped.
	id := fmt.Sprintf("file-local-%d", time.Now().UnixNano())
	accountID := ""
	if user != nil {
		accountID = user.AccountID
		if accountID == "" {
			accountID = user.CallerID
		}
	}
	return &dsclient.UploadFileResult{ID: id, Filename: req.Filename, Bytes: int64(len(req.Data)), Status: "uploaded", Purpose: req.Purpose, AccountID: accountID}, nil
}

func (a *OpenAIAdapter) CallCompletion(ctx context.Context, user *auth.RequestAuth, payload map[string]any, _ string, _ int) (*http.Response, error) {
	cfg := a.externalConfig(user)
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("external_ai.base_url or EXTERNAL_AI_BASE_URL is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("external_ai.api_key, EXTERNAL_AI_API_KEY, or caller bearer token is required")
	}
	prompt := strings.TrimSpace(asString(payload["prompt"]))
	if prompt == "" {
		return nil, errors.New("completion prompt is empty")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}
	body := map[string]any{
		"model":    model,
		"stream":   true,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	}
	copySamplingParams(body, payload)
	upstreamBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	reqURL, err := chatCompletionsURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	upstream, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		return upstream, nil
	}
	return convertOpenAIStreamResponse(upstream), nil
}

func (a *OpenAIAdapter) DeleteSessionForToken(_ context.Context, _ string, _ string) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (a *OpenAIAdapter) DeleteAllSessionsForToken(_ context.Context, _ string) error { return nil }

func (a *OpenAIAdapter) GetSessionCountForToken(_ context.Context, _ string) (*dsclient.SessionStats, error) {
	return &dsclient.SessionStats{Success: true}, nil
}

type externalAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Headers map[string]string
}

func (a *OpenAIAdapter) externalConfig(user *auth.RequestAuth) externalAIConfig {
	cfg := externalAIConfig{Headers: map[string]string{}}
	if a != nil && a.Store != nil {
		storeCfg := a.Store.ExternalAI()
		cfg.BaseURL = storeCfg.BaseURL
		cfg.APIKey = storeCfg.APIKey
		cfg.Model = storeCfg.Model
		cfg.Headers = storeCfg.Headers
		if cfg.Headers == nil {
			cfg.Headers = map[string]string{}
		}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimSpace(os.Getenv("EXTERNAL_AI_BASE_URL"))
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("EXTERNAL_AI_API_KEY"))
	}
	if cfg.APIKey == "" && user != nil && !user.UseConfigToken {
		cfg.APIKey = strings.TrimSpace(user.DeepSeekToken)
	}
	if cfg.Model == "" {
		cfg.Model = strings.TrimSpace(os.Getenv("EXTERNAL_AI_MODEL"))
	}
	return cfg
}

func chatCompletionsURL(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("empty base url")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		return u.String(), nil
	}
	if strings.HasSuffix(path, "/v1") {
		u.Path = path + "/chat/completions"
	} else {
		u.Path = path + "/v1/chat/completions"
	}
	return u.String(), nil
}

func copySamplingParams(dst, src map[string]any) {
	for _, k := range []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "presence_penalty", "frequency_penalty", "stop"} {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
}

func convertOpenAIStreamResponse(upstream *http.Response) *http.Response {
	pr, pw := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       pr,
	}
	resp.Header.Set("Content-Type", "text/event-stream")
	go func() {
		defer upstream.Body.Close()
		defer pw.Close()
		writeOpenAIDeltaAsDeepSeekSSE(pw, upstream.Body)
	}()
	return resp
}

func writeOpenAIDeltaAsDeepSeekSSE(w io.Writer, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(line), "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			break
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, part := range extractOpenAIContentDeltas(chunk) {
			if part.Text == "" {
				continue
			}
			path := "response/content"
			if part.Thinking {
				path = "response/thinking_content"
			}
			writeDeepSeekSSE(w, path, part.Text)
		}
	}
	writeDeepSeekSSE(w, "response/status", "FINISHED")
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

type openAIDeltaPart struct {
	Text     string
	Thinking bool
}

func extractOpenAIContentDeltas(chunk map[string]any) []openAIDeltaPart {
	choices, _ := chunk["choices"].([]any)
	out := make([]openAIDeltaPart, 0, len(choices))
	for _, rawChoice := range choices {
		choice, _ := rawChoice.(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if text := asString(delta["reasoning_content"]); text != "" {
			out = append(out, openAIDeltaPart{Text: text, Thinking: true})
		}
		if text := asString(delta["reasoning"]); text != "" {
			out = append(out, openAIDeltaPart{Text: text, Thinking: true})
		}
		if text := asString(delta["content"]); text != "" {
			out = append(out, openAIDeltaPart{Text: text})
		}
	}
	return out
}

func writeDeepSeekSSE(w io.Writer, path, text string) {
	b, _ := json.Marshal(map[string]any{"p": path, "v": text})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
