package service

// Audit action names. Every write-path service operation records one of
// these values when appending to the audit log.
const (
	AuditActionLogin          = "login"
	AuditActionLogout         = "logout"
	AuditActionPasswordReset  = "password_reset"
	AuditActionBrandCreate    = "brand_create"
	AuditActionBrandUpdate    = "brand_update"
	AuditActionBrandDelete    = "brand_delete"
	AuditActionManagerAssign  = "manager_assign"
	AuditActionManagerRemove  = "manager_remove"
)

// Audit entity types used alongside AuditAction* values.
const (
	AuditEntityTypeUser  = "user"
	AuditEntityTypeBrand = "brand"
)
