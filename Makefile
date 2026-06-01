include .env
BINARY=bin/server
MIGRATION_PATH=./cmd/migrate/migrations

.PHONY: build
build:
	@go build -o $(BINARY) ./cmd/server

.PHONY: run
run: build
	./$(BINARY)

.PHONY: proto
proto:
	@protoc generate

.PHONY: migrate-create
migrate-create:
	@migrate create -seq -ext sql -dir $(MIGRATION_PATH) $(filter-out $@,$(MAKECMDGOALS))

.PHONY: migrate-up
migrate-up:
	@migrate -path $(MIGRATION_PATH) -database=$(DATABASE_ADDR) up

.PHONY: migrate-down
migrate-down:
	@migrate -path $(MIGRATION_PATH) -database=$(DATABASE_ADDR) down $(filter-out $@,$(MAKECMDGOALS))

.PHONY: test
test:
	@go test -race -v ./...

.PHONY: proto
proto:
	@buf generate

.PHONY: proto-lint
proto-lint:
	@buf lint

.PHONY: proto-breaking
proto-breaking:
	@buf breaking --against '.git#branch=main'

%:
	@:
