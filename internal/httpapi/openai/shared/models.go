package shared

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/config"
)

type ExternalAIConfigReader interface {
	ExternalAI() config.ExternalAIConfig
}

type ModelsHandler struct {
	Store      ConfigReader
	ExtStore   ExternalAIConfigReader
	HTTPClient *http.Client
}

func (h *ModelsHandler) ListModels(w http.ResponseWriter, _ *http.Request) {
	cfg := h.ExtStore.ExternalAI()
	if strings.TrimSpace(cfg.BaseURL) != "" {
		models, ok := h.fetchFromProvider(cfg.BaseURL, cfg.APIKey)
		if ok {
			WriteJSON(w, http.StatusOK, models)
			return
		}
	}
	WriteJSON(w, http.StatusOK, emptyModelsResponse())
}

func (h *ModelsHandler) fetchFromProvider(baseURL, apiKey string) (map[string]any, bool) {
	client := h.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	url := strings.TrimSuffix(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false
	}
	return result, true
}

func emptyModelsResponse() map[string]any {
	return map[string]any{"object": "list", "data": []any{}}
}

func (h *ModelsHandler) GetModel(w http.ResponseWriter, r *http.Request) {
	modelID := strings.TrimSpace(chi.URLParam(r, "model_id"))
	cfg := h.ExtStore.ExternalAI()
	if strings.TrimSpace(cfg.BaseURL) != "" {
		models, ok := h.fetchFromProvider(cfg.BaseURL, cfg.APIKey)
		if ok {
			if data, ok := models["data"].([]any); ok {
				for _, item := range data {
					if m, ok := item.(map[string]any); ok {
						if id, _ := m["id"].(string); id == modelID {
							WriteJSON(w, http.StatusOK, m)
							return
						}
					}
				}
			}
		}
	}
	WriteOpenAIError(w, http.StatusNotFound, "Model not found.")
}
