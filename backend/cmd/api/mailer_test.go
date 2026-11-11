package main

import (
	"context"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/platform/config"
	"errors"
	"testing"
)

func TestMailerSelection(t *testing.T) {
	logger := quiet()
	_, delivers := newMailer(config.Config{Env: "production"}, logger)
	if delivers {
		t.Fatal("without an email service nothing is delivered and the dispatcher must not run")
	}
	unconfigured, _ := newMailer(config.Config{Env: "production"}, logger)
	if err := unconfigured.Send(context.Background(), mail.Message{To: "a@b.pe"}); !errors.Is(err, mail.ErrNotConfigured) {
		t.Fatalf("an unconfigured production sender fails loudly: %v", err)
	}
	if _, delivers := newMailer(config.Config{Env: "local"}, logger); !delivers {
		t.Fatal("local development logs emails, so the dispatcher runs against the log sender")
	}

	real := config.Config{Env: "production", MailAppsScriptURL: "https://script.google.com/macros/s/ID/exec", MailAppsScriptSecret: "s"}
	if _, delivers := newMailer(real, logger); !delivers {
		t.Fatal("with the web app configured email is delivered")
	}
	real.MailAllowedRecipients = []string{"@grupopana.pe"}
	restricted, _ := newMailer(real, logger)
	if err := restricted.Send(context.Background(), mail.Message{To: "cliente@gmail.com"}); !errors.Is(err, mail.ErrRecipientNotAllowed) {
		t.Fatalf("an allow-list protects every module that sends email: %v", err)
	}
}
