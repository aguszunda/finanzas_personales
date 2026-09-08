package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ── MES: Cerrar error branches ───────────────────────────────────────────────

func TestMesService_Cerrar_ErrorFindByPeriodo(t *testing.T) {
	svc, mock := newMesService(t)

	// FindByID succeeds (abierto).
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	// FindByPeriodo fails.
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from FindByPeriodo")
	}
}

func TestMesService_Cerrar_ErrorCalcAcumulados_SuperavitAnterior(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	// Empty transacciones list.
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	// SumSuperavitAnterior fails → calcularAcumulados error.
	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from calcularAcumulados")
	}
}

func TestMesService_Cerrar_ErrorMesUpdate(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	// Update fails.
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, int64(9), int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from mesRepo.Update")
	}
}

func TestMesService_Cerrar_ErrorNextPeriodo(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	// FindByID returns mes with invalid period → nextPeriodo will fail.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, "invalid-period", "abierto", 0, 0, 0, nil, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "invalid-period-01", "invalid-period-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "invalid-period").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from nextPeriodo with invalid period")
	}
}

func TestMesService_Cerrar_ErrorFindOrCreateProximo(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// FindOrCreate for proximo periodo fails at FindByPeriodo stage.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from FindOrCreate proximo periodo")
	}
}

func TestMesService_Cerrar_CreaProximoMes(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// FindOrCreate for proximo succeeds.
	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	// Guardar el próximo mes como abierto.
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.Estado != "cerrado" {
		t.Errorf("expected cerrado, got %s", mes.Estado)
	}
}

func TestMesService_Cerrar_SinIngresos(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	// Only egresos, no ingresos.
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	// TasaAhorro should be nil when no ingresos.
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 30000.0, -30000.0, nil, -30000.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.TasaAhorro != nil {
		t.Errorf("expected nil tasa_ahorro with no ingresos, got %v", mes.TasaAhorro)
	}
}

// ── AUTH: ReenviarVerificacion error branches ────────────────────────────────

func TestAuthService_Reenviar_ErrorDB_FindByEmail(t *testing.T) {
	f := newAuthServiceFixture(t)
	// FindByEmail returns a generic DB error (not ErrNotFound).
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("a@test.com").
		WillReturnError(errors.New("db timeout"))

	err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "a@test.com"})
	if err == nil {
		t.Fatal("expected error from FindByEmail DB failure")
	}
}

func TestAuthService_Reenviar_ErrorGuardarToken(t *testing.T) {
	f := newAuthServiceFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnError(errors.New("db timeout"))

	err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "a@test.com"})
	if err == nil {
		t.Fatal("expected error from GuardarTokenVerificacion")
	}
	if len(f.mailer.links) != 0 {
		t.Error("must not send email when token save fails")
	}
}
