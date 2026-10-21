//go:build linux

// Package linux provides the service manager for FortiClient VPN on Linux.
package linux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"fcvpn/pkg/platform"
)

// Manager handles starting/stopping FortiClient background services on Linux.
type Manager struct {
	platform  platform.Platform
	fctsched   *exec.Cmd
	configHand *exec.Cmd
	fortitray  *exec.Cmd
	mu         sync.Mutex
}

// NewManager creates a new Linux service manager.
func NewManager(plat platform.Platform) *Manager {
	return &Manager{platform: plat}
}

// StartRequired starts only services that aren't already running.
func (m *Manager) StartRequired(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if fctservice is already running by reading the port file
	port, err := m.getExistingPort()
	if err == nil && port != "" {
		if m.isServiceAlive() {
			return port, nil
		}
	}

	bin := m.platform.Binaries()

	if err := m.startService(ctx, "fctsched", bin.Sched); err != nil {
		return "", fmt.Errorf("start fctsched: %w", err)
	}

	if err := m.startService(ctx, "confighandler", bin.Config); err != nil {
		return "", fmt.Errorf("start confighandler: %w", err)
	}

	if err := m.startService(ctx, "fortitray", bin.Tray); err != nil {
		return "", fmt.Errorf("start fortitray: %w", err)
	}

	port, err = m.waitForServicePort(ctx)
	if err != nil {
		m.StopAll()
		return "", fmt.Errorf("wait for service port: %w", err)
	}

	return port, nil
}

// StopAll stops all FortiClient services that we started.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, svc := range []*exec.Cmd{m.fortitray, m.configHand, m.fctsched} {
		if svc != nil && svc.Process != nil {
			svc.Process.Signal(syscall.SIGTERM)
			svc.Wait()
		}
	}
	m.fctsched = nil
	m.configHand = nil
	m.fortitray = nil
}

// Status checks whether the FortiClient services are running.
func (m *Manager) Status() string {
	var status []string
	bin := m.platform.Binaries()

	for _, entry := range []struct {
		name string
		path string
	}{
		{"fctsched", bin.Sched},
		{"confighandler", bin.Config},
		{"fortitray", bin.Tray},
	} {
		if entry.path == "" {
			continue
		}
		if isProcessRunning(entry.name) {
			status = append(status, fmt.Sprintf("  %s: running", entry.name))
		} else {
			status = append(status, fmt.Sprintf("  %s: stopped", entry.name))
		}
	}

	portFile := m.platform.ServicePortPath()
	if portFile != "" {
		port, err := os.ReadFile(portFile)
		if err == nil && len(strings.TrimSpace(string(port))) > 0 {
			status = append(status, fmt.Sprintf("  service_port: %s", strings.TrimSpace(string(port))))
		} else {
			status = append(status, "  service_port: not found")
		}
	}

	return strings.Join(status, "\n")
}

func (m *Manager) isServiceAlive() bool {
	return isProcessRunning("fortitray")
}

func (m *Manager) getExistingPort() (string, error) {
	portFile := m.platform.ServicePortPath()
	if portFile == "" {
		return "", fmt.Errorf("no service port path for this platform")
	}
	data, err := os.ReadFile(portFile)
	if err != nil {
		return "", err
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		return "", fmt.Errorf("empty service port file")
	}
	return port, nil
}

func (m *Manager) waitForServicePort(ctx context.Context) (string, error) {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for service port: %w", ctx.Err())
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for service port file %s", m.platform.ServicePortPath())
		case <-ticker.C:
			port, err := m.getExistingPort()
			if err == nil {
				return port, nil
			}
		}
	}
}

func (m *Manager) startService(ctx context.Context, name, path string) error {
	if path == "" {
		return nil // Not applicable on this platform
	}

	if isProcessRunning(name) {
		return nil // Already running
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("binary not found: %s", path)
	}

	proc := exec.CommandContext(ctx, path)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := proc.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	switch name {
	case "fctsched":
		m.fctsched = proc
	case "confighandler":
		m.configHand = proc
	case "fortitray":
		m.fortitray = proc
	}

	return nil
}

func isProcessRunning(name string) bool {
	cmd := exec.Command("pgrep", "-x", name)
	return cmd.Run() == nil
}
