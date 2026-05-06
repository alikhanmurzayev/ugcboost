package service

// Audit action names. Every write-path service operation records one of
// these values when appending to the audit log.
const (
	AuditActionLogin                                = "login"
	AuditActionLogout                               = "logout"
	AuditActionPasswordReset                        = "password_reset"
	AuditActionBrandCreate                          = "brand_create"
	AuditActionBrandUpdate                          = "brand_update"
	AuditActionBrandDelete                          = "brand_delete"
	AuditActionManagerAssign                        = "manager_assign"
	AuditActionManagerRemove                        = "manager_remove"
	AuditActionCreatorApplicationSubmit             = "creator_application_submit"
	AuditActionCreatorApplicationLinkTelegram       = "creator_application_link_telegram"
	AuditActionCreatorApplicationVerificationAuto   = "creator_application_verification_auto"
	AuditActionCreatorApplicationVerificationManual = "creator_application_verification_manual"
	AuditActionCreatorApplicationReject             = "creator_application_reject"
	AuditActionCreatorApplicationApprove            = "creator_application_approve"
	AuditActionCampaignCreate                       = "campaign_create"
)

// Audit entity types used alongside AuditAction* values.
const (
	AuditEntityTypeUser               = "user"
	AuditEntityTypeBrand              = "brand"
	AuditEntityTypeCreatorApplication = "creator_application"
	AuditEntityTypeCampaign           = "campaign"
)
