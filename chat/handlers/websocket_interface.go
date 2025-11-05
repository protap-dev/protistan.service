package handlers

// WebSocketBroadcaster defines the interface for broadcasting WebSocket messages
type WebSocketBroadcaster interface {
	Broadcast(message *WSMessage)
	BroadcastToThread(threadID string, message *WSMessage)
}
