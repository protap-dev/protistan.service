package admin

import (
	"context"
	"time"

	"encore.app/core"
	"encore.app/core/db"
	"encore.app/user"
	"encore.dev/beta/auth"
	"encore.dev/types/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	validator AdminValidator
	logger    ServiceLogger
	coreSvc   *core.CoreService
}

// ============================================================================
// ADMIN SERVICE OVERVIEW
// ============================================================================
//
// This service provides administrative functions for managing users.
// All endpoints require admin privileges (user_type = "admin").
//
// SECURITY MODEL:

// Initialize service with dependency injection
func initService() (*Service, error) {
	// Initialize GORM connection using Encore's database
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db.ProtisanDB.Stdlib(),
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Initialize core service for shared infrastructure
	coreSvc := core.NewCoreService(gormDB)

	return &Service{
		validator: NewAdminValidator(),
		logger:    NewServiceLogger(),
		coreSvc:   coreSvc,
	}, nil
}

// ============================================================================
// DOMAIN MODELS

// User represents the core user data that admins can modify
type User struct {
	ID              string    `json:"id" gorm:"primarykey;type:uuid"`
	Email           string    `json:"email" gorm:"index;unique;not null;type:varchar(255)"`
	EmailVerified   bool      `json:"email_verified" gorm:"default:false"`
	UserType        string    `json:"user_type" gorm:"default:'customer'"`
	ProfileComplete bool      `json:"profile_complete" gorm:"default:false"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// ============================================================================
// API REQUEST/RESPONSE TYPES
// ============================================================================

type UpdateUserCoreRequest struct {
	UserType        *string `json:"user_type,omitempty"`        // Optional: 'customer', 'artisan', 'admin'
	EmailVerified   *bool   `json:"email_verified,omitempty"`   // Optional: manual verification
	ProfileComplete *bool   `json:"profile_complete,omitempty"` // Optional: admin override
}

// ============================================================================
// PUBLIC APIs
// ============================================================================

// UpdateUserCore - Admin-only API to update sensitive user fields
//
//encore:api auth method=PUT path=/v0/admin/users/:userID/core
func (s *Service) UpdateUserCore(ctx context.Context, userID uuid.UUID, req *UpdateUserCoreRequest) (*User, error) {
	// Verify requesting user is an admin
	requestingUserID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	requestingUserIDStr := string(requestingUserID)
	s.logger.LogAdminAction(ctx, "update_user_core_attempt", requestingUserIDStr, userID.String())

	// Get requesting user's info to verify admin status
	requestingUserUUID, err := uuid.FromString(requestingUserIDStr)
	if err != nil {
		s.logger.LogError(ctx, "invalid_requesting_user_uuid", err)
		return nil, ErrUnauthenticated
	}

	requestingUser, err := user.GetUserByID(ctx, requestingUserUUID)
	if err != nil {
		s.logger.LogError(ctx, "get_requesting_user", err)
		return nil, err
	}

	// Verify requesting user is an admin
	if requestingUser.UserType != "admin" {
		s.logger.LogError(ctx, "unauthorized_admin_access", nil)
		return nil, ErrUnauthorizedAction
	}

	// Validate the update request
	if err := s.validator.ValidateUpdateUserCoreRequest(req); err != nil {
		s.logger.LogError(ctx, "validate_update_request", err)
		return nil, err
	}

	// Use transaction for atomic user core updates
	var updatedUser *User
	err = core.WithTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) UserRepository {
		return NewUserRepository(db)
	}, func(userRepo UserRepository) error {
		// Get target user to verify they exist
		targetUser, err := userRepo.GetByID(ctx, userID.String())
		if err != nil {
			s.logger.LogError(ctx, "get_target_user", err)
			return err
		}

		// Prevent admin from modifying their own admin status (security measure)
		if targetUser.ID == requestingUserIDStr && req.UserType != nil && *req.UserType != "admin" {
			s.logger.LogError(ctx, "admin_self_demotion_attempt", nil)
			return ErrCannotDemoteSelf
		}

		// Prepare updates map
		updates := make(map[string]any)

		if req.UserType != nil {
			updates["user_type"] = *req.UserType
		}
		if req.EmailVerified != nil {
			updates["email_verified"] = *req.EmailVerified
		}
		if req.ProfileComplete != nil {
			updates["profile_complete"] = *req.ProfileComplete
		}

		// Apply updates
		if err := userRepo.UpdateCore(ctx, userID.String(), updates); err != nil {
			s.logger.LogError(ctx, "update_user_core", err)
			return ErrDatabaseError
		}

		// Fetch updated user within transaction
		updatedUser, err = userRepo.GetByID(ctx, userID.String())
		if err != nil {
			s.logger.LogError(ctx, "get_updated_user", err)
			return ErrDatabaseError
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_user_core_transaction", err)
		return nil, ErrDatabaseError
	}

	s.logger.LogAdminAction(ctx, "update_user_core_success", requestingUserIDStr, userID.String())

	return updatedUser, nil
}

// ============================================================================
// PRIVATE HELPER METHODS
// ============================================================================
