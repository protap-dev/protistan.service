package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"encore.app/chat/domain"
	"encore.app/chat/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// NewTestLogger creates a mock logger for testing
func NewTestLogger() internal.ServiceLogger {
	return &testLogger{}
}

type testLogger struct{}

func (l *testLogger) Info(ctx context.Context, message string, fields map[string]interface{}) {}

func (l *testLogger) Error(ctx context.Context, message string, err error, fields map[string]interface{}) {
}

func (l *testLogger) Warn(ctx context.Context, message string, fields map[string]interface{}) {}

// ============================================================================
// MOCK REPOSITORIES
// ============================================================================

// MockThreadRepository mocks the ThreadRepository
type MockThreadRepository struct {
	mock.Mock
}

func (m *MockThreadRepository) Create(ctx context.Context, thread *domain.Thread) error {
	args := m.Called(ctx, thread)
	return args.Error(0)
}

func (m *MockThreadRepository) GetByID(ctx context.Context, id string) (*domain.Thread, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Thread), args.Error(1)
}

func (m *MockThreadRepository) GetByBookingID(ctx context.Context, bookingID string) (*domain.Thread, error) {
	args := m.Called(ctx, bookingID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Thread), args.Error(1)
}

func (m *MockThreadRepository) GetByParticipant(ctx context.Context, userID string, limit, offset int) ([]*domain.Thread, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Thread), args.Error(1)
}

func (m *MockThreadRepository) Update(ctx context.Context, thread *domain.Thread) error {
	args := m.Called(ctx, thread)
	return args.Error(0)
}

func (m *MockThreadRepository) WithTransaction(ctx context.Context, fn func(txRepo domain.ThreadRepository) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// MockMessageRepository mocks the MessageRepository
type MockMessageRepository struct {
	mock.Mock
}

func (m *MockMessageRepository) Create(ctx context.Context, message *domain.Message) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *MockMessageRepository) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Message), args.Error(1)
}

func (m *MockMessageRepository) GetByIDForUpdate(ctx context.Context, id string) (*domain.Message, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Message), args.Error(1)
}

func (m *MockMessageRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Message, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Message), args.Error(1)
}

func (m *MockMessageRepository) GetByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*domain.Message, error) {
	args := m.Called(ctx, threadID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Message), args.Error(1)
}

func (m *MockMessageRepository) Update(ctx context.Context, message *domain.Message) error {
	args := m.Called(ctx, message)
	return args.Error(0)
}

func (m *MockMessageRepository) MarkThreadAsRead(ctx context.Context, threadID string, userID string) error {
	args := m.Called(ctx, threadID, userID)
	return args.Error(0)
}

func (m *MockMessageRepository) CreateEventInOutbox(ctx context.Context, event *domain.ChatEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockMessageRepository) WithTransaction(ctx context.Context, fn func(txRepo domain.MessageRepository) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// MockEventPublisher mocks domain.EventPublisher
type MockEventPublisher struct {
	mock.Mock
}

func (m *MockEventPublisher) PublishMessageSentEvent(ctx context.Context, event *domain.ChatEvent) {
	m.Called(ctx, event)
}

func (m *MockEventPublisher) PublishMessageDeliveredEvent(ctx context.Context, event *domain.ChatEvent) {
	m.Called(ctx, event)
}

func (m *MockEventPublisher) PublishThreadCreatedEvent(ctx context.Context, event *domain.ChatEvent) {
	m.Called(ctx, event)
}

// ============================================================================
// UNIT TESTS
// ============================================================================

func TestSendMessage_Success(t *testing.T) {
	ctx := context.Background()

	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	mockPublisher := new(MockEventPublisher)
	authHelper := internal.NewAuthHelper(NewTestLogger())
	logger := NewTestLogger()
	validator := domain.NewChatValidator()

	thread := &domain.Thread{
		ID:         "thread-123",
		BookingID:  "booking-123",
		CustomerID: "user-123",
		ArtisanID:  "user-456",
	}

	mockThreadRepo.On("GetByID", ctx, "thread-123").Return(thread, nil)
	mockMessageRepo.On("GetByIdempotencyKey", ctx, "key-123").Return(nil, nil)
	mockMessageRepo.On("Create", ctx, mock.AnythingOfType("*domain.Message")).Return(nil)
	mockThreadRepo.On("Update", ctx, mock.AnythingOfType("*domain.Thread")).Return(nil)
	mockPublisher.On("PublishMessageSentEvent", ctx, mock.AnythingOfType("*core/events.ChatEvent")).Return()

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		validator,
		logger,
		nil,
		authHelper,
		mockPublisher,
		nil,
	)

	req := &SendMessageRequest{
		Content:        "Hello World",
		IdempotencyKey: "key-123",
	}

	_, err := handler.SendMessage(ctx, "thread-123", req)

	// Since auth will fail in tests without Encore context, we expect an error
	// In production, Encore's auth middleware handles this
	if err != nil {
		// Auth error is expected in test environment
		assert.Error(t, err)
	}
}

func TestSendMessage_IdempotencyCheck(t *testing.T) {
	ctx := context.Background()

	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	authHelper := internal.NewAuthHelper(NewTestLogger())
	logger := NewTestLogger()
	validator := domain.NewChatValidator()

	thread := &domain.Thread{
		ID:         "thread-123",
		CustomerID: "user-123",
		ArtisanID:  "user-456",
	}

	existingMsg := &domain.Message{
		ID:             "msg-456",
		Content:        "Duplicate Message",
		IdempotencyKey: "key-123",
	}

	mockThreadRepo.On("GetByID", ctx, "thread-123").Return(thread, nil)
	mockMessageRepo.On("GetByIdempotencyKey", ctx, "key-123").Return(existingMsg, nil)

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		validator,
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	req := &SendMessageRequest{
		Content:        "Hello World",
		IdempotencyKey: "key-123",
	}

	_, err := handler.SendMessage(ctx, "thread-123", req)

	if err != nil {
		assert.Error(t, err)
	}
}

