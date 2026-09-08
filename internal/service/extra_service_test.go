package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"optipay/internal/model"
	"optipay/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func newDashboardServiceForTest(t *testing.T) (*DashboardService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)
	return svc, mock
}

// ─── AUTH SERVICE ────────────────────────────────────────────────────────────────

const (
	queryUsuarioFindByID      = `SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?`
	queryUsuarioActualizarNom = `UPDATE usuarios SET nombre = ? WHERE id = ?`
	queryUsuarioActualizarPwd = `UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?`
)

func TestAuthService_GetUsuario_Success(t *testing.T) {
	svc, mock := newAuthService(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(5, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true))

	u, err := svc.GetUsuario(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetUsuario: %v", err)
	}
	if u.ID != 5 || u.Nombre != "Agustin" {
		t.Errorf("unexpected user: %+v", u)
	}
}

func TestAuthService_GetUsuario_NotFound(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.GetUsuario(context.Background(), 99)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAuthService_UpdateNombre_Success(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryUsuarioActualizarNom)).
		WithArgs("Nuevo Nombre", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.UpdateNombre(context.Background(), 5, UpdateNombreInput{Nombre: "Nuevo Nombre"}); err != nil {
		t.Fatalf("UpdateNombre: %v", err)
	}
}

func TestAuthService_UpdateNombre_TrimSpaces(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryUsuarioActualizarNom)).
		WithArgs("limpio", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.UpdateNombre(context.Background(), 5, UpdateNombreInput{Nombre: "  limpio  "}); err != nil {
		t.Fatalf("UpdateNombre: %v", err)
	}
}

func TestAuthService_UpdateNombre_EmptyName(t *testing.T) {
	svc, _ := newAuthService(t)
	err := svc.UpdateNombre(context.Background(), 5, UpdateNombreInput{Nombre: ""})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAuthService_UpdateNombre_WhitespaceOnly(t *testing.T) {
	svc, _ := newAuthService(t)
	err := svc.UpdateNombre(context.Background(), 5, UpdateNombreInput{Nombre: "   "})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAuthService_UpdatePassword_Success(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("claveActual1"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true))
	mock.ExpectExec(regexp.QuoteMeta(queryUsuarioActualizarPwd)).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.UpdatePassword(context.Background(), 5, UpdatePasswordInput{
		PasswordActual: "claveActual1",
		PasswordNuevo:  "nuevaClave1",
		Password2:      "nuevaClave1",
	}); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
}

func TestAuthService_UpdatePassword_WrongCurrentPassword(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("claveActual1"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true))

	err := svc.UpdatePassword(context.Background(), 5, UpdatePasswordInput{
		PasswordActual: "contraseñaIncorrecta",
		PasswordNuevo:  "nuevaClave1",
		Password2:      "nuevaClave1",
	})
	if !errors.Is(err, model.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestAuthService_UpdatePassword_PasswordMismatch(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("claveActual1"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true))

	err := svc.UpdatePassword(context.Background(), 5, UpdatePasswordInput{
		PasswordActual: "claveActual1",
		PasswordNuevo:  "nuevaClave1",
		Password2:      "otraClave1",
	})
	if !errors.Is(err, model.ErrPasswordInvalido) {
		t.Fatalf("expected ErrPasswordInvalido, got %v", err)
	}
}

func TestAuthService_UpdatePassword_InvalidNewPassword(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("claveActual1"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, pw := range []string{"corto", "12345678", "abcdefgh", "contraseñá1", "clave uno1"} {
		svc, mock := newAuthService(t)
		mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
			WithArgs(int64(5)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
				AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true))

		err := svc.UpdatePassword(context.Background(), 5, UpdatePasswordInput{
			PasswordActual: "claveActual1",
			PasswordNuevo:  pw,
			Password2:      pw,
		})
		if !errors.Is(err, model.ErrPasswordInvalido) {
			t.Errorf("password %q: expected ErrPasswordInvalido, got %v", pw, err)
		}
	}
}

func TestAuthService_UpdatePassword_SameAsCurrent(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("claveActual1"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true))

	err := svc.UpdatePassword(context.Background(), 5, UpdatePasswordInput{
		PasswordActual: "claveActual1",
		PasswordNuevo:  "claveActual1",
		Password2:      "claveActual1",
	})
	if !errors.Is(err, model.ErrPasswordIgualAnterior) {
		t.Fatalf("expected ErrPasswordIgualAnterior, got %v", err)
	}
}

