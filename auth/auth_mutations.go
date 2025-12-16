package auth

import (
	"context"
	"errors"
	"strings"

	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"gorm.io/gorm"
)

// EnableRoleRequest represents the request to enable a role for a user.
type EnableRoleRequest struct {
	Role string `json:"role"`
}

// SetActiveRoleRequest represents the request to set the active role for a user.
type SetActiveRoleRequest struct {
	Role string `json:"role"`
}

//encore:api auth method=POST path=/v0/auth/user/roles/enable
func (s *Service) EnableRole(ctx context.Context, req *EnableRoleRequest) (*AuthResponse, error) {
	uid, _ := auth.UserID()
	if uid == "" {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("unauthorized").Err()
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "customer" && role != "artisan" {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("role must be customer or artisan").Err()
	}

	var user User
	if err := s.db.First(&user, "id = ?", string(uid)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.B().Code(errs.NotFound).Msg("user not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("database error").Err()
	}

	// Add role if not already present
	roleExists := false
	for _, r := range user.Roles {
		if r == role {
			roleExists = true
			break
		}
	}
	if !roleExists {
		user.Roles = append(user.Roles, role)
	}

	// If active_role is null, set it to this role
	if user.ActiveRole == nil {
		user.ActiveRole = &role
	}

	if err := s.db.Save(&user).Error; err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to update user").Err()
	}

	// Generate new token
	token, err := s.generateJWT(user.ID, user.Email, user.Roles, user.ActiveRole, user.ProfileComplete)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to generate token").Err()
	}

	refreshToken, err := s.createRefreshToken(user.ID) // or user.ID for login
	if err != nil {
		return nil, errs.B().Msg("failed to generate refresh token").Err()
	}

	return &AuthResponse{Token: token, RefreshToken: refreshToken}, nil
}

//encore:api auth method=PUT path=/v0/auth/user/active-role
func (s *Service) SetActiveRole(ctx context.Context, req *SetActiveRoleRequest) (*AuthResponse, error) {
	uid, _ := auth.UserID()
	if uid == "" {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("unauthorized").Err()
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))

	var user User
	if err := s.db.First(&user, "id = ?", string(uid)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.B().Code(errs.NotFound).Msg("user not found").Err()
		}
		return nil, errs.B().Code(errs.Internal).Msg("database error").Err()
	}

	// Check if role is in user.roles
	roleExists := false
	for _, r := range user.Roles {
		if r == role {
			roleExists = true
			break
		}
	}
	if !roleExists {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("role not enabled for user").Err()
	}

	user.ActiveRole = &role

	if err := s.db.Save(&user).Error; err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to update user").Err()
	}

	// Generate new token
	token, err := s.generateJWT(user.ID, user.Email, user.Roles, user.ActiveRole, user.ProfileComplete)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to generate token").Err()
	}

	refreshToken, err := s.createRefreshToken(user.ID) // or user.ID for login
	if err != nil {
		return nil, errs.B().Msg("failed to generate refresh token").Err()
	}

	return &AuthResponse{Token: token, RefreshToken: refreshToken}, nil
}
