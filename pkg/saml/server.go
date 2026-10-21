package saml

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// CallbackResult holds the result of a SAML callback.
type CallbackResult struct {
	AuthID string
	State  int // SAML state constant
}

// Server handles the local HTTP callback for SAML authentication.
type Server struct {
	Port      int
	AuthPath  string       // e.g., /remote/saml/auth_id
	ResultCh  chan CallbackResult
	httpSrv   *http.Server
	listener  net.Listener
}

// NewServer creates a new SAML callback server.
func NewServer(port int) *Server {
	return &Server{
		Port:     port,
		AuthPath: "/remote/saml/auth_id",
		ResultCh: make(chan CallbackResult, 1),
	}
}

// Start starts the HTTP server and returns the actual bound port.
func (s *Server) Start(ctx context.Context) (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc(s.AuthPath, s.handleAuthID)

	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", s.Port))
	if err != nil {
		return 0, fmt.Errorf("SAML server listen: %w", err)
	}

	actualPort := s.listener.Addr().(*net.TCPAddr).Port
	s.Port = actualPort

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpSrv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := s.httpSrv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			// Expected when server is shut down
		}
	}()

	return actualPort, nil
}

// Wait blocks until a SAML callback is received or the context is cancelled.
func (s *Server) Wait(ctx context.Context) (*CallbackResult, error) {
	select {
	case result := <-s.ResultCh:
		return &result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("SAML callback cancelled: %w", ctx.Err())
	}
}

// Close stops the HTTP server.
func (s *Server) Close() error {
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(ctx)
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// handleAuthID processes the FortiGate redirect with auth_id parameter.
func (s *Server) handleAuthID(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	authID := query.Get("id")

	if authID == "" {
		// No auth_id in request
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<html><body><h1>SAML Error</h1><p>No auth ID received.</p></body></html>`))
		s.ResultCh <- CallbackResult{
			AuthID: "",
			State:  6, // REDIRECT_UNAVAILABLE
		}
		return
	}

	// Return success page (same as Electron's vpn_message.html)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<html>
<head><title>VPN Connected</title></head>
<body style="background:#2e2c29;color:#fff;display:flex;justify-content:center;align-items:center;height:100vh;margin:0;">
<div style="text-align:center;">
<h1>VPN Connected</h1>
<p>You may close the browser now.</p>
</div>
</body></html>`))

	s.ResultCh <- CallbackResult{
		AuthID: authID,
		State:  1, // SUCCESS
	}
}

// BuildSAMLURL constructs the FortiGate SAML authentication URL.
func BuildSAMLURL(gateway, server, port, samlPath string, redirPort int) string {
	// Clean up the gateway/server address
	server = strings.TrimPrefix(server, "https://")
	server = strings.TrimPrefix(server, "http://")

	// Build the base SAML URL
	url := fmt.Sprintf("https://%s%s", server, samlPath)

	// Add redirect parameters for external browser mode
	if redirPort > 0 {
		// For IPsec: /saml_login?
		// For SSL: /remote/saml/start
		base := strings.Split(url, "?")[0]
		url = fmt.Sprintf("%s?redirect=1&redirport=%d", base, redirPort)
	}

	return url
}

// BuildIPsacSAMLURL constructs the IPsec SAML URL: https://<server>:<port>/saml_login?
func BuildIPsecSAMLURL(server string, port int, redirPort int) string {
	url := fmt.Sprintf("https://%s:%d/saml_login?", server, port)
	if redirPort > 0 {
		url += fmt.Sprintf("redirect=1&redirport=%d", redirPort)
	}
	return url
}

// BuildSSLVPNAuthURL constructs the SSL VPN redirect URL.
func BuildSSLVPNAuthURL(gateway, server, port string) string {
	server = strings.TrimPrefix(server, "https://")
	server = strings.TrimPrefix(server, "http://")
	return fmt.Sprintf("https://%s/remote/saml/auth_id", server)
}
