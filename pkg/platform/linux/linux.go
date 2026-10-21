//go:build linux

// Package linux provides platform-specific configuration for FortiClient VPN on Linux.
package linux

import (
	"fcvpn/pkg/platform"
)

// platform implements the platform.Platform interface for Linux.
type platformImpl struct {
	binDir  string
	binaries platform.Binaries
	services platform.Services
}

// New creates a new Linux platform configuration.
func New() platform.Platform {
	return &platformImpl{
		binDir: "/opt/forticlient",
		binaries: platform.Binaries{
			Sched:     "/opt/forticlient/fctsched",
			Config:    "/opt/forticlient/confighandler",
			Tray:      "/opt/forticlient/fortitray",
			TunnelCtl: "/opt/forticlient/fortivpn",
			IKE:       "/opt/forticlient/iked",
		},
		services: platform.Services{
			Sched:  true,
			Config: true,
			Tray:   true,
			CTLD:   false,
		},
	}
}

func (p *platformImpl) BinicDir() string  { return p.binDir }
func (p *platformImpl) ServicePortPath() string {
	return "/var/run/forticlient/service_port"
}
func (p *platformImpl) BridgePort() string { return "" } // Uses dynamic port from file
func (p *platformImpl) Binaries() platform.Binaries {
	return p.binaries
}
func (p *platformImpl) Services() platform.Services {
	return p.services
}

func (p *platformImpl) BrowserLaunch(url string) []string {
	return []string{"xdg-open", url}
}

func (p *platformImpl) ServiceName(name string) string {
	switch name {
	case "fctsched":
		return "fctsched"
	case "confighandler":
		return "confighandler"
	case "fortitray":
		return "fortitray"
	default:
		return name
	}
}
