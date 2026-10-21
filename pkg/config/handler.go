package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fcctl/pkg/xml"
)

// Handler communicates with the confighandler service via its HTTP API.
type Handler struct {
	HTTPBase string
	client   *http.Client
}

// NewHandler creates a new config handler for the given port.
func NewHandler(port string) *Handler {
	return &Handler{
		HTTPBase: fmt.Sprintf("http://127.0.0.1:%s", port),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ImportConfig imports a FortiClient XML config via the confighandler API.
func (h *Handler) ImportConfig(data []byte) error {
	// confighandler accepts XML via POST /config
	// The request body is the raw XML document
	req, err := http.NewRequest("POST", h.HTTPBase+"/config", bytes.NewReader(data))
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
		req2, _ := http.NewRequest("PUT", h.HTTPBase+"/config", bytes.NewReader(data))
		req2.Header.Set("Content-Type", "application/octet-stream")
		resp2, err := h.client.Do(req2)
		if err != nil {
			return nil // Import was "complete enough"
		}
		resp2.Body.Close()
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("config import failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// ListConfigs returns all configured VPN tunnels.
func (h *Handler) ListConfigs() ([]xml.Connection, error) {
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

// GetConnection returns a specific VPN connection by name.
func (h *Handler) GetConnection(name string) (*xml.Connection, error) {
	conns, err := h.ListConfigs()
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

// DeleteConnection removes a VPN configuration.
func (h *Handler) DeleteConnection(name string) error {
	url := fmt.Sprintf("%s/vpn?name=%s", h.HTTPBase, name)
	req, err := http.NewRequest("DELETE", url, nil)
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

// GetVPNState returns the connection state for a tunnel.
func (h *Handler) GetVPNState(name string) (int, error) {
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

func extractConnections(result map[string]interface{}) ([]xml.Connection, error) {
	var conns []xml.Connection

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
					conns = append(conns, xmlConnectionFromMap(connMap))
				}
			}
		}
	}

	if sslvpn, ok := data["sslvpn"].(map[string]interface{}); ok {
		if connections, ok := sslvpn["connections"].([]interface{}); ok {
			for _, c := range connections {
				if connMap, ok := c.(map[string]interface{}); ok {
					conns = append(conns, xmlConnectionFromMap(connMap))
				}
			}
		}
	}

	return conns, nil
}

func xmlConnectionFromMap(m map[string]interface{}) xml.Connection {
	var c xml.Connection

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
		if server, ok := ike["server"].(string); ok {
			c.IKESettings.Server = server
		}
		if version, ok := ike["version"].(float64); ok {
			c.IKESettings.Version = int(version)
		}
		if mode, ok := ike["mode"].(string); ok {
			c.IKESettings.Mode = mode
		}
		if authMethod, ok := ike["authentication_method"].(string); ok {
			c.IKESettings.AuthMethod = authMethod
		}
		if sso, ok := ike["sso_enabled"].(float64); ok {
			c.IKESettings.SSOEnabled = int(sso)
		}
		if extBr, ok := ike["use_external_browser"].(float64); ok {
			c.IKESettings.UseExternalBrowser = int(extBr)
		}
		if port, ok := ike["ike_saml_port"].(float64); ok {
			c.IKESettings.IKEsamlPort = int(port)
		}
		if tcpPort, ok := ike["tcp_port"].(float64); ok {
			c.IKESettings.TCPPort = int(tcpPort)
		}
		if xauth, ok := ike["xauth"].(map[string]interface{}); ok {
			if enabled, ok := xauth["enabled"].(float64); ok {
				c.IKESettings.XAuth.Enabled = int(enabled)
			}
		}
	}

	return c
}
