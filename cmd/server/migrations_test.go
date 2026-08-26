package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers de migración
// ---------------------------------------------------------------------------

// migrationVersions devuelve la lista ordenada de prefijos NNN de
// migrations/NNN_*<suffix> (.up.sql / .down.sql).
func migrationVersions(t *testing.T, suffix string) []int {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*" + suffix)
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	versions := make([]int, 0, len(files))
	for _, f := range files {
		prefix := strings.SplitN(filepath.Base(f), "_", 2)[0]
		v, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("prefijo de migración inválido en %s: %v", f, err)
		}
		versions = append(versions, v)
	}
	sort.Ints(versions)
	return versions
}

// latestMigrationVersion devuelve la versión de esquema más alta declarada en
// migrations/*.up.sql (la que golang-migrate debe registrar al terminar).
func latestMigrationVersion(t *testing.T) int {
	t.Helper()
	versions := migrationVersions(t, ".up.sql")
	if len(versions) == 0 {
		t.Fatal("no hay archivos de migración .up.sql")
	}
	return versions[len(versions)-1]
}

// migrationUpFiles devuelve las rutas a migrations/*.up.sql ordenadas.
func migrationUpFiles(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(files)
	return files
}

// tablesInUpFile extrae los nombres de tabla creados por un archivo .up.sql.
func tablesInUpFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(`(?i)CREATE TABLE IF NOT EXISTS\s+([a-z_]+)`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	tables := make([]string, 0, len(matches))
	for _, m := range matches {
		tables = append(tables, m[1])
	}
	return tables
}

// tablesInDownFile extrae los nombres de tabla que un archivo .down.sql borra.
func tablesInDownFile(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(`(?i)DROP TABLE IF EXISTS\s+([a-z_]+)`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	tables := make([]string, 0, len(matches))
	for _, m := range matches {
		tables = append(tables, m[1])
	}
	return tables
}

// newBlankTestDB crea una base MySQL descartable (sin migrar) y devuelve una
// conexión a ella junto con su DSN.
func newBlankTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	admin, cfg, _ := adminDB(t)
	name := fmt.Sprintf("finanzas_mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name)
		admin.Close()
	})
	return db, cfg.FormatDSN()
}

func currentSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	var name string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&name); err != nil {
		t.Fatalf("query current schema: %v", err)
	}
	return name
}

func tableExistsIn(t *testing.T, db *sql.DB, schema, table string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
		schema, table,
	).Scan(&n); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return n == 1
}

func columnExistsIn(t *testing.T, db *sql.DB, schema, table, column string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?",
		schema, table, column,
	).Scan(&n); err != nil {
		t.Fatalf("query column %s.%s: %v", table, column, err)
	}
	return n == 1
}

func schemaVersion(t *testing.T, db *sql.DB) (version int, dirty bool) {
	t.Helper()
	if err := db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	return version, dirty
}

// ---------------------------------------------------------------------------
// Tests de migración
// ---------------------------------------------------------------------------

// Contrato del esquema: cada tabla con las columnas que la app espera. Se
// verifican contra information_schema tras migrar desde los archivos.
var expectedColumns = map[string][]string{
	"usuarios":      {"id", "nombre", "email", "password_hash", "moneda_default", "email_verificado", "token_verificacion", "token_expiracion", "created_at"},
	"categorias":    {"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"},
	"meses":         {"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"},
	"transacciones": {"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"},
	"costos_fijos":  {"id", "usuario_id", "categoria_id", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"},
	"deudas":        {"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "proximo_vencimiento", "created_at", "updated_at"},
}

func TestMigrations_FreshSchemaDesdeArchivos(t *testing.T) {
	db, dsn := newBlankTestDB(t)
	applyMigrations(t, dsn, -1)

	schema := currentSchema(t, db)
	wantVersion := latestMigrationVersion(t)

	// Todas las tablas declaradas en los .up.sql existen tras migrar.
	for _, f := range migrationUpFiles(t) {
		for _, table := range tablesInUpFile(t, f) {
			if !tableExistsIn(t, db, schema, table) {
				t.Errorf("tabla %q (definida en %s) no existe tras migrar", table, filepath.Base(f))
			}
		}
	}

	// Contrato de columnas por tabla.
	for table, cols := range expectedColumns {
		if !tableExistsIn(t, db, schema, table) {
			t.Errorf("tabla contrato %q no existe", table)
			continue
		}
		for _, col := range cols {
			if !columnExistsIn(t, db, schema, table, col) {
				t.Errorf("columna %s.%s no existe tras migrar", table, col)
			}
		}
	}

	// La base hereda el collation esperado (utf8mb4_unicode_ci).
	var collation string
	if err := db.QueryRow(
		"SELECT DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?",
		schema,
	).Scan(&collation); err != nil {
		t.Fatalf("query collation: %v", err)
	}
	if collation != "utf8mb4_unicode_ci" {
		t.Errorf("collation inesperado: %s (esperado utf8mb4_unicode_ci)", collation)
	}

	// Una sola fila de versión, en la última migración, sin dirty.
	version, dirty := schemaVersion(t, db)
	if version != wantVersion {
		t.Errorf("versión de esquema %d, esperada %d", version, wantVersion)
	}
	if dirty {
		t.Errorf("schema_migrations quedó marcado como dirty")
	}

	// Categorías por defecto: 13 de sistema (es_personalizada FALSE, sin usuario).
	var total, system int
	if err := db.QueryRow("SELECT COUNT(*) FROM categorias").Scan(&total); err != nil {
		t.Fatalf("count categorias: %v", err)
	}
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM categorias WHERE es_personalizada = FALSE AND usuario_id IS NULL",
	).Scan(&system); err != nil {
		t.Fatalf("count categorias de sistema: %v", err)
	}
	if total != 13 || system != 13 {
		t.Errorf("categorías por defecto: total=%d sistema=%d, esperadas 13/13", total, system)
	}
}

func TestMigrations_Idempotente(t *testing.T) {
	db, dsn := newBlankTestDB(t)
	applyMigrations(t, dsn, -1)

	// Insertamos datos de negocio y volvemos a migrar: no debe borrar nada
	// ni alterar la versión.
	if _, err := db.Exec(
		"INSERT INTO usuarios (nombre, email, password_hash) VALUES ('a', 'a@b.c', 'hash')",
	); err != nil {
		t.Fatalf("insert usuario: %v", err)
	}

	applyMigrations(t, dsn, -1) // segunda pasada: golang-migrate reporta no-change

	var users int
	if err := db.QueryRow("SELECT COUNT(*) FROM usuarios").Scan(&users); err != nil {
		t.Fatalf("count usuarios: %v", err)
	}
	if users != 1 {
		t.Errorf("re-migrar borró datos: %d usuarios (esperado 1)", users)
	}

	version, dirty := schemaVersion(t, db)
	if version != latestMigrationVersion(t) {
		t.Errorf("versión %d, esperada %d tras re-migrar", version, latestMigrationVersion(t))
	}
	if dirty {
		t.Errorf("schema_migrations dirty tras re-migrar")
	}

	// schema_migrations debe tener exactamente una fila (golang-migrate no
	// tolera filas duplicadas de la migración inline vieja).
	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&rows); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if rows != 1 {
		t.Errorf("schema_migrations tiene %d filas, esperada 1", rows)
	}
}

