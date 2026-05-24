package chat

import (
	"context"
	"net/http"
	"sync"
	"time"

	"tool-gateway/internal/auth"
	"tool-gateway/internal/chathistory"
	"tool-gateway/internal/httpapi/openai/files"
	"tool-gateway/internal/httpapi/openai/history"
	"tool-gateway/internal/httpapi/openai/shared"
	"tool-gateway/internal/promptcompat"
	"tool-gateway/internal/textclean"
	"tool-gateway/internal/toolcall"
	"tool-gateway/internal/toolstream"
)

const openAIGeneralMaxSize = shared.GeneralMaxSize

var writeJSON = shared.WriteJSON

type Handler struct {
	Store       shared.ConfigReader
	Auth        shared.AuthResolver
	Backend     shared.CompletionBackend
	ChatHistory *chathistory.Store

	leaseMu      sync.Mutex
	streamLeases map[string]streamLease
}

type streamLease struct {
	Auth      *auth.RequestAuth
	Standard  promptcompat.StandardRequest
	SessionID string
	ExpiresAt time.Time
}

func stripReferenceMarkersEnabled() bool {
	return textclean.StripReferenceMarkersEnabled()
}

func (h *Handler) applyCurrentInputFile(ctx context.Context, stdReq promptcompat.StandardRequest) (promptcompat.StandardRequest, error) {
	if h == nil {
		return stdReq, nil
	}
	stdReq = shared.ApplyThinkingInjection(h.Store, stdReq)
	svc := history.Service{Store: h.Store, Backend: h.Backend}
	out, err := svc.ApplyCurrentInputFile(ctx, stdReq)
	if err != nil || out.CurrentInputFileApplied {
		return out, err
	}
	return out, nil
}

func (h *Handler) preprocessInlineFileInputs(ctx context.Context, req map[string]any) error {
	if h == nil {
		return nil
	}
	return (&files.Handler{Store: h.Store, Auth: h.Auth, Backend: h.Backend, ChatHistory: h.ChatHistory}).PreprocessInlineFileInputs(ctx, req)
}

func (h *Handler) toolcallFeatureMatchEnabled() bool {
	if h == nil {
		return shared.ToolcallFeatureMatchEnabled(nil)
	}
	return shared.ToolcallFeatureMatchEnabled(h.Store)
}

func (h *Handler) toolcallEarlyEmitHighConfidence() bool {
	if h == nil {
		return shared.ToolcallEarlyEmitHighConfidence(nil)
	}
	return shared.ToolcallEarlyEmitHighConfidence(h.Store)
}

func writeOpenAIError(w http.ResponseWriter, status int, message string) {
	shared.WriteOpenAIError(w, status, message)
}

func writeOpenAIErrorWithCode(w http.ResponseWriter, status int, message, code string) {
	shared.WriteOpenAIErrorWithCode(w, status, message, code)
}

func openAIErrorType(status int) string {
	return shared.OpenAIErrorType(status)
}

func writeOpenAIInlineFileError(w http.ResponseWriter, err error) {
	files.WriteInlineFileError(w, err)
}

func mapCurrentInputFileError(err error) (int, string) {
	return history.MapError(err)
}

func requestTraceID(r *http.Request) string {
	return shared.RequestTraceID(r)
}

func asString(v any) string {
	return shared.AsString(v)
}

func cleanVisibleOutput(text string, stripReferenceMarkers bool) string {
	return shared.CleanVisibleOutput(text, stripReferenceMarkers)
}

func emptyOutputRetryEnabled() bool {
	return shared.EmptyOutputRetryEnabled()
}

func emptyOutputRetryMaxAttempts() int {
	return shared.EmptyOutputRetryMaxAttempts()
}

func formatIncrementalStreamToolCallDeltas(deltas []toolstream.ToolCallDelta, ids map[int]string) []map[string]any {
	return shared.FormatIncrementalStreamToolCallDeltas(deltas, ids)
}

func filterIncrementalToolCallDeltasByAllowed(deltas []toolstream.ToolCallDelta, seenNames map[int]string) []toolstream.ToolCallDelta {
	return shared.FilterIncrementalToolCallDeltasByAllowed(deltas, seenNames)
}

func formatFinalStreamToolCallsWithStableIDs(calls []toolcall.ParsedToolCall, ids map[int]string, toolsRaw any) []map[string]any {
	return shared.FormatFinalStreamToolCallsWithStableIDs(calls, ids, toolsRaw)
}
