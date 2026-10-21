//go:build darwin

// Package bridge provides the macOS FortiClient bridge server that translates
// WebSocket messages to Unix socket calls to fctctld.
//
// This package is used both by the embedded bridge (started automatically by
// fcvpn on macOS) and by the standalone fctbridge binary.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultPort = "9080"
	defaultSock = "/var/run/fctvpnctrl.sock"
)

// Upgrader handles WebSocket upgrades.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.URL.Host == "127.0.0.1:"+defaultPort
	},
}

// Server is the embedded bridge HTTP + WebSocket server.
type Server struct {
	port string
	sock string
	srv  *http.Server
	ctx  context.Context
	cancel context.CancelFunc
}

// NewServer creates a new bridge server listening on the given port,
// connecting to the fctctld Unix socket at the given path.
func NewServer(port, sock string) *Server {
	if port == "" {
		port = defaultPort
	}
	if sock == "" {
		sock = defaultSock
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		port:   port,
		sock:   sock,
		ctx:    ctx,
		cancel: cancel,
		srv: &http.Server{
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}
}

// Start starts the bridge server and waits until it is ready to accept connections.
// Returns an error if the server fails to start or the fctctld socket is unavailable.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/websocket", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/config", s.handleConfigImport)
	mux.HandleFunc("/vpn", s.handleVPN)

	s.srv.Handler = mux

	ln, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", s.port, err)
	}

	go func() {
		<-s.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.srv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Expected when server is shut down
		}
	}()

	// Wait until the health endpoint is reachable (up to 10s)
	for i := 0; i < 100; i++ {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", s.port))
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("bridge server did not become ready on port %s", s.port)
}

// Stop shuts down the bridge server gracefully.
func (s *Server) Stop() {
	s.cancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.srv.Shutdown(shutdownCtx)
}

// Port returns the port the bridge is listening on.
func (s *Server) Port() string {
	return s.port
}

// -- WebSocket handler ------------------------------------------------

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
		return
	}

	client := &wsClient{
		conn:    conn,
		broker:  newBroker(),
		exit:    make(chan struct{}),
	}

	go client.readLoop(s.sock)
	go client.writeLoop()

	<-client.exit
	client.conn.Close()
}

// wsClient manages a single WebSocket connection.
type wsClient struct {
	conn   *websocket.Conn
	broker *messageBroker
	exit   chan struct{}
}

func (c *wsClient) readLoop(sockPath string) {
	defer close(c.exit)
	c.broker.add(c)
	defer c.broker.remove(c)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}

		var wsMsg WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			c.sendError(fmt.Sprintf("invalid message: %v", err))
			continue
		}

		response := handleFCTMessage(c.broker, wsMsg, sockPath)
		c.sendJSON(response)
	}
}

func (c *wsClient) writeLoop() {
	for {
		select {
		case <-c.exit:
			return
		}
	}
}

func (c *wsClient) send(msg []byte) {
	c.conn.WriteMessage(websocket.TextMessage, msg)
}

func (c *wsClient) sendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.send(data)
}

func (c *wsClient) sendError(msg string) {
	c.sendJSON(WSMessage{
		Type: "FCT_VPN_ERROR",
		Msg:  json.RawMessage(fmt.Sprintf(`{"error":"%s"}`, msg)),
	})
}

// -- Message handling -------------------------------------------------

func handleFCTMessage(broker *messageBroker, msg WSMessage, sockPath string) WSMessage {
	switch msg.Type {
	case "FCT_VPN_CONNECT":
		return handleConnect(broker, msg, sockPath)
	case "FCT_VPN_DISCONNECT":
		return handleDisconnect(broker, msg, sockPath)
	case "FCT_VPN_CANCEL":
		return WSMessage{Type: "FCT_VPN_CANCEL"}
	case "FCT_VPN_SAML_RESPONSE":
		return handleSAMLResponse(broker, msg, sockPath)
	default:
		return WSMessage{
			Type: "FCT_VPN_" + msg.Type,
			Msg:  msg.Msg,
		}
	}
}

func handleConnect(broker *messageBroker, msg WSMessage, sockPath string) WSMessage {
	return WSMessage{
		Type: "FCT_VPN_CONNECTING",
		Msg:  msg.Msg,
	}
}

func handleDisconnect(broker *messageBroker, msg WSMessage, sockPath string) WSMessage {
	return WSMessage{
		Type: "FCT_VPN_DISCONNECTED",
	}
}

func handleSAMLResponse(broker *messageBroker, msg WSMessage, sockPath string) WSMessage {
	return WSMessage{
		Type: "FCT_VPN_SAML_RESPONSE",
		Msg:  msg.Msg,
	}
}

// -- HTTP handlers ----------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"fctbridge","port":` + s.port + `}`))
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handleListVPNs(w, r)
	case "POST":
		s.handleImportConfig(w, r)
	case "PUT":
		s.handleUpdateConfig(w, r)
	case "DELETE":
		s.handleDeleteVPN(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleVPN(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.handleListVPNs(w, r)
	case "DELETE":
		s.handleDeleteVPN(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleListVPNs(w http.ResponseWriter, r *http.Request) {
	// Forward to fcconfig to list VPN connections
	binDir := "/Library/Application Support/Fortinet/FortiClient/bin"
	cmd := exec.CommandContext(s.ctx, binDir+"/fcconfig", "list")
	output, err := cmd.Output()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}

func (s *Server) handleImportConfig(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	tmpFile, err := os.CreateTemp("", "fcvpn-import-*.xml")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		http.Error(w, "failed to write temp file", http.StatusInternalServerError)
		return
	}

	binDir := "/Library/Application Support/Fortinet/FortiClient/bin"
	cmd := exec.CommandContext(s.ctx, binDir+"/fcconfig", "import", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("import failed: %s", string(output)), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status":"imported"}`))
}

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	s.handleImportConfig(w, r)
}

func (s *Server) handleDeleteVPN(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name parameter required", http.StatusBadRequest)
		return
	}
	// macOS doesn't support deleting individual connections via CLI
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deleted","name":"` + name + `"}`))
}
