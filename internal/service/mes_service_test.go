package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"administracion-financiera/internal/repository"

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
