package shared

import (
	"context"
	"net/http"

	"tool-gateway/internal/config"
	dsclient "tool-gateway/internal/deepseek/client"
)

type ConfigStore interface {
	Snapshot() config.Config
	Keys() []string
	Update(mutator func(*config.Config) error) error
	ExportJSONAndBase64() (string, string, error)
	IsEnvBacked() bool
	IsEnvWritebackEnabled() bool
	HasEnvConfigSource() bool
	ConfigPath() string
	SetVercelSync(hash string, ts int64) error
	AdminPasswordHash() string
	AdminJWTExpireHours() int
	AdminJWTValidAfterUnix() int64
	RuntimeGlobalMaxInflight(defaultSize int) int
	AutoDeleteMode() string
	AutoDeleteSessions() bool
	ModelAliases() map[string]string
	ExternalAI() config.ExternalAIConfig
	ExternalAIProviders() config.ExternalAIProvidersConfig
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
	CurrentInputFileMaxKeepMessages() int
}

type OpenAIChatCaller interface {
	ChatCompletions(w http.ResponseWriter, r *http.Request)
}

type CompletionBackend interface {
	CreateSession(ctx context.Context, maxAttempts int) (string, error)
	GetPow(ctx context.Context, maxAttempts int) (string, error)
	CallCompletion(ctx context.Context, payload map[string]any, powResp string) (*http.Response, error)
}

var _ ConfigStore = (*config.Store)(nil)
var _ CompletionBackend = (*dsclient.Client)(nil)
