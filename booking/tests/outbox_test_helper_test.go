package booking

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func ensureOutboxTable(t *testing.T, db *gorm.DB) {
	t.Helper()

	err := db.Exec(`
		CREATE TABLE IF NOT EXISTS outbox (
			id UUID PRIMARY KEY DEFAULT generate_uuid(),
			topic TEXT NOT NULL,
			data JSONB NOT NULL,
			inserted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			processed_at TIMESTAMPTZ,
			retry_count INTEGER DEFAULT 0,
			next_retry_at TIMESTAMPTZ,
			last_error TEXT,
			status TEXT DEFAULT 'pending'
		)
	`).Error
	require.NoError(t, err)
}
