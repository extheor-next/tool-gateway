package configmgmt

import (
	"net/http"
	"strings"

	"tool-gateway/internal/config"
)

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	safe := map[string]any{
		"keys":                  snap.Keys,
		"api_keys":              snap.APIKeys,
		"accounts":              []map[string]any{},
		"external_ai":           safeExternalAI(snap.ExternalAI),
		"external_ai_providers": safeExternalAIProviders(snap.ExternalAIProviders),
		"proxies":               []map[string]any{},
		"env_backed":            h.Store.IsEnvBacked(),
		"env_source_present":    h.Store.HasEnvConfigSource(),
		"env_writeback_enabled": h.Store.IsEnvWritebackEnabled(),
		"config_path":           h.Store.ConfigPath(),
		"model_aliases":         snap.ModelAliases,
		"vercel": map[string]any{
			"has_token":     strings.TrimSpace(snap.Vercel.Token) != "",
			"token_preview": maskSecretPreview(snap.Vercel.Token),
			"project_id":    snap.Vercel.ProjectID,
			"team_id":       snap.Vercel.TeamID,
		},
	}
	accounts := make([]map[string]any, 0, len(snap.Accounts))
	for _, acc := range snap.Accounts {
		token := strings.TrimSpace(acc.Token)
		accounts = append(accounts, map[string]any{
			"identifier":    acc.Identifier(),
			"name":          acc.Name,
			"remark":        acc.Remark,
			"email":         acc.Email,
			"mobile":        acc.Mobile,
			"proxy_id":      acc.ProxyID,
			"has_password":  strings.TrimSpace(acc.Password) != "",
			"has_token":     token != "",
			"token_preview": maskSecretPreview(token),
		})
	}
	safe["accounts"] = accounts
	proxies := make([]map[string]any, 0, len(snap.Proxies))
	for _, proxy := range snap.Proxies {
		proxy = config.NormalizeProxy(proxy)
		proxies = append(proxies, map[string]any{
			"id":           proxy.ID,
			"name":         proxy.Name,
			"type":         proxy.Type,
			"host":         proxy.Host,
			"port":         proxy.Port,
			"username":     proxy.Username,
			"has_password": strings.TrimSpace(proxy.Password) != "",
		})
	}
	safe["proxies"] = proxies
	writeJSON(w, http.StatusOK, safe)
}

func safeExternalAI(cfg config.ExternalAIConfig) map[string]any {
	apiKey := strings.TrimSpace(cfg.APIKey)
	return map[string]any{
		"base_url":        strings.TrimSpace(cfg.BaseURL),
		"api_key":         apiKey,
		"model":           strings.TrimSpace(cfg.Model),
		"mode":            normalizeExternalAIMode(cfg.Mode),
		"headers":         cfg.Headers,
		"max_inflight":    cfg.MaxInflight,
		"max_queue":       cfg.MaxQueue,
		"has_api_key":     apiKey != "",
		"api_key_preview": maskSecretPreview(apiKey),
	}
}

func safeExternalAIProviders(cfg config.ExternalAIProvidersConfig) map[string]any {
	cfg = config.NormalizeExternalAIProvidersConfig(cfg)
	providers := make([]map[string]any, 0, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		apiKey := strings.TrimSpace(provider.APIKey)
		providers = append(providers, map[string]any{
			"id":              strings.TrimSpace(provider.ID),
			"name":            strings.TrimSpace(provider.Name),
			"base_url":        strings.TrimSpace(provider.BaseURL),
			"model":           strings.TrimSpace(provider.Model),
			"mode":            normalizeExternalAIMode(provider.Mode),
			"headers":         provider.Headers,
			"max_inflight":    provider.MaxInflight,
			"max_queue":       provider.MaxQueue,
			"has_api_key":     apiKey != "",
			"api_key_preview": maskSecretPreview(apiKey),
		})
	}
	return map[string]any{
		"active":    cfg.Active,
		"providers": providers,
	}
}

func normalizeExternalAIMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "openai", "claude", "gemini":
		return mode
	default:
		return "auto"
	}
}

func (h *Handler) exportConfig(w http.ResponseWriter, _ *http.Request) {
	h.configExport(w, nil)
}

func (h *Handler) configExport(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	jsonStr, b64, err := h.Store.ExportJSONAndBase64()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"config":  snap,
		"json":    jsonStr,
		"base64":  b64,
	})
}
