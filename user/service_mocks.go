package user

import (
	"context"
	"errors"
	"time"
)

// Mock implementations for testing

// Mock UserRepository
type mockUserRepository struct {
	users map[string]*User
}

func NewMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users: make(map[string]*User),
	}
}

func (m *mockUserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	if user, exists := m.users[id]; exists {
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) Create(ctx context.Context, user *User) error {
	if _, exists := m.users[user.ID]; exists {
		return errors.New("user already exists")
	}
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	if _, exists := m.users[id]; !exists {
		return ErrUserNotFound
	}
	// Simple implementation - in real tests you'd want more sophisticated updating
	return nil
}

func (m *mockUserRepository) UpdateProfileComplete(ctx context.Context, id string, complete bool) error {
	if user, exists := m.users[id]; exists {
		user.ProfileComplete = complete
		return nil
	}
	return ErrUserNotFound
}

// Mock ProfileRepository
type mockProfileRepository struct {
	profiles         map[string]*UserProfile
	shouldFailCreate bool
}

func NewMockProfileRepository() *mockProfileRepository {
	return &mockProfileRepository{
		profiles:         make(map[string]*UserProfile),
		shouldFailCreate: false,
	}
}

func (m *mockProfileRepository) SetShouldFailCreate(fail bool) {
	m.shouldFailCreate = fail
}

func (m *mockProfileRepository) GetByUserID(ctx context.Context, userID string) (*UserProfile, error) {
	if profile, exists := m.profiles[userID]; exists {
		return profile, nil
	}
	return nil, ErrProfileNotFound
}

func (m *mockProfileRepository) Create(ctx context.Context, profile *UserProfile) error {
	if m.shouldFailCreate {
		return errors.New("failed to create profile")
	}
	if _, exists := m.profiles[profile.UserID]; exists {
		return errors.New("profile already exists")
	}
	m.profiles[profile.UserID] = profile
	return nil
}

func (m *mockProfileRepository) Update(ctx context.Context, userID string, updates map[string]interface{}) error {
	if _, exists := m.profiles[userID]; !exists {
		return ErrProfileNotFound
	}
	return nil
}

func (m *mockProfileRepository) Upsert(ctx context.Context, profile *UserProfile) error {
	m.profiles[profile.UserID] = profile
	return nil
}

// Mock SettingsRepository
type mockSettingsRepository struct {
	settings map[string]*UserSettings
}

func NewMockSettingsRepository() *mockSettingsRepository {
	return &mockSettingsRepository{
		settings: make(map[string]*UserSettings),
	}
}

func (m *mockSettingsRepository) GetByUserID(ctx context.Context, userID string) (*UserSettings, error) {
	if settings, exists := m.settings[userID]; exists {
		return settings, nil
	}
	return nil, ErrSettingsNotFound
}

func (m *mockSettingsRepository) Create(ctx context.Context, settings *UserSettings) error {
	if _, exists := m.settings[settings.UserID]; exists {
		return errors.New("settings already exists")
	}
	m.settings[settings.UserID] = settings
	return nil
}

func (m *mockSettingsRepository) Update(ctx context.Context, userID string, updates map[string]interface{}) error {
	if _, exists := m.settings[userID]; !exists {
		return ErrSettingsNotFound
	}
	return nil
}

func (m *mockSettingsRepository) Upsert(ctx context.Context, settings *UserSettings) error {
	m.settings[settings.UserID] = settings
	return nil
}

// Mock UserValidator
type mockUserValidator struct {
	validateNameFunc     func(name string, maxLength int) error
	validatePhoneFunc    func(phone string) error
	validateEmailFunc    func(email string) error
	validatePasswordFunc func(password string) []string
}

func NewMockUserValidator() *mockUserValidator {
	return &mockUserValidator{}
}

func (m *mockUserValidator) ValidateName(name string, maxLength int) error {
	if m.validateNameFunc != nil {
		return m.validateNameFunc(name, maxLength)
	}
	return nil
}

func (m *mockUserValidator) ValidatePhone(phone string) error {
	if m.validatePhoneFunc != nil {
		return m.validatePhoneFunc(phone)
	}
	return nil
}

func (m *mockUserValidator) ValidateEmail(email string) error {
	if m.validateEmailFunc != nil {
		return m.validateEmailFunc(email)
	}
	return nil
}

func (m *mockUserValidator) ValidatePassword(password string) []string {
	if m.validatePasswordFunc != nil {
		return m.validatePasswordFunc(password)
	}
	return []string{}
}

// Mock ServiceLogger
type mockServiceLogger struct {
	logUserActionCalls []LogCall
	logErrorCalls      []ErrorLogCall
}

type LogCall struct {
	Action string
	UserID string
}

type ErrorLogCall struct {
	Operation string
	Err       error
}

func NewMockServiceLogger() *mockServiceLogger {
	return &mockServiceLogger{
		logUserActionCalls: []LogCall{},
		logErrorCalls:      []ErrorLogCall{},
	}
}

func (m *mockServiceLogger) LogUserAction(ctx context.Context, action, userID string) {
	m.logUserActionCalls = append(m.logUserActionCalls, LogCall{
		Action: action,
		UserID: userID,
	})
}

func (m *mockServiceLogger) LogError(ctx context.Context, operation string, err error) {
	m.logErrorCalls = append(m.logErrorCalls, ErrorLogCall{
		Operation: operation,
		Err:       err,
	})
}

// Helper function to create a test service with mocks
func createTestService() (*Service, *mockUserRepository, *mockProfileRepository, *mockSettingsRepository, *mockUserValidator, *mockServiceLogger) {
	userRepo := NewMockUserRepository()
	profileRepo := NewMockProfileRepository()
	settingsRepo := NewMockSettingsRepository()
	validator := NewMockUserValidator()
	logger := NewMockServiceLogger()

	service := &Service{
		userRepo:     userRepo,
		profileRepo:  profileRepo,
		settingsRepo: settingsRepo,
		validator:    validator,
		logger:       logger,
	}

	return service, userRepo, profileRepo, settingsRepo, validator, logger
}

// Helper function to create test data
func createTestUser(userID string) *User {
	activeRole := "customer"
	return &User{
		ID:              userID,
		Email:           "test@example.com",
		PasswordHash:    "hashed_password",
		EmailVerified:   true,
		ActiveRole:      &activeRole,
		ProfileComplete: false,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func createTestProfile(userID string) *UserProfile {
	return &UserProfile{
		ID:        "profile_" + userID,
		UserID:    userID,
		FirstName: "John",
		LastName:  "Doe",
		Phone:     "1234567890",
		AvatarURL: "https://example.com/avatar.jpg",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func createTestSettings(userID string) *UserSettings {
	return &UserSettings{
		ID:                 "settings_" + userID,
		UserID:             userID,
		EmailNotifications: true,
		SMSNotifications:   false,
		PushNotifications:  true,
		Language:           "en",
		Timezone:           "Africa/Lagos",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}
