package client

import (
	"context"
	dsprotocol "tool-gateway/internal/deepseek/protocol"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tool-gateway/internal/config"
)

// SessionInfo 会话信息
type SessionInfo struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	TitleType string  `json:"title_type"`
	Pinned    bool    `json:"pinned"`
	UpdatedAt float64 `json:"updated_at"`
}

// SessionStats 会话统计结果
type SessionStats struct {
	CallerID       string // caller identifier
	FirstPageCount int    // 第一页会话数量（当 HasMore 为 true 时，真实总数可能更大）
	PinnedCount    int    // 置顶会话数量
	HasMore        bool   // 是否还有更多页
	Success        bool   // 请求是否成功
	ErrorMessage   string // 错误信息
}

// GetSessionCount 获取会话数量
func (c *Client) GetSessionCount(ctx context.Context, callerID string, maxAttempts int) (*SessionStats, error) {
	if maxAttempts <= 0 {
		maxAttempts = c.maxRetries
	}
	clients := c.requestClients()

	stats := &SessionStats{
		CallerID: callerID,
	}

	attempts := 0
	for attempts < maxAttempts {
		headers := c.authHeaders()

		// 构建请求 URL
		reqURL := dsprotocol.DeepSeekFetchSessionURL + "?lte_cursor.pinned=false"

		resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
		if err != nil {
			config.Logger.Warn("[get_session_count] request error", "error", err, "caller", callerID)
			attempts++
			continue
		}

		code, bizCode, msg, bizMsg := extractResponseStatus(resp)
		if status == http.StatusOK && code == 0 && bizCode == 0 {
			data, _ := resp["data"].(map[string]any)
			bizData, _ := data["biz_data"].(map[string]any)
			chatSessions, _ := bizData["chat_sessions"].([]any)
			hasMore, _ := bizData["has_more"].(bool)

			stats.FirstPageCount = len(chatSessions)
			stats.HasMore = hasMore
			stats.Success = true

			// 统计置顶会话数量
			for _, session := range chatSessions {
				if s, ok := session.(map[string]any); ok {
					if pinned, ok := s["pinned"].(bool); ok && pinned {
						stats.PinnedCount++
					}
				}
			}

			return stats, nil
		}

		stats.ErrorMessage = fmt.Sprintf("status=%d, code=%d, msg=%s", status, code, msg)
		config.Logger.Warn("[get_session_count] failed", "status", status, "code", code, "biz_code", bizCode, "msg", msg, "biz_msg", bizMsg, "caller", callerID)

		attempts++
	}

	stats.Success = false
	stats.ErrorMessage = "get session count failed after retries"
	return stats, errors.New(stats.ErrorMessage)
}

// GetSessionCountForToken 直接使用 token 获取会话数量（直通模式）
func (c *Client) GetSessionCountForToken(ctx context.Context, token string) (*SessionStats, error) {
	clients := c.requestClients()
	headers := c.authHeaders()
	reqURL := dsprotocol.DeepSeekFetchSessionURL + "?lte_cursor.pinned=false"

	resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, err
	}

	code, bizCode, msg, bizMsg := extractResponseStatus(resp)
	if status != http.StatusOK || code != 0 || bizCode != 0 {
		if strings.TrimSpace(bizMsg) != "" {
			msg = bizMsg
		}
		return nil, fmt.Errorf("request failed: status=%d, code=%d, msg=%s", status, code, msg)
	}

	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	chatSessions, _ := bizData["chat_sessions"].([]any)
	hasMore, _ := bizData["has_more"].(bool)

	stats := &SessionStats{
		FirstPageCount: len(chatSessions),
		HasMore:        hasMore,
		Success:        true,
	}

	// 统计置顶会话数量
	for _, session := range chatSessions {
		if s, ok := session.(map[string]any); ok {
			if pinned, ok := s["pinned"].(bool); ok && pinned {
				stats.PinnedCount++
			}
		}
	}

	return stats, nil
}

// FetchSessionPage 获取会话列表（支持分页）
func (c *Client) FetchSessionPage(ctx context.Context, cursor string) ([]SessionInfo, bool, error) {
	clients := c.requestClients()
	headers := c.authHeaders()

	// 构建请求 URL
	params := url.Values{}
	params.Set("lte_cursor.pinned", "false")
	if cursor != "" {
		params.Set("lte_cursor", cursor)
	}
	reqURL := dsprotocol.DeepSeekFetchSessionURL + "?" + params.Encode()

	resp, status, err := c.getJSONWithStatus(ctx, clients.regular, reqURL, headers)
	if err != nil {
		return nil, false, err
	}

	code := intFrom(resp["code"])
	if status != http.StatusOK || code != 0 {
		msg, _ := resp["msg"].(string)
		return nil, false, fmt.Errorf("request failed: status=%d, code=%d, msg=%s", status, code, msg)
	}

	data, _ := resp["data"].(map[string]any)
	bizData, _ := data["biz_data"].(map[string]any)
	chatSessions, _ := bizData["chat_sessions"].([]any)
	hasMore, _ := bizData["has_more"].(bool)

	sessions := make([]SessionInfo, 0, len(chatSessions))
	for _, s := range chatSessions {
		if m, ok := s.(map[string]any); ok {
			session := SessionInfo{
				ID:        stringFromMap(m, "id"),
				Title:     stringFromMap(m, "title"),
				TitleType: stringFromMap(m, "title_type"),
				Pinned:    boolFromMap(m, "pinned"),
				UpdatedAt: floatFromMap(m, "updated_at"),
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, hasMore, nil
}

// 辅助函数
func stringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolFromMap(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func floatFromMap(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
