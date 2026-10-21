//go:build darwin

// Package darwin provides the WebSocket connector for FortiClient VPN on macOS.
// Connects to the bridge service which translates WebSocket ↔ Unix socket (fctctld).
package darwin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"fcvpn/pkg/platform"

	"github.com/gorilla/websocket"
)

// Connector creates WebSocket connections to the FortiClient bridge on macOS.
type Connector struct {
	port  string
	url   string
	platform.Platform
}

// NewConnector creates a new macOS WebSocket connector.
// Uses the bridge port from platform config (default: 9080).
func NewConnector(plat platform.Platform) *Connector {
	port := plat.BridgePort()
	return &Connector{
		port:     port,
		url:      fmt.Sprintf("ws://127.0.0.1:%s/websocket", port),
		Platform: plat,
	}
}

// URL returns the WebSocket URL.
func (c *Connector) URL() string {
	return c.url
}

// Connect dials the bridge WebSocket endpoint.
func (c *Connector) Connect(ctx context.Context) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	header.Set("Origin", "http://127.0.0.1")

	conn, _, err := dialer.Dial(c.url, header)
	if err != nil {
		return nil, fmt.Errorf("dial WebSocket %s: %w", c.url, err)
	}

	return conn, nil
}
