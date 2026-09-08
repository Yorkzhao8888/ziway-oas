.PHONY: build run test clean migrate keys docker-up docker-down

# Variables
APP_NAME=ziway-ams
BUILD_DIR=bin
MAIN=cmd/server/main.go

# Build
build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)

# Run (development with hot reload via air if available, otherwise go run)
run:
	go run $(MAIN)

dev:
	@which air > /dev/null 2>&1 && air || go run $(MAIN)

# Test
test:
	go test -v -cover ./...

# Clean
clean:
	rm -rf $(BUILD_DIR) tmp/

# Database migrate (using golang-migrate)
migrate-up:
	migrate -path migrations -database "postgres://ziway:ziway123@localhost:5432/ziway_ams?sslmode=disable" up

migrate-down:
	migrate -path migrations -database "postgres://ziway:ziway123@localhost:5432/ziway_ams?sslmode=disable" down

migrate-force:
	migrate -path migrations -database "postgres://ziway:ziway123@localhost:5432/ziway_ams?sslmode=disable" force $(v)

# Generate RSA keys
keys:
	mkdir -p configs/keys
	openssl genrsa -out configs/keys/private.pem 2048
	openssl rsa -in configs/keys/private.pem -pubout -out configs/keys/public.pem
	@echo "RSA keypair generated in configs/keys/"

# Docker
docker-up:
	docker compose up -d postgres redis

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

# Docker build
docker-build:
	docker build -t $(APP_NAME):latest -f deploy/Dockerfile .

# Lint
lint:
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || go vet ./...

# Dependencies
deps:
	go mod tidy
	go mod download

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  run          - Run the application"
	@echo "  dev          - Run with hot reload (air) or go run"
	@echo "  test         - Run tests with coverage"
	@echo "  keys         - Generate RS256 keypair"
	@echo "  migrate-up   - Run database migrations"
	@echo "  docker-up    - Start PostgreSQL + Redis"
	@echo "  docker-down  - Stop Docker services"
	@echo "  docker-build - Build Docker image"
	@echo "  clean        - Remove build artifacts"
	@echo "  deps         - Download dependencies"
