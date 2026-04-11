package domain

import "time"

type Brand struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	LogoURL   *string   `json:"logoUrl,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ManagerInfo struct {
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"assignedAt"`
}
