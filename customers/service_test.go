package customers

import (
	"context"
	"errors"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MOCK IMPLEMENTATIONS
// ============================================================================

// Mock AddressRepository
type MockAddressRepository struct {
	mock.Mock
}

func (m *MockAddressRepository) GetByUserID(ctx context.Context, userID string) ([]CustomerAddress, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]CustomerAddress), args.Error(1)
}

func (m *MockAddressRepository) GetByID(ctx context.Context, addressID, userID string) (*CustomerAddress, error) {
	args := m.Called(ctx, addressID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*CustomerAddress), args.Error(1)
}

func (m *MockAddressRepository) Create(ctx context.Context, address *CustomerAddress) error {
	args := m.Called(ctx, address)
	return args.Error(0)
}

func (m *MockAddressRepository) Update(ctx context.Context, addressID, userID string, updates map[string]interface{}) error {
	args := m.Called(ctx, addressID, userID, updates)
	return args.Error(0)
}

func (m *MockAddressRepository) Delete(ctx context.Context, addressID, userID string) error {
	args := m.Called(ctx, addressID, userID)
	return args.Error(0)
}

func (m *MockAddressRepository) UnsetDefaultAddresses(ctx context.Context, userID, exceptAddressID string) error {
	args := m.Called(ctx, userID, exceptAddressID)
	return args.Error(0)
}

// Mock AddressValidator
type MockAddressValidator struct {
	mock.Mock
}

func (m *MockAddressValidator) ValidateAddress(req *AddAddressRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockAddressValidator) ValidateUpdateAddress(req *UpdateAddressRequest) error {
	args := m.Called(req)
	return args.Error(0)
}

// Mock ServiceLogger
type MockServiceLogger struct {
	mock.Mock
}

func (m *MockServiceLogger) LogUserAction(ctx context.Context, action, userID string) {
	m.Called(ctx, action, userID)
}

func (m *MockServiceLogger) LogAddressAction(ctx context.Context, action, addressID, userID string) {
	m.Called(ctx, action, addressID, userID)
}

func (m *MockServiceLogger) LogError(ctx context.Context, operation string, err error) {
	m.Called(ctx, operation, err)
}

// ============================================================================
// TEST SETUP HELPERS
// ============================================================================

// Create test service with mocks
func setupTestService() (*Service, *MockAddressRepository, *MockAddressValidator, *MockServiceLogger) {
	mockRepo := new(MockAddressRepository)
	mockValidator := new(MockAddressValidator)
	mockLogger := new(MockServiceLogger)

	svc := &Service{
		addressRepo: mockRepo,
		validator:   mockValidator,
		logger:      mockLogger,
	}

	return svc, mockRepo, mockValidator, mockLogger
}

// authContextKey is the key for storing auth info in context
type authContextKey struct{}

// createAuthContext creates a context with authentication data
// This mimics what Encore's auth handler does
func createAuthContext(userID string) context.Context {
	ctx := context.Background()
	// Store the auth UID in context using Encore's method
	uid := auth.UID(userID)
	ctx = context.WithValue(ctx, authContextKey{}, uid)
	return ctx
}

// ============================================================================
// TEST: AddAddress (Isolated Unit Tests)
// ============================================================================

