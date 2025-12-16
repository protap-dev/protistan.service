package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateJWT creates a JWT token for the authenticated user
func (s *Service) generateJWT(userID, email string, roles []string, activeRole *string, profileComplete bool) (string, error) {
	claims := JWTClaims{
		UserID:          userID,
		Email:           email,
		Roles:           roles,
		ActiveRole:      derefOrEmpty(activeRole),
		ProfileComplete: profileComplete,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secrets.JWTSecret))
}

// parse and validate JWT, returns claims
func parseJWT(tokenStr string) (claims *JWTClaims, err error) {
	defer func() {
		if r := recover(); r != nil {
			// JWT parsing panicked, likely due to malformed token
			claims = nil
			err = errors.New("invalid token format")
		}
	}()

	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secrets.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return claims, nil
}

// createRefreshToken creates a refresh token for the authenticated user
func (s *Service) createRefreshToken(userID string) (string, error) {
	token, err := generateSecureToken(32)
	if err != nil {
		return "", err
	}

	refreshToken := RefreshToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour), // 30 days
	}

	if err := s.db.Create(&refreshToken).Error; err != nil {
		return "", err
	}

	return token, nil
}
