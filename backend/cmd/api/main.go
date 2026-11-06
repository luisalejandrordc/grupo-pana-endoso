// Command api starts the EndosoFlow HTTP server.
package main

import (
	"context"
	"endosoflow/internal/adapters/codart"
	"endosoflow/internal/adapters/mail"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/platform/config"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/logging"
	"endosoflow/internal/sales"
	"endosoflow/internal/staff"
	"endosoflow/internal/tasks"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(cfg, os.Stdout)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Until the real email service exists, emails are only written to the log in local and test
	// environments; elsewhere sending fails loudly instead of silently dropping messages.
	var sender mail.Sender = mail.Unconfigured()
	if cfg.Env == "local" || cfg.Env == "test" {
		sender = mail.NewLogSender(logger)
	}
	authService := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: cfg.JWTSecret, AppBaseURL: cfg.AppBaseURL, Mail: sender, Logger: logger,
	})
	authHandler := auth.NewHandler(authService, logger, auth.HandlerConfig{
		AppBaseURL: cfg.AppBaseURL, CookieSecure: cfg.CookieSecure, TrustProxy: cfg.TrustProxy,
	})

	staffService := staff.NewService(staff.Deps{
		DB: pool, Activation: authService, Access: authService, Logger: logger,
	})
	staffHandler := staff.NewHandler(staffService, authHandler, logger)

	// Automatic name lookup needs the Codart token; without it the lookup answers "unavailable" and the
	// screens fall back to manual entry.
	var identity customers.IdentityProvider
	if cfg.CodartToken != "" {
		identity = codart.New(cfg.CodartBaseURL, cfg.CodartToken, nil)
	} else {
		logger.Warn("CODART_TOKEN is not set: automatic DNI/RUC name lookup is off")
	}
	customersService := customers.NewService(customers.Deps{DB: pool, Identity: identity, Logger: logger})
	customersHandler := customers.NewHandler(customersService, authHandler, logger)

	// Catalog images live in Cloudflare R2. Local and test environments use a folder served at /media;
	// anywhere else a missing store makes image uploads fail loudly.
	var store storage.Store = storage.Unconfigured()
	var localStore *storage.Local
	switch {
	case cfg.R2Configured():
		store = storage.NewR2(storage.R2Config{
			AccountID: cfg.R2AccountID, AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
			Bucket: cfg.R2Bucket, PublicBaseURL: cfg.R2PublicBaseURL, Endpoint: cfg.R2Endpoint,
		})
	case cfg.Env == "local" || cfg.Env == "test":
		localStore = storage.NewLocal(cfg.LocalMediaDir, cfg.MediaBaseURL)
		store = localStore
		logger.Warn("R2 is not configured: catalog images are kept on local disk", "dir", cfg.LocalMediaDir)
	default:
		logger.Warn("R2 is not configured: catalog image uploads will fail")
	}
	catalogService := catalog.NewService(catalog.Deps{DB: pool, Store: store, Logger: logger})
	catalogHandler := catalog.NewHandler(catalogService, authHandler, logger)

	// Sales and the task engine. Document checks come with phase 1F: until then a stand-in that reports
	// nothing missing is wired in local and test environments only, and everywhere else completing a
	// task answers DOCUMENTS_UNAVAILABLE.
	var documents tasks.DocumentChecker
	if cfg.Env == "local" || cfg.Env == "test" {
		documents = tasks.StubDocuments{}
		logger.Warn("document checks are stubbed: tasks can be completed without their documents")
	} else {
		logger.Warn("document checks are not available yet: tasks cannot be completed")
	}
	salesService := sales.NewService(sales.Deps{DB: pool, Logger: logger})
	salesHandler := sales.NewHandler(salesService, authHandler, logger)
	tasksService := tasks.NewService(tasks.Deps{DB: pool, Sales: sales.Gate{}, Documents: documents, Logger: logger})
	tasksHandler := tasks.NewHandler(tasksService, authHandler, logger)

	var handler http.Handler = newRouter(logger, pool, authHandler.Mount, staffHandler.Mount, customersHandler.Mount, catalogHandler.Mount,
		salesHandler.Mount, tasksHandler.Mount)
	if localStore != nil {
		mux := http.NewServeMux()
		mux.Handle("/media/", http.StripPrefix("/media/", localStore.Handler()))
		mux.Handle("/", handler)
		handler = mux
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second, // uploads of up to 10 MB
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		authService.WaitForBackgroundWork()
		return nil
	}
}
