package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"administracion-financiera/internal/model"
	"administracion-financiera/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
)

const queryCostoFijoInsert = `INSERT INTO costos_fijos (usuario_id, categoria_id, descripcion, monto_estimado, dia_vencimiento, activo, tipo_periodo)
		 VALUES (?,?,?,?,?,?,?)`

func newCostoFijoService(t *testing.T) (*CostoFijoService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewCostoFijoService(repository.NewCostoFijoRepo(db)), mock
}

func TestCostoFijoService_Create_Valid(t *testing.T) {
	svc, mock := newCostoFijoService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryCostoFijoInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))

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
