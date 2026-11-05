.PHONY: build test test-integration lint fmt generate migration-create migrate migrate-up migrate-down migrate-status run docker-up docker-down connect

build:
	go build ./...

test:
	@coverage=$$(mktemp); trap 'rm -f "$$coverage"' EXIT; \
	go test -race -count=2 -coverpkg=./internal/domain/...,./internal/usecase/... -coverprofile="$$coverage" ./... && \
	go tool cover -func="$$coverage" | tail -1

test-integration:
	@set -a; . ./.env && TEST_DATABASE_URL="$${TEST_DATABASE_URL:-$$DATABASE_URL}" go test -race -count=2 -tags=integration ./internal/repository/...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

docker-up:
	tripgoctl cluster start
	tripgoctl environment start

docker-down:
	tripgoctl environment stop

connect:
	tripgoctl connect

migration-create:
	mkdir -p migrations
	go tool goose -dir migrations -s create "$(NAME)" sql

migrate: migrate-up

migrate-up:
	@. ./.env && GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" go tool goose -dir migrations up

migrate-down:
	@. ./.env && GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" go tool goose -dir migrations down

migrate-status:
	@. ./.env && GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" go tool goose -dir migrations status

generate:
	mkdir -p internal/generated
	go tool oapi-codegen -generate types,chi-server -include-operation-ids createTrip,getTrip,finishTrip,health,ready -package api -o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml
	go generate ./internal/...

run:
	@set -a; . ./config/service.env && . ./.env && go run ./cmd/trip-service
