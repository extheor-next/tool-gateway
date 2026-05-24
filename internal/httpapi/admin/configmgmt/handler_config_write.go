package configmgmt

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/config"
)

func (h *Handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	old := h.Store.Snapshot()
	err := h.Store.Update(func(c *config.Config) error {
		if apiKeys, ok := toAPIKeys(req["api_keys"]); ok {
			c.APIKeys = apiKeys
		} else if keys, ok := toStringSlice(req["keys"]); ok {
			c.Keys = keys
		}

		if externalAIRaw, ok := req["external_ai"].(map[string]any); ok {
			c.ExternalAI = normalizeExternalAIForStorage(toExternalAI(externalAIRaw, old.ExternalAI))
		}
		if providersRaw, ok := req["external_ai_providers"].(map[string]any); ok {
			providers := toExternalAIProviders(providersRaw, old.ExternalAIProviders)
			c.ExternalAIProviders = providers
			if active, ok := activeExternalAIProvider(providers); ok {
				c.ExternalAI = normalizeExternalAIForStorage(config.ExternalAIFromProvider(active))
			}
		}
		if m, ok := req["model_aliases"].(map[string]any); ok {
			aliases := make(map[string]string, len(m))
			for k, v := range m {
				aliases[k] = fmt.Sprintf("%v", v)
			}
			c.ModelAliases = aliases
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "配置已更新"})
}

func toExternalAI(m map[string]any, prev config.ExternalAIConfig) config.ExternalAIConfig {
	cfg := config.ExternalAIConfig{
		BaseURL:     fieldString(m, "base_url"),
		APIKey:      fieldString(m, "api_key"),
		Model:       fieldString(m, "model"),
		Mode:        fieldString(m, "mode"),
		Headers:     prev.Headers,
		MaxInflight: fieldInt(m, "max_inflight"),
		MaxQueue:    fieldInt(m, "max_queue"),
	}
	if cfg.APIKey == "" && fieldString(m, "api_key_preview") != "" {
		cfg.APIKey = prev.APIKey
	}
	if headersRaw, ok := m["headers"].(map[string]any); ok {
		headers := map[string]string{}
		for k, v := range headersRaw {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(fmt.Sprintf("%v", v))
			if key != "" && val != "" {
				headers[key] = val
			}
		}
		cfg.Headers = headers
	}
	return cfg
}

func toExternalAIProviders(m map[string]any, prev config.ExternalAIProvidersConfig) config.ExternalAIProvidersConfig {
	prevByID := map[string]config.ExternalAIProviderConfig{}
	prev = config.NormalizeExternalAIProvidersConfig(prev)
	for _, provider := range prev.Providers {
		prevByID[provider.ID] = provider
	}
	cfg := config.ExternalAIProvidersConfig{Active: fieldString(m, "active")}
	providersRaw, _ := m["providers"].([]any)
	cfg.Providers = make([]config.ExternalAIProviderConfig, 0, len(providersRaw))
	for _, item := range providersRaw {
		pm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provider := config.ExternalAIProviderConfig{
			ID:          fieldString(pm, "id"),
			Name:        fieldString(pm, "name"),
			BaseURL:     fieldString(pm, "base_url"),
			APIKey:      fieldString(pm, "api_key"),
			Model:       fieldString(pm, "model"),
			Mode:        fieldString(pm, "mode"),
			MaxInflight: fieldInt(pm, "max_inflight"),
			MaxQueue:    fieldInt(pm, "max_queue"),
		}
		if prevProvider, ok := prevByID[strings.TrimSpace(provider.ID)]; ok {
			if strings.TrimSpace(provider.APIKey) == "" && fieldString(pm, "api_key_preview") != "" {
				provider.APIKey = prevProvider.APIKey
			}
			if provider.APIKey == "" && boolField(pm, "has_api_key") {
				provider.APIKey = prevProvider.APIKey
			}
			provider.Headers = prevProvider.Headers
		}
		if headersRaw, ok := pm["headers"].(map[string]any); ok {
			headers := map[string]string{}
			for k, v := range headersRaw {
				key := strings.TrimSpace(k)
				val := strings.TrimSpace(fmt.Sprintf("%v", v))
				if key != "" && val != "" {
					headers[key] = val
				}
			}
			provider.Headers = headers
		}
		cfg.Providers = append(cfg.Providers, provider)
	}
	return config.NormalizeExternalAIProvidersConfig(cfg)
}

func mergeExternalAIProviders(current, incoming config.ExternalAIProvidersConfig) config.ExternalAIProvidersConfig {
	current = config.NormalizeExternalAIProvidersConfig(current)
	incoming = config.NormalizeExternalAIProvidersConfig(incoming)
	byID := map[string]config.ExternalAIProviderConfig{}
	order := make([]string, 0, len(current.Providers)+len(incoming.Providers))
	for _, provider := range current.Providers {
		byID[provider.ID] = provider
		order = append(order, provider.ID)
	}
	for _, provider := range incoming.Providers {
		if _, ok := byID[provider.ID]; !ok {
			order = append(order, provider.ID)
		}
		byID[provider.ID] = provider
	}
	merged := config.ExternalAIProvidersConfig{Active: current.Active}
	if strings.TrimSpace(incoming.Active) != "" {
		merged.Active = incoming.Active
	}
	for _, id := range order {
		if provider, ok := byID[id]; ok {
			merged.Providers = append(merged.Providers, provider)
		}
	}
	return config.NormalizeExternalAIProvidersConfig(merged)
}

func activeExternalAIProvider(cfg config.ExternalAIProvidersConfig) (config.ExternalAIProviderConfig, bool) {
	cfg = config.NormalizeExternalAIProvidersConfig(cfg)
	for _, provider := range cfg.Providers {
		if provider.ID == cfg.Active {
			return provider, true
		}
	}
	if len(cfg.Providers) > 0 {
		return cfg.Providers[0], true
	}
	return config.ExternalAIProviderConfig{}, false
}

func boolField(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func fieldInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i
		}
	}
	return 0
}

