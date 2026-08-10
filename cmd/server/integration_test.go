package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"finanzas_personales/internal/config"

	"github.com/go-sql-driver/mysql"
)

// adminDB connects to MySQL without selecting a database so each test can
// create (and later drop) its own isolated schema.
func adminDB(t *testing.T) (*sql.DB, *mysql.Config, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "root:@tcp(127.0.0.1:3306)/finanzas?parseTime=true&multiStatements=true&charset=utf8mb4&loc=Local"
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	baseName := cfg.DBName
	cfg.DBName = ""
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("MySQL no disponible, se omiten tests de integración: %v", err)
	}
	return db, cfg, baseName
}

type testEnv struct {
	router http.Handler
	cfg    *config.Config
	db     *sql.DB
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	admin, cfg, _ := adminDB(t)
	name := fmt.Sprintf("finanzas_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	applyMigrations(t, cfg.FormatDSN(), -1)
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name)
		admin.Close()
	})

	appCfg := &config.Config{
		Port:          "0",
		DatabaseURL:   cfg.FormatDSN(),
		JWTSecret:     "test-secret",
		JWTExpiration: 72 * time.Hour,
		CORSOrigin:    "*",
		LogLevel:      "info",
	}
	return &testEnv{router: buildRouter(appCfg, db), cfg: appCfg, db: db}
}

