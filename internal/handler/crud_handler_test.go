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
		"egresos_total", "superavit", "tasa_ahorro", "ahorro_acumulado", "created_at"}

	transCols = []string{"id", "usuario_id", "tipo", "monto", "fecha",
		"categoria_id", "categoria", "descripcion", "medio_pago", "estado",
		"mes_id", "created_at", "updated_at"}

	catCols = []string{"id", "nombre", "tipo", "icono", "es_personalizada",
		"usuario_id", "created_at"}
)

func fixedTime() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

func mesRows(id int64, periodo, estado string) *sqlmock.Rows {
	return sqlmock.NewRows(mesCols).
		AddRow(id, int64(1), periodo, estado, 0.0, 0.0, 0.0, nil, 0.0, fixedTime())
}

func transRow(id int64, tipo string, monto float64, mesID int64) *sqlmock.Rows {
	return sqlmock.NewRows(transCols).
		AddRow(id, int64(1), tipo, monto, fixedTime(), int64(5), "Sueldo", "desc",
			"transferencia", "confirmado", mesID, fixedTime(), fixedTime())
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
	qMesByPeriodo = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`
	qMesByID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`
	qMesInsert = `INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`
	qMesUpdate = `UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?
		 WHERE id=? AND usuario_id=?`
	qSumSuperavitAnterior = `SELECT COALESCE(SUM(superavit), 0) FROM meses
		 WHERE usuario_id = ? AND estado = 'cerrado' AND periodo < ?`

	qTransFindByUsuarioID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC
		 LIMIT ? OFFSET ?`
	qTransFindByID = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.id = ? AND t.usuario_id = ?`
	qTransFindByPeriodo = `SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`
	qTransInsert = `INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?)`
	qTransUpdate = `UPDATE transacciones SET tipo=?, monto=?, fecha=?, categoria_id=?, descripcion=?, medio_pago=?, updated_at=NOW()
		 WHERE id=? AND usuario_id=?`
	qTransDelete = `DELETE FROM transacciones WHERE id=? AND usuario_id=?`

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
			"transferencia", "confirmado", int64(9)).
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

func TestTransaccionHandler_Create_HTMX_DesdeInicio(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	f.mock.ExpectExec(regexp.QuoteMeta(qTransInsert)).
		WithArgs(int64(1), "ingreso", 1000.0, "2026-08-10", int64(1), "Sueldo",
			"transferencia", "confirmado", int64(9)).
		WillReturnResult(sqlmock.NewResult(4, 1))

	body := `{"tipo":"ingreso","monto":1000,"fecha":"2026-08-10","categoria_id":1,"descripcion":"Sueldo","medio_pago":"transferencia"}`
	req := httptest.NewRequest("POST", "/api/transacciones", strings.NewReader(body)).
		WithContext(ctxWithHTMX(1))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HX-Current-URL", "/api/dashboard/page")
	rec := httptest.NewRecorder()

	f.transH.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/api/dashboard/page" {
		t.Fatalf("expected HX-Redirect to /api/dashboard/page, got %q", got)
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
			int64(4), int64(1)).
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
// MesHandler tests
// ============================================================================

func TestMesHandler_List(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
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

func TestMesHandler_List_Error(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`)).
		WithArgs(int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/meses", f.mesH.List)
	req := httptest.NewRequest("GET", "/api/meses", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
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

func TestMesHandler_Current_Error(t *testing.T) {
	f := newHandlerFixture(t)
	periodo := time.Now().Format("2006-01")
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), periodo).
		WillReturnError(sql.ErrConnDone)

	r := routeParam("GET", "/api/meses/current", f.mesH.Current)
	req := httptest.NewRequest("GET", "/api/meses/current", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
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
				"Sueldo", "transferencia", "confirmado", int64(9), fixedTime(), fixedTime()).
			AddRow(2, int64(1), "egreso", 30000.0, fixedTime(), int64(5), "Alquiler",
				"Alquiler", "debito", "confirmado", int64(9), fixedTime(), fixedTime()))
	// 3. SumSuperavitAnterior
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 4. Update mes → cerrado
	tasa := 70.0
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("cerrado", 100000.0, 30000.0, 70000.0, &tasa, 70000.0,
			int64(9), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 5. FindOrCreate proximo mes (2026-09): not found → insert → re-read
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnError(sql.ErrNoRows)
	f.mock.ExpectExec(regexp.QuoteMeta(qMesInsert)).
		WithArgs(int64(1), "2026-09").
		WillReturnResult(sqlmock.NewResult(10, 1))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(1), "2026-09").
		WillReturnRows(mesRows(10, "2026-09", "abierto"))
	// 6. Update proximo mes → abierto
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("abierto", 0.0, 0.0, 0.0, nil, 0.0,
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
				"Sueldo", "transferencia", "confirmado", int64(9), fixedTime(), fixedTime()))
	// 3. SumSuperavitAnterior
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumSuperavitAnterior)).
		WithArgs(int64(1), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	// 4. Update mes
	tasa := 100.0
	f.mock.ExpectExec(regexp.QuoteMeta(qMesUpdate)).
		WithArgs("abierto", 100000.0, 0.0, 100000.0, &tasa, 100000.0,
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

func TestCategoriaHandler_List_Error(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(1)).
		WillReturnError(model.ErrNotFound)

	r := routeParam("GET", "/api/categorias", f.catH.List)
	req := httptest.NewRequest("GET", "/api/categorias", nil).WithContext(ctxWithUserID(1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
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
				"Sueldo", "transferencia", "confirmado", int64(9), fixedTime(), fixedTime()))
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
				"Sueldo", "transferencia", "confirmado", int64(9), fixedTime(), fixedTime()))

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
