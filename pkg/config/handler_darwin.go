//go:build darwin

package config

import (
	"fcvpn/pkg/platform"

	"fcvpn/pkg/config/darwin"
)

func newPlatformHandler(plat platform.Platform, port string) platformConfigHandler {
	return darwin.NewHandler(plat, port)
}
