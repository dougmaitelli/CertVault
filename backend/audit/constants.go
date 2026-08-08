package audit

const (
	ActorAdmin          = "admin"
	ActorBootstrapAdmin = "bootstrap-admin"
	ActorLocalCLI       = "local-cli"
	ActorSystem         = "system"
)

const (
	ActionACMEAccountDelete    = "acme_account.delete"
	ActionAPIKeyCreate         = "api_key.create"
	ActionAPIKeyDelete         = "api_key.delete"
	ActionAPIKeyRevoke         = "api_key.revoke"
	ActionAuthLogin            = "auth.login"
	ActionCertificateDownload  = "certificate.download"
	ActionCertificateInitial   = "certificate.initial"
	ActionCertificateManual    = "certificate.manual"
	ActionCertificateScheduled = "certificate.scheduled"
	ActionRenewalTrigger       = "renewal.trigger"
)

const (
	DetailAuthBootstrap = "bootstrap"
	DetailAuthOIDC      = "oidc"
)

const ResourceUI = "ui"
