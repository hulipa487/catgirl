package models

import (
	"time"

	"github.com/google/uuid"
)

type TelegramUser struct {
	TelegramUserID int64      `json:"telegram_user_id" db:"telegram_user_id"`
	SessionID      *uuid.UUID `json:"session_id" db:"session_id"`
	Username       *string    `json:"username" db:"username"`
	FirstName      *string    `json:"first_name" db:"first_name"`
	LastName       *string    `json:"last_name" db:"last_name"`
	IsBanned       bool       `json:"is_banned" db:"is_banned"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	LastActivity   time.Time  `json:"last_activity" db:"last_activity"`
}
