//go:build darwin

package platforms

import "fcvpn/pkg/platform/darwin"

func init()  { defaultPlatform = darwin.New() }
func CurrentOS() string { return "darwin" }
func IsLinux() bool      { return false }
func IsDarwin() bool     { return true }
