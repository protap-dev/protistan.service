package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	bookingdomain "encore.app/booking/domain"
	"encore.app/chat/domain"
	"encore.app/chat/handlers"
	eventscommon "encore.app/core/events"
	topics_booking "encore.app/core/events/topics/booking"
	"encore.dev/pubsub"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const SystemSenderID = "00000000-0000-0000-0000-000000000000"

// BookingSubscriber handles incoming booking events
type BookingSubscriber struct {
	threadRepo  domain.ThreadRepository
	messageRepo domain.MessageRepository
	validator   *domain.Validator
	publisher   domain.EventPublisher
	wsManager   handlers.WebSocketBroadcaster
}

// NewBookingSubscriber creates a new booking event subscriber
func NewBookingSubscriber(
	threadRepo domain.ThreadRepository,
	messageRepo domain.MessageRepository,
	validator *domain.Validator,
	publisher domain.EventPublisher,
	wsManager handlers.WebSocketBroadcaster,
) *BookingSubscriber {
	return &BookingSubscriber{
		threadRepo:  threadRepo,
		messageRepo: messageRepo,
		validator:   validator,
		publisher:   publisher,
		wsManager:   wsManager,
	}
}

// Package-level singleton for dependency injection
var subscriberInstance *BookingSubscriber

// SetSubscriberDependencies sets the dependencies for the subscriber
// This should be called during service initialization
func SetSubscriberDependencies(
	threadRepo domain.ThreadRepository,
	messageRepo domain.MessageRepository,
	validator *domain.Validator,
	publisher domain.EventPublisher,
	wsManager handlers.WebSocketBroadcaster,
) {
	subscriberInstance = NewBookingSubscriber(
		threadRepo,
		messageRepo,
		validator,
		publisher,
		wsManager,
	)
}

// Subscription for booking-assigned events (creates thread)
var _ = pubsub.NewSubscription(
	topics_booking.BookingAssigned,
	"chat-booking-assigned-handler",
	pubsub.SubscriptionConfig[eventscommon.EventEnvelope[bookingdomain.BookingEvent]]{
		Handler: HandleBookingAssigned,
	},
)

// Subscription to booking-status topic (automated messages)
var _ = pubsub.NewSubscription(
	topics_booking.BookingStatus,
	"chat-automated-messages",
	pubsub.SubscriptionConfig[eventscommon.EventEnvelope[bookingdomain.BookingEvent]]{
		Handler: HandleBookingStatusChange,
	},
)

// HandleBookingStatusChange processes booking status changes and generates automated messages
func HandleBookingStatusChange(ctx context.Context, envelope eventscommon.EventEnvelope[bookingdomain.BookingEvent]) error {
	if subscriberInstance == nil {
		return fmt.Errorf("subscriber not initialized")
	}

	event := envelope.Data

	// The 'assigned' status is handled by HandleBookingAssigned to create the thread and welcome message atomically.
	if event.Status == bookingdomain.BookingAssigned {
		return nil
	}

	// Check if we have an automated message template for this status
	template, exists := domain.GetAutomatedMessage(string(event.Status))
	if !exists {
		// No automated message configured for this status - that's fine
		return nil
	}

	// Get or create thread for this booking
	thread, err := subscriberInstance.getOrCreateThread(ctx, &event)
	if err != nil {
		return fmt.Errorf("failed to get/create thread: %w", err)
	}

	// Generate and save automated message
	if err := subscriberInstance.createAutomatedMessage(ctx, thread, template, &event); err != nil {
		return fmt.Errorf("failed to create automated message: %w", err)
	}

	return nil
}

