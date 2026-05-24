package client

import (
	"context"
	dsprotocol "tool-gateway/internal/deepseek/protocol"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"tool-gateway/internal/config"
)

// CreateSession creates a new DeepSeek chat session using the gateway API key.
func (c *Client) CreateSession(ctx context.Context, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	clients := c.requestClients()
	attempts := 0
	for attempts < maxAttempts {
		headers := c.authHeaders()
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekCreateSessionURL, headers, map[string]any{"agent": "chat"})
		if err != nil {
			config.Logger.Warn("[create_session] request error", "error", err)
			attempts++
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			sessionID := extractCreateSessionID(resp)
			if sessionID != "" {
				return sessionID, nil
			}
		}
		config.Logger.Warn("[create_session] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg)
		attempts++
	}
	return "", errors.New("create session failed")
}

// GetPow fetches a proof-of-work challenge for the completion endpoint.
func (c *Client) GetPow(ctx context.Context, maxAttempts int) (string, error) {
	return c.GetPowForTarget(ctx, dsprotocol.DeepSeekCompletionTargetPath, maxAttempts)
}

// GetPowForTarget fetches a proof-of-work challenge for a specific target path.
func (c *Client) GetPowForTarget(ctx context.Context, targetPath string, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		targetPath = dsprotocol.DeepSeekCompletionTargetPath
	}
	clients := c.requestClients()
	attempts := 0
	for attempts < maxAttempts {
		headers := c.authHeaders()
		resp, status, err := c.postJSONWithStatus(ctx, clients.regular, clients.fallback, dsprotocol.DeepSeekCreatePowURL, headers, map[string]any{"target_path": targetPath})
		if err != nil {
			config.Logger.Warn("[get_pow] request error", "error", err, "target_path", targetPath)
			attempts++
			continue
		}
		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			data, _ := resp["data"].(map[string]any)
			bizData, _ := data["biz_data"].(map[string]any)
			challenge, _ := bizData["challenge"].(map[string]any)
			answer, err := ComputePow(ctx, challenge)
			if err != nil {
				attempts++
				continue
			}
			return BuildPowHeader(challenge, answer)
		}
		config.Logger.Warn("[get_pow] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "target_path", targetPath)
		attempts++
	}
	return "", errors.New("get pow failed")
}

// authHeaders returns headers for DeepSeek API requests using the gateway key.
func (c *Client) authHeaders() map[string]string {
	headers := make(map[string]string, len(dsprotocol.BaseHeaders)+1)
	for k, v := range dsprotocol.BaseHeaders {
		headers[k] = v
	}
	headers["authorization"] = "Bearer " + c.deepseekKey
	return headers
}

// extractCreateSessionID extracts the session ID from create-session API response.
func extractCreateSessionID(resp map[string]any) string {
	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	if sessionID, _ := bizData["id"].(string); strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
	if chatSession, ok := bizData["chat_session"].(map[string]any); ok {
		if sessionID, _ := chatSession["id"].(string); strings.TrimSpace(sessionID) != "" {
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

func extractResponseStatus(resp map[string]any) (code int, bizCode int, msg string, bizMsg string) {
	code = intFrom(resp["code"])
	msg, _ = resp["msg"].(string)
	data, _ := resp["data"].(map[string]any)
	bizCode = intFrom(data["biz_code"])
	bizMsg, _ = data["biz_msg"].(string)
	if strings.TrimSpace(bizMsg) == "" {
		if bizData, ok := data["biz_data"].(map[string]any); ok {
			bizMsg, _ = bizData["msg"].(string)
		}
	}
	return code, bizCode, msg, bizMsg
}

func normalizeMobileForLogin(raw string) (mobile string, areaCode any) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	hasPlus := strings.HasPrefix(s, "+")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return "", nil
	}
	if (hasPlus || strings.HasPrefix(digits, "86")) && strings.HasPrefix(digits, "86") && len(digits) == 13 {
		return digits[2:], nil
	}
	return digits, nil
}
