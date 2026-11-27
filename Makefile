BINARY   = ./bin/voice-assistant
PKG      = ./cmd/voice-assistant
MIGRATE  = migrate
SQLC_DIR = internal/infra/postgres
MS_DB_DSN = postgres://postgres:postgres@localhost:5432/voice_assistant?sslmode=disable

.PHONY: build run test test-race profile-cpu profile-mem sqlc sqlc-check migrate-up migrate-down lint tidy

build:
	go build -o $(BINARY) $(PKG)

run:
	go run $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

profile-cpu:
	go test -run '^$$' -bench . -benchmem -cpuprofile ./cpu.prof \
		./internal/application/... ./internal/infra/llm/...
	go tool pprof -top ./cpu.prof

profile-mem:
	go test -run '^$$' -bench . -benchmem -memprofile ./mem.prof \
		./internal/application/... ./internal/infra/llm/...
	go tool pprof -top ./mem.prof

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
