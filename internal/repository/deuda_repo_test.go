package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"finanzas_personales/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	queryDeudaInsertRepo = `INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?)`
	queryDeudaByIDRepo = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`
	queryDeudaListRepo = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento, created_at
		 FROM deudas WHERE usuario_id = ?
		 ORDER BY created_at DESC`
	queryDeudaUpdateRepo = `UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`
	queryDeudaDeleteRepo = `DELETE FROM deudas WHERE id=? AND usuario_id=?`
	queryDeudaSumRepo    = `SELECT COALESCE(SUM(monto_total), 0) FROM deudas WHERE usuario_id = ?`
)

func deudaCols() []string {
	return []string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "proximo_vencimiento", "created_at"}
}

func TestDeudaRepo_Create(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)

	// Sin vencimiento: la columna DATE recibe NULL.
	mock.ExpectExec(regexp.QuoteMeta(queryDeudaInsertRepo)).
		WithArgs(int64(1), "prestamo", "Banco", "Auto", 500000.0, nil).
		WillReturnResult(sqlmock.NewResult(7, 1))

	d := &model.Deuda{UsuarioID: 1, Tipo: "prestamo", Entidad: "Banco", Descripcion: "Auto", MontoTotal: 500000}
	if err := r.Create(context.Background(), d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID != 7 {
		t.Errorf("expected ID 7, got %d", d.ID)
	}

	// Con vencimiento: se pasa el string.
	mock.ExpectExec(regexp.QuoteMeta(queryDeudaInsertRepo)).
		WithArgs(int64(1), "tarjeta_credito", "Visa", "", 80000.0, "2026-09-05").
		WillReturnResult(sqlmock.NewResult(8, 1))
	d2 := &model.Deuda{UsuarioID: 1, Tipo: "tarjeta_credito", Entidad: "Visa", MontoTotal: 80000, ProximoVencimiento: "2026-09-05"}
	if err := r.Create(context.Background(), d2); err != nil {
		t.Fatalf("Create con vencimiento: %v", err)
	}
	if d2.ID != 8 {
		t.Errorf("expected ID 8, got %d", d2.ID)
	}
}

func TestDeudaRepo_FindByID(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)
	created := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByIDRepo)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaCols()).
			AddRow(7, 1, "prestamo", "Banco", "Auto", 500000.0, "2026-09-10", created))

	d, err := r.FindByID(context.Background(), 7, 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if d.Entidad != "Banco" || d.MontoTotal != 500000 || d.ProximoVencimiento != "2026-09-10" {
		t.Errorf("unexpected deuda: %+v", d)
	}

	// Vencimiento NULL: queda vacío.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByIDRepo)).
		WithArgs(int64(8), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaCols()).
			AddRow(8, 1, "otro", "X", "", 100.0, nil, created))
	d, err = r.FindByID(context.Background(), 8, 1)
	if err != nil {
		t.Fatalf("FindByID con vencimiento NULL: %v", err)
	}
	if d.ProximoVencimiento != "" {
		t.Errorf("expected empty vencimiento, got %q", d.ProximoVencimiento)
	}

	// No encontrada -> ErrNotFound.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByIDRepo)).
		WithArgs(int64(999), int64(1)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Error de DB se propaga.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByIDRepo)).
		WithArgs(int64(7), int64(1)).WillReturnError(errors.New("db caido"))
	if _, err := r.FindByID(context.Background(), 7, 1); err == nil {
		t.Fatal("expected db error")
	}
}

func TestDeudaRepo_FindByUsuarioID(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)
	created := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaListRepo)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaCols()).
			AddRow(1, 1, "tarjeta_credito", "Visa", "", 200000.0, "2026-09-05", created).
			AddRow(2, 1, "prestamo", "Banco", "Auto", 500000.0, nil, created))

	list, err := r.FindByUsuarioID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByUsuarioID: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 deudas, got %d", len(list))
	}
	if list[0].Tipo != "tarjeta_credito" || list[1].ProximoVencimiento != "" {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestDeudaRepo_Update(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)
	d := &model.Deuda{ID: 7, UsuarioID: 1, Tipo: "prestamo", Entidad: "Banco", Descripcion: "Auto", MontoTotal: 500000}

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaUpdateRepo)).
		WithArgs("prestamo", "Banco", "Auto", 500000.0, nil, int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Update(context.Background(), d); err != nil {
		t.Fatalf("Update: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaUpdateRepo)).
		WithArgs("prestamo", "Banco", "Auto", 500000.0, nil, int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Update(context.Background(), d); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeudaRepo_Delete(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaDeleteRepo)).
		WithArgs(int64(7), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Delete(context.Background(), 7, 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaDeleteRepo)).
		WithArgs(int64(99), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Delete(context.Background(), 99, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaDeleteRepo)).
		WithArgs(int64(7), int64(1)).WillReturnError(errors.New("db caido"))
	if err := r.Delete(context.Background(), 7, 1); err == nil {
		t.Fatal("expected db error")
	}
}

func TestDeudaRepo_SumMontoTotal(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewDeudaRepo(db)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaSumRepo)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(380000.0))
	sum, err := r.SumMontoTotal(context.Background(), 1)
	if err != nil {
		t.Fatalf("SumMontoTotal: %v", err)
	}
	if sum != 380000 {
		t.Errorf("expected 380000, got %v", sum)
	}

	// Sin deudas: COALESCE devuelve 0.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaSumRepo)).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	sum, err = r.SumMontoTotal(context.Background(), 2)
	if err != nil {
		t.Fatalf("SumMontoTotal vacío: %v", err)
	}
	if sum != 0 {
		t.Errorf("expected 0, got %v", sum)
	}
}
