package main

import (
	"context"
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

	"optipay/internal/config"
	"optipay/internal/service"

	"github.com/go-sql-driver/mysql"
)

// fakeMailer captura los enlaces de verificación que la app "envía", para que
// los tests puedan completar el flujo real de confirmación de email.
type fakeMailer struct {
	links []string
}

func (f *fakeMailer) SendVerificacion(_ context.Context, _, _, link string) error {
	f.links = append(f.links, link)
	return nil
}

func (f *fakeMailer) SendPasswordReset(_ context.Context, _, _, link string) error {
	f.links = append(f.links, link)
	return nil
}

func (f *fakeMailer) lastLink() string {
	if len(f.links) == 0 {
		return ""
	}
	return f.links[len(f.links)-1]
}

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
	mailer *fakeMailer
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

	// Inyecta un mailer falso que captura los links de verificación.
	fm := &fakeMailer{}
	prevFactory := mailerFactory
	mailerFactory = func(*config.Config) service.Mailer { return fm }
	t.Cleanup(func() { mailerFactory = prevFactory })

	appCfg := &config.Config{
		Port:          "0",
		DatabaseURL:   cfg.FormatDSN(),
		JWTSecret:     "test-secret",
		JWTExpiration: 72 * time.Hour,
		CORSOrigin:    "*",
		LogLevel:      "info",
		AppBaseURL:    "http://localhost:8080",
	}
	return &testEnv{router: buildRouter(appCfg, db), cfg: appCfg, db: db, mailer: fm}
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

// verificationTokenFromLink extrae el parámetro token del link capturado.
func verificationTokenFromLink(link string) string {
	i := strings.Index(link, "token=")
	if i < 0 {
		return ""
	}
	return link[i+len("token="):]
}

// verificarEmail consume el último enlace de verificación capturado por el
// mailer falso y afirma que la página de resultado muestra el éxito.
func verificarEmail(t *testing.T, env *testEnv) {
	t.Helper()
	link := env.mailer.lastLink()
	if link == "" {
		t.Fatal("no se capturó ningún link de verificación")
	}
	rec := doReq(t, env.router, http.MethodGet,
		"/api/auth/verificar?token="+verificationTokenFromLink(link), "", "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Email confirmado") {
		t.Fatalf("verify: expected 200 con éxito, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// registerJSON registra al usuario, confirma su email con el enlace capturado
// del mailer falso e inicia sesión. Devuelve el JWT de sesión.
func registerJSON(t *testing.T, env *testEnv, email string) (token string, body map[string]interface{}) {
	t.Helper()
	payload := fmt.Sprintf(`{"nombre":"Test","email":%q,"password":"secreto123","moneda_default":"ARS"}`, email)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "", payload, "application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: expected 201, got %d (%s)", email, rec.Code, rec.Body.String())
	}
	verificarEmail(t, env)
	return loginJSON(t, env, email, "secreto123")
}

func loginJSON(t *testing.T, env *testEnv, email, password string) (string, map[string]interface{}) {
	t.Helper()
	payload := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/login", "", payload, "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: expected 200, got %d (%s)", email, rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
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

	// El alta responde 201 pero NO emite sesión ni cookie.
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		`{"nombre":"Pepe","email":"pepe@test.com","password":"secreto123","moneda_default":"ARS"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", rec.Code)
	}
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if tok, _ := body["token"].(string); tok != "" {
		t.Error("register must not issue a session token")
	}
	if msg, _ := body["mensaje"].(string); msg == "" {
		t.Error("expected a pending-verification message")
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "token" && c.Value != "" {
			t.Error("register must not set the auth cookie")
		}
	}

	// Sin verificar, el login está bloqueado con 403.
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"pepe@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login unverified: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Tras confirmar el email del enlace capturado, el login funciona.
	verificarEmail(t, env)
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"pepe@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("login verified: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

// El registro vía HTMX devuelve el fragmento del pop-up (no una página
// completa) con el email precargado en el form de reenvío.
func TestAuth_Register_HTMX_PopUpReenvio(t *testing.T) {
	env := newTestEnv(t)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		`{"nombre":"Test","email":"pop@test.com","password":"secreto123"}`,
		"application/json", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 fragment, got %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="auth-panel"`,
		"modal-overlay",
		"Cuenta creada",
		"pop@test.com",
		"/api/auth/reenviar-verificacion",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment missing %q", want)
		}
	}
	if strings.Contains(body, "<!DOCTYPE") || strings.Contains(body, "Iniciar Sesión") {
		t.Error("HTMX response must be the fragment, not the full page")
	}

	// Reenviar desde el pop-up (HTMX) reemplaza #auth-panel con la confirmación.
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/reenviar-verificacion", "",
		`{"email":"pop@test.com"}`, "application/json", true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "te enviamos un nuevo enlace") {
		t.Fatalf("htmx resend: expected 200 confirmación, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `id="auth-panel"`) {
		t.Error("resend response must replace #auth-panel")
	}
}