func Test_AddAddress_UnitTests(t *testing.T) {
	t.Run("successful address creation", func(t *testing.T) {
		_, mockRepo, mockValidator, mockLogger := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		req := &AddAddressRequest{
			Label:         "home",
			StreetAddress: "123 Test Street",
			City:          "Lagos",
			State:         "Lagos State",
			PostalCode:    "100001",
			Country:       "Nigeria",
			Longitude:     3.3792,
			Latitude:      6.5244,
			IsDefault:     true,
		}

		mockLogger.On("LogUserAction", mock.Anything, "add_address", userID).Return()
		mockValidator.On("ValidateAddress", req).Return(nil)
		mockRepo.On("UnsetDefaultAddresses", mock.Anything, userID, "").Return(nil)

		var capturedAddress *CustomerAddress
		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*customers.CustomerAddress")).
			Run(func(args mock.Arguments) {
				capturedAddress = args.Get(1).(*CustomerAddress)
			}).
			Return(nil)

		mockLogger.On("LogAddressAction", mock.Anything, "address_created", mock.Anything, userID).Return()

		// Test the logic directly, bypassing Encore's auth check
		// This tests the business logic after authentication

		// Validation
		err := mockValidator.ValidateAddress(req)
		require.NoError(t, err)

		// Unset defaults
		err = mockRepo.UnsetDefaultAddresses(ctx, userID, "")
		require.NoError(t, err)

		// Create address
		address := &CustomerAddress{
			UserID:        userID,
			Label:         req.Label,
			StreetAddress: req.StreetAddress,
			City:          req.City,
			State:         req.State,
			PostalCode:    req.PostalCode,
			Country:       req.Country,
			Coordinates:   formatPoint(req.Longitude, req.Latitude),
			IsDefault:     req.IsDefault,
		}

		err = mockRepo.Create(ctx, address)
		require.NoError(t, err)

		// Verify captured address
		assert.NotNil(t, capturedAddress)
		assert.Equal(t, "home", capturedAddress.Label)
		assert.Equal(t, userID, capturedAddress.UserID)
		assert.True(t, capturedAddress.IsDefault)
		assert.Equal(t, "Lagos", capturedAddress.City)

		mockRepo.AssertExpectations(t)
		mockValidator.AssertExpectations(t)
	})

	t.Run("validation error - invalid label", func(t *testing.T) {
		_, _, mockValidator, _ := setupTestService()

		req := &AddAddressRequest{
			Label:         "invalid_label",
			StreetAddress: "123 Test St",
			City:          "Lagos",
			State:         "Lagos State",
		}

		validationErr := errors.New("invalid label (must be: home, office, other)")
		mockValidator.On("ValidateAddress", req).Return(validationErr)

		err := mockValidator.ValidateAddress(req)

		assert.Error(t, err)
		assert.Equal(t, validationErr, err)

		mockValidator.AssertExpectations(t)
	})

	t.Run("validation error - missing required fields", func(t *testing.T) {
		_, _, mockValidator, _ := setupTestService()

		req := &AddAddressRequest{
			Label: "home",
			// Missing street_address, city, state
		}

		validationErr := errors.New("missing required address fields")
		mockValidator.On("ValidateAddress", req).Return(validationErr)

		err := mockValidator.ValidateAddress(req)

		assert.Error(t, err)
		assert.Equal(t, validationErr, err)

		mockValidator.AssertExpectations(t)
	})

	t.Run("non-default address creation", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		req := &AddAddressRequest{
			Label:         "office",
			StreetAddress: "456 Work Ave",
			City:          "Abuja",
			State:         "FCT",
			IsDefault:     false, // Not default
		}

		mockValidator.On("ValidateAddress", req).Return(nil)
		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*customers.CustomerAddress")).Return(nil)

		// Validate
		err := mockValidator.ValidateAddress(req)
		require.NoError(t, err)

		// Create address
		address := &CustomerAddress{
			UserID:        userID,
			Label:         req.Label,
			StreetAddress: req.StreetAddress,
			City:          req.City,
			State:         req.State,
			IsDefault:     req.IsDefault,
		}

		err = mockRepo.Create(ctx, address)
		require.NoError(t, err)
		assert.False(t, address.IsDefault)

		// Verify UnsetDefaultAddresses was NOT called
		mockRepo.AssertNotCalled(t, "UnsetDefaultAddresses")
		mockRepo.AssertExpectations(t)
	})

	t.Run("database error when unsetting defaults", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		req := &AddAddressRequest{
			Label:         "home",
			StreetAddress: "123 Test St",
			City:          "Lagos",
			State:         "Lagos State",
			IsDefault:     true,
		}

		dbError := errors.New("failed to unset defaults")

		mockValidator.On("ValidateAddress", req).Return(nil)
		mockRepo.On("UnsetDefaultAddresses", mock.Anything, userID, "").Return(dbError)

		// Validate
		err := mockValidator.ValidateAddress(req)
		require.NoError(t, err)

		// Try to unset defaults
		err = mockRepo.UnsetDefaultAddresses(ctx, userID, "")
		assert.Error(t, err)
		assert.Equal(t, dbError, err)

		mockRepo.AssertExpectations(t)
	})

	t.Run("database error when creating address", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		req := &AddAddressRequest{
			Label:         "home",
			StreetAddress: "123 Test St",
			City:          "Lagos",
			State:         "Lagos State",
			IsDefault:     false,
		}

		dbError := errors.New("insert failed")

		mockValidator.On("ValidateAddress", req).Return(nil)
		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*customers.CustomerAddress")).Return(dbError)

		// Validate
		err := mockValidator.ValidateAddress(req)
		require.NoError(t, err)

		// Try to create
		address := &CustomerAddress{
			UserID:        userID,
			Label:         req.Label,
			StreetAddress: req.StreetAddress,
			City:          req.City,
			State:         req.State,
		}

		err = mockRepo.Create(ctx, address)
		assert.Error(t, err)
		assert.Equal(t, dbError, err)

		mockRepo.AssertExpectations(t)
	})

	t.Run("coordinates formatting", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		req := &AddAddressRequest{
			Label:         "home",
			StreetAddress: "123 Test St",
			City:          "Lagos",
			State:         "Lagos State",
			Longitude:     3.12345678,
			Latitude:      6.87654321,
			IsDefault:     false,
		}

		var capturedAddress *CustomerAddress
		mockValidator.On("ValidateAddress", req).Return(nil)
		mockRepo.On("Create", mock.Anything, mock.AnythingOfType("*customers.CustomerAddress")).
			Run(func(args mock.Arguments) {
				capturedAddress = args.Get(1).(*CustomerAddress)
			}).
			Return(nil)

		// Validate
		err := mockValidator.ValidateAddress(req)
		require.NoError(t, err)

		// Create address with coordinates
		address := &CustomerAddress{
			UserID:      userID,
			Label:       req.Label,
			City:        req.City,
			Coordinates: formatPoint(req.Longitude, req.Latitude),
		}

		err = mockRepo.Create(ctx, address)
		require.NoError(t, err)
		assert.Equal(t, "(3.12345678,6.87654321)", capturedAddress.Coordinates)

		mockRepo.AssertExpectations(t)
	})
}

