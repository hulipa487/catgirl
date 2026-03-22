package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/rs/zerolog"
)

type AuthService struct {
	config *config.AuthConfig
	logger zerolog.Logger
	httpClient *http.Client
}

func NewAuthService(cfg *config.AuthConfig, logger zerolog.Logger) *AuthService {
	return &AuthService{
		config: cfg,
		logger: logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type MTFPassUser struct {
	UID      int64  `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Credits  int    `json:"credits"`
}

type MTFPassResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ValidateToken calls mtfpass /api/v1/auth/check to validate the JWT token
// Returns the user info if valid, error otherwise
// Only role "admin" is allowed - returns error for "user" and other roles
func (s *AuthService) ValidateToken(ctx context.Context, tokenString string) (*MTFPassUser, error) {
	if s.config.MTFPassURL == "" {
		return nil, fmt.Errorf("mtfpass_url not configured")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", s.config.MTFPassURL+"/api/v1/auth/check", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "mtf_auth", Value: tokenString})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call mtfpass: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mtfpass returned status %d", resp.StatusCode)
	}

	var mtfResp MTFPassResponse
	if err := json.NewDecoder(resp.Body).Decode(&mtfResp); err != nil {
		return nil, fmt.Errorf("failed to decode mtfpass response: %w", err)
	}

	if !mtfResp.Success {
		return nil, fmt.Errorf("mtfpass auth failed: %s", mtfResp.Error)
	}

	// Parse the data field into MTFPassUser
	dataBytes, err := json.Marshal(mtfResp.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mtfpass data: %w", err)
	}

	var user MTFPassUser
	if err := json.Unmarshal(dataBytes, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mtfpass user: %w", err)
	}

	// Only allow role "admin" - reject "user" and any other role
	if user.Role != "admin" {
		return nil, fmt.Errorf("access denied: role '%s' is not allowed", user.Role)
	}

	return &user, nil
}

// AuthorizationResult represents the result of authorization
type AuthorizationResult struct {
	Authorized bool
	UserID     string
	Reason     string
}

// Authorize validates the token and returns the authorization result
func (s *AuthService) Authorize(ctx context.Context, tokenString string) (*AuthorizationResult, error) {
	user, err := s.ValidateToken(ctx, tokenString)
	if err != nil {
		return &AuthorizationResult{
			Authorized: false,
			Reason:    err.Error(),
		}, nil
	}

	return &AuthorizationResult{
		Authorized: true,
		UserID:    fmt.Sprintf("%d", user.UID),
	}, nil
}
