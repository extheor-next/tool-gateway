package configmgmt

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) previewProviderModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "base_url is required"})
		return
	}

	models, err := h.fetchModels(baseURL, req.APIKey)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (h *Handler) fetchModels(baseURL, apiKey string) (map[string]any, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	url := strings.TrimSuffix(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, &providerError{status: resp.StatusCode, body: string(body)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

type providerError struct {
	status int
	body   string
}

func (e *providerError) Error() string {
	msg := "provider returned status " + http.StatusText(e.status)
	if strings.TrimSpace(e.body) != "" {
		msg += ": " + e.body
	}
	return msg
}
