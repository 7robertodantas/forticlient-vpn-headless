//go:build linux

// Package linux provides the WebSocket connector for FortiClient VPN on Linux.
// Connects to fctservice via HTTP-detected dynamic port.
package linux

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"fcvpn/pkg/platform"

	"github.com/gorilla/websocket"
)

// Connector creates WebSocket connections to fctservice on Linux.
type Connector struct {
	port  string
	url   string
	platform.Platform
}

// NewConnector creates a new Linux WebSocket connector.
func NewConnector(plat platform.Platform, port string) *Connector {
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

// Connect dials the fctservice WebSocket endpoint.
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

// ReadPort reads the service port from the platform-specific port file.
func ReadPort(plat platform.Platform) (string, error) {
	portFile := plat.ServicePortPath()
	if portFile == "" {
		return "", fmt.Errorf("no service port path for Linux")
	}
	data, err := os.ReadFile(portFile)
	if err != nil {
		return "", fmt.Errorf("read service port file %s: %w", portFile, err)
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		return "", fmt.Errorf("empty service port file")
	}
	return port, nil
}
