package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"optipay/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
)

// ── COSTO FIJO: ToggleActivo error branches ──────────────────────────────────

func TestCostoFijoService_ToggleActivo_NotFound(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	queryFind := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`)
	mock.ExpectQuery(queryFind).WithArgs(int64(99), int64(1)).WillReturnError(sql.ErrNoRows)

	_, err := svc.ToggleActivo(context.Background(), 1, 99)
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCostoFijoService_ToggleActivo_Deactivacion(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cols := []string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}
	queryFind := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`)
	queryUpdate := regexp.QuoteMeta(`UPDATE costos_fijos SET categoria_id=?, descripcion=?, monto_estimado=?, dia_vencimiento=?, activo=?, tipo_periodo=?
		 WHERE id=? AND usuario_id=?`)

	// FindByID returns active costofijo.
	mock.ExpectQuery(queryFind).WithArgs(int64(3), int64(1)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(3, 1, 6, "Servicios", "Internet", 5000.0, 5, true, "mensual", created))
	// Update persists it as inactive.
	mock.ExpectExec(queryUpdate).
		WithArgs(int64(6), "Internet", 5000.0, 5, false, "mensual", int64(3), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// No syncMesActual because Activo is now false.

	cf, err := svc.ToggleActivo(context.Background(), 1, 3)
	if err != nil {
		t.Fatalf("ToggleActivo: %v", err)
	}
	if cf.Activo {
		t.Error("expected activo=false after deactivation")
	}
}

func TestCostoFijoService_ToggleActivo_UpdateError(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cols := []string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}
	queryFind := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`)
	queryUpdate := regexp.QuoteMeta(`UPDATE costos_fijos SET categoria_id=?, descripcion=?, monto_estimado=?, dia_vencimiento=?, activo=?, tipo_periodo=?
		 WHERE id=? AND usuario_id=?`)

	mock.ExpectQuery(queryFind).WithArgs(int64(3), int64(1)).
		WillReturnRows(sqlmock.NewRows(cols).AddRow(3, 1, 6, "Servicios", "Internet", 5000.0, 5, true, "mensual", created))
	mock.ExpectExec(queryUpdate).
		WithArgs(int64(6), "Internet", 5000.0, 5, false, "mensual", int64(3), int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err := svc.ToggleActivo(context.Background(), 1, 3)
	if err == nil {
		t.Fatal("expected error from Update")
	}
}

// ── COSTO FIJO: syncMesActual cerrado path ───────────────────────────────────

func TestCostoFijoService_Create_SyncMesCerrado_SkipsPrecarga(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(queryCostoFijoInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))

	// syncMesActual: mes exists and is cerrado → skip precarga.
	periodo := time.Now().Format("2006-01")
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, 1, periodo, "cerrado", 0, 0, 0, nil, 0, 0, 0, created))

	cf, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 12000,
		DiaVencimiento: 5, TipoPeriodo: "mensual",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cf.ID != 3 {
		t.Errorf("expected ID 3, got %d", cf.ID)
	}
}

func TestCostoFijoService_Create_SyncMes_FindOrCreateError(t *testing.T) {
	svc, mock := newCostoFijoService(t)

	mock.ExpectExec(regexp.QuoteMeta(queryCostoFijoInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))

	periodo := time.Now().Format("2006-01")
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 12000,
		DiaVencimiento: 5, TipoPeriodo: "mensual",
	})
	if err == nil {
		t.Fatal("expected error from syncMesActual")
	}
}

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
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}))

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
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectQuery(regexp.QuoteMeta(querySumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	// Update fails.
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(9), int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from mesRepo.Update")
	}
}

func TestMesService_Cerrar_ErrorNextPeriodo(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mesCols := []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}

	// FindByID returns mes with invalid period → nextPeriodo will fail.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows(mesCols).
			AddRow(9, 1, "invalid-period", "abierto", 0, 0, 0, nil, 0, 0, 0, created))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "invalid-period-01", "invalid-period-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "invalid-period").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectQuery(regexp.QuoteMeta(querySumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(9), int64(1)).
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectQuery(regexp.QuoteMeta(querySumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(9), int64(1)).
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

func TestMesService_Cerrar_ErrorSyncFijos(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectQuery(regexp.QuoteMeta(querySumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// FindOrCreate for proximo succeeds.
	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	// SyncFijosPeriodo: FindOrCreate for 2026-09 → exists + FindActivos fails.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRow(10, "2026-09", "abierto"))
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnError(errors.New("db timeout"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error from SyncFijosPeriodo")
	}
}

func TestMesService_Cerrar_SinIngresos(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	// Only egresos, no ingresos.
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", false, nil, nil, "confirmado", 9, created, created))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectQuery(regexp.QuoteMeta(querySumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	// TasaAhorro should be nil when no ingresos.
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 0.0, 30000.0, -30000.0, nil, -30000.0, 0.0, -30000.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRow(10, "2026-09", "abierto"))
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}))

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.TasaAhorro != nil {
		t.Errorf("expected nil tasa_ahorro with no ingresos, got %v", mes.TasaAhorro)
	}
}

// ── MES: SyncFijosPeriodo branches ──────────────────────────────────────────

func TestMesService_SyncFijos_ErrorFindOrCreate(t *testing.T) {
	svc, mock := newMesService(t)

	// FindOrCreate fails at FindByPeriodo.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnError(errors.New("db timeout"))

	err := svc.SyncFijosPeriodo(context.Background(), 1, "2026-09")
	if err == nil {
		t.Fatal("expected error from FindOrCreate")
	}
}

func TestMesService_SyncFijos_MesCerrado_Noop(t *testing.T) {
	svc, mock := newMesService(t)

	// FindOrCreate returns cerrado mes → no-op.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(mesRow(9, "2026-08", "cerrado"))

	if err := svc.SyncFijosPeriodo(context.Background(), 1, "2026-08"); err != nil {
		t.Fatalf("expected nil for cerrado mes, got %v", err)
	}
}

func TestMesService_SyncFijos_ErrorFindActivos(t *testing.T) {
	svc, mock := newMesService(t)

	// FindOrCreate succeeds (abierto).
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRow(10, "2026-09", "abierto"))

	// FindActivos fails.
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnError(errors.New("db timeout"))

	err := svc.SyncFijosPeriodo(context.Background(), 1, "2026-09")
	if err == nil {
		t.Fatal("expected error from FindActivos")
	}
}

func TestMesService_SyncFijos_ErrorCreateTransaccionesFromFijos(t *testing.T) {
	svc, mock := newMesService(t)

	// FindOrCreate succeeds (abierto).
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRow(10, "2026-09", "abierto"))

	// FindActivos returns one active costofijo.
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}).
			AddRow(3, 1, 6, "Servicios", "Internet", 5000.0, 5, true, "mensual", created))

	// PrecargarEnPeriodo: COUNT query fails.
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFijoCount)).
		WithArgs(int64(1), int64(6), "Internet", "2026-09-01", "2026-09-31").
		WillReturnError(errors.New("db timeout"))

	err := svc.SyncFijosPeriodo(context.Background(), 1, "2026-09")
	if err == nil {
		t.Fatal("expected error from CreateTransaccionesFromFijos")
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
