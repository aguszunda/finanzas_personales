package service

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"finanzas_personales/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNextPeriodo(t *testing.T) {
	tests := []struct {
		current string
		want    string
		wantErr bool
	}{
		{"2026-01", "2026-02", false},
		{"2026-12", "2027-01", false},
		{"2027-01", "2027-02", false},
		{"2026-13", "", true},
		{"ene-2026", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := nextPeriodo(tt.current)
		if tt.wantErr {
			if err == nil {
				t.Errorf("nextPeriodo(%q): expected error, got %q", tt.current, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("nextPeriodo(%q): unexpected error %v", tt.current, err)
			continue
		}
		if got != tt.want {
			t.Errorf("nextPeriodo(%q) = %q, want %q", tt.current, got, tt.want)
		}
	}
}

const (
	queryMesRecalcularByID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`
	queryTransaccionPeriodo = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`
	queryMesUpdate = `UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?, pasivos_total=?, patrimonio=?
		 WHERE id=? AND usuario_id=?`
	queryCostosFijosActivos = `SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ? AND cf.activo = TRUE
		 ORDER BY cf.dia_vencimiento, cf.descripcion`
)

func newMesService(t *testing.T) (*MesService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewMesService(repository.NewMesRepo(db), repository.NewTransaccionRepo(db), repository.NewCostoFijoRepo(db)), mock
}

func TestMesService_Recalcular(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, 0, 0, created))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", false, nil, nil, "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", false, nil, nil, "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	tasa := 70.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 100000.0, 30000.0, 70000.0, &tasa, 0.0, 0.0, 0.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Recalcular(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Recalcular: %v", err)
	}
	if mes.IngresosTotal != 100000 || mes.EgresosTotal != 30000 {
		t.Errorf("unexpected totals: ingresos=%v egresos=%v", mes.IngresosTotal, mes.EgresosTotal)
	}
	if mes.Superavit != 70000 {
		t.Errorf("expected superavit 70000, got %v", mes.Superavit)
	}
	if mes.TasaAhorro == nil || *mes.TasaAhorro != 70 {
		t.Errorf("expected tasa ahorro 70, got %v", mes.TasaAhorro)
	}
}

func TestMesService_Balance_RecomputaTotales(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, 0, 0, created))

	// SyncFijosPeriodo: FindOrCreate (FindByPeriodo) + FindActivos (sin fijos).
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, 0, 0, created))
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", false, nil, nil, "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", false, nil, nil, "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	mes, transacciones, err := svc.Balance(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if mes.IngresosTotal != 100000 || mes.EgresosTotal != 30000 || mes.Superavit != 70000 {
		t.Errorf("unexpected totals: ingresos=%v egresos=%v superavit=%v", mes.IngresosTotal, mes.EgresosTotal, mes.Superavit)
	}
	if mes.TasaAhorro == nil || *mes.TasaAhorro != 70 {
		t.Errorf("expected tasa ahorro 70, got %v", mes.TasaAhorro)
	}
	if len(transacciones) != 2 {
		t.Errorf("expected 2 transacciones, got %d", len(transacciones))
	}
}

func TestMesService_Cerrar_AlreadyClosed(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "cerrado"))

	_, err := svc.Cerrar(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error for already closed month")
	}
	if !strings.Contains(err.Error(), "ya está cerrado") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMesService_Cerrar_Success(t *testing.T) {
	svc, mock := newMesService(t)

	// 1. FindByID del mes a cerrar (abierto).
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	// 2. Transacciones del período.
	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", false, nil, nil, "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", false, nil, nil, "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	// 3. Último mes cerrado: arrastra ahorro acumulado y pasivos.
	queryUltimo := `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND estado = 'cerrado'
		 ORDER BY periodo DESC LIMIT 1`
	mock.ExpectQuery(regexp.QuoteMeta(queryUltimo)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(5, 1, "2026-07", "cerrado", 90000, 70000, 20000, 22.2, 10000.0, 5000.0, 5000.0, created))

	// 4. Persistir el mes cerrado (tasa 70, ahorro acumulado 30000, pasivos 5000, patrimonio 25000).
	tasa := 70.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 100000.0, 30000.0, 70000.0, &tasa, 30000.0, 5000.0, 25000.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 5. FindOrCreate del próximo período (2026-09): no existe -> insertar.
	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	// 6. SyncFijosPeriodo(1, "2026-09"): el mes ya existe -> FindByPeriodo directo.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRow(10, "2026-09", "abierto"))
	// ... un costo fijo activo que se materializa en el período.
	mock.ExpectQuery(regexp.QuoteMeta(queryCostosFijosActivos)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "categoria_id", "categoria", "descripcion", "monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}).
			AddRow(3, 1, 6, "Servicios", "Internet", 5000.0, 5, true, "mensual", created))
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFijoCount)).
		WithArgs(int64(1), int64(6), "Internet", "2026-09-01", "2026-09-31").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(queryTransaccionFijoInsert)).
		WithArgs(int64(1), 5000.0, "2026-09-01", int64(6), "Internet").
		WillReturnResult(sqlmock.NewResult(4, 1))

	// 7. Guardar el próximo mes como abierto (solo actualiza estado).
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.Estado != "cerrado" {
		t.Errorf("expected cerrado, got %q", mes.Estado)
	}
	if mes.AhorroAcumulado != 30000 {
		t.Errorf("expected ahorro acumulado 30000, got %v", mes.AhorroAcumulado)
	}
	if mes.Patrimonio != 25000 {
		t.Errorf("expected patrimonio 25000, got %v", mes.Patrimonio)
	}
}

func TestMesService_Cerrar_SinMesAnterior(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", false, nil, nil, "confirmado", 9, created, created))

	queryUltimo := `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND estado = 'cerrado'
		 ORDER BY periodo DESC LIMIT 1`
	mock.ExpectQuery(regexp.QuoteMeta(queryUltimo)).
		WithArgs(int64(1)).
		WillReturnError(sql.ErrNoRows)

	tasa := 100.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 100000.0, 0.0, 100000.0, &tasa, 0.0, 0.0, 0.0, int64(9), int64(1)).
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
	if mes.AhorroAcumulado != 0 || mes.PasivosTotal != 0 || mes.Patrimonio != 0 {
		t.Errorf("expected 0 carry-over without previous month, got %+v", mes)
	}
}
