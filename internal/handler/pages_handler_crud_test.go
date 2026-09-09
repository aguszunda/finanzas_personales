package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"optipay/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-chi/chi/v5"
)

const (
	qMesFindByUsuarioID = `SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`
	qUsuarioFindByID = `SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado
		 FROM usuarios WHERE id = ?`
)

// installTestTemplates setea un TemplateManager mínimo y devuelve un restaurador.
func installTestTemplates(t *testing.T, pages map[string]string) {
	t.Helper()
	old := tmpl
	t.Cleanup(func() { tmpl = old })
	fs := fstest.MapFS{
		"layout.html": {Data: []byte(`<html>{{template "content" .}}</html>`)},
	}
	for name, tmplBody := range pages {
		fs[name+".html"] = &fstest.MapFile{Data: []byte(tmplBody)}
	}
	tmpl = newTestTemplateManager(fs)
}

var pageTemplates = map[string]string{
	"dashboard":        `{{define "content"}}<div>dash user={{.userName}}</div>{{end}}`,
	"meses":            `{{define "content"}}<div>meses user={{.userName}}</div>{{end}}`,
	"balance":          `{{define "content"}}<div>balance user={{.userName}}</div>{{end}}`,
	"transaccion_form": `{{define "transaccion_form"}}<div>transaccion_form</div>{{end}}`,
}

func TestLoginPage(t *testing.T) {
	installTestTemplates(t, map[string]string{
		"login": `{{define "content"}}<div>login hideNav={{.hideNav}}</div>{{end}}`,
	})
	h := &PagesHandler{}
	req := httptest.NewRequest("GET", "/login", nil)
	rec := httptest.NewRecorder()
	h.LoginPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hideNav=true") {
		t.Errorf("expected hideNav=true, body: %s", rec.Body.String())
	}
}

func TestRegisterPage(t *testing.T) {
	installTestTemplates(t, map[string]string{
		"register": `{{define "content"}}<div>register hideNav={{.hideNav}}</div>{{end}}`,
	})
	h := &PagesHandler{}
	req := httptest.NewRequest("GET", "/register", nil)
	rec := httptest.NewRecorder()
	h.RegisterPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hideNav=true") {
		t.Errorf("expected hideNav=true, body: %s", rec.Body.String())
	}
}

func TestDashboardPage_Success(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"dashboard": pageTemplates["dashboard"]})
	periodoActual := time.Now().Format("2006-01")
	periodoAnterior := time.Now().AddDate(0, -1, 0).Format("2006-01")
	hasta := time.Now()
	desde := hasta.AddDate(0, 0, -9)

	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(0), periodoActual).
		WillReturnRows(mesRows(9, periodoActual, "abierto"))
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(0), periodoActual+"-01", periodoActual+"-31").
		WillReturnRows(sqlmock.NewRows(transCols).
			AddRow(1, int64(0), "ingreso", 100.0, fixedTime(), int64(1), "Sueldo",
				"Sueldo", "transferencia", "confirmado", int64(9), fixedTime(), fixedTime()))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(0), periodoAnterior).
		WillReturnError(model.ErrNotFound)
	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(0)).
		WillReturnRows(catRow(1, "Sueldo", "ingreso"))
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(0), desde.Format("2006-01-02"), hasta.Format("2006-01-02")).
		WillReturnRows(sqlmock.NewRows(transCols))
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesFindByUsuarioID)).
		WithArgs(int64(0)).
		WillReturnRows(mesRows(9, periodoActual, "abierto"))

	req := httptest.NewRequest("GET", "/dashboard", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.DashboardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dash") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestDashboardPage_Error(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"dashboard": pageTemplates["dashboard"]})
	periodoActual := time.Now().Format("2006-01")

	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByPeriodo)).
		WithArgs(int64(0), periodoActual).
		WillReturnError(errors.New("db caido"))

	req := httptest.NewRequest("GET", "/dashboard", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.DashboardPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even on error, got %d", rec.Code)
	}
}

