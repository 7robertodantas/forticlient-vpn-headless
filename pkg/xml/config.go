package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)
type Config struct {
	XMLName             xml.Name     `xml:"forticlient_configuration"`
	PartialConfig       int          `xml:"partial_configuration"`
	VPN                 VPNSection   `xml:"vpn"`
}

type VPNSection struct {
	SSLVPN   SSLVPNSection    `xml:"sslvpn"`
	IPSecVPN IPSecVPNSection  `xml:"ipsecvpn"`
}

type SSLVPNSection struct {
	Connections []Connection `xml:"connections>connection"`
}

type IPSecVPNSection struct {
	Options     IPSecOptions     `xml:"options"`
	Connections []Connection     `xml:"connections>connection"`
}

type IPSecOptions struct {
	Enabled                  int    `xml:"enabled"`
	BlockIPv6                int    `xml:"block_ipv6"`
	MTUSize                  int    `xml:"mtu_size"`
	DisallowedInvalidServer  int    `xml:"disallow_invalid_server_certificate"`
	PreferIPSecDNS           int    `xml:"prefer_ipsecvpn_dns"`
}

type Connection struct {
	Name               string        `xml:"name"`
	Type               string        `xml:"type"`
	UID                string        `xml:"uid"`
	KeepRunning        int           `xml:"keep_running"`
	SingleUserMode     int           `xml:"single_user_mode"`
	DisclamerMsg       string        `xml:"disclaimer_msg"`
	DNSPriority        int           `xml:"dns_priority"`
	WarnInvalidCert    int           `xml:"warn_invalid_server_certificate"`
	Pinned             int           `xml:"pinned"`
	AllowConcurrent    int           `xml:"allow_concurrent"`
	Options            ConnectionOptions `xml:"options"`
	IKESettings        IKESettings   `xml:"ike_settings"`
	IPSecSettings      IPSecSettings `xml:"ipsec_settings"`
}

type ConnectionOptions struct {
	Enabled                int `xml:"enabled"`
	UseWinCert             int `xml:"usewincert"`
	UseSmartCardCert       int `xml:"usesmcardcert"`
	UseLocalCert           int `xml:"uselocalcert"`
}

type IKESettings struct {
	Version                int            `xml:"version"`
	AuthMethod             string         `xml:"authentication_method"`
	TransportMode          int            `xml:"transport_mode"`
	Mode                   string         `xml:"mode"`
	EAPMethod              int            `xml:"eap_method"`
	SSOEnabled             int            `xml:"sso_enabled"`
	UseExternalBrowser     int            `xml:"use_external_browser"`
	FGT                    int            `xml:"fgt"`
	NATTraversal           int            `xml:"nat_traversal"`
	DPD                    int            `xml:"dpd"`
	ModeConfig             int            `xml:"mode_config"`
	SessionResume          int            `xml:"session_resume"`
	EnableIKEFragmentation int            `xml:"enable_ike_fragmentation"`
	Server                 string         `xml:"server"`
	LocalID                string         `xml:"localid"`
	XAuthTimeout           int            `xml:"xauth_timeout"`
	KeyLife                int            `xml:"key_life"`
	DPDRetryCount          int            `xml:"dpd_retry_count"`
	DPDRetryInterval       int            `xml:"dpd_retry_interval"`
	UDPPort                int            `xml:"udp_port"`
	TCPPort                int            `xml:"tcp_port"`
	IKEsamlPort            int            `xml:"ike_saml_port"`
	DHGroup                int            `xml:"dhgroup"`
	NetworkID              int            `xml:"networkid"`
	NATAliveFreq           int            `xml:"nat_alive_freq"`
	XAuth                  XAuth          `xml:"xauth"`
	AuthData               AuthData       `xml:"auth_data"`
	Proposals              []string       `xml:"proposals>proposal"`
}

type XAuth struct {
	Enabled       int    `xml:"enabled"`
	UseOTP        int    `xml:"use_otp"`
	PromptUsername int   `xml:"prompt_username"`
	Username      string `xml:"username"`
	Password      string `xml:"password"`
}