func doReq(t *testing.T, h http.Handler, method, path string, token string, body string, contentType string, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func registerJSON(t *testing.T, env *testEnv, email string) (token string, body map[string]interface{}) {
	t.Helper()
	payload := fmt.Sprintf(`{"nombre":"Test","email":%q,"password":"secreto123","moneda_default":"ARS"}`, email)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "", payload, "application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: expected 201, got %d (%s)", email, rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	tok, _ := body["token"].(string)
	return tok, body
}

func TestHealth(t *testing.T) {
	env := newTestEnv(t)
	rec := doReq(t, env.router, http.MethodGet, "/health", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Errorf("unexpected health body: %s", rec.Body.String())
	}
}

func TestRootRedirect(t *testing.T) {
	env := newTestEnv(t)
	rec := doReq(t, env.router, http.MethodGet, "/", "", "", "", false)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login, got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAuthFlow_RegisterLogin(t *testing.T) {
	env := newTestEnv(t)

	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		`{"nombre":"Pepe","email":"pepe@test.com","password":"secreto123","moneda_default":"ARS"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", rec.Code)
	}
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("expected token")
	}

	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"pepe@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", rec.Code)
	}
}

func TestAuth_RegisterForm_SetsMoneda(t *testing.T) {
	env := newTestEnv(t)
	// Regression: the register screen must persist the selected currency.
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		"nombre=Pepe&email=moneda@test.com&password=secreto123&moneda_default=USD",
		"application/x-www-form-urlencoded", false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 for form register, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/dashboard/page" {
		t.Errorf("unexpected redirect: %q", rec.Header().Get("Location"))
	}
	var cookies []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "token" {
			cookies = append(cookies, c)
		}
	}
	if len(cookies) == 0 {
		t.Error("expected token cookie")
	}

	// The selected currency must actually be persisted (regression: the input
	// used to be ignored and always stored ARS).
	var moneda string
	if err := env.db.QueryRow("SELECT moneda_default FROM usuarios WHERE email = ?", "moneda@test.com").Scan(&moneda); err != nil {
		t.Fatalf("query moneda_default: %v", err)
	}
	if moneda != "USD" {
		t.Errorf("expected stored moneda USD, got %q", moneda)
	}
}

func TestAuth_RegisterDuplicateEmail(t *testing.T) {
	env := newTestEnv(t)
	registerJSON(t, env, "dup@test.com")
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		`{"nombre":"Otro","email":"dup@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)
	registerJSON(t, env, "login@test.com")
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"login@test.com","password":"incorrecta"}`,
		"application/json", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestProtectedRoutes_RequireToken(t *testing.T) {
	env := newTestEnv(t)
	paths := []string{
		"/api/transacciones",
		"/api/costos-fijos",
		"/api/meses",
		"/api/deudas",
		"/api/dashboard",
		"/api/categorias",
		"/api/dashboard/page",
		"/api/transacciones/page",
		"/api/costos-fijos/page",
		"/api/deudas/page",
		"/api/balance/page",
	}
	for _, p := range paths {
		rec := doReq(t, env.router, http.MethodGet, p, "", "", "", false)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", p, rec.Code)
		}
	}
}

func TestTransacciones_CRUD(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "trans@test.com")

	created := createTransaction(t, env, token, "ingreso", 100000, "2026-08-10")
	id := int64(created["id"].(float64))
	if id <= 0 {
		t.Fatalf("invalid transaction id: %v", created["id"])
	}

	rec := doReq(t, env.router, http.MethodGet, "/api/transacciones", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(list))
	}

	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/transacciones/%d", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodPut, fmt.Sprintf("/api/transacciones/%d", id), token,
		`{"tipo":"ingreso","monto":120000,"fecha":"2026-08-10","categoria_id":1,"descripcion":"sueldo corregido","medio_pago":"transferencia"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// invalid update: monto <= 0 must be rejected
	rec = doReq(t, env.router, http.MethodPut, fmt.Sprintf("/api/transacciones/%d", id), token,
		`{"tipo":"ingreso","monto":0,"fecha":"2026-08-10","categoria_id":1}`,
		"application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid update: expected 400, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodDelete, fmt.Sprintf("/api/transacciones/%d", id), token, "", "", false)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}
}

func TestTransacciones_InvalidCreate(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "invalid@test.com")
	rec := doReq(t, env.router, http.MethodPost, "/api/transacciones", token,
		`{"tipo":"ingreso","monto":0,"fecha":"2026-08-10","categoria_id":1}`,
		"application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for monto=0, got %d", rec.Code)
	}
	rec = doReq(t, env.router, http.MethodPost, "/api/transacciones", token,
		`{"tipo":"inversion","monto":100,"fecha":"2026-08-10","categoria_id":1}`,
		"application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid tipo, got %d", rec.Code)
	}
}

func TestTransacciones_HTMXCreateRedirects(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "htmx@test.com")
	rec := doReq(t, env.router, http.MethodPost, "/api/transacciones", token,
		"tipo=ingreso&monto=1000&fecha=2026-08-01&categoria_id=1&descripcion=test&medio_pago=efectivo",
		"application/x-www-form-urlencoded", true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "/api/transacciones/page" {
		t.Errorf("unexpected HX-Redirect: %q", rec.Header().Get("HX-Redirect"))
	}
}

func TestCostosFijos_CRUD(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "cf@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/costos-fijos", token,
		`{"categoria_id":6,"descripcion":"Internet","monto_estimado":12000,"dia_vencimiento":5,"tipo_periodo":"mensual"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var cf map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	id := int64(cf["id"].(float64))

	rec = doReq(t, env.router, http.MethodPatch, fmt.Sprintf("/api/costos-fijos/%d/toggle", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	if cf["activo"].(bool) != false {
		t.Error("expected activo=false after toggle")
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/costos-fijos", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodDelete, fmt.Sprintf("/api/costos-fijos/%d", id), token, "", "", false)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}
}

func TestDeudas_CRUD(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "deuda@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco Galicia","descripcion":"Auto","monto_total":500000,"proximo_vencimiento":"2026-09-10"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))

	rec = doReq(t, env.router, http.MethodGet, "/api/deudas", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var list []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 deuda, got %d", len(list))
	}

	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/deudas/%d", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rec.Code)
	}

	// invalid create: monto total no puede ser <= 0.
	rec = doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco","monto_total":0}`,
		"application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid create: expected 400, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodPut, fmt.Sprintf("/api/deudas/%d", id), token,
		`{"tipo":"prestamo","entidad":"Banco Galicia","descripcion":"Auto","monto_total":450000,"proximo_vencimiento":"2026-09-10"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	if deuda["monto_total"].(float64) != 450000 {
		t.Errorf("update not applied: %v", deuda["monto_total"])
	}

	rec = doReq(t, env.router, http.MethodDelete, fmt.Sprintf("/api/deudas/%d", id), token, "", "", false)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", rec.Code)
	}
}

func TestDeudas_CrossUserIsolation(t *testing.T) {
	env := newTestEnv(t)
	tokenA, _ := registerJSON(t, env, "deudaA@test.com")
	tokenB, _ := registerJSON(t, env, "deudaB@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", tokenA,
		`{"tipo":"prestamo","entidad":"Banco","monto_total":100000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rec.Code)
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))

	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/deudas/%d", id), tokenB, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user get: expected 404, got %d", rec.Code)
	}
	rec = doReq(t, env.router, http.MethodDelete, fmt.Sprintf("/api/deudas/%d", id), tokenB, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user delete: expected 404, got %d", rec.Code)
	}
}

