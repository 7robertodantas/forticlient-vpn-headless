package fcvpn

import (
	"fmt"
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
	SAMLTypeUnknown          = 0
	SAMLTypeSSLVPN           = 1
	SAMLTypeIPSECVPN         = 2
	SAMLTypeZTNA             = 3
	SAMLTypeEMS              = 4
	SAMLStateUnknown         = 0
	SAMLStateSuccess         = 1
	SAMLStateTimeout         = 2
	SAMLStateCancelled       = 3
	SAMLStateClose           = 4
	SAMLStateEmptyResponse   = 5
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
	ErrNoServicePort = fmt.Errorf("service port file not found")
	ErrNoGateway     = fmt.Errorf("no gateway host reachable")
	ErrSAMLTimeout   = fmt.Errorf("SAML authentication timed out")
	ErrNoConnection  = fmt.Errorf("no VPN connection established")
)
