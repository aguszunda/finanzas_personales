package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

const queryCostoFijoInsert = `INSERT INTO costos_fijos (usuario_id, categoria_id, descripcion, monto_estimado, dia_vencimiento, activo, tipo_periodo)
		 VALUES (?,?,?,?,?,?,?)`

const (
	queryTransaccionFijoCount = `SELECT COUNT(*) FROM transacciones
		 WHERE usuario_id = ? AND es_fijo = TRUE AND estado = 'pendiente'
		   AND categoria_id = ? AND descripcion = ?
		   AND fecha >= ? AND fecha <= ?`
	queryTransaccionFijoInsert = `INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, estado)
		 VALUES (?, 'egreso', ?, ?, ?, ?, 'debito', TRUE, 'pendiente')`
)

func newCostoFijoService(t *testing.T) (*CostoFijoService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewCostoFijoService(repository.NewCostoFijoRepo(db), repository.NewMesRepo(db)), mock
}

// expectSyncMesActual mocks the queries that create/validate the current month
// and precarga a fixed cost into it as a "pendiente" transaction.
func expectSyncMesActual(t *testing.T, mock sqlmock.Sqlmock, usuarioID int64, f model.CostoFijo) {
	t.Helper()
	periodo := time.Now().Format("2006-01")
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(usuarioID, periodo).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(queryMesFindOrCreate)).
		WithArgs(usuarioID, periodo).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(usuarioID, periodo).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, usuarioID, periodo, "abierto", 0, 0, 0, nil, 0, 0, 0, created))
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionFijoCount)).
		WithArgs(usuarioID, f.CategoriaID, f.Descripcion, periodo+"-01", periodo+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(queryTransaccionFijoInsert)).
		WithArgs(usuarioID, f.MontoEstimado, periodo+"-01", f.CategoriaID, f.Descripcion).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestCostoFijoService_Create_Valid(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryCostoFijoInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))
	expectSyncMesActual(t, mock, 1, model.CostoFijo{CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 12000, Activo: true})

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
	if !cf.Activo {
		t.Error("new costo fijo should be active by default")
	}
}

func TestCostoFijoService_Create_DefaultsMensual(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryCostoFijoInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))
	expectSyncMesActual(t, mock, 1, model.CostoFijo{CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 12000, Activo: true})

	cf, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 12000,
		DiaVencimiento: 5, TipoPeriodo: "",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cf.TipoPeriodo != "mensual" {
		t.Errorf("expected default mensual, got %q", cf.TipoPeriodo)
	}
}

func TestCostoFijoService_Create_EmptyDescripcion(t *testing.T) {
	svc, _ := newCostoFijoService(t)
	_, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "", MontoEstimado: 100, DiaVencimiento: 5,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCostoFijoService_Create_InvalidMonto(t *testing.T) {
	svc, _ := newCostoFijoService(t)
	_, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 0, DiaVencimiento: 5,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCostoFijoService_Create_InvalidDia(t *testing.T) {
	svc, _ := newCostoFijoService(t)
	for _, dia := range []int{0, 32} {
		_, err := svc.Create(context.Background(), 1, CreateCostoFijoInput{
			CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 100, DiaVencimiento: dia,
		})
		if !errors.Is(err, model.ErrInvalidInput) {
			t.Fatalf("dia %d: expected ErrInvalidInput, got %v", dia, err)
		}
	}
}

func TestCostoFijoService_Update_NotFound(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	queryFind := regexp.QuoteMeta(`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`)
	mock.ExpectQuery(queryFind).WithArgs(int64(99), int64(1)).WillReturnError(sql.ErrNoRows)

	_, err := svc.Update(context.Background(), 1, 99, CreateCostoFijoInput{
		CategoriaID: 6, Descripcion: "Internet", MontoEstimado: 100, DiaVencimiento: 5,
	})
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
