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
	if cfg.AppBaseURL != "http://localhost:5173" || cfg.CookieSecure || cfg.TrustProxy {
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
		t.Fatalf("got %q, want 127.0.0.1:9000", cfg.HTTPAddr)
	}
}

func TestLoadProductionDefaults(t *testing.T) {
	cfg, err := Load(env(withBase(map[string]string{"ENV": "production", "APP_BASE_URL": "https://app.example.pe/"})))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogFormat != "json" || !cfg.CookieSecure || !cfg.TrustProxy || cfg.AppBaseURL != "https://app.example.pe" {
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

func fullR2() map[string]string {
	return map[string]string{
		"R2_ACCOUNT_ID":                  "account",
		"R2_CATALOG_BUCKET":              "endosoflow-catalog-images",
		"R2_CATALOG_ACCESS_KEY_ID":       "catalog-access-key-id",
		"R2_CATALOG_SECRET_ACCESS_KEY":   "catalog-secret-access-key",
		"R2_CATALOG_PUBLIC_BASE_URL":     "https://img.example.pe/",
		"R2_DOCUMENTS_BUCKET":            "endosoflow-sale-documents",
		"R2_DOCUMENTS_ACCESS_KEY_ID":     "documents-access-key-id",
		"R2_DOCUMENTS_SECRET_ACCESS_KEY": "documents-secret-access-key",
		"R2_AVATARS_BUCKET":              "endosoflow-employee-avatars",
		"R2_AVATARS_ACCESS_KEY_ID":       "avatars-access-key-id",
		"R2_AVATARS_SECRET_ACCESS_KEY":   "avatars-secret-access-key",
	}
}

func TestLoadAcceptsAFullR2Setup(t *testing.T) {
	cfg, err := Load(env(withBase(fullR2())))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.R2Configured() {
		t.Fatalf("unexpected R2 settings: %+v", cfg)
	}
}

func TestLoadRejectsAHalfConfiguredR2WithoutLeakingValues(t *testing.T) {
	_, err := Load(env(withBase(map[string]string{
		"R2_ACCOUNT_ID":                "account",
		"R2_CATALOG_SECRET_ACCESS_KEY": "catalog-secret-access-key",
	})))
	if err == nil ||
		!strings.Contains(err.Error(), "R2_CATALOG") ||
		!strings.Contains(err.Error(), "R2_DOCUMENTS") ||
		!strings.Contains(err.Error(), "R2_AVATARS") {

		t.Fatalf("a partial R2 setup must be reported: %v", err)
	}
	if strings.Contains(err.Error(), "catalog-secret-access-key") {
		t.Fatalf("the error must not echo secrets: %v", err)
	}
}

func TestLoadRequiresADifferentPrivateBucketForDocumentsAndAvatars(t *testing.T) {
	full := fullR2()
	full["R2_DOCUMENTS_BUCKET"] = " endosoflow-sale-documents "
	cfg, err := Load(env(withBase(full)))
	if err != nil || cfg.R2DocumentsBucket != "endosoflow-sale-documents" {
		t.Fatalf("a separate documents bucket must be accepted and trimmed: %+v, %v", cfg, err)
	}
	full["R2_DOCUMENTS_BUCKET"] = "endosoflow-catalog-images"
	full["R2_AVATARS_BUCKET"] = "endosoflow-catalog-images"
	cfg, err = Load(env(withBase(full)))
	if err == nil ||
		!strings.Contains(err.Error(), "R2_DOCUMENTS_BUCKET") ||
		!strings.Contains(err.Error(), "R2_AVATARS_BUCKET") {
		t.Fatalf("the public bucket must be refused for documents and avatars: %v", err)
	}
	cfg, err = Load(env(withBase(map[string]string{"R2_DOCUMENTS_BUCKET": "endosoflow-sale-documents"})))
	if err == nil || !strings.Contains(err.Error(), "R2_DOCUMENTS") {
		t.Fatalf("a documents bucket without the rest of R2 must be refused: %v", err)
	}
	cfg, err = Load(env(withBase(map[string]string{"R2_AVATARS_BUCKET": "endosoflow-employee-avatars"})))
	if err == nil || !strings.Contains(err.Error(), "R2_AVATARS") {
		t.Fatalf("an avatars bucket without the rest of R2 must be refused: %v", err)
	}
}

func TestBucketMisconfigurationsAreRefused(t *testing.T) {
	cases := map[string]map[string]string{
		"avatars bucket equal to the documents bucket": {"R2_DOCUMENTS_BUCKET": "same", "R2_AVATARS_BUCKET": "same"},
		"half of a documents token":                    {"R2_DOCUMENTS_BUCKET": "docs", "R2_DOCUMENTS_ACCESS_KEY_ID": "only-id", "R2_DOCUMENTS_SECRET_ACCESS_KEY": ""},
		"a documents token without its bucket":         {"R2_DOCUMENTS_BUCKET": "", "R2_DOCUMENTS_ACCESS_KEY_ID": "id", "R2_DOCUMENTS_SECRET_ACCESS_KEY": "s"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			vars := fullR2()
			for k, v := range extra {
				vars[k] = v
			}
			_, err := Load(env(withBase(vars)))
			if err == nil {
				t.Fatal("the configuration must be refused")
			}
			if strings.Contains(err.Error(), "secret") && strings.Contains(err.Error(), "only-id") {
				t.Fatalf("the error must not echo values: %v", err)
			}
		})
	}
}
