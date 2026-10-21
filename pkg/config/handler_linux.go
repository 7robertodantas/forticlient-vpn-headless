//go:build linux

package config

import (
	"fcvpn/pkg/platform"

	"fcvpn/pkg/config/linux"
)

func newPlatformHandler(plat platform.Platform, port string) platformConfigHandler {
	return linux.NewHandler(plat, port)
}
