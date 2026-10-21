// Package service provides VPN service lifecycle management.
//
// The shared manager delegates to platform-specific implementations:
//
//   - pkg/service/linux/manager.go   (Linux — starts/stops fctsched, confighandler, fortitray)
//   - pkg/service/darwin/manager.go  (macOS — detects LaunchDaemon services)
package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"fcvpn/pkg/platform"
)

// Manager handles starting/stopping FortiClient services.
// Built with the appropriate platform-specific manager via build tags.
type Manager struct {
	manager platformServiceManager
	plat    platform.Platform
}

// platformServiceManager is the internal service manager interface.
type platformServiceManager interface {
	StartRequired(ctx context.Context) (string, error)
	StopAll()
	Status() string
}

// NewManager creates a platform-specific service manager.
func NewManager(plat platform.Platform) *Manager {
	return &Manager{
		manager: newPlatformManager(plat),
		plat:    plat,
	}
}

// StartRequired starts only services that aren't already running.
// Returns the service port to use for fctservice communication.
func (m *Manager) StartRequired(ctx context.Context) (string, error) {
	return m.manager.StartRequired(ctx)
}

// StopAll stops all services that we started.
func (m *Manager) StopAll() {
	m.manager.StopAll()
}

// Status returns a human-readable status string.
func (m *Manager) Status() string {
	return m.manager.Status()
}

// GetServicePort returns the service port for fctservice communication.
// On Linux, reads from the service port file. On macOS, returns the bridge port.
func GetServicePort(plat platform.Platform) (string, error) {
	// Check for existing service port file (Linux)
	portFile := plat.ServicePortPath()
	if portFile != "" {
		data, err := os.ReadFile(portFile)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			return strings.TrimSpace(string(data)), nil
		}
	}

	// Fall back to bridge port (macOS)
	bridgePort := plat.BridgePort()
	if bridgePort != "" {
		return bridgePort, nil
	}

	return "", fmt.Errorf("no service port available")
}
