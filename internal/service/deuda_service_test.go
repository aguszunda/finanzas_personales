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
	queryDeudaInsert = `INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?,?,?)`
	queryDeudaByID = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`
	queryDeudaList = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE usuario_id = ?
		 ORDER BY created_at DESC`
	queryDeudaUpdate = `UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, categoria_id=?, medio_pago=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`
	queryDeudaDelete = `DELETE FROM deudas WHERE id=? AND usuario_id=?`
	queryDeudaPagar  = `UPDATE deudas SET estado = 'pagada'
		 WHERE id=? AND usuario_id=? AND estado='pendiente'`
)

func deudaSvcCols() []string {
	return []string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "categoria_id", "medio_pago", "proximo_vencimiento", "estado", "created_at"}
}

func newDeudaService(t *testing.T) (*DeudaService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := repository.NewDeudaRepo(db)
	catRepo := repository.NewCategoriaRepo(db)
	transSvc := NewTransaccionService(repository.NewTransaccionRepo(db), repository.NewMesRepo(db))
	return NewDeudaService(repo, catRepo, transSvc), mock
}

func TestDeudaService_Create_Validaciones(t *testing.T) {
	svc, _ := newDeudaService(t)

	tests := []CreateDeudaInput{
		{Entidad: "", MontoTotal: 100},
		{Entidad: "Banco", MontoTotal: 0},
		{Entidad: "Banco", MontoTotal: -1},
		{Entidad: "Banco", MontoTotal: 100, Tipo: "cripto"},
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
		WithArgs(int64(1), "prestamo", "Banco Galicia", "Auto", 500000.0, nil, "", "2026-09-10").
		WillReturnResult(sqlmock.NewResult(42, 1))

	d, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Tipo:               "prestamo",
		Entidad:            "Banco Galicia",
		Descripcion:        "Auto",
		MontoTotal:         500000,
		CategoriaID:        0,
		MedioPago:          "",
		ProximoVencimiento: "2026-09-10",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID != 42 || d.Entidad != "Banco Galicia" || d.MontoTotal != 500000 {
		t.Errorf("unexpected deuda: %+v", d)
	}
}

func TestDeudaService_Update_Success(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "prestamo", "Banco", "", 500000.0, 0, "", nil, "pendiente", created))

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaUpdate)).
		WithArgs("prestamo", "Banco", "Monto actualizado", 450000.0, nil, "", nil, int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	d, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Tipo:        "prestamo",
		Entidad:     "Banco",
		Descripcion: "Monto actualizado",
		MontoTotal:  450000,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.MontoTotal != 450000 {
		t.Errorf("expected monto 450000, got %v", d.MontoTotal)
	}
}

