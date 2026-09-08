package repository

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

func newRepoDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func mesCols() []string {
	return []string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}
}

func TestMesRepo_Cerrar(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewMesRepo(db)

	query := regexp.QuoteMeta(`UPDATE meses SET estado='cerrado' WHERE id=? AND usuario_id=? AND estado='abierto'`)

	mock.ExpectExec(query).WithArgs(int64(9), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Cerrar(context.Background(), 9, 1); err != nil {
		t.Fatalf("Cerrar: %v", err)
	}

	mock.ExpectExec(query).WithArgs(int64(99), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Cerrar(context.Background(), 99, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	mock.ExpectExec(query).WithArgs(int64(9), int64(1)).WillReturnError(errors.New("db caido"))
	if err := r.Cerrar(context.Background(), 9, 1); err == nil {
		t.Fatal("expected db error")
	}
}

func TestCategoriaRepo_FindByID(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewCategoriaRepo(db)

	query := regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias WHERE id = ?`)
	created := time.Now()
	mock.ExpectQuery(query).WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(6, "Servicios", "egreso", "💡", false, nil, created))

	cat, err := r.FindByID(context.Background(), 6)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if cat.Nombre != "Servicios" {
		t.Errorf("expected Servicios, got %q", cat.Nombre)
	}

	mock.ExpectQuery(query).WithArgs(int64(999)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999); err == nil {
		t.Fatal("expected sql.ErrNoRows propagated")
	}
}

func TestUsuarioRepo_FindByID(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewUsuarioRepo(db)

	query := regexp.QuoteMeta(`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado
		 FROM usuarios WHERE id = ?`)
	created := time.Now()
	mock.ExpectQuery(query).WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(1, "Pepe", "pepe@test.com", "hash", "ARS", created, true))

	u, err := r.FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if u.Email != "pepe@test.com" {
		t.Errorf("unexpected email %q", u.Email)
	}

	mock.ExpectQuery(query).WithArgs(int64(999)).WillReturnError(sql.ErrNoRows)
	if _, err := r.FindByID(context.Background(), 999); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTransaccionRepo_FindByMesID(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	query := regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.mes_id = ? AND t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)
	created := time.Now()
	mock.ExpectQuery(query).WithArgs(int64(9), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "ingreso", 1000.0, created, 1, "Sueldo", "", "transferencia", "confirmado", 9, created, created))

	ts, err := r.FindByMesID(context.Background(), 9, 1)
	if err != nil {
		t.Fatalf("FindByMesID: %v", err)
	}
	if len(ts) != 1 || ts[0].ID != 1 {
		t.Errorf("unexpected result: %+v", ts)
	}
}

func TestTransaccionRepo_FindByRango(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	query := regexp.QuoteMeta(`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`)
	created := time.Now()
	mock.ExpectQuery(query).WithArgs(int64(1), "2026-08-01", "2026-08-10").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}).
			AddRow(1, 1, "egreso", 45000.0, created, 6, "Servicios", "", "debito", "confirmado", 9, created, created).
			AddRow(2, 1, "ingreso", 150000.0, created, 1, "Sueldo", "", "transferencia", "confirmado", 9, created, created))

	ts, err := r.FindByRango(context.Background(), 1, "2026-08-01", "2026-08-10")
	if err != nil {
		t.Fatalf("FindByRango: %v", err)
	}
	if len(ts) != 2 || ts[0].ID != 1 || ts[1].Categoria != "Sueldo" {
		t.Errorf("unexpected result: %+v", ts)
	}

	// Sin resultados no es error.
	mock.ExpectQuery(query).WithArgs(int64(1), "2026-01-01", "2026-01-31").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "monto", "fecha", "categoria_id", "categoria", "descripcion", "medio_pago", "estado", "mes_id", "created_at", "updated_at"}))
	if ts, err := r.FindByRango(context.Background(), 1, "2026-01-01", "2026-01-31"); err != nil || len(ts) != 0 {
		t.Fatalf("expected empty, got %v (%v)", ts, err)
	}
}

func TestTransaccionRepo_SumByCategoria(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	query := regexp.QuoteMeta(`SELECT categoria_id, SUM(monto) FROM transacciones
		 WHERE usuario_id=? AND fecha>=? AND fecha<=? AND tipo='egreso'
		 GROUP BY categoria_id`)
	mock.ExpectQuery(query).WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows([]string{"categoria_id", "sum"}).
			AddRow(5, 30000.0).
			AddRow(6, 20000.0))

	m, err := r.SumByCategoria(context.Background(), 1, "2026-08")
	if err != nil {
		t.Fatalf("SumByCategoria: %v", err)
	}
	if m[5] != 30000 || m[6] != 20000 {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestTransaccionRepo_CreateAjuste(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	query := regexp.QuoteMeta(`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, estado)
		 VALUES (?, 'egreso', ?, ?, ?, ?, 'ajuste')`)
	mock.ExpectExec(query).WithArgs(int64(1), 5000.0, "2026-08-01", int64(5), "ajuste manual").
		WillReturnResult(sqlmock.NewResult(7, 1))

	if err := r.CreateAjuste(context.Background(), 1, 5000, 5, "ajuste manual", "2026-08"); err != nil {
		t.Fatalf("CreateAjuste: %v", err)
	}
}

func TestTransaccionRepo_UpdateDelete_NotFound(t *testing.T) {
	db, mock := newRepoDB(t)
	r := NewTransaccionRepo(db)

	upd := regexp.QuoteMeta(`UPDATE transacciones SET tipo=?, monto=?, fecha=?, categoria_id=?, descripcion=?, medio_pago=?, updated_at=NOW()
		 WHERE id=? AND usuario_id=?`)
	mock.ExpectExec(upd).
		WithArgs("egreso", 100.0, "2026-08-10", int64(5), "x", "", int64(99), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Update(context.Background(), &model.Transaccion{
		ID: 99, UsuarioID: 1, Tipo: "egreso", Monto: 100, Fecha: "2026-08-10", CategoriaID: 5, Descripcion: "x",
	}); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on update, got %v", err)
	}

	del := regexp.QuoteMeta(`DELETE FROM transacciones WHERE id=? AND usuario_id=?`)
	mock.ExpectExec(del).WithArgs(int64(99), int64(1)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Delete(context.Background(), 99, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on delete, got %v", err)
	}
}
