package domain

import "time"

// BookingStatus represents the state of a booking
type BookingStatus string

const (
	BookingRequested      BookingStatus = "requested"
	BookingOfferPending   BookingStatus = "offer_pending"
	BookingOfferRejected  BookingStatus = "offer_rejected"
	BookingAssigned       BookingStatus = "assigned"
	BookingPendingQuote   BookingStatus = "pending_quote"
	BookingQuoteProposed  BookingStatus = "quote_proposed"
	BookingQuoteAccepted  BookingStatus = "quote_accepted"
	BookingPaymentPending BookingStatus = "payment_pending"
	BookingConfirmed      BookingStatus = "confirmed"
	BookingEnroute        BookingStatus = "enroute"
	BookingInProgress     BookingStatus = "in_progress"
	BookingCompleted      BookingStatus = "completed"
	BookingCancelled      BookingStatus = "cancelled"
	BookingClosed         BookingStatus = "closed"
)

// Valid status transitions
var validTransitions = map[BookingStatus]map[BookingStatus]bool{
	BookingRequested:      {BookingOfferPending: true, BookingCancelled: true},
	BookingOfferPending:   {BookingAssigned: true, BookingOfferRejected: true, BookingCancelled: true},
	BookingOfferRejected:  {BookingOfferPending: true, BookingCancelled: true},
	BookingAssigned:       {BookingPendingQuote: true, BookingCancelled: true},
	BookingPendingQuote:   {BookingQuoteProposed: true, BookingCancelled: true},
	BookingQuoteProposed:  {BookingQuoteAccepted: true, BookingCancelled: true},
	BookingQuoteAccepted:  {BookingPaymentPending: true, BookingCancelled: true},
	BookingPaymentPending: {BookingConfirmed: true, BookingCancelled: true},
	BookingConfirmed:      {BookingEnroute: true, BookingCancelled: true},
	BookingEnroute:        {BookingInProgress: true, BookingCancelled: true},
	BookingInProgress:     {BookingCompleted: true, BookingCancelled: true},
	BookingCompleted:      {BookingClosed: true},
	// No transitions from cancelled/closed
}

// Booking represents a booking entity
type Booking struct {
	ID                    string            `json:"id"`
	CustomerID            string            `json:"customer_id"`
	ArtisanID             *string           `json:"artisan_id,omitempty"`
	ServiceCategoryID     string            `json:"service_category_id"`
	Title                 string            `json:"title"`
	Description           string            `json:"description,omitempty"`
	CustomerAddressID     string            `json:"customer_address_id"`
	Status                BookingStatus     `json:"status"`
	Priority              string            `json:"priority"`
	ScheduledAt           *time.Time        `json:"scheduled_at,omitempty"`
	EstimatedDurationMins int               `json:"estimated_duration_mins,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	
	// Offer tracking
	IsSpecificArtisan bool `json:"is_specific_artisan"` // true if customer requested specific artisan
	OffersCount       int  `json:"offers_count"`        // Total offers made
	
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int64     `json:"version"` // For optimistic locking
}

// CanTransition checks if a status transition is valid
func CanTransition(from, to BookingStatus) bool {
	if transitions, exists := validTransitions[from]; exists {
		return transitions[to]
	}
	return false
}
