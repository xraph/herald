package template

import (
	"github.com/xraph/herald/id"
)

// DefaultTemplates returns the default notification templates to seed per app.
func DefaultTemplates(appID string) []*Template {
	return []*Template{
		// ═══════════════════════════════════════════════════
		// Email Templates
		// ═══════════════════════════════════════════════════

		// ─── Auth ────────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.welcome",
			Name: "Welcome Email", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true, Description: "User's display name"},
				{Name: "app_name", Type: "string", Required: true, Description: "Application name"},
				{Name: "login_url", Type: "url", Required: false, Description: "Login URL"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Welcome to {{.app_name}}!",
				HTML:    welcomeEmailHTML,
				Text:    "Hi {{.user_name}},\n\nWelcome to {{.app_name}}! We're excited to have you on board.\n\n{{if .login_url}}Log in at: {{.login_url}}{{end}}",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.email-verification",
			Name: "Email Verification", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "verify_url", Type: "url", Required: true},
				{Name: "code", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Verify your email for {{.app_name}}",
				HTML:    verificationEmailHTML,
				Text:    "Hi {{.user_name}},\n\nPlease verify your email for {{.app_name}}.\n\nVerification link: {{.verify_url}}\n{{if .code}}Or use code: {{.code}}{{end}}",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.password-reset",
			Name: "Password Reset", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "reset_url", Type: "url", Required: true},
				{Name: "expires_in", Type: "string", Required: false, Default: "1 hour"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Reset your {{.app_name}} password",
				HTML:    passwordResetEmailHTML,
				Text:    "Hi {{.user_name}},\n\nWe received a request to reset your {{.app_name}} password.\n\nReset link: {{.reset_url}}\n\nThis link expires in {{.expires_in}}.\n\nIf you didn't request this, you can safely ignore this email.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.password-changed",
			Name: "Password Changed", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} password was changed",
				HTML:    passwordChangedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour password for {{.app_name}} has been successfully changed.\n\nIf you didn't make this change, please contact support immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.invitation",
			Name: "Organization Invitation", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "inviter_name", Type: "string", Required: true},
				{Name: "org_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "accept_url", Type: "url", Required: true},
				{Name: "role", Type: "string", Required: false, Default: "member"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "You've been invited to {{.org_name}} on {{.app_name}}",
				HTML:    invitationEmailHTML,
				Text:    "Hi,\n\n{{.inviter_name}} has invited you to join {{.org_name}} on {{.app_name}} as a {{.role}}.\n\nAccept the invitation: {{.accept_url}}",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.account-locked",
			Name: "Account Locked Alert", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "unlock_url", Type: "url", Required: false},
				{Name: "attempts", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} account has been locked",
				HTML:    accountLockedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour {{.app_name}} account has been temporarily locked due to multiple failed login attempts{{if .attempts}} ({{.attempts}} attempts){{end}}.\n\n{{if .unlock_url}}Unlock your account: {{.unlock_url}}{{else}}Please contact support to unlock your account.{{end}}",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.email-verified",
			Name: "Email Verified", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} email has been verified",
				HTML:    emailVerifiedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour email address for {{.app_name}} has been successfully verified. You now have full access to your account.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-disabled",
			Name: "MFA Disabled Alert", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Two-factor authentication disabled on {{.app_name}}",
				HTML:    mfaDisabledEmailHTML,
				Text:    "Hi {{.user_name}},\n\nTwo-factor authentication has been disabled on your {{.app_name}} account.\n\nIf you didn't make this change, please secure your account immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-recovery-used",
			Name: "Recovery Code Used", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "A recovery code was used on your {{.app_name}} account",
				HTML:    mfaRecoveryUsedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nA recovery code was used to sign in to your {{.app_name}} account. If this wasn't you, please secure your account immediately.\n\nWe recommend generating new recovery codes in your account settings.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-recovery-regenerated",
			Name: "Recovery Codes Regenerated", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "New recovery codes generated for {{.app_name}}",
				Text:    "Hi {{.user_name}},\n\nNew recovery codes have been generated for your {{.app_name}} account. Your previous recovery codes are no longer valid.\n\nPlease store the new codes in a safe place.",
			}},
		},

		// ─── Security ────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "security.signin-alert",
			Name: "Sign-In Alert", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "device_info", Type: "string", Required: false},
				{Name: "location", Type: "string", Required: false},
				{Name: "time", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "New sign-in to your {{.app_name}} account",
				HTML:    signinAlertEmailHTML,
				Text:    "Hi {{.user_name}},\n\nA new sign-in was detected on your {{.app_name}} account.\n\n{{if .device_info}}Device: {{.device_info}}{{end}}\n{{if .location}}Location: {{.location}}{{end}}\n{{if .time}}Time: {{.time}}{{end}}\n\nIf this wasn't you, please secure your account immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "security.session-revoked",
			Name: "Session Revoked", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "A session was revoked on your {{.app_name}} account",
				Text:    "Hi {{.user_name}},\n\nA session on your {{.app_name}} account has been revoked. If this wasn't you, please change your password immediately.",
			}},
		},

		// ─── User ────────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "user.account-deleted",
			Name: "Account Deleted", Channel: "email", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} account has been deleted",
				HTML:    accountDeletedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour {{.app_name}} account has been successfully deleted. All your data has been removed.\n\nIf you didn't request this, please contact support immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "user.data-export-ready",
			Name: "Data Export Ready", Channel: "email", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "download_url", Type: "url", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} data export is ready",
				HTML:    dataExportReadyEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour data export from {{.app_name}} is ready for download.\n\n{{if .download_url}}Download your data: {{.download_url}}{{else}}Please log in to your account to download your data.{{end}}",
			}},
		},

		// ─── Admin ───────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "admin.user-banned",
			Name: "Account Suspended", Channel: "email", Category: CategorySystem,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "reason", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} account has been suspended",
				HTML:    userBannedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYour {{.app_name}} account has been suspended.{{if .reason}}\n\nReason: {{.reason}}{{end}}\n\nIf you believe this was a mistake, please contact support.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "admin.user-unbanned",
			Name: "Account Reinstated", Channel: "email", Category: CategorySystem,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your {{.app_name}} account has been reinstated",
				Text:    "Hi {{.user_name}},\n\nYour {{.app_name}} account has been reinstated. You can now log in and use the service as usual.",
			}},
		},

		// ─── Organization ────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.member-added",
			Name: "Added to Organization", Channel: "email", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "org_name", Type: "string", Required: true},
				{Name: "role", Type: "string", Required: false, Default: "member"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "You've been added to {{.org_name}} on {{.app_name}}",
				HTML:    orgMemberAddedEmailHTML,
				Text:    "Hi {{.user_name}},\n\nYou've been added to {{.org_name}} on {{.app_name}} as a {{.role}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.member-removed",
			Name: "Removed from Organization", Channel: "email", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "org_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "You've been removed from {{.org_name}} on {{.app_name}}",
				Text:    "Hi {{.user_name}},\n\nYou've been removed from {{.org_name}} on {{.app_name}}. If you believe this was a mistake, please contact an organization administrator.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.role-changed",
			Name: "Role Changed", Channel: "email", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "org_name", Type: "string", Required: true},
				{Name: "old_role", Type: "string", Required: false},
				{Name: "new_role", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "Your role in {{.org_name}} has changed",
				Text:    "Hi {{.user_name}},\n\nYour role in {{.org_name}} on {{.app_name}} has been changed{{if .old_role}} from {{.old_role}}{{end}} to {{.new_role}}.",
			}},
		},

		// ─── Credentials ─────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "credential.registered",
			Name: "Credential Registered", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "credential_type", Type: "string", Required: true, Description: "e.g. passkey, api_key"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "New {{.credential_type}} added to your {{.app_name}} account",
				Text:    "Hi {{.user_name}},\n\nA new {{.credential_type}} has been registered on your {{.app_name}} account.\n\nIf you didn't make this change, please secure your account immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "credential.removed",
			Name: "Credential Removed", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "credential_type", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "A {{.credential_type}} was removed from your {{.app_name}} account",
				Text:    "Hi {{.user_name}},\n\nA {{.credential_type}} has been removed from your {{.app_name}} account.\n\nIf you didn't make this change, please secure your account immediately.",
			}},
		},

		// ═══════════════════════════════════════════════════
		// SMS Templates
		// ═══════════════════════════════════════════════════
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-code",
			Name: "MFA Verification Code", Channel: "sms", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "code", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "expires_in", Type: "string", Required: false, Default: "5 minutes"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Text: "Your {{.app_name}} verification code is: {{.code}}. Expires in {{.expires_in}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.phone-verification",
			Name: "Phone Verification Code", Channel: "sms", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "code", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Text: "Your {{.app_name}} phone verification code is: {{.code}}",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.account-locked",
			Name: "Account Locked SMS", Channel: "sms", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Text: "Your {{.app_name}} account has been locked due to too many failed login attempts. Please reset your password or contact support.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-challenge",
			Name: "MFA Challenge Code", Channel: "sms", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "code", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "expires_in", Type: "string", Required: false, Default: "5 minutes"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Text: "{{.app_name}} login code: {{.code}}. Expires in {{.expires_in}}. Do not share this code.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "security.signin-alert",
			Name: "Sign-In Alert SMS", Channel: "sms", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
				{Name: "device_info", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Text: "New sign-in to your {{.app_name}} account{{if .device_info}} from {{.device_info}}{{end}}. Not you? Secure your account now.",
			}},
		},

		// ═══════════════════════════════════════════════════
		// In-App Templates
		// ═══════════════════════════════════════════════════

		// ─── Auth ────────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.welcome",
			Name: "Welcome Notification", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Welcome to {{.app_name}}!",
				Text:  "Hi {{.user_name}}, welcome aboard! Take a look around to get started.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.password-changed",
			Name: "Password Changed Alert", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Password Changed",
				Text:  "Your {{.app_name}} password was recently changed. If this wasn't you, please take action.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.account-locked",
			Name: "Account Locked", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Account Locked",
				Text:  "Your {{.app_name}} account has been locked due to too many failed login attempts.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.email-verified",
			Name: "Email Verified", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Email Verified",
				Text:  "Your email for {{.app_name}} has been successfully verified.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-disabled",
			Name: "MFA Disabled", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Two-Factor Auth Disabled",
				Text:  "Two-factor authentication has been disabled on your {{.app_name}} account.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.mfa-recovery-used",
			Name: "Recovery Code Used", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Recovery Code Used",
				Text:  "A recovery code was used to sign in to your {{.app_name}} account. Consider generating new codes.",
			}},
		},

		// ─── Security ────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "security.signin-alert",
			Name: "New Sign-In", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
				{Name: "device_info", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "New Sign-In Detected",
				Text:  "A new sign-in was detected on your {{.app_name}} account{{if .device_info}} from {{.device_info}}{{end}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "security.session-revoked",
			Name: "Session Revoked", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Session Revoked",
				Text:  "A session on your {{.app_name}} account has been revoked.",
			}},
		},

		// ─── User ────────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "user.profile-updated",
			Name: "Profile Updated", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Profile Updated",
				Text:  "Your {{.app_name}} profile has been updated successfully.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "user.account-deleted",
			Name: "Account Deleted", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Account Deleted",
				Text:  "Your {{.app_name}} account has been deleted.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "user.data-export-ready",
			Name: "Data Export Ready", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Data Export Ready",
				Text:  "Your {{.app_name}} data export is ready for download.",
			}},
		},

		// ─── Admin ───────────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "admin.user-banned",
			Name: "Account Suspended", Channel: "inapp", Category: CategorySystem,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Account Suspended",
				Text:  "Your {{.app_name}} account has been suspended. Please contact support.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "admin.user-unbanned",
			Name: "Account Reinstated", Channel: "inapp", Category: CategorySystem,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Account Reinstated",
				Text:  "Your {{.app_name}} account has been reinstated. Welcome back!",
			}},
		},

		// ─── Organization ────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.created",
			Name: "Organization Created", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "org_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Organization Created",
				Text:  "Organization \"{{.org_name}}\" has been created successfully.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.deleted",
			Name: "Organization Deleted", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "org_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Organization Deleted",
				Text:  "Organization \"{{.org_name}}\" has been deleted.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.member-added",
			Name: "Added to Organization", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "org_name", Type: "string", Required: true},
				{Name: "role", Type: "string", Required: false, Default: "member"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Added to Organization",
				Text:  "You've been added to {{.org_name}} as a {{.role}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.member-removed",
			Name: "Removed from Organization", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "org_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Removed from Organization",
				Text:  "You've been removed from {{.org_name}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "org.role-changed",
			Name: "Role Changed", Channel: "inapp", Category: CategoryTransactional,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "org_name", Type: "string", Required: true},
				{Name: "new_role", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Role Changed",
				Text:  "Your role in {{.org_name}} has been changed to {{.new_role}}.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.invitation",
			Name: "Organization Invitation", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "inviter_name", Type: "string", Required: true},
				{Name: "org_name", Type: "string", Required: true},
				{Name: "role", Type: "string", Required: false, Default: "member"},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Organization Invitation",
				Text:  "{{.inviter_name}} invited you to join {{.org_name}} as a {{.role}}.",
			}},
		},

		// ─── Credentials ─────────────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "credential.registered",
			Name: "Credential Registered", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "credential_type", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Credential Added",
				Text:  "A new {{.credential_type}} has been registered on your {{.app_name}} account.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "credential.removed",
			Name: "Credential Removed", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "credential_type", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "Credential Removed",
				Text:  "A {{.credential_type}} has been removed from your {{.app_name}} account.",
			}},
		},

		// ─── Legacy compatibility ────────────────────────
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.new-device-login",
			Name: "New Device Login Alert", Channel: "email", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "user_name", Type: "string", Required: true},
				{Name: "app_name", Type: "string", Required: true},
				{Name: "device_info", Type: "string", Required: true},
				{Name: "location", Type: "string", Required: false},
				{Name: "time", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Subject: "New sign-in to your {{.app_name}} account",
				Text:    "Hi {{.user_name}},\n\nA new sign-in was detected on your {{.app_name}} account.\n\nDevice: {{.device_info}}\n{{if .location}}Location: {{.location}}{{end}}\n{{if .time}}Time: {{.time}}{{end}}\n\nIf this wasn't you, please secure your account immediately.",
			}},
		},
		{
			ID: id.NewTemplateID(), AppID: appID, Slug: "auth.new-device-login",
			Name: "New Device Login Alert", Channel: "inapp", Category: CategoryAuth,
			IsSystem: true, Enabled: true,
			Variables: []Variable{
				{Name: "device_info", Type: "string", Required: true},
				{Name: "location", Type: "string", Required: false},
			},
			Versions: []Version{{
				ID: id.NewTemplateVersionID(), Locale: "en", Active: true,
				Title: "New Sign-In Detected",
				Text:  "A new sign-in was detected from {{.device_info}}{{if .location}} in {{.location}}{{end}}.",
			}},
		},
	}
}

