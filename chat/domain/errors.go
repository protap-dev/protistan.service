package domain

import "fmt"

// ChatError represents a domain error
type ChatError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *ChatError) Error() string {
	return e.Message
}

// Predefined errors
var (
	ErrThreadNotFound      = &ChatError{Code: "thread_not_found", Message: "thread not found"}
	ErrMessageNotFound     = &ChatError{Code: "message_not_found", Message: "message not found"}
	ErrInvalidTransition   = &ChatError{Code: "invalid_transition", Message: "invalid status transition"}
	ErrEmptyContent        = &ChatError{Code: "empty_content", Message: "message content cannot be empty"}
	ErrInvalidParticipant  = &ChatError{Code: "invalid_participant", Message: "sender is not a thread participant"}
	ErrDuplicateMessage    = &ChatError{Code: "duplicate_message", Message: "message with this idempotency key already exists"}
	ErrThreadAlreadyExists = &ChatError{Code: "thread_already_exists", Message: "thread for this booking already exists"}
)

// ErrOptimisticLockFailure represents optimistic locking failure
type ErrOptimisticLockFailure struct {
	EntityID string
}

func (e *ErrOptimisticLockFailure) Error() string {
	return fmt.Sprintf("optimistic lock failure for message: %s", e.EntityID)
}
