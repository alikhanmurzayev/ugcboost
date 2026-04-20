package domain

import (
	"time"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
)

// User is the domain representation of a user (without sensitive fields like PasswordHash).
type User struct {
	ID        string       `json:"id"`
	Email     string       `json:"email"`
	Role      api.UserRole `json:"role"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}
