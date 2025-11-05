package tests

import (
	"context"
	"testing"
	"time"

	"encore.app/chat/domain"
	"encore.app/chat/repository"
	"encore.dev/et"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMessageRepository_CreateWithIdempotency(t *testing.T) {
	t.Run("create message successfully", func(t *testing.T) {
		ctx, repo, _, thread, cleanup := setupMessageRepositoryTest(t)
		defer cleanup()

		msg := newTestMessage(thread.ID, uuid.NewString(), "Hello, world", "key-create", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, msg))

		stored, err := repo.GetByIdempotencyKey(ctx, "key-create")
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.Equal(t, "Hello, world", stored.Content)
		assert.Equal(t, domain.MessageSent, stored.Status)
		assert.NotEmpty(t, stored.ID)
	})

	t.Run("duplicate idempotency key returns error", func(t *testing.T) {
		ctx, repo, _, thread, cleanup := setupMessageRepositoryTest(t)
		defer cleanup()

		first := newTestMessage(thread.ID, uuid.NewString(), "First", "dup-key", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, first))

		duplicate := newTestMessage(thread.ID, uuid.NewString(), "Duplicate", "dup-key", domain.MessageSent)
		err := repo.Create(ctx, duplicate)
		require.Error(t, err)
		require.ErrorIs(t, err, domain.ErrDuplicateMessage)
	})
}

func TestMessageRepository_Update_OptimisticLocking(t *testing.T) {
	t.Run("update succeeds with correct version", func(t *testing.T) {
		ctx, repo, _, thread, cleanup := setupMessageRepositoryTest(t)
		defer cleanup()

		orig := newTestMessage(thread.ID, uuid.NewString(), "Pending delivery", "update-success", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, orig))

		saved, err := repo.GetByIdempotencyKey(ctx, "update-success")
		require.NoError(t, err)
		require.NotNil(t, saved)

		deliveredAt := time.Now().UTC()
		saved.Status = domain.MessageDelivered
		saved.DeliveredAt = &deliveredAt
		saved.UpdatedAt = deliveredAt

		require.NoError(t, repo.Update(ctx, saved))
		assert.Equal(t, int64(2), saved.DBVersion)

		reloaded, err := repo.GetByID(ctx, saved.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.MessageDelivered, reloaded.Status)
		assert.NotNil(t, reloaded.DeliveredAt)
		assert.Equal(t, int64(2), reloaded.DBVersion)
	})

	t.Run("update fails with stale version", func(t *testing.T) {
		ctx, repo, _, thread, cleanup := setupMessageRepositoryTest(t)
		defer cleanup()

		orig := newTestMessage(thread.ID, uuid.NewString(), "Needs optimistic locking", "update-stale", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, orig))

		saved, err := repo.GetByIdempotencyKey(ctx, "update-stale")
		require.NoError(t, err)
		require.NotNil(t, saved)

		deliveredAt := time.Now().UTC()
		saved.Status = domain.MessageDelivered
		saved.DeliveredAt = &deliveredAt
		saved.UpdatedAt = deliveredAt
		require.NoError(t, repo.Update(ctx, saved))

		stale := *saved
		stale.DBVersion = 1
		readAt := time.Now().Add(2 * time.Second).UTC()
		stale.Status = domain.MessageRead
		stale.ReadAt = &readAt
		stale.UpdatedAt = readAt

		err = repo.Update(ctx, &stale)
		require.Error(t, err)

		var optimisticErr *domain.ErrOptimisticLockFailure
		require.ErrorAs(t, err, &optimisticErr)

		reloaded, err := repo.GetByID(ctx, saved.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.MessageDelivered, reloaded.Status)
		assert.Equal(t, int64(2), reloaded.DBVersion)
	})
}

func TestMessageRepository_MarkThreadAsRead(t *testing.T) {
	t.Run("marks only unread messages as read", func(t *testing.T) {
		ctx, repo, _, thread, cleanup := setupMessageRepositoryTest(t)
		defer cleanup()

		recipientID := uuid.NewString()
		otherSenderID := uuid.NewString()

		unread := newTestMessage(thread.ID, otherSenderID, "Incoming message", "mark-read-target", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, unread))

		selfMessage := newTestMessage(thread.ID, recipientID, "Own message", "mark-read-self", domain.MessageSent)
		require.NoError(t, repo.Create(ctx, selfMessage))

		alreadyRead := newTestMessage(thread.ID, otherSenderID, "Already read", "mark-read-existing", domain.MessageRead)
		initialReadAt := time.Now().Add(-1 * time.Minute).UTC()
		alreadyRead.ReadAt = &initialReadAt
		require.NoError(t, repo.Create(ctx, alreadyRead))

		require.NoError(t, repo.MarkThreadAsRead(ctx, thread.ID, recipientID))

		messages, err := repo.GetByThreadID(ctx, thread.ID, 10, 0)
		require.NoError(t, err)

		msgByKey := map[string]*domain.Message{}
		for _, m := range messages {
			msgByKey[m.IdempotencyKey] = m
		}

		updatedUnread := msgByKey["mark-read-target"]
		require.NotNil(t, updatedUnread)
		assert.Equal(t, domain.MessageRead, updatedUnread.Status)
		require.NotNil(t, updatedUnread.ReadAt)

		selfResult := msgByKey["mark-read-self"]
		require.NotNil(t, selfResult)
		assert.Equal(t, domain.MessageSent, selfResult.Status)
		assert.Nil(t, selfResult.ReadAt)

		alreadyReadResult := msgByKey["mark-read-existing"]
		require.NotNil(t, alreadyReadResult)
		assert.Equal(t, domain.MessageRead, alreadyReadResult.Status)
		require.NotNil(t, alreadyReadResult.ReadAt)
		assert.WithinDuration(t, initialReadAt, *alreadyReadResult.ReadAt, time.Second)
	})
}

func setupMessageRepositoryTest(t *testing.T) (context.Context, domain.MessageRepository, *gorm.DB, *domain.Thread, func()) {
	t.Helper()
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "chat")
	require.NoError(t, err)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: testDB.Stdlib(),
	}), &gorm.Config{})
	require.NoError(t, err)

	repo := repository.NewMessageRepository(gormDB, gormDB)

	thread := &domain.Thread{
		ID:         uuid.NewString(),
		BookingID:  uuid.NewString(),
		CustomerID: uuid.NewString(),
		ArtisanID:  uuid.NewString(),
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	require.NoError(t, gormDB.WithContext(ctx).Create(thread).Error)

	cleanup := func() {}
	return ctx, repo, gormDB, thread, cleanup
}

func newTestMessage(threadID, senderID, content, key string, status domain.MessageStatus) *domain.Message {
	now := time.Now().UTC()
	return &domain.Message{
		ThreadID:       threadID,
		SenderID:       senderID,
		Content:        content,
		MessageType:    domain.MessageTypeUser,
		IdempotencyKey: key,
		Status:         status,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
