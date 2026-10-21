//go:build darwin

package service

import (
	"fcvpn/pkg/platform"

	"fcvpn/pkg/service/darwin"
)

func newPlatformManager(plat platform.Platform) platformServiceManager {
	return darwin.NewManager(plat)
}
