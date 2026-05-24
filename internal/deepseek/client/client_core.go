package client

import (
	"context"
	"net/http"
	"sync"
	"time"

	"tool-gateway/internal/config"
	trans "tool-gateway/internal/deepseek/transport"
	"tool-gateway/internal/devcapture"
	"tool-gateway/internal/util"
)

// intFrom is a package-internal alias for the shared util version.
var intFrom = util.IntFrom

type Client struct {
	deepseekKey string
	capture     *devcapture.Store
	regular     trans.Doer
	stream      trans.Doer
	fallback    *http.Client
	fallbackS   *http.Client
	maxRetries  int

	proxyCfg       config.Proxy
	proxyClientsMu sync.RWMutex
	proxyClients   requestClients
	proxyInit      bool
}

func NewClient(deepseekKey string) *Client {
	return &Client{
		deepseekKey: deepseekKey,
		capture:     devcapture.Global(),
		regular:     trans.New(60 * time.Second),
		stream:      trans.New(0),
		fallback:    &http.Client{Timeout: 60 * time.Second},
		fallbackS:   &http.Client{Timeout: 0},
		maxRetries:  3,
	}
}

// PreloadPow 保留兼容接口，纯 Go 实现无需预加载。
func (c *Client) PreloadPow(_ context.Context) error {
	return nil
}
