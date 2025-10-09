package customers

import (
	"context"
	"fmt"
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
	addressRepo AddressRepository
	validator   AddressValidator
	logger      ServiceLogger
	coreSvc     *core.CoreService // Core service for shared infrastructure
}

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
		addressRepo: NewAddressRepository(coreSvc.DB()),
		validator:   NewAddressValidator(),
		logger:      NewServiceLogger(),
		coreSvc:     coreSvc,
	}, nil
}

// ============================================================================
// DOMAIN MODELS
// ============================================================================

// Customer address model
type CustomerAddress struct {
	ID            string    `json:"id" gorm:"primarykey;type:uuid;default:generate_uuid()"`
	UserID        string    `json:"user_id"`
	Label         string    `json:"label"`
	StreetAddress string    `json:"street_address"`
	City          string    `json:"city"`
	State         string    `json:"state"`
	PostalCode    string    `json:"postal_code"`
	Country       string    `json:"country"`
	Coordinates   string    `json:"coordinates" gorm:"type:point"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Complete customer profile (user + addresses)
type CustomerProfile struct {
	User      user.User         `json:"user"`
	Profile   user.UserProfile  `json:"profile"`
	Settings  user.UserSettings `json:"settings"`
	Addresses []CustomerAddress `json:"addresses"`
}

// ============================================================================
// API REQUEST/RESPONSE TYPES
// ============================================================================

type AddAddressRequest struct {
	Label         string  `json:"label"`
	StreetAddress string  `json:"street_address"`
	City          string  `json:"city"`
	State         string  `json:"state"`
	PostalCode    string  `json:"postal_code"`
	Country       string  `json:"country"`
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	IsDefault     bool    `json:"is_default"`
}

type UpdateAddressRequest struct {
	AddressID     string   `json:"address_id"`
	Label         *string  `json:"label,omitempty"`
	StreetAddress *string  `json:"street_address,omitempty"`
	City          *string  `json:"city,omitempty"`
	State         *string  `json:"state,omitempty"`
	PostalCode    *string  `json:"postal_code,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	IsDefault     *bool    `json:"is_default,omitempty"`
}

// List addresses response
type ListAddressesResponse struct {
	Addresses []CustomerAddress `json:"addresses"`
}

// ============================================================================
// PUBLIC APIs
// ============================================================================

// Get complete customer profile (user data + addresses)
//
//encore:api auth method=GET path=/v0/customers/profile
func (s *Service) GetProfile(ctx context.Context) (*CustomerProfile, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	// Convert UUID to string properly for service-to-service calls
	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "get_profile", userIDStr)

	// Convert string to UUID for service call
	userUUID, err := uuid.FromString(userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "invalid_uuid_format", err)
		return nil, ErrUnauthenticated
	}

	// Call user service for profile data (service-to-service RPC)
	completeProfile, err := user.GetCompleteProfileByUserID(ctx, userUUID)
	if err != nil {
		s.logger.LogError(ctx, "get_user_profile", err)
		return nil, err
	}

	// Fetch customer-specific addresses
	addresses, err := s.addressRepo.GetByUserID(ctx, userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "get_addresses", err)
		return nil, ErrDatabaseError
	}

	return &CustomerProfile{
		User:      completeProfile.User,
		Profile:   completeProfile.Profile,
		Settings:  completeProfile.Settings,
		Addresses: addresses,
	}, nil
}

// ============================================================================
// ADDRESS MANAGEMENT
// ============================================================================

// Add customer address
//
//encore:api auth method=POST path=/v0/customers/addresses
func (s *Service) AddAddress(ctx context.Context, req *AddAddressRequest) (*CustomerAddress, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "add_address", userIDStr)

	// Validation
	if err := s.validator.ValidateAddress(req); err != nil {
		s.logger.LogError(ctx, "validate_address", err)
		return nil, err
	}

	// Use transaction for atomic address creation and default logic
	var newAddress *CustomerAddress
	err := core.WithTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) AddressRepository {
		return NewAddressRepository(db)
	}, func(repo AddressRepository) error {
		// If this is default, unset other defaults first
		if req.IsDefault {
			if err := repo.UnsetDefaultAddresses(ctx, userIDStr, ""); err != nil {
				s.logger.LogError(ctx, "unset_default_addresses", err)
				return ErrDatabaseError
			}
		}

		// Create address
		address := &CustomerAddress{
			UserID:        userIDStr,
			Label:         req.Label,
			StreetAddress: req.StreetAddress,
			City:          req.City,
			State:         req.State,
			PostalCode:    req.PostalCode,
			Country:       req.Country,
			Coordinates:   formatPoint(req.Longitude, req.Latitude),
			IsDefault:     req.IsDefault,
		}

		if err := repo.Create(ctx, address); err != nil {
			s.logger.LogError(ctx, "create_address", err)
			return ErrDatabaseError
		}

		newAddress = address
		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "add_address_transaction", err)
		return nil, ErrDatabaseError
	}

	s.logger.LogAddressAction(ctx, "address_created", newAddress.ID, userIDStr)
	return newAddress, nil
}

