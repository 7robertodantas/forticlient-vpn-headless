// Package platforms provides platform detection and the default platform instance.
//
// This package is kept separate from pkg/platform to avoid import cycles.
// Platform-specific initialization and functions are handled via build tags:
//
//   - init_linux.go   (Linux)
//   - init_darwin.go  (macOS)
package platforms

import "fcvpn/pkg/platform"

var defaultPlatform platform.Platform

// Default returns the default platform for the current OS.
func Default() platform.Platform { return defaultPlatform }
