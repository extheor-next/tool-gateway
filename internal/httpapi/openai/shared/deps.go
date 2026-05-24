package shared

import (
	"context"
	"net/http"

	"tool-gateway/internal/auth"
	"tool-gateway/internal/chathistory"
	"tool-gateway/internal/config"
	dsclient "tool-gateway/internal/deepseek/client"
	"tool-gateway/internal/util"
)

const (
	// UploadMaxSize limits total multipart request body size (100 MiB).
	UploadMaxSize = 100 << 20
	// GeneralMaxSize limits total JSON request body size (100 MiB).
	GeneralMaxSize = 100 << 20
)

type AuthResolver interface {
	Determine(req *http.Request) (*auth.RequestAuth, error)
	DetermineCaller(req *http.Request) (*auth.RequestAuth, error)
}

type CompletionBackend interface {
	CreateSession(ctx context.Context, maxAttempts int) (string, error)
	GetPow(ctx context.Context, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
	CallCompletion(ctx context.Context, payload map[string]any, powResp string) (*http.Response, error)
	DeleteSession(ctx context.Context, sessionID string, maxAttempts int) (*dsclient.DeleteSessionResult, error)
	DeleteAllSessions(ctx context.Context) error
}

type ConfigReader interface {
	ModelAliases() map[string]string
	ToolcallMode() string
	ToolcallEarlyEmitConfidence() string
	ResponsesStoreTTLSeconds() int
	EmbeddingsProvider() string
	AutoDeleteMode() string
	AutoDeleteSessions() bool
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
	ThinkingInjectionEnabled() bool
	ThinkingInjectionPrompt() string
}

type Deps struct {
	Store       ConfigReader
	Auth        AuthResolver
	Backend     CompletionBackend
	ChatHistory *chathistory.Store
}

var WriteJSON = util.WriteJSON

var _ AuthResolver = (*auth.Resolver)(nil)
var _ CompletionBackend = (*dsclient.Client)(nil)
var _ ConfigReader = (*config.Store)(nil)
