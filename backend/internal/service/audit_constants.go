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
	AuditActionCampaignUpdate                       = "campaign_update"
	AuditActionCampaignCreatorAdd                   = "campaign_creator_add"
	AuditActionCampaignCreatorRemove                = "campaign_creator_remove"
	AuditActionCampaignCreatorInvite                = "campaign_creator_invite"
	AuditActionCampaignCreatorRemind                = "campaign_creator_remind"
	AuditActionCampaignCreatorAgree                 = "campaign_creator_agree"
	AuditActionCampaignCreatorDecline               = "campaign_creator_decline"
)

// Audit entity types used alongside AuditAction* values.
const (
	AuditEntityTypeUser               = "user"
	AuditEntityTypeBrand              = "brand"
	AuditEntityTypeCreatorApplication = "creator_application"
	AuditEntityTypeCampaign           = "campaign"
	AuditEntityTypeCampaignCreator    = "campaign_creator"
)
