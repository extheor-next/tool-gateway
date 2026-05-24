package config

import (
	"fmt"
	"strings"
)

func ValidateConfig(c Config) error {
	if err := ValidateProxyConfig(c.Proxies); err != nil {
		return err
	}
	if err := ValidateAdminConfig(c.Admin); err != nil {
		return err
	}
	if err := ValidateRuntimeConfig(c.Runtime); err != nil {
		return err
	}
	if err := ValidateResponsesConfig(c.Responses); err != nil {
		return err
	}
	if err := ValidateEmbeddingsConfig(c.Embeddings); err != nil {
		return err
	}
	if err := ValidateExternalAIConfig(c.ExternalAI); err != nil {
		return err
	}
	if err := ValidateAutoDeleteConfig(c.AutoDelete); err != nil {
		return err
	}
	if err := ValidateExternalAIProvidersConfig(c.ExternalAIProviders); err != nil {
		return err
	}
	if err := ValidateCurrentInputFileConfig(c.CurrentInputFile); err != nil {
		return err
	}
	return nil
}

func ValidateProxyConfig(proxies []Proxy) error {
	seen := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		proxy = NormalizeProxy(proxy)
		if err := ValidateTrimmedString("proxies.id", proxy.ID, true); err != nil {
			return err
		}
		switch proxy.Type {
		case "socks5", "socks5h":
		default:
			return fmt.Errorf("proxies.type must be one of socks5, socks5h")
		}
		if err := ValidateTrimmedString("proxies.host", proxy.Host, true); err != nil {
			return err
		}
		if err := ValidateIntRange("proxies.port", proxy.Port, 1, 65535, true); err != nil {
			return err
		}
		if _, ok := seen[proxy.ID]; ok {
			return fmt.Errorf("duplicate proxy id: %s", proxy.ID)
		}
		seen[proxy.ID] = struct{}{}
	}
	return nil
}

func ValidateAdminConfig(admin AdminConfig) error {
	return ValidateIntRange("admin.jwt_expire_hours", admin.JWTExpireHours, 1, 720, false)
}

func ValidateRuntimeConfig(runtime RuntimeConfig) error {
	if err := ValidateIntRange("runtime.global_max_inflight", runtime.GlobalMaxInflight, 1, 200000, false); err != nil {
		return err
	}
	if err := ValidateIntRange("runtime.token_refresh_interval_hours", runtime.TokenRefreshIntervalHours, 1, 720, false); err != nil {
		return err
	}
	return nil
}

func ValidateResponsesConfig(responses ResponsesConfig) error {
	return ValidateIntRange("responses.store_ttl_seconds", responses.StoreTTLSeconds, 30, 86400, false)
}

func ValidateEmbeddingsConfig(embeddings EmbeddingsConfig) error {
	return ValidateTrimmedString("embeddings.provider", embeddings.Provider, false)
}

func ValidateAutoDeleteConfig(autoDelete AutoDeleteConfig) error {
	return ValidateAutoDeleteMode(autoDelete.Mode)
}

func ValidateExternalAIConfig(cfg ExternalAIConfig) error {
	if err := ValidateIntRange("external_ai.max_inflight", cfg.MaxInflight, 0, 200000, false); err != nil {
		return err
	}
	return ValidateIntRange("external_ai.max_queue", cfg.MaxQueue, 0, 200000, false)
}

func ValidateExternalAIProvidersConfig(cfg ExternalAIProvidersConfig) error {
	seen := map[string]struct{}{}
	for _, provider := range cfg.Providers {
		provider = NormalizeExternalAIProvider(provider)
		if err := ValidateTrimmedString("external_ai_providers.providers.id", provider.ID, true); err != nil {
			return err
		}
		if _, ok := seen[provider.ID]; ok {
			return fmt.Errorf("duplicate external_ai provider id: %s", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		switch normalizeExternalAIModeValue(provider.Mode) {
		case "auto", "openai", "claude", "gemini":
		default:
			return fmt.Errorf("external_ai_providers.providers.mode must be one of auto, openai, claude, gemini")
		}
		if err := ValidateIntRange("external_ai_providers.providers.max_inflight", provider.MaxInflight, 0, 200000, false); err != nil {
			return err
		}
		if err := ValidateIntRange("external_ai_providers.providers.max_queue", provider.MaxQueue, 0, 200000, false); err != nil {
			return err
		}
	}
	active := strings.TrimSpace(cfg.Active)
	if active != "" {
		if _, ok := seen[active]; !ok {
			return fmt.Errorf("external_ai_providers.active references unknown provider: %s", active)
		}
	}
	return nil
}

func ValidateCurrentInputFileConfig(currentInputFile CurrentInputFileConfig) error {
	if currentInputFile.MinChars != 0 {
		return ValidateIntRange("current_input_file.min_chars", currentInputFile.MinChars, 1, 100000000, true)
	}
	return nil
}

func ValidateIntRange(name string, value, min, max int, required bool) error {
	if value == 0 && !required {
		return nil
	}
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return nil
}

func ValidateTrimmedString(name, value string, required bool) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if !required && value == "" {
			return nil
		}
		return fmt.Errorf("%s cannot be empty", name)
	}
	return nil
}

func ValidateAutoDeleteMode(mode string) error {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "none", "single", "all":
		return nil
	default:
		return fmt.Errorf("auto_delete.mode must be one of none, single, all")
	}
}
