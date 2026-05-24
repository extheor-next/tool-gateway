package settings

import (
	"net/http"
	"strings"

	authn "tool-gateway/internal/auth"
	"tool-gateway/internal/config"
	"tool-gateway/internal/promptcompat"
)

func (h *Handler) getSettings(w http.ResponseWriter, _ *http.Request) {
	snap := h.Store.Snapshot()
	recommended := 100
	needsSync := config.IsVercel() && snap.VercelSyncHash != "" && snap.VercelSyncHash != h.computeSyncHash()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"admin": map[string]any{
			"has_password_hash":        strings.TrimSpace(snap.Admin.PasswordHash) != "",
			"jwt_expire_hours":         h.Store.AdminJWTExpireHours(),
			"jwt_valid_after_unix":     snap.Admin.JWTValidAfterUnix,
			"default_password_warning": authn.UsingDefaultAdminKey(h.Store),
		},
		"runtime": map[string]any{
			"account_max_inflight":         recommended,
			"account_max_queue":            recommended,
			"global_max_inflight":          h.Store.RuntimeGlobalMaxInflight(recommended),
			"token_refresh_interval_hours": 0,
		},
		"responses":   snap.Responses,
		"embeddings":  snap.Embeddings,
		"auto_delete": snap.AutoDelete,
		"current_input_file": map[string]any{
			"enabled":           h.Store.CurrentInputFileEnabled(),
			"min_chars":         h.Store.CurrentInputFileMinChars(),
			"max_keep_messages": h.Store.CurrentInputFileMaxKeepMessages(),
		},
		"thinking_injection": map[string]any{
			"enabled":        true,
			"prompt":         "",
			"default_prompt": promptcompat.DefaultThinkingInjectionPrompt,
		},
		"model_aliases":     snap.ModelAliases,
		"env_backed":        h.Store.IsEnvBacked(),
		"needs_vercel_sync": needsSync,
	})
}
