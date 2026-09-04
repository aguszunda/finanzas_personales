package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"optipay/internal/middleware"
	"optipay/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var (
	mesCols = []string{"id", "usuario_id", "periodo", "estado", "ingresos_total",
		"egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado",
		"pasivos_total", "patrimonio", "created_at"}

	transCols = []string{"id", "usuario_id", "tipo", "monto", "fecha",
		"categoria_id", "categoria", "descripcion", "medio_pago", "es_fijo",
		"cuotas_total", "cuota_actual", "estado", "mes_id", "created_at", "updated_at"}

	cfCols = []string{"id", "usuario_id", "categoria_id", "categoria", "descripcion",
		"monto_estimado", "dia_vencimiento", "activo", "tipo_periodo", "created_at"}

	deudaCols = []string{"id", "usuario_id", "tipo", "entidad", "descripcion",
		"monto_total", "categoria_id", "medio_pago", "proximo_vencimiento",
		"estado", "created_at"}

	catCols = []string{"id", "nombre", "tipo", "icono", "es_personalizada",
		"usuario_id", "created_at"}
)

func fixedTime() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

func mesRows(id int64, periodo, estado string) *sqlmock.Rows {
	return sqlmock.NewRows(mesCols).
		AddRow(id, int64(1), periodo, estado, 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0, fixedTime())
}

func transRow(id int64, tipo string, monto float64, mesID int64) *sqlmock.Rows {
	return sqlmock.NewRows(transCols).
		AddRow(id, int64(1), tipo, monto, fixedTime(), int64(5), "Sueldo", "desc",
			"transferencia", false, nil, nil, "confirmado", mesID, fixedTime(), fixedTime())
}

func cfRow(id, catID int64, desc string, monto float64, dia int, activo bool, periodo string) *sqlmock.Rows {
	return sqlmock.NewRows(cfCols).
		AddRow(id, int64(1), catID, "Servicios", desc, monto, dia, activo, periodo, fixedTime())
}

func deudaRow(id int64, tipo, entidad string, monto float64, catID int64, estado string) *sqlmock.Rows {
	return sqlmock.NewRows(deudaCols).
		AddRow(id, int64(1), tipo, entidad, "", monto, catID, "", nil, estado, fixedTime())
}

func catRow(id int64, nombre, tipo string) *sqlmock.Rows {
	return sqlmock.NewRows(catCols).
		AddRow(id, nombre, tipo, "icon", false, nil, fixedTime())
}

func catRowsEmpty() *sqlmock.Rows { return sqlmock.NewRows(catCols) }

// ctxWithHTMX builds a context with both userID and the HTMX flag set.
func ctxWithHTMX(userID int64) context.Context {
	ctx := ctxWithUserID(userID)
	return context.WithValue(ctx, middleware.IsHTMXKey, true)
}

// routeParam builds a chi.Router that sets URL params for a single-route handler.
func routeParam(method, path string, handler http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	switch method {
	case http.MethodGet:
		r.Get(path, handler)
	case http.MethodPost:
		r.Post(path, handler)
	case http.MethodPut:
		r.Put(path, handler)
	case http.MethodDelete:
		r.Delete(path, handler)
	case http.MethodPatch:
		r.Patch(path, handler)
	}
	return r
}

// ---------------------------------------------------------------------------
// SQL query constants (whitespace must match repo source exactly)
// ---------------------------------------------------------------------------

