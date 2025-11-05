package handlers

import (
	"context"
	"time"

	"encore.app/chat/domain"
	"encore.app/chat/internal"
	"encore.app/core"
)

type MessagesHandler struct {
	threadRepo  domain.ThreadRepository
	messageRepo domain.MessageRepository
	validator   *domain.Validator
	logger      internal.ServiceLogger
	coreSvc     *core.CoreService
	authHelper  *internal.AuthHelper
	publisher   domain.EventPublisher
	wsManager   WebSocketBroadcaster
}

func NewMessagesHandler(
	threadRepo domain.ThreadRepository,
	messageRepo domain.MessageRepository,
	validator *domain.Validator,
	logger internal.ServiceLogger,
	coreSvc *core.CoreService,
	authHelper *internal.AuthHelper,
	publisher domain.EventPublisher,
	wsManager WebSocketBroadcaster,
) *MessagesHandler {
	return &MessagesHandler{
		threadRepo:  threadRepo,
		messageRepo: messageRepo,
		validator:   validator,
		logger:      logger,
		coreSvc:     coreSvc,
		authHelper:  authHelper,
		publisher:   publisher,
		wsManager:   wsManager,
	}
}

// SendMessage sends a message in a thread (HTTP endpoint)
func (h *MessagesHandler) SendMessage(ctx context.Context, threadID string, req *SendMessageRequest) (*MessageResponse, error) {
	// Extract user from context
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "send_message")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	userID := userCtx.ID

	// Get thread to verify access
	thread, err := h.threadRepo.GetByID(ctx, threadID)
	if err != nil {
		return nil, internal.HandleRepositoryError(err)
	}

	// Verify user is thread participant
	if err := internal.AuthorizeSendMessage(ctx, userID, thread); err != nil {
		return nil, err
	}

	// Validate request
	sendInput := &domain.SendMessageInput{
		ThreadID:       threadID,
		SenderID:       userID,
		Content:        req.Content,
		IdempotencyKey: req.IdempotencyKey,
		MessageType:    domain.MessageTypeUser,
	}

	if err := h.validator.ValidateSendMessage(ctx, sendInput); err != nil {
		return nil, internal.HandleValidationError(err)
	}

	// Check for duplicate using idempotency key
	existing, err := h.messageRepo.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, internal.HandleRepositoryError(err)
	}
	if existing != nil {
		h.logger.Info(ctx, "message_already_exists", map[string]interface{}{"idempotency_key": req.IdempotencyKey})
		return MessageToResponse(existing), nil
	}

	// Create message
	now := time.Now()
	message := &domain.Message{
		ThreadID:       threadID,
		SenderID:       userID,
		Content:        req.Content,
		MessageType:    domain.MessageTypeUser,
		IdempotencyKey: req.IdempotencyKey,
		Status:         domain.MessageSent,
		SentAt:         &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.messageRepo.Create(ctx, message); err != nil {
		return nil, internal.HandleRepositoryError(err)
	}

	// Broadcast the message to connected WebSocket clients
	h.wsManager.Broadcast(&WSMessage{
		ID:       message.ID,
		ThreadID: message.ThreadID,
		SenderID: message.SenderID,
		Content:  message.Content,
		Status:   string(message.Status),
		SentAt:   *message.SentAt,
	})

	// Update thread's last message
	thread.LastMessageAt = &now
	thread.LastMessageID = &message.ID
	if err := h.threadRepo.Update(ctx, thread); err != nil {
		h.logger.Error(ctx, "failed_to_update_thread", err, nil)
	}

	// Publish message sent event
	chatEvent := &domain.ChatEvent{
		MessageID:     message.ID,
		ThreadID:      threadID,
		BookingID:     thread.BookingID,
		SenderID:      userID,
		ReceiverID:    getOtherParticipant(thread, userID),
		Content:       &message.Content,
		MessageStatus: string(domain.MessageSent),
		EventType:     "message.sent",
		Timestamp:     now,
		UserID:        userID,
	}
	h.publisher.PublishMessageSentEvent(ctx, chatEvent)

	return MessageToResponse(message), nil
}

// GetMessages retrieves paginated messages from a thread
func (h *MessagesHandler) GetMessages(ctx context.Context, threadID string, params *ListMessagesRequest) (*MessagesResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "get_messages")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	userID := userCtx.ID

	// Verify thread exists and user has access
	thread, err := h.threadRepo.GetByID(ctx, threadID)
	if err != nil {
		return nil, internal.HandleRepositoryError(err)
	}

	if err := internal.AuthorizeReadMessages(ctx, userID, thread); err != nil {
		return nil, err
	}

	// Normalize pagination parameters
	offset, limit, err := internal.HandlePaginationParams(params.Offset, params.Limit)
	if err != nil {
		return nil, err
	}
	params.Offset = offset
	params.Limit = limit

	// Get messages
	messages, err := h.messageRepo.GetByThreadID(ctx, threadID, params.Limit, params.Offset)
	if err != nil {
		return nil, internal.HandleRepositoryError(err)
	}

	responses := make([]*MessageResponse, len(messages))
	for i, msg := range messages {
		responses[i] = MessageToResponse(msg)
	}

	return &MessagesResponse{
		Messages: responses,
		Total:    len(responses),
	}, nil
}

// MarkThreadAsRead marks all messages in a thread as read
func (h *MessagesHandler) MarkThreadAsRead(ctx context.Context, threadID string) error {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "mark_thread_as_read")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil
	}

	userID := userCtx.ID

	// Verify thread exists
	thread, err := h.threadRepo.GetByID(ctx, threadID)
	if err != nil {
		return internal.HandleRepositoryError(err)
	}

	// Verify user is participant
	if err := internal.AuthorizeMarkThreadAsRead(ctx, userID, thread); err != nil {
		return err
	}

	// Mark messages as read
	if err := h.messageRepo.MarkThreadAsRead(ctx, threadID, userID); err != nil {
		return internal.HandleRepositoryError(err)
	}

	return nil
}

// Helper
func getOtherParticipant(thread *domain.Thread, userID string) string {
	if thread.CustomerID == userID {
		return thread.ArtisanID
	}
	return thread.CustomerID
}