// El login de una cuenta sin verificar muestra el pop-up de confirmación con
// el form de reenvío precargado: HTMX recibe el fragmento, un form plano la
// página completa y la API JSON conserva el 403. Reenviar desde el pop-up,
// verificar con el nuevo enlace y loguearse cierra el flujo.
func TestAuth_Login_NoVerificado_PopUpReenvio(t *testing.T) {
	env := newTestEnv(t)
	registrarSinVerificar(t, env, "pendiente@test.com")

	// HTMX: fragmento con el pop-up y el email intentado precargado.
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"pendiente@test.com","password":"secreto123"}`,
		"application/json", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("htmx login unverified: expected 200 pop-up, got %d (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="auth-panel"`,
		"modal-overlay",
		"Falta confirmar tu email",
		"pendiente@test.com",
		"/api/auth/reenviar-verificacion",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("pop-up missing %q", want)
		}
	}
	if strings.Contains(body, "<!DOCTYPE") || strings.Contains(body, "Iniciar Sesión") {
		t.Error("HTMX response must be the fragment, not the full page")
	}

	// Form plano: página completa del login con el pop-up embebido.
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		"email=pendiente@test.com&password=secreto123",
		"application/x-www-form-urlencoded", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Falta confirmar tu email") {
		t.Fatalf("form login unverified: expected 200 con pop-up, got %d (%s)", rec.Code, rec.Body.String())
	}

	// API JSON: contrato previo intacto (403).
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"pendiente@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "confirmá tu email") {
		t.Fatalf("json login unverified: expected 403, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Reenviar desde el pop-up regenera el enlace; verificar y loguear funciona.
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/reenviar-verificacion", "",
		`{"email":"pendiente@test.com"}`, "application/json", true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "te enviamos un nuevo enlace") {
		t.Fatalf("resend from pop-up: expected 200 confirmación, got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(env.mailer.links) < 2 {
		t.Fatalf("resend must deliver a second email, got %d links", len(env.mailer.links))
	}
	verificarEmail(t, env)
	loginJSON(t, env, "pendiente@test.com", "secreto123")
}

