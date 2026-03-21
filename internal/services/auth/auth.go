package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type AuthService struct {
	config *config.AuthConfig
	logger zerolog.Logger
}

func NewAuthService(cfg *config.AuthConfig, logger zerolog.Logger) *AuthService {
	return &AuthService{
		config: cfg,
		logger: logger,
	}
}

type TokenClaims struct {
	UserID       string                  `json:"sub"`
	Email       string                  `json:"email"`
	Membership   models.MembershipLevel `json:"membership"`
	SessionID    uuid.UUID               `json:"session_id"`
	Permissions []string                `json:"permissions"`
	jwt.RegisteredClaims
}

func (s *AuthService) ValidateJWT(tokenString string) (*TokenClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWTSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	return claims, nil
}

func (s *AuthService) GetMembershipPriority(membership models.MembershipLevel) float64 {
	switch membership {
	case models.MembershipFree:
		return 0
	case models.MembershipBasic:
		return 0.5
	case models.MembershipPro:
		return 1.0
	case models.MembershipEnterprise:
		return 2.0
	default:
		return 0
	}
}

func (s *AuthService) CheckPermission(claims *TokenClaims, permission string) bool {
	for _, p := range claims.Permissions {
		if p == permission || p == "*" {
			return true
		}
	}
	return false
}

type AuthorizationResult struct {
	Authorized bool
	UserID    string
	SessionID uuid.UUID
	Membership models.MembershipLevel
	Reason    string
}

func (s *AuthService) Authorize(ctx context.Context, tokenString string) (*AuthorizationResult, error) {
	claims, err := s.ValidateJWT(tokenString)
	if err != nil {
		return &AuthorizationResult{
			Authorized: false,
			Reason:    err.Error(),
		}, nil
	}

	if !s.isMembershipAllowed(claims.Membership) {
		return &AuthorizationResult{
			Authorized: false,
			Reason:    fmt.Sprintf("membership %s not allowed", claims.Membership),
		}, nil
	}

	return &AuthorizationResult{
		Authorized: true,
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
		Membership: claims.Membership,
	}, nil
}

func (s *AuthService) isMembershipAllowed(membership models.MembershipLevel) bool {
	if len(s.config.AllowedMemberships) == 0 {
		return true
	}

	for _, allowed := range s.config.AllowedMemberships {
		if string(allowed) == string(membership) {
			return true
		}
	}
	return false
}
