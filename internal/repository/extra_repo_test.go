package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"optipay/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

// ── column helpers ──

func usuarioCols() []string {
	return []string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}
}

func usuarioTokenCols() []string {
	return []string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion"}
}

func usuarioResetCols() []string {
	return []string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion"}
}

func transaccionCols() []string {
	return []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
}

func costofijoCols() []string {
	return []string{"id", "usuario_id", "categoria_id", "nombre", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}
}

func ptrInt64(v int64) *int64 { return &v }

// ═══════════════════════════════════════════════════════════════════
// UsuarioRepo
// ═══════════════════════════════════════════════════════════════════

func TestUsuarioRepo_Create_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`INSERT INTO usuarios (nombre, email, password_hash, moneda_default)
		 VALUES (?, ?, ?, ?)`)

	// Success — sets ID on the struct.
	mock.ExpectExec(q).WithArgs("Pepe", "pepe@test.com", "hash", "ARS").
		WillReturnResult(sqlmock.NewResult(5, 1))
	u := &model.Usuario{Nombre: "Pepe", Email: "pepe@test.com", PasswordHash: "hash", MonedaDefault: "ARS"}
	if err := r.Create(context.Background(), u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID != 5 {
		t.Errorf("expected ID 5, got %d", u.ID)
	}

	// Duplicate email → MySQL 1062 → ErrEmailExiste.
	mock.ExpectExec(q).WithArgs("Pepe", "pepe@test.com", "hash", "ARS").
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry"})
	if err := r.Create(context.Background(), u); !errors.Is(err, model.ErrEmailExiste) {
		t.Fatalf("expected ErrEmailExiste, got %v", err)
	}

	// Generic DB error propagated.
	mock.ExpectExec(q).WithArgs("Pepe", "pepe@test.com", "hash", "ARS").
		WillReturnError(errors.New("db caido"))
	if err := r.Create(context.Background(), u); err == nil {
		t.Fatal("expected db error")
	}
}

func TestUsuarioRepo_FindByEmail_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado
		 FROM usuarios WHERE email = ?`)
	created := time.Now()

	// Found.
	mock.ExpectQuery(q).WithArgs("pepe@test.com").
		WillReturnRows(sqlmock.NewRows(usuarioCols()).
			AddRow(1, "Pepe", "pepe@test.com", "hash", "ARS", created, true))
	u, err := r.FindByEmail(context.Background(), "pepe@test.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if u.Nombre != "Pepe" || u.EmailVerificado != true {
		t.Errorf("unexpected: %+v", u)
	}

	// Not found → ErrNotFound.
	mock.ExpectQuery(q).WithArgs("no@existe.com").WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByEmail(context.Background(), "no@existe.com"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUsuarioRepo_GuardarTokenVerificacion_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?`)
	expira := time.Now().Add(24 * time.Hour)
	mock.ExpectExec(q).WithArgs("tok_hash", expira, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.GuardarTokenVerificacion(context.Background(), 1, "tok_hash", expira); err != nil {
		t.Fatalf("GuardarTokenVerificacion: %v", err)
	}
}

func TestUsuarioRepo_FindByTokenVerificacion_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion
		 FROM usuarios WHERE token_verificacion = ?`)
	created := time.Now()
	expira := time.Now().Add(24 * time.Hour)

	// Found.
	mock.ExpectQuery(q).WithArgs("tok_hash").
		WillReturnRows(sqlmock.NewRows(usuarioTokenCols()).
			AddRow(1, "Pepe", "pepe@test.com", "hash", "ARS", created, true, expira))
	u, err := r.FindByTokenVerificacion(context.Background(), "tok_hash")
	if err != nil {
		t.Fatalf("FindByTokenVerificacion: %v", err)
	}
	if u.ID != 1 || *u.TokenExpiracion != expira {
		t.Errorf("unexpected: %+v", u)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs("bad").WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByTokenVerificacion(context.Background(), "bad"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUsuarioRepo_MarcarVerificado_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`UPDATE usuarios SET email_verificado = TRUE, token_expiracion = NULL WHERE id = ?`)
	mock.ExpectExec(q).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.MarcarVerificado(context.Background(), 1); err != nil {
		t.Fatalf("MarcarVerificado: %v", err)
	}
}

func TestUsuarioRepo_GuardarTokenPasswordReset_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?`)
	expira := time.Now().Add(1 * time.Hour)
	mock.ExpectExec(q).WithArgs("reset_hash", expira, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.GuardarTokenPasswordReset(context.Background(), 1, "reset_hash", expira); err != nil {
		t.Fatalf("GuardarTokenPasswordReset: %v", err)
	}
}

