# ============================================
# ziway-backend - 知味生态全栈OS v4.9 (P0)
# P0 modular monolith: 3 processes (OAS/MBS/BOS)
# ZW-ARC-017 LOCKED (BOS 8→12)
# ============================================

GOPROXY ?= https://goproxy.cn,direct
export GOPROXY

BIN_DIR := bin
CMD_DIR := cmd

.PHONY: all deps build build-oas build-ms build-os run-oas run-ms run-os run-all clean test tidy fmt vet help

all: deps build ## Build all three P0 binaries

deps: ## Download Go module dependencies
	go mod download

tidy: ## Tidy go.mod
	go mod tidy

fmt: ## Format code
	gofmt -s -w .

vet: ## Run go vet
	go vet ./...

build: build-oas build-ms build-os ## Build all binaries

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-oas: $(BIN_DIR) ## Build ziway-oas (Owner+Admin, port 8080)
	go build -o $(BIN_DIR)/oas ./$(CMD_DIR)/oas/

build-ms: $(BIN_DIR) ## Build ziway-ms (12 MBS, port 8081)
	go build -o $(BIN_DIR)/ms ./$(CMD_DIR)/ms/

build-os: $(BIN_DIR) ## Build ziway-os (12 BOS, port 8082)
	go build -o $(BIN_DIR)/os ./$(CMD_DIR)/os/

build-ams: ## Build AMS P1 independent service (services/ams)
	cd services/ams && go mod tidy && go build -o $(SCRIPT_DIR)/$(BIN_DIR)/ams ./cmd/server/

run-oas: build-oas ## Run OAS on port 8080 (SQLite dev)
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8080 ./$(BIN_DIR)/oas

run-ms: build-ms ## Run MBS on port 8081 (SQLite dev)
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8081 ./$(BIN_DIR)/ms

run-os: build-os ## Run BOS on port 8082 (SQLite dev)
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8082 ./$(BIN_DIR)/os

run-all: build ## Run all three processes in background, logs to tmp/
	mkdir -p tmp
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8080 ./$(BIN_DIR)/oas > tmp/oas.log 2>&1 &
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8081 ./$(BIN_DIR)/ms > tmp/ms.log 2>&1 &
	APP_ENV=dev ZIWAY_SERVER_HTTP_PORT=8082 ./$(BIN_DIR)/os > tmp/os.log 2>&1 &
	@echo "P0 cluster starting: OAS:8080 MBS:8081 BOS:8082"
	@echo "Logs: tmp/oas.log tmp/ms.log tmp/os.log"
	@sleep 2
	@curl -s http://localhost:8080/health || echo "OAS not ready"
	@curl -s http://localhost:8081/health || echo "MBS not ready"
	@curl -s http://localhost:8082/health || echo "BOS not ready"

stop: ## Stop all P0 processes
	-pkill -f 'bin/oas' 2>/dev/null
	-pkill -f 'bin/ms' 2>/dev/null
	-pkill -f 'bin/os' 2>/dev/null
	@echo "P0 cluster stopped"

test: ## Run tests
	go test ./... -v -count=1

clean: ## Remove build artifacts and dev database
	rm -rf $(BIN_DIR) tmp data/*.db
	@echo "cleaned"

# ============================================
# Service map
# ============================================
# P0 Modular Monolith (3 processes):
# OAS  :8080  Owner Plane /owner/* + Admin Plane /admin/*
# MBS  :8081  /api/v1/{ams,cmbs,dmbs,hmbs,fmbs,pmbs,embs,gmbs,ombs,vmbs,imbs,smbs}/
# BOS  :8082  /api/v1/os/{cbos,dbos,ibos,vbos,pbos,abos,ebos,hbos,sbos,fbos,gbos,obos}/
#
# P1 Independent Service:
# AMS  services/ams/  (独立Go模块, JWT+Casbin+12U角色, 99.99% SLA)
# ============================================

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
