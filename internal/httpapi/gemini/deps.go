package gemini

import (
	"context"
	"net/http"

	"tool-gateway/internal/auth"
	"tool-gateway/internal/config"
	dsclient "tool-gateway/internal/deepseek/client"
)

type AuthResolver interface {
	Determine(req *http.Request) (*auth.RequestAuth, error)
}

type CompletionBackend interface {
	CreateSession(ctx context.Context, maxAttempts int) (string, error)
	GetPow(ctx context.Context, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, req dsclient.UploadFileRequest, maxAttempts int) (*dsclient.UploadFileResult, error)
	CallCompletion(ctx context.Context, payload map[string]any, powResp string) (*http.Response, error)
}

type ConfigReader interface {
	ModelAliases() map[string]string
	CurrentInputFileEnabled() bool
	CurrentInputFileMinChars() int
}

type OpenAIChatRunner interface {
	ChatCompletions(w http.ResponseWriter, r *http.Request)
}

var _ AuthResolver = (*auth.Resolver)(nil)
var _ CompletionBackend = (*dsclient.Client)(nil)
var _ ConfigReader = (*config.Store)(nil)