func TestDeudas_FormCreate_HTMXAndPlainRedirects(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "deudaform@test.com")

	// HTMX: la página debe recargarse vía HX-Redirect.
	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		"tipo=tarjeta_credito&entidad=Visa&descripcion=Tarjeta&monto_total=80000",
		"application/x-www-form-urlencoded", true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("htmx create: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "/api/deudas/page" {
		t.Errorf("unexpected HX-Redirect: %q", rec.Header().Get("HX-Redirect"))
	}

	// Form plano (sin HTMX): 303 al listado.
	rec = doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		"tipo=prestamo&entidad=Banco&monto_total=100000",
		"application/x-www-form-urlencoded", false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("plain form create: expected 303, got %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/deudas/page" {
		t.Errorf("unexpected Location: %q", rec.Header().Get("Location"))
	}
}

func TestDeudas_PageConDatos(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "deudapage@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco Galicia","descripcion":"Auto","monto_total":500000,"proximo_vencimiento":"2026-09-10"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/deudas/page", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("deudas page: expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Banco Galicia") || !strings.Contains(body, "$ 500000.00") {
		t.Errorf("deuda no visible en la página: %s", body)
	}
	if strings.Contains(body, "No hay deudas registradas") {
		t.Error("deudas page should not show empty state with a deuda present")
	}
}

func TestBalance_AhorroAcumuladoYPasivos(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "bal@test.com")
	dia := time.Now().Format("2006-01-02")

	createTransaction(t, env, token, "ingreso", 100000, dia)
	createTransaction(t, env, token, "egreso", 40000, dia)

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"tarjeta_credito","entidad":"Visa","monto_total":80000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/balance/page", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("balance page: expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Ahorro acumulado = superávit histórico (0) + superávit actual (60000).
	if !strings.Contains(body, "Ahorro Acumulado") || !strings.Contains(body, "$ 60000.00") {
		t.Errorf("ahorro acumulado no visible en balance: %s", body)
	}
	// Pasivos = suma de montos totales de deudas (80000), visibles con su desglose.
	if !strings.Contains(body, "Pasivos") || !strings.Contains(body, "$ 80000.00") || !strings.Contains(body, "Visa") {
		t.Errorf("pasivos no visibles en balance: %s", body)
	}
	// Patrimonio = 60000 - 80000 = -20000.
	if !strings.Contains(body, "$ -20000.00") {
		t.Errorf("patrimonio neto incorrecto en balance: %s", body)
	}
}

