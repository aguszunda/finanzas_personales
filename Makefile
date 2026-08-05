.PHONY: build run test coverage clean dev deps db-init migrate-up migrate-down docker-build docker-run docker-up docker-down git-hooks help

MIGRATE_VERSION ?= v4.18.3

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

dev:
	air -- server

test:
	go test ./... -v

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
	docker build -t finanzas-personales .

docker-run:
	docker run -p 8080:8080 --env-file .env finanzas-personales

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

git-hooks:
	git config core.hooksPath .githooks
	chmod +x .githooks/commit-msg
	@echo "Hooks de git instalados (core.hooksPath = .githooks)"

.PHONY: help
help:
	@echo "Comandos disponibles:"
	@echo "  make build       - Compilar el binario"
	@echo "  make run         - Compilar y ejecutar"
	@echo "  make dev         - Ejecutar con recarga automática (air)"
	@echo "  make test        - Ejecutar tests"
	@echo "  make clean       - Limpiar binarios"
	@echo "  make deps        - Actualizar dependencias"
	@echo "  make db-init     - Crear la base y aplicar migraciones (scripts/db-init.sh)"
	@echo "  make migrate-up  - Aplicar migraciones pendientes (golang-migrate)"
	@echo "  make migrate-down - Revertir la última migración"
	@echo "  make docker-build - Construir imagen Docker"
	@echo "  make docker-run  - Ejecutar contenedor Docker"
	@echo "  make docker-up   - Levantar stack Docker Compose (MySQL + app)"
	@echo "  make docker-down - Detener stack Docker Compose"
	@echo "  make git-hooks   - Instalar hooks de git (valida Conventional Commits)"
