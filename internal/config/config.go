package config

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

type Config struct {
	Keys                []string                  `json:"keys,omitempty"`
	APIKeys             []APIKey                  `json:"api_keys,omitempty"`
	Proxies             []Proxy                   `json:"proxies,omitempty"`
	ModelAliases        map[string]string         `json:"model_aliases,omitempty"`
	Admin               AdminConfig               `json:"admin,omitempty"`
	Runtime             RuntimeConfig             `json:"runtime,omitempty"`
	Responses           ResponsesConfig           `json:"responses,omitempty"`
	Embeddings          EmbeddingsConfig          `json:"embeddings,omitempty"`
	ExternalAI          ExternalAIConfig          `json:"external_ai,omitempty"`
	ExternalAIProviders ExternalAIProvidersConfig `json:"external_ai_providers,omitempty"`
	AutoDelete          AutoDeleteConfig          `json:"auto_delete"`
	CurrentInputFile    CurrentInputFileConfig    `json:"current_input_file,omitempty"`
	ThinkingInjection   ThinkingInjectionConfig   `json:"thinking_injection,omitempty"`
	Vercel              VercelConfig              `json:"vercel,omitempty"`
	VercelSyncHash      string                    `json:"_vercel_sync_hash,omitempty"`
	VercelSyncTime      int64                     `json:"_vercel_sync_time,omitempty"`
	AdditionalFields    map[string]any            `json:"-"`
}

type APIKey struct {
	Key    string `json:"key"`
	Name   string `json:"name,omitempty"`
	Remark string `json:"remark,omitempty"`
}

type Proxy struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Type     string `json:"type,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func NormalizeProxy(p Proxy) Proxy {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.Password = strings.TrimSpace(p.Password)
	if p.ID == "" {
		p.ID = StableProxyID(p)
	}
	if p.Name == "" && p.Host != "" && p.Port > 0 {
		p.Name = fmt.Sprintf("%s:%d", p.Host, p.Port)
	}
	return p
}

func StableProxyID(p Proxy) string {
	sum := sha1.Sum([]byte(strings.ToLower(strings.TrimSpace(p.Type)) + "|" + strings.ToLower(strings.TrimSpace(p.Host)) + "|" + fmt.Sprintf("%d", p.Port) + "|" + strings.TrimSpace(p.Username)))
	return "proxy_" + hex.EncodeToString(sum[:6])
}

func (c *Config) NormalizeCredentials() {
	if c == nil {
		return
	}
	normalizedAPIKeys := normalizeAPIKeys(c.APIKeys)
	if len(normalizedAPIKeys) > 0 {
		c.APIKeys = normalizedAPIKeys
		c.Keys = apiKeysToStrings(c.APIKeys)
	} else {
		c.Keys = normalizeKeys(c.Keys)
		c.APIKeys = apiKeysFromStrings(c.Keys, nil)
	}

	c.Vercel = NormalizeVercelConfig(c.Vercel)
	c.normalizeModelAliases()
}

func (c *Config) normalizeModelAliases() {
	if c == nil {
		return
	}

	aliases := map[string]string{}
	for k, v := range c.ModelAliases {
		key := strings.TrimSpace(lower(k))
		val := strings.TrimSpace(lower(v))
		if key == "" || val == "" {
			continue
		}
		aliases[key] = val
	}
	if len(aliases) == 0 {
		c.ModelAliases = nil
	} else {
		c.ModelAliases = aliases
	}
}

type AdminConfig struct {
	PasswordHash      string `json:"password_hash,omitempty"`
	JWTExpireHours    int    `json:"jwt_expire_hours,omitempty"`
	JWTValidAfterUnix int64  `json:"jwt_valid_after_unix,omitempty"`
}

type RuntimeConfig struct {
	GlobalMaxInflight         int `json:"global_max_inflight,omitempty"`
	TokenRefreshIntervalHours int `json:"token_refresh_interval_hours,omitempty"`
}

type ResponsesConfig struct {
	StoreTTLSeconds int `json:"store_ttl_seconds,omitempty"`
}

type EmbeddingsConfig struct {
	Provider string `json:"provider,omitempty"`
}

type ExternalAIConfig struct {
	BaseURL     string            `json:"base_url,omitempty"`
	APIKey      string            `json:"api_key,omitempty"`
	Model       string            `json:"model,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	MaxInflight int               `json:"max_inflight,omitempty"`
	MaxQueue    int               `json:"max_queue,omitempty"`
}

type ExternalAIProvidersConfig struct {
	Active    string                     `json:"active,omitempty"`
	Providers []ExternalAIProviderConfig `json:"providers,omitempty"`
}

