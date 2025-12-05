package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"encore.app/chat/domain"
	"encore.app/chat/internal"
	"github.com/gorilla/websocket"
)

var wsManager *WebSocketManager

func init() {
	wsManager = &WebSocketManager{
		clients:   make(map[string]map[*WSClient]bool),
		broadcast: make(chan *WSMessage, 256),
	}
	go wsManager.broadcastLoop()
}

// GetWSManager returns the global WebSocket manager instance.
func GetWSManager() *WebSocketManager {
	return wsManager
}

// RegisterClient registers a new WebSocket client for a thread.
func (wm *WebSocketManager) RegisterClient(threadID, userID string, send chan *WSMessage) *WSClient {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.clients[threadID]; !exists {
		wm.clients[threadID] = make(map[*WSClient]bool)
	}

	client := &WSClient{
		threadID: threadID,
		userID:   userID,
		send:     send,
		close:    make(chan struct{}),
	}

	wm.clients[threadID][client] = true
	return client
}

// UnregisterClient removes a specific WebSocket client.
func (wm *WebSocketManager) UnregisterClient(client *WSClient) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if client == nil {
		return
	}

	threadID := client.threadID
	if threadClients, exists := wm.clients[threadID]; exists {
		if _, ok := threadClients[client]; ok {
			close(client.close)
			delete(threadClients, client)
		}
		if len(threadClients) == 0 {
			delete(wm.clients, threadID)
		}
	}
}

// Broadcast sends a message to all clients in a thread.
func (wm *WebSocketManager) Broadcast(msg *WSMessage) {
	select {
	case wm.broadcast <- msg:
	case <-time.After(100 * time.Millisecond):
		log.Printf("Broadcast channel full, message dropped for thread %s", msg.ThreadID)
	}
}

// BroadcastToThread sends a message to all clients in a specific thread.
func (wm *WebSocketManager) BroadcastToThread(threadID string, msg *WSMessage) {
	msg.ThreadID = threadID
	wm.Broadcast(msg)
}

// broadcastLoop handles message distribution with a <100ms latency guarantee.
func (wm *WebSocketManager) broadcastLoop() {
	for msg := range wm.broadcast {
		wm.mu.RLock()
		threadClientSet, exists := wm.clients[msg.ThreadID]

		// Create a list of clients to send to, so we can release the lock quickly.
		var clients []*WSClient
		if exists {
			for client := range threadClientSet {
				clients = append(clients, client)
			}
		}
		wm.mu.RUnlock()

		if !exists || len(clients) == 0 {
			continue
		}

		// Send to all clients in the thread with a timeout.
		for _, client := range clients {
			// Note: This design broadcasts to the sender as well.
			// Client-side logic is expected to handle this (e.g., by not displaying your own messages twice).
			select {
			case client.send <- msg:
			case <-time.After(50 * time.Millisecond):
				log.Printf("Failed to send message to client %s in thread %s: channel blocked", client.userID, msg.ThreadID)
			}
		}
	}
}

