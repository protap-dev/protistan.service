package tests

import (
	"context"
	"testing"
	"time"

	bookingdomain "encore.app/booking/domain"
	"encore.app/chat/domain"
	"encore.app/chat/events"
	"encore.app/chat/handlers"
	eventscommon "encore.app/core/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ============================================
// MOCKS
// ============================================

// MockThreadRepository for testing
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
	return args.Get(0).([]*domain.Thread), args.Error(1)
}

func (m *MockThreadRepository) Update(ctx context.Context, thread *domain.Thread) error {
	args := m.Called(ctx, thread)
	return args.Error(0)
}

// Implements handlers.WebSocketBroadcaster interface
func (m *MockWebSocketManager) Broadcast(message *handlers.WSMessage) {
	m.broadcasted = append(m.broadcasted, message)
}

// Implements handlers.WebSocketBroadcaster interface
func (m *MockWebSocketManager) BroadcastToThread(threadID string, message *handlers.WSMessage) {
	m.broadcasted = append(m.broadcasted, message)
}

func (m *MockThreadRepository) WithTransaction(ctx context.Context, fn func(txRepo domain.ThreadRepository) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// MockMessageRepository for testing
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
		return []*domain.Message{}, args.Error(1)
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

func (m *MockMessageRepository) WithTransaction(ctx context.Context, fn func(txRepo domain.MessageRepository) error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// Missing method from interface
func (m *MockMessageRepository) CreateEventInOutbox(ctx context.Context, event *domain.ChatEvent) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

// MockWebSocketManager for testing
type MockWebSocketManager struct {
	broadcasted []*handlers.WSMessage
}

func NewMockWebSocketManager() *MockWebSocketManager {
	return &MockWebSocketManager{
		broadcasted: make([]*handlers.WSMessage, 0),
	}
}

func (m *MockWebSocketManager) GetBroadcasted() []*handlers.WSMessage {
	return m.broadcasted
}

// ============================================
// TESTS: HandleBookingAssigned
// ============================================

func TestBookingAssignedHandler_CreatesThread(t *testing.T) {
	ctx := context.Background()

	t.Run("creates thread when booking is assigned", func(t *testing.T) {
		// Setup mocks
		mockThreadRepo := new(MockThreadRepository)
		mockMessageRepo := new(MockMessageRepository)
		mockPublisher := events.NewMockEventPublisher()
		mockWsManager := NewMockWebSocketManager()
		mockValidator := domain.NewChatValidator()

		// Initialize subscriber with mocks
		events.SetSubscriberDependencies(
			mockThreadRepo,
			mockMessageRepo,
			mockValidator,
			mockPublisher,
			mockWsManager,
		)

		// Prepare test data
		artisanID := "artisan-123"
		bookingEvent := bookingdomain.BookingEvent{
			BookingID: "booking-123",
			UserID:    "customer-123",
			ArtisanID: &artisanID,
			Status:    bookingdomain.BookingAssigned,
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"customer_id": "customer-123",
			},
		}

		envelope := eventscommon.EventEnvelope[bookingdomain.BookingEvent]{
			EventID:    "event-123",
			EventType:  "booking.assigned",
			OccurredAt: time.Now(),
			Producer:   "booking-service",
			Data:       bookingEvent,
		}

		// Mock expectations
		mockThreadRepo.On("GetByBookingID", ctx, "booking-123").Return(nil, domain.ErrThreadNotFound)
		mockThreadRepo.On("Create", ctx, mock.AnythingOfType("*domain.Thread")).Return(nil)
		mockMessageRepo.On("Create", ctx, mock.AnythingOfType("*domain.Message")).Return(nil)

		// Execute handler
		err := events.HandleBookingAssigned(ctx, envelope)

		// Assertions
		assert.NoError(t, err)
		mockThreadRepo.AssertExpectations(t)

		// Verify thread-created event was published
		publishedEvents := mockPublisher.GetThreadCreatedEvents()
		assert.Len(t, publishedEvents, 1)
		assert.Equal(t, "booking-123", publishedEvents[0].BookingID)
		assert.Equal(t, "customer-123", publishedEvents[0].CustomerID)
		assert.Equal(t, "artisan-123", publishedEvents[0].ArtisanID)
	})

	t.Run("skips thread creation if thread already exists (idempotency)", func(t *testing.T) {
		// Setup mocks
		mockThreadRepo := new(MockThreadRepository)
		mockMessageRepo := new(MockMessageRepository)
		mockPublisher := events.NewMockEventPublisher()
		mockWsManager := NewMockWebSocketManager()
		mockValidator := domain.NewChatValidator()

		events.SetSubscriberDependencies(
			mockThreadRepo,
			mockMessageRepo,
			mockValidator,
			mockPublisher,
			mockWsManager,
		)

		// Prepare test data
		artisanID := "artisan-123"
		bookingEvent := bookingdomain.BookingEvent{
			BookingID: "booking-123",
			UserID:    "customer-123",
			ArtisanID: &artisanID,
			Status:    bookingdomain.BookingAssigned,
		}

		envelope := eventscommon.EventEnvelope[bookingdomain.BookingEvent]{
			EventID:   "event-123",
			EventType: "booking.assigned",
			Data:      bookingEvent,
		}

		existingThread := &domain.Thread{
			ID:         "thread-456",
			BookingID:  "booking-123",
			CustomerID: "customer-123",
			ArtisanID:  "artisan-123",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Mock: Thread already exists
		mockThreadRepo.On("GetByBookingID", ctx, "booking-123").Return(existingThread, nil)

		// Execute handler
		err := events.HandleBookingAssigned(ctx, envelope)

		// Assertions
		assert.NoError(t, err)
		mockThreadRepo.AssertExpectations(t)

		// Verify Create was NOT called
		mockThreadRepo.AssertNotCalled(t, "Create")

		// Verify no events were published
		publishedEvents := mockPublisher.GetThreadCreatedEvents()
		assert.Len(t, publishedEvents, 0)
	})

	t.Run("skips thread creation if no artisan assigned", func(t *testing.T) {
		// Setup mocks
		mockThreadRepo := new(MockThreadRepository)
		mockMessageRepo := new(MockMessageRepository)
		mockPublisher := events.NewMockEventPublisher()
		mockWsManager := NewMockWebSocketManager()
		mockValidator := domain.NewChatValidator()

		events.SetSubscriberDependencies(
			mockThreadRepo,
			mockMessageRepo,
			mockValidator,
			mockPublisher,
			mockWsManager,
		)

		// Prepare test data (no artisan)
		bookingEvent := bookingdomain.BookingEvent{
			BookingID: "booking-123",
			UserID:    "customer-123",
			ArtisanID: nil, // No artisan assigned
			Status:    bookingdomain.BookingRequested,
		}

		envelope := eventscommon.EventEnvelope[bookingdomain.BookingEvent]{
			EventID:   "event-123",
			EventType: "booking.created",
			Data:      bookingEvent,
		}

		// Execute handler
		err := events.HandleBookingAssigned(ctx, envelope)

		// Assertions
		assert.NoError(t, err)

		// Verify no repository calls were made
		mockThreadRepo.AssertNotCalled(t, "GetByBookingID")
		mockThreadRepo.AssertNotCalled(t, "Create")

		// Verify no events were published
		publishedEvents := mockPublisher.GetThreadCreatedEvents()
		assert.Len(t, publishedEvents, 0)
	})
}

// ============================================
// TESTS: HandleBookingStatusChange (Automated Messages)
// ============================================

func TestAutomatedMessage_QuoteProposed_ChatRoomStyle(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	mockPublisher := events.NewMockEventPublisher()
	mockWsManager := NewMockWebSocketManager()
	mockValidator := domain.NewChatValidator()

	events.SetSubscriberDependencies(
		mockThreadRepo,
		mockMessageRepo,
		mockValidator,
		mockPublisher,
		mockWsManager,
	)

	// Prepare test data
	artisanID := "artisan-123"
	bookingEvent := bookingdomain.BookingEvent{
		BookingID: "booking-456",
		Status:    bookingdomain.BookingQuoteProposed,
		UserID:    "customer-789",
		ArtisanID: &artisanID,
		Metadata: map[string]string{
			"amount":   "500.00",
			"currency": "USD",
		},
	}

	envelope := eventscommon.EventEnvelope[bookingdomain.BookingEvent]{
		EventID:   "event-123",
		EventType: "booking.quote_proposed",
		Data:      bookingEvent,
	}

	existingThread := &domain.Thread{
		ID:         "thread-789",
		BookingID:  "booking-456",
		CustomerID: "customer-789",
		ArtisanID:  "artisan-123",
	}

	// Mock expectations
	mockThreadRepo.On("GetByBookingID", ctx, "booking-456").Return(existingThread, nil)
	mockMessageRepo.On("Create", ctx, mock.AnythingOfType("*domain.Message")).Return(nil)

	// Execute handler
	err := events.HandleBookingStatusChange(ctx, envelope)

	// Assertions
	require.NoError(t, err)

	// Verify message has chat-room style (neutral, not customer-directed)
	var createdMessage *domain.Message
	for _, call := range mockMessageRepo.Calls {
		if call.Method == "Create" {
			createdMessage = call.Arguments.Get(1).(*domain.Message)
			break
		}
	}

	require.NotNil(t, createdMessage)
	assert.Contains(t, createdMessage.Content, "A quote for USD 500.00 has been submitted")
	assert.NotContains(t, createdMessage.Content, "Good news") // Not customer-directed
	assert.NotContains(t, createdMessage.Content, "your")      // Not possessive
}

func TestAutomatedMessage_NoMessageForPendingQuote(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	mockThreadRepo := new(MockThreadRepository)
	mockMessageRepo := new(MockMessageRepository)
	mockPublisher := events.NewMockEventPublisher()
	mockWsManager := NewMockWebSocketManager()
	mockValidator := domain.NewChatValidator()

	events.SetSubscriberDependencies(
		mockThreadRepo,
		mockMessageRepo,
		mockValidator,
		mockPublisher,
		mockWsManager,
	)

	// Prepare test data (pending_quote should NOT trigger message)
	artisanID := "artisan-123"
	bookingEvent := bookingdomain.BookingEvent{
		BookingID: "booking-456",
		Status:    bookingdomain.BookingPendingQuote,
		UserID:    "customer-789",
		ArtisanID: &artisanID,
	}

	envelope := eventscommon.EventEnvelope[bookingdomain.BookingEvent]{
		EventID:   "event-123",
		EventType: "booking.pending_quote",
		Data:      bookingEvent,
	}

	// Execute handler
	err := events.HandleBookingStatusChange(ctx, envelope)

	// Assertions
	require.NoError(t, err)

	// Verify NO message was created
	mockThreadRepo.AssertNotCalled(t, "GetByBookingID")
	mockMessageRepo.AssertNotCalled(t, "Create")

	// Verify no WebSocket broadcasts
	broadcasted := mockWsManager.GetBroadcasted()
	assert.Len(t, broadcasted, 0)
}
