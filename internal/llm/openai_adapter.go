package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tool-gateway/internal/config"
	dsclient "tool-gateway/internal/deepseek/client"
	"tool-gateway/internal/promptcompat"
)

// OpenAIAdapter is the OpenAI-compatible upstream backend used at runtime.
// It keeps tool-gateway's existing prompt/tool-call parsing pipeline by converting
// an OpenAI-compatible upstream stream into the small SSE shape that
// the rest of the project already understands.
type OpenAIAdapter struct {
	Store  *config.Store
	Client *http.Client

	limitersMu sync.Mutex
	limiters   map[string]*providerLimiter
}

func NewOpenAIAdapter(store *config.Store) *OpenAIAdapter {
	return &OpenAIAdapter{Store: store, Client: &http.Client{Timeout: 0}}
}

func (a *OpenAIAdapter) ExternalAIAdapter() bool { return true }

func (a *OpenAIAdapter) PreserveInlineFileInputs() bool { return true }

func (a *OpenAIAdapter) CreateSession(_ context.Context, _ int) (string, error) {
	return fmt.Sprintf("chatcmpl-tool-gateway-%d", time.Now().UnixNano()), nil
}

func (a *OpenAIAdapter) GetPow(_ context.Context, _ int) (string, error) {
	return "", nil
}

func (a *OpenAIAdapter) UploadFile(_ context.Context, req dsclient.UploadFileRequest, _ int) (*dsclient.UploadFileResult, error) {
	if len(req.Data) == 0 {
		return nil, errors.New("file is required")
	}
	id := fmt.Sprintf("file-local-%d", time.Now().UnixNano())
	return &dsclient.UploadFileResult{ID: id, Filename: req.Filename, Bytes: int64(len(req.Data)), Status: "uploaded", Purpose: req.Purpose}, nil
}

// UploadFileToProvider uploads a file to the external AI provider using OpenAI-compatible Files API.
func (a *OpenAIAdapter) UploadFileToProvider(ctx context.Context, filename string, content []byte) (string, error) {
	return a.UploadFileToProviderWithPurpose(ctx, filename, content, "assistants", "")
}

// UploadFileToProviderWithPurpose uploads a file to the external AI provider using OpenAI-compatible Files API.
func (a *OpenAIAdapter) UploadFileToProviderWithPurpose(ctx context.Context, filename string, content []byte, purpose string, contentType string) (string, error) {
	cfg := a.externalConfig()
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return "", errors.New("external_ai base_url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", errors.New("external_ai api_key is required")
	}
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	url := base + "/v1/files"
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "assistants"
	}
	_ = writer.WriteField("purpose", purpose)
	part, err := createFilePart(writer, filename, contentType)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("upload file: status %d: %s", resp.StatusCode, string(msg))
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("upload file: decode response: %w", err)
	}
	if result.ID == "" {
		return "", errors.New("upload file: empty file id")
	}
	return result.ID, nil
}

func createFilePart(writer *multipart.Writer, filename string, contentType string) (io.Writer, error) {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
	if strings.TrimSpace(contentType) != "" {
		header.Set("Content-Type", strings.TrimSpace(contentType))
	}
	return writer.CreatePart(header)
}

