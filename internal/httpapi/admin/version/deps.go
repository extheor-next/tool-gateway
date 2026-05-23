package version

import (
	"tool-gateway/internal/chathistory"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
)

type Handler struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	Backend     adminshared.CompletionBackend
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

var writeJSON = adminshared.WriteJSON