func TestMesesPage(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"meses": pageTemplates["meses"]})

	f.mock.ExpectQuery(regexp.QuoteMeta(qMesFindByUsuarioID)).
		WithArgs(int64(0)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))

	req := httptest.NewRequest("GET", "/meses", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.MesesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "meses") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestMesesPage_Error(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"meses": `{{define "content"}}<div>meses err={{.error}}</div>{{end}}`})

	f.mock.ExpectQuery(regexp.QuoteMeta(qMesFindByUsuarioID)).
		WithArgs(int64(0)).
		WillReturnError(errors.New("boom"))

	req := httptest.NewRequest("GET", "/meses", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.MesesPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even on error, got %d", rec.Code)
	}
}

func TestBalancePage(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"balance": pageTemplates["balance"]})

	// Balance(id=9): FindByID mes
	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(0)).
		WillReturnRows(mesRows(9, "2026-08", "abierto"))
	// FindByPeriodo transacciones
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByPeriodo)).
		WithArgs(int64(0), "2026-08-01", "2026-08-31").
		WillReturnRows(sqlmock.NewRows(transCols))
	// calcularAcumulados: SumSuperavitAnterior
	f.mock.ExpectQuery(regexp.QuoteMeta(qSumSuperavitAnterior)).
		WithArgs(int64(0), "2026-08").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))

	r := chi.NewRouter()
	r.Get("/balance/{id}", f.pagesH.BalancePage)
	req := httptest.NewRequest("GET", "/balance/9", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "balance") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestBalancePage_BalanceError(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"balance": `{{define "content"}}<div>balance err={{.error}}</div>{{end}}`})

	f.mock.ExpectQuery(regexp.QuoteMeta(qMesByID)).
		WithArgs(int64(9), int64(0)).
		WillReturnError(errors.New("boom"))

	r := chi.NewRouter()
	r.Get("/balance/{id}", f.pagesH.BalancePage)
	req := httptest.NewRequest("GET", "/balance/9", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even on error, got %d", rec.Code)
	}
}

func TestTransaccionForm_NoEdit(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"transaccion_form": pageTemplates["transaccion_form"]})

	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(0)).
		WillReturnRows(catRow(1, "Sueldo", "ingreso"))

	req := httptest.NewRequest("GET", "/transacciones/form", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.TransaccionForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "transaccion_form") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestTransaccionForm_Edit(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"transaccion_form": pageTemplates["transaccion_form"]})

	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(0)).
		WillReturnRows(catRow(1, "Sueldo", "ingreso"))
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(4), int64(0)).
		WillReturnRows(transRow(4, "ingreso", 1000, 9))

	req := httptest.NewRequest("GET", "/transacciones/form?edit_id=4", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.TransaccionForm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTransaccionForm_Edit_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	installTestTemplates(t, map[string]string{"transaccion_form": pageTemplates["transaccion_form"]})

	f.mock.ExpectQuery(regexp.QuoteMeta(qCatFindAll)).
		WithArgs(int64(0)).
		WillReturnRows(catRow(1, "Sueldo", "ingreso"))
	f.mock.ExpectQuery(regexp.QuoteMeta(qTransFindByID)).
		WithArgs(int64(99), int64(0)).
		WillReturnError(model.ErrNotFound)

	req := httptest.NewRequest("GET", "/transacciones/form?edit_id=99", nil).WithContext(ctxWithUserID(0))
	rec := httptest.NewRecorder()
	f.pagesH.TransaccionForm(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUserName_CeroDevuelveVacio(t *testing.T) {
	f := newHandlerFixture(t)
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctxWithUserID(0))
	if got := f.pagesH.userName(req); got != "" {
		t.Errorf("userName() with uid=0 = %q, want empty", got)
	}
}

func TestUserName_DevuelveNombre(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qUsuarioFindByID)).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(int64(7), "Maria", "m@x.com", "hash", "ARS", fixedTime(), true))

	req := httptest.NewRequest("GET", "/", nil).WithContext(ctxWithUserID(7))
	if got := f.pagesH.userName(req); got != "Maria" {
		t.Errorf("userName() = %q, want Maria", got)
	}
}

func TestUserName_ErrorDevuelveVacio(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(qUsuarioFindByID)).
		WithArgs(int64(7)).
		WillReturnError(sql.ErrNoRows)

	req := httptest.NewRequest("GET", "/", nil).WithContext(ctxWithUserID(7))
	if got := f.pagesH.userName(req); got != "" {
		t.Errorf("userName() on error = %q, want empty", got)
	}
}