func escapeQuotes(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\\\\`)
	return strings.ReplaceAll(s, `"`, `\\"`)
}

func (a *OpenAIAdapter) CallCompletion(ctx context.Context, payload map[string]any, _ string) (*http.Response, error) {
	cfg := a.externalConfig()
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("external_ai_providers active provider base_url, external_ai.base_url, or EXTERNAL_AI_BASE_URL is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("external_ai_providers active provider api_key, external_ai.api_key, EXTERNAL_AI_API_KEY, or caller bearer token is required")
	}
	prompt := strings.TrimSpace(asString(payload["prompt"]))
	if prompt == "" {
		return nil, errors.New("completion prompt is empty")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" || mode == "auto" {
		mode = detectModeFromURL(cfg.BaseURL)
	}

	switch mode {
	case "claude":
		return a.callClaudeUpstream(ctx, cfg, model, prompt, payload)
	default:
		return a.callOpenAIUpstream(ctx, cfg, model, prompt, payload)
	}
}

func (a *OpenAIAdapter) callOpenAIUpstream(ctx context.Context, cfg externalAIConfig, model string, prompt string, payload map[string]any) (*http.Response, error) {
	messages := buildMessages(prompt, payload["request_messages"])
	fileIDs, err := a.prepareOpenAIContextFileIDs(ctx, payload)
	if err != nil {
		return nil, err
	}
	if isKimiConfig(cfg) {
		var imageFileIDs []string
		messages, imageFileIDs, err = a.uploadKimiImageURLParts(ctx, messages)
		if err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, imageFileIDs...)
	}
	body := map[string]any{
		"model":    model,
		"stream":   true,
		"messages": messages,
	}
	if len(fileIDs) > 0 {
		body["file_ids"] = stringsToAnySlice(fileIDs)
	}
	if !isKimiConfig(cfg) {
		copySamplingParams(body, payload)
	}
	reqURL, err := chatCompletionsURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return a.doUpstreamRequest(ctx, cfg, reqURL, body, "Bearer "+cfg.APIKey)
}

func (a *OpenAIAdapter) prepareOpenAIContextFileIDs(ctx context.Context, payload map[string]any) ([]string, error) {
	fileIDs := make([]string, 0, 4)
	if historyText := strings.TrimSpace(asString(payload["history_text"])); historyText != "" {
		fileID, err := a.UploadFileToProviderWithPurpose(ctx, promptcompat.CurrentInputContextFilename, []byte(historyText), "assistants", "text/plain; charset=utf-8")
		if err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}
	if toolsText := strings.TrimSpace(asString(payload["tools_text"])); toolsText != "" {
		fileID, err := a.UploadFileToProviderWithPurpose(ctx, promptcompat.CurrentToolsContextFilename, []byte(toolsText), "assistants", "text/plain; charset=utf-8")
		if err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}
	fileIDs = append(fileIDs, payloadFileIDs(payload["ref_file_ids"])...)
	return fileIDs, nil
}

func (a *OpenAIAdapter) uploadKimiImageURLParts(ctx context.Context, messages []any) ([]any, []string, error) {
	out := make([]any, 0, len(messages))
	fileIDs := []string{}
	for _, rawMsg := range messages {
		msg, ok := rawMsg.(map[string]any)
		if !ok {
			out = append(out, rawMsg)
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			out = append(out, msg)
			continue
		}
		newParts := make([]any, 0, len(parts))
		seenText := map[string]struct{}{}
		for _, rawPart := range parts {
			part, ok := anyMap(rawPart)
			if !ok {
				newParts = append(newParts, rawPart)
				continue
			}
			partType := strings.ToLower(strings.TrimSpace(asString(part["type"])))
			if partType == "text" {
				text := strings.TrimSpace(asString(part["text"]))
				if _, exists := seenText[text]; exists {
					continue
				}
				seenText[text] = struct{}{}
				newParts = append(newParts, rawPart)
				continue
			}
			if partType != "image_url" {
				newParts = append(newParts, rawPart)
				continue
			}
			imageData, filename, contentType, ok := parseImageURLPart(part["image_url"])
			if !ok {
				newParts = append(newParts, rawPart)
				continue
			}
			fileID, err := a.UploadFileToProviderWithPurpose(ctx, filename, imageData, "vision", contentType)
			if err != nil {
				return nil, nil, err
			}
			fileIDs = append(fileIDs, fileID)
		}
		cloned := cloneStringAnyMap(msg)
		cloned["content"] = newParts
		out = append(out, cloned)
	}
	return out, fileIDs, nil
}

func parseImageURLPart(raw any) ([]byte, string, string, bool) {
	urlStr := ""
	switch v := raw.(type) {
	case string:
		urlStr = v
	case map[string]any:
		urlStr = asString(v["url"])
	}
	if !strings.HasPrefix(urlStr, "data:") {
		return nil, "", "", false
	}
	parts := strings.SplitN(urlStr[len("data:"):], ";base64,", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return nil, "", "", false
	}
	contentType := strings.TrimSpace(parts[0])
	if contentType == "" {
		contentType = "image/png"
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, "", "", false
	}
	ext := "png"
	if slash := strings.LastIndex(contentType, "/"); slash >= 0 && slash+1 < len(contentType) {
		ext = strings.TrimSpace(contentType[slash+1:])
	}
	if ext == "jpeg" {
		ext = "jpg"
	}
	return data, "image." + ext, contentType, true
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func payloadFileIDs(raw any) []string {
	out := []string{}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if fileID := strings.TrimSpace(asString(item)); fileID != "" {
				out = append(out, fileID)
			}
		}
	case []string:
		for _, item := range v {
			if fileID := strings.TrimSpace(item); fileID != "" {
				out = append(out, fileID)
			}
		}
	}
	return out
}

func stringsToAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func (a *OpenAIAdapter) callClaudeUpstream(ctx context.Context, cfg externalAIConfig, model string, prompt string, payload map[string]any) (*http.Response, error) {
	messages := buildMessages(prompt, payload["request_messages"])
	body := map[string]any{
		"model":      model,
		"stream":     true,
		"max_tokens": 4096,
		"messages":   messagesToClaudeFormat(messages),
	}
	if v, ok := payload["temperature"]; ok {
		body["temperature"] = v
	}
	if v, ok := payload["top_p"]; ok {
		body["top_p"] = v
	}
	if maxTok := asFloat(payload["max_tokens"]); maxTok > 0 {
		body["max_tokens"] = int(maxTok)
	}
	if maxTok := asFloat(payload["max_completion_tokens"]); maxTok > 0 {
		body["max_tokens"] = int(maxTok)
	}
	reqURL, err := claudeMessagesURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return a.doUpstreamRequest(ctx, cfg, reqURL, body, "x-api-key "+cfg.APIKey)
}

func (a *OpenAIAdapter) doUpstreamRequest(ctx context.Context, cfg externalAIConfig, reqURL string, body map[string]any, authHeader string) (*http.Response, error) {
	upstreamBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", authHeader)
	// Also set x-api-key for Claude-compatible APIs that use it
	if strings.HasPrefix(authHeader, "x-api-key ") {
		req.Header.Set("x-api-key", cfg.APIKey)
	}
	for k, v := range cfg.Headers {
		if strings.TrimSpace(k) != "" && strings.TrimSpace(v) != "" {
			req.Header.Set(k, v)
		}
	}
	client := a.Client
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	limiter, release, err := a.acquireProviderLimiter(ctx, cfg)
	if err != nil {
		return nil, err
	}
	upstream, err := client.Do(req)
	if err != nil {
		if release != nil {
			release()
		}
		return nil, err
	}
	if limiter != nil {
		upstream.Body = &providerLimitReadCloser{ReadCloser: upstream.Body, release: release}
	}
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 {
		return upstream, nil
	}
	return convertOpenAIStreamResponse(upstream), nil
}

func (a *OpenAIAdapter) DeleteSession(_ context.Context, _ string, _ int) (*dsclient.DeleteSessionResult, error) {
	return &dsclient.DeleteSessionResult{Success: true}, nil
}

func (a *OpenAIAdapter) DeleteAllSessions(_ context.Context) error { return nil }

type externalAIConfig struct {
	ProviderID  string
	BaseURL     string
	APIKey      string
	Model       string
	Mode        string
	Headers     map[string]string
	MaxInflight int
	MaxQueue    int
}

func (a *OpenAIAdapter) externalConfig() externalAIConfig {
	cfg := externalAIConfig{ProviderID: "external_ai", Headers: map[string]string{}}
	if a != nil && a.Store != nil {
		providers := a.Store.ExternalAIProviders()
		if len(providers.Providers) > 0 {
			for _, provider := range providers.Providers {
				if provider.ID == providers.Active {
					cfg = externalAIConfigFromProvider(provider)
					break
				}
			}
			if cfg.BaseURL == "" && cfg.APIKey == "" && cfg.Model == "" {
				cfg = externalAIConfigFromProvider(providers.Providers[0])
			}
		} else {
			storeCfg := a.Store.ExternalAI()
			cfg.BaseURL = storeCfg.BaseURL
			cfg.APIKey = storeCfg.APIKey
			cfg.Model = storeCfg.Model
			cfg.Mode = storeCfg.Mode
			cfg.Headers = storeCfg.Headers
			cfg.MaxInflight = storeCfg.MaxInflight
			cfg.MaxQueue = storeCfg.MaxQueue
		}
		if cfg.Headers == nil {
			cfg.Headers = map[string]string{}
		}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = strings.TrimSpace(os.Getenv("EXTERNAL_AI_BASE_URL"))
		if cfg.BaseURL != "" {
			cfg.ProviderID = "env_external_ai"
		}
	}
	if cfg.APIKey == "" {
		cfg.APIKey = strings.TrimSpace(os.Getenv("EXTERNAL_AI_API_KEY"))
		if cfg.APIKey != "" && cfg.ProviderID == "external_ai" && cfg.BaseURL != "" {
			cfg.ProviderID = "env_external_ai"
		}
	}
	if cfg.Model == "" {
		cfg.Model = strings.TrimSpace(os.Getenv("EXTERNAL_AI_MODEL"))
	}
	if cfg.MaxInflight == 0 {
		cfg.MaxInflight = envInt("EXTERNAL_AI_MAX_INFLIGHT")
	}
	if cfg.MaxQueue == 0 {
		cfg.MaxQueue = envInt("EXTERNAL_AI_MAX_QUEUE")
	}
	if cfg.ProviderID == "" {
		cfg.ProviderID = "external_ai"
	}
	return cfg
}

func externalAIConfigFromProvider(provider config.ExternalAIProviderConfig) externalAIConfig {
	provider = config.NormalizeExternalAIProvider(provider)
	return externalAIConfig{
		ProviderID:  provider.ID,
		BaseURL:     provider.BaseURL,
		APIKey:      provider.APIKey,
		Model:       provider.Model,
		Mode:        provider.Mode,
		Headers:     provider.Headers,
		MaxInflight: provider.MaxInflight,
		MaxQueue:    provider.MaxQueue,
	}
}

func envInt(key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if n < 0 {
		return 0
	}
	return n
}

type providerLimiter struct {
	mu       sync.Mutex
	cond     *sync.Cond
	inflight int
	waiting  int
}

func (a *OpenAIAdapter) acquireProviderLimiter(ctx context.Context, cfg externalAIConfig) (*providerLimiter, func(), error) {
	if cfg.MaxInflight <= 0 {
		return nil, nil, nil
	}
	key := strings.TrimSpace(cfg.ProviderID)
	if key == "" {
		key = "external_ai"
	}
	limiter := a.providerLimiter(key)
	if err := limiter.acquire(ctx, cfg.MaxInflight, cfg.MaxQueue); err != nil {
		return limiter, nil, err
	}
	return limiter, limiter.release, nil
}

func (a *OpenAIAdapter) providerLimiter(key string) *providerLimiter {
	a.limitersMu.Lock()
	defer a.limitersMu.Unlock()
	if a.limiters == nil {
		a.limiters = map[string]*providerLimiter{}
	}
	limiter := a.limiters[key]
	if limiter == nil {
		limiter = &providerLimiter{}
		limiter.cond = sync.NewCond(&limiter.mu)
		a.limiters[key] = limiter
	}
	return limiter
}

func (l *providerLimiter) acquire(ctx context.Context, maxInflight, maxQueue int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight < maxInflight {
		l.inflight++
		return nil
	}
	if maxQueue <= 0 || l.waiting >= maxQueue {
		return errors.New("active external AI provider concurrency limit reached")
	}
	l.waiting++
	defer func() { l.waiting-- }()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.cond.Broadcast()
			l.mu.Unlock()
		case <-done:
		}
	}()
	defer close(done)
	for l.inflight >= maxInflight {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.cond.Wait()
	}
	l.inflight++
	return nil
}

func (l *providerLimiter) release() {
	l.mu.Lock()
	if l.inflight > 0 {
		l.inflight--
	}
	l.cond.Signal()
	l.mu.Unlock()
}

type providerLimitReadCloser struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (r *providerLimitReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(func() {
		if r.release != nil {
			r.release()
		}
	})
	return err
}

func claudeMessagesURL(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("empty base url")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(path, "/messages") {
		return u.String(), nil
	}
	if strings.HasSuffix(path, "/v1") {
		u.Path = path + "/messages"
	} else {
		u.Path = path + "/v1/messages"
	}
	return u.String(), nil
}

func isKimiConfig(cfg externalAIConfig) bool {
	return strings.Contains(strings.ToLower(cfg.BaseURL), "kimi") || strings.Contains(strings.ToLower(cfg.Model), "kimi")
}

func detectModeFromURL(baseURL string) string {
	lower := strings.ToLower(baseURL)
	if strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude") {
		return "claude"
	}
	if strings.Contains(lower, "gemini") || strings.Contains(lower, "googleapis") {
		return "gemini"
	}
	return "openai"
}

// messagesToClaudeFormat converts OpenAI-format messages to Claude-format.
// OpenAI: {"role": "user", "content": [...]} or {"role": "user", "content": "text"}
// Claude:  {"role": "user", "content": [{"type": "text", "text": "..."}, {"type": "image", "source": {...}}]}
func messagesToClaudeFormat(messages []any) []any {
	result := make([]any, 0, len(messages))
	for _, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok {
			result = append(result, m)
			continue
		}
		result = append(result, openAIMessageToClaude(msg))
	}
	return result
}

func openAIMessageToClaude(msg map[string]any) map[string]any {
	role, _ := msg["role"].(string)
	content := msg["content"]

	// Handle array content (multimodal)
	if parts, ok := content.([]any); ok {
		claudeParts := make([]any, 0, len(parts))
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(asString(part["type"]))) {
			case "image_url":
				if img, ok := part["image_url"].(map[string]any); ok {
					url := asString(img["url"])
					claudeParts = append(claudeParts, imageURLToClaudeSource(url))
				} else if url, ok := part["image_url"].(string); ok {
					claudeParts = append(claudeParts, imageURLToClaudeSource(url))
				}
			case "text":
				claudeParts = append(claudeParts, map[string]string{
					"type": "text",
					"text": asString(part["text"]),
				})
			}
		}
		return map[string]any{"role": role, "content": claudeParts}
	}

	// Handle string content (text only)
	return map[string]any{"role": role, "content": stringOrStringMap(content)}
}

func stringOrStringMap(v any) any {
	if s, ok := v.(string); ok {
		return s
	}
	return v
}

// imageURLToClaudeSource converts an OpenAI image_url to a Claude image source block.
// OpenAI: {"url": "data:image/png;base64,ABC..."} or "data:image/png;base64,ABC..."
// Claude:  {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "ABC..."}}
func imageURLToClaudeSource(raw any) map[string]any {
	urlStr := ""
	switch v := raw.(type) {
	case string:
		urlStr = v
	case map[string]any:
		urlStr = asString(v["url"])
	}
	// Parse data URI: "data:image/png;base64,ABC..."
	mediaType, data := "image/png", ""
	if strings.HasPrefix(urlStr, "data:") {
		parts := strings.SplitN(urlStr[len("data:"):], ";", 2)
		if len(parts) > 0 && parts[0] != "" {
			mediaType = parts[0]
		}
		if len(parts) == 2 && strings.HasPrefix(parts[1], "base64,") {
			data = parts[1][len("base64,"):]
		}
	} else {
		data = urlStr
		mediaType = "image/png"
	}
	return map[string]any{
		"type": "image",
		"source": map[string]string{
			"type":       "base64",
			"media_type": mediaType,
			"data":       data,
		},
	}
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	default:
		return 0
	}
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

// buildMessages constructs the messages array for the upstream provider.
// If the original request contained image content (image_url blocks), those
// are preserved alongside the prompt text to support multimodal requests.
func buildMessages(prompt string, rawMessages any) []any {
	if rawMessages == nil {
		return []any{map[string]string{"role": "user", "content": prompt}}
	}
	msgs, ok := rawMessages.([]any)
	if !ok || len(msgs) == 0 {
		return []any{map[string]string{"role": "user", "content": prompt}}
	}

	// Find the last user message with image content
	var lastImageParts []any
	for i := len(msgs) - 1; i >= 0; i-- {
		msg, ok := msgs[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if strings.ToLower(role) != "user" {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		hasImage := false
		for _, p := range parts {
			if part, ok := p.(map[string]any); ok {
				if strings.ToLower(strings.TrimSpace(asString(part["type"]))) == "image_url" {
					hasImage = true
					break
				}
			}
		}
		if hasImage {
			lastImageParts = parts
			break
		}
	}

	if len(lastImageParts) == 0 {
		return []any{map[string]string{"role": "user", "content": prompt}}
	}

	// Build content array: prompt text + non-duplicate multimodal parts.
	content := make([]any, 0, len(lastImageParts)+1)
	if strings.TrimSpace(prompt) != "" && !contentHasText(lastImageParts, prompt) {
		content = append(content, map[string]string{"type": "text", "text": prompt})
	}
	content = append(content, lastImageParts...)
	return []any{map[string]any{"role": "user", "content": content}}
}

func contentHasText(parts []any, text string) bool {
	want := strings.TrimSpace(text)
	if want == "" {
		return false
	}
	for _, raw := range parts {
		part, ok := anyMap(raw)
		if !ok || strings.ToLower(strings.TrimSpace(asString(part["type"]))) != "text" {
			continue
		}
		if strings.TrimSpace(asString(part["text"])) == want {
			return true
		}
	}
	return false
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

func anyMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	if m, ok := v.(map[string]string); ok {
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, true
	}
	return nil, false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
