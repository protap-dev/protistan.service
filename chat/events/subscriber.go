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
var initTrigger func() // Callback to trigger service initialization

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

// SetInitTrigger sets the callback to trigger service initialization
func SetInitTrigger(f func()) {
	initTrigger = f
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
	// Ensure service is initialized (lazy initialization for subscribers)
	if subscriberInstance == nil && initTrigger != nil {
		initTrigger()
	}

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
		metadataMap, err := fn(event)
		if err != nil {
			// Special Case: We want automated messages even if Metadata building fails.
			// Log the error for visibility but continue so the message is actually sent.
			log.Printf("ERROR: Failed to build message metadata for booking %s: %v", event.BookingID, err)
		} else if len(metadataMap) > 0 {
			bytes, err := json.Marshal(metadataMap)
			if err != nil {
				log.Printf("WARN: Failed to marshal message metadata for booking %s: %v", event.BookingID, err)
			} else {
				metadataJSON = bytes
			}
		}
	}

	data := buildTemplateData(event)

	// Create automated message
	now := time.Now()
	idempotencyKey := fmt.Sprintf("auto-%s-%s", event.BookingID, event.Status)
	// Check for quote_id in the enriched data map (potentially from RawData)
	if quoteID, ok := data["quote_id"].(string); ok && quoteID != "" {
		idempotencyKey = fmt.Sprintf("auto-%s-%s-%s", event.BookingID, event.Status, quoteID)
	}

	msg := &domain.Message{
		ID:             uuid.NewString(),
		ThreadID:       thread.ID,
		SenderID:       SystemSenderID, // Special system sender
		Content:        template.ContentFunc(data),
		MessageType:    template.MessageType,
		Status:         domain.MessageSent,
		SentAt:         &now,
		IdempotencyKey: idempotencyKey,
		Metadata:       metadataJSON, // ✅ Now includes useful IDs
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Save to database
	if err := s.messageRepo.Create(ctx, msg); err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}

	var metadata json.RawMessage
	if len(msg.Metadata) > 0 {
		metadata = json.RawMessage(msg.Metadata)
	}

	// Broadcast to WebSocket (if users are connected)
	s.wsManager.Broadcast(&handlers.WSMessage{
		ID:             msg.ID,
		ThreadID:       msg.ThreadID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		MessageType:    string(msg.MessageType),
		Status:         string(msg.Status),
		IdempotencyKey: msg.IdempotencyKey,
		SentAt:         now,
		Metadata:       metadata,
		Type:           "message",
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

	// 1. If RawData is present, unmarshal it first (contains rich typed data)
	if len(event.RawData) > 0 {
		var rawMap map[string]interface{}
		if err := json.Unmarshal(event.RawData, &rawMap); err == nil {
			for k, v := range rawMap {
				data[k] = v
			}
		} else {
			log.Printf("WARN: Failed to unmarshal RawData for booking %s: %v", event.BookingID, err)
		}
	}

	// 2. Add/Override with Metadata fields
	for k, v := range event.Metadata {
		data[k] = v
	}

	// 3. Normalized computed fields
	if event.ArtisanID != nil {
		data["artisan_id"] = *event.ArtisanID
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

	// Parse amount if present (check data map which includes RawData unmarshaled fields)
	if _, ok := data["amount"]; !ok {
		// Fallback to amount_cents if present (from RawData)
		if cents, ok := data["amount_cents"]; ok {
			switch v := cents.(type) {
			case float64:
				data["amount"] = v / 100.0
			case int64:
				data["amount"] = float64(v) / 100.0
			case int:
				data["amount"] = float64(v) / 100.0
			case json.Number:
				if f, err := v.Float64(); err == nil {
					data["amount"] = f / 100.0
				} else {
					log.Printf("WARN: Failed to parse amount_cents as float for booking %s: %v", event.BookingID, err)
					data["amount"] = float64(0)
				}
			}
		} else {
			data["amount"] = float64(0)
		}
	} else {
		// If amount exists as string (from Metadata), convert to float
		if str, ok := data["amount"].(string); ok {
			if parsed, err := strconv.ParseFloat(str, 64); err == nil {
				data["amount"] = parsed
			} else {
				log.Printf("WARN: Failed to parse amount string '%s' for booking %s: %v", str, event.BookingID, err)
				data["amount"] = float64(0)
			}
		}
	}

	// Add currency (default to NGN, check data map)
	if _, ok := data["currency"]; !ok {
		data["currency"] = "NGN"
	}

	// Add reason if present
	if event.Reason != nil {
		data["reason"] = *event.Reason
	} else if _, ok := data["reason"]; !ok {
		data["reason"] = ""
	}

	// Parse ETA minutes if present
	if _, ok := data["eta_minutes"]; !ok {
		data["eta_minutes"] = 0
	} else {
		if str, ok := data["eta_minutes"].(string); ok {
			if parsed, err := strconv.Atoi(str); err == nil {
				data["eta_minutes"] = parsed
			} else {
				log.Printf("WARN: Failed to parse eta_minutes '%s' for booking %s: %v", str, event.BookingID, err)
				data["eta_minutes"] = 0
			}
		}
	}

	// Parse scheduled time if provided
	if val, ok := data["scheduled_at"]; ok {
		if str, ok := val.(string); ok {
			if ts, err := time.Parse(time.RFC3339, str); err == nil {
				data["scheduled_at"] = ts
			} else {
				log.Printf("WARN: Failed to parse scheduled_at '%s' for booking %s: %v", str, event.BookingID, err)
			}
		}
	}

	// Determine who cancelled
	if event.Status == bookingdomain.BookingCancelled {
		if _, ok := data["cancelled_by"]; !ok {
			data["cancelled_by"] = "the customer"
		}
	}

	return data
}