// HandleBookingAssigned is the handler function that will be called when a booking-assigned event is received.
// It is responsible for creating the thread and sending the initial welcome message.
func HandleBookingAssigned(ctx context.Context, envelope eventscommon.EventEnvelope[bookingdomain.BookingEvent]) error {
	if subscriberInstance == nil {
		return fmt.Errorf("subscriber not initialized")
	}

	// Extract booking event data
	bookingEvent := envelope.Data

	// Ensure artisan is assigned
	if bookingEvent.ArtisanID == nil || *bookingEvent.ArtisanID == "" {
		log.Printf("Skipping thread creation: no artisan assigned to booking %s", bookingEvent.BookingID)
		return nil
	}

	// Check if thread already exists (idempotency)
	existingThread, err := subscriberInstance.threadRepo.GetByBookingID(ctx, bookingEvent.BookingID)
	if err == nil && existingThread != nil {
		log.Printf("Thread already exists for booking %s, skipping creation", bookingEvent.BookingID)
		return nil // Idempotent: thread already created
	}

	// Validate thread creation input
	input := &domain.CreateThreadInput{
		BookingID:     bookingEvent.BookingID,
		CustomerID:    bookingEvent.Metadata["customer_id"],
		ArtisanID:     *bookingEvent.ArtisanID,
		ArtisanUserID: bookingEvent.UserID,
	}

	if err := subscriberInstance.validator.ValidateCreateThread(ctx, input); err != nil {
		log.Printf("Failed to validate thread creation: %v", err)
		return err
	}
	metadataMap := map[string]string{
		"artisan_user_id": bookingEvent.UserID,
	}

	jsonMetadata, err := json.Marshal(metadataMap)

	if err != nil {
		log.Printf("Failed to marshal thread metadata: %v", err)
	}

	// Create new thread
	now := time.Now()
	thread := &domain.Thread{
		ID:         uuid.NewString(),
		BookingID:  bookingEvent.BookingID,
		CustomerID: bookingEvent.Metadata["customer_id"],
		ArtisanID:  *bookingEvent.ArtisanID,
		CreatedAt:  now,
		UpdatedAt:  now,
		Metadata:   jsonMetadata,
	}

	if err := subscriberInstance.threadRepo.Create(ctx, thread); err != nil {
		// Handle duplicate key error gracefully (race condition)
		if err == domain.ErrThreadAlreadyExists {
			log.Printf("Thread already exists for booking %s (race condition)", bookingEvent.BookingID)
			return nil
		}
		log.Printf("Failed to create thread: %v", err)
		return err
	}

	log.Printf("Successfully created thread %s for booking %s", thread.ID, bookingEvent.BookingID)

	// Now, create the automated welcome message for the new thread
	template, exists := domain.GetAutomatedMessage(string(bookingEvent.Status))
	if !exists {
		// This shouldn't happen for 'assigned', but good to check
		log.Printf("No automated message template found for status: %s", bookingEvent.Status)
	} else {
		if err := subscriberInstance.createAutomatedMessage(ctx, thread, template, &bookingEvent); err != nil {
			log.Printf("Failed to create automated welcome message for thread %s: %v", thread.ID, err)
			// Don't fail the whole transaction, but log it as an error
		}
	}

	// Publish thread-created event
	chatEvent := &domain.ChatEvent{
		ThreadID:   thread.ID,
		BookingID:  thread.BookingID,
		CustomerID: thread.CustomerID,
		ArtisanID:  thread.ArtisanID,
		EventType:  "thread.created",
		Timestamp:  now,
		UserID:     bookingEvent.UserID,
	}

	subscriberInstance.publisher.PublishThreadCreatedEvent(ctx, chatEvent)

	return nil
}

// ========================
// HELPERS
// ========================

