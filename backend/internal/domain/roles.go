package domain

// UserRole represents user roles matching the OpenAPI spec.
type UserRole string

const (
	RoleAdmin        UserRole = "admin"
	RoleBrandManager UserRole = "brand_manager"
)
