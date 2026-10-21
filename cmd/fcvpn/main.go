// Command fcvpn is the headless FortiClient VPN CLI.
//
// This CLI provides a headless interface for FortiClient VPN tunnels
// with SAML authentication support. It works on both Linux and macOS:
//
//   - Linux: Wraps FortiClient's fctservice HTTP API
//   - macOS: Embeds the fctbridge service internally
//
// Usage:
//   fcvpn import <file.xml>    # Import a FortiClient VPN configuration
//   fcvpn connect [name]       # Connect to a VPN tunnel (SAML authentication)
//   fcvpn disconnect [name]    # Disconnect from a VPN tunnel
//   fcvpn status               # Show connection status
//   fcvpn list                 # List configured VPN tunnels
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"fcvpn/pkg/config"
	"fcvpn/pkg/platform"
	"fcvpn/pkg/service"
	"fcvpn/pkg/saml"
	"fcvpn/pkg/ws"
	"fcvpn/pkg/xml"
	"fcvpn/platforms"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	plat := platforms.Default()

	switch os.Args[1] {
	case "import":
		exitOnErr(runImport(ctx, os.Args[2:], plat))
	case "connect":
		exitOnErr(runConnect(ctx, os.Args[2:], plat))
	case "disconnect":
		exitOnErr(runDisconnect(ctx, os.Args[2:], plat))
	case "status":
		exitOnErr(runStatus(ctx, plat))
	case "list":
		exitOnErr(runList(ctx, plat))
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: fcvpn <command> [arguments]

Headless FortiClient VPN CLI with SAML authentication.

Platform: %s

Commands:
  import <file.xml>     Import a FortiClient VPN configuration file
  connect [name]        Connect to a VPN tunnel (SAML authentication)
  disconnect [name]     Disconnect from a VPN tunnel
  status                Show connection status
  list                  List configured VPN tunnels

Examples:
  fcvpn import ~/MyCorp_Corp_VPN_North.xml
  fcvpn connect "MyCorp Corp VPN"
  fcvpn list
  fcvpn status
`, platforms.CurrentOS())
}

func runImport(ctx context.Context, args []string, plat platform.Platform) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: fcvpn import <file.xml>")
	}
	file := args[0]

	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	cfg, err := xml.ParseConfig(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	conns := cfg.GetAllConnections()
	if len(conns) == 0 {
		return fmt.Errorf("no VPN connections found in config")
	}

	// Start required services and get the service port
	mgr := service.NewManager(plat)
	port, err := mgr.StartRequired(ctx)
	if err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	defer mgr.StopAll()

	// Create platform-specific config handler
	h := config.NewHandler(plat, port)

	imported := 0
	for _, conn := range conns {
		// Check if already exists
		if _, err := h.GetConnection(ctx, conn.Name); err == nil {
			fmt.Printf("  ⚠  %s: already configured, skipping\n", conn.Name)
			continue
		}

		// Import via platform-specific handler
		if err := h.ImportConfig(ctx, data); err != nil {
			fmt.Printf("  ✗ %s: import failed: %v\n", conn.Name, err)
			continue
		}
		imported++
		fmt.Printf("  ✓ %s: imported\n", conn.Name)
	}

	fmt.Printf("\nImported %d of %d connections\n", imported, len(conns))
	return nil
}

func runConnect(ctx context.Context, args []string, plat platform.Platform) error {
	var tunnelName string
	if len(args) > 0 {
		tunnelName = args[0]
	}

	// Start required services and get the service port
	mgr := service.NewManager(plat)
	port, err := mgr.StartRequired(ctx)
	if err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	defer mgr.StopAll()

	// Connect to fctservice via WebSocket
	wsClient := ws.NewClient(port)
	if err := wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("connect to fctservice: %w", err)
	}
	defer wsClient.Close()

	// Find the tunnel name if not specified
	if tunnelName == "" {
		tunnelName, err = findTunnel(plat, port)
		if err != nil {
			return fmt.Errorf("no tunnel specified and no default tunnel found: %w", err)
		}
	}

	// Get connection details from config handler
	h := config.NewHandler(plat, port)
	conn, err := h.GetConnection(ctx, tunnelName)
	if err != nil {
		return fmt.Errorf("get connection: %w", err)
	}

	fmt.Printf("Connecting to tunnel: %s\n", tunnelName)
	fmt.Printf("  Server:       %s\n", conn.IKESettings.Server)
	fmt.Printf("  IKE Version:  %d\n", conn.IKESettings.Version)
	fmt.Printf("  Mode:         %s\n", conn.IKESettings.Mode)
	fmt.Printf("  Auth:         %s\n", conn.IKESettings.AuthMethod)
	fmt.Println()

	// Handle SAML authentication if needed
	if conn.HasSAML() {
		fmt.Println("Starting SAML authentication...")
		fmt.Println()

		if err := startSAMLAuth(ctx, wsClient, conn, plat); err != nil {
			return fmt.Errorf("SAML authentication: %w", err)
		}
	}

	// Send connect request
	req := ws.ConnectRequest{
		ConnectionName: tunnelName,
		Success:        true,
	}
	if err := wsClient.SendConnect(req); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	fmt.Println("Connection request sent. Waiting for status...")

	// Wait for connection status
	for {
		select {
		case <-ctx.Done():
			wsClient.SendCancel()
			return ctx.Err()
		case msg := <-wsClient.ReceiveAll(ctx):
			switch msg.Type {
			case "FCT_VPN_CONNECTED":
				fmt.Println("\n✓ VPN connected successfully!")
				return nil
			case "FCT_VPN_DISCONNECTED":
				fmt.Println("\n✗ VPN disconnected")
				return fmt.Errorf("no VPN connection established")
			case "FCT_VPN_CONNECTING":
				fmt.Println("  Connecting...")
			case "FCT_VPN_SAML_RESPONSE":
				fmt.Println("  SAML response received")
			case "FCT_VPN_XAUTH":
				fmt.Println("  XAuth challenge received (unexpected)")
			default:
				fmt.Printf("  Status: %s\n", msg.Type)
			}
		}
	}
}

func startSAMLAuth(ctx context.Context, client *ws.Client, conn *platform.Connection, plat platform.Platform) error {
	samlPort := conn.GetSAMLPort()

	samlServer := saml.NewServer(samlPort)

	samlCtx, samlCancel := context.WithCancel(ctx)
	defer samlCancel()

	actualPort, err := samlServer.Start(samlCtx)
	if err != nil {
		return fmt.Errorf("start SAML server: %w", err)
	}
	defer samlServer.Close()

	fmt.Printf("  SAML callback server started on port %d\n", actualPort)

	// Build the SAML URL for IPsec
	tcpPort := conn.IKESettings.TCPPort
	if tcpPort == 0 {
		tcpPort = 443
	}
	samlURL := fmt.Sprintf("https://%s:%d/saml_login?redirect=1&redirport=%d",
		conn.IKESettings.Server, tcpPort, actualPort)
	fmt.Printf("  Opening browser to: %s\n", samlURL)
	fmt.Println()

	// Open external browser using platform-specific command
	browserArgs := plat.BrowserLaunch(samlURL)
	browserCmd := exec.CommandContext(ctx, browserArgs[0], browserArgs[1:]...)
	if err := browserCmd.Start(); err != nil {
		return fmt.Errorf("no browser found")
	}

	// Wait for SAML callback
	fmt.Printf("  Waiting for SAML authentication (timeout: %ds)...\n", conn.GetAuthTimeout())
	result, err := samlServer.Wait(ctx)
	if err != nil {
		return fmt.Errorf("SAML wait: %w", err)
	}

	if result.State != 1 { // SUCCESS
		return fmt.Errorf("SAML authentication failed (state: %d)", result.State)
	}

	fmt.Println("  ✓ SAML authentication successful!")
	fmt.Println()

	// Send SAML response to fctservice
	if err := client.SendSAMLResponse(true); err != nil {
		return fmt.Errorf("send SAML response: %w", err)
	}

	return nil
}

func runDisconnect(ctx context.Context, args []string, plat platform.Platform) error {
	tunnelName := ""
	if len(args) > 0 {
		tunnelName = args[0]
	}

	mgr := service.NewManager(plat)
	port, err := mgr.StartRequired(ctx)
	if err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	defer mgr.StopAll()

	wsClient := ws.NewClient(port)
	if err := wsClient.Connect(ctx); err != nil {
		return fmt.Errorf("connect to fctservice: %w", err)
	}
	defer wsClient.Close()

	if tunnelName != "" {
		fmt.Printf("Disconnecting tunnel: %s\n", tunnelName)
	}

	if err := wsClient.SendDisconnect(); err != nil {
		return fmt.Errorf("send disconnect: %w", err)
	}

	fmt.Println("Disconnect request sent")
	return nil
}

func runStatus(ctx context.Context, plat platform.Platform) error {
	mgr := service.NewManager(plat)

	fmt.Println("FortiClient VPN Status")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println(mgr.Status())

	// Try to list configs if the service is running
	port, err := service.GetServicePort(plat)
	if err == nil {
		h := config.NewHandler(plat, port)
		conns, err := h.ListConnections(ctx)
		if err == nil && len(conns) > 0 {
			fmt.Printf("\nConfigured tunnels: %d\n", len(conns))
		}
	}

	return nil
}

func runList(ctx context.Context, plat platform.Platform) error {
	port, err := service.GetServicePort(plat)
	if err != nil {
		fmt.Printf("Service not running: %v\n", err)
		fmt.Println("Import a config with: fcvpn import <file.xml>")
		return nil
	}

	h := config.NewHandler(plat, port)
	conns, err := h.ListConnections(ctx)
	if err != nil {
		return fmt.Errorf("list configs: %w", err)
	}

	if len(conns) == 0 {
		fmt.Println("No VPN configurations found.")
		fmt.Println("Import one with: fcvpn import <file.xml>")
		return nil
	}

	fmt.Printf("Configured VPN tunnels (%d):\n", len(conns))
	for _, c := range conns {
		saml := ""
		if c.HasSAML() {
			saml = " [SAML]"
		}
		fmt.Printf("  %-30s %s%s\n", c.Name, c.IKESettings.Server, saml)
	}

	return nil
}

func findTunnel(plat platform.Platform, port string) (string, error) {
	h := config.NewHandler(plat, port)
	conns, err := h.ListConnections(context.Background())
	if err != nil {
		return "", err
	}

	if len(conns) == 0 {
		return "", fmt.Errorf("no tunnels configured")
	}
	if len(conns) == 1 {
		return conns[0].Name, nil
	}

	// Return the first one that has SAML enabled (primary tunnel)
	for _, c := range conns {
		if c.HasSAML() {
			return c.Name, nil
		}
	}

	return conns[0].Name, nil
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
