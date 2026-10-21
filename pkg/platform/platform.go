// Package platform provides platform-agnostic configuration for FortiClient VPN.
// Each platform implements the Platform interface via build tags.
package platform

import (
	"context"
)

// Platform defines all platform-specific paths, services, and binaries.
// Implementations live in pkg/platform/linux/ and pkg/platform/darwin/.
type Platform interface {
	// ForticlientBase returns the base directory where FortiClient is installed.
	// Linux: /opt/forticlient
	// macOS: /Library/Application Support/Fortinet/FortiClient
	BinicDir() string

	// ServicePortPath returns the path to the service port file (Linux only).
	// On macOS, this is empty — the bridge uses a fixed port.
	ServicePortPath() string

	// BridgePort returns the port the bridge service listens on.
	// On Linux, this is empty — fctservice uses a dynamic port.
	BridgePort() string

	// Binaries returns the set of required FortiClient binaries.
	Binaries() Binaries

	// Services returns the set of system services that must be running.
	Services() Services

	// BrowserLaunch returns the command to open a URL in the user's browser.
	// Linux: []string{"xdg-open", url}
	// macOS: []string{"open", "-a", "Safari", url}
	BrowserLaunch(url string) []string

	// ServiceName returns the process/service name for pgrep.
	ServiceName(string) string
}

// Binaries lists required FortiClient binaries on this platform.
type Binaries struct {
	Sched     string // fctsched / (N/A on macOS)
	Config    string // confighandler / fcconfig
	Tray      string // fortitray / (N/A on macOS)
	TunnelCtl string // fortivpn / fct_tunnel_ctl
	IKE       string // iked
}

// Services lists system services that must be running.
type Services struct {
	Sched     bool // fctsched
	Config    bool // confighandler
	Tray      bool // fortitray
	CTLD      bool // fctctld (macOS only)
}

// ConfigHandler is the platform-specific config handler.
type ConfigHandler interface {
	// ImportConfig imports a FortiClient XML configuration.
	ImportConfig(ctx context.Context, data []byte) error

	// ListConnections returns all configured VPN tunnels.
	ListConnections(ctx context.Context) ([]Connection, error)

	// GetConnection returns a specific VPN connection by name.
	GetConnection(ctx context.Context, name string) (*Connection, error)

	// DeleteConnection removes a VPN configuration.
	DeleteConnection(ctx context.Context, name string) error

	// GetVPNState returns the connection state for a tunnel.
	GetVPNState(ctx context.Context, name string) (int, error)
}

// Connection represents a VPN connection from the config.
type Connection struct {
	Name          string
	Type          string
	UID           string
	IKESettings   IKESettings
	Options       ConnectionOptions
}

// IKESettings holds VPN connection settings.
type IKESettings struct {
	Server            string
	Version           int
	Mode              string
	AuthMethod        string
	SSOEnabled        int
	UseExternalBrowser int
	IKEsamlPort       int
	TCPPort           int
	XAuthTimeout      int
}

// ConnectionOptions holds per-connection options.
type ConnectionOptions struct {
	Enabled         int
	UseWinCert      int
	UseSmartCardCert int
	UseLocalCert    int
}

// HasSAML returns true if SAML/SSO is enabled for this connection.
func (c *Connection) HasSAML() bool {
	return c.IKESettings.SSOEnabled == 1
}

// ShouldUseExternalBrowser returns true if this connection uses external browser SAML.
func (c *Connection) ShouldUseExternalBrowser() bool {
	return c.IKESettings.SSOEnabled == 1 && c.IKESettings.UseExternalBrowser == 1
}

// GetSAMLPort returns the SAML callback port for this connection.
func (c *Connection) GetSAMLPort() int {
	if c.IKESettings.IKEsamlPort > 0 {
		return c.IKESettings.IKEsamlPort
	}
	return 8020 // SAML default port
}

// GetAuthTimeout returns the XAuth/SAML timeout in seconds.
func (c *Connection) GetAuthTimeout() int {
	if c.IKESettings.XAuthTimeout > 0 {
		return c.IKESettings.XAuthTimeout
	}
	return 300 // Default SAML timeout
}

// ServiceManager handles starting/stopping FortiClient services.
type ServiceManager interface {
	// StartRequired starts only services that aren't already running.
	// Returns the service port to use for fctservice bridge.
	StartRequired(ctx context.Context) (string, error)

	// StopAll stops all services that we started.
	StopAll()

	// Status returns a human-readable status string.
	Status() string
}

// WSConnector connects to the fctservice WebSocket.
type WSConnector interface {
	// Connect dials the WebSocket endpoint.
	Connect(ctx context.Context) error

	// Close disconnects the WebSocket.
	Close() error

	// URL returns the WebSocket URL.
	URL() string
}
