package domain

import "time"

// Brand is the domain representation of a brand.
type Brand struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	LogoURL   *string   `json:"logo_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BrandListItem is a brand with manager count for list views.
type BrandListItem struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	LogoURL      *string   `json:"logo_url"`
	ManagerCount int       `json:"manager_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BrandManager represents a user assigned as a brand manager.
type BrandManager struct {
	UserID     string    `json:"user_id"`
	Email      string    `json:"email"`
	AssignedAt time.Time `json:"assigned_at"`
}