type ExternalAIProviderConfig struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name,omitempty"`
	BaseURL     string            `json:"base_url,omitempty"`
	APIKey      string            `json:"api_key,omitempty"`
	Model       string            `json:"model,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	MaxInflight int               `json:"max_inflight,omitempty"`
	MaxQueue    int               `json:"max_queue,omitempty"`
}

func NormalizeExternalAIProvider(p ExternalAIProviderConfig) ExternalAIProviderConfig {
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	p.APIKey = strings.TrimSpace(p.APIKey)
	p.Model = strings.TrimSpace(p.Model)
	p.Mode = normalizeExternalAIModeValue(p.Mode)
	if p.MaxInflight < 0 {
		p.MaxInflight = 0
	}
	if p.MaxQueue < 0 {
		p.MaxQueue = 0
	}
	if len(p.Headers) > 0 {
		headers := map[string]string{}
		for k, v := range p.Headers {
			key := strings.TrimSpace(k)
			val := strings.TrimSpace(v)
			if key != "" && val != "" {
				headers[key] = val
			}
		}
		p.Headers = headers
	}
	if p.ID == "" {
		p.ID = StableExternalAIProviderID(p)
	}
	if p.Name == "" && p.BaseURL != "" {
		p.Name = p.BaseURL
	}
	return p
}

func NormalizeExternalAIConfig(cfg ExternalAIConfig) ExternalAIConfig {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	cfg.Mode = normalizeExternalAIModeValue(cfg.Mode)
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

func NormalizeExternalAIProvidersConfig(cfg ExternalAIProvidersConfig) ExternalAIProvidersConfig {
	cfg.Active = strings.TrimSpace(cfg.Active)
	providers := make([]ExternalAIProviderConfig, 0, len(cfg.Providers))
	seen := map[string]struct{}{}
	for _, provider := range cfg.Providers {
		provider = NormalizeExternalAIProvider(provider)
		if provider.ID == "" {
			continue
		}
		if _, ok := seen[provider.ID]; ok {
			continue
		}
		seen[provider.ID] = struct{}{}
		providers = append(providers, provider)
	}
	cfg.Providers = providers
	if len(cfg.Providers) == 0 {
		cfg.Active = ""
		return cfg
	}
	if cfg.Active != "" {
		if _, ok := seen[cfg.Active]; ok {
			return cfg
		}
	}
	cfg.Active = cfg.Providers[0].ID
	return cfg
}

func StableExternalAIProviderID(p ExternalAIProviderConfig) string {
	seed := strings.ToLower(strings.TrimSpace(p.Name)) + "|" + strings.ToLower(strings.TrimSpace(p.BaseURL)) + "|" + strings.TrimSpace(p.Model)
	if strings.Trim(seed, "|") == "" {
		return ""
	}
	sum := sha1.Sum([]byte(seed))
	return "provider_" + hex.EncodeToString(sum[:6])
}

func ExternalAIFromProvider(p ExternalAIProviderConfig) ExternalAIConfig {
	p = NormalizeExternalAIProvider(p)
	return ExternalAIConfig{
		BaseURL:     p.BaseURL,
		APIKey:      p.APIKey,
		Model:       p.Model,
		Mode:        p.Mode,
		Headers:     p.Headers,
		MaxInflight: p.MaxInflight,
		MaxQueue:    p.MaxQueue,
	}
}

func ExternalAIProviderFromLegacy(id, name string, cfg ExternalAIConfig) ExternalAIProviderConfig {
	cfg = NormalizeExternalAIConfig(cfg)
	return NormalizeExternalAIProvider(ExternalAIProviderConfig{
		ID:          id,
		Name:        name,
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Mode:        cfg.Mode,
		Headers:     cfg.Headers,
		MaxInflight: cfg.MaxInflight,
		MaxQueue:    cfg.MaxQueue,
	})
}

func normalizeExternalAIModeValue(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "openai", "claude", "gemini":
		return mode
	default:
		return "auto"
	}
}

type AutoDeleteConfig struct {
	Mode     string `json:"mode,omitempty"`
	Sessions bool   `json:"sessions,omitempty"`
}

type CurrentInputFileConfig struct {
	Enabled  *bool `json:"enabled,omitempty"`
	MinChars int   `json:"min_chars,omitempty"`
}

type ThinkingInjectionConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

type VercelConfig struct {
	Token     string `json:"token,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	TeamID    string `json:"team_id,omitempty"`
}

func NormalizeVercelConfig(v VercelConfig) VercelConfig {
	return VercelConfig{
		Token:     strings.TrimSpace(v.Token),
		ProjectID: strings.TrimSpace(v.ProjectID),
		TeamID:    strings.TrimSpace(v.TeamID),
	}
}

func (c *Config) ClearVercelCredentials() {
	if c == nil {
		return
	}
	c.Vercel = VercelConfig{}
}
