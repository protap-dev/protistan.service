package auth

import (
	"context"
	"testing"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"github.com/stretchr/testify/assert"
)

func TestEnableRole(t *testing.T) {
	s, _ := initService()
	defer s.db.Migrator().DropTable(&User{}, &RefreshToken{})

	// Create a user
	user := User{
		Email:        "test@example.com",
		PasswordHash: "dummy-hash",
	}
	s.db.Create(&user)

	// Create a context with the user's ID and mock user data
	mockActiveRole := "customer"
	ctx := auth.WithContext(context.Background(), auth.UID(user.ID), &UserData{
		ID:         user.ID,
		Email:      user.Email,
		Roles:      StringArray{mockActiveRole},
		ActiveRole: &mockActiveRole,
	})

	// Test case: Enable a valid role
	reqBody := EnableRoleRequest{Role: "customer"}

	resp, err := s.EnableRole(ctx, &reqBody)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Token)

	var updatedUser User
	s.db.First(&updatedUser, "id = ?", user.ID)
	assert.Contains(t, updatedUser.Roles, "customer")
	assert.Equal(t, "customer", *updatedUser.ActiveRole)

	// Test case: Enable an invalid role
	reqBody = EnableRoleRequest{Role: "invalid"}

	resp, err = s.EnableRole(ctx, &reqBody)

	assert.Error(t, err)
	assert.Nil(t, resp)
	var errsErr *errs.Error
	assert.ErrorAs(t, err, &errsErr)
	assert.Equal(t, errs.InvalidArgument, errsErr.Code)
}

func TestSetActiveRole(t *testing.T) {
	s, _ := initService()
	defer s.db.Migrator().DropTable(&User{}, &RefreshToken{})

	// Create a user with roles
	user := User{
		Email:        "test@example.com",
		PasswordHash: "dummy-hash",
		Roles:        []string{"customer", "artisan"},
	}
	s.db.Create(&user)

	// Create a context with the user's ID and mock user data
	activeRole := "customer" // Assuming "customer" is the initial active role
	ctx := auth.WithContext(context.Background(), auth.UID(user.ID), &UserData{
		ID:         user.ID,
		Email:      user.Email,
		Roles:      user.Roles, // Use the roles created for the user
		ActiveRole: &activeRole,
	})

	// Test case: Set a valid active role
	reqBody := SetActiveRoleRequest{Role: "artisan"}

	resp, err := s.SetActiveRole(ctx, &reqBody)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Token)

	var updatedUser User
	s.db.First(&updatedUser, "id = ?", user.ID)
	assert.Equal(t, "artisan", *updatedUser.ActiveRole)

	// Test case: Set an invalid active role
	reqBody = SetActiveRoleRequest{Role: "invalid"}

	resp, err = s.SetActiveRole(ctx, &reqBody)

	assert.Error(t, err)
	assert.Nil(t, resp)
	var errsErr *errs.Error
	assert.ErrorAs(t, err, &errsErr)
	assert.Equal(t, errs.InvalidArgument, errsErr.Code)
}