func TestMeses_CierreYCostoFijoPrecargado(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "mes@test.com")

	periodoActual := time.Now().Format("2006-01")
	dia := time.Now().Format("2006-01-02")

	createTransaction(t, env, token, "ingreso", 100000, dia)
	createTransaction(t, env, token, "egreso", 40000, dia)
	createCostoFijo(t, env, token)

	rec := doReq(t, env.router, http.MethodGet, "/api/meses/current", token, "", "", false)
	var mes map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	mesID := int64(mes["id"].(float64))

	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/cerrar", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cerrar: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	if mes["estado"] != "cerrado" {
		t.Errorf("expected cerrado, got %v", mes["estado"])
	}
	// The active fixed cost (5000) is materialized into the current month on
	// creation, so it counts in the close totals too.
	if mes["ingresos_total"].(float64) != 100000 || mes["egresos_total"].(float64) != 45000 {
		t.Errorf("unexpected totals: %v / %v", mes["ingresos_total"], mes["egresos_total"])
	}
	if mes["superavit"].(float64) != 55000 {
		t.Errorf("expected superavit 55000, got %v", mes["superavit"])
	}

	// Closed month is immutable: a new transaction must be rejected.
	rec = doReq(t, env.router, http.MethodPost, "/api/transacciones", token,
		fmt.Sprintf(`{"tipo":"egreso","monto":100,"fecha":%q,"categoria_id":5,"descripcion":"no deberia poder"}`, dia),
		"application/json", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("create in closed month: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	// The active fixed cost must be precarged into the next period as pendiente.
	proximoPeriodo := time.Now().AddDate(0, 1, 0).Format("2006-01")
	rec = doReq(t, env.router, http.MethodGet, "/api/transacciones?periodo="+proximoPeriodo, token, "", "", false)
	var precargadas []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &precargadas)
	found := false
	for _, tr := range precargadas {
		if tr["es_fijo"].(bool) == true && tr["estado"] == "pendiente" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected precarged fixed cost in period %s, got %v", proximoPeriodo, precargadas)
	}

	// Sanity: the period actually closed.
	if periodoActual != time.Now().Format("2006-01") {
		t.Fatal("test crossed month boundary")
	}
}

func TestMeses_CierreConDeudasYRecalcularCerrado(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "cierred@test.com")
	periodoActual := time.Now().Format("2006-01")
	dia := time.Now().Format("2006-01-02")

	createTransaction(t, env, token, "ingreso", 100000, dia)
	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco","monto_total":50000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/meses/current", token, "", "", false)
	var mes map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	mesID := int64(mes["id"].(float64))

	// Cerrar con deudas: pasivos quedan congelados en el mes cerrado.
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/cerrar", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cerrar: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	if mes["estado"] != "cerrado" {
		t.Errorf("expected cerrado, got %v", mes["estado"])
	}
	if mes["superavit"].(float64) != 100000 || mes["ahorro_acumulado"].(float64) != 100000 {
		t.Errorf("unexpected superavit/ahorro: %v / %v", mes["superavit"], mes["ahorro_acumulado"])
	}
	if mes["pasivos_total"].(float64) != 50000 || mes["patrimonio"].(float64) != 50000 {
		t.Errorf("unexpected pasivos/patrimonio: %v / %v", mes["pasivos_total"], mes["patrimonio"])
	}

	// Un mes cerrado no se puede recalcular (regresión del nuevo flujo).
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/recalcular", mesID), token, "", "", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("recalcular closed month: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Los acumulados persisten inmutables en el mes cerrado.
	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/meses/%d", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get closed month: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	if mes["ahorro_acumulado"].(float64) != 100000 || mes["pasivos_total"].(float64) != 50000 {
		t.Errorf("closed month persisted wrong acumulados: ahorro=%v pasivos=%v", mes["ahorro_acumulado"], mes["pasivos_total"])
	}

	if periodoActual != time.Now().Format("2006-01") {
		t.Fatal("test crossed month boundary")
	}
}

func TestDashboard_Totals(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "dash@test.com")

	dia := time.Now().Format("2006-01-02")
	createTransaction(t, env, token, "ingreso", 100000, dia)
	createTransaction(t, env, token, "egreso", 25000, dia)

	rec := doReq(t, env.router, http.MethodGet, "/api/dashboard", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	mesActual, _ := dash["mes_actual"].(map[string]interface{})
	if mesActual == nil {
		t.Fatal("mes_actual missing")
	}
	if mesActual["ingresos_total"].(float64) != 100000 {
		t.Errorf("expected ingresos 100000, got %v", mesActual["ingresos_total"])
	}
	if mesActual["egresos_total"].(float64) != 25000 {
		t.Errorf("expected egresos 25000, got %v", mesActual["egresos_total"])
	}
	if mesActual["superavit"].(float64) != 75000 {
		t.Errorf("expected superavit 75000, got %v", mesActual["superavit"])
	}
	gastos, _ := dash["gastos_por_categoria"].([]interface{})
	if len(gastos) != 1 {
		t.Errorf("expected 1 gasto por categoria, got %d", len(gastos))
	}
}

