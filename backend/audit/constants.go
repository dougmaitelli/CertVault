package audit

type Actor string

const (
	ActorAdmin          Actor = "admin"
	ActorBootstrapAdmin Actor = "bootstrap-admin"
	ActorLocalCLI       Actor = "local-cli"
	ActorSystem         Actor = "system"
)

type Action string

const (
	ActionACMEAccountDelete    Action = "acme_account.delete"
	ActionAPIKeyCreate         Action = "api_key.create"
	ActionAPIKeyDelete         Action = "api_key.delete"
	ActionAPIKeyRevoke         Action = "api_key.revoke"
	ActionAuthLogin            Action = "auth.login"
	ActionCertificateDownload  Action = "certificate.download"
	ActionCertificateInitial   Action = "certificate.initial"
	ActionCertificateManual    Action = "certificate.manual"
	ActionCertificateScheduled Action = "certificate.scheduled"
	ActionRenewalTrigger       Action = "renewal.trigger"
)

const (
	DetailAuthBootstrap = "bootstrap"
	DetailAuthOIDC      = "oidc"
)

const ResourceUI = "ui"
