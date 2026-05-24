package proxies

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

func fieldString(m map[string]any, key string) string {
	return adminshared.FieldString(m, key)
}
func toProxy(m map[string]any) config.Proxy { return adminshared.ToProxy(m) }
func findProxyByID(c config.Config, proxyID string) (config.Proxy, bool) {
	return adminshared.FindProxyByID(c, proxyID)
}
func newRequestError(detail string) error { return adminshared.NewRequestError(detail) }
func requestErrorDetail(err error) (string, bool) {
	return adminshared.RequestErrorDetail(err)
}
