.PHONY: build run test fmt lint-sec coverage clean dev deps db-init migrate-up migrate-down docker-build docker-run docker-up docker-down git-hooks help

MIGRATE_VERSION ?= v4.18.3
GOSEC_VERSION ?= v2.28.0

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

dev:
	air -- server

test:
	go test ./... -v

fmt:
	gofmt -w .

lint-sec:
	go run github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) -quiet ./...

coverage:
	MIN_COVERAGE=85 ./scripts/coverage.sh

clean:
	rm -rf bin/

deps:
	go mod tidy

db-init:
	./scripts/db-init.sh

migrate-up:
	go run -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -source file://migrations -database "mysql://$(DATABASE_URL)" up

migrate-down:
	go run -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -source file://migrations -database "mysql://$(DATABASE_URL)" down

docker-build:
	docker build -t optipay .

docker-run:
	docker run -p 8080:8080 --env-file env.secrets optipay

docker-up:
	docker compose --env-file env.secrets up -d --build

docker-down:
	docker compose --env-file env.secrets down

git-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/commit-msg .githooks/pre-push
	@echo "Hooks de git instalados (core.hooksPath = .githooks)"
	@echo "  commit-msg: valida Conventional Commits"
	@echo "  pre-push:   bloquea el push si gofmt -l . detecta desvíos"

.PHONY: help
help:
	@echo "Comandos disponibles:"
	@echo "  make build       - Compilar el binario"
	@echo "  make run         - Compilar y ejecutar"
	@echo "  make dev         - Ejecutar con recarga automática (air)"
	@echo "  make test        - Ejecutar tests"
	@echo "  make fmt         - Formatear todo el código (gofmt -w .)"
	@echo "  make lint-sec    - Análisis de seguridad (gosec, igual que CI)"
	@echo "  make clean       - Limpiar binarios"
	@echo "  make deps        - Actualizar dependencias"
	@echo "  make db-init     - Crear la base y aplicar migraciones (scripts/db-init.sh)"
	@echo "  make migrate-up  - Aplicar migraciones pendientes (golang-migrate)"
	@echo "  make migrate-down - Revertir la última migración"
	@echo "  make docker-build - Construir imagen Docker"
	@echo "  make docker-run  - Ejecutar contenedor Docker"
	@echo "  make docker-up   - Levantar stack Docker Compose (MySQL + app)"
	@echo "  make docker-down - Detener stack Docker Compose"
	@echo "  make git-hooks   - Instalar hooks de git (Conventional Commits + gofmt pre-push)"