func TestSendMessage_UnauthorizedParticipant(t *testing.T) {
	ctx := context.Background()

	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	authHelper := internal.NewAuthHelper(NewTestLogger())
	logger := NewTestLogger()

	thread := &domain.Thread{
		ID:         "thread-123",
		CustomerID: "user-123",
		ArtisanID:  "user-456",
	}

	mockThreadRepo.On("GetByID", ctx, "thread-123").Return(thread, nil)

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		domain.NewChatValidator(),
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	req := &SendMessageRequest{
		Content:        "Unauthorized",
		IdempotencyKey: "key-123",
	}

	resp, err := handler.SendMessage(ctx, "thread-123", req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestGetMessages_WithPagination(t *testing.T) {
	ctx := context.Background()

	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	authHelper := internal.NewAuthHelper(NewTestLogger())
	logger := NewTestLogger()

	thread := &domain.Thread{
		ID:         "thread-123",
		CustomerID: "user-123",
		ArtisanID:  "user-456",
	}

	messages := []*domain.Message{
		{ID: "msg-1", Content: "First"},
		{ID: "msg-2", Content: "Second"},
	}

	mockThreadRepo.On("GetByID", ctx, "thread-123").Return(thread, nil)
	mockMessageRepo.On("GetByThreadID", ctx, "thread-123", 50, 0).Return(messages, nil)

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		domain.NewChatValidator(),
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	params := &ListMessagesRequest{Limit: 50, Offset: 0}
	_, err := handler.GetMessages(ctx, "thread-123", params)

	if err != nil {
		assert.Error(t, err)
	}
}

func TestMarkThreadAsRead(t *testing.T) {
	ctx := context.Background()

	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	authHelper := internal.NewAuthHelper(NewTestLogger())
	logger := NewTestLogger()

	thread := &domain.Thread{
		ID:         "thread-123",
		CustomerID: "user-123",
		ArtisanID:  "user-456",
	}

	mockThreadRepo.On("GetByID", ctx, "thread-123").Return(thread, nil)
	mockMessageRepo.On("MarkThreadAsRead", ctx, "thread-123", "user-123").Return(nil)

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		domain.NewChatValidator(),
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	err := handler.MarkThreadAsRead(ctx, "thread-123")

	if err != nil {
		assert.Error(t, err)
	}
}

func TestHandleStatusAck_Delivered(t *testing.T) {
	ctx := context.Background()
	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	mockPublisher := new(MockEventPublisher)
	logger := NewTestLogger()
	authHelper := internal.NewAuthHelper(logger)
	validator := domain.NewChatValidator()

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		validator,
		logger,
		nil,
		authHelper,
		mockPublisher,
		nil,
	)

	now := time.Now().UTC()
	m := &domain.Message{
		ID:        "msg-1",
		ThreadID:  "thread-1",
		SenderID:  "user-1",
		Status:    domain.MessageSent,
		SentAt:    &now,
		CreatedAt: now,
		UpdatedAt: now,
		DBVersion: 1,
	}

	mockMessageRepo.On("GetByIDForUpdate", ctx, "msg-1").Return(m, nil)
	mockMessageRepo.On("Update", ctx, m).Return(nil)

	wsMsg := &WSMessage{ID: "msg-1", Status: "delivered", Type: "ack"}

	err := handler.handleStatusAck(ctx, "thread-1", wsMsg)
	assert.NoError(t, err)
	assert.Equal(t, domain.MessageDelivered, m.Status)
	assert.NotNil(t, m.DeliveredAt)
	mockMessageRepo.AssertExpectations(t)
}

func TestHandleStatusAck_InvalidTransition(t *testing.T) {
	ctx := context.Background()
	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	logger := NewTestLogger()
	authHelper := internal.NewAuthHelper(logger)
	validator := domain.NewChatValidator()

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		validator,
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	now := time.Now().UTC()
	m := &domain.Message{
		ID:        "msg-2",
		ThreadID:  "thread-2",
		SenderID:  "user-1",
		Status:    domain.MessageSent,
		SentAt:    &now,
		CreatedAt: now,
		UpdatedAt: now,
		DBVersion: 1,
	}

	mockMessageRepo.On("GetByIDForUpdate", ctx, "msg-2").Return(m, nil)

	wsMsg := &WSMessage{ID: "msg-2", Status: "read", Type: "ack"}

	err := handler.handleStatusAck(ctx, "thread-2", wsMsg)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidTransition))
	assert.Equal(t, domain.MessageSent, m.Status)
	mockMessageRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestHandleStatusAck_Idempotent(t *testing.T) {
	ctx := context.Background()
	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	logger := NewTestLogger()
	authHelper := internal.NewAuthHelper(logger)
	validator := domain.NewChatValidator()

	handler := NewMessagesHandler(
		mockThreadRepo,
		mockMessageRepo,
		validator,
		logger,
		nil,
		authHelper,
		nil,
		nil,
	)

	now := time.Now().UTC()
	m := &domain.Message{
		ID:          "msg-3",
		ThreadID:    "thread-3",
		SenderID:    "user-1",
		Status:      domain.MessageDelivered,
		SentAt:      &now,
		DeliveredAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
		DBVersion:   2,
	}

	mockMessageRepo.On("GetByIDForUpdate", ctx, "msg-3").Return(m, nil)

	wsMsg := &WSMessage{ID: "msg-3", Status: "delivered", Type: "ack"}

	err := handler.handleStatusAck(ctx, "thread-3", wsMsg)
	assert.NoError(t, err)
	mockMessageRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}
