# EndosoFlow developer commands
# Run `make help` to list them.

include backend/.env
export DATABASE_URL
export TEST_DATABASE_URL

DEV_DB  := $(firstword $(subst ?, ,$(notdir $(DATABASE_URL))))
TEST_DB := $(firstword $(subst ?, ,$(notdir $(TEST_DATABASE_URL))))
MIGRATIONS  := backend/db/migrations
VERIFY_SQL  := backend/db/verify/constraints.sql

.PHONY: help db-create db-drop db-reset migrate-up migrate-status db-verify tidy fmt build vet test run bootstrap-admin seed-demo

help:
	@echo "db-create       create the dev and test databases"
	@echo "db-drop         drop the dev and test databases"
	@echo "migrate-up      apply all migrations to the dev database"
	@echo "migrate-status  show which migrations are applied"
	@echo "db-reset        drop and recreate the dev database, then migrate"
	@echo "db-verify       rebuild the test database and run the SQL rule checks"
	@echo "tidy            download Go dependencies and update go.mod / go.sum"
	@echo "fmt             format the Go code (run before committing)"
	@echo "build           compile the backend"
	@echo "vet             run go vet (static checks)"
	@echo "test            run the Go tests (run make db-verify once first)"
	@echo "run             start the API on http://localhost:8080"
	@echo "bootstrap-admin create the first System Admin (see README)"
	@echo "seed-demo       load demo data (needs an admin; refuses production): make seed-demo ARGS=\"-yes\""

db-create:
	-createdb $(DEV_DB)
	-createdb $(TEST_DB)

db-drop:
	dropdb --if-exists $(DEV_DB)
	dropdb --if-exists $(TEST_DB)

migrate-up:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" up

migrate-status:
	goose -dir $(MIGRATIONS) postgres "$(DATABASE_URL)" status

db-reset:
	dropdb --if-exists $(DEV_DB)
	createdb $(DEV_DB)
	$(MAKE) migrate-up

db-verify:
	dropdb --if-exists $(TEST_DB)
	createdb $(TEST_DB)
	goose -dir $(MIGRATIONS) postgres "$(TEST_DATABASE_URL)" up
	psql -v ON_ERROR_STOP=1 "$(TEST_DATABASE_URL)" -f $(VERIFY_SQL)

tidy:
	cd backend && go mod tidy

fmt:
	cd backend && gofmt -w .

build:
	cd backend && go build ./...

vet:
	cd backend && go vet ./...

test:
	cd backend && TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test ./...

run:
	cd backend && set -a && . ./.env && set +a && go run ./cmd/api

# Example: make bootstrap-admin ARGS="-doc-number 10000001 -first-name Maju -last-name Admin -gender F -phone 999111222 -email maju@admin.com -birthdate 2004-10-05 -hire-date 2026-01-01"
bootstrap-admin:
	cd backend && set -a && . ./.env && set +a && go run ./cmd/bootstrap-admin $(ARGS)

# Loads a believable demo (staff, catalog, customers, sales at every stage). Needs an active System Admin
# (make bootstrap-admin first). Example: make seed-demo ARGS="-yes"
seed-demo:
	cd backend && set -a && . ./.env && set +a && go run ./cmd/seed-demo $(ARGS)