// getOrCreateThread retrieves existing thread or creates new one
func (s *BookingSubscriber) getOrCreateThread(
	ctx context.Context,
	event *bookingdomain.BookingEvent,
) (*domain.Thread, error) {
	// Try to get existing thread
	thread, err := s.threadRepo.GetByBookingID(ctx, event.BookingID)
	if err == nil {
		return thread, nil // Thread exists
	}

	// Thread doesn't exist yet - create it (only if artisan is assigned)
	if event.ArtisanID == nil {
		return nil, fmt.Errorf("cannot create thread: artisan not assigned yet")
	}

	// Extract customer_id from metadata
	customerID, ok := event.Metadata["customer_id"]
	if !ok || customerID == "" {
		return nil, fmt.Errorf("customer_id not found in event metadata for booking %s", event.BookingID)
	}

	now := time.Now()
	newThread := &domain.Thread{
		ID:         uuid.NewString(),
		BookingID:  event.BookingID,
		CustomerID: customerID, // Use customerID from metadata
		ArtisanID:  *event.ArtisanID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.threadRepo.Create(ctx, newThread); err != nil {
		return nil, fmt.Errorf("failed to create thread: %w", err)
	}

	return newThread, nil
}

// createAutomatedMessage creates and broadcasts system message
func (s *BookingSubscriber) createAutomatedMessage(
	ctx context.Context,
	thread *domain.Thread,
	template domain.AutomatedMessageTemplate,
	event *bookingdomain.BookingEvent,
) error {
	// Build message metadata
	metadataJSON := datatypes.JSON(nil)
	if fn := template.MetadataFunc; fn != nil {
		if metadataMap := fn(event); len(metadataMap) > 0 {
			if bytes, err := json.Marshal(metadataMap); err != nil {
				log.Printf("Failed to marshal message metadata: %v", err)
			} else {
				metadataJSON = bytes
			}
		}
	}

	data := buildTemplateData(event)

	// Generate message content
	content := template.ContentFunc(data)

	// Create automated message
	now := time.Now()
	msg := &domain.Message{
		ID:             uuid.NewString(),
		ThreadID:       thread.ID,
		SenderID:       SystemSenderID, // Special system sender
		Content:        content,
		MessageType:    template.MessageType,
		Status:         domain.MessageSent,
		SentAt:         &now,
		IdempotencyKey: fmt.Sprintf("auto-%s-%s", event.BookingID, event.Status),
		Metadata:       metadataJSON, // ✅ Now includes useful IDs
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Save to database
	if err := s.messageRepo.Create(ctx, msg); err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	// Broadcast to WebSocket (if users are connected)
	s.wsManager.Broadcast(&handlers.WSMessage{
		ID:       msg.ID,
		ThreadID: msg.ThreadID,
		SenderID: msg.SenderID,
		Content:  msg.Content,
		Status:   string(msg.Status),
		SentAt:   now,
		Metadata: json.RawMessage(msg.Metadata), // Include metadata in WebSocket broadcast
	})

	return nil
}

// buildTemplateData constructs data map for message templates
func buildTemplateData(event *bookingdomain.BookingEvent) map[string]interface{} {
	data := map[string]interface{}{
		"booking_id": event.BookingID,
		"status":     event.Status,
		"timestamp":  event.Timestamp,
	}

	// Add artisan info if available
	if event.ArtisanID != nil {
		data["artisan_id"] = *event.ArtisanID
	}

	for k, v := range event.Metadata {
		data[k] = v
	}

	// Normalize artisan name
	artisanName := "The artisan"
	if raw, ok := data["artisan_name"]; ok {
		if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
			artisanName = str
		}
	}
	data["artisan_name"] = artisanName

	// Normalize service type
	serviceType := "service"
	if raw, ok := data["service_type"]; ok {
		if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
			serviceType = str
		}
	}
	data["service_type"] = serviceType

	// Parse amount if present
	if amountStr, ok := event.Metadata["amount"]; ok {
		if parsed, err := strconv.ParseFloat(amountStr, 64); err == nil {
			data["amount"] = parsed
		}
	}
	if _, ok := data["amount"]; !ok {
		data["amount"] = float64(0)
	}

	// Add currency (default to NGN)
	if currency, ok := event.Metadata["currency"]; ok {
		data["currency"] = currency
	} else {
		data["currency"] = "NGN"
	}

	// Add reason if present
	if reason, ok := event.Metadata["reason"]; ok {
		data["reason"] = reason
	} else {
		data["reason"] = ""
	}

	// Parse ETA minutes if present
	if etaStr, ok := event.Metadata["eta_minutes"]; ok {
		if parsed, err := strconv.Atoi(etaStr); err == nil {
			data["eta_minutes"] = parsed
		}
	}
	if _, ok := data["eta_minutes"]; !ok {
		data["eta_minutes"] = 0
	}

	// Parse scheduled time if provided
	if scheduledAtStr, ok := event.Metadata["scheduled_at"]; ok {
		if ts, err := time.Parse(time.RFC3339, scheduledAtStr); err == nil {
			data["scheduled_at"] = ts
		}
	}

	// Determine who cancelled
	if event.Status == bookingdomain.BookingCancelled {
		cancelledBy := "the customer"
		if raw, ok := event.Metadata["cancelled_by"]; ok && strings.TrimSpace(raw) != "" {
			cancelledBy = raw
		}
		data["cancelled_by"] = cancelledBy
	}

	return data
}