func normalizeExternalAIForStorage(cfg config.ExternalAIConfig) config.ExternalAIConfig {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.Mode = normalizeExternalAIMode(cfg.Mode)
	if cfg.MaxInflight < 0 {
		cfg.MaxInflight = 0
	}
	if cfg.MaxQueue < 0 {
		cfg.MaxQueue = 0
	}
	if len(cfg.Headers) > 0 {
		headers := map[string]string{}
		for k, v := range cfg.Headers {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(v)
			if key != "" && val != "" {
				headers[key] = val
			}
		}
		cfg.Headers = headers
	}
	return cfg
}

func (h *Handler) addKey(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	key, _ := req["key"].(string)
	key = strings.TrimSpace(key)
	name := fieldString(req, "name")
	remark := fieldString(req, "remark")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "Key 不能为空"})
		return
	}
	err := h.Store.Update(func(c *config.Config) error {
		for _, item := range c.APIKeys {
			if item.Key == key {
				return fmt.Errorf("key 已存在")
			}
		}
		c.APIKeys = append(c.APIKeys, config.APIKey{Key: key, Name: name, Remark: remark})
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) updateKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "key 不能为空"})
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "invalid json"})
		return
	}
	name, nameOK := fieldStringOptional(req, "name")
	remark, remarkOK := fieldStringOptional(req, "remark")

	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, item := range c.APIKeys {
			if item.Key == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("key 不存在")
		}
		if nameOK {
			c.APIKeys[idx].Name = name
		}
		if remarkOK {
			c.APIKeys[idx].Remark = remark
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	err := h.Store.Update(func(c *config.Config) error {
		idx := -1
		for i, item := range c.APIKeys {
			if item.Key == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fmt.Errorf("key 不存在")
		}
		c.APIKeys = append(c.APIKeys[:idx], c.APIKeys[idx+1:]...)
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "total_keys": len(h.Store.Snapshot().Keys)})
}

func (h *Handler) batchImport(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"detail": "无效的 JSON 格式"})
		return
	}
	importedKeys, importedExternalAI := 0, 0
	err := h.Store.Update(func(c *config.Config) error {
		if apiKeys, ok := toAPIKeys(req["api_keys"]); ok {
			var changed int
			c.APIKeys, changed = mergeAPIKeysPreferStructured(c.APIKeys, apiKeys)
			importedKeys += changed
		}
		if keys, ok := req["keys"].([]any); ok {
			legacy := make([]config.APIKey, 0, len(keys))
			for _, k := range keys {
				key := strings.TrimSpace(fmt.Sprintf("%v", k))
				if key == "" {
					continue
				}
				legacy = append(legacy, config.APIKey{Key: key})
			}
			var changed int
			c.APIKeys, changed = mergeAPIKeysPreferStructured(c.APIKeys, legacy)
			importedKeys += changed
		}
		if externalAIRaw, ok := req["external_ai"].(map[string]any); ok {
			c.ExternalAI = normalizeExternalAIForStorage(toExternalAI(externalAIRaw, c.ExternalAI))
			if strings.TrimSpace(c.ExternalAI.BaseURL) != "" || strings.TrimSpace(c.ExternalAI.APIKey) != "" || strings.TrimSpace(c.ExternalAI.Model) != "" || strings.TrimSpace(c.ExternalAI.Mode) != "" || len(c.ExternalAI.Headers) > 0 || c.ExternalAI.MaxInflight > 0 || c.ExternalAI.MaxQueue > 0 {
				importedExternalAI = 1
			}
		}
		if providersRaw, ok := req["external_ai_providers"].(map[string]any); ok {
			incomingProviders := toExternalAIProviders(providersRaw, c.ExternalAIProviders)
			c.ExternalAIProviders = mergeExternalAIProviders(c.ExternalAIProviders, incomingProviders)
		}
		return nil
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "imported_keys": importedKeys, "imported_accounts": 0, "imported_external_ai": importedExternalAI})
}
