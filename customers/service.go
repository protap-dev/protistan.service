package customers

import (
	"context"
	"fmt"
	"strings"
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

type CustomerProfile struct {
	User      CustomerUserDTO   `json:"user"`
	Profile   user.UserProfile  `json:"profile"`
	Settings  user.UserSettings `json:"settings"`
	Addresses []CustomerAddress `json:"addresses"`
}

type CustomerUserDTO struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	EmailVerified   bool      `json:"email_verified"`
	RolesEnabled    []string  `json:"roles_enabled"`
	ActiveRole      *string   `json:"active_role"`
	ProfileComplete bool      `json:"profile_complete"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	Country       *string `json:"country"` // Changed to pointer to allow nil for default
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

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "get_profile", userIDStr)

	userUUID, err := uuid.FromString(userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "invalid_uuid_format", err)
		return nil, ErrUnauthenticated
	}

	// Call user service using consolidated internal API
	resp, err := user.FetchInternal(ctx, &user.InternalUserFetchRequest{
		UserID: userUUID,

		IncludeUser:     true,
		IncludeProfile:  true,
		IncludeSettings: true,

		EnsureProfile:  true, // Create profile if it doesn't exist
		EnsureSettings: true, // Ensure default settings are created
	})
	if err != nil {
		s.logger.LogError(ctx, "fetch_user_internal", err)
		return nil, err
	}
	if resp.User == nil || resp.Profile == nil || resp.Settings == nil {
		return nil, ErrUnauthenticated
	}

	// Fetch customer-specific addresses
	addresses, err := s.addressRepo.GetByUserID(ctx, userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "get_addresses", err)
		return nil, ErrDatabaseError
	}

	return &CustomerProfile{
		User: CustomerUserDTO{
			ID:              resp.User.ID,
			Email:           resp.User.Email,
			EmailVerified:   resp.User.EmailVerified,
			RolesEnabled:    []string(resp.User.Roles),
			ActiveRole:      resp.User.ActiveRole,
			ProfileComplete: resp.User.ProfileComplete,
			CreatedAt:       resp.User.CreatedAt,
			UpdatedAt:       resp.User.UpdatedAt,
		},
		Profile:   *resp.Profile,
		Settings:  *resp.Settings,
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

	// Normalize label to lowercase before validation and storage
	req.Label = strings.ToLower(req.Label)

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
			Coordinates:   formatPoint(req.Longitude, req.Latitude),
			IsDefault:     req.IsDefault,
		}

		// Access the underlying transactional DB from the repository
		dbRepo, ok := repo.(*addressRepository)
		if !ok {
			return fmt.Errorf("internal error: failed to cast repository to concrete type")
		}
		txDB := dbRepo.db // Get the transactional *gorm.DB instance

		var createOperation *gorm.DB
		if req.Country != nil {
			address.Country = *req.Country // Assign the provided country
			createOperation = txDB.Create(address)
		} else {
			// If country is not provided, omit it from the insert to use DB default
			createOperation = txDB.Omit("country").Create(address)
		}

		if createOperation.Error != nil {
			s.logger.LogError(ctx, "create_address", createOperation.Error)
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
			lowerCaseLabel := strings.ToLower(*req.Label)
			updates["label"] = lowerCaseLabel
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
// INTERNAL APIs (for service-to-service calls)
// ============================================================================

// GetAddressByID - Internal API for other services
//
// ValidateAddressOwnership - Internal API to validate address belongs to user
//
//encore:api private method=GET path=/internal/customers/addresses/:addressID/validate/:userID
func (s *Service) ValidateAddressOwnership(ctx context.Context, addressID uuid.UUID, userID uuid.UUID) (*CustomerAddress, error) {
	// Get all addresses for the user
	addresses, err := s.addressRepo.GetByUserID(ctx, userID.String())
	if err != nil {
		s.logger.LogError(ctx, "get_user_addresses", err)
		return nil, err
	}

	// Check if the address ID is in the user's addresses
	addressIDStr := addressID.String()
	for _, addr := range addresses {
		if addr.ID == addressIDStr {
			return &addr, nil
		}
	}

	// Address not found in user's addresses
	s.logger.LogError(ctx, "address_not_owned_by_user", fmt.Errorf("address %s not owned by user %s", addressIDStr, userID.String()))
	return nil, ErrAddressNotFound
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func formatPoint(lon, lat float64) string {
	// PostgreSQL POINT format: (longitude, latitude)
	return "(" + fmt.Sprintf("%.8f", lon) + "," + fmt.Sprintf("%.8f", lat) + ")"
}
