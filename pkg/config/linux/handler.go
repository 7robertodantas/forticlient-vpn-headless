//go:build linux

// Package linux provides the config handler for FortiClient VPN on Linux.
// Communicates with confighandler via its HTTP API.
package linux

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fcvpn/pkg/platform"
)

// Handler communicates with the confighandler service via its HTTP API.
type Handler struct {
	platform platform.Platform
	HTTPBase string
	client   *http.Client
}

// NewHandler creates a new config handler for the given port.
func NewHandler(plat platform.Platform, port string) *Handler {
	return &Handler{
		platform: plat,
		HTTPBase: fmt.Sprintf("http://127.0.0.1:%s", port),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (h *Handler) ImportConfig(ctx context.Context, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", h.HTTPBase+"/config", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create config import request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("import config request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Handle 409 Conflict (already exists) by trying PUT
	if resp.StatusCode == http.StatusConflict {
		req2, _ := http.NewRequestWithContext(ctx, "PUT", h.HTTPBase+"/config", bytes.NewReader(data))
		req2.Header.Set("Content-Type", "application/octet-stream")
		resp2, err := h.client.Do(req2)
		if err != nil {
			return nil
		}
		resp2.Body.Close()
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("config import failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (h *Handler) ListConnections(ctx context.Context) ([]platform.Connection, error) {
	resp, err := h.client.Get(h.HTTPBase + "/vpn")
	if err != nil {
		return nil, fmt.Errorf("list configs request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read config response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list configs failed (HTTP %d)", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse config response: %w", err)
	}

	return extractConnections(result)
}

func (h *Handler) GetConnection(ctx context.Context, name string) (*platform.Connection, error) {
	conns, err := h.ListConnections(ctx)
	if err != nil {
		return nil, err
	}

	for i := range conns {
		if conns[i].Name == name {
			return &conns[i], nil
		}
	}
	return nil, fmt.Errorf("connection %q not found", name)
}

func (h *Handler) DeleteConnection(ctx context.Context, name string) error {
	url := fmt.Sprintf("%s/vpn?name=%s", h.HTTPBase, name)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete connection failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (h *Handler) GetVPNState(ctx context.Context, name string) (int, error) {
	url := fmt.Sprintf("%s/vpn/state?name=%s", h.HTTPBase, name)
	resp, err := h.client.Get(url)
	if err != nil {
		return -1, fmt.Errorf("get VPN state: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return -1, fmt.Errorf("parse state response: %w", err)
	}

	if state, ok := result["state"].(float64); ok {
		return int(state), nil
	}
	return -1, nil
}

func extractConnections(result map[string]interface{}) ([]platform.Connection, error) {
	var conns []platform.Connection

	var data map[string]interface{}
	if d, ok := result["data"].(map[string]interface{}); ok {
		data = d
	} else if vpn, ok := result["vpn"].(map[string]interface{}); ok {
		data = vpn
	} else {
		data = result
	}

	if ipsec, ok := data["ipsecvpn"].(map[string]interface{}); ok {
		if connections, ok := ipsec["connections"].([]interface{}); ok {
			for _, c := range connections {
				if connMap, ok := c.(map[string]interface{}); ok {
					conns = append(conns, connectionFromMap(connMap))
				}
			}
		}
	}

	if sslvpn, ok := data["sslvpn"].(map[string]interface{}); ok {
		if connections, ok := sslvpn["connections"].([]interface{}); ok {
			for _, c := range connections {
				if connMap, ok := c.(map[string]interface{}); ok {
					conns = append(conns, connectionFromMap(connMap))
				}
			}
		}
	}

	return conns, nil
}

func connectionFromMap(m map[string]interface{}) platform.Connection {
	var c platform.Connection

	if name, ok := m["name"].(string); ok {
		c.Name = name
	}
	if t, ok := m["type"].(string); ok {
		c.Type = t
	}
	if uid, ok := m["uid"].(string); ok {
		c.UID = uid
	}

	if ike, ok := m["ike_settings"].(map[string]interface{}); ok {
		c.IKESettings = platform.IKESettings{
			Server:            strVal(ike, "server"),
			Version:           intVal(ike, "version"),
			Mode:              strVal(ike, "mode"),
			AuthMethod:        strVal(ike, "authentication_method"),
			SSOEnabled:        intVal(ike, "sso_enabled"),
			UseExternalBrowser: intVal(ike, "use_external_browser"),
			IKEsamlPort:       intVal(ike, "ike_saml_port"),
			TCPPort:           intVal(ike, "tcp_port"),
			XAuthTimeout:      intVal(ike, "xauth_timeout"),
		}
	}

	return c
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
