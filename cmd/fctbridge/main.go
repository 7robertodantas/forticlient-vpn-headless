// Command fctbridge starts the macOS bridge service as a standalone binary.
//
// On macOS, fcvpn automatically starts the bridge internally, so this binary
// is only needed for backward compatibility or if you want the bridge running
// as a separate long-lived process.
//
// Usage:
//   fctbridge                          # Start bridge on default port (9080)
//   fctbridge --port 9080              # Start bridge on custom port
//   fctbridge --socket /var/run/fctvpnctrl.sock
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"fcvpn/pkg/bridge"
)

var (
	port   = flag.String("port", "9080", "Bridge listen port")
	socket = flag.String("socket", "/var/run/fctvpnctrl.sock", "fctctld Unix socket path")
)

func main() {
	flag.Parse()

	fmt.Fprintf(os.Stderr, "fctbridge - macOS FortiClient bridge service\n")
	fmt.Fprintf(os.Stderr, "  Listening on: %s\n", *port)
	fmt.Fprintf(os.Stderr, "  fctctld sock: %s\n", *socket)

	// Check if fctctld is available
	if _, err := os.Stat(*socket); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: fctctld socket not found: %s\n", *socket)
		fmt.Fprintf(os.Stderr, "Ensure FortiClient VPN is installed and running.\n")
		os.Exit(1)
	}

	srv := bridge.NewServer(*port, *socket)
	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Bridge service started on port %s\n", *port)

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "Shutting down bridge service...\n")
	srv.Stop()
	fmt.Fprintf(os.Stderr, "Bridge service stopped\n")
}