// Update address
//
//encore:api auth method=PUT path=/v0/customers/addresses/:addressID
func (s *Service) UpdateAddress(ctx context.Context, addressID string, req *UpdateAddressRequest) (*CustomerAddress, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogAddressAction(ctx, "update_address", addressID, userIDStr)

	// Use transaction for atomic default address logic
	var updatedAddress *CustomerAddress
	err := core.WithTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) AddressRepository {
		return NewAddressRepository(db)
	}, func(repo AddressRepository) error {
		// Verify address exists and belongs to user
		_, err := repo.GetByID(ctx, addressID, userIDStr)
		if err != nil {
			s.logger.LogError(ctx, "get_address_for_update", err)
			return err
		}

		// Validate update request
		if err := s.validator.ValidateUpdateAddress(req); err != nil {
			s.logger.LogError(ctx, "validate_update_address", err)
			return err
		}

		// Build updates
		updates := make(map[string]any)
		if req.Label != nil {
			updates["label"] = *req.Label
		}
		if req.StreetAddress != nil {
			updates["street_address"] = *req.StreetAddress
		}
		if req.City != nil {
			updates["city"] = *req.City
		}
		if req.State != nil {
			updates["state"] = *req.State
		}
		if req.PostalCode != nil {
			updates["postal_code"] = *req.PostalCode
		}
		if req.Longitude != nil && req.Latitude != nil {
			updates["coordinates"] = formatPoint(*req.Longitude, *req.Latitude)
		}
		if req.IsDefault != nil && *req.IsDefault {
			// Atomic: Unset other defaults AND set current as default
			if err := repo.UnsetDefaultAddresses(ctx, userIDStr, addressID); err != nil {
				s.logger.LogError(ctx, "unset_default_addresses", err)
				return ErrDatabaseError
			}
			updates["is_default"] = true
		}

		if len(updates) > 0 {
			if err := repo.Update(ctx, addressID, userIDStr, updates); err != nil {
				s.logger.LogError(ctx, "update_address", err)
				return ErrDatabaseError
			}
		}

		// Reload updated address within transaction
		updatedAddress, err = repo.GetByID(ctx, addressID, userIDStr)
		if err != nil {
			s.logger.LogError(ctx, "reload_address", err)
			return ErrDatabaseError
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_address_transaction", err)
		return nil, ErrDatabaseError
	}

	s.logger.LogAddressAction(ctx, "address_updated", addressID, userIDStr)
	return updatedAddress, nil
}

// Delete address
//
//encore:api auth method=DELETE path=/v0/customers/addresses/:addressID
func (s *Service) DeleteAddress(ctx context.Context, addressID string) error {
	userID, ok := auth.UserID()
	if !ok {
		return ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogAddressAction(ctx, "delete_address", addressID, userIDStr)

	if err := s.addressRepo.Delete(ctx, addressID, userIDStr); err != nil {
		s.logger.LogError(ctx, "delete_address", err)
		return err
	}

	s.logger.LogAddressAction(ctx, "address_deleted", addressID, userIDStr)
	return nil
}

// List addresses
//
//encore:api auth method=GET path=/v0/customers/addresses
func (s *Service) ListAddresses(ctx context.Context) (*ListAddressesResponse, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "list_addresses", userIDStr)

	addresses, err := s.addressRepo.GetByUserID(ctx, userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "list_addresses", err)
		return nil, ErrDatabaseError
	}

	return &ListAddressesResponse{
		Addresses: addresses,
	}, nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func formatPoint(lon, lat float64) string {
	// PostgreSQL POINT format: (longitude, latitude)
	return "(" + fmt.Sprintf("%.8f", lon) + "," + fmt.Sprintf("%.8f", lat) + ")"
}