func TestAuth_Verificar_TokenInvalidoYExpirado(t *testing.T) {
	env := newTestEnv(t)

	// Token inventado: página de resultado con estado inválido.
	rec := doReq(t, env.router, http.MethodGet, "/api/auth/verificar?token=no-existe", "", "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no es válido o ya fue usado") {
		t.Fatalf("invalid token: expected 200 con estado inválido, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "reenviar-verificacion") {
		t.Error("el estado inválido debe ofrecer reenviar el enlace")
	}

	// Token real pero vencido: se fuerza la expiración en la base.
	registrarSinVerificar(t, env, "vencido@test.com")
	link := env.mailer.lastLink()
	if _, err := env.db.Exec(
		"UPDATE usuarios SET token_expiracion = DATE_SUB(NOW(), INTERVAL 1 HOUR) WHERE email = ?",
		"vencido@test.com"); err != nil {
		t.Fatalf("force token expiry: %v", err)
	}
	rec = doReq(t, env.router, http.MethodGet,
		"/api/auth/verificar?token="+verificationTokenFromLink(link), "", "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "está expirado") {
		t.Fatalf("expired token: expected 200 con estado expirado, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestAuth_Reenviar_NuevoEnlaceInvalidaAnterior(t *testing.T) {
	env := newTestEnv(t)
	viejoLink := registrarSinVerificar(t, env, "reenvio@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/auth/reenviar-verificacion", "",
		`{"email":"reenvio@test.com"}`, "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("resend: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	nuevoLink := env.mailer.lastLink()
	if nuevoLink == viejoLink {
		t.Fatal("resend must generate a fresh link")
	}

	// El enlace viejo ya no sirve; el nuevo verifica y habilita el login.
	rec = doReq(t, env.router, http.MethodGet,
		"/api/auth/verificar?token="+verificationTokenFromLink(viejoLink), "", "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no es válido o ya fue usado") {
		t.Fatalf("old link after resend: expected estado inválido, got %d (%s)", rec.Code, rec.Body.String())
	}
	rec = doReq(t, env.router, http.MethodGet,
		"/api/auth/verificar?token="+verificationTokenFromLink(nuevoLink), "", "", "", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Email confirmado") {
		t.Fatalf("new link: expected éxito, got %d (%s)", rec.Code, rec.Body.String())
	}
	loginJSON(t, env, "reenvio@test.com", "secreto123")
}

// El form de login debe swappear #auth-panel: si el template vuelve a
// hx-swap="none", el pop-up de cuenta sin verificar viaja al navegador pero
// htmx lo descarta y el botón "Ingresar" parece muerto.
func TestAuth_LoginPage_FormSwapeaAuthPanel(t *testing.T) {
	env := newTestEnv(t)
	rec := doReq(t, env.router, http.MethodGet, "/login", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /login: expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-post="/api/auth/login" hx-target="#auth-panel" hx-swap="outerHTML"`) {
		t.Error("el form de login debe swappear #auth-panel (hx-target + outerHTML)")
	}
	if strings.Contains(body, `hx-post="/api/auth/login" hx-swap="none"`) {
		t.Error(`login con hx-swap="none": el pop-up de verificación no se mostraría`)
	}
}

func TestAuth_Reenviar_RespuestaGenericaAntiEnumeracion(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name     string
		email    string
		password string
	}{
		{"email inexistente", "nadie@test.com", ""},
		{"usuario ya verificado", "ya-verificado@test.com", "secreto123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.password != "" {
				doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
					fmt.Sprintf(`{"nombre":"Test","email":%q,"password":%q}`, tc.email, tc.password),
					"application/json", false)
				verificarEmail(t, env)
			}
			linksAntes := len(env.mailer.links)

			rec := doReq(t, env.router, http.MethodPost, "/api/auth/reenviar-verificacion", "",
				fmt.Sprintf(`{"email":%q}`, tc.email), "application/json", false)
			if rec.Code != http.StatusOK {
				t.Fatalf("resend: expected generic 200, got %d (%s)", rec.Code, rec.Body.String())
			}
			if len(env.mailer.links) != linksAntes {
				t.Errorf("must not send emails for this case")
			}
		})
	}
}

// registrarSinVerificar crea la cuenta y devuelve el link de verificación sin
// consumirlo.
func registrarSinVerificar(t *testing.T, env *testEnv, email string) string {
	t.Helper()
	before := len(env.mailer.links)
	payload := fmt.Sprintf(`{"nombre":"Test","email":%q,"password":"secreto123","moneda_default":"ARS"}`, email)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "", payload, "application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: expected 201, got %d (%s)", email, rec.Code, rec.Body.String())
	}
	if len(env.mailer.links) <= before {
		t.Fatalf("register %s did not capture a verification link", email)
	}
	return env.mailer.lastLink()
}

func TestAuth_RegisterForm_SetsMoneda(t *testing.T) {
	env := newTestEnv(t)
	// Regression: the register screen must persist the selected currency.
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/register", "",
		"nombre=Pepe&email=moneda@test.com&password=secreto123&moneda_default=USD",
		"application/x-www-form-urlencoded", false)
	// El alta responde con la propia página de registro mostrando el pop-up
	// de verificación (sin cookie, sin redirect, sin tocar el login).
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with success modal, got %d (%s)", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"Cuenta creada", "moneda@test.com", "Reenviar email de verificación"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("success modal missing %q", want)
		}
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "token" && c.Value != "" {
			t.Error("register must not set the auth cookie")
		}
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

func TestAuth_ForgotPassword_EmailExistente(t *testing.T) {
	env := newTestEnv(t)
	registerJSON(t, env, "forgot@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/auth/forgot-password", "",
		`{"email":"forgot@test.com"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(env.mailer.links) == 0 {
		t.Fatal("expected a reset email to be sent")
	}
	last := env.mailer.lastLink()
	if !strings.Contains(last, "/reset-password?token=") {
		t.Errorf("reset link malformed: %q", last)
	}
}

func TestAuth_ForgotPassword_EmailInexistente(t *testing.T) {
	env := newTestEnv(t)

	rec := doReq(t, env.router, http.MethodPost, "/api/auth/forgot-password", "",
		`{"email":"noexiste@test.com"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("anti-enumeration: expected 200, got %d", rec.Code)
	}
	if len(env.mailer.links) != 0 {
		t.Error("must not send emails for unknown addresses")
	}
}

func TestAuth_ResetPassword_FlujoCompleto(t *testing.T) {
	env := newTestEnv(t)
	registerJSON(t, env, "reset@test.com")

	// 1. Solicitar reseteo
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/forgot-password", "",
		`{"email":"reset@test.com"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("forgot-password: expected 200, got %d", rec.Code)
	}

	// 2. Extraer token del link capturado
	link := env.mailer.lastLink()
	rawToken := verificationTokenFromLink(link)
	if rawToken == "" {
		t.Fatal("no token captured in reset link")
	}

	// 3. Resetear contraseña
	payload := fmt.Sprintf(`{"token":%q,"password":"nuevaclave1","password2":"nuevaclave1"}`, rawToken)
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/reset-password", "",
		payload, "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset-password: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Login con contraseña vieja debe fallar
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"reset@test.com","password":"secreto123"}`,
		"application/json", false)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("old password should fail: expected 401, got %d", rec.Code)
	}

	// 5. Login con contraseña nueva debe funcionar
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/login", "",
		`{"email":"reset@test.com","password":"nuevaclave1"}`,
		"application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("new password login: expected 200, got %d", rec.Code)
	}
}

func TestAuth_ResetPassword_TokenInvalido(t *testing.T) {
	env := newTestEnv(t)

	payload := `{"token":"token-falso","password":"nuevaclave1","password2":"nuevaclave1"}`
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/reset-password", "",
		payload, "application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid token, got %d", rec.Code)
	}
}

func TestAuth_ResetPassword_TokenReutilizado(t *testing.T) {
	env := newTestEnv(t)
	registerJSON(t, env, "reuse@test.com")

	// Solicitar reseteo
	doReq(t, env.router, http.MethodPost, "/api/auth/forgot-password", "",
		`{"email":"reuse@test.com"}`, "application/json", false)
	link := env.mailer.lastLink()
	rawToken := verificationTokenFromLink(link)

	// Primer uso: éxito
	payload := fmt.Sprintf(`{"token":%q,"password":"nuevaclave1","password2":"nuevaclave1"}`, rawToken)
	rec := doReq(t, env.router, http.MethodPost, "/api/auth/reset-password", "",
		payload, "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("first use: expected 200, got %d", rec.Code)
	}

	// Segundo uso: token ya consumido
	rec = doReq(t, env.router, http.MethodPost, "/api/auth/reset-password", "",
		payload, "application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("reuse: expected 400, got %d", rec.Code)
	}
}

func TestPages_ForgotPassword(t *testing.T) {
	env := newTestEnv(t)

	rec := doReq(t, env.router, http.MethodGet, "/forgot-password", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Recuperar contraseña") {
		t.Error("forgot-password page missing marker")
	}
}

func TestPages_ResetPassword(t *testing.T) {
	env := newTestEnv(t)

	rec := doReq(t, env.router, http.MethodGet, "/reset-password?token=abc123", "", "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc123") {
		t.Error("reset-password page should contain token")
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

func TestDeudas_MarcarPagada_generaEgreso(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "pagar@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"tarjeta_credito","entidad":"Visa","descripcion":"Tarjeta","monto_total":80000,"medio_pago":"debito"}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))
	if deuda["estado"] != "pendiente" {
		t.Fatalf("estado inicial debería ser pendiente, got %v", deuda["estado"])
	}

	// Una categoría de tipo egreso del listado.
	rec = doReq(t, env.router, http.MethodGet, "/api/categorias", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("categorias: expected 200, got %d", rec.Code)
	}
	var cats []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &cats)
	var catID int64
	for _, c := range cats {
		if c["tipo"] == "egreso" {
			catID = int64(c["id"].(float64))
			break
		}
	}
	if catID == 0 {
		t.Fatal("no hay categorías de egreso de sistema")
	}

	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		fmt.Sprintf(`{"categoria_id":%d,"medio_pago":"efectivo"}`, catID), "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("pagar deuda: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	if deuda["estado"] != "pagada" {
		t.Errorf("estado tras pagar debería ser pagada, got %v", deuda["estado"])
	}

	// El listado conserva la deuda, ahora pagada.
	rec = doReq(t, env.router, http.MethodGet, "/api/deudas", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list deudas: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"estado":"pagada"`) {
		t.Errorf("deuda pagada no figura en el listado: %s", rec.Body.String())
	}

	// El egreso apareció en el feed de últimos movimientos y la deuda ya no.
	rec = doReq(t, env.router, http.MethodGet, "/api/dashboard", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	movimientos := dash["ultimos_movimientos"].([]interface{})
	var foundEgreso, foundDeuda bool
	for _, m := range movimientos {
		mv := m.(map[string]interface{})
		if mv["origen"] == "deuda" {
			foundDeuda = true
		}
		if mv["origen"] == "transaccion" && mv["tipo"] == "egreso" && mv["monto"].(float64) == 80000 {
			foundEgreso = true
		}
	}
	if !foundEgreso {
		t.Error("el egreso por el pago de la deuda no aparece en últimos movimientos")
	}
	if foundDeuda {
		t.Error("la deuda pagada no debería aparecer en últimos movimientos")
	}
	if mes := dash["mes_actual"].(map[string]interface{}); mes["egresos_total"].(float64) != 80000 {
		t.Errorf("egresos_total debería incluir el egreso del pago: %v", mes["egresos_total"])
	}

	// El egreso registró la forma de pago elegida en la confirmación ("efectivo"),
	// no la guardada en la deuda ("debito").
	rec = doReq(t, env.router, http.MethodGet, "/api/transacciones", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("list transacciones: expected 200, got %d", rec.Code)
	}
	var txs []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &txs)
	var foundMedio bool
	for _, tx := range txs {
		if tx["tipo"] == "egreso" && strings.HasPrefix(tx["descripcion"].(string), "Pago deuda:") {
			if tx["medio_pago"] == "efectivo" {
				foundMedio = true
			} else {
				t.Errorf("egreso del pago debería tener medio_pago efectivo, got %v", tx["medio_pago"])
			}
		}
	}
	if !foundMedio {
		t.Error("no se encontró el egreso generado por el pago de la deuda")
	}

	// Pagar de nuevo -> rechazado (no duplica egreso).
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		fmt.Sprintf(`{"categoria_id":%d}`, catID), "application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("doble pago: expected 400, got %d", rec.Code)
	}
}

func TestDeudas_MarcarPagada_CategoriaInvalida(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "pagarcinvalida@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco","monto_total":10000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))

	// Categoría 1 (Sueldo) es de ingreso: no puede registrar el egreso.
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		`{"categoria_id":1}`, "application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("categoria de ingreso: expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}

	// La deuda sigue pendiente.
	rec = doReq(t, env.router, http.MethodGet, fmt.Sprintf("/api/deudas/%d", id), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("get deuda: expected 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `"estado":"pagada"`) {
		t.Error("la deuda no debería haberse marcado pagada con categoría inválida")
	}

	// Categoría inexistente -> 400.
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		`{"categoria_id":999999}`, "application/json", false)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("categoria inexistente: expected 400, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDeudas_MarcarPagada_MesCerrado(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "pagarcerrado@test.com")

	rec := doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"tarjeta_credito","entidad":"Visa","monto_total":80000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))

	// Cerramos el mes actual como hizo el usuario.
	rec = doReq(t, env.router, http.MethodGet, "/api/meses/current", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("mes current: expected 200, got %d", rec.Code)
	}
	var mes map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	mesID := int64(mes["id"].(float64))
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/cerrar", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cerrar mes: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Sin fecha (default hoy -> mes cerrado) debe devolver 409 y no marcar nada.
	rec = doReq(t, env.router, http.MethodGet, "/api/categorias", token, "", "", false)
	var cats []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &cats)
	var catID int64
	for _, c := range cats {
		if c["tipo"] == "egreso" {
			catID = int64(c["id"].(float64))
			break
		}
	}
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		`{"categoria_id":`+fmt.Sprintf("%d", catID)+`}`, "application/json", false)
	if rec.Code != http.StatusConflict {
		t.Fatalf("pagar sin fecha con mes cerrado: expected 409, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Pagar indicando el próximo mes (abierto, dejado por Cerrar) -> 200.
	proximoPeriodo := time.Now().AddDate(0, 1, 0).Format("2006-01")
	fechaPago := proximoPeriodo + "-10"
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		fmt.Sprintf(`{"categoria_id":%d,"fecha":%q}`, catID, fechaPago), "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("pagar con fecha en mes abierto: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	if deuda["estado"] != "pagada" {
		t.Errorf("estado tras pagar debería ser pagada, got %v", deuda["estado"])
	}

	// El egreso quedó en el mes abierto indicado.
	rec = doReq(t, env.router, http.MethodGet, "/api/transacciones?periodo="+proximoPeriodo, token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("transacciones del próximo periodo: expected 200, got %d", rec.Code)
	}
	var trans []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &trans)
	found := false
	for _, tr := range trans {
		if tr["tipo"] == "egreso" && tr["monto"].(float64) == 80000 && tr["fecha"] == fechaPago {
			found = true
		}
	}
	if !found {
		t.Errorf("egreso del pago no quedó en el mes %s: %v", proximoPeriodo, trans)
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

func TestDashboard_MesActualCerrado_MuestraMesAbierto(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "dashcerrado@test.com")
	periodoActual := time.Now().Format("2006-01")
	proximoPeriodo := time.Now().AddDate(0, 1, 0).Format("2006-01")

	createTransaction(t, env, token, "egreso", 10000, time.Now().Format("2006-01-02"))

	// Cerramos el mes actual.
	rec := doReq(t, env.router, http.MethodGet, "/api/meses/current", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("mes current: expected 200, got %d", rec.Code)
	}
	var mes map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &mes)
	mesID := int64(mes["id"].(float64))
	if mes["periodo"] != periodoActual {
		t.Fatalf("expected current period %s, got %v", periodoActual, mes["periodo"])
	}
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/meses/%d/cerrar", mesID), token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("cerrar mes: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// Una deuda pagada cae en el mes abierto siguiente.
	rec = doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"tarjeta_credito","entidad":"Visa","monto_total":80000}`,
		"application/json", false)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create deuda: expected 201, got %d (%s)", rec.Code, rec.Body.String())
	}
	var deuda map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &deuda)
	id := int64(deuda["id"].(float64))

	rec = doReq(t, env.router, http.MethodGet, "/api/categorias", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("categorias: expected 200, got %d", rec.Code)
	}
	var cats []map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &cats)
	var catID int64
	for _, c := range cats {
		if c["tipo"] == "egreso" {
			catID = int64(c["id"].(float64))
			break
		}
	}
	fechaPago := proximoPeriodo + "-10"
	rec = doReq(t, env.router, http.MethodPost, fmt.Sprintf("/api/deudas/%d/pagar", id), token,
		fmt.Sprintf(`{"categoria_id":%d,"fecha":%q}`, catID, fechaPago), "application/json", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("pagar deuda en mes abierto: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// El dashboard debe reflejar el mes abierto, con el egreso del pago.
	rec = doReq(t, env.router, http.MethodGet, "/api/dashboard", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	mesActual, _ := dash["mes_actual"].(map[string]interface{})
	if mesActual == nil {
		t.Fatal("mes_actual missing")
	}
	if mesActual["periodo"] != proximoPeriodo {
		t.Errorf("dashboard con mes actual cerrado debería mostrar el mes abierto %s, got %v", proximoPeriodo, mesActual["periodo"])
	}
	if mesActual["egresos_total"].(float64) != 80000 {
		t.Errorf("los egresos del mes abierto deberían incluir el pago de la deuda (80000), got %v", mesActual["egresos_total"])
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
		{"/api/dashboard/page", []string{"Balance General", "Ingresos del Mes", "Egresos del Mes", "Tasa de Ahorro", "Últimos Movimientos", "Últimos 10 días"}},
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
		{fmt.Sprintf("/api/deudas/%d/pagar/form", dID), []string{"Confirmar pago", fmt.Sprintf(`hx-post="/api/deudas/%d/pagar"`, dID), "Comida", `name="medio_pago"`}, []string{`hx-put`}},
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
	if len(ultimos) != 6 {
		t.Errorf("expected 6 ultimos_movimientos (los del día, sin límite de 5), got %d", len(ultimos))
	}
}

func TestDashboard_FeedUnificadoConDeudas(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "dashfeed@test.com")
	dia := time.Now().Format("2006-01-02")

	createTransaction(t, env, token, "egreso", 25000, dia)
	doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"tarjeta_credito","entidad":"Visa","descripcion":"Celular","monto_total":60000}`,
		"application/json", false)

	rec := doReq(t, env.router, http.MethodGet, "/api/dashboard", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	ultimos, _ := dash["ultimos_movimientos"].([]interface{})
	if len(ultimos) != 2 {
		t.Fatalf("expected 2 ultimos_movimientos (transacción + deuda), got %d", len(ultimos))
	}
	var transaccionVista, deudaVista bool
	for _, m := range ultimos {
		mm, _ := m.(map[string]interface{})
		switch mm["origen"] {
		case "deuda":
			deudaVista = true
			if mm["monto"] != 60000.0 {
				t.Errorf("expected deuda monto 60000, got %v", mm["monto"])
			}
		case "transaccion":
			transaccionVista = true
		}
	}
	if !transaccionVista || !deudaVista {
		t.Errorf("expected feed with transaccion and deuda, got transaccion=%v deuda=%v", transaccionVista, deudaVista)
	}
	// El feed NO debe afectar los egresos del mes (deuda no es egreso).
	mesActual, _ := dash["mes_actual"].(map[string]interface{})
	if mesActual["egresos_total"].(float64) != 25000 {
		t.Errorf("egresos_total no debe incluir la deuda, got %v", mesActual["egresos_total"])
	}
}

