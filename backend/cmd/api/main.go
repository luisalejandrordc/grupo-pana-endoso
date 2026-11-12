// Command api starts the EndosoFlow HTTP server.
package main

import (
	"context"
	"endosoflow/internal/adapters/codart"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
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

	sender, delivers := newMailer(cfg, logger)
	authService := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: cfg.JWTSecret, AppBaseURL: cfg.AppBaseURL, Mail: sender, Logger: logger,
	})
	authHandler := auth.NewHandler(authService, logger, auth.HandlerConfig{
		AppBaseURL: cfg.AppBaseURL, CookieSecure: cfg.CookieSecure, TrustProxy: cfg.TrustProxy,
	})

	st := newStores(cfg, logger) // the three buckets: catalog (public), documents and avatars (private)
	staffService := staff.NewService(staff.Deps{
		DB: pool, Activation: authService, Access: authService, Avatars: st.Avatars, Logger: logger,
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

	// Object storage: public catalog images and private documents and photos, each in its own bucket.
	localStore := st.Local
	catalogService := catalog.NewService(catalog.Deps{DB: pool, Store: st.Catalog, Logger: logger})
	catalogHandler := catalog.NewHandler(catalogService, authHandler, logger)

	// Sales, documents and the task engine. The sales module implements the gate that the other two use to
	// reach the sale, and the documents module tells the task engine which required files are missing.
	salesGate := sales.Gate{}
	documentsService := documents.NewService(documents.Deps{DB: pool, Store: st.Documents, Sales: salesGate, Logger: logger})
	salesService := sales.NewService(sales.Deps{
		DB: pool, Images: st.Catalog, Documents: documentsService, DocumentStore: st.Documents, Logger: logger,
	})
	salesHandler := sales.NewHandler(salesService, authHandler, logger)
	documentsHandler := documents.NewHandler(documentsService, authHandler, logger)
	tasksService := tasks.NewService(tasks.Deps{DB: pool, Sales: salesGate, Documents: documentsService, Logger: logger})
	tasksHandler := tasks.NewHandler(tasksService, authHandler, logger)

	var handler http.Handler = newRouter(logger, pool, authHandler.Mount, staffHandler.Mount, customersHandler.Mount, catalogHandler.Mount,
		salesHandler.Mount, tasksHandler.Mount, documentsHandler.Mount)
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

	// Background work: the notification dispatcher and the retry of failed file deletions.
	jobsCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	waitForJobs := startJobs(jobsCtx, cfg, pool, st, mailer{Sender: sender, delivers: delivers}, logger)

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
		stopJobs()
		waitForJobs()
		return nil
	}
}