func TestAuthService_UpdatePassword_UserNotFound(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryUsuarioFindByID)).
		WithArgs(int64(99)).
		WillReturnError(sql.ErrNoRows)

	err := svc.UpdatePassword(context.Background(), 99, UpdatePasswordInput{
		PasswordActual: "claveActual1",
		PasswordNuevo:  "nuevaClave1",
		Password2:      "nuevaClave1",
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ─── MES SERVICE ────────────────────────────────────────────────────────────────

const queryMesFindByUsuarioID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`

func TestMesService_List_Success(t *testing.T) {
	svc, mock := newMesService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesFindByUsuarioID)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, created).
			AddRow(8, 1, "2026-07", "cerrado", 100000, 30000, 70000, nil, 70000, created))

	meses, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(meses) != 2 {
		t.Fatalf("expected 2 meses, got %d", len(meses))
	}
	if meses[0].Estado != "abierto" || meses[1].Estado != "cerrado" {
		t.Errorf("unexpected order: %+v", meses)
	}
}

func TestMesService_GetByID_Success(t *testing.T) {
	svc, mock := newMesService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, created))

	m, err := svc.GetByID(context.Background(), 9, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m.ID != 9 || m.Periodo != "2026-08" {
		t.Errorf("unexpected: %+v", m)
	}
}

func TestMesService_GetByID_NotFound(t *testing.T) {
	svc, mock := newMesService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.GetByID(context.Background(), 99, 1)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMesService_GetCurrent_Existing(t *testing.T) {
	svc, mock := newMesService(t)
	periodo := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, periodo, "abierto", 0, 0, 0, nil, 0, created))

	m, err := svc.GetCurrent(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if m.ID != 9 || m.Periodo != periodo {
		t.Errorf("unexpected: %+v", m)
	}
}

func TestMesService_GetCurrent_CreatedOnDemand(t *testing.T) {
	svc, mock := newMesService(t)
	periodo := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(queryMesFindOrCreate)).
		WithArgs(int64(1), periodo).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(10, 1, periodo, "abierto", 0, 0, 0, nil, 0, created))

	m, err := svc.GetCurrent(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if m.ID != 10 || m.Periodo != periodo {
		t.Errorf("unexpected: %+v", m)
	}
}

// ─── TRANSACCION SERVICE ────────────────────────────────────────────────────────

func TestTransaccionService_GetByID_Success(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectTransaccionByID(mock, 4, 1, "ingreso", 1000, "2026-08-10", 9)

	tx, err := svc.GetByID(context.Background(), 4, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if tx.ID != 4 || tx.Tipo != "ingreso" {
		t.Errorf("unexpected: %+v", tx)
	}
}

func TestTransaccionService_GetByID_NotFound(t *testing.T) {
	svc, mock := newTransaccionService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(sql.ErrNoRows)

	_, err := svc.GetByID(context.Background(), 99, 1)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

const queryTransaccionFindByUsuarioID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC
		 LIMIT ? OFFSET ?`

func TestTransaccionService_List_Success(t *testing.T) {
	svc, mock := newTransaccionService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByUsuarioID)).
		WithArgs(int64(1), 10, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created))

	list, err := svc.List(context.Background(), 1, 10, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 transaccion, got %d", len(list))
	}
}

func TestTransaccionService_List_Error(t *testing.T) {
	svc, mock := newTransaccionService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByUsuarioID)).
		WithArgs(int64(1), 10, 0).
		WillReturnError(errors.New("db caido"))

	_, err := svc.List(context.Background(), 1, 10, 0)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

