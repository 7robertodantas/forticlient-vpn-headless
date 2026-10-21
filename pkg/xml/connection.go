package xml

import "fcvpn/pkg/platform"

// ToPlatform converts an XML connection to a platform.Connection.
func (c *Connection) ToPlatform() platform.Connection {
	return platform.Connection{
		Name:   c.Name,
		Type:   c.Type,
		UID:    c.UID,
		IKESettings: platform.IKESettings{
			Server:             c.IKESettings.Server,
			Version:            c.IKESettings.Version,
			Mode:               c.IKESettings.Mode,
			AuthMethod:         c.IKESettings.AuthMethod,
			SSOEnabled:         c.IKESettings.SSOEnabled,
			UseExternalBrowser: c.IKESettings.UseExternalBrowser,
			IKEsamlPort:        c.IKESettings.IKEsamlPort,
			TCPPort:            c.IKESettings.TCPPort,
			XAuthTimeout:       c.IKESettings.XAuthTimeout,
		},
	}
}

// FromPlatform converts a platform.Connection back to an XML connection.
func FromPlatform(p platform.Connection) *Connection {
	return &Connection{
		Name:   p.Name,
		Type:   p.Type,
		UID:    p.UID,
		IKESettings: IKESettings{
			Server:             p.IKESettings.Server,
			Version:            p.IKESettings.Version,
			Mode:               p.IKESettings.Mode,
			AuthMethod:         p.IKESettings.AuthMethod,
			SSOEnabled:         p.IKESettings.SSOEnabled,
			UseExternalBrowser: p.IKESettings.UseExternalBrowser,
			IKEsamlPort:        p.IKESettings.IKEsamlPort,
			TCPPort:            p.IKESettings.TCPPort,
			XAuthTimeout:       p.IKESettings.XAuthTimeout,
		},
	}
}