func TestPages_Render(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "pages@test.com")
	dia := time.Now().Format("2006-01-02")
	createTransaction(t, env, token, "ingreso", 50000, dia)
	createTransaction(t, env, token, "egreso", 10000, dia)
	createCostoFijo(t, env, token)

	periodoActual := time.Now().Format("2006-01")
	tests := []struct {
		path    string
		markers []string
	}{
		{"/api/dashboard/page", []string{"Ingresos del Mes", "Egresos del Mes", "Tasa de Ahorro"}},
		{"/api/transacciones/page", []string{"Transacciones", "Todos", `value="` + periodoActual + `"`}},
		{"/api/costos-fijos/page", []string{"Costos Fijos", "Internet"}},
		{"/api/balance/page", []string{"Balance", "RESULTADO NETO", "$ 50000.00", "$ 15000.00", "$ 35000.00", "Ahorro Acumulado", "PATRIMONIO NETO"}},
		{"/api/meses/page", []string{"Meses", periodoActual}},
		{"/api/deudas/page", []string{"Deudas", "No hay deudas registradas"}},
	}
	for _, tt := range tests {
		rec := doReq(t, env.router, http.MethodGet, tt.path, token, "", "", false)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", tt.path, rec.Code)
			continue
		}
		body := rec.Body.String()
		for _, m := range tt.markers {
			if !strings.Contains(body, m) {
				t.Errorf("%s: marker %q not found", tt.path, m)
			}
		}
	}
}

func TestFormFragments_Render(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "frags@test.com")

	created := createTransaction(t, env, token, "egreso", 45000, "2026-08-01")
	tID := int64(created["id"].(float64))
	deudaRec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco Galicia","descripcion":"Auto","monto_total":500000,"proximo_vencimiento":"2026-09-10"}`,
		"application/json", false)
	var deuda map[string]interface{}
	json.Unmarshal(deudaRec.Body.Bytes(), &deuda)
	dID := int64(deuda["id"].(float64))

	type tc struct {
		path    string
		want    []string
		missing []string
	}
	tests := []tc{
		{"/api/transacciones/form", []string{"Nueva Transacción", `hx-post="/api/transacciones"`}, []string{`hx-put`}},
		{fmt.Sprintf("/api/transacciones/form?edit_id=%d", tID), []string{"Editar Transacción", fmt.Sprintf(`hx-put="/api/transacciones/%d"`, tID), `value="test egreso"`}, []string{`hx-post`}},
		{"/api/deudas/form", []string{"Nueva Deuda", `hx-post="/api/deudas"`}, []string{`hx-put`}},
		{fmt.Sprintf("/api/deudas/form?edit_id=%d", dID), []string{"Editar Deuda", fmt.Sprintf(`hx-put="/api/deudas/%d"`, dID), `value="Banco Galicia"`}, []string{`hx-post`}},
	}
	for _, tt := range tests {
		rec := doReq(t, env.router, http.MethodGet, tt.path, token, "", "", false)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", tt.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("%s: unexpected Content-Type %q", tt.path, ct)
		}
		body := rec.Body.String()
		for _, m := range tt.want {
			if !strings.Contains(body, m) {
				t.Errorf("%s: marker %q not found", tt.path, m)
			}
		}
		for _, m := range tt.missing {
			if strings.Contains(body, m) {
				t.Errorf("%s: unexpected marker %q found", tt.path, m)
			}
		}
	}

	// Sin editar con edit_id=0 se comporta como modo "nueva".
	rec := doReq(t, env.router, http.MethodGet, "/api/transacciones/form?edit_id=0", token, "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Nueva Transacción") {
		t.Errorf("edit_id=0 debe renderizar modo nueva, got %d", rec.Code)
	}

	// edit_id inexistente -> 404.
	rec = doReq(t, env.router, http.MethodGet, "/api/transacciones/form?edit_id=999999", token, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Errorf("edit_id inexistente: expected 404, got %d", rec.Code)
	}
}

