//go:build linux

package platforms

import "fcvpn/pkg/platform/linux"

func init()  { defaultPlatform = linux.New() }
func CurrentOS() string { return "linux" }
func IsLinux() bool      { return true }
func IsDarwin() bool     { return false }
