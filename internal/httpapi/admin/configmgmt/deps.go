package configmgmt

import (
	"tool-gateway/internal/chathistory"
	"tool-gateway/internal/config"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
)

type Handler struct {
	Store       adminshared.ConfigStore

	Backend     adminshared.CompletionBackend
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

var writeJSON = adminshared.WriteJSON

func maskSecretPreview(secret string) string {
	return adminshared.MaskSecretPreview(secret)
}
func toStringSlice(v any) ([]string, bool) { return adminshared.ToStringSlice(v) }
func toAPIKeys(v any) ([]config.APIKey, bool) { return adminshared.ToAPIKeys(v) }
func mergeAPIKeysPreferStructured(existing, incoming []config.APIKey) ([]config.APIKey, int) {
	return adminshared.MergeAPIKeysPreferStructured(existing, incoming)
}
func fieldString(m map[string]any, key string) string {
	return adminshared.FieldString(m, key)
}
func fieldStringOptional(m map[string]any, key string) (string, bool) {
	return adminshared.FieldStringOptional(m, key)
}
func newRequestError(detail string) error { return adminshared.NewRequestError(detail) }
func requestErrorDetail(err error) (string, bool) {
	return adminshared.RequestErrorDetail(err)
}
func normalizeSettingsConfig(c *config.Config) { adminshared.NormalizeSettingsConfig(c) }
func validateSettingsConfig(c config.Config) error {
	return adminshared.ValidateSettingsConfig(c)
}
