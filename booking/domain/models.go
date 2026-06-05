package domain

import "time"

// BookingStatus represents the state of a booking
type BookingStatus string

const (
	BookingRequested         BookingStatus = "requested"
	BookingOfferPending      BookingStatus = "offer_pending"
	BookingOfferRejected     BookingStatus = "offer_rejected"
	BookingAssigned          BookingStatus = "assigned"
	BookingPendingQuote      BookingStatus = "pending_quote"
	BookingQuoteProposed     BookingStatus = "quote_proposed"
	BookingQuoteAccepted     BookingStatus = "quote_accepted"
	BookingPaymentPending    BookingStatus = "payment_pending"
	BookingConfirmed         BookingStatus = "confirmed"
	BookingEnroute           BookingStatus = "enroute"
	BookingInProgress        BookingStatus = "in_progress"
	BookingCompletionPending BookingStatus = "completion_pending"
	BookingCompleted         BookingStatus = "completed"
	BookingCancelled         BookingStatus = "cancelled"
	BookingClosed            BookingStatus = "closed"
)

// Valid status transitions
var validTransitions = map[BookingStatus]map[BookingStatus]bool{
	BookingRequested:         {BookingOfferPending: true, BookingCancelled: true},
	BookingOfferPending:      {BookingAssigned: true, BookingOfferRejected: true, BookingCancelled: true},
	BookingOfferRejected:     {BookingOfferPending: true, BookingCancelled: true},
	BookingAssigned:          {BookingPendingQuote: true, BookingCancelled: true},
	BookingPendingQuote:      {BookingQuoteProposed: true, BookingCancelled: true},
	BookingQuoteProposed:     {BookingQuoteAccepted: true, BookingCancelled: true},
	BookingQuoteAccepted:     {BookingPaymentPending: true, BookingCancelled: true},
	BookingPaymentPending:    {BookingConfirmed: true, BookingCancelled: true, BookingQuoteAccepted: true, BookingAssigned: true},
	BookingConfirmed:         {BookingEnroute: true, BookingCancelled: true},
	BookingEnroute:           {BookingInProgress: true, BookingCancelled: true},
	BookingInProgress:        {BookingCompletionPending: true},
	BookingCompletionPending: {BookingCompleted: true},
	BookingCompleted:         {BookingClosed: true},
	// No transitions from cancelled/closed
}

// Booking represents a booking entity
type Booking struct {
	ID                    string            `json:"id"`
	CustomerID            string            `json:"customer_id"`
	ArtisanID             *string           `json:"artisan_id,omitempty"`
	ServiceCategoryID     string            `json:"service_category_id"`
	ServiceID             string            `json:"service_id"`
	Title                 string            `json:"title"`
	Description           string            `json:"description,omitempty"`
	CustomerAddressID     string            `json:"customer_address_id"`
	Status                BookingStatus     `json:"status"`
	Priority              string            `json:"priority"`
	ScheduledAt           *time.Time        `json:"scheduled_at,omitempty"`
	EstimatedDurationMins int               `json:"estimated_duration_mins,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	MediaURLs             []string          `json:"media_urls"`
	IsFlexible            bool              `json:"is_flexible"`

	// Offer tracking
	IsSpecificArtisan bool `json:"is_specific_artisan"` // true if customer requested specific artisan
	OffersCount       int  `json:"offers_count"`        // Total offers made

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"` // For optimistic locking
}

// IsValidBookingStatus checks whether a status is a known booking status.
func IsValidBookingStatus(status BookingStatus) bool {
	switch status {
	case BookingRequested,
		BookingOfferPending,
		BookingOfferRejected,
		BookingAssigned,
		BookingPendingQuote,
		BookingQuoteProposed,
		BookingQuoteAccepted,
		BookingPaymentPending,
		BookingConfirmed,
		BookingEnroute,
		BookingInProgress,
		BookingCompletionPending,
		BookingCompleted,
		BookingCancelled,
		BookingClosed:
		return true
	default:
		return false
	}
}

// CanRematchFromStatus checks whether a booking status is eligible for customer rematch.
func CanRematchFromStatus(status BookingStatus) bool {
	switch status {
	case BookingOfferPending,
		BookingOfferRejected,
		BookingAssigned,
		BookingPendingQuote:
		return true
	default:
		return false
	}
}

// CanTransition checks if a status transition is valid
func CanTransition(from, to BookingStatus) bool {
	if transitions, exists := validTransitions[from]; exists {
		return transitions[to]
	}
	return false
}
