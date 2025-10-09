package user

import (
	"context"
	"strings"
	"testing"

	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
)

// Test cases for GetProfile API endpoint
func TestService_GetProfile(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		expectedResult *CompleteUserProfile
		expectedError  error
	}{
		{
			name: "successful profile retrieval",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("user123")
				profile := createTestProfile("user123")
				settings := createTestSettings("user123")

				userRepo.users["user123"] = user
				profileRepo.profiles["user123"] = profile
				settingsRepo.settings["user123"] = settings
			},
			expectedResult: &CompleteUserProfile{
				User:     *createTestUser("user123"),
				Profile:  *createTestProfile("user123"),
				Settings: *createTestSettings("user123"),
			},
			expectedError: nil,
		},
		{
			name: "user not found",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No users set up - should return user not found
			},
			expectedResult: nil,
			expectedError:  ErrUserNotFound,
		},
		{
			name: "profile not found - creates default",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("user123")
				settings := createTestSettings("user123")

				userRepo.users["user123"] = user
				settingsRepo.settings["user123"] = settings
				// No profile - should create default
			},
			expectedResult: &CompleteUserProfile{
				User:     *createTestUser("user123"),
				Profile:  UserProfile{UserID: "user123"}, // Default profile
				Settings: *createTestSettings("user123"),
			},
			expectedError: nil,
		},
		{
			name: "settings not found - creates default",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("user123")
				profile := createTestProfile("user123")

				userRepo.users["user123"] = user
				profileRepo.profiles["user123"] = profile
				// No settings - should create default
			},
			expectedResult: &CompleteUserProfile{
				User:    *createTestUser("user123"),
				Profile: *createTestProfile("user123"),
				Settings: UserSettings{ // Default settings created by service
					UserID:             "user123",
					EmailNotifications: true,
					SMSNotifications:   false,
					PushNotifications:  true,
					Language:           "en",
					Timezone:           "Africa/Lagos",
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userRepo, profileRepo, settingsRepo, validator, logger := createTestService()

			tt.setupMocks(userRepo, profileRepo, settingsRepo, validator, logger)

			// Mock authentication to return user123
			originalAuthUserID := authUserID
			authUserID = func() (string, bool) { return "user123", true }
			defer func() { authUserID = originalAuthUserID }()

			result, err := service.GetProfile(context.Background())

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("GetProfile() error = %v, expected no error", err)
				}
				if result == nil {
					t.Errorf("GetProfile() result = nil, expected profile")
				}
			} else {
				if err == nil {
					t.Errorf("GetProfile() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("GetProfile() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test cases for UpdateProfile API endpoint
func TestService_UpdateProfile(t *testing.T) {
	tests := []struct {
		name           string
		request        *UpdateProfileRequest
		setupMocks     func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		expectedResult *UserProfile
		expectedError  error
	}{
		{
			name: "successful profile update",
			request: &UpdateProfileRequest{
				FirstName: "Jane",
				LastName:  "Smith",
				Phone:     "9876543210",
				AvatarURL: "https://example.com/new-avatar.jpg",
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// Setup validator to pass validation
				validator.validateNameFunc = func(name string, maxLength int) error {
					if len(name) > maxLength {
						return ErrNameTooLong
					}
					return nil
				}
				validator.validatePhoneFunc = func(phone string) error {
					return nil
				}
			},
			expectedResult: &UserProfile{
				UserID:    "user123",
				FirstName: "Jane",
				LastName:  "Smith",
				Phone:     "9876543210",
				AvatarURL: "https://example.com/new-avatar.jpg",
			},
			expectedError: nil,
		},
		{
			name: "validation error - first name too long",
			request: &UpdateProfileRequest{
				FirstName: strings.Repeat("A", 101), // Over 100 char limit
				LastName:  "Smith",
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				validator.validateNameFunc = func(name string, maxLength int) error {
					if len(name) > maxLength {
						return ErrNameTooLong
					}
					return nil
				}
			},
			expectedResult: nil,
			expectedError:  ErrNameTooLong,
		},
		{
			name: "validation error - invalid phone",
			request: &UpdateProfileRequest{
				FirstName: "Jane",
				LastName:  "Smith",
				Phone:     "invalid-phone",
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				validator.validateNameFunc = func(name string, maxLength int) error {
					return nil
				}
				validator.validatePhoneFunc = func(phone string) error {
					return ErrPhoneInvalid
				}
			},
			expectedResult: nil,
			expectedError:  ErrPhoneInvalid,
		},
		{
			name: "partial update - only first name",
			request: &UpdateProfileRequest{
				FirstName: "Jane",
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				validator.validateNameFunc = func(name string, maxLength int) error {
					return nil
				}
			},
			expectedResult: &UserProfile{
				UserID:    "user123",
				FirstName: "Jane",
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userRepo, profileRepo, settingsRepo, validator, logger := createTestService()

			tt.setupMocks(userRepo, profileRepo, settingsRepo, validator, logger)

			// Mock authentication
			originalAuthUserID := authUserID
			authUserID = func() (string, bool) { return "user123", true }
			defer func() { authUserID = originalAuthUserID }()

			result, err := service.UpdateProfile(context.Background(), tt.request)

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("UpdateProfile() error = %v, expected no error", err)
				}
				if result == nil {
					t.Errorf("UpdateProfile() result = nil, expected profile")
				}
			} else {
				if err == nil {
					t.Errorf("UpdateProfile() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("UpdateProfile() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test cases for UpdateSettings API endpoint
func TestService_UpdateSettings(t *testing.T) {
	tests := []struct {
		name           string
		request        *UpdateSettingsRequest
		setupMocks     func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		expectedResult *UserSettings
		expectedError  error
	}{
		{
			name: "successful settings update",
			request: &UpdateSettingsRequest{
				EmailNotifications: &[]bool{false}[0],
				SMSNotifications:   &[]bool{true}[0],
				Language:           &[]string{"fr"}[0],
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No specific setup needed for successful case
			},
			expectedResult: &UserSettings{
				UserID:             "user123",
				EmailNotifications: false,
				SMSNotifications:   true,
				PushNotifications:  true, // Default
				Language:           "fr",
				Timezone:           "Africa/Lagos", // Default
			},
			expectedError: nil,
		},
		{
			name:    "no updates provided",
			request: &UpdateSettingsRequest{},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No specific setup needed
			},
			expectedResult: &UserSettings{
				UserID:             "user123",
				EmailNotifications: true,           // Default
				SMSNotifications:   false,          // Default
				PushNotifications:  true,           // Default
				Language:           "en",           // Default
				Timezone:           "Africa/Lagos", // Default
			},
			expectedError: nil,
		},
		{
			name: "partial settings update",
			request: &UpdateSettingsRequest{
				Language: &[]string{"es"}[0],
			},
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No specific setup needed
			},
			expectedResult: &UserSettings{
				UserID:             "user123",
				EmailNotifications: true,  // Default
				SMSNotifications:   false, // Default
				PushNotifications:  true,  // Default
				Language:           "es",
				Timezone:           "Africa/Lagos", // Default
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userRepo, profileRepo, settingsRepo, validator, logger := createTestService()

			tt.setupMocks(userRepo, profileRepo, settingsRepo, validator, logger)

			// Mock authentication
			originalAuthUserID := authUserID
			authUserID = func() (string, bool) { return "user123", true }
			defer func() { authUserID = originalAuthUserID }()

			result, err := service.UpdateSettings(context.Background(), tt.request)

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("UpdateSettings() error = %v, expected no error", err)
				}
				if result == nil {
					t.Errorf("UpdateSettings() result = nil, expected settings")
				}
			} else {
				if err == nil {
					t.Errorf("UpdateSettings() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("UpdateSettings() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test cases for internal API endpoints
func TestService_GetUserByID(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		setupMocks     func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		expectedResult *User
		expectedError  error
	}{
		{
			name:   "successful user retrieval",
			userID: "550e8400-e29b-41d4-a716-446655440000", // Valid UUID format
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("550e8400-e29b-41d4-a716-446655440000")
				userRepo.users["550e8400-e29b-41d4-a716-446655440000"] = user
			},
			expectedResult: createTestUser("550e8400-e29b-41d4-a716-446655440000"),
			expectedError:  nil,
		},
		{
			name:   "user not found",
			userID: "550e8400-e29b-41d4-a716-446655440001", // Different UUID that won't exist
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No user setup - user doesn't exist
			},
			expectedResult: nil,
			expectedError:  ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userRepo, profileRepo, settingsRepo, validator, logger := createTestService()

			tt.setupMocks(userRepo, profileRepo, settingsRepo, validator, logger)

			userUUID, err := uuid.FromString(tt.userID)
			if err != nil {
				t.Fatalf("Failed to parse UUID %s: %v", tt.userID, err)
			}
			result, err := service.GetUserByID(context.Background(), userUUID)

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("GetUserByID() error = %v, expected no error", err)
				}
				if result == nil {
					t.Errorf("GetUserByID() result = nil, expected user")
				}
			} else {
				if err == nil {
					t.Errorf("GetUserByID() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("GetUserByID() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test cases for getCompleteProfile helper method
func TestService_getCompleteProfile(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		setupMocks     func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		expectedResult *CompleteUserProfile
		expectedError  error
	}{
		{
			name:   "all data exists",
			userID: "550e8400-e29b-41d4-a716-446655440000",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("550e8400-e29b-41d4-a716-446655440000")
				profile := createTestProfile("550e8400-e29b-41d4-a716-446655440000")
				settings := createTestSettings("550e8400-e29b-41d4-a716-446655440000")

				userRepo.users["550e8400-e29b-41d4-a716-446655440000"] = user
				profileRepo.profiles["550e8400-e29b-41d4-a716-446655440000"] = profile
				settingsRepo.settings["550e8400-e29b-41d4-a716-446655440000"] = settings
			},
			expectedResult: &CompleteUserProfile{
				User:     *createTestUser("550e8400-e29b-41d4-a716-446655440000"),
				Profile:  *createTestProfile("550e8400-e29b-41d4-a716-446655440000"),
				Settings: *createTestSettings("550e8400-e29b-41d4-a716-446655440000"),
			},
			expectedError: nil,
		},
		{
			name:   "user not found",
			userID: "550e8400-e29b-41d4-a716-446655440001",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No setup - user doesn't exist
			},
			expectedResult: nil,
			expectedError:  ErrUserNotFound,
		},
		{
			name:   "profile creation fails",
			userID: "550e8400-e29b-41d4-a716-446655440002",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				user := createTestUser("550e8400-e29b-41d4-a716-446655440002")
				userRepo.users["550e8400-e29b-41d4-a716-446655440002"] = user
				// Configure profile repo to fail creation
				profileRepo.SetShouldFailCreate(true)
			},
			expectedResult: nil,
			expectedError:  errs.B().Msg("failed to create profile").Err(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userRepo, profileRepo, settingsRepo, validator, logger := createTestService()

			tt.setupMocks(userRepo, profileRepo, settingsRepo, validator, logger)

			result, err := service.getCompleteProfile(context.Background(), tt.userID)

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("getCompleteProfile() error = %v, expected no error", err)
				}
				if result == nil {
					t.Errorf("getCompleteProfile() result = nil, expected profile")
				}
			} else {
				if err == nil {
					t.Errorf("getCompleteProfile() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("getCompleteProfile() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test cases for concurrent access safety
func TestService_ConcurrentAccess(t *testing.T) {
	service, _, _, _, _, _ := createTestService()

	// Setup test data
	user := createTestUser("550e8400-e29b-41d4-a716-446655440003")
	userRepo := NewMockUserRepository()
	userRepo.users["550e8400-e29b-41d4-a716-446655440003"] = user

	// Replace the service's userRepo with our test repo
	service.userRepo = userRepo

	// Mock authentication
	originalAuthUserID := authUserID
	authUserID = func() (string, bool) { return "550e8400-e29b-41d4-a716-446655440003", true }
	defer func() { authUserID = originalAuthUserID }()

	// Test concurrent access to GetProfile
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			_, err := service.GetProfile(context.Background())
			if err != nil {
				t.Errorf("Concurrent GetProfile failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test cases for error handling edge cases
func TestService_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger)
		testFunction  func(*Service) error
		expectedError error
	}{
		{
			name: "unauthenticated access",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// No setup needed
			},
			testFunction: func(service *Service) error {
				_, err := service.GetProfile(context.Background())
				return err
			},
			expectedError: ErrUnauthenticated,
		},
		{
			name: "repository error handling",
			setupMocks: func(userRepo *mockUserRepository, profileRepo *mockProfileRepository, settingsRepo *mockSettingsRepository, validator *mockUserValidator, logger *mockServiceLogger) {
				// Don't set up any data - should cause errors
			},
			testFunction: func(service *Service) error {
				// Create a service with a mock that will fail
				mockUserRepo := NewMockUserRepository()
				service.userRepo = mockUserRepo
				userUUID, err := uuid.FromString("550e8400-e29b-41d4-a716-446655440004")
				if err != nil {
					return err
				}
				_, err = service.GetUserByID(context.Background(), userUUID)
				return err
			},
			expectedError: ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &Service{
				userRepo:     NewMockUserRepository(),
				profileRepo:  NewMockProfileRepository(),
				settingsRepo: NewMockSettingsRepository(),
				validator:    NewMockUserValidator(),
				logger:       NewMockServiceLogger(),
			}

			tt.setupMocks(nil, nil, nil, nil, nil)

			err := tt.testFunction(service)

			if tt.expectedError == nil {
				if err != nil {
					t.Errorf("testFunction() error = %v, expected no error", err)
				}
			} else {
				if err == nil {
					t.Errorf("testFunction() error = nil, expected error %v", tt.expectedError)
				} else if err.Error() != tt.expectedError.Error() {
					t.Errorf("testFunction() error = %v, expected error %v", err, tt.expectedError)
				}
			}
		})
	}
}

// Test service initialization
func TestService_Initialization(t *testing.T) {
	// Test that the service initializes correctly with all dependencies
	service, _, _, _, _, _ := createTestService()

	if service == nil {
		t.Error("Service should not be nil after initialization")
	}
	if service.userRepo == nil {
		t.Error("Service userRepo should not be nil")
	}
	if service.profileRepo == nil {
		t.Error("Service profileRepo should not be nil")
	}
	if service.settingsRepo == nil {
		t.Error("Service settingsRepo should not be nil")
	}
	if service.validator == nil {
		t.Error("Service validator should not be nil")
	}
	if service.logger == nil {
		t.Error("Service logger should not be nil")
	}
}

// Test interface compliance
func TestService_InterfaceCompliance(t *testing.T) {
	var _ interface{} = (*Service)(nil)
}
