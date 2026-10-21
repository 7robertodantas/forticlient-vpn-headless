package fcctl

import (
	"fmt"
	"os"
	"strings"
)

// Constants
const (
	// Service port file on Linux
	ServicePortPath = "/var/run/forticlient/service_port"

	// WebSocket endpoint
	WSURL = "ws://127.0.0.1:PORT/websocket"

	// SAML default ports
	SAMLDefaultSSOPort = 8020
	SAMLDefaultTimeout = 300 // seconds

	// FortiClient binary paths
	BinFortitray  = "/opt/forticlient/fortitray"
	BinFctsched   = "/opt/forticlient/fctsched"
	BinFortivpn   = "/opt/forticlient/fortivpn"
	BinIked       = "/opt/forticlient/iked"
	BinConfigHand = "/opt/forticlient/confighandler"

	// IPC paths
	IPCBase     = "/var/run/forticlient"
	NSSDBPath   = "/home/%s/.pki/nssdb"
	KeytarStore = "FortiClientLinuxVPN"
)

// SAML event types (from Electron app source)
const (
	FCT_VPN_CONNECT              = "FCT_VPN_CONNECT"
	FCT_VPN_DISCONNECT           = "FCT_VPN_DISCONNECT"
	FCT_VPN_SAML_RESPONSE        = "FCT_VPN_SAML_RESPONSE"
	FCT_VPN_XAUTH                = "FCT_VPN_XAUTH"
	FCT_VPN_CANCEL               = "FCT_VPN_CANCEL"
	FCT_VPN_CONFIRMATION         = "FCT_VPN_CONFIRMATION"
	FCT_VPN_CONNECTED            = "FCT_VPN_CONNECTED"
	FCT_VPN_DISCONNECTED         = "FCT_VPN_DISCONNECTED"
	FCT_VPN_CONNECTING           = "FCT_VPN_CONNECTING"
	FCT_VPN_RECONNECTING         = "FCT_VPN_RECONNECTING"
	FCT_VPN_XAUTH_TOKEN          = "FCT_VPN_XAUTH_TOKEN"
	FCT_VPN_XAUTH_RSA            = "FCT_VPN_XAUTH_RSA"
	FCT_VPN_XAUTH_NEW_PWD        = "FCT_VPN_XAUTH_NEW_PWD"
	FCT_VPN_XAUTH_CREDENTIALS    = "FCT_VPN_XAUTH_CREDENTIALS"
	FCT_VPN_INVALID_CERTIFICATE  = "FCT_VPN_INVALID_CERTIFICATE"
	FCT_VPN_LIMIT_USER           = "FCT_VPN_LIMIT_USER"
	FCT_VPN_DISCLAIMER           = "FCT_VPN_DISCLAIMER"
	FCT_VPN_XAUTH_PROXY          = "vpn-xauth-proxy"
	FCT_VPN_SAML_LOGIN_RESPONSE  = "vpn-saml-login-resp"
	FCT_VPN_SAML_LOGIN_CANCELLED = "vpn-saml-login-cancelled"
	FCT_VPN_SAML_LOGIN_ERROR     = "vpn-saml-login-error"
)

// SAML types
const (
	SAMLTypeUnknown  = 0
	SAMLTypeSSLVPN   = 1
	SAMLTypeIPSECVPN = 2
	SAMLTypeZTNA     = 3
	SAMLTypeEMS      = 4
)

// SAML states
const (
	SAMLStateUnknown           = 0
	SAMLStateSuccess           = 1
	SAMLStateTimeout           = 2
	SAMLStateCancelled         = 3
	SAMLStateClose             = 4
	SAMLStateEmptyResponse     = 5
	SAMLStateRedirectUnavailable = 6
)

// VPN states
const (
	VPNStateDisconnected = 0
	VPNStateConnecting   = 1
	VPNStateConnected    = 2
	VPNStateUnknown      = 3
	VPNStateReconnecting = 4
	VPNStateXAuth        = 5
)

// Error types
var (
	ErrNoServicePort = fmt.Errorf("service port file not found: %s", ServicePortPath)
	ErrNoGateway     = fmt.Errorf("no gateway host reachable")
	ErrSAMLTimeout   = fmt.Errorf("SAML authentication timed out")
	ErrNoConnection  = fmt.Errorf("no VPN connection established")
)

// PortReader reads the fctservice port from the service port file.
type PortReader struct {
	Port string
}

func (pr PortReader) Read() (string, error) {
	data, err := os.ReadFile(ServicePortPath)
	if err != nil {
		return "", ErrNoServicePort
	}
	pr.Port = strings.TrimSpace(string(data))
	if pr.Port == "" {
		return "", ErrNoServicePort
	}
	return pr.Port, nil
}

// GetWSURL returns the WebSocket URL for the given port.
func (pr PortReader) GetWSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%s/websocket", pr.Port)
}

// GetHTTPURL returns the HTTP URL for the given port.
func (pr PortReader) GetHTTPURL() string {
	return fmt.Sprintf("http://127.0.0.1:%s", pr.Port)
}
