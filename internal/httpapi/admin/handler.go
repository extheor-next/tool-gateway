package admin

import (
	"github.com/go-chi/chi/v5"

	"tool-gateway/internal/chathistory"
	adminaccounts "tool-gateway/internal/httpapi/admin/accounts"
	adminauth "tool-gateway/internal/httpapi/admin/auth"
	adminconfig "tool-gateway/internal/httpapi/admin/configmgmt"
	admindevcapture "tool-gateway/internal/httpapi/admin/devcapture"
	adminhistory "tool-gateway/internal/httpapi/admin/history"
	adminproxies "tool-gateway/internal/httpapi/admin/proxies"
	adminrawsamples "tool-gateway/internal/httpapi/admin/rawsamples"
	adminsettings "tool-gateway/internal/httpapi/admin/settings"
	adminshared "tool-gateway/internal/httpapi/admin/shared"
	adminvercel "tool-gateway/internal/httpapi/admin/vercel"
	adminversion "tool-gateway/internal/httpapi/admin/version"
)

type Handler struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	Backend     adminshared.CompletionBackend
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

func RegisterRoutes(r chi.Router, h *Handler) {
	deps := adminsharedDeps(h)
	authHandler := &adminauth.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	accountsHandler := &adminaccounts.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	configHandler := &adminconfig.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	settingsHandler := &adminsettings.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	proxiesHandler := &adminproxies.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	rawSamplesHandler := &adminrawsamples.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	vercelHandler := &adminvercel.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	historyHandler := &adminhistory.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	devCaptureHandler := &admindevcapture.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	versionHandler := &adminversion.Handler{Store: deps.Store, Pool: deps.Pool, Backend: deps.Backend, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}

	adminauth.RegisterPublicRoutes(r, authHandler)
	r.Group(func(pr chi.Router) {
		pr.Use(authHandler.RequireAdmin)
		adminauth.RegisterProtectedRoutes(pr, authHandler)
		adminconfig.RegisterRoutes(pr, configHandler)
		adminsettings.RegisterRoutes(pr, settingsHandler)
		adminproxies.RegisterRoutes(pr, proxiesHandler)
		adminaccounts.RegisterRoutes(pr, accountsHandler)
		adminrawsamples.RegisterRoutes(pr, rawSamplesHandler)
		adminvercel.RegisterRoutes(pr, vercelHandler)
		admindevcapture.RegisterRoutes(pr, devCaptureHandler)
		adminhistory.RegisterRoutes(pr, historyHandler)
		adminversion.RegisterRoutes(pr, versionHandler)
	})
}

func adminsharedDeps(h *Handler) adminsharedDepsValue {
	if h == nil {
		return adminsharedDepsValue{}
	}
	return adminsharedDepsValue{Store: h.Store, Pool: h.Pool, Backend: h.Backend, OpenAI: h.OpenAI, ChatHistory: h.ChatHistory}
}

type adminsharedDepsValue struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	Backend     adminshared.CompletionBackend
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}