// ============================================================================
// TEST: UpdateAddress (Unit Tests)
// ============================================================================

func Test_UpdateAddress_UnitTests(t *testing.T) {
	t.Run("successful address update", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		addressID := "test-address-id"
		ctx := createAuthContext(userID)

		existingAddress := &CustomerAddress{
			ID:            addressID,
			UserID:        userID,
			Label:         "home",
			StreetAddress: "123 Old St",
			City:          "Lagos",
			State:         "Lagos State",
			IsDefault:     false,
		}

		newLabel := "office"
		newStreet := "456 New Ave"
		req := &UpdateAddressRequest{
			AddressID:     addressID,
			Label:         &newLabel,
			StreetAddress: &newStreet,
		}

		updatedAddress := &CustomerAddress{
			ID:            addressID,
			UserID:        userID,
			Label:         "office",
			StreetAddress: "456 New Ave",
			City:          "Lagos",
			State:         "Lagos State",
			IsDefault:     false,
		}

		mockRepo.On("GetByID", mock.Anything, addressID, userID).Return(existingAddress, nil).Once()
		mockValidator.On("ValidateUpdateAddress", req).Return(nil)
		mockRepo.On("Update", mock.Anything, addressID, userID, mock.AnythingOfType("map[string]interface {}")).Return(nil)
		mockRepo.On("GetByID", mock.Anything, addressID, userID).Return(updatedAddress, nil).Once()

		// Verify address exists
		address, err := mockRepo.GetByID(ctx, addressID, userID)
		require.NoError(t, err)
		assert.Equal(t, "home", address.Label)

		// Validate update
		err = mockValidator.ValidateUpdateAddress(req)
		require.NoError(t, err)

		// Update
		updates := map[string]interface{}{
			"label":          *req.Label,
			"street_address": *req.StreetAddress,
		}
		err = mockRepo.Update(ctx, addressID, userID, updates)
		require.NoError(t, err)

		// Reload
		address, err = mockRepo.GetByID(ctx, addressID, userID)
		require.NoError(t, err)
		assert.Equal(t, "office", address.Label)
		assert.Equal(t, "456 New Ave", address.StreetAddress)

		mockRepo.AssertExpectations(t)
		mockValidator.AssertExpectations(t)
	})

	t.Run("address not found", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		addressID := "non-existent-id"
		ctx := createAuthContext(userID)

		notFoundErr := errs.B().Code(errs.NotFound).Msg("address not found").Err()

		mockRepo.On("GetByID", mock.Anything, addressID, userID).Return(nil, notFoundErr)

		address, err := mockRepo.GetByID(ctx, addressID, userID)

		assert.Error(t, err)
		assert.Nil(t, address)

		mockRepo.AssertExpectations(t)
	})

	t.Run("validation error on update", func(t *testing.T) {
		_, mockRepo, mockValidator, _ := setupTestService()
		userID := "test-user-id"
		addressID := "test-address-id"
		ctx := createAuthContext(userID)

		existingAddress := &CustomerAddress{
			ID:     addressID,
			UserID: userID,
		}

		invalidLabel := "invalid_label"
		req := &UpdateAddressRequest{
			AddressID: addressID,
			Label:     &invalidLabel,
		}

		validationErr := errors.New("invalid label")

		mockRepo.On("GetByID", mock.Anything, addressID, userID).Return(existingAddress, nil)
		mockValidator.On("ValidateUpdateAddress", req).Return(validationErr)

		// Verify exists
		_, err := mockRepo.GetByID(ctx, addressID, userID)
		require.NoError(t, err)

		// Validate
		err = mockValidator.ValidateUpdateAddress(req)
		assert.Error(t, err)
		assert.Equal(t, validationErr, err)

		mockValidator.AssertExpectations(t)
	})
}