// ═══════════════════════════════════════════════════
// Email HTML Templates
// ═══════════════════════════════════════════════════

const emailBodyStyle = `font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;`
const emailBtnStyle = `display: inline-block; padding: 12px 24px; background-color: #4F46E5; color: #fff; text-decoration: none; border-radius: 6px;`
const emailFooterStyle = `color: #666; font-size: 14px;`

var welcomeEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Welcome to {{.app_name}}!</h2>
<p>Hi {{.user_name}},</p>
<p>We're excited to have you on board. Your account has been created successfully.</p>
{{if .login_url}}<p><a href="{{.login_url}}" style="` + emailBtnStyle + `">Log In</a></p>{{end}}
<p style="` + emailFooterStyle + `">If you didn't create this account, please ignore this email.</p>
</body></html>`

var verificationEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Verify Your Email</h2>
<p>Hi {{.user_name}},</p>
<p>Please verify your email address for {{.app_name}} by clicking the button below.</p>
<p><a href="{{.verify_url}}" style="` + emailBtnStyle + `">Verify Email</a></p>
{{if .code}}<p>Or enter this code: <strong>{{.code}}</strong></p>{{end}}
<p style="` + emailFooterStyle + `">If you didn't request this, you can safely ignore this email.</p>
</body></html>`

var passwordResetEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Reset Your Password</h2>
<p>Hi {{.user_name}},</p>
<p>We received a request to reset your {{.app_name}} password.</p>
<p><a href="{{.reset_url}}" style="` + emailBtnStyle + `">Reset Password</a></p>
<p>This link expires in {{.expires_in}}.</p>
<p style="` + emailFooterStyle + `">If you didn't request a password reset, you can safely ignore this email.</p>
</body></html>`

var passwordChangedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Password Changed</h2>
<p>Hi {{.user_name}},</p>
<p>Your password for {{.app_name}} has been successfully changed.</p>
<p style="` + emailFooterStyle + `">If you didn't make this change, please contact support immediately.</p>
</body></html>`

var invitationEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">You're Invited!</h2>
<p>{{.inviter_name}} has invited you to join <strong>{{.org_name}}</strong> on {{.app_name}} as a {{.role}}.</p>
<p><a href="{{.accept_url}}" style="` + emailBtnStyle + `">Accept Invitation</a></p>
<p style="` + emailFooterStyle + `">If you weren't expecting this invitation, you can safely ignore this email.</p>
</body></html>`

var accountLockedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #D32F2F;">Account Locked</h2>
<p>Hi {{.user_name}},</p>
<p>Your {{.app_name}} account has been temporarily locked due to multiple failed login attempts{{if .attempts}} ({{.attempts}} attempts){{end}}.</p>
{{if .unlock_url}}<p><a href="{{.unlock_url}}" style="` + emailBtnStyle + `">Unlock Account</a></p>{{else}}<p>Please contact support to unlock your account.</p>{{end}}
<p style="` + emailFooterStyle + `">If you didn't attempt to sign in, someone else may be trying to access your account.</p>
</body></html>`

var emailVerifiedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #2E7D32;">Email Verified</h2>
<p>Hi {{.user_name}},</p>
<p>Your email address for {{.app_name}} has been successfully verified. You now have full access to your account.</p>
<p style="` + emailFooterStyle + `">Thank you for verifying your email.</p>
</body></html>`

var mfaDisabledEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #D32F2F;">Two-Factor Authentication Disabled</h2>
<p>Hi {{.user_name}},</p>
<p>Two-factor authentication has been disabled on your {{.app_name}} account.</p>
<p style="` + emailFooterStyle + `">If you didn't make this change, please secure your account immediately by changing your password and re-enabling two-factor authentication.</p>
</body></html>`

var mfaRecoveryUsedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #E65100;">Recovery Code Used</h2>
<p>Hi {{.user_name}},</p>
<p>A recovery code was used to sign in to your {{.app_name}} account.</p>
<p>If this wasn't you, please secure your account immediately.</p>
<p style="` + emailFooterStyle + `">We recommend generating new recovery codes in your account settings.</p>
</body></html>`

var signinAlertEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">New Sign-In Detected</h2>
<p>Hi {{.user_name}},</p>
<p>A new sign-in was detected on your {{.app_name}} account.</p>
<table style="border-collapse: collapse; margin: 16px 0;">
{{if .device_info}}<tr><td style="padding: 4px 12px 4px 0; color: #666;">Device</td><td style="padding: 4px 0;">{{.device_info}}</td></tr>{{end}}
{{if .location}}<tr><td style="padding: 4px 12px 4px 0; color: #666;">Location</td><td style="padding: 4px 0;">{{.location}}</td></tr>{{end}}
{{if .time}}<tr><td style="padding: 4px 12px 4px 0; color: #666;">Time</td><td style="padding: 4px 0;">{{.time}}</td></tr>{{end}}
</table>
<p style="` + emailFooterStyle + `">If this wasn't you, please secure your account immediately.</p>
</body></html>`

var accountDeletedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Account Deleted</h2>
<p>Hi {{.user_name}},</p>
<p>Your {{.app_name}} account has been successfully deleted. All your data has been removed.</p>
<p style="` + emailFooterStyle + `">If you didn't request this deletion, please contact support immediately.</p>
</body></html>`

var dataExportReadyEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Your Data Export is Ready</h2>
<p>Hi {{.user_name}},</p>
<p>Your data export from {{.app_name}} is ready for download.</p>
{{if .download_url}}<p><a href="{{.download_url}}" style="` + emailBtnStyle + `">Download Data</a></p>{{else}}<p>Please log in to your account to download your data.</p>{{end}}
<p style="` + emailFooterStyle + `">This download link may expire. Please download your data promptly.</p>
</body></html>`

var userBannedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #D32F2F;">Account Suspended</h2>
<p>Hi {{.user_name}},</p>
<p>Your {{.app_name}} account has been suspended.{{if .reason}}</p>
<p><strong>Reason:</strong> {{.reason}}</p>{{else}}</p>{{end}}
<p style="` + emailFooterStyle + `">If you believe this was a mistake, please contact support.</p>
</body></html>`

var orgMemberAddedEmailHTML = `<!DOCTYPE html>
<html><body style="` + emailBodyStyle + `">
<h2 style="color: #333;">Added to Organization</h2>
<p>Hi {{.user_name}},</p>
<p>You've been added to <strong>{{.org_name}}</strong> on {{.app_name}} as a {{.role}}.</p>
<p style="` + emailFooterStyle + `">You can now access the organization's resources and settings.</p>
</body></html>`
