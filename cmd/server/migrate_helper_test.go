package main

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// applyMigrations corre las migraciones reales desde migrations/*.sql
// contra la base indicada por dsn. target: -1 = hasta la última disponible,
// 0 = baja todas (down), N = deja el esquema en la versión N. Así los tests
// ejercitan los mismos archivos que usa dev/prod (única fuente de verdad).
func applyMigrations(t *testing.T, dsn string, target int) {
	t.Helper()
	m, err := migrate.New("file://../../migrations", "mysql://"+dsn)
	if err != nil {
		t.Fatalf("init migrate: %v", err)
	}
	defer m.Close()

	if target < 0 {
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			t.Fatalf("apply migrations: %v", err)
		}
		return
	}
	if target == 0 {
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			t.Fatalf("apply down migrations: %v", err)
		}
		return
	}
	if err := m.Migrate(uint(target)); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("apply migrations up to v%d: %v", target, err)
	}
}
