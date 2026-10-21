//go:build linux

package service

import (
	"fcvpn/pkg/platform"

	"fcvpn/pkg/service/linux"
)

func newPlatformManager(plat platform.Platform) platformServiceManager {
	return linux.NewManager(plat)
}
