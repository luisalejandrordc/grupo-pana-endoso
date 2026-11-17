// Command bootstrap-admin creates the first System Admin and prints their activation link.
// It refuses to run when an active System Admin already exists.
package main

import (
	"context"
	"endosoflow/internal/auth"
	"endosoflow/internal/platform/config"
	"endosoflow/internal/platform/database"
	"endosoflow/internal/platform/logging"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func parseDate(name, value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must look like 1990-04-23 (got %q)", name, value)
	}
	return t, nil
}

func run() error {
	var in auth.BootstrapAdminInput
	var birthdate, hireDate string
	flag.StringVar(&in.DocumentType, "doc-type", "DNI", "document type: DNI, CE or PAS")
	flag.StringVar(&in.DocumentNumber, "doc-number", "", "document number")
	flag.StringVar(&in.FirstName, "first-name", "", "first name")
	flag.StringVar(&in.LastName, "last-name", "", "last name")
	flag.StringVar(&in.Nationality, "nationality", "Peruana", "nationality")
	flag.StringVar(&in.Gender, "gender", "", "gender: M or F")
	flag.StringVar(&in.BusinessPhone, "phone", "", "business phone")
	flag.StringVar(&in.BusinessEmail, "email", "", "business email (the activation link is sent here)")
	flag.StringVar(&birthdate, "birthdate", "", "birthdate, YYYY-MM-DD")
	flag.StringVar(&hireDate, "hire-date", "", "hire date, YYYY-MM-DD")
	flag.Parse()

	var err error
	if in.Birthdate, err = parseDate("birthdate", birthdate); err != nil {
		return err
	}
	if in.HireDate, err = parseDate("hire-date", hireDate); err != nil {
		return err
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	logger := logging.New(cfg, os.Stderr)

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// The command prints the link itself, so it needs no email service and queues nothing.
	svc := auth.NewService(auth.Deps{
		DB: pool, JWTSecret: cfg.JWTSecret,
		AppBaseURL: cfg.AppBaseURL, PortalBaseURL: cfg.PortalBaseURL, Logger: logger,
	})
	link, err := auth.BootstrapAdmin(ctx, svc, in)
	if err != nil {
		return err
	}
	fmt.Println("The first System Admin was created. Open this link to set the password (valid for 72 hours):")
	fmt.Println(link)
	return nil
}
