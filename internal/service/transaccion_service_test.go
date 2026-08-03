package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"administracion-financiera/internal/model"
	"administracion-financiera/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	queryMesByPeriodo = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`
	queryMesFindOrCreate = `INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`
	queryMesByID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`
	queryTransaccionInsert = `INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	queryTransaccionByID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.id = ? AND t.usuario_id = ?`
	queryTransaccionUpdate = `UPDATE transacciones SET tipo=?, monto=?, fecha=?, categoria_id=?, descripcion=?, medio_pago=?, es_fijo=?, cuotas_total=?, cuota_actual=?, updated_at=NOW()
		 WHERE id=? AND usuario_id=?`
	queryTransaccionDelete = `DELETE FROM transacciones WHERE id=? AND usuario_id=?`
)

func newTransaccionService(t *testing.T) (*TransaccionService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewTransaccionService(repository.NewTransaccionRepo(db), repository.NewMesRepo(db)), mock
}

func mesRow(id int64, periodo, estado string) *sqlmock.Rows {
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
		AddRow(id, 1, periodo, estado, 0, 0, 0, nil, 0, 0, 0, created)
}

func expectFindOrCreateAbierto(mock sqlmock.Sqlmock, usuarioID int64, periodo string, mesID int64) {
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(usuarioID, periodo).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(queryMesFindOrCreate)).
		WithArgs(usuarioID, periodo).WillReturnResult(sqlmock.NewResult(mesID, 1))
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(usuarioID, periodo).WillReturnRows(mesRow(mesID, periodo, "abierto"))
}

func expectFindOrCreateReturning(mock sqlmock.Sqlmock, usuarioID int64, periodo, estado string, mesID int64) {
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByPeriodo)).
		WithArgs(usuarioID, periodo).WillReturnRows(mesRow(mesID, periodo, estado))
}

func transaccionRow(id int64, tipo string, monto float64, fecha string, mesID int64) *sqlmock.Rows {
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo", "cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}).
		AddRow(id, 1, tipo, monto, created, 5, "Sueldo", "desc", "transferencia", false, nil, nil, "confirmado", mesID, created, created)
}

func expectTransaccionByID(mock sqlmock.Sqlmock, id, usuarioID int64, tipo string, monto float64, fecha string, mesID int64) {
	mock.ExpectQuery(regexp.QuoteMeta(queryTransaccionByID)).
		WithArgs(id, usuarioID).WillReturnRows(transaccionRow(id, tipo, monto, fecha, mesID))
}

func TestTransaccionService_Create_Valid(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectFindOrCreateAbierto(mock, 1, "2026-08", 9)
	mock.ExpectExec(regexp.QuoteMeta(queryTransaccionInsert)).
		WithArgs(1, "ingreso", 1000.0, "2026-08-10", int64(1), "Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9)).
		WillReturnResult(sqlmock.NewResult(4, 1))

	tx, err := svc.Create(context.Background(), 1, CreateTransaccionInput{
		Tipo: "ingreso", Monto: 1000, Fecha: "2026-08-10", CategoriaID: 1,
		Descripcion: "Sueldo", MedioPago: "transferencia",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tx.ID != 4 {
		t.Errorf("expected ID 4, got %d", tx.ID)
	}
	if tx.Estado != "confirmado" {
		t.Errorf("expected estado confirmado, got %q", tx.Estado)
	}
	if tx.MesID == nil || *tx.MesID != 9 {
		t.Errorf("expected MesID 9, got %v", tx.MesID)
	}
}

func TestTransaccionService_Create_InvalidMonto(t *testing.T) {
	svc, _ := newTransaccionService(t)
	_, err := svc.Create(context.Background(), 1, CreateTransaccionInput{
		Tipo: "ingreso", Monto: 0, Fecha: "2026-08-10", CategoriaID: 1,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestTransaccionService_Create_InvalidTipo(t *testing.T) {
	svc, _ := newTransaccionService(t)
	_, err := svc.Create(context.Background(), 1, CreateTransaccionInput{
		Tipo: "inversion", Monto: 100, Fecha: "2026-08-10", CategoriaID: 1,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestTransaccionService_Create_ClosedMonth(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectFindOrCreateReturning(mock, 1, "2026-08", "cerrado", 9)

	_, err := svc.Create(context.Background(), 1, CreateTransaccionInput{
		Tipo: "ingreso", Monto: 100, Fecha: "2026-08-10", CategoriaID: 1,
	})
	if !errors.Is(err, model.ErrMesCerrado) {
		t.Fatalf("expected ErrMesCerrado, got %v", err)
	}
}

func TestTransaccionService_Update_InvalidMonto(t *testing.T) {
	svc, _ := newTransaccionService(t)
	_, err := svc.Update(context.Background(), 1, 4, CreateTransaccionInput{
		Tipo: "ingreso", Monto: 0, Fecha: "2026-08-10", CategoriaID: 1,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestTransaccionService_Update_ClosedMonth(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectTransaccionByID(mock, 4, 1, "ingreso", 1000, "2026-08-10", 9)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).WillReturnRows(mesRow(9, "2026-08", "cerrado"))

	_, err := svc.Update(context.Background(), 1, 4, CreateTransaccionInput{
		Tipo: "ingreso", Monto: 2000, Fecha: "2026-08-10", CategoriaID: 1,
	})
	if !errors.Is(err, model.ErrMesCerrado) {
		t.Fatalf("expected ErrMesCerrado, got %v", err)
	}
}

func TestTransaccionService_Update_Valid(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectTransaccionByID(mock, 4, 1, "ingreso", 1000, "2026-08-10", 9)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).WillReturnRows(mesRow(9, "2026-08", "abierto"))
	mock.ExpectExec(regexp.QuoteMeta(queryTransaccionUpdate)).
		WithArgs("egreso", 2000.0, "2026-08-10", int64(2), "nueva desc", "debito", false, nil, nil, int64(4), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, err := svc.Update(context.Background(), 1, 4, CreateTransaccionInput{
		Tipo: "egreso", Monto: 2000, Fecha: "2026-08-10", CategoriaID: 2,
		Descripcion: "nueva desc", MedioPago: "debito",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if tx.Monto != 2000 || tx.Tipo != "egreso" {
		t.Errorf("unexpected updated values: %+v", tx)
	}
}

func TestTransaccionService_Delete_ClosedMonth(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectTransaccionByID(mock, 4, 1, "egreso", 100, "2026-08-10", 9)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).WillReturnRows(mesRow(9, "2026-08", "cerrado"))

	err := svc.Delete(context.Background(), 1, 4)
	if !errors.Is(err, model.ErrMesCerrado) {
		t.Fatalf("expected ErrMesCerrado, got %v", err)
	}
}

func TestTransaccionService_Delete_Valid(t *testing.T) {
	svc, mock := newTransaccionService(t)
	expectTransaccionByID(mock, 4, 1, "egreso", 100, "2026-08-10", 9)
	mock.ExpectQuery(regexp.QuoteMeta(queryMesByID)).
		WithArgs(int64(9), int64(1)).WillReturnRows(mesRow(9, "2026-08", "abierto"))
	mock.ExpectExec(regexp.QuoteMeta(queryTransaccionDelete)).
		WithArgs(int64(4), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.Delete(context.Background(), 1, 4); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
