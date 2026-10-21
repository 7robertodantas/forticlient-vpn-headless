package bridge

import (
	"encoding/json"
	"sync"
)

// WSMessage represents a WebSocket message to/from fctctld.
type WSMessage struct {
	Type string          `json:"type"`
	Msg  json.RawMessage `json:"msg,omitempty"`
}

// MessageHandler is a bridge route handler that processes a message
// and returns a response.
type MessageHandler func(broker *messageBroker, msg WSMessage) WSMessage

// messageBroker routes messages between WebSocket clients and fctctld.
type messageBroker struct {
	mu          sync.Mutex
	connections map[*wsClient]bool
}

func newBroker() *messageBroker {
	return &messageBroker{
		connections: make(map[*wsClient]bool),
	}
}

func (b *messageBroker) add(c *wsClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.connections[c] = true
}

func (b *messageBroker) remove(c *wsClient) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.connections, c)
}

func (b *messageBroker) broadcast(msg []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.connections {
		if c != nil {
			c.send(msg)
		}
	}
}
