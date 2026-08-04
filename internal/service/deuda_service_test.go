package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	queryDeudaInsert = `INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, saldo_pendiente, tasa_interes, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?,?,?)`
	queryDeudaByID = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, saldo_pendiente, tasa_interes, proximo_vencimiento, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`
	queryDeudaList = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, saldo_pendiente, tasa_interes, proximo_vencimiento, created_at
		 FROM deudas WHERE usuario_id = ?
		 ORDER BY created_at DESC`
	queryDeudaUpdate = `UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, saldo_pendiente=?, tasa_interes=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`
	queryDeudaDelete = `DELETE FROM deudas WHERE id=? AND usuario_id=?`
)

func newDeudaService(t *testing.T) (*DeudaService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewDeudaService(repository.NewDeudaRepo(db)), mock
}

func TestDeudaService_Create_Validaciones(t *testing.T) {
	svc, _ := newDeudaService(t)

	tests := []CreateDeudaInput{
		{Entidad: "", MontoTotal: 100, SaldoPendiente: 50},
		{Entidad: "Banco", MontoTotal: 0, SaldoPendiente: 0},
		{Entidad: "Banco", MontoTotal: 100, SaldoPendiente: -1},
		{Entidad: "Banco", MontoTotal: 100, SaldoPendiente: 150},
		{Entidad: "Banco", MontoTotal: 100, SaldoPendiente: 50, Tipo: "cripto"},
	}
	for i, input := range tests {
		if _, err := svc.Create(context.Background(), 1, input); err == nil {
			t.Errorf("case %d: expected error for %+v", i, input)
		}
	}
}

func TestDeudaService_Create_Success(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaInsert)).
		WithArgs(int64(1), "prestamo", "Banco Galicia", "Auto", 500000.0, 300000.0, 25.0, "2026-09-10").
		WillReturnResult(sqlmock.NewResult(42, 1))

	d, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Tipo:               "prestamo",
		Entidad:            "Banco Galicia",
		Descripcion:        "Auto",
		MontoTotal:         500000,
		SaldoPendiente:     300000,
		TasaInteres:        25,
		ProximoVencimiento: "2026-09-10",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID != 42 || d.Entidad != "Banco Galicia" {
		t.Errorf("unexpected deuda: %+v", d)
	}
}

func TestDeudaService_Update_Success(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "saldo_pendiente", "tasa_interes", "proximo_vencimiento", "created_at"}).
			AddRow(7, 1, "prestamo", "Banco", "", 500000.0, 300000.0, 25.0, nil, created))

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaUpdate)).
		WithArgs("prestamo", "Banco", "Saldo actualizado", 500000.0, 250000.0, 25.0, nil, int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	d, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Tipo:           "prestamo",
		Entidad:        "Banco",
		Descripcion:    "Saldo actualizado",
		MontoTotal:     500000,
		SaldoPendiente: 250000,
		TasaInteres:    25,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.SaldoPendiente != 250000 {
		t.Errorf("expected saldo 250000, got %v", d.SaldoPendiente)
	}
}

func TestDeudaService_Update_NotFound(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "saldo_pendiente", "tasa_interes", "proximo_vencimiento", "created_at"}))

	_, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Entidad:        "Banco",
		MontoTotal:     500000,
		SaldoPendiente: 250000,
	})
	if err == nil {
		t.Fatal("expected error for missing deuda")
	}
}

func TestDeudaService_Delete(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaDelete)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := svc.Delete(context.Background(), 1, 7); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDeudaService_Create_TipoPorDefecto(t *testing.T) {
	svc, mock := newDeudaService(t)

	// Tipo vacío: el servicio debe persistir "otro" por defecto y NULL vencimiento.
	mock.ExpectExec(regexp.QuoteMeta(queryDeudaInsert)).
		WithArgs(int64(1), "otro", "Banco", "", 100000.0, 50000.0, 0.0, nil).
		WillReturnResult(sqlmock.NewResult(9, 1))

	d, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Entidad:        "Banco",
		MontoTotal:     100000,
		SaldoPendiente: 50000,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.Tipo != "otro" {
		t.Errorf("expected tipo 'otro', got %q", d.Tipo)
	}
}

func TestDeudaService_Create_ErrorRepo(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaInsert)).
		WithArgs(int64(1), "otro", "Banco", "", 100.0, 50.0, 0.0, nil).
		WillReturnError(errors.New("db caido"))

	_, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Entidad:        "Banco",
		MontoTotal:     100,
		SaldoPendiente: 50,
	})
	if err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

func TestDeudaService_GetByID_Success(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "saldo_pendiente", "tasa_interes", "proximo_vencimiento", "created_at"}).
			AddRow(7, 1, "prestamo", "Banco", "Auto", 500000.0, 300000.0, 25.0, "2026-09-10", created))

	d, err := svc.GetByID(context.Background(), 7, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if d.ID != 7 || d.SaldoPendiente != 300000 {
		t.Errorf("unexpected deuda: %+v", d)
	}
}

func TestDeudaService_GetByID_NotFound(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(99), int64(1)).WillReturnError(model.ErrNotFound)

	if _, err := svc.GetByID(context.Background(), 99, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeudaService_Update_Invalid(t *testing.T) {
	svc, _ := newDeudaService(t)

	// Validaciones idénticas a Create; el saldo no puede exceder el monto.
	_, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Entidad:        "Banco",
		MontoTotal:     100,
		SaldoPendiente: 200,
	})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestDeudaService_Delete_NotFound(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaDelete)).
		WithArgs(int64(99), int64(1)).WillReturnError(model.ErrNotFound)

	if err := svc.Delete(context.Background(), 1, 99); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeudaService_List(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaList)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "saldo_pendiente", "tasa_interes", "proximo_vencimiento", "created_at"}).
			AddRow(1, 1, "tarjeta_credito", "Visa", "", 200000.0, 80000.0, 40.0, "2026-09-05", created).
			AddRow(2, 1, "prestamo", "Banco", "Auto", 500000.0, 300000.0, 25.0, nil, created))

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 deudas, got %d", len(list))
	}
	if list[0].Entidad != "Visa" || list[1].SaldoPendiente != 300000 {
		t.Errorf("unexpected list: %+v", list)
	}
}
