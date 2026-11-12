// Package config reads the application's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting the API needs. It is built once at startup.
type Config struct {
	// Application
	Env        string // local, test, staging or production
	AppBaseURL string

	// HTTP server
	HTTPAddr        string
	ShutdownTimeout time.Duration // max time to gracefully shut down the server
	TrustProxy      bool          // read the client IP from X-Forwarded-For

	// Logging
	LogLevel  slog.Level // debug, info, warn or error
	LogFormat string     // text (readable) or json (for hosting platforms)

	// Database
	DatabaseURL string

	// Authentication
	JWTSecret    string
	CookieSecure bool // refresh cookie "Secure" flag

	// Codart integration
	CodartToken   string
	CodartBaseURL string

	// Cloudflare R2
	R2AccountID string
	R2Endpoint  string // optional override, for tests

	// R2 catalog bucket
	R2CatalogBucket          string
	R2CatalogAccessKeyID     string
	R2CatalogSecretAccessKey string
	R2CatalogPublicBaseURL   string

	// R2 documents bucket
	R2DocumentsBucket          string
	R2DocumentsAccessKeyID     string
	R2DocumentsSecretAccessKey string

	// R2 avatars bucket
	R2AvatarsBucket          string
	R2AvatarsAccessKeyID     string
	R2AvatarsSecretAccessKey string

	// Local media
	LocalMediaDir string
	MediaBaseURL  string

	// Email delivery
	MailAppsScriptURL     string // Google Apps Script web app URL; empty to log emails
	MailAppsScriptSecret  string
	MailAllowedRecipients []string      // allowed emails or domains; required in staging
	MailDispatchInterval  time.Duration // notification dispatch interval
}

// R2Configured reports whether Cloudflare R2 should be used.
func (c Config) R2Configured() bool { return c.R2AccountID != "" }

var validEnvs = map[string]bool{"local": true, "test": true, "staging": true, "production": true}

