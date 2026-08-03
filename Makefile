.PHONY: build run test clean dev

build:
	go build -o bin/server ./cmd/server

run: build
	./bin/server

dev:
	air -- server

test:
	go test ./... -v

clean:
	rm -rf bin/

deps:
	go mod tidy

migrate-up:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest -source file://migrations -database "mysql://$(DATABASE_URL)" up

migrate-down:
	go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest -source file://migrations -database "mysql://$(DATABASE_URL)" down

docker-build:
	docker build -t finanzas-personales .

docker-run:
	docker run -p 8080:8080 --env-file .env finanzas-personales

.PHONY: help
help:
	@echo "Comandos disponibles:"
	@echo "  make build       - Compilar el binario"
	@echo "  make run         - Compilar y ejecutar"
	@echo "  make dev         - Ejecutar con recarga automática (air)"
	@echo "  make test        - Ejecutar tests"
	@echo "  make clean       - Limpiar binarios"
	@echo "  make deps        - Actualizar dependencias"
	@echo "  make docker-build - Construir imagen Docker"
	@echo "  make docker-run  - Ejecutar contenedor Docker"