const queryTransaccionFindByPeriodo = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`

func TestTransaccionService_ListByPeriodo_Success(t *testing.T) {
	svc, mock := newTransaccionService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
			AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created))

	list, err := svc.ListByPeriodo(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("ListByPeriodo: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 transacciones, got %d", len(list))
	}
}

func TestTransaccionService_ListByPeriodo_Empty(t *testing.T) {
	svc, mock := newTransaccionService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), "2026-12-01", "2026-12-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	list, err := svc.ListByPeriodo(context.Background(), 1, "2026-12")
	if err != nil {
		t.Fatalf("ListByPeriodo: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 transacciones, got %d", len(list))
	}
}

// ─── DASHBOARD SERVICE ──────────────────────────────────────────────────────────

func TestPrimerMesAbierto_FindsEarliestOpenAfterPeriod(t *testing.T) {
	meses := []model.Mes{
		{Periodo: "2026-06", Estado: "cerrado"},
		{Periodo: "2026-07", Estado: "abierto"},
		{Periodo: "2026-08", Estado: "abierto"},
	}
	got := primerMesAbierto(meses, "2026-06")
	if got != "2026-07" {
		t.Errorf("expected 2026-07, got %q", got)
	}
}

func TestPrimerMesAbierto_NoneOpen(t *testing.T) {
	meses := []model.Mes{
		{Periodo: "2026-06", Estado: "cerrado"},
		{Periodo: "2026-07", Estado: "cerrado"},
	}
	got := primerMesAbierto(meses, "2026-06")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPrimerMesAbierto_EmptyList(t *testing.T) {
	got := primerMesAbierto(nil, "2026-06")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPrimerMesAbierto_IgnoresPeriodosBefore(t *testing.T) {
	meses := []model.Mes{
		{Periodo: "2026-05", Estado: "abierto"},
		{Periodo: "2026-06", Estado: "cerrado"},
	}
	got := primerMesAbierto(meses, "2026-06")
	if got != "" {
		t.Errorf("expected empty (periodo before is ignored), got %q", got)
	}
}

func TestRango10Dias_RangeIsCorrect(t *testing.T) {
	desde, hasta := rango10Dias()
	d, err := time.Parse("2006-01-02", desde)
	if err != nil {
		t.Fatalf("desde parse: %v", err)
	}
	h, err := time.Parse("2006-01-02", hasta)
	if err != nil {
		t.Fatalf("hasta parse: %v", err)
	}
	diff := h.Sub(d)
	if diff != 9*24*time.Hour {
		t.Errorf("expected 9-day span, got %v", diff)
	}
	if h.After(time.Now().Add(time.Hour)) {
		t.Error("hasta should be around now")
	}
}

func TestUnirMovimientos_SortsByFechaDesc(t *testing.T) {
	svc, _ := newDashboardServiceForTest(t)
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	transacciones := []model.Transaccion{
		{ID: 1, Tipo: "egreso", Monto: 100, Fecha: yesterday.Format("2006-01-02"), Categoria: "Alquiler", CreatedAt: yesterday},
		{ID: 2, Tipo: "ingreso", Monto: 500, Fecha: now.Format("2006-01-02"), Categoria: "Sueldo", CreatedAt: now},
	}

	movs := svc.unirMovimientos(transacciones)
	if len(movs) != 2 {
		t.Fatalf("expected 2 movimientos, got %d", len(movs))
	}
	// First item should be most recent
	if movs[0].Fecha != now.Format("2006-01-02") {
		t.Errorf("first should be today, got %q", movs[0].Fecha)
	}
	if movs[0].Origen != "transaccion" {
		t.Errorf("unexpected origen: %q", movs[0].Origen)
	}
}

func TestUnirMovimientos_OnlyTransacciones(t *testing.T) {
	svc, _ := newDashboardServiceForTest(t)
	now := time.Now()
	transacciones := []model.Transaccion{
		{ID: 1, Tipo: "egreso", Monto: 100, Fecha: now.Format("2006-01-02"), Categoria: "Alquiler", CreatedAt: now},
	}

	movs := svc.unirMovimientos(transacciones)
	if len(movs) != 1 || movs[0].Origen != "transaccion" {
		t.Errorf("expected single transaccion, got %+v", movs)
	}
}

func TestUnirMovimientos_Empty(t *testing.T) {
	svc, _ := newDashboardServiceForTest(t)
	movs := svc.unirMovimientos(nil)
	if len(movs) != 0 {
		t.Errorf("expected 0, got %d", len(movs))
	}
}

func TestNewDashboardService(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)
	if svc == nil {
		t.Fatal("NewDashboardService returned nil")
	}
}

// GetDashboard success path: mocks every ordered repo call.
func TestGetDashboard_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	pt, _ := time.Parse("2006-01", periodoActual)
	periodoAnterior := pt.AddDate(0, -1, 0).Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	// 1. FindOrCreate mes actual -> exists
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	// 2. FindByPeriodo transacciones del mes actual
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
			AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created))

	// 3. FindByPeriodo mes anterior (may return not-found)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoAnterior).
		WillReturnError(sql.ErrNoRows)

	// 4. FindAll categorias
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(1, "Sueldo", "ingreso", "💰", false, nil, created).
			AddRow(5, "Alquiler", "egreso", "🏠", false, nil, created))

	// 5. FindByRango transacciones últimos días (rango10Dias)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)).
		WithArgs(int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	data, err := svc.GetDashboard(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if data.MesActual == nil {
		t.Fatal("expected mesActual")
	}
	if data.MesActual.IngresosTotal != 100000 || data.MesActual.EgresosTotal != 30000 {
		t.Errorf("unexpected totals: ingresos=%v egresos=%v", data.MesActual.IngresosTotal, data.MesActual.EgresosTotal)
	}
	if data.MesActual.Superavit != 70000 {
		t.Errorf("expected superavit 70000, got %v", data.MesActual.Superavit)
	}
	if data.MesAnterior != nil {
		t.Errorf("expected nil mesAnterior, got %+v", data.MesAnterior)
	}
	if len(data.IngresosPorCategoria) != 1 {
		t.Errorf("expected 1 ingreso por categoria, got %d", len(data.IngresosPorCategoria))
	} else {
		if data.IngresosPorCategoria[0].CategoriaID != 1 || data.IngresosPorCategoria[0].Monto != 100000 {
			t.Errorf("unexpected ingreso: %+v", data.IngresosPorCategoria[0])
		}
	}
	if len(data.EgresosPorCategoria) != 1 {
		t.Errorf("expected 1 egreso por categoria, got %d", len(data.EgresosPorCategoria))
	} else {
		if data.EgresosPorCategoria[0].CategoriaID != 5 || data.EgresosPorCategoria[0].Monto != 30000 {
			t.Errorf("unexpected egreso: %+v", data.EgresosPorCategoria[0])
		}
	}
}

// GetDashboard with period filter uses the given period's full range for rango.
func TestGetDashboard_ConPeriodo(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	pt, _ := time.Parse("2006-01", periodoActual)
	periodoAnterior := pt.AddDate(0, -1, 0).Format("2006-01")
	periodoFilter := "2026-06"
	created := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	// 1. FindOrCreate mes actual -> exists (abierto)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	// 2. FindByPeriodo transacciones del mes actual
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created))

	// 3. FindByPeriodo mes anterior (not found)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoAnterior).
		WillReturnError(sql.ErrNoRows)

	// 4. FindAll categorias
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(1, "Sueldo", "ingreso", "💰", false, nil, created))

	// 5. FindByRango transacciones: uses periodo filter range (2026-06-01 to 2026-06-30)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)).
		WithArgs(int64(1), "2026-06-01", "2026-06-30").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	data, err := svc.GetDashboard(context.Background(), 1, periodoFilter)
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if data.MesActual.IngresosTotal != 100000 {
		t.Errorf("expected ingresos 100000, got %v", data.MesActual.IngresosTotal)
	}
}

// GetDashboard: early error propagation (FindOrCreate fails).
func TestGetDashboard_ErrorMesActual(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnError(errors.New("db timeout"))

	_, err = svc.GetDashboard(context.Background(), 1, "")
	if err == nil {
		t.Fatal("expected error propagation from FindOrCreate")
	}
}

// GetDashboard: FindByPeriodo transacciones error.
func TestGetDashboard_ErrorTransacciones(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnError(errors.New("db timeout"))

	_, err = svc.GetDashboard(context.Background(), 1, "")
	if err == nil {
		t.Fatal("expected error from transacciones query")
	}
}

// GetDashboard: mes actual cerrado triggers the "find first open" fallback path.
func TestGetDashboard_MesCerrado_FallbackAbierto(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	pt, _ := time.Parse("2006-01", periodoActual)
	periodoAbierto := pt.AddDate(0, 2, 0).Format("2006-01") // 2 months ahead
	ptAbierto, _ := time.Parse("2006-01", periodoAbierto)
	periodoAnteriorDelAbierto := ptAbierto.AddDate(0, -1, 0).Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	// 1. FindOrCreate mes actual -> cerrado
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "cerrado", 100000, 30000, 70000, nil, 0, created))

	// 2. FindByUsuarioID -> list of months (one open ahead)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesFindByUsuarioID)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "cerrado", 100000, 30000, 70000, nil, 0, created).
			AddRow(12, 1, periodoAbierto, "abierto", 0, 0, 0, nil, 0, created))

	// 3. FindOrCreate for the found open periodo
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoAbierto).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(12, 1, periodoAbierto, "abierto", 0, 0, 0, nil, 0, created))

	// 4. FindByPeriodo transacciones del periodo abierto
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoAbierto+"-01", periodoAbierto+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	// 5. FindByPeriodo mes anterior (not found) — after fallback, periodoActual = periodoAbierto
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoAnteriorDelAbierto).
		WillReturnError(sql.ErrNoRows)

	// 6. FindAll categorias
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}))

	// 7. FindByRango transacciones
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)).
		WithArgs(int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	data, err := svc.GetDashboard(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if data.MesActual == nil || data.MesActual.Periodo != periodoAbierto {
		t.Errorf("expected fallback to open period %s, got %+v", periodoAbierto, data.MesActual)
	}
}

// GetDashboard: FindAll categorias error.
func TestGetDashboard_ErrorCategorias(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err = svc.GetDashboard(context.Background(), 1, "")
	if err == nil {
		t.Fatal("expected error from categorias query")
	}
}

// GetDashboard: FindByRango transacciones error.
func TestGetDashboard_ErrorRangoTransacciones(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)).
		WithArgs(int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("db timeout"))

	_, err = svc.GetDashboard(context.Background(), 1, "")
	if err == nil {
		t.Fatal("expected error from rango transacciones query")
	}
}

// GetDashboard: invalid period filter.
func TestGetDashboard_InvalidPeriodoFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "abierto", 0, 0, 0, nil, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}))

	// Invalid period "not-a-period" should cause time.Parse to fail
	_, err = svc.GetDashboard(context.Background(), 1, "not-a-period")
	if err == nil {
		t.Fatal("expected error for invalid period filter")
	}
}

// GetDashboard: FindByUsuarioID error when mesActual is cerrado.
func TestGetDashboard_ErrorFindByUsuarioID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	svc := NewDashboardService(
		repository.NewMesRepo(db),
		repository.NewTransaccionRepo(db),
		repository.NewCategoriaRepo(db),
	)

	periodoActual := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	// mes actual is cerrado
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, periodoActual, "cerrado", 100000, 30000, 70000, nil, 0, created))

	// FindByUsuarioID fails
	mock.ExpectQuery(regexp.QuoteMeta(queryMesFindByUsuarioID)).
		WithArgs(int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err = svc.GetDashboard(context.Background(), 1, "")
	if err == nil {
		t.Fatal("expected error from FindByUsuarioID fallback")
	}
}

func TestPrimerMesAbierto(t *testing.T) {
	tests := []struct {
		name    string
		meses   []model.Mes
		periodo string
		want    string
	}{
		{
			name:    "elige el menor periodo abierto mayor al dado",
			meses:   []model.Mes{{Periodo: "2026-04", Estado: "abierto"}, {Periodo: "2026-03", Estado: "cerrado"}, {Periodo: "2026-02", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "2026-02",
		},
		{
			name:    "no toma meses cerrados",
			meses:   []model.Mes{{Periodo: "2026-04", Estado: "cerrado"}, {Periodo: "2026-03", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "2026-03",
		},
		{
			name:    "no toma periodos menores o iguales",
			meses:   []model.Mes{{Periodo: "2026-01", Estado: "abierto"}, {Periodo: "2025-12", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "",
		},
		{
			name:    "vacio si no hay mes abierto posterior",
			meses:   []model.Mes{{Periodo: "2026-03", Estado: "cerrado"}},
			periodo: "2026-01",
			want:    "",
		},
		{
			name:    "vacio si no hay meses",
			periodo: "2026-01",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := primerMesAbierto(tt.meses, tt.periodo); got != tt.want {
				t.Errorf("primerMesAbierto() = %q, want %q", got, tt.want)
			}
		})
	}
}
