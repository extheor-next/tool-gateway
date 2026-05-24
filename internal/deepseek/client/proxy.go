package client

import (
	"context"
	dsprotocol "tool-gateway/internal/deepseek/protocol"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"tool-gateway/internal/config"
	trans "tool-gateway/internal/deepseek/transport"
)

type requestClients struct {
	regular   trans.Doer
	stream    trans.Doer
	fallback  *http.Client
	fallbackS *http.Client
}

type hostLookupFunc func(ctx context.Context, network, host string) ([]string, error)

var proxyConnectivityTestURL = "https://chat.deepseek.com/"

var defaultHostLookup hostLookupFunc = func(ctx context.Context, _ string, host string) ([]string, error) {
	return net.DefaultResolver.LookupHost(ctx, host)
}

func proxyDialAddress(ctx context.Context, proxyType, address string, lookup hostLookupFunc) (string, error) {
	proxyType = strings.ToLower(strings.TrimSpace(proxyType))
	if proxyType != "socks5" {
		return address, nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if net.ParseIP(host) != nil {
		return address, nil
	}
	if lookup == nil {
		lookup = defaultHostLookup
	}
	addrs, err := lookup(ctx, "ip", host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("no ip address resolved for %s", host)
	}
	return net.JoinHostPort(addrs[0], port), nil
}

func proxyCacheKey(proxyCfg config.Proxy) string {
	proxyCfg = config.NormalizeProxy(proxyCfg)
	return strings.Join([]string{
		proxyCfg.ID,
		proxyCfg.Type,
		strings.ToLower(proxyCfg.Host),
		strconv.Itoa(proxyCfg.Port),
		proxyCfg.Username,
		proxyCfg.Password,
	}, "|")
}

func proxyDialContext(proxyCfg config.Proxy) (trans.DialContextFunc, error) {
	proxyCfg = config.NormalizeProxy(proxyCfg)
	var authCfg *proxy.Auth
	if proxyCfg.Username != "" || proxyCfg.Password != "" {
		authCfg = &proxy.Auth{User: proxyCfg.Username, Password: proxyCfg.Password}
	}
	forward := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	dialer, err := proxy.SOCKS5("tcp", net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port)), authCfg, forward)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target, err := proxyDialAddress(ctx, proxyCfg.Type, address, defaultHostLookup)
		if err != nil {
			return nil, err
		}
		if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
			return ctxDialer.DialContext(ctx, network, target)
		}
		return dialer.Dial(network, target)
	}, nil
}

func (c *Client) defaultRequestClients() requestClients {
	return requestClients{
		regular:   c.regular,
		stream:    c.stream,
		fallback:  c.fallback,
		fallbackS: c.fallbackS,
	}
}

// requestClients returns the appropriate HTTP clients, optionally routing
// through a configured proxy. The proxy config comes from the Client struct.
func (c *Client) requestClients() requestClients {
	if c == nil {
		return requestClients{}
	}
	if c.proxyCfg.Host == "" || c.proxyCfg.Port == 0 {
		return c.defaultRequestClients()
	}
	proxyCfg := config.NormalizeProxy(c.proxyCfg)

	c.proxyClientsMu.RLock()
	if c.proxyInit {
		clients := c.proxyClients
		c.proxyClientsMu.RUnlock()
		return clients
	}
	c.proxyClientsMu.RUnlock()

	c.proxyClientsMu.Lock()
	defer c.proxyClientsMu.Unlock()
	if c.proxyInit {
		return c.proxyClients
	}

	dialContext, err := proxyDialContext(proxyCfg)
	if err != nil {
		config.Logger.Warn("[proxy] build dialer failed", "proxy_id", proxyCfg.ID, "error", err)
		c.proxyClients = c.defaultRequestClients()
		c.proxyInit = true
		return c.proxyClients
	}

	c.proxyClients = requestClients{
		regular:   trans.NewWithDialContext(60*time.Second, dialContext),
		stream:    trans.NewWithDialContext(0, dialContext),
		fallback:  trans.NewFallbackClient(60*time.Second, dialContext),
		fallbackS: trans.NewFallbackClient(0, dialContext),
	}
	c.proxyInit = true
	return c.proxyClients
}

func applyProxyConnectivityHeaders(req *http.Request) {
	if req == nil {
		return
	}
	for key, value := range dsprotocol.BaseHeaders {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		req.Header.Set(key, value)
	}
}

func proxyConnectivityStatus(statusCode int) (bool, string) {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return true, fmt.Sprintf("代理可达，目标返回 HTTP %d", statusCode)
	case statusCode >= 300 && statusCode < 500:
		return true, fmt.Sprintf("代理可达，但目标返回 HTTP %d（可能是风控或挑战）", statusCode)
	default:
		return false, fmt.Sprintf("目标返回 HTTP %d", statusCode)
	}
}

func TestProxyConnectivity(ctx context.Context, proxyCfg config.Proxy) map[string]any {
	start := time.Now()
	proxyCfg = config.NormalizeProxy(proxyCfg)
	result := map[string]any{
		"success":       false,
		"proxy_id":      proxyCfg.ID,
		"proxy_type":    proxyCfg.Type,
		"response_time": 0,
	}

	if err := config.ValidateProxyConfig([]config.Proxy{proxyCfg}); err != nil {
		result["message"] = "代理配置无效: " + err.Error()
		return result
	}
	dialContext, err := proxyDialContext(proxyCfg)
	if err != nil {
		result["message"] = "代理拨号器初始化失败: " + err.Error()
		return result
	}

	client := trans.NewFallbackClient(15*time.Second, dialContext)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyConnectivityTestURL, nil)
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	applyProxyConnectivityHeaders(req)

	resp, err := client.Do(req)
	result["response_time"] = int(time.Since(start).Milliseconds())
	if err != nil {
		result["message"] = err.Error()
		return result
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			config.Logger.Warn("[proxy] close response body failed", "proxy_id", proxyCfg.ID, "error", closeErr)
		}
	}()

	result["status_code"] = resp.StatusCode
	result["success"], result["message"] = proxyConnectivityStatus(resp.StatusCode)
	return result
}
