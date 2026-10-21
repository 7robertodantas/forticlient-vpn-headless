package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message represents a WebSocket message to/from fctservice.
type Message struct {
	Type string          `json:"type"`
	Msg  json.RawMessage `json:"msg,omitempty"`
}

// MessageResponse is a generic response from fctservice.
type MessageResponse struct {
	Success bool              `json:"success,omitempty"`
	Error   string            `json:"error,omitempty"`
	Msg     json.RawMessage   `json:"msg,omitempty"`
	Data    json.RawMessage   `json:"data,omitempty"`
	Status  string            `json:"status,omitempty"`
}

// ConnectRequest is sent to initiate a VPN connection.
type ConnectRequest struct {
	SAMData                string `json:"samldata,omitempty"`
	RemoteInfo             string `json:"remoteinfo,omitempty"`
	DBusSessionBusAddress  string `json:"dbus_session_bus_address,omitempty"`
	ConnectionName         string `json:"connection_name,omitempty"`
	Success                bool   `json:"success,omitempty"`
	Username               string `json:"username,omitempty"`
	UserName               string `json:"userName,omitempty"` // FortiGate may use either casing
	UID                    string `json:"uid,omitempty"`
	Certificate            string `json:"certificate,omitempty"`
	CertificateLabel       string `json:"certificate_label,omitempty"`
	PKCS11Lib              string `json:"pkcs11_lib,omitempty"`
	AlwaysUp               bool   `json:"always_up,omitempty"`
	AutoConnect            bool   `json:"auto_connect,omitempty"`
	SaveUsername           bool   `json:"save_username,omitempty"`
	SavePassword           bool   `json:"save_password,omitempty"`
	KeepRunning            bool   `json:"keep_running,omitempty"`
}

// XAuthRequest for interactive XAuth prompts.
type XAuthRequest struct {
	UserInput int    `json:"user_input,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Token     string `json:"token,omitempty"`
}

// Client handles WebSocket communication with fctservice.
type Client struct {
	URL      string
	conn     *websocket.Conn
	mu       sync.Mutex
	closed   bool
	resultCh chan Message
	errCh    chan error
	done     chan struct{}
}

// NewClient creates a new WebSocket client for fctservice.
func NewClient(port string) *Client {
	return &Client{
		URL:      fmt.Sprintf("ws://127.0.0.1:%s/websocket", port),
		resultCh: make(chan Message, 100),
		errCh:    make(chan error, 10),
		done:     make(chan struct{}),
	}
}

// Connect dials the WebSocket endpoint.
func (c *Client) Connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	header := http.Header{}
	header.Set("Origin", "http://127.0.0.1")

	conn, _, err := dialer.Dial(c.URL, header)
	if err != nil {
		return fmt.Errorf("dial WebSocket %s: %w", c.URL, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.closed = false
	c.mu.Unlock()

	// Start reader goroutine
	go c.readLoop(ctx)

	return nil
}

// Close disconnects the WebSocket.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	close(c.done)

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Send sends a message to fctservice.
func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.conn == nil {
		return fmt.Errorf("WebSocket connection is closed")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// SendConnect sends a VPN connect request.
func (c *Client) SendConnect(req ConnectRequest) error {
	msg := Message{
		Type: "FCT_VPN_CONNECT",
		Msg:  toJSON(req),
	}
	return c.Send(msg)
}

// SendSAMLResponse sends SAML authentication success response.
func (c *Client) SendSAMLResponse(success bool) error {
	msg := Message{
		Type: "FCT_VPN_SAML_RESPONSE",
		Msg:  json.RawMessage(fmt.Sprintf(`{"success":%t}`, success)),
	}
	return c.Send(msg)
}

// SendDisconnect sends a VPN disconnect request.
func (c *Client) SendDisconnect() error {
	return c.Send(Message{Type: "FCT_VPN_DISCONNECT"})
}

// SendCancel sends a VPN cancel request.
func (c *Client) SendCancel() error {
	return c.Send(Message{Type: "FCT_VPN_CANCEL"})
}

// SendXAuth sends XAuth credentials response.
func (c *Client) SendXAuth(req XAuthRequest) error {
	msg := Message{
		Type: "FCT_VPN_XAUTH",
		Msg:  toJSON(req),
	}
	return c.Send(msg)
}

// SendConfirmation sends a VPN confirmation.
func (c *Client) SendConfirmation() error {
	return c.Send(Message{Type: "FCT_VPN_CONFIRMATION"})
}

// Receive waits for the next message from fctservice.
func (c *Client) Receive(ctx context.Context) (*Message, error) {
	select {
	case msg := <-c.resultCh:
		return &msg, nil
	case err := <-c.errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ReceiveAll starts reading messages and returns a channel to receive them on.
// Callers should read from the returned channel until it's closed.
func (c *Client) ReceiveAll(ctx context.Context) <-chan Message {
	ch := make(chan Message, 100)
	go func() {
		defer close(ch)
		for {
			select {
			case msg := <-c.resultCh:
				ch <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// readLoop reads messages from the WebSocket.
func (c *Client) readLoop(ctx context.Context) {
	defer close(c.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if !c.closed {
				select {
				case c.errCh <- fmt.Errorf("read WebSocket: %w", err):
				default:
				}
			}
			return
		}

		var result Message
		if err := json.Unmarshal(msg, &result); err != nil {
			select {
			case c.errCh <- fmt.Errorf("unmarshal message: %w", err):
			default:
			}
			continue
		}

		select {
		case c.resultCh <- result:
		default:
			// Channel full, drop message
		}
	}
}

// Helper to convert to json.RawMessage safely.
func toJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return data
}
