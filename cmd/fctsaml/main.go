package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"fcvpn"
	"fcvpn/pkg/config"
	"fcvpn/pkg/saml"
	"fcvpn/pkg/service"
	"fcvpn/pkg/ws"
	"fcvpn/pkg/xml"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	switch os.Args[1] {
	case "import":
		exitOnErr(runImport(ctx, os.Args[2:]))
	case "connect":
		exitOnErr(runConnect(ctx, os.Args[2:]))
	case "disconnect":
		exitOnErr(runDisconnect(ctx, os.Args[2:]))
	case "status":
		exitOnErr(runStatus(ctx, os.Args[2:]))
	case "list":
		exitOnErr(runList(ctx))
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

Commands:
  import <file.xml>     Import a FortiClient VPN configuration file
  connect [name]        Connect to a VPN tunnel (SAML authentication)
  disconnect [name]     Disconnect from a VPN tunnel
  status [name]         Show connection status
  list                  List configured VPN tunnels

Examples:
  fcvpn import ~/MyCorp_Corp_VPN_North_Linux_READY.xml
  fcvpn connect "MyCorp Corp VPN North"
  fcvpn list
  fcvpn status
`)
}

func runImport(ctx context.Context, args []string) error {
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

	// Import only uses confighandler's HTTP API; fortitray is not needed.
	mgr := service.NewManager()
	port, err := mgr.StartConfig(ctx)
	if err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	defer mgr.StopAll()

	h := config.NewHandler(port)

	imported := 0
	for _, conn := range conns {
		// Check if already exists
		if _, err := h.GetConnection(conn.Name); err == nil {
			fmt.Printf("  ⚠  %s: already configured, skipping\n", conn.Name)
			continue
		}

		// Import via confighandler API
		if err := h.ImportConfig(data); err != nil {
			fmt.Printf("  ✗ %s: import failed: %v (may need to restart services)\n", conn.Name, err)
			continue
		}
		imported++
		fmt.Printf("  ✓ %s: imported\n", conn.Name)
	}

	fmt.Printf("\nImported %d of %d connections\n", imported, len(conns))
	return nil
}

func runConnect(ctx context.Context, args []string) error {
	var tunnelName string
	if len(args) > 0 {
		tunnelName = args[0]
	}

	// Start required services
	mgr := service.NewManager()
	port, err := mgr.StartRequired(ctx)
	if err != nil {
		return fmt.Errorf("start services: %w", err)
	}
	defer mgr.StopAll()

	// Connect to fctservice
	client := ws.NewClient(port)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to fctservice: %w", err)
	}
	defer client.Close()

	// Find the tunnel name if not specified
	if tunnelName == "" {
		tunnelName, err = findTunnel(port)
		if err != nil {
			return fmt.Errorf("no tunnel specified and no default tunnel found: %w", err)
		}
	}

	fmt.Printf("Connecting to tunnel: %s\n", tunnelName)

	// Get connection details from confighandler
	h := config.NewHandler(port)
	conn, err := h.GetConnection(tunnelName)
	if err != nil {
		return fmt.Errorf("get connection: %w", err)
	}

	fmt.Printf("  Server:       %s\n", conn.IKESettings.Server)
	fmt.Printf("  IKE Version:  %d\n", conn.IKESettings.Version)
	fmt.Printf("  Mode:         %s\n", conn.IKESettings.Mode)
	fmt.Printf("  Auth:         %s\n", conn.IKESettings.AuthMethod)
	fmt.Println()

	// Handle SAML authentication if needed
	if conn.HasSAML() {
		fmt.Println("Starting SAML authentication...")
		fmt.Println()

		if err := startSAMLAuth(ctx, client, conn); err != nil {
			return fmt.Errorf("SAML authentication: %w", err)
		}
	}

	// Send connect request
	req := ws.ConnectRequest{
		ConnectionName: tunnelName,
		Success:        true,
	}
	if err := client.SendConnect(req); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	fmt.Println("Connection request sent. Waiting for status...")

	// Wait for connection status
	for {
		select {
		case <-ctx.Done():
			client.SendCancel()
			return ctx.Err()
		case msg := <-client.ReceiveAll(ctx):
			switch msg.Type {
			case fcvpn.FCT_VPN_CONNECTED:
				fmt.Println("\n✓ VPN connected successfully!")
				return nil
			case fcvpn.FCT_VPN_DISCONNECTED:
				fmt.Println("\n✗ VPN disconnected")
				return fcvpn.ErrNoConnection
			case fcvpn.FCT_VPN_CONNECTING:
				fmt.Println("  Connecting...")
			case fcvpn.FCT_VPN_SAML_RESPONSE:
				// SAML response processed
				fmt.Println("  SAML response received")
			case fcvpn.FCT_VPN_XAUTH:
				// XAuth challenge - not expected with SAML
				fmt.Println("  XAuth challenge received (unexpected)")
			default:
				// Unknown message, might contain useful info
				fmt.Printf("  Status: %s\n", msg.Type)
			}
		}
	}
}

func startSAMLAuth(ctx context.Context, client *ws.Client, conn *xml.Connection) error {
	// Build SAML URL
	samlPort := conn.GetSAMLPort()
	gateway := conn.IKESettings.Server

	samlServer := saml.NewServer(samlPort)

	// Start SAML callback HTTP server
	samlCtx, samlCancel := context.WithCancel(ctx)
	defer samlCancel()

	actualPort, err := samlServer.Start(samlCtx)
	if err != nil {
		return fmt.Errorf("start SAML server: %w", err)
	}
	defer samlServer.Close()

	fmt.Printf("  SAML callback server started on port %d\n", actualPort)

	// Build the SAML URL for IPsec: https://<server>:<tcp_port>/saml_login?redirect=1&redirport=<port>
	tcpPort := conn.IKESettings.TCPPort
	if tcpPort == 0 {
		tcpPort = 443
	}
	samlURL := fmt.Sprintf("https://%s:%d/saml_login?redirect=1&redirport=%d",
		gateway, tcpPort, actualPort)
	fmt.Printf("  Opening browser to: %s\n", samlURL)
	fmt.Println()

	// Open external browser
	browserCmd := exec.CommandContext(ctx, "xdg-open", samlURL)
	if err := browserCmd.Start(); err != nil {
		// Try chromium, google-chrome, firefox as fallbacks
		for _, browser := range []string{"chromium-browser", "google-chrome", "chromium", "firefox"} {
			browserCmd = exec.CommandContext(ctx, browser, samlURL)
			if err := browserCmd.Start(); err == nil {
				break
			}
		}
		if browserCmd.Process == nil {
			return fmt.Errorf("no browser found (tried: xdg-open, chromium-browser, google-chrome, chromium, firefox)")
		}
	}

	// Wait for SAML callback
	fmt.Printf("  Waiting for SAML authentication (timeout: %ds)...\n", conn.GetAuthTimeout())
	result, err := samlServer.Wait(ctx)
	if err != nil {
		return fmt.Errorf("SAML wait: %w", err)
	}

	if result.State != fcvpn.SAMLStateSuccess {
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

func runDisconnect(ctx context.Context, args []string) error {
	tunnelName := ""
	if len(args) > 0 {
		tunnelName = args[0]
	}

	port, err := fcvpn.PortReader{}.Read()
	if err != nil {
		return err
	}

	client := ws.NewClient(port)
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("connect to fctservice: %w", err)
	}
	defer client.Close()

	if tunnelName != "" {
		fmt.Printf("Disconnecting tunnel: %s\n", tunnelName)
	}

	if err := client.SendDisconnect(); err != nil {
		return fmt.Errorf("send disconnect: %w", err)
	}

	fmt.Println("Disconnect request sent")
	return nil
}

func runStatus(ctx context.Context, args []string) error {
	port, err := fcvpn.PortReader{}.Read()
	if err != nil {
		return err
	}

	h := config.NewHandler(port)
	conns, err := h.ListConfigs()
	if err != nil {
		// Service might not be running, that's OK for status check
	}

	// Show service status
	mgr := service.NewManager()

	fmt.Println("FortiClient VPN Status")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println(mgr.Status())

	if len(conns) > 0 {
		fmt.Printf("\nConfigured tunnels: %d\n", len(conns))
	}

	return nil
}

func runList(ctx context.Context) error {
	port, err := fcvpn.PortReader{}.Read()
	if err != nil {
		return err
	}

	h := config.NewHandler(port)
	conns, err := h.ListConfigs()
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
		extBrowser := ""
		if c.ShouldUseExternalBrowser() {
			extBrowser = " (external browser)"
		}
		fmt.Printf("  %-30s %s%s%s\n", c.Name, c.IKESettings.Server, saml, extBrowser)
	}

	return nil
}

func findTunnel(port string) (string, error) {
	h := config.NewHandler(port)
	conns, err := h.ListConfigs()
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

	// Otherwise return the first one
	return conns[0].Name, nil
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