func TestDeudaService_Update_NotFound(t *testing.T) {
	svc, mock := newDeudaService(t)

	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "tipo", "entidad", "descripcion", "monto_total", "proximo_vencimiento", "created_at"}))

	_, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Entidad:    "Banco",
		MontoTotal: 500000,
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
		WithArgs(int64(1), "otro", "Banco", "", 100000.0, nil, "", nil).
		WillReturnResult(sqlmock.NewResult(9, 1))

	d, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Entidad:    "Banco",
		MontoTotal: 100000,
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
		WithArgs(int64(1), "otro", "Banco", "", 100.0, nil, "", nil).
		WillReturnError(errors.New("db caido"))

	_, err := svc.Create(context.Background(), 1, CreateDeudaInput{
		Entidad:    "Banco",
		MontoTotal: 100,
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
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "prestamo", "Banco", "Auto", 500000.0, 5, "debito", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), "pendiente", created))

	d, err := svc.GetByID(context.Background(), 7, 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if d.ID != 7 || d.MontoTotal != 500000 {
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

	// Validaciones idénticas a Create.
	_, err := svc.Update(context.Background(), 1, 7, CreateDeudaInput{
		Entidad:    "Banco",
		MontoTotal: 0,
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
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(1, 1, "tarjeta_credito", "Visa", "", 200000.0, 7, "", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created).
			AddRow(2, 1, "prestamo", "Banco", "Auto", 500000.0, 5, "debito", nil, "pendiente", created))

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 deudas, got %d", len(list))
	}
	if list[0].Entidad != "Visa" || list[1].MontoTotal != 500000 {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestDeudaService_MarcarPagada_Success(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// 1) Busca la deuda.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "transferencia", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))

	// 2) Valida que la categoría 7 ("Comida") sea de tipo egreso.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(7, "Comida", "egreso", "🍽️", false, nil, created))

	// 3) TransaccionService.Create: FindOrCreate mes (find por periodo -> not found).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnError(model.ErrNotFound)

	// 4) Insert del mes por upsert.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`)).
		WithArgs(int64(1), "2026-08").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 5) Re-lectura del mes creado.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, 1, "2026-08", "abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))

	// 6) INSERT del egreso (hereda categoría y medio de pago de la deuda).
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)).
		WithArgs(int64(1), "egreso", 80000.0, "2026-08-10", int64(7), "Pago deuda: Visa", "transferencia", false, nil, nil, "confirmado", int64(1)).
		WillReturnResult(sqlmock.NewResult(9, 1))

	// 7) UPDATE deuda -> pagada.
	mock.ExpectExec(regexp.QuoteMeta(queryDeudaPagar)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	d, err := svc.MarcarPagada(context.Background(), 1, 7, 7, "2026-08-10", "")
	if err != nil {
		t.Fatalf("MarcarPagada: %v", err)
	}
	if d.Estado != "pagada" {
		t.Errorf("expected estado pagada, got %q", d.Estado)
	}
}

func TestDeudaService_MarcarPagada_MesCerrado(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// La deuda es válida y la categoría de egreso existe.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(7, "Comida", "egreso", "🍽️", false, nil, created))

	// El mes de la fecha pedida está cerrado -> ErrMesCerrado.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, 1, "2026-08", "cerrado", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))

	_, err := svc.MarcarPagada(context.Background(), 1, 7, 7, "2026-08-10", "")
	if !errors.Is(err, model.ErrMesCerrado) {
		t.Fatalf("expected ErrMesCerrado, got %v", err)
	}
}

func TestDeudaService_MarcarPagada_CategoriaPorDefecto(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// La deuda tiene categoría 7 y medio de pago; el form manda categoriaID 0
	// -> el servicio debe usar la categoría de la deuda.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "debito", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))

	// Categoría 7 de la deuda es egreso.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(7, "Comida", "egreso", "🍽️", false, nil, created))

	// FindOrCreate mes 2026-08: not found -> insert -> re-lectura.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnError(model.ErrNotFound)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`)).
		WithArgs(int64(1), "2026-08").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, 1, "2026-08", "abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))

	// El egreso usa la categoría 7 y el medio de pago "debito" de la deuda.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)).
		WithArgs(int64(1), "egreso", 80000.0, "2026-08-10", int64(7), "Pago deuda: Visa", "debito", false, nil, nil, "confirmado", int64(1)).
		WillReturnResult(sqlmock.NewResult(9, 1))

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaPagar)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	d, err := svc.MarcarPagada(context.Background(), 1, 7, 0, "2026-08-10", "")
	if err != nil {
		t.Fatalf("MarcarPagada: %v", err)
	}
	if d.Estado != "pagada" {
		t.Errorf("expected estado pagada, got %q", d.Estado)
	}
}

func TestDeudaService_MarcarPagada_MedioPagoOverride(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// La deuda tiene medio de pago "debito" pero el form manda "efectivo":
	// el egreso debe usar el medio seleccionado en el pago.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "debito", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(7, "Comida", "egreso", "🍽️", false, nil, created))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnError(model.ErrNotFound)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`)).
		WithArgs(int64(1), "2026-08").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"id", "usuario_id", "periodo", "estado", "ingresos_total", "egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "pasivos_total", "patrimonio", "created_at"}).
			AddRow(1, 1, "2026-08", "abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, created))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)).
		WithArgs(int64(1), "egreso", 80000.0, "2026-08-10", int64(7), "Pago deuda: Visa", "efectivo", false, nil, nil, "confirmado", int64(1)).
		WillReturnResult(sqlmock.NewResult(9, 1))

	mock.ExpectExec(regexp.QuoteMeta(queryDeudaPagar)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if _, err := svc.MarcarPagada(context.Background(), 1, 7, 0, "2026-08-10", "efectivo"); err != nil {
		t.Fatalf("MarcarPagada: %v", err)
	}
}

func TestDeudaService_MarcarPagada_Errores(t *testing.T) {
	svc, mock := newDeudaService(t)
	created := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// Ya pagada -> ErrInvalidInput, sin egreso.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pagada", created))
	if _, err := svc.MarcarPagada(context.Background(), 1, 7, 7, "", ""); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for deuda ya pagada, got %v", err)
	}

	// Categoría de ingreso o inexistente -> ErrInvalidInput.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "tipo", "icono", "es_personalizada", "usuario_id", "created_at"}).
			AddRow(1, "Sueldo", "ingreso", "💰", false, nil, created))
	if _, err := svc.MarcarPagada(context.Background(), 1, 7, 1, "", ""); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for categoria no egreso, got %v", err)
	}

	// Fecha mal formada -> ErrInvalidInput.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(sqlmock.NewRows(deudaSvcCols()).
			AddRow(7, 1, "tarjeta_credito", "Visa", "", 80000.0, 7, "", time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "pendiente", created))
	if _, err := svc.MarcarPagada(context.Background(), 1, 7, 7, "no-es-fecha", ""); !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for fecha malformada, got %v", err)
	}

	// Deuda inexistente -> ErrNotFound.
	mock.ExpectQuery(regexp.QuoteMeta(queryDeudaByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)
	if _, err := svc.MarcarPagada(context.Background(), 1, 99, 7, "", ""); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