// Load builds the configuration from the given lookup function (os.Getenv in production,
// a map in tests). It reports every problem at once instead of stopping at the first.
func Load(getenv func(string) string) (Config, error) {
	var problems []string
	var cfg Config

	cfg.Env = valueOr(getenv("ENV"), "local")
	if !validEnvs[cfg.Env] {
		problems = append(problems, fmt.Sprintf("ENV must be local, test, staging or production (got %q)", cfg.Env))
	}

	cfg.HTTPAddr = getenv("HTTP_ADDR")
	if cfg.HTTPAddr == "" {
		// Render and similar hosts provide the port in PORT.
		if port := getenv("PORT"); port != "" {
			cfg.HTTPAddr = ":" + port
		} else {
			cfg.HTTPAddr = ":8080"
		}
	}

	cfg.DatabaseURL = getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}

	levelText := valueOr(getenv("LOG_LEVEL"), "info")
	if err := cfg.LogLevel.UnmarshalText([]byte(levelText)); err != nil {
		problems = append(problems, fmt.Sprintf("LOG_LEVEL must be debug, info, warn or error (got %q)", levelText))
	}

	defaultFormat := "json"
	if cfg.Env == "local" {
		defaultFormat = "text"
	}
	cfg.LogFormat = valueOr(getenv("LOG_FORMAT"), defaultFormat)
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		problems = append(problems, fmt.Sprintf("LOG_FORMAT must be text or json (got %q)", cfg.LogFormat))
	}

	timeoutText := valueOr(getenv("SHUTDOWN_TIMEOUT"), "10s")
	timeout, err := time.ParseDuration(timeoutText)
	if err != nil || timeout <= 0 {
		problems = append(problems, fmt.Sprintf("SHUTDOWN_TIMEOUT must be a positive duration such as 10s (got %q)", timeoutText))
	} else {
		cfg.ShutdownTimeout = timeout
	}

	cfg.JWTSecret = getenv("JWT_SECRET")
	if len(cfg.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET is required and must be at least 32 characters")
	}

	cfg.AppBaseURL = strings.TrimRight(getenv("APP_BASE_URL"), "/")
	if cfg.AppBaseURL == "" {
		if cfg.Env == "local" {
			cfg.AppBaseURL = "http://localhost:5173"
		} else {
			problems = append(problems, "APP_BASE_URL is required outside local development (emailed links use it)")
		}
	}

	cfg.CookieSecure = cfg.Env == "staging" || cfg.Env == "production"
	if text := getenv("COOKIE_SECURE"); text != "" {
		secure, err := strconv.ParseBool(text)
		if err != nil {
			problems = append(problems, fmt.Sprintf("COOKIE_SECURE must be true or false (got %q)", text))
		} else {
			cfg.CookieSecure = secure
		}
	}

	cfg.TrustProxy = cfg.Env == "staging" || cfg.Env == "production"
	if text := getenv("TRUST_PROXY"); text != "" {
		trust, err := strconv.ParseBool(text)
		if err != nil {
			problems = append(problems, fmt.Sprintf("TRUST_PROXY must be true or false (got %q)", text))
		} else {
			cfg.TrustProxy = trust
		}
	}

	cfg.CodartToken = strings.TrimSpace(getenv("CODART_TOKEN"))
	cfg.CodartBaseURL = strings.TrimRight(strings.TrimSpace(getenv("CODART_BASE_URL")), "/")

	cfg.R2AccountID = strings.TrimSpace(getenv("R2_ACCOUNT_ID"))
	cfg.R2Endpoint = strings.TrimRight(strings.TrimSpace(getenv("R2_ENDPOINT")), "/")

	cfg.R2CatalogBucket = strings.TrimSpace(getenv("R2_CATALOG_BUCKET"))
	cfg.R2CatalogAccessKeyID = strings.TrimSpace(getenv("R2_CATALOG_ACCESS_KEY_ID"))
	cfg.R2CatalogSecretAccessKey = strings.TrimSpace(getenv("R2_CATALOG_SECRET_ACCESS_KEY"))
	cfg.R2CatalogPublicBaseURL = strings.TrimRight(strings.TrimSpace(getenv("R2_CATALOG_PUBLIC_BASE_URL")), "/")

	cfg.R2DocumentsBucket = strings.TrimSpace(getenv("R2_DOCUMENTS_BUCKET"))
	cfg.R2DocumentsAccessKeyID = strings.TrimSpace(getenv("R2_DOCUMENTS_ACCESS_KEY_ID"))
	cfg.R2DocumentsSecretAccessKey = strings.TrimSpace(getenv("R2_DOCUMENTS_SECRET_ACCESS_KEY"))

	cfg.R2AvatarsBucket = strings.TrimSpace(getenv("R2_AVATARS_BUCKET"))
	cfg.R2AvatarsAccessKeyID = strings.TrimSpace(getenv("R2_AVATARS_ACCESS_KEY_ID"))
	cfg.R2AvatarsSecretAccessKey = strings.TrimSpace(getenv("R2_AVATARS_SECRET_ACCESS_KEY"))

	for key, values := range map[string][]string{
		"CATALOG":   {cfg.R2AccountID, cfg.R2CatalogBucket, cfg.R2CatalogAccessKeyID, cfg.R2CatalogSecretAccessKey, cfg.R2CatalogPublicBaseURL},
		"DOCUMENTS": {cfg.R2AccountID, cfg.R2DocumentsBucket, cfg.R2DocumentsAccessKeyID, cfg.R2DocumentsSecretAccessKey},
		"AVATARS":   {cfg.R2AccountID, cfg.R2AvatarsBucket, cfg.R2AvatarsAccessKeyID, cfg.R2AvatarsSecretAccessKey},
	} {
		setCount := 0
		for _, v := range values {
			if v != "" {
				setCount++
			}
		}
		if setCount > 0 && setCount < len(values) {
			problems = append(problems, fmt.Sprintf("All R2_%s configuration variables (including R2_ACCOUNT_ID) must be set together", key))
		}
	}

	if cfg.R2DocumentsBucket != "" && cfg.R2DocumentsBucket == cfg.R2CatalogBucket {
		problems = append(problems, "R2_DOCUMENTS_BUCKET must be a different, private bucket than R2_CATALOG_BUCKET, which is public")
	}
	if cfg.R2AvatarsBucket != "" && cfg.R2AvatarsBucket == cfg.R2CatalogBucket {
		problems = append(problems, "R2_AVATARS_BUCKET must be a different, private bucket than R2_CATALOG_BUCKET, which is public")
	}
	if cfg.R2DocumentsBucket != "" && cfg.R2DocumentsBucket == cfg.R2AvatarsBucket {
		problems = append(problems, "R2_DOCUMENTS_BUCKET and R2_AVATARS_BUCKET must be different buckets")
	}

	cfg.LocalMediaDir = valueOr(getenv("LOCAL_MEDIA_DIR"), "./var/media")
	cfg.MediaBaseURL = strings.TrimRight(valueOr(getenv("MEDIA_BASE_URL"), "http://localhost:8080/media"), "/")

	cfg.MailAppsScriptURL = strings.TrimSpace(getenv("MAIL_APPS_SCRIPT_URL"))
	cfg.MailAppsScriptSecret = getenv("MAIL_APPS_SCRIPT_HMAC_SECRET")
	switch {
	case cfg.MailAppsScriptURL == "" && cfg.MailAppsScriptSecret == "":
	case cfg.MailAppsScriptURL == "" || cfg.MailAppsScriptSecret == "":
		problems = append(problems, "MAIL_APPS_SCRIPT_URL and MAIL_APPS_SCRIPT_HMAC_SECRET must be set together")
	default:
		if u, err := url.Parse(cfg.MailAppsScriptURL); err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (cfg.Env == "local" || cfg.Env == "test"))) {
			problems = append(problems, "MAIL_APPS_SCRIPT_URL must be an https address")
		}
		if len(cfg.MailAppsScriptSecret) < 32 {
			problems = append(problems, "MAIL_APPS_SCRIPT_HMAC_SECRET must be at least 32 characters")
		}
	}
	for _, entry := range strings.Split(getenv("MAIL_ALLOWED_RECIPIENTS"), ",") {
		if entry = strings.ToLower(strings.TrimSpace(entry)); entry != "" {
			cfg.MailAllowedRecipients = append(cfg.MailAllowedRecipients, entry)
		}
	}
	if cfg.MailAppsScriptURL != "" && cfg.Env == "staging" && len(cfg.MailAllowedRecipients) == 0 {
		problems = append(problems, "MAIL_ALLOWED_RECIPIENTS is required in staging when email is configured (addresses or @domains, comma separated)")
	}
	intervalText := valueOr(getenv("MAIL_DISPATCH_INTERVAL"), "15s")
	interval, err := time.ParseDuration(intervalText)
	if err != nil || interval < 5*time.Second {
		problems = append(problems, fmt.Sprintf("MAIL_DISPATCH_INTERVAL must be a duration of at least 5s (got %q)", intervalText))
	} else {
		cfg.MailDispatchInterval = interval
	}

	if len(problems) > 0 {
		return Config{}, errors.New("invalid configuration:\n  - " + strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