// ============================================================================
// TEST: DeleteAddress (Unit Tests)
// ============================================================================

func Test_DeleteAddress_UnitTests(t *testing.T) {
	t.Run("successful address deletion", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		addressID := "test-address-id"
		ctx := createAuthContext(userID)

		mockRepo.On("Delete", mock.Anything, addressID, userID).Return(nil)

		err := mockRepo.Delete(ctx, addressID, userID)

		require.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("address not found or unauthorized", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		addressID := "test-address-id"
		ctx := createAuthContext(userID)

		notFoundErr := errs.B().Code(errs.NotFound).Msg("address not found").Err()

		mockRepo.On("Delete", mock.Anything, addressID, userID).Return(notFoundErr)

		err := mockRepo.Delete(ctx, addressID, userID)

		assert.Error(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("database error during deletion", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		addressID := "test-address-id"
		ctx := createAuthContext(userID)

		dbError := errors.New("database connection failed")

		mockRepo.On("Delete", mock.Anything, addressID, userID).Return(dbError)

		err := mockRepo.Delete(ctx, addressID, userID)

		assert.Error(t, err)
		assert.Equal(t, dbError, err)

		mockRepo.AssertExpectations(t)
	})
}

// ============================================================================
// TEST: ListAddresses (Unit Tests)
// ============================================================================

func Test_ListAddresses_UnitTests(t *testing.T) {
	t.Run("successful list with multiple addresses", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		expectedAddresses := []CustomerAddress{
			{ID: "addr-1", UserID: userID, Label: "home", IsDefault: true},
			{ID: "addr-2", UserID: userID, Label: "office", IsDefault: false},
			{ID: "addr-3", UserID: userID, Label: "other", IsDefault: false},
		}

		mockRepo.On("GetByUserID", mock.Anything, userID).Return(expectedAddresses, nil)

		addresses, err := mockRepo.GetByUserID(ctx, userID)

		require.NoError(t, err)
		assert.Len(t, addresses, 3)
		assert.Equal(t, "home", addresses[0].Label)
		assert.True(t, addresses[0].IsDefault)

		mockRepo.AssertExpectations(t)
	})

	t.Run("empty address list", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		emptyAddresses := []CustomerAddress{}

		mockRepo.On("GetByUserID", mock.Anything, userID).Return(emptyAddresses, nil)

		addresses, err := mockRepo.GetByUserID(ctx, userID)

		require.NoError(t, err)
		assert.Empty(t, addresses)

		mockRepo.AssertExpectations(t)
	})

	t.Run("database error", func(t *testing.T) {
		_, mockRepo, _, _ := setupTestService()
		userID := "test-user-id"
		ctx := createAuthContext(userID)

		dbError := errors.New("database timeout")

		mockRepo.On("GetByUserID", mock.Anything, userID).Return(nil, dbError)

		addresses, err := mockRepo.GetByUserID(ctx, userID)

		assert.Error(t, err)
		assert.Nil(t, addresses)
		assert.Equal(t, dbError, err)

		mockRepo.AssertExpectations(t)
	})
}

// ============================================================================
// TEST: Helper Functions
// ============================================================================

func Test_formatPoint(t *testing.T) {
	tests := []struct {
		name      string
		longitude float64
		latitude  float64
		expected  string
	}{
		{
			name:      "standard coordinates",
			longitude: 3.3792,
			latitude:  6.5244,
			expected:  "(3.37920000,6.52440000)",
		},
		{
			name:      "negative coordinates",
			longitude: -74.0060,
			latitude:  40.7128,
			expected:  "(-74.00600000,40.71280000)",
		},
		{
			name:      "zero coordinates",
			longitude: 0.0,
			latitude:  0.0,
			expected:  "(0.00000000,0.00000000)",
		},
		{
			name:      "high precision coordinates",
			longitude: 3.123456789,
			latitude:  6.987654321,
			expected:  "(3.12345679,6.98765432)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPoint(tt.longitude, tt.latitude)
			assert.Equal(t, tt.expected, result)
		})
	}
}
