package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"tool-gateway/internal/config"
)

type ctxKey string

const authCtxKey ctxKey = "auth_context"

var (
	ErrUnauthorized  = errors.New("unauthorized: missing auth token")
	ErrInvalidAPIKey = errors.New("unauthorized: invalid api key")
)

// RequestAuth holds the auth context for a single request.
// With the account-pool removed, all requests are direct: the caller's
// API key is validated and the request passes through unchanged.
type RequestAuth struct {
	CallerID string
}

// Resolver validates gateway API keys and populates RequestAuth.
// There is no account pool, no token refresh, and no managed account
// acquisition — just API key validation.
type Resolver struct {
	Store *config.Store
}

// NewResolver creates a Resolver backed by the given config store.
func NewResolver(store *config.Store) *Resolver {
	return &Resolver{Store: store}
}

// Determine validates the caller's API key and returns a RequestAuth.
func (r *Resolver) Determine(req *http.Request) (*RequestAuth, error) {
	callerKey := extractCallerToken(req)
	if callerKey == "" {
		return nil, ErrUnauthorized
	}
	if r == nil || r.Store == nil || !r.Store.HasAPIKey(callerKey) {
		return nil, ErrInvalidAPIKey
	}
	return &RequestAuth{
		CallerID: callerTokenID(callerKey),
	}, nil
}

// DetermineCaller is an alias for Determine — with the account pool
// removed there is no distinction between the two.
func (r *Resolver) DetermineCaller(req *http.Request) (*RequestAuth, error) {
	return r.Determine(req)
}

// WithAuth stores the auth context in the request context.
func WithAuth(ctx context.Context, a *RequestAuth) context.Context {
	return context.WithValue(ctx, authCtxKey, a)
}

// FromContext retrieves the auth context previously stored with WithAuth.
func FromContext(ctx context.Context) (*RequestAuth, bool) {
	v := ctx.Value(authCtxKey)
	a, ok := v.(*RequestAuth)
	return a, ok
}

// extractCallerToken pulls the caller's credential from the request using
// standard conventions (Bearer header, x-api-key, x-goog-api-key, query params).
func extractCallerToken(req *http.Request) string {
	authHeader := strings.TrimSpace(req.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token := strings.TrimSpace(authHeader[7:])
		if token != "" {
			return token
		}
	}
	if key := strings.TrimSpace(req.Header.Get("x-api-key")); key != "" {
		return key
	}
	// Gemini/Google clients commonly send API key via x-goog-api-key.
	if key := strings.TrimSpace(req.Header.Get("x-goog-api-key")); key != "" {
		return key
	}
	// Gemini AI Studio compatibility: allow query key fallback only when no
	// header-based credential is present.
	if key := strings.TrimSpace(req.URL.Query().Get("key")); key != "" {
		return key
	}
	return strings.TrimSpace(req.URL.Query().Get("api_key"))
}

// callerTokenID returns a stable, short hash prefix of the token for logging/identification.
func callerTokenID(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return "caller:" + hex.EncodeToString(sum[:8])
}
