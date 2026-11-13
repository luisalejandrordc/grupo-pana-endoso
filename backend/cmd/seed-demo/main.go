// Command seed-demo fills a development or staging database with a believable demo: a manager and three
// advisors, a small catalog with pictures, customers of every type, and sales at every stage (some just
// registered, some late, some completed, some cancelled) with their documents and history.
//
// Everything goes through the same services as the application, so the rules, the audit trail and the files
// are real. The clock is moved through each sale's story so tasks have start, due and completion times in the
// past. It refuses to run against production, and a second run is refused because the demo accounts exist.
//
//	make seed-demo ARGS="-yes"
package main

import (
	"context"
	"endosoflow/internal/adapters/storage"
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
	"endosoflow/internal/platform/config"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/sales"
	"endosoflow/internal/tasks"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
)

const defaultPassword = "Demo-Password-2026!"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed-demo:", err)
		os.Exit(1)
	}
}

func run() error {
	password := flag.String("password", defaultPassword, "password of the demo accounts")
	seed := flag.Uint64("seed", 7, "seed of the random values, so two runs make the same story")
	yes := flag.Bool("yes", false, "confirm that this database may receive demo data")
	flag.Parse()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if cfg.Env == "production" {
		return fmt.Errorf("ENV is production: demo data is never written there")
	}
	target := cfg.DatabaseURL
	if u, err := url.Parse(cfg.DatabaseURL); err == nil {
		target = u.Host + u.Path
	}
	if !*yes {
		return fmt.Errorf("this writes demo data to %s (ENV=%s). Run again with -yes to confirm", target, cfg.Env)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM employees WHERE business_email LIKE '%@' || $1`, demoEmailDomain).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return fmt.Errorf("the demo accounts already exist in %s: the demo data was already loaded", target)
	}
	var adminID uuid.UUID
	err = pool.QueryRow(ctx, `SELECT id FROM employees WHERE role = 'system_admin' AND is_active AND NOT is_system
		ORDER BY created_at LIMIT 1`).Scan(&adminID)
	if err != nil {
		return fmt.Errorf("no active System Admin was found: create one with `make bootstrap-admin` first (%w)", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// The same store choices as the API: catalog pictures in the public bucket (or the local folder), documents
	// in the private one.
	var publicStore storage.Store = storage.Unconfigured()
	var localStore *storage.Local
	switch {
	case cfg.R2Configured():
		publicStore = storage.NewR2(storage.R2Config{
			AccountID: cfg.R2AccountID, AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
			Bucket: cfg.R2Bucket, PublicBaseURL: cfg.R2PublicBaseURL, Endpoint: cfg.R2Endpoint,
		})
	case cfg.Env == "local" || cfg.Env == "test":
		localStore = storage.NewLocal(cfg.LocalMediaDir, cfg.MediaBaseURL)
		publicStore = localStore
	}
	var documentStore storage.Store = storage.Unconfigured()
	switch {
	case cfg.R2Configured() && cfg.R2DocumentsBucket != "":
		documentStore = storage.NewR2(storage.R2Config{
			AccountID: cfg.R2AccountID, AccessKeyID: cfg.R2AccessKeyID, SecretAccessKey: cfg.R2SecretAccessKey,
			Bucket: cfg.R2DocumentsBucket, PublicBaseURL: cfg.R2PublicBaseURL, Endpoint: cfg.R2Endpoint,
		})
	case localStore != nil:
		documentStore = localStore
	}

	now := time.Now().UTC().Truncate(time.Second)
	clk := &clock{t: now}
	gate := sales.Gate{}
	docs := documents.NewService(documents.Deps{DB: pool, Store: documentStore, Sales: gate, Logger: logger, Now: clk.Now})
	s := &seeder{
		ctx: ctx, pool: pool, clock: clk, now: now, limit: now.Add(-30 * time.Minute),
		rng:       rand.New(rand.NewPCG(*seed, *seed^0x9e3779b97f4a7c15)),
		customers: customers.NewService(customers.Deps{DB: pool, Logger: logger}),
		catalog:   catalog.NewService(catalog.Deps{DB: pool, Store: publicStore, Logger: logger}),
		sales:     sales.NewService(sales.Deps{DB: pool, Documents: docs, Store: documentStore, Logger: logger, Now: clk.Now}),
		tasks:     tasks.NewService(tasks.Deps{DB: pool, Sales: gate, Documents: docs, Logger: logger, Now: clk.Now}),
		docs:      docs,
		password:  *password,
		admin:     auth.Actor{ID: adminID, Role: auth.RoleSystemAdmin},
	}

	fmt.Printf("Loading demo data into %s (ENV=%s)\n", target, cfg.Env)
	if err := s.loadSlots(); err != nil {
		return err
	}
	if err := s.seedEmployees(); err != nil {
		return err
	}
	if err := s.seedCatalog(); err != nil {
		return err
	}
	if err := s.seedCustomers(); err != nil {
		return err
	}
	for i, sc := range scenarios {
		if err := s.runSale(i, sc); err != nil {
			return fmt.Errorf("sale %d of %d: %w", i+1, len(scenarios), err)
		}
	}
	s.logf("%d sales, %d tasks run, %d documents", s.created.sales, s.created.tasks, s.created.documents)

	fmt.Println("\nDone. Demo accounts (all with the same password):")
	for _, e := range demoEmployees {
		fmt.Printf("  %-14s %s\n", e.role, e.email)
	}
	fmt.Printf("  password: %s\n", *password)
	return nil
}