func TestCostosFijos_GetUpdate(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "cfget@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/costos-fijos", token,
		`{"categoria_id":6,"descripcion":"Gimnasio","monto_estimado":8000,"dia_vencimiento":10,"tipo_periodo":"mensual"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var cf map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	id := int64(cf["id"].(float64))

	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/costos-fijos/%d", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get by id: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	if cf["descripcion"] != "Gimnasio" {
		t.Errorf("unexpected descripcion: %v", cf["descripcion"])
	}

	rec = doReq(t, env.router, http.MethodPut, fmt.Sprintf("/api/costos-fijos/%d", id), token,
		`{"categoria_id":6,"descripcion":"Gimnasio Premium","monto_estimado":9000,"dia_vencimiento":12,"tipo_periodo":"mensual"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	if cf["descripcion"] != "Gimnasio Premium" || cf["monto_estimado"].(float64) != 9000 {
		t.Errorf("update not applied: %v", cf["descripcion"])
	}

	// Toggle desactiva y luego reactiva el costo fijo (activo vuelve a true y
	// se re-materializa el costo en el mes actual vía syncMesActual).
	rec = doReq(t, env.router, http.MethodPatch, fmt.Sprintf("/api/costos-fijos/%d/toggle", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle off: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	if cf["activo"].(bool) != false {
		t.Error("expected activo=false after first toggle")
	}

	rec = doReq(t, env.router, http.MethodPatch, fmt.Sprintf("/api/costos-fijos/%d/toggle", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle on: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &cf)
	if cf["activo"].(bool) != true {
		t.Error("expected activo=true after reactivating")
	}

	rec = doReq(t, env.router, http.MethodPut, fmt.Sprintf("/api/costos-fijos/%d", id), token,
		`{"categoria_id":6,"descripcion":"x","monto_estimado":0,"dia_vencimiento":5,"tipo_periodo":"mensual"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("update with monto_estimado 0: expected 200 (sin validación de servicio), got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodPost, "/api/costos-fijos", token,
		`{esto-no-es-json`,
		"application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid create body: expected 400, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/costos-fijos/999999", token, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: expected 404, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodPut, "/api/costos-fijos/999999", token,
		`{"categoria_id":6,"descripcion":"x","monto_estimado":100,"dia_vencimiento":5}`,
		"application/json", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing: expected 404, got %d", rec.Code)
	}
}

func TestMeses_ListGetRecalcular(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "mes2@test.com")
	dia := time.Now().Format("2006-01-02")
	createTransaction(t, env, token, "ingreso", 100000, dia)

	rec := doReq(t, env.router, http.MethodGet, "/api/meses", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list meses: expected 200, got %d", rec.Code)
	}
	var lista []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &lista)
	if len(lista) == 0 {
		t.Fatal("expected at least the current month in the list")
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/meses/current", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("current: expected 200, got %d", rec.Code)
	}
	var mes map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	mesID := int64(mes["id"].(float64))

	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/meses/%d", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get by id: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	if mes["periodo"] != time.Now().Format("2006-01") {
		t.Errorf("unexpected periodo: %v", mes["periodo"])
	}

	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/recalcular", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("recalcular: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	if mes["ingresos_total"].(float64) != 100000 {
		t.Errorf("expected ingresos_total 100000 after recalcular, got %v", mes["ingresos_total"])
	}

	rec = doReq(t, env.router, http.MethodGet, "/api/meses/999999", token, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing month: expected 404, got %d", rec.Code)
	}

	rec = doReq(t, env.router, http.MethodPost, "/api/meses/999999/recalcular", token, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("recalcular missing month: expected 404, got %d", rec.Code)
	}
}

func TestCategorias_List(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "cat@test.com")

	rec := doReq(t, env.router, http.MethodGet, "/api/categorias", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list categorias: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sueldo") || !strings.Contains(rec.Body.String(), "Alquiler") {
		t.Errorf("expected system categories in response, got: %s", rec.Body.String())
	}
}

func TestPages_LoginRegister(t *testing.T) {
	env := newTestEnv(t)

	rec := doReq(t, env.router, http.MethodGet, "/login", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("login page: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Iniciar Sesión") {
		t.Error("login page missing marker")
	}

	rec = doReq(t, env.router, http.MethodGet, "/register", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("register page: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Crear Cuenta") {
		t.Error("register page missing marker")
	}
}

func TestTransacciones_CrossUserIsolation(t *testing.T) {
	env := newTestEnv(t)
	tokenA, _ := registerJSON(t, env, "owner@test.com")
	tokenB, _ := registerJSON(t, env, "other@test.com")

	// owner crea una transacción
	tx := createTransaction(t, env, tokenA, "ingreso", 5000, time.Now().Format("2006-01-02"))
	id := int64(tx["id"].(float64))

	// El otro usuario no puede verla ni borrarla (tenancy obligatoria).
	rec := doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/transacciones/%d", id), tokenB, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user get: expected 404, got %d", rec.Code)
	}
	rec = doReq(t, env.router, http.MethodDelete, fmt.Sprintf("/api/transacciones/%d", id), tokenB, "", "", false)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-user delete: expected 404, got %d", rec.Code)
	}
}

func TestDashboard_EgresosSoloYMuchosMovimientos(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "dash2@test.com")
	dia := time.Now().Format("2006-01-02")

	// Sin ingresos: la tasa de ahorro debe quedar vacía.
	for i := 0; i < 6; i++ {
		createTransaction(t, env, token, "egreso", 1000, dia)
	}

	rec := doReq(t, env.router, http.MethodGet, "/api/dashboard", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	mesActual, _ := dash["mes_actual"].(map[string]interface{})
	if mesActual == nil {
		t.Fatal("mes_actual missing")
	}
	if mesActual["tasa_ahorro"] != nil {
		t.Errorf("expected tasa_ahorro nil with no ingresos, got %v", mesActual["tasa_ahorro"])
	}
	ultimos, _ := dash["ultimos_movimientos"].([]interface{})
	if len(ultimos) != 5 {
		t.Errorf("expected 5 ultimos_movimientos, got %d", len(ultimos))
	}
}

func TestBalancePage_WithClosedMonthAndError(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "balance@test.com")

	rec := doReq(t, env.router, http.MethodGet, "/api/balance/999999/page", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("balance of missing month: expected 200 with error alert, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "alert-error") {
		t.Error("expected error alert on balance page for missing month")
	}
}

func TestMigrations_UpgradeDesdeV1(t *testing.T) {
	admin, cfg, _ := adminDB(t)
	name := fmt.Sprintf("finanzas_mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name)
		admin.Close()
	})

	// Simula una base existente en v1: corre solo la migración 001 desde los
	// archivos (golang-migrate registra la versión en schema_migrations).
	applyMigrations(t, cfg.FormatDSN(), 1)

	// La app ya no migra en runtime; el upgrade se hace igual que en dev:
	// correr las migraciones desde los archivos hasta la última versión.
	applyMigrations(t, cfg.FormatDSN(), -1)

	var tabla int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'deudas'", name).Scan(&tabla); err != nil {
		t.Fatalf("query deudas table: %v", err)
	}
	if tabla != 1 {
		t.Errorf("expected deudas table after upgrade from v1, found %d", tabla)
	}
	var version int64
	if err := db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query version: %v", err)
	}
	wantVersion := int64(latestMigrationVersion(t))
	if version != wantVersion {
		t.Errorf("expected schema version %d after upgrade, got %d", wantVersion, version)
	}
}

func createTransaction(t *testing.T, env *testEnv, token, tipo string, monto float64, fecha string) map[string]interface{} {
	t.Helper()
	payload := fmt.Sprintf(`{"tipo":%q,"monto":%v,"fecha":%q,"categoria_id":1,"descripcion":"test %s","medio_pago":"transferencia"}`, tipo, monto, fecha, tipo)
	rec := doReq(t, env.router, http.MethodPost, "/api/transacciones", token, payload, "application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s transaction: expected 201, got %d (%s)", tipo, rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func createCostoFijo(t *testing.T, env *testEnv, token string) {
	t.Helper()
	rec := doReq(t, env.router, http.MethodPost, "/api/costos-fijos", token,
		`{"categoria_id":6,"descripcion":"Internet","monto_estimado":5000,"dia_vencimiento":5,"tipo_periodo":"mensual"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create costo fijo: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
}
