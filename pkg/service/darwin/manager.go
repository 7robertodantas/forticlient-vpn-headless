//go:build darwin

// Package darwin provides the service manager for FortiClient VPN on macOS.
// On macOS, the bridge server is embedded and started automatically by fcvpn.
package darwin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"fcvpn/pkg/bridge"
	"fcvpn/pkg/platform"
)

// Manager handles service lifecycle on macOS.
// On macOS, fctctld runs as a LaunchDaemon and the bridge is embedded in fcvpn.
type Manager struct {
	platform platform.Platform
	mu       sync.Mutex
	server   *bridge.Server
}

// NewManager creates a new macOS service manager.
func NewManager(plat platform.Platform) *Manager {
	return &Manager{platform: plat}
}

// StartRequired ensures the bridge server is running.
// Returns the bridge port for fctservice communication.
func (m *Manager) StartRequired(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bridgePort := m.platform.BridgePort()
	if bridgePort == "" {
		bridgePort = "9080"
	}

	// Check if fctctld is available
	socketPath := "/var/run/fctvpnctrl.sock"
	if _, err := os.Stat(socketPath); err != nil {
		return "", fmt.Errorf(
			"fctctld socket not found: %s\n"+
				"Ensure FortiClient VPN is installed and running.", socketPath)
	}

	// Start embedded bridge server
	m.server = bridge.NewServer(bridgePort, socketPath)
	if err := m.server.Start(); err != nil {
		return "", fmt.Errorf("start bridge: %w", err)
	}

	return m.server.Port(), nil
}

// StopAll stops the embedded bridge server.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		m.server.Stop()
		m.server = nil
	}
}

// Status checks whether the FortiClient services are running on macOS.
func (m *Manager) Status() string {
	var status []string

	if m.isProcessRunning("fctctld") {
		status = append(status, "  fctctld (control daemon): running")
	} else {
		status = append(status, "  fctctld (control daemon): stopped")
	}

	bridgePort := m.platform.BridgePort()
	if m.server != nil {
		status = append(status, fmt.Sprintf("  bridge (port %s): embedded", bridgePort))
	} else if m.isPortListening(bridgePort) {
		status = append(status, fmt.Sprintf("  bridge (port %s): external", bridgePort))
	} else {
		status = append(status, fmt.Sprintf("  bridge (port %s): not listening", bridgePort))
	}

	return strings.Join(status, "\n")
}

func (m *Manager) isProcessRunning(name string) bool {
	cmd := exec.Command("pgrep", "-x", name)
	return cmd.Run() == nil
}

func (m *Manager) isPortListening(port string) bool {
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%s", port))
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}
