package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	PurposeEmailVerify   = "email_verify"
	PurposePasswordReset = "password_reset"
	PurposeAccess        = "access"
)

type TokenClaims struct {
	UserID  int    `json:"user_id"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Purpose string `json:"purpose"`
	JTI     string `json:"jti"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	return []byte(GetEnv("JWT_SECRET", "default-secret-change-me"))
}

func GenerateAccessToken(userID int, email, role string) (string, int64, error) {
	expiryHours := GetEnvAsInt("JWT_EXPIRY_HOURS", 24)
	expiresAt := time.Now().Add(time.Duration(expiryHours) * time.Hour)
	return signToken(TokenClaims{
		UserID:  userID,
		Email:   email,
		Role:    role,
		Purpose: PurposeAccess,
		JTI:     uuid.New().String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
}

func GeneratePurposeToken(userID int, email, role, purpose string, ttl time.Duration) (string, error) {
	expiresAt := time.Now().Add(ttl)
	token, _, err := signToken(TokenClaims{
		UserID:  userID,
		Email:   email,
		Role:    role,
		Purpose: purpose,
		JTI:     uuid.New().String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	return token, err
}

func signToken(claims TokenClaims) (string, int64, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret())
	if err != nil {
		return "", 0, err
	}
	var exp int64
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Unix()
	}
	return tokenString, exp, nil
}

func ParseToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
