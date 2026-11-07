package main

import (
	"endosoflow/internal/auth"
	"endosoflow/internal/catalog"
	"endosoflow/internal/customers"
	"endosoflow/internal/documents"
	"endosoflow/internal/sales"
	"endosoflow/internal/staff"
	"endosoflow/internal/tasks"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

// The OpenAPI file is the contract. This test fails when the code serves a route the contract does not
// describe, or the contract describes a route the code does not serve, or the file is not valid OpenAPI.
func TestRoutesMatchTheOpenAPIContract(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load the contract: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("the contract is not a valid OpenAPI document: %v", err)
	}

	inContract := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			inContract[strings.ToUpper(method)+" /api/v1"+path] = true
		}
	}

	// The handlers only register routes here, so they can be built without a database.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authHandler := auth.NewHandler(nil, logger, auth.HandlerConfig{AppBaseURL: "http://app.test"})
	staffHandler := staff.NewHandler(nil, authHandler, logger)
	customersHandler := customers.NewHandler(nil, authHandler, logger)
	catalogHandler := catalog.NewHandler(nil, authHandler, logger)
	salesHandler := sales.NewHandler(nil, authHandler, logger)
	tasksHandler := tasks.NewHandler(nil, authHandler, logger)
	documentsHandler := documents.NewHandler(nil, authHandler, logger)
	router := newRouter(logger, okPinger{}, authHandler.Mount, staffHandler.Mount, customersHandler.Mount, catalogHandler.Mount,
		salesHandler.Mount, tasksHandler.Mount, documentsHandler.Mount)

	inCode := map[string]bool{}
	err = chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if len(route) > 1 {
			route = strings.TrimSuffix(route, "/")
		}
		inCode[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk the routes: %v", err)
	}

	var problems []string
	for route := range inCode {
		if !inContract[route] {
			problems = append(problems, "served by the code but missing from the contract: "+route)
		}
	}
	for route := range inContract {
		if !inCode[route] {
			problems = append(problems, "in the contract but not served by the code: "+route)
		}
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("the code and the contract disagree:\n  %s", strings.Join(problems, "\n  "))
	}
}