func TestUsuarioRepo_FindByPasswordResetToken_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion
		 FROM usuarios WHERE password_reset_token = ?`)
	created := time.Now()
	expira := time.Now().Add(1 * time.Hour)

	// Found.
	mock.ExpectQuery(q).WithArgs("reset_hash").
		WillReturnRows(sqlmock.NewRows(usuarioResetCols()).
			AddRow(1, "Pepe", "pepe@test.com", "hash", "ARS", created, true, expira))
	u, err := r.FindByPasswordResetToken(context.Background(), "reset_hash")
	if err != nil {
		t.Fatalf("FindByPasswordResetToken: %v", err)
	}
	if u.ID != 1 || *u.PasswordResetExpiracion != expira {
		t.Errorf("unexpected: %+v", u)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs("bad").WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByPasswordResetToken(context.Background(), "bad"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUsuarioRepo_ActualizarPassword_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?`)
	mock.ExpectExec(q).WithArgs("new_hash", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.ActualizarPassword(context.Background(), 1, "new_hash"); err != nil {
		t.Fatalf("ActualizarPassword: %v", err)
	}
}

func TestUsuarioRepo_ActualizarNombre_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	q := regexp.QuoteMeta(`UPDATE usuarios SET nombre = ? WHERE id = ?`)
	mock.ExpectExec(q).WithArgs("NuevoNombre", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.ActualizarNombre(context.Background(), 1, "NuevoNombre"); err != nil {
		t.Fatalf("ActualizarNombre: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// MesRepo
// ═══════════════════════════════════════════════════════════════════

func TestMesRepo_FindByUsuarioID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	q := regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`)
	created := time.Now()

	// Two rows.
	mock.ExpectQuery(q).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(mesCols()).
			AddRow(2, 1, "2026-09", "abierto", 200000.0, 150000.0, 50000.0, 0.25, 50000.0, 0.0, 50000.0, created).
			AddRow(1, 1, "2026-08", "cerrado", 180000.0, 140000.0, 40000.0, 0.222, 40000.0, 0.0, 40000.0, created))
	ms, err := r.FindByUsuarioID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByUsuarioID: %v", err)
	}
	if len(ms) != 2 || ms[0].Periodo != "2026-09" {
		t.Errorf("unexpected: %+v", ms)
	}

	// Empty.
	mock.ExpectQuery(q).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(mesCols()))
	ms, err = r.FindByUsuarioID(context.Background(), 2)
	if err != nil || len(ms) != 0 {
		t.Fatalf("expected empty, got %v (%v)", ms, err)
	}
}

func TestMesRepo_FindByID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	q := regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`)
	created := time.Now()

	// Found.
	mock.ExpectQuery(q).WithArgs(int64(1), int64(1)).
		WillReturnRows(sqlmock.NewRows(mesCols()).
			AddRow(1, 1, "2026-08", "cerrado", 180000.0, 140000.0, 40000.0, 0.222, 40000.0, 0.0, 40000.0, created))
	m, err := r.FindByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if m.Periodo != "2026-08" || m.Estado != "cerrado" {
		t.Errorf("unexpected: %+v", m)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs(int64(999), int64(1)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMesRepo_FindByPeriodo_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	q := regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)
	created := time.Now()

	// Found.
	mock.ExpectQuery(q).WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows(mesCols()).
			AddRow(1, 1, "2026-08", "cerrado", 180000.0, 140000.0, 40000.0, 0.222, 40000.0, 0.0, 40000.0, created))
	m, err := r.FindByPeriodo(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("FindByPeriodo: %v", err)
	}
	if m.ID != 1 {
		t.Errorf("expected ID 1, got %d", m.ID)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs(int64(1), "2026-99").WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByPeriodo(context.Background(), 1, "2026-99"); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMesRepo_FindOrCreate_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	pq := regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)
	iq := regexp.QuoteMeta(`INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`)
	created := time.Now()

	// Path 1: already exists → returns existing.
	mock.ExpectQuery(pq).WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows(mesCols()).
			AddRow(1, 1, "2026-08", "abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))
	m, err := r.FindOrCreate(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("FindOrCreate (exists): %v", err)
	}
	if m.ID != 1 {
		t.Errorf("expected ID 1, got %d", m.ID)
	}

	// Path 2: missing → INSERT → re-read.
	mock.ExpectQuery(pq).WithArgs(int64(1), "2026-09").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(iq).WithArgs(int64(1), "2026-09").WillReturnResult(sqlmock.NewResult(3, 1))
	mock.ExpectQuery(pq).WithArgs(int64(1), "2026-09").
		WillReturnRows(sqlmock.NewRows(mesCols()).
			AddRow(3, 1, "2026-09", "abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))
	m, err = r.FindOrCreate(context.Background(), 1, "2026-09")
	if err != nil {
		t.Fatalf("FindOrCreate (new): %v", err)
	}
	if m.ID != 3 || m.Periodo != "2026-09" {
		t.Errorf("unexpected: %+v", m)
	}

	// Path 3: DB error on initial read propagates.
	mock.ExpectQuery(pq).WithArgs(int64(1), "2026-10").
		WillReturnError(errors.New("db caido"))
	if _, err := r.FindOrCreate(context.Background(), 1, "2026-10"); err == nil {
		t.Fatal("expected db error")
	}
}

func TestMesRepo_Update_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	q := regexp.QuoteMeta(`UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?, pasivos_total=?, patrimonio=?
		 WHERE id=? AND usuario_id=?`)
	m := &model.Mes{
		ID: 1, UsuarioID: 1, Estado: "abierto", IngresosTotal: 200000,
		EgresosTotal: 150000, Superavit: 50000, AhorroAcumulado: 50000,
		PasivosTotal: 0, Patrimonio: 50000,
	}
	mock.ExpectExec(q).
		WithArgs("abierto", 200000.0, 150000.0, 50000.0, nil, 50000.0, 0.0, 50000.0, int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Update(context.Background(), m); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestMesRepo_SumSuperavitAnterior_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	q := regexp.QuoteMeta(`SELECT COALESCE(SUM(superavit), 0) FROM meses
		 WHERE usuario_id = ? AND estado = 'cerrado' AND periodo < ?`)

	// With accumulated surplus.
	mock.ExpectQuery(q).WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(90000.0))
	sum, err := r.SumSuperavitAnterior(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("SumSuperavitAnterior: %v", err)
	}
	if sum != 90000 {
		t.Errorf("expected 90000, got %v", sum)
	}

	// No closed months → COALESCE returns 0.
	mock.ExpectQuery(q).WithArgs(int64(1), "2026-01").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	sum, err = r.SumSuperavitAnterior(context.Background(), 1, "2026-01")
	if err != nil || sum != 0 {
		t.Fatalf("expected 0, got %v (%v)", sum, err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// TransaccionRepo
// ═══════════════════════════════════════════════════════════════════

func TestTransaccionRepo_Create_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	q := regexp.QuoteMeta(`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	mock.ExpectExec(q).
		WithArgs(int64(1), "ingreso", 100000.0, "2026-08-01", int64(1), "Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9)).
		WillReturnResult(sqlmock.NewResult(10, 1))
	tr := &model.Transaccion{
		UsuarioID: 1, Tipo: "ingreso", Monto: 100000, Fecha: "2026-08-01",
		CategoriaID: 1, Descripcion: "Sueldo", MedioPago: "transferencia",
		Estado: "confirmado", MesID: ptrInt64(9),
	}
	if err := r.Create(context.Background(), tr); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tr.ID != 10 {
		t.Errorf("expected ID 10, got %d", tr.ID)
	}
	if tr.CreatedAt.IsZero() || tr.UpdatedAt.IsZero() {
		t.Error("expected timestamps to be set")
	}
}

func TestTransaccionRepo_FindByID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	q := regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.id = ? AND t.usuario_id = ?`)
	created := time.Now()
	fecha := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	// Found — fecha is formatted as "2006-01-02".
	mock.ExpectQuery(q).WithArgs(int64(5), int64(1)).
		WillReturnRows(sqlmock.NewRows(transaccionCols()).
			AddRow(5, 1, "egreso", 45000.0, fecha, 6, "Servicios", "", "debito", false, nil, nil, "confirmado", int64(9), created, created))
	tr, err := r.FindByID(context.Background(), 5, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if tr.Fecha != "2026-08-10" {
		t.Errorf("expected fecha 2026-08-10, got %q", tr.Fecha)
	}
	if tr.Categoria != "Servicios" {
		t.Errorf("expected Servicios, got %q", tr.Categoria)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs(int64(999), int64(1)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTransaccionRepo_FindByUsuarioID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	q := regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC
		 LIMIT ? OFFSET ?`)
	created := time.Now()
	fecha := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	// Default limit = 50 when limit <= 0.
	mock.ExpectQuery(q).WithArgs(int64(1), 50, 0).
		WillReturnRows(sqlmock.NewRows(transaccionCols()).
			AddRow(1, 1, "egreso", 45000.0, fecha, 6, "Servicios", "", "debito", false, nil, nil, "confirmado", int64(9), created, created))
	ts, err := r.FindByUsuarioID(context.Background(), 1, 0, 0)
	if err != nil {
		t.Fatalf("FindByUsuarioID (default limit): %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1, got %d", len(ts))
	}

	// Custom limit.
	mock.ExpectQuery(q).WithArgs(int64(1), 10, 5).
		WillReturnRows(sqlmock.NewRows(transaccionCols()).
			AddRow(2, 1, "ingreso", 150000.0, fecha, 1, "Sueldo", "", "transferencia", false, nil, nil, "confirmado", int64(9), created, created))
	ts, err = r.FindByUsuarioID(context.Background(), 1, 10, 5)
	if err != nil {
		t.Fatalf("FindByUsuarioID (custom): %v", err)
	}
	if len(ts) != 1 || ts[0].Categoria != "Sueldo" {
		t.Errorf("unexpected: %+v", ts)
	}

	// Empty list.
	mock.ExpectQuery(q).WithArgs(int64(1), 50, 0).
		WillReturnRows(sqlmock.NewRows(transaccionCols()))
	ts, err = r.FindByUsuarioID(context.Background(), 1, 0, 0)
	if err != nil || len(ts) != 0 {
		t.Fatalf("expected empty, got %v (%v)", ts, err)
	}
}

func TestTransaccionRepo_FindByPeriodo_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	// findByRango uses fecha >= ? AND fecha <= ? with periodo+"-01" and periodo+"-31".
	q := regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)
	created := time.Now()
	fecha := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(q).WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(transaccionCols()).
			AddRow(1, 1, "egreso", 45000.0, fecha, 6, "Servicios", "", "debito", false, nil, nil, "confirmado", int64(9), created, created).
			AddRow(2, 1, "ingreso", 150000.0, fecha, 1, "Sueldo", "", "transferencia", false, nil, nil, "confirmado", int64(9), created, created))
	ts, err := r.FindByPeriodo(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("FindByPeriodo: %v", err)
	}
	if len(ts) != 2 {
		t.Errorf("expected 2 transacciones, got %d", len(ts))
	}
}

// ═══════════════════════════════════════════════════════════════════
// CostoFijoRepo
// ═══════════════════════════════════════════════════════════════════

func TestCostoFijoRepo_Create_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCostoFijoRepo(db)

	q := regexp.QuoteMeta(`INSERT INTO costos_fijos (usuario_id, categoria_id, descripcion, monto_estimado, dia_vencimiento, activo, tipo_periodo)
		 VALUES (?,?,?,?,?,?,?)`)
	mock.ExpectExec(q).
		WithArgs(int64(1), int64(6), "Internet", 5000.0, 10, true, "mensual").
		WillReturnResult(sqlmock.NewResult(4, 1))
	cf := &model.CostoFijo{
		UsuarioID: 1, CategoriaID: 6, Descripcion: "Internet",
		MontoEstimado: 5000, DiaVencimiento: 10, Activo: true, TipoPeriodo: "mensual",
	}
	if err := r.Create(context.Background(), cf); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cf.ID != 4 {
		t.Errorf("expected ID 4, got %d", cf.ID)
	}
}

func TestCostoFijoRepo_FindByUsuarioID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCostoFijoRepo(db)

	q := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ?
		 ORDER BY cf.dia_vencimiento, cf.descripcion`)
	created := time.Now()

	// Two rows.
	mock.ExpectQuery(q).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(costofijoCols()).
			AddRow(1, 1, 6, "Servicios", "Internet", 5000.0, 10, true, "mensual", created).
			AddRow(2, 1, 7, "Servicios", "Netflix", 3000.0, 15, true, "mensual", created))
	cfs, err := r.FindByUsuarioID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByUsuarioID: %v", err)
	}
	if len(cfs) != 2 || cfs[0].Descripcion != "Internet" {
		t.Errorf("unexpected: %+v", cfs)
	}

	// Empty.
	mock.ExpectQuery(q).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(costofijoCols()))
	cfs, err = r.FindByUsuarioID(context.Background(), 2)
	if err != nil || len(cfs) != 0 {
		t.Fatalf("expected empty, got %v (%v)", cfs, err)
	}
}

func TestCostoFijoRepo_FindActivos_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCostoFijoRepo(db)

	q := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ? AND cf.activo = TRUE
		 ORDER BY cf.dia_vencimiento, cf.descripcion`)
	created := time.Now()

	mock.ExpectQuery(q).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(costofijoCols()).
			AddRow(1, 1, 6, "Servicios", "Internet", 5000.0, 10, true, "mensual", created))
	cfs, err := r.FindActivos(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindActivos: %v", err)
	}
	if len(cfs) != 1 || cfs[0].Descripcion != "Internet" {
		t.Errorf("unexpected: %+v", cfs)
	}

	// Empty.
	mock.ExpectQuery(q).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(costofijoCols()))
	cfs, err = r.FindActivos(context.Background(), 2)
	if err != nil || len(cfs) != 0 {
		t.Fatalf("expected empty, got %v (%v)", cfs, err)
	}
}

func TestCostoFijoRepo_FindByID_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCostoFijoRepo(db)

	q := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`)
	created := time.Now()

	// Found.
	mock.ExpectQuery(q).WithArgs(int64(3), int64(1)).
		WillReturnRows(sqlmock.NewRows(costofijoCols()).
			AddRow(3, 1, 7, "Servicios", "Netflix", 3000.0, 15, true, "mensual", created))
	cf, err := r.FindByID(context.Background(), 3, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if cf.Descripcion != "Netflix" || cf.Categoria != "Servicios" {
		t.Errorf("unexpected: %+v", cf)
	}

	// Not found.
	mock.ExpectQuery(q).WithArgs(int64(999), int64(1)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════
// CategoriaRepo
// ═══════════════════════════════════════════════════════════════════

func TestCategoriaRepo_FindAll_Extra(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCategoriaRepo(db)

	q := regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)
	created := time.Now()

	// System + custom categories.
	mock.ExpectQuery(q).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(1, "Sueldo", "ingreso", "💰", false, nil, created).
			AddRow(2, "Servicios", "egreso", "💡", false, nil, created).
			AddRow(3, "Gimnasio", "egreso", "🏋️", true, int64(1), created))
	cats, err := r.FindAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3, got %d", len(cats))
	}
	if cats[0].Nombre != "Sueldo" || cats[2].EsPersonalizada != true {
		t.Errorf("unexpected: %+v", cats)
	}

	// Empty.
	mock.ExpectQuery(q).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}))
	cats, err = r.FindAll(context.Background(), 2)
	if err != nil || len(cats) != 0 {
		t.Fatalf("expected empty, got %v (%v)", cats, err)
	}
}
