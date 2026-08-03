package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"finanzas_personales/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

//go:generate go run github.com/golang-migrate/migrate/v4/cmd/migrate@latest -source file://migrations -database "mysql://$DATABASE_URL" up

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", cfg.DatabaseURL)
	if err != nil {
		slog.Error("cannot open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		slog.Error("cannot ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	runMigrations(db)

	r := buildRouter(cfg, db)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	slog.Info("server stopped")
}

func runMigrations(db *sql.DB) {
	slog.Info("running database migrations...")
	var version int64
	err := db.QueryRowContext(context.Background(), "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		slog.Warn("cannot check migration version, running full setup", "error", err)
	}
	if version < 1 {
		slog.Info("running migration 001_init...")
		migration := `
		CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, dirty BOOLEAN NOT NULL);
		-- CreateSchema
		CREATE TABLE IF NOT EXISTS usuarios (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			nombre VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			moneda_default VARCHAR(10) DEFAULT 'ARS',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS categorias (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			nombre VARCHAR(255) NOT NULL,
			tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('ingreso', 'egreso')),
			icono VARCHAR(50) DEFAULT '',
			es_personalizada BOOLEAN DEFAULT FALSE,
			usuario_id BIGINT REFERENCES usuarios(id) ON DELETE CASCADE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS meses (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
			periodo VARCHAR(7) NOT NULL,
			estado VARCHAR(20) DEFAULT 'abierto' CHECK (estado IN ('abierto', 'cerrado')),
			ingresos_total DECIMAL(15,2) DEFAULT 0,
			egresos_total DECIMAL(15,2) DEFAULT 0,
			superavit DECIMAL(15,2) DEFAULT 0,
			tasa_ahorro DECIMAL(5,2),
			ahorro_acumulado DECIMAL(15,2) DEFAULT 0,
			pasivos_total DECIMAL(15,2) DEFAULT 0,
			patrimonio DECIMAL(15,2) DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(usuario_id, periodo)
		);

		CREATE TABLE IF NOT EXISTS transacciones (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
			tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('ingreso', 'egreso')),
			monto DECIMAL(15,2) NOT NULL,
			fecha DATE NOT NULL,
			categoria_id BIGINT NOT NULL REFERENCES categorias(id),
			descripcion TEXT,
			medio_pago VARCHAR(50) DEFAULT '',
			es_fijo BOOLEAN DEFAULT FALSE,
			cuotas_total INT,
			cuota_actual INT,
			estado VARCHAR(20) DEFAULT 'confirmado' CHECK (estado IN ('pendiente', 'confirmado', 'ajuste')),
			mes_id BIGINT REFERENCES meses(id),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS costos_fijos (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			usuario_id BIGINT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
			categoria_id BIGINT NOT NULL REFERENCES categorias(id),
			descripcion VARCHAR(255) NOT NULL,
			monto_estimado DECIMAL(15,2) NOT NULL,
			dia_vencimiento INT NOT NULL CHECK (dia_vencimiento BETWEEN 1 AND 31),
			activo BOOLEAN DEFAULT TRUE,
			tipo_periodo VARCHAR(20) DEFAULT 'mensual' CHECK (tipo_periodo IN ('mensual', 'bimestral', 'anual')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX idx_transacciones_usuario_fecha ON transacciones(usuario_id, fecha DESC);
		CREATE INDEX idx_transacciones_mes ON transacciones(mes_id);
		CREATE INDEX idx_costos_fijos_usuario ON costos_fijos(usuario_id);
		CREATE INDEX idx_meses_usuario_periodo ON meses(usuario_id, periodo);

		-- Insert default categories
		INSERT IGNORE INTO categorias (nombre, tipo, icono, es_personalizada) VALUES
			('Sueldo', 'ingreso', '💰', FALSE),
			('Freelance', 'ingreso', '💻', FALSE),
			('Ventas', 'ingreso', '📦', FALSE),
			('Otros Ingresos', 'ingreso', '📥', FALSE),
			('Alquiler', 'egreso', '🏠', FALSE),
			('Servicios', 'egreso', '💡', FALSE),
			('Comida', 'egreso', '🍽️', FALSE),
			('Transporte', 'egreso', '🚗', FALSE),
			('Salud', 'egreso', '🏥', FALSE),
			('Educación', 'egreso', '📚', FALSE),
			('Entretenimiento', 'egreso', '🎬', FALSE),
			('Suscripciones', 'egreso', '📱', FALSE),
			('Imprevistos', 'egreso', '⚠️', FALSE)
		;
		`
		if _, err := db.ExecContext(context.Background(), migration); err != nil {
			slog.Error("migration 001_init failed", "error", err)
			os.Exit(1)
		}
		if _, err := db.ExecContext(context.Background(), "INSERT IGNORE INTO schema_migrations (version, dirty) VALUES (1, FALSE)"); err != nil {
			slog.Warn("cannot record migration version", "error", err)
		}
		slog.Info("migration 001_init completed")
	}
}