type AuthData struct {
	PresharedKey string `xml:"preshared_key"`
}

type IPSecSettings struct {
	ReplayDetection int       `xml:"replay_detection"`
	PFS             int       `xml:"pfs"`
	UseVIP          int       `xml:"use_vip"`
	KeyLifeSeconds  int       `xml:"key_life_seconds"`
	KeyLifeKbytes   int       `xml:"key_life_Kbytes"`
	DHGroup         int       `xml:"dhgroup"`
	KeyLifeType     string    `xml:"key_life_type"`
	VirtualIP       VirtualIP `xml:"virtualip"`
	RemoteNetworks  []Network `xml:"remote_networks>network"`
	Proposals       []string  `xml:"proposals>proposal"`
}

type VirtualIP struct {
	Type     string `xml:"type"`
	IP       string `xml:"ip"`
	Mask     string `xml:"mask"`
	DNSServer string `xml:"dnsserver"`
}

type Network struct {
	Addr   string `xml:"addr"`
	Mask   string `xml:"mask"`
}

// ParseConfig reads and parses a FortiClient XML config file.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse XML config: %w", err)
	}
	return &cfg, nil
}

// ParseConfigFile reads and parses a FortiClient XML config file from disk.
func ParseConfigFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}
	return ParseConfig(data)
}

// Marshal serializes a Config to XML bytes.
func Marshal(cfg *Config) ([]byte, error) {
	return xml.MarshalIndent(cfg, "", "  ")
}

// GetVPNConnection returns the first matching VPN connection by name.
func (c *Config) GetVPNConnection(name string) *Connection {
	// Check IPsec connections
	for i := range c.VPN.IPSecVPN.Connections {
		if c.VPN.IPSecVPN.Connections[i].Name == name {
			return &c.VPN.IPSecVPN.Connections[i]
		}
	}
	// Check SSL VPN connections
	for i := range c.VPN.SSLVPN.Connections {
		if c.VPN.SSLVPN.Connections[i].Name == name {
			return &c.VPN.SSLVPN.Connections[i]
		}
	}
	return nil
}

// GetAllConnections returns all VPN connections (IPsec + SSL).
func (c *Config) GetAllConnections() []Connection {
	var all []Connection
	all = append(all, c.VPN.IPSecVPN.Connections...)
	all = append(all, c.VPN.SSLVPN.Connections...)
	return all
}

// VPNType returns the VPN type string for a connection.
func (c *Connection) VPNType() string {
	if c.IKESettings.Server != "" {
		return "ipsec"
	}
	return "ssl"
}

// Gateway returns the server/gateway host.
func (c *Connection) Gateway() string {
	// Strip protocol prefix if present
	server := c.IKESettings.Server
	server = strings.TrimPrefix(server, "https://")
	server = strings.TrimPrefix(server, "http://")
	return server
}

// ShouldUseExternalBrowser returns true if this connection uses external browser SAML.
func (c *Connection) ShouldUseExternalBrowser() bool {
	return c.IKESettings.SSOEnabled == 1 && c.IKESettings.UseExternalBrowser == 1
}

// GetSAMLPort returns the SAML callback port for this connection.
func (c *Connection) GetSAMLPort() int {
	if c.IKESettings.IKEsamlPort > 0 {
		return c.IKESettings.IKEsamlPort
	}
	return 8020 // SAML default port
}

// GetAuthTimeout returns the XAuth/SAML timeout in seconds.
func (c *Connection) GetAuthTimeout() int {
	if c.IKESettings.XAuthTimeout > 0 {
		return c.IKESettings.XAuthTimeout
	}
	return 300 // Default SAML timeout
}

// HasSAML returns true if SAML/SSO is enabled for this connection.
func (c *Connection) HasSAML() bool {
	return c.IKESettings.SSOEnabled == 1
}

// SupportsIPsec returns true if this is an IPsec connection.
func (c *Connection) SupportsIPsec() bool {
	return c.IKESettings.Server != ""
}
