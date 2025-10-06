package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateJWT creates a JWT token for the authenticated user
func (s *Service) generateJWT(userID, email, userType string, profileComplete bool) (string, error) {
	claims := JWTClaims{
		UserID:          userID,
		Email:           email,
		UserType:        userType,
		ProfileComplete: profileComplete,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
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