func TestDashboard_FeedFiltradoPorMes(t *testing.T) {
	env := newTestEnv(t)
	token, _ := registerJSON(t, env, "dashmes@test.com")
	periodo := time.Now().Format("2006-01")

	// transacción y deuda creadas hoy, dentro del período actual.
	createTransaction(t, env, token, "egreso", 1000, time.Now().Format("2006-01-02"))
	doReq(t, env.router, http.MethodPost, "/api/deudas", token,
		`{"tipo":"prestamo","entidad":"Banco","monto_total":90000}`,
		"application/json", false)

	rec := doReq(t, env.router, http.MethodGet, "/api/dashboard?periodo="+periodo, token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard filtered: expected 200, got %d", rec.Code)
	}
	var dash map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	ultimos, _ := dash["ultimos_movimientos"].([]interface{})
	if len(ultimos) != 2 {
		t.Errorf("expected 2 movimientos al filtrar por mes, got %d", len(ultimos))
	}

	// Filtrar por un mes sin movimientos devuelve un feed vacío.
	rec = doReq(t, env.router, http.MethodGet, "/api/dashboard?periodo=2001-01", token, "", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard filtered empty: expected 200, got %d", rec.Code)
	}
	json.Unmarshal(rec.Body.Bytes(), &dash)
	ultimos, _ = dash["ultimos_movimientos"].([]interface{})
	if len(ultimos) != 0 {
		t.Errorf("expected 0 movimientos en mes vacío, got %d", len(ultimos))
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
