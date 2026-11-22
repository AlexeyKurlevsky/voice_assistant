BINARY   = ./bin/voice-assistant
PKG      = ./cmd/voice-assistant
MIGRATE  = migrate
SQLC_DIR = internal/infra/postgres

.PHONY: build run test test-race sqlc sqlc-check migrate-up migrate-down lint tidy

build:
	go build -o $(BINARY) $(PKG)

run:
	go run $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

sqlc:
	cd $(SQLC_DIR) && sqlc generate

sqlc-check:
	cd $(SQLC_DIR) && sqlc diff

migrate-up:
	$(MIGRATE) -path ./internal/infra/postgres/migrations \
	           -database "$(MS_DB_DSN)" up

migrate-down:
	$(MIGRATE) -path ./internal/infra/postgres/migrations \
	           -database "$(MS_DB_DSN)" down 1

lint:
	go vet ./...

tidy:
	go mod tidy