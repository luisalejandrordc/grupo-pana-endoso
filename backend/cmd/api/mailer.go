package main

import (
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/platform/config"
	"log/slog"
)

// newMailer builds the email sender every module uses and says whether a real delivery service stands behind
// it (the notification dispatcher only runs when one does).
//
//   - With MAIL_APPS_SCRIPT_URL set, email goes out through the Google Apps Script web app. When an allow-list is
//     configured (required in staging) nothing is sent to anyone outside it.
//   - Without it, local and test environments only write emails to the log, so links can be copied from there.
//   - Anywhere else sending fails loudly instead of silently dropping messages.
func newMailer(cfg config.Config, logger *slog.Logger) (sender mail.Sender, delivers bool) {
	switch {
	case cfg.MailAppsScriptURL != "":
		sender = mail.NewAppsScript(mail.AppsScriptConfig{URL: cfg.MailAppsScriptURL, Secret: cfg.MailAppsScriptSecret})
		if len(cfg.MailAllowedRecipients) > 0 {
			sender = mail.Restrict(sender, mail.NewAllowList(cfg.MailAllowedRecipients))
			logger.Info("email is restricted to an allow-list", "entries", len(cfg.MailAllowedRecipients))
		}
		return sender, true
	case cfg.Env == "local" || cfg.Env == "test":
		logger.Warn("MAIL_APPS_SCRIPT_URL is not set: emails are written to the log instead of being sent")
		return mail.NewLogSender(logger), true
	default:
		logger.Warn("MAIL_APPS_SCRIPT_URL is not set: emails cannot be sent and notifications will wait in the queue")
		return mail.Unconfigured(), false
	}
}