// HandleWebSocketConnection manages a WebSocket client connection.
func (h *MessagesHandler) HandleWebSocketConnection(ctx context.Context, threadID string, conn *websocket.Conn) {
	userContext, err := h.authHelper.ExtractUserContext(ctx, "websocket_stream")
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, []byte("Unauthorized"))
		conn.Close()
		return
	}
	userID := userContext.ID

	thread, err := h.threadRepo.GetByID(ctx, threadID)
	if err != nil {
		conn.WriteMessage(websocket.CloseMessage, []byte("Thread not found"))
		conn.Close()
		return
	}

	if err := internal.AuthorizeThreadSubscription(ctx, userID, thread); err != nil {
		conn.WriteMessage(websocket.CloseMessage, []byte("Access denied"))
		conn.Close()
		return
	}

	msgChan := make(chan *WSMessage, 32)
	client := GetWSManager().RegisterClient(threadID, userID, msgChan)
	defer GetWSManager().UnregisterClient(client)

	recentMessages, err := h.messageRepo.GetByThreadID(ctx, threadID, 50, 0)
	if err == nil {
		for i := len(recentMessages) - 1; i >= 0; i-- {
			msg := recentMessages[i]
			wsMsg := &WSMessage{
				ID:          msg.ID,
				ThreadID:    msg.ThreadID,
				SenderID:    msg.SenderID,
				Content:     msg.Content,
				Status:      string(msg.Status),
				SentAt:      *msg.SentAt,
				MessageType: string(msg.MessageType),
			}

			// Add metadata if it exists
			if len(msg.Metadata) > 0 && string(msg.Metadata) != "null" {
				wsMsg.Metadata = json.RawMessage(msg.Metadata)
			}

			client.send <- wsMsg
		}
	}

	go func() {
		for {
			var msg WSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				conn.Close()
				return
			}
			msg.ThreadID = threadID
			if strings.EqualFold(msg.Type, "ack") {
				if err := h.handleStatusAck(ctx, threadID, &msg); err != nil {
					h.logger.Warn(ctx, "websocket_ack_failed", map[string]interface{}{
						"thread_id":  threadID,
						"message_id": msg.ID,
						"error":      err.Error(),
					})
				}
				continue
			}
			msg.Type = "message"
			msg.SenderID = userID
			msg.SentAt = time.Now()
			if msg.Status == "" {
				msg.Status = string(domain.MessageSent)
			}
			GetWSManager().Broadcast(&msg)
		}
	}()

	for msg := range msgChan {
		if err := conn.WriteJSON(msg); err != nil {
			conn.Close()
			return
		}
	}
}

func (h *MessagesHandler) handleStatusAck(ctx context.Context, threadID string, msg *WSMessage) error {
	if msg == nil || msg.ID == "" {
		return internal.ErrInvalidInput
	}

	targetStatus, err := parseAckStatus(msg.Status)
	if err != nil {
		return err
	}

	message, err := h.messageRepo.GetByIDForUpdate(ctx, msg.ID)
	if err != nil {
		return err
	}

	if message.ThreadID != threadID {
		return domain.ErrInvalidParticipant
	}

	if message.Status == targetStatus {
		h.broadcastStatusUpdate(message)
		return nil
	}

	if !message.CanTransitionTo(targetStatus) {
		return domain.ErrInvalidTransition
	}

	now := time.Now().UTC()
	message.Status = targetStatus
	message.UpdatedAt = now

	switch targetStatus {
	case domain.MessageDelivered:
		if message.DeliveredAt == nil {
			message.DeliveredAt = &now
		}
	case domain.MessageRead:
		if message.DeliveredAt == nil {
			message.DeliveredAt = &now
		}
		message.ReadAt = &now
	}

	if err := h.messageRepo.Update(ctx, message); err != nil {
		return err
	}

	h.broadcastStatusUpdate(message)
	return nil
}

func parseAckStatus(status string) (domain.MessageStatus, error) {
	switch strings.ToLower(status) {
	case string(domain.MessageDelivered):
		return domain.MessageDelivered, nil
	case string(domain.MessageRead):
		return domain.MessageRead, nil
	default:
		return "", domain.ErrInvalidTransition
	}
}

func (h *MessagesHandler) broadcastStatusUpdate(message *domain.Message) {
	if message == nil {
		return
	}

	wsMsg := &WSMessage{
		ID:          message.ID,
		ThreadID:    message.ThreadID,
		SenderID:    message.SenderID,
		MessageType: string(message.MessageType),
		Status:      string(message.Status),
		Type:        "status",
	}

	if message.SentAt != nil {
		wsMsg.SentAt = *message.SentAt
	} else {
		wsMsg.SentAt = message.CreatedAt
	}
	if message.DeliveredAt != nil {
		wsMsg.DeliveredAt = message.DeliveredAt
	}
	if message.ReadAt != nil {
		wsMsg.ReadAt = message.ReadAt
	}

	GetWSManager().Broadcast(wsMsg)
}
