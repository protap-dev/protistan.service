package chat

import (
	"context"
	"net/http"
	"sync"

	"encore.app/chat/domain"
	"encore.app/chat/events"
	"encore.app/chat/handlers"
	"encore.app/chat/internal"
	"encore.app/chat/relay"
	"encore.app/chat/repository"
	"encore.app/core"
	coredb "encore.app/core/db"
	corerelay "encore.app/core/relay"
	"encore.dev"
	"encore.dev/storage/sqldb"
	"github.com/gorilla/websocket"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	MessagesHandler *handlers.MessagesHandler
	ThreadsHandler  *handlers.ThreadsHandler
	relay           *relay.OutboxRelay
}

var ChatDB = sqldb.NewDatabase("chat", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

var serviceInstance *Service
var serviceOnce sync.Once

func initService() (*Service, error) {
	var initErr error

	serviceOnce.Do(func() {
		// Initialize GORM connections
		ChatgormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: ChatDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		coreGormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: coredb.ProtisanDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		// Initialize dependencies
		logger := internal.NewServiceLogger("chat")
		authHelper := internal.NewAuthHelper(logger)
		validator := domain.NewChatValidator()
		threadRepo := repository.NewThreadRepository(ChatgormDB)
		messageRepo := repository.NewMessageRepository(ChatgormDB, coreGormDB)
		coreSvc := core.NewCoreService(ChatgormDB)
		publisher := events.NewEventPublisher()

		// Initialize handlers
		MessagesHandler := handlers.NewMessagesHandler(
			threadRepo,
			messageRepo,
			validator,
			logger,
			coreSvc,
			authHelper,
			publisher,
			handlers.GetWSManager(),
		)

		ThreadsHandler := handlers.NewThreadsHandler(
			threadRepo,
			messageRepo,
			logger,
			authHelper,
			validator,
		)

		events.SetSubscriberDependencies(
			threadRepo,
			messageRepo,
			validator,
			publisher,
			handlers.GetWSManager(),
		)

		// Initialize relay
		relayConfig := corerelay.Config{
			PollingInterval: internal.DefaultOutboxRelayConfig().PollingInterval,
			BatchSize:       internal.DefaultOutboxRelayConfig().BatchSize,
			MaxRetries:      internal.DefaultOutboxRelayConfig().MaxRetries,
			RetryBaseDelay:  internal.DefaultOutboxRelayConfig().RetryBaseDelay,
			RetryMaxDelay:   internal.DefaultOutboxRelayConfig().RetryMaxDelay,
			AuditRetention:  internal.DefaultOutboxRelayConfig().AuditRetention,
		}
		relayInstance := relay.NewOutboxRelay(coreGormDB, relayConfig)
		relayInstance.StartAsync(context.Background())

		serviceInstance = &Service{
			MessagesHandler: MessagesHandler,
			ThreadsHandler:  ThreadsHandler,
			relay:           relayInstance,
		}
	})

	return serviceInstance, initErr
}

// StartRelay allows external callers to start the outbox relay with a custom context.
func StartRelay(ctx context.Context) error {
	svc, err := initService()
	if err != nil {
		return err
	}
	svc.relay.StartAsync(ctx)
	return nil
}

// StopRelay gracefully stops the outbox relay if it is running.
func StopRelay() {
	if serviceInstance == nil || serviceInstance.relay == nil {
		return
	}
	serviceInstance.relay.Stop()
}

// SendMessage sends a message in a thread
//
//encore:api auth method=POST path=/v0/chat/threads/:threadId/messages
func SendMessage(ctx context.Context, threadId string, req *handlers.SendMessageRequest) (*handlers.MessageResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.MessagesHandler.SendMessage(ctx, threadId, req)
}

// GetMessages retrieves messages from a thread
//
//encore:api auth method=GET path=/v0/chat/threads/:threadId/messages
func GetMessages(ctx context.Context, threadId string) (*handlers.MessagesResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	params := &handlers.ListMessagesRequest{Limit: 50, Offset: 0}
	return svc.MessagesHandler.GetMessages(ctx, threadId, params)
}

// MarkThreadAsRead marks thread messages as read
//
//encore:api auth method=PUT path=/v0/chat/threads/:threadId/read
func MarkThreadAsRead(ctx context.Context, threadId string) error {
	svc, err := initService()
	if err != nil {
		return err
	}
	return svc.MessagesHandler.MarkThreadAsRead(ctx, threadId)
}

// ListThreads lists threads for the authenticated user
//
//encore:api auth method=GET path=/v0/chat/threads
func ListThreads(ctx context.Context) (*handlers.ThreadsResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	params := &handlers.ListThreadsRequest{Limit: 50, Offset: 0}
	return svc.ThreadsHandler.ListThreads(ctx, params)
}

// ============================================================================
// WEBSOCKET ENDPOINT (Raw HTTP)
// ============================================================================

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, validate origin properly
		return true
	},
}

// StreamMessages establishes WebSocket connection for real-time messaging
//
//encore:api auth raw method=GET path=/v0/chat/threads/:threadId/stream
func StreamMessages(w http.ResponseWriter, req *http.Request) {
	svc, err := initService()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	threadId := encore.CurrentRequest().PathParams.Get("threadId")
	if threadId == "" {
		http.Error(w, "Missing threadId", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		http.Error(w, "WebSocket upgrade failed", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	// Extract user context (Encore provides this via auth middleware)
	ctx := req.Context()

	// Call handler to manage WebSocket connection
	svc.MessagesHandler.HandleWebSocketConnection(ctx, threadId, conn)
}