func TestMigrations_Down_Up_RoundTrip(t *testing.T) {
	db, dsn := newBlankTestDB(t)
	applyMigrations(t, dsn, -1)
	schema := currentSchema(t, db)
	ups := migrationUpFiles(t)

	// Down hasta v1: 002_deudas.down.sql elimina deudas, el resto queda.
	applyMigrations(t, dsn, 1)
	if tableExistsIn(t, db, schema, "deudas") {
		t.Error("deudas debería no existir tras down a v1")
	}
	if !tableExistsIn(t, db, schema, "usuarios") {
		t.Error("usuarios no debería borrarse en down a v1")
	}
	if version, _ := schemaVersion(t, db); version != 1 {
		t.Errorf("versión %d tras down a v1, esperada 1", version)
	}

	// Down hasta 0: 001_init.down.sql elimina todas las tablas de negocio
	// (schema_migrations, de golang-migrate, permanece).
	applyMigrations(t, dsn, 0)
	for _, f := range ups {
		for _, table := range tablesInUpFile(t, f) {
			if tableExistsIn(t, db, schema, table) {
				t.Errorf("tabla %q (de %s) debería no existir tras down a 0", table, filepath.Base(f))
			}
		}
	}

	// Up de nuevo: todo recreado desde los archivos.
	applyMigrations(t, dsn, -1)
	for _, f := range ups {
		for _, table := range tablesInUpFile(t, f) {
			if !tableExistsIn(t, db, schema, table) {
				t.Errorf("tabla %q (de %s) no existe tras el round-trip", table, filepath.Base(f))
			}
		}
	}
	if version, _ := schemaVersion(t, db); version != latestMigrationVersion(t) {
		t.Errorf("versión %d tras round-trip, esperada %d", version, latestMigrationVersion(t))
	}
}

// Los .up.sql y .down.sql deben venir de a pares, con versiones contiguas
// desde 1. Un .down faltante rompería los rollbacks silenciosamente.
func TestMigrations_Archivos_UpDownCoherentes(t *testing.T) {
	ups := migrationVersions(t, ".up.sql")
	downs := migrationVersions(t, ".down.sql")

	if !slices.Equal(ups, downs) {
		t.Errorf("versiones up/down difieren: up=%v down=%v", ups, downs)
	}
	for i, v := range ups {
		if v != i+1 {
			t.Errorf("versiones no contiguas: %v", ups)
			break
		}
	}

	// Cada archivo .down.sql debe dropear exactamente las tablas que su
	// .up.sql crea (inversa exacta).
	for _, up := range migrationUpFiles(t) {
		base := strings.TrimSuffix(filepath.Base(up), ".up.sql")
		down := filepath.Join("../../migrations", base+".down.sql")
		upTables := tablesInUpFile(t, up)
		downTables := tablesInDownFile(t, down)
		for _, table := range upTables {
			if !slices.Contains(downTables, table) {
				t.Errorf("%s crea %q pero %s no la dropea", filepath.Base(up), table, base+".down.sql")
			}
		}
	}
}

// Guard: la app no debe volver a llevar SQL embebido; la única fuente de
// verdad del esquema son migrations/*.sql.
func TestMain_NoSQLEmbeido(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, forbidden := range []string{"CREATE TABLE", "schema_migrations", "runMigrations"} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("main.go no debería contener %q (la migración vive en migrations/)", forbidden)
		}
	}
}
