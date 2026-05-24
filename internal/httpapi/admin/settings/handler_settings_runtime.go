package settings

import "tool-gateway/internal/config"

func validateMergedRuntimeSettings(current config.RuntimeConfig, incoming *config.RuntimeConfig) error {
	merged := current
	if incoming != nil {
		if incoming.GlobalMaxInflight > 0 {
			merged.GlobalMaxInflight = incoming.GlobalMaxInflight
		}
	}
	return validateRuntimeSettings(merged)
}

func (h *Handler) applyRuntimeSettings() {
	if h == nil || h.Store == nil {
		return
	}
}
