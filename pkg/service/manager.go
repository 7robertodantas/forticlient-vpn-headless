package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"fcvpn"
)

// Manager handles starting/stopping FortiClient background services.
type Manager struct {
	fctsched   *exec.Cmd
	configHand *exec.Cmd
	fortitray  *exec.Cmd
	mu         sync.Mutex
	ports      []string
}

// NewManager creates a new service manager.
func NewManager() *Manager {
	return &Manager{}
}

// StartAll starts all required FortiClient services and returns the service port.
func (m *Manager) StartAll(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Start fctsched (scheduler)
	if err := m.startService(ctx, "fctsched", fcvpn.BinFctsched); err != nil {
		return "", fmt.Errorf("start fctsched: %w", err)
	}

	// Start confighandler (config DB + HTTP server)
	if err := m.startService(ctx, "confighandler", fcvpn.BinConfigHand); err != nil {
		return "", fmt.Errorf("start confighandler: %w", err)
	}

	// Start fortitray (fctservice - WebSocket + IPC)
	if err := m.startService(ctx, "fortitray", fcvpn.BinFortitray); err != nil {
		return "", fmt.Errorf("start fortitray: %w", err)
	}

	// Wait for the service port file to be created
	port, err := m.waitForServicePort(ctx)
	if err != nil {
		m.StopAll()
		return "", fmt.Errorf("wait for service port: %w", err)
	}

	return port, nil
}

// StartRequired starts only services that aren't already running.
func (m *Manager) StartRequired(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if fctservice is already running by reading the port file
	port, err := m.getExistingPort()
	if err == nil && port != "" {
		// Service is already running, verify it's alive
		if m.isServiceAlive() {
			return port, nil
		}
		// Port file exists but service is dead, clean up
	}

	// Start services fresh
	if err := m.startService(ctx, "fctsched", fcvpn.BinFctsched); err != nil {
		return "", fmt.Errorf("start fctsched: %w", err)
	}

	if err := m.startService(ctx, "confighandler", fcvpn.BinConfigHand); err != nil {
		return "", fmt.Errorf("start confighandler: %w", err)
	}

	if err := m.startService(ctx, "fortitray", fcvpn.BinFortitray); err != nil {
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

	if m.fortitray != nil && m.fortitray.Process != nil {
		m.fortitray.Process.Signal(syscall.SIGTERM)
		m.fortitray.Wait()
		m.fortitray = nil
	}

	if m.configHand != nil && m.configHand.Process != nil {
		m.configHand.Process.Signal(syscall.SIGTERM)
		m.configHand.Wait()
		m.configHand = nil
	}

	if m.fctsched != nil && m.fctsched.Process != nil {
		m.fctsched.Process.Signal(syscall.SIGTERM)
		m.fctsched.Wait()
		m.fctsched = nil
	}
}

// Status checks whether the FortiClient services are running.
func (m *Manager) Status() string {
	var status []string

	// Check each binary
	for _, name := range []string{"fctsched", "confighandler", "fortitray"} {
		cmd := exec.Command("pgrep", "-x", name)
		if err := cmd.Run(); err == nil {
			status = append(status, fmt.Sprintf("  %s: running", name))
		} else {
			status = append(status, fmt.Sprintf("  %s: stopped", name))
		}
	}

	// Check port file
	port, err := os.ReadFile(fcvpn.ServicePortPath)
	if err == nil && len(strings.TrimSpace(string(port))) > 0 {
		status = append(status, fmt.Sprintf("  service_port: %s", strings.TrimSpace(string(port))))
	} else {
		status = append(status, "  service_port: not found")
	}

	return strings.Join(status, "\n")
}

// isServiceAlive checks if the fctservice (fortitray) is still running.
func (m *Manager) isServiceAlive() bool {
	cmd := exec.Command("pgrep", "-x", "fortitray")
	return cmd.Run() == nil
}

// getExistingPort reads the service port file without starting services.
func (m *Manager) getExistingPort() (string, error) {
	data, err := os.ReadFile(fcvpn.ServicePortPath)
	if err != nil {
		return "", err
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		return "", fcvpn.ErrNoServicePort
	}
	return port, nil
}

// waitForServicePort waits for the service port file to appear.
func (m *Manager) waitForServicePort(ctx context.Context) (string, error) {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for service port: %w", ctx.Err())
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for service port file %s", fcvpn.ServicePortPath)
		case <-ticker.C:
			port, err := m.getExistingPort()
			if err == nil {
				return port, nil
			}
		}
	}
}

// startService starts a FortiClient service binary as a background process.
func (m *Manager) startService(ctx context.Context, name, path string) error {
	// Check if already running
	cmd := exec.Command("pgrep", "-x", name)
	if cmd.Run() == nil {
		return nil // Already running
	}

	// Check binary exists
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("binary not found: %s", path)
	}

	// Start the process
	proc := exec.CommandContext(ctx, path)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Put in own process group for cleanup
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