const (
	qMesByPeriodo = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`
	qMesByID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`
	qMesInsert = `INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`
	qMesUpdate = `UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?, pasivos_total=?, patrimonio=?
		 WHERE id=? AND usuario_id=?`
	qSumSuperavitAnterior = `SELECT COALESCE(SUM(superavit), 0) FROM meses
		 WHERE usuario_id = ? AND estado = 'cerrado' AND periodo < ?`
	qSumMontoTotal = `SELECT COALESCE(SUM(monto_total), 0) FROM deudas WHERE usuario_id = ? AND estado != 'pagada'`

	qTransFindByUsuarioID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC
		 LIMIT ? OFFSET ?`
	qTransFindByID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.id = ? AND t.usuario_id = ?`
	qTransFindByPeriodo = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`
	qTransInsert = `INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`
	qTransUpdate = `UPDATE transacciones SET tipo=?, monto=?, fecha=?, categoria_id=?, descripcion=?, medio_pago=?, es_fijo=?, cuotas_total=?, cuota_actual=?, updated_at=NOW()
		 WHERE id=? AND usuario_id=?`
	qTransDelete = `DELETE FROM transacciones WHERE id=? AND usuario_id=?`

	qCFFindByUsuarioID = `SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ?
		 ORDER BY cf.dia_vencimiento, cf.descripcion`
	qCFFindByID = `SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`
	qCFInsert = `INSERT INTO costos_fijos (usuario_id, categoria_id, descripcion, monto_estimado, dia_vencimiento, activo, tipo_periodo)
		 VALUES (?,?,?,?,?,?,?)`
	qCFUpdate = `UPDATE costos_fijos SET categoria_id=?, descripcion=?, monto_estimado=?, dia_vencimiento=?, activo=?, tipo_periodo=?
		 WHERE id=? AND usuario_id=?`
	qCFDelete        = `DELETE FROM costos_fijos WHERE id=? AND usuario_id=?`
	qCFPrecargaCount = `SELECT COUNT(*) FROM transacciones
		 WHERE usuario_id = ? AND es_fijo = TRUE AND estado = 'pendiente'
		   AND categoria_id = ? AND descripcion = ?
		   AND fecha >= ? AND fecha <= ?`
	qCFPrecargaInsert = `INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, estado)
		 VALUES (?, 'egreso', ?, ?, ?, ?, 'debito', TRUE, 'pendiente')`
	qCFActivos = `SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ? AND cf.activo = TRUE
		 ORDER BY cf.dia_vencimiento, cf.descripcion`

	qDeudaFindByID = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`
	qDeudaFindByUsuarioID = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE usuario_id = ?
		 ORDER BY created_at DESC`
	qDeudaInsert = `INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?,?,?)`
	qDeudaUpdate = `UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, categoria_id=?, medio_pago=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`
	qDeudaDelete       = `DELETE FROM deudas WHERE id=? AND usuario_id=?`
	qDeudaMarcarPagada = `UPDATE deudas SET estado = 'pagada'
		 WHERE id=? AND usuario_id=? AND estado='pendiente'`
	qDeudaFindByRango = `SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE usuario_id = ? AND estado != 'pagada' AND DATE(created_at) BETWEEN ? AND ?
		 ORDER BY created_at DESC`

	qCatFindAll = `SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`
)

// ============================================================================
// TransaccionHandler tests
// ============================================================================

func TestTransaccionHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByUsuarioID)).
		WithArgs(int64(1), 50, 0).
		WillReturnRows(sqlmock.NewRows(transCols))

	r := routeParam("GET", "/api/transacciones", f.transH.List)
	req := httptest.NewRequest("GET", "/api/transacciones", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTransaccionHandler_ListByPeriodo(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(transRow(1, "ingreso", 1000, 9))

	r := routeParam("GET", "/api/transacciones", f.transH.List)
	req := httptest.NewRequest("GET", "/api/transacciones?periodo=2026-08", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ingreso") {
		t.Fatal("expected body to contain transaccion data")
	}
}

func TestTransaccionHandler_GetByID(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(42), int64(1)).
		WillReturnRows(transRow(42, "ingreso", 5000, 9))

	r := routeParam("GET", "/api/transacciones/{id}", f.transH.GetByID)
	req := httptest.NewRequest("GET", "/api/transacciones/42", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTransaccionHandler_GetByID_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/transacciones/{id}", f.transH.GetByID)
	req := httptest.NewRequest("GET", "/api/transacciones/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Create(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	f.mock.ExpectExec(regexp.QuoteMeta(qTransInsert)).
		WithArgs(int64(1), "ingreso", 1000.0, "2026-08-10", int64(1), "Sueldo",
			"transferencia", false, nil, nil, "confirmado", int64(9)).
		WillReturnResult(sqlmock.NewResult(4, 1))

	body := `{"tipo":"ingreso","monto":1000,"fecha":"2026-08-10","categoria_id":1,"descripcion":"Sueldo","medio_pago":"transferencia"}`
	req := httptest.NewRequest("POST", "/api/transacciones", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.transH.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTransaccionHandler_Create_BadJSON(t *testing.T) {
	f := newHandlerFixture(t)

	req := httptest.NewRequest("POST", "/api/transacciones", strings.NewReader("not-json")).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.transH.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Create_InvalidMonto(t *testing.T) {
	f := newHandlerFixture(t)

	body := `{"tipo":"ingreso","monto":0,"fecha":"2026-08-10","categoria_id":1}`
	req := httptest.NewRequest("POST", "/api/transacciones", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.transH.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Create_MesCerrado(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(mesRows(9, "2026-08", "cerrado"))

	body := `{"tipo":"ingreso","monto":1000,"fecha":"2026-08-10","categoria_id":1,"descripcion":"Sueldo"}`
	req := httptest.NewRequest("POST", "/api/transacciones", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.transH.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Update(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(4), int64(1)).
		WillReturnRows(transRow(4, "ingreso", 1000, 9))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	f.mock.ExpectExec(regexp.QuoteMeta(qTransUpdate)).
		WithArgs("egreso", 2000.0, "2026-08-10", int64(2), "nueva desc", "debito",
			false, nil, nil, int64(4), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"tipo":"egreso","monto":2000,"fecha":"2026-08-10","categoria_id":2,"descripcion":"nueva desc","medio_pago":"debito"}`
	r := routeParam("PUT", "/api/transacciones/{id}", f.transH.Update)
	req := httptest.NewRequest("PUT", "/api/transacciones/4", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTransaccionHandler_Update_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	body := `{"tipo":"egreso","monto":2000,"fecha":"2026-08-10","categoria_id":2}`
	r := routeParam("PUT", "/api/transacciones/{id}", f.transH.Update)
	req := httptest.NewRequest("PUT", "/api/transacciones/99", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Delete(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(4), int64(1)).
		WillReturnRows(transRow(4, "egreso", 100, 9))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	f.mock.ExpectExec(regexp.QuoteMeta(qTransDelete)).
		WithArgs(int64(4), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/transacciones/{id}", f.transH.Delete)
	req := httptest.NewRequest("DELETE", "/api/transacciones/4", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestTransaccionHandler_Delete_HTMX(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(4), int64(1)).
		WillReturnRows(transRow(4, "egreso", 100, 9))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	f.mock.ExpectExec(regexp.QuoteMeta(qTransDelete)).
		WithArgs(int64(4), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/transacciones/{id}", f.transH.Delete)
	req := httptest.NewRequest("DELETE", "/api/transacciones/4", nil).WithContext(ctxWithHTMX(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("HX-Redirect") != "/api/transacciones/page" {
		t.Fatalf("expected HX-Redirect header, got %q", rec.Header().Get("HX-Redirect"))
	}
}

func TestTransaccionHandler_Delete_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("DELETE", "/api/transacciones/{id}", f.transH.Delete)
	req := httptest.NewRequest("DELETE", "/api/transacciones/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ============================================================================
// CostoFijoHandler tests
// ============================================================================

func TestCostoFijoHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByUsuarioID)).
		WithArgs(int64(1)).
		WillReturnRows(cfRow(3, 6, "Internet", 5000, 5, true, "mensual"))

	r := routeParam("GET", "/api/costos-fijos", f.cfH.List)
	req := httptest.NewRequest("GET", "/api/costos-fijos", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_GetByID(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByID)).
		WithArgs(int64(3), int64(1)).
		WillReturnRows(cfRow(3, 6, "Internet", 5000, 5, true, "mensual"))

	r := routeParam("GET", "/api/costos-fijos/{id}", f.cfH.GetByID)
	req := httptest.NewRequest("GET", "/api/costos-fijos/3", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_GetByID_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/costos-fijos/{id}", f.cfH.GetByID)
	req := httptest.NewRequest("GET", "/api/costos-fijos/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_Create(t *testing.T) {
	f := newHandlerFixture(t)
	periodo := time.Now().Format("2006-01")

	// 1. INSERT costofijo
	f.mock.ExpectExec(regexp.QuoteMeta(qCFInsert)).
		WithArgs(int64(1), int64(6), "Internet", 12000.0, 5, true, "mensual").
		WillReturnResult(sqlmock.NewResult(3, 1))
	// 2. syncMesActual: FindOrCreate → mes exists, abierto
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(mesRows(1, periodo, "abierto"))
	// 3. PrecargarEnPeriodo: COUNT → 0
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFPrecargaCount)).
		WithArgs(int64(1), int64(6), "Internet", periodo+"-01", periodo+"-31").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// 4. PrecargarEnPeriodo: INSERT transaccion
	f.mock.ExpectExec(regexp.QuoteMeta(qCFPrecargaInsert)).
		WithArgs(int64(1), 12000.0, periodo+"-01", int64(6), "Internet").
		WillReturnResult(sqlmock.NewResult(1, 1))

	body := `{"categoria_id":6,"descripcion":"Internet","monto_estimado":12000,"dia_vencimiento":5,"tipo_periodo":"mensual"}`
	req := httptest.NewRequest("POST", "/api/costos-fijos", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.cfH.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCostoFijoHandler_Create_Invalid(t *testing.T) {
	f := newHandlerFixture(t)

	body := `{"categoria_id":6,"descripcion":"","monto_estimado":0,"dia_vencimiento":5}`
	req := httptest.NewRequest("POST", "/api/costos-fijos", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.cfH.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_Update(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByID)).
		WithArgs(int64(3), int64(1)).
		WillReturnRows(cfRow(3, 6, "Internet", 5000, 5, true, "mensual"))
	f.mock.ExpectExec(regexp.QuoteMeta(qCFUpdate)).
		WithArgs(int64(7), "Net", 6000.0, 10, true, "mensual", int64(3), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"categoria_id":7,"descripcion":"Net","monto_estimado":6000,"dia_vencimiento":10,"tipo_periodo":"mensual"}`
	r := routeParam("PUT", "/api/costos-fijos/{id}", f.cfH.Update)
	req := httptest.NewRequest("PUT", "/api/costos-fijos/3", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCostoFijoHandler_Update_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	body := `{"categoria_id":7,"descripcion":"Net","monto_estimado":6000,"dia_vencimiento":10,"tipo_periodo":"mensual"}`
	r := routeParam("PUT", "/api/costos-fijos/{id}", f.cfH.Update)
	req := httptest.NewRequest("PUT", "/api/costos-fijos/99", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_Toggle(t *testing.T) {
	f := newHandlerFixture(t)
	// Toggle from active→inactive: FindByID returns activo=true, Update sets activo=false.
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFFindByID)).
		WithArgs(int64(3), int64(1)).
		WillReturnRows(cfRow(3, 6, "Internet", 5000, 5, true, "mensual"))
	f.mock.ExpectExec(regexp.QuoteMeta(qCFUpdate)).
		WithArgs(int64(6), "Internet", 5000.0, 5, false, "mensual", int64(3), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("PATCH", "/api/costos-fijos/{id}/toggle", f.cfH.Toggle)
	req := httptest.NewRequest("PATCH", "/api/costos-fijos/3/toggle", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCostoFijoHandler_Delete(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qCFDelete)).
		WithArgs(int64(3), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/costos-fijos/{id}", f.cfH.Delete)
	req := httptest.NewRequest("DELETE", "/api/costos-fijos/3", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestCostoFijoHandler_Delete_HTMX(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qCFDelete)).
		WithArgs(int64(3), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/costos-fijos/{id}", f.cfH.Delete)
	req := httptest.NewRequest("DELETE", "/api/costos-fijos/3", nil).WithContext(ctxWithHTMX(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("HX-Redirect") != "/api/costos-fijos/page" {
		t.Fatalf("expected HX-Redirect to /api/costos-fijos/page, got %q", rec.Header().Get("HX-Redirect"))
	}
}

func TestCostoFijoHandler_Delete_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qCFDelete)).
		WithArgs(int64(99), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	r := routeParam("DELETE", "/api/costos-fijos/{id}", f.cfH.Delete)
	req := httptest.NewRequest("DELETE", "/api/costos-fijos/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ============================================================================
// DeudaHandler tests
// ============================================================================

func TestDeudaHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByUsuarioID)).
		WithArgs(int64(1)).
		WillReturnRows(deudaRow(1, "prestamo", "Banco", 500000, 0, "pendiente"))

	r := routeParam("GET", "/api/deudas", f.deudaH.List)
	req := httptest.NewRequest("GET", "/api/deudas", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDeudaHandler_GetByID(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(deudaRow(7, "prestamo", "Banco", 500000, 5, "pendiente"))

	r := routeParam("GET", "/api/deudas/{id}", f.deudaH.GetByID)
	req := httptest.NewRequest("GET", "/api/deudas/7", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDeudaHandler_GetByID_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/deudas/{id}", f.deudaH.GetByID)
	req := httptest.NewRequest("GET", "/api/deudas/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeudaHandler_Create(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaInsert)).
		WithArgs(int64(1), "prestamo", "Banco Galicia", "Auto", 500000.0,
			nil, "", "2026-09-10").
		WillReturnResult(sqlmock.NewResult(42, 1))

	body := `{"tipo":"prestamo","entidad":"Banco Galicia","descripcion":"Auto","monto_total":500000,"proximo_vencimiento":"2026-09-10"}`
	req := httptest.NewRequest("POST", "/api/deudas", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.deudaH.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeudaHandler_Create_Invalid(t *testing.T) {
	f := newHandlerFixture(t)

	body := `{"entidad":"","monto_total":0}`
	req := httptest.NewRequest("POST", "/api/deudas", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.deudaH.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeudaHandler_Update(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(deudaRow(7, "prestamo", "Banco", 500000, 0, "pendiente"))
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaUpdate)).
		WithArgs("prestamo", "Banco", "Actualizado", 450000.0, nil, "", nil,
			int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"tipo":"prestamo","entidad":"Banco","descripcion":"Actualizado","monto_total":450000}`
	r := routeParam("PUT", "/api/deudas/{id}", f.deudaH.Update)
	req := httptest.NewRequest("PUT", "/api/deudas/7", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeudaHandler_Update_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	body := `{"entidad":"Banco","monto_total":500000}`
	r := routeParam("PUT", "/api/deudas/{id}", f.deudaH.Update)
	req := httptest.NewRequest("PUT", "/api/deudas/99", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeudaHandler_Delete(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaDelete)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/deudas/{id}", f.deudaH.Delete)
	req := httptest.NewRequest("DELETE", "/api/deudas/7", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestDeudaHandler_Delete_HTMX(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaDelete)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("DELETE", "/api/deudas/{id}", f.deudaH.Delete)
	req := httptest.NewRequest("DELETE", "/api/deudas/7", nil).WithContext(ctxWithHTMX(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("HX-Redirect") != "/api/deudas/page" {
		t.Fatalf("expected HX-Redirect, got %q", rec.Header().Get("HX-Redirect"))
	}
}

func TestDeudaHandler_Delete_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaDelete)).
		WithArgs(int64(99), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	r := routeParam("DELETE", "/api/deudas/{id}", f.deudaH.Delete)
	req := httptest.NewRequest("DELETE", "/api/deudas/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeudaHandler_MarcarPagada(t *testing.T) {
	f := newHandlerFixture(t)
	periodo := time.Now().Format("2006-01")

	// 1) FindByID deuda
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(deudaRow(7, "tarjeta_credito", "Visa", 80000, 7, "pendiente"))
	// 2) validarCategoriaEgreso → FindAll
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(1)).
		WillReturnRows(catRow(7, "Comida", "egreso"))
	// 3) TransaccionService.Create → FindOrCreate mes (already exists)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(mesRows(1, periodo, "abierto"))
	// 4) INSERT transaccion egreso (fecha defaults to today)
	f.mock.ExpectExec(regexp.QuoteMeta(qTransInsert)).
		WithArgs(int64(1), "egreso", 80000.0, time.Now().Format("2006-01-02"), int64(7), "Pago deuda: Visa",
			"", false, nil, nil, "confirmado", int64(1)).
		WillReturnResult(sqlmock.NewResult(9, 1))
	// 5) MarcarPagada
	f.mock.ExpectExec(regexp.QuoteMeta(qDeudaMarcarPagada)).
		WithArgs(int64(7), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := `{"categoria_id":7}`
	r := routeParam("POST", "/api/deudas/{id}/pagar", f.deudaH.MarcarPagada)
	req := httptest.NewRequest("POST", "/api/deudas/7/pagar", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeudaHandler_MarcarPagada_MesCerrado(t *testing.T) {
	f := newHandlerFixture(t)
	periodo := time.Now().Format("2006-01")

	// 1) FindByID deuda
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByID)).
		WithArgs(int64(7), int64(1)).
		WillReturnRows(deudaRow(7, "tarjeta_credito", "Visa", 80000, 7, "pendiente"))
	// 2) validarCategoriaEgreso
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(1)).
		WillReturnRows(catRow(7, "Comida", "egreso"))
	// 3) FindOrCreate mes → cerrado
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(mesRows(1, periodo, "cerrado"))

	body := `{"categoria_id":7}`
	r := routeParam("POST", "/api/deudas/{id}/pagar", f.deudaH.MarcarPagada)
	req := httptest.NewRequest("POST", "/api/deudas/7/pagar", strings.NewReader(body)).
		WithContext(ctxWithUserID(1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// MesHandler tests
// ============================================================================

func TestMesHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`)).
		WithArgs(int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))

	r := routeParam("GET", "/api/meses", f.mesH.List)
	req := httptest.NewRequest("GET", "/api/meses", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMesHandler_GetByID(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))

	r := routeParam("GET", "/api/meses/{id}", f.mesH.GetByID)
	req := httptest.NewRequest("GET", "/api/meses/9", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMesHandler_GetByID_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(99), int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/meses/{id}", f.mesH.GetByID)
	req := httptest.NewRequest("GET", "/api/meses/99", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestMesHandler_Current(t *testing.T) {
	f := newHandlerFixture(t)
	periodo := time.Now().Format("2006-01")
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnRows(mesRows(9, periodo, "abierto"))

	r := routeParam("GET", "/api/meses/current", f.mesH.Current)
	req := httptest.NewRequest("GET", "/api/meses/current", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMesHandler_Cerrar(t *testing.T) {
	f := newHandlerFixture(t)

	// 1. FindByID mes abierto
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	// 2. FindByPeriodo transacciones
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(transCols).
			AddRow(1, int64(1), "ingreso", 100000.0, fixedTime(), int64(1), "Sueldo",
				"Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9), fixedTime(), fixedTime()).
			AddRow(2, int64(1), "egreso", 30000.0, fixedTime(), int64(5), "Alquiler",
				"Alquiler", "debito", false, nil, nil, "confirmado", int64(9), fixedTime(), fixedTime()))
	// 3. SumSuperavitAnterior
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 4. SumMontoTotal
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 5. Update mes → cerrado
	tasa := 70.0
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("cerrado", 100000.0, 30000.0, 70000.0, &tasa, 70000.0, 0.0, 70000.0,
			int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 6. FindOrCreate proximo mes (2026-09): not found → insert → re-read
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnError(sql.ErrNoRows)
	f.mock.ExpectExec(regexp.QuoteMeta(qMesInsert)).
		WithArgs(int64(1), "2026-09").
		WillReturnResult(sqlmock.NewResult(10, 1))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRows(10, "2026-09", "abierto"))
	// 7. SyncFijosPeriodo: FindOrCreate → already exists
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRows(10, "2026-09", "abierto"))
	// 8. FindActivos → empty
	f.mock.ExpectQuery(regexp.QuoteMeta(qCFActivos)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(cfCols))
	// 9. Update proximo mes → abierto
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0, 0.0, 0.0,
			int64(10), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("POST", "/api/meses/{id}/cerrar", f.mesH.Cerrar)
	req := httptest.NewRequest("POST", "/api/meses/9/cerrar", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMesHandler_Cerrar_AlreadyClosed(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "cerrado"))

	r := routeParam("POST", "/api/meses/{id}/cerrar", f.mesH.Cerrar)
	req := httptest.NewRequest("POST", "/api/meses/9/cerrar", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// The service returns a generic error (not model.ErrMesCerrado), which maps to 500.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestMesHandler_Recalcular(t *testing.T) {
	f := newHandlerFixture(t)

	// 1. FindByID mes abierto
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	// 2. FindByPeriodo transacciones
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(1), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(transCols).
			AddRow(1, int64(1), "ingreso", 100000.0, fixedTime(), int64(1), "Sueldo",
				"Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9), fixedTime(), fixedTime()))
	// 3. SumSuperavitAnterior
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 4. SumMontoTotal
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumMontoTotal)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 5. Update mes
	tasa := 100.0
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("abierto", 100000.0, 0.0, 100000.0, &tasa, 100000.0, 0.0, 100000.0,
			int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	r := routeParam("POST", "/api/meses/{id}/recalcular", f.mesH.Recalcular)
	req := httptest.NewRequest("POST", "/api/meses/9/recalcular", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMesHandler_Recalcular_MesCerrado(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(1)).
		WillReturnRows(mesRows(9, "2026-08", "cerrado"))

	r := routeParam("POST", "/api/meses/{id}/recalcular", f.mesH.Recalcular)
	req := httptest.NewRequest("POST", "/api/meses/9/recalcular", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// CategoriaHandler tests
// ============================================================================

func TestCategoriaHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows(catCols).
			AddRow(1, "Sueldo", "ingreso", "💰", false, nil, fixedTime()).
			AddRow(5, "Alquiler", "egreso", "🏠", false, nil, fixedTime()))

	r := routeParam("GET", "/api/categorias", f.catH.List)
	req := httptest.NewRequest("GET", "/api/categorias", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sueldo") {
		t.Fatal("expected body to contain Sueldo")
	}
}

// ============================================================================
// DashboardHandler tests
// ============================================================================

func TestDashboardHandler_GetDashboard(t *testing.T) {
	f := newHandlerFixture(t)
	periodoActual := time.Now().Format("2006-01")
	periodoAnterior := time.Now().AddDate(0, -1, 0).Format("2006-01")
	hasta := time.Now()
	desde := hasta.AddDate(0, 0, -9)

	// 1. FindOrCreate mes actual → exists, abierto
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnRows(mesRows(9, periodoActual, "abierto"))
	// 2. FindByPeriodo transacciones del mes actual
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(1), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows(transCols).
			AddRow(1, int64(1), "ingreso", 100000.0, fixedTime(), int64(1), "Sueldo",
				"Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9), fixedTime(), fixedTime()))
	// 3. FindByPeriodo mes anterior → not found
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodoAnterior).
		WillReturnError(model.ErrNotFound)
	// 4. FindAll categorias
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(1)).
		WillReturnRows(catRow(1, "Sueldo", "ingreso"))
	// 5. FindByRango transacciones últimos días
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(1), desde.Format("2006-01-02"), hasta.Format("2006-01-02")).
		WillReturnRows(sqlmock.NewRows(transCols).
			AddRow(1, int64(1), "ingreso", 100000.0, fixedTime(), int64(1), "Sueldo",
				"Sueldo", "transferencia", false, nil, nil, "confirmado", int64(9), fixedTime(), fixedTime()))
	// 6. FindByRango deudas últimos días
	f.mock.ExpectQuery(regexp.QuoteMeta(qDeudaFindByRango)).
		WithArgs(int64(1), desde.Format("2006-01-02"), hasta.Format("2006-01-02")).
		WillReturnRows(sqlmock.NewRows(deudaCols))

	rec := httptest.NewRecorder()
	f.dashH.GetDashboard(rec,
		httptest.NewRequest("GET", "/api/dashboard", nil).WithContext(ctxWithUserID(1)))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["mes_actual"] == nil {
		t.Fatal("expected mes_actual to be non-nil")
	}
}

func TestDashboardHandler_GetDashboard_Error(t *testing.T) {
	f := newHandlerFixture(t)
	periodoActual := time.Now().Format("2006-01")

	// First query fails with a DB error
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodoActual).
		WillReturnError(errors.New("db caido"))

	rec := httptest.NewRecorder()
	f.dashH.GetDashboard(rec,
		httptest.NewRequest("GET", "/api/dashboard", nil).WithContext(ctxWithUserID(1)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
