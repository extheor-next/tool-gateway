package devcapture

import (
	"tool-gateway/internal/chathistory"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
)

type Handler struct {
	Store       adminshared.ConfigStore

	Backend     adminshared.CompletionBackend
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

var writeJSON = adminshared.WriteJSON
