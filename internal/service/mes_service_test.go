package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"optipay/internal/repository"

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
	queryMesRecalcularByID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`
	queryTransaccionPeriodo = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`
	queryMesUpdate = `UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?
		 WHERE id=? AND usuario_id=?`
	querySumSuperavitAnterior = `SELECT COALESCE(SUM(superavit), 0) FROM meses
		 WHERE usuario_id = ? AND estado = 'cerrado' AND periodo < ?`
)

func newMesService(t *testing.T) (*MesService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewMesService(repository.NewMesRepo(db), repository.NewTransaccionRepo(db)), mock
}

func TestMesService_Recalcular(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, created))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	// Sin meses cerrados previos: el acumulado del período es 70000.
	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	tasa := 70.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 100000.0, 30000.0, 70000.0, &tasa, 70000.0, int64(9), int64(1)).
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
	if mes.AhorroAcumulado != 70000 {
		t.Errorf("unexpected ahorro acumulado: %v", mes.AhorroAcumulado)
	}
}

func TestMesService_Recalcular_MesCerrado(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, "2026-08", "cerrado", 100000, 30000, 70000, nil, 0, created))

	_, err := svc.Recalcular(context.Background(), 1, 9)
	if err == nil {
		t.Fatal("expected error recalculating a closed month")
	}
	if !strings.Contains(err.Error(), "cerrado") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMesService_Balance_RecomputaTotales(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}).
			AddRow(9, 1, "2026-08", "abierto", 0, 0, 0, nil, 0, created))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

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
	if mes.AhorroAcumulado != 70000 {
		t.Errorf("unexpected ahorro acumulado: %v", mes.AhorroAcumulado)
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
	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows(cols).
		AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
		AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(rows)

	// 3. Acumulados: superávit de meses cerrados anteriores (20000).
	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(20000.0))

	// 4. Persistir el mes cerrado (tasa 70, ahorro acumulado 90000).
	tasa := 70.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 100000.0, 30000.0, 70000.0, &tasa, 90000.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 5. FindOrCreate del próximo período (2026-09): no existe -> insertar.
	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	// 6. Guardar el próximo mes como abierto (solo actualiza estado).
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.Estado != "cerrado" {
		t.Errorf("expected cerrado, got %q", mes.Estado)
	}
	if mes.AhorroAcumulado != 90000 {
		t.Errorf("expected ahorro acumulado 90000, got %v", mes.AhorroAcumulado)
	}
}

func TestMesService_Cerrar_SinMesAnterior(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	tasa := 100.0
	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("cerrado", 100000.0, 0.0, 100000.0, &tasa, 100000.0, int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectFindOrCreateAbierto(mock, 1, "2026-09", 10)

	mock.ExpectExec(regexp.QuoteMeta(queryMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mes, err := svc.Cerrar(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Cerrar: %v", err)
	}
	if mes.AhorroAcumulado != 100000 {
		t.Errorf("expected ahorro acumulado 100000 (primer mes incluye su propio superávit), got %v", mes.AhorroAcumulado)
	}
}

func TestMesService_Balance_MesActual_SinID(t *testing.T) {
	svc, mock := newMesService(t)
	periodo := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// mesID=0 -> FindOrCreate del período actual: no existe -> insertar -> re-leer.
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(queryMesFindOrCreate)).
		WithArgs(int64(1), periodo).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(int64(1), periodo).WillReturnRows(mesRow(9, periodo, "abierto"))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), periodo+"-01", periodo+"-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
			AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created))

	// Sin meses cerrados previos.
	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), periodo).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	mes, transacciones, err := svc.Balance(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if mes.ID != 9 || mes.Periodo != periodo {
		t.Errorf("expected current month 9/%s, got %+v", periodo, mes)
	}
	if mes.Superavit != 70000 || mes.AhorroAcumulado != 70000 {
		t.Errorf("unexpected acumulados: superavit=%v ahorro=%v", mes.Superavit, mes.AhorroAcumulado)
	}
	if len(transacciones) != 2 {
		t.Errorf("expected 2 transacciones, got %d", len(transacciones))
	}
}

func TestMesService_Balance_ConMesCerradoAnterior(t *testing.T) {
	svc, mock := newMesService(t)

	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	cols := []string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 1, "ingreso", 100000.0, created, 1, "Sueldo", "Sueldo", "transferencia", "confirmado", 9, created, created).
			AddRow(2, 1, "egreso", 30000.0, created, 5, "Alquiler", "Alquiler", "debito", "confirmado", 9, created, created))

	// Un mes cerrado previo (20000).
	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(20000.0))

	mes, _, err := svc.Balance(context.Background(), 1, 9)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	// Ahorro = 20000 histórico + 70000 actual.
	if mes.AhorroAcumulado != 90000 {
		t.Errorf("unexpected ahorro acumulado: %v", mes.AhorroAcumulado)
	}
}

func TestMesService_Balance_ErrorEnAcumulados(t *testing.T) {
	svc, mock := newMesService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryMesRecalcularByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRow(9, "2026-08", "abierto"))

	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(querySumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnError(errors.New("db caido"))

	if _, _, err := svc.Balance(context.Background(), 1, 9); err == nil {
		t.Fatal("expected acumulados query error to propagate")
	}
}
