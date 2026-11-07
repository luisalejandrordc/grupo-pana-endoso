package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// withBase adds the settings every configuration needs.
func withBase(extra map[string]string) map[string]string {
	values := map[string]string{"DATABASE_URL": "postgres://localhost/db", "JWT_SECRET": testSecret}
	for key, value := range extra {
		values[key] = value
	}
	return values
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(withBase(nil)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != "local" || cfg.HTTPAddr != ":8080" || cfg.LogFormat != "text" ||
		cfg.LogLevel != slog.LevelInfo || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.AppBaseURL != "http://localhost:5173" || cfg.CookieSecure {
		t.Fatalf("unexpected local defaults for links and cookies: %+v", cfg)
	}
}

func TestLoadUsesPortWhenAddressIsNotSet(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{"PORT": "10000"})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":10000" {
		t.Fatalf("got %q, want :10000", cfg.HTTPAddr)
	}
}

func TestLoadExplicitAddressWinsOverPort(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{"PORT": "10000", "HTTP_ADDR": "127.0.0.1:9000"})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Fatalf("got %q", cfg.HTTPAddr)
	}
}

func TestLoadProductionDefaults(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{"ENV": "production", "APP_BASE_URL": "https://app.example.pe/"})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogFormat != "json" || !cfg.CookieSecure || cfg.AppBaseURL != "https://app.example.pe" {
		t.Fatalf("unexpected production defaults: %+v", cfg)
	}
}

func TestLoadRequiresBaseURLOutsideLocal(t *testing.T) {
	_, err := Load(env(withBase(map[string]string{"ENV": "production"})))
	if err == nil || !strings.Contains(err.Error(), "APP_BASE_URL is required") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCookieSecureCanBeOverridden(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{"COOKIE_SECURE": "true"})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.CookieSecure {
		t.Fatal("COOKIE_SECURE=true must win over the local default")
	}
}

func TestLoadTrustProxyDefaultsToFalse(t *testing.T) {
	cfg, err := Load(env(withBase(nil)))
	if err != nil || cfg.TrustProxy {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	cfg, err = Load(env(withBase(map[string]string{"TRUST_PROXY": "true"})))
	if err != nil || !cfg.TrustProxy {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoadRejectsAShortJWTSecret(t *testing.T) {
	_, err := Load(env(withBase(map[string]string{"JWT_SECRET": "too-short"})))
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	_, err := Load(env(map[string]string{"ENV": "nope", "LOG_LEVEL": "loud", "SHUTDOWN_TIMEOUT": "soon", "COOKIE_SECURE": "maybe"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{
		"DATABASE_URL is required", "JWT_SECRET", "ENV must be", "LOG_LEVEL must be",
		"SHUTDOWN_TIMEOUT must be", "COOKIE_SECURE must be",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestLoadStorageDefaultsToLocalFolder(t *testing.T) {
	cfg, err := Load(env(withBase(nil)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.R2Configured() || cfg.LocalMediaDir != "./var/media" || cfg.MediaBaseURL != "http://localhost:8080/media" {
		t.Fatalf("unexpected storage defaults: %+v", cfg)
	}
	if cfg.CodartToken != "" {
		t.Fatalf("Codart must be off without a token: %+v", cfg)
	}
}

func TestLoadAcceptsAFullR2Setup(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{
		"R2_ACCOUNT_ID": "acct", "R2_ACCESS_KEY_ID": "key", "R2_SECRET_ACCESS_KEY": "secret",
		"R2_BUCKET": "endosoflow", "R2_PUBLIC_BASE_URL": "https://img.example.pe/",
	})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.R2Configured() || cfg.R2PublicBaseURL != "https://img.example.pe" {
		t.Fatalf("unexpected R2 settings: %+v", cfg)
	}
}

func TestLoadRejectsAHalfConfiguredR2WithoutLeakingValues(t *testing.T) {
	_, err := Load(env(withBase(map[string]string{"R2_ACCOUNT_ID": "acct", "R2_SECRET_ACCESS_KEY": "super-secret-value"})))
	if err == nil || !strings.Contains(err.Error(), "R2_BUCKET") {
		t.Fatalf("a partial R2 setup must be reported: %v", err)
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("the error must not echo secrets: %v", err)
	}
}

func TestLoadRequiresADifferentPrivateBucketForDocuments(t *testing.T) {
	full := map[string]string{
		"R2_ACCOUNT_ID": "acct", "R2_ACCESS_KEY_ID": "key", "R2_SECRET_ACCESS_KEY": "secret",
		"R2_BUCKET": "endosoflow", "R2_PUBLIC_BASE_URL": "https://img.example.pe",
	}
	with := func(extra map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range full {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	cfg, err := Load(env(withBase(with(map[string]string{"R2_DOCUMENTS_BUCKET": " endosoflow-docs "}))))
	if err != nil || cfg.R2DocumentsBucket != "endosoflow-docs" {
		t.Fatalf("a separate documents bucket must be accepted and trimmed: %+v, %v", cfg, err)
	}
	if _, err := Load(env(withBase(with(map[string]string{"R2_DOCUMENTS_BUCKET": "endosoflow"})))); err == nil ||
		!strings.Contains(err.Error(), "R2_DOCUMENTS_BUCKET") {
		t.Fatalf("the public bucket must be refused for documents: %v", err)
	}
	if _, err := Load(env(withBase(map[string]string{"R2_DOCUMENTS_BUCKET": "docs"}))); err == nil ||
		!strings.Contains(err.Error(), "R2_DOCUMENTS_BUCKET") {
		t.Fatalf("a documents bucket without the rest of R2 must be refused: %v", err)
	}
}
