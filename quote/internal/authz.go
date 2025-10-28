package internal

import (
	"context"

	"encore.app/quote/domain"
)

// TODO: Implement proper authorization without creating import cycles
// For now, allow all authenticated users to access quotes
func AuthorizeQuoteAccess(ctx context.Context, role string, userID string, quote *domain.Quote) error {
	// Temporary implementation - always allow access
	// In production, this should verify:
	// - Customer owns the booking (for customer role)
	// - Artisan proposed the quote (for artisan role)
	// - User has admin privileges (for admin role)

	switch role {
	case "customer", "artisan", "admin":
		return nil
	default:
		return ErrPermissionDenied
	}
}
