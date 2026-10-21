//go:build darwin

// Package darwin provides platform-specific configuration for FortiClient VPN on macOS.
package darwin

import (
	"fcvpn/pkg/platform"
)

const (
	// FortiClient base directory on macOS.
	FortiClientBin = "/Library/Application Support/Fortinet/FortiClient/bin"

	// fctctld control socket (used by the bridge service).
	FctvCtrlSocket = "/var/run/fctvpnctrl.sock"

	// Default bridge port for macOS — the bridge listens here.
	DefaultBridgePort = "9080"

	// fct_tunnel_ctl handles VPN tunnel control on macOS.
	FctTunnelCtl = FortiClientBin + "/fct_tunnel_ctl"
)

// platformImpl implements the platform.Platform interface for macOS.
type platformImpl struct{}

// New creates a new macOS platform configuration.
func New() platform.Platform {
	return &platformImpl{}
}

func (p *platformImpl) BinicDir() string {
	return FortiClientBin
}

func (p *platformImpl) ServicePortPath() string {
	return "" // macOS uses a fixed bridge port, not a dynamic port file
}

func (p *platformImpl) BridgePort() string {
	return DefaultBridgePort
}

func (p *platformImpl) Binaries() platform.Binaries {
	return platform.Binaries{
		Sched:     "", // macOS: LaunchDaemon handles scheduling
		Config:    FortiClientBin + "/fcconfig",
		Tray:      "", // macOS: LaunchDaemon handles service (fctctld)
		TunnelCtl: FctTunnelCtl,
		IKE:       FortiClientBin + "/iked",
	}
}

func (p *platformImpl) Services() platform.Services {
	return platform.Services{
		Sched:  false, // macOS: LaunchDaemon
		Config: false, // macOS: LaunchDaemon (fctservctl2)
		Tray:   false, // macOS: fctctld runs as LaunchDaemon
		CTLD:   true,  // macOS: fctctld is the main control daemon
	}
}

func (p *platformImpl) BrowserLaunch(url string) []string {
	return []string{"open", "-a", "Safari", url}
}

func (p *platformImpl) ServiceName(name string) string {
	switch name {
	case "fctctld":
		return "fctctld"
	case "fcconfig":
		return "fcconfig"
	default:
		return name
	}
}
