// Package config provides VPN configuration management via platform-specific handlers.
//
// The shared handler (handler.go) is platform-agnostic and delegates to
// platform-specific implementations loaded via build tags:
//
//   - pkg/config/linux/handler.go   (Linux)
//   - pkg/config/darwin/handler.go  (macOS)
package config

import (
	"context"

	"fcvpn/pkg/platform"
)

// Handler manages VPN configurations.
// Built with the appropriate platform-specific handler via build tags.
type Handler struct {
	handler platformConfigHandler
}

// platformConfigHandler is the internal config handler interface.
type platformConfigHandler interface {
	ImportConfig(ctx context.Context, data []byte) error
	ListConnections(ctx context.Context) ([]platform.Connection, error)
	GetConnection(ctx context.Context, name string) (*platform.Connection, error)
	DeleteConnection(ctx context.Context, name string) error
	GetVPNState(ctx context.Context, name string) (int, error)
}

// NewHandler creates a platform-specific config handler for the given port.
// On Linux, port is the fctservice HTTP port. On macOS, port is the bridge port.
func NewHandler(plat platform.Platform, port string) *Handler {
	return &Handler{handler: newPlatformHandler(plat, port)}
}

func (h *Handler) ImportConfig(ctx context.Context, data []byte) error {
	return h.handler.ImportConfig(ctx, data)
}

func (h *Handler) ListConnections(ctx context.Context) ([]platform.Connection, error) {
	return h.handler.ListConnections(ctx)
}

func (h *Handler) GetConnection(ctx context.Context, name string) (*platform.Connection, error) {
	return h.handler.GetConnection(ctx, name)
}

func (h *Handler) DeleteConnection(ctx context.Context, name string) error {
	return h.handler.DeleteConnection(ctx, name)
}

func (h *Handler) GetVPNState(ctx context.Context, name string) (int, error) {
	return h.handler.GetVPNState(ctx, name)
}
