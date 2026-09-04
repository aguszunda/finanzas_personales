package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"optipay/internal/middleware"
	"optipay/internal/model"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

// hashToken computes the SHA-256 hex of a raw token, matching the service's
// hashVerificationToken function.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ctxWithIsHTMX returns a context with the HTMX detection flag set to true.
func ctxWithIsHTMX(ctx context.Context) context.Context {
	return context.WithValue(ctx, middleware.IsHTMXKey, true)
}

// minimalTemplateFS builds a fstest.MapFS with a layout and the given named
// template files so renderTemplate / renderTemplateFragment don't 500.
func minimalTemplateFS(names ...string) fstest.MapFS {
	fs := fstest.MapFS{
		"layout.html": {Data: []byte(`<html>{{template "content" .}}</html>`)},
	}
	for _, n := range names {
		fs[n+".html"] = &fstest.MapFile{
			Data: []byte(`{{define "content"}}` + n + `{{end}}`),
		}
	}
	return fs
}

// ---------- Register ----------

func TestAuthHandler_Register_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO usuarios (nombre, email, password_hash, moneda_default) VALUES (?, ?, ?, ?)")).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(7, 1))
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"nombre":"Agustin","email":"a@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Mensaje string `json:"mensaje"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Mensaje == "" {
		t.Error("expected a non-empty mensaje")
	}
}

func TestAuthHandler_Register_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("POST", "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAuthHandler_Register_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO usuarios (nombre, email, password_hash, moneda_default) VALUES (?, ?, ?, ?)")).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(7, 1))
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":         {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"register_exito.html": {Data: []byte(`OK`)},
	})

	body := strings.NewReader(`{"nombre":"Agustin","email":"a@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.Register(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "OK") {
		t.Errorf("expected fragment content, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_Register_FormURLEncoded(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta("INSERT INTO usuarios (nombre, email, password_hash, moneda_default) VALUES (?, ?, ?, ?)")).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(7, 1))
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":         {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"register.html":       {Data: []byte(`{{define "content"}}register page{{end}}`)},
		"register_exito.html": {Data: []byte(`{{define "content"}}exito fragment{{end}}`)},
	})

	body := strings.NewReader("nombre=Agustin&email=a@test.com&password=secreto123")
	req := httptest.NewRequest("POST", "/api/auth/register", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	f.authH.Register(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %s", ct)
	}
}

// ---------- Login ----------

func TestAuthHandler_Login_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)

	body := strings.NewReader(`{"email":"a@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Set-Cookie with token")
	}
}

func TestAuthHandler_Login_SuccessHTMX(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)

	body := strings.NewReader(`{"email":"a@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if hx := rec.Header().Get("HX-Redirect"); hx != "/api/dashboard/page" {
		t.Errorf("expected HX-Redirect to /api/dashboard/page, got %q", hx)
	}
}

func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)

	body := strings.NewReader(`{"email":"a@test.com","password":"wrongpass1"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Login_EmailNoVerificado_JSON(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(9, "Agustin", "pendiente@test.com", string(hash), "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("pendiente@test.com").
		WillReturnRows(rows)

	body := strings.NewReader(`{"email":"pendiente@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Login_EmailNoVerificado_HTMX(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(9, "Agustin", "pendiente@test.com", string(hash), "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("pendiente@test.com").
		WillReturnRows(rows)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"login_verificar.html": {Data: []byte(`verificar fragment`)},
	})

	body := strings.NewReader(`{"email":"pendiente@test.com","password":"secreto123"}`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "verificar fragment") {
		t.Errorf("expected verificar fragment, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_Login_EmailNoVerificado_Form(t *testing.T) {
	f := newHandlerFixture(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(9, "Agustin", "pendiente@test.com", string(hash), "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("pendiente@test.com").
		WillReturnRows(rows)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"login.html":           {Data: []byte(`{{define "content"}}login page{{end}}`)},
		"login_verificar.html": {Data: []byte(`{{define "content"}}verificar popup{{end}}`)},
	})

	body := strings.NewReader("email=pendiente@test.com&password=secreto123")
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html, got %s", ct)
	}
}

func TestAuthHandler_Login_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("POST", "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.Login(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- Verificar ----------

func TestAuthHandler_Verificar_Success(t *testing.T) {
	f := newHandlerFixture(t)
	rawToken := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	tokenHash := hashToken(rawToken)
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion",
	}).AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", time.Now(), false, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion FROM usuarios WHERE token_verificacion = ?")).
		WithArgs(tokenHash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET email_verificado = TRUE, token_expiracion = NULL WHERE id = ?")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}verificacion page{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "content"}}reenvio form{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/auth/verificar?token="+rawToken, nil)
	rec := httptest.NewRecorder()

	f.authH.Verificar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Verificar_Expirado(t *testing.T) {
	f := newHandlerFixture(t)
	rawToken := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	tokenHash := hashToken(rawToken)
	expira := time.Now().Add(-time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion",
	}).AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", time.Now(), false, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion FROM usuarios WHERE token_verificacion = ?")).
		WithArgs(tokenHash).
		WillReturnRows(rows)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}verificacion page{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "content"}}reenvio form{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/auth/verificar?token="+rawToken, nil)
	rec := httptest.NewRecorder()

	f.authH.Verificar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Verificar_Invalido(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion FROM usuarios WHERE token_verificacion = ?")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(model.ErrNotFound)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}verificacion page{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "content"}}reenvio form{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/auth/verificar?token=invalid-token", nil)
	rec := httptest.NewRecorder()

	f.authH.Verificar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_Verificar_TokenVacio(t *testing.T) {
	f := newHandlerFixture(t)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}verificacion page{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "content"}}reenvio form{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/auth/verificar?token=%20%20", nil)
	rec := httptest.NewRecorder()

	f.authH.Verificar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------- ReenviarVerificacion ----------

func TestAuthHandler_ReenviarVerificacion_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"email":"a@test.com"}`)
	req := httptest.NewRequest("POST", "/api/auth/reenviar-verificacion", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ReenviarVerificacion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %s", ct)
	}
}

func TestAuthHandler_ReenviarVerificacion_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":     {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reenvio_ok.html": {Data: []byte(`reenvio ok`)},
	})

	body := strings.NewReader(`{"email":"a@test.com"}`)
	req := httptest.NewRequest("POST", "/api/auth/reenviar-verificacion", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.ReenviarVerificacion(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "reenvio ok") {
		t.Errorf("expected reenvio ok fragment, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_ReenviarVerificacion_FormRedirect(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader("email=a@test.com")
	req := httptest.NewRequest("POST", "/api/auth/reenviar-verificacion", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	f.authH.ReenviarVerificacion(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/verificacion?estado=reenviado" {
		t.Errorf("expected redirect to /verificacion?estado=reenviado, got %q", loc)
	}
}

func TestAuthHandler_ReenviarVerificacion_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("POST", "/api/auth/reenviar-verificacion", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ReenviarVerificacion(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- ForgotPassword ----------

func TestAuthHandler_ForgotPassword_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"email":"a@test.com"}`)
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ForgotPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %s", ct)
	}
}

func TestAuthHandler_ForgotPassword_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":    {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"forgot_ok.html": {Data: []byte(`forgot ok`)},
	})

	body := strings.NewReader(`{"email":"a@test.com"}`)
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.ForgotPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "forgot ok") {
		t.Errorf("expected forgot ok fragment, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_ForgotPassword_FormRedirect(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?")).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader("email=a@test.com")
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	f.authH.ForgotPassword(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/forgot-password?estado=ok" {
		t.Errorf("expected redirect to /forgot-password?estado=ok, got %q", loc)
	}
}

func TestAuthHandler_ForgotPassword_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("POST", "/api/auth/forgot-password", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ForgotPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- ResetPassword ----------

func TestAuthHandler_ResetPassword_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	rawToken := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	tokenHash := hashToken(rawToken)
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion",
	}).AddRow(7, "Agustin", "a@test.com", "$2a$oldhash", "ARS", time.Now(), true, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion FROM usuarios WHERE password_reset_token = ?")).
		WithArgs(tokenHash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"token":"` + rawToken + `","password":"nuevaclave1","password2":"nuevaclave1"}`)
	req := httptest.NewRequest("POST", "/api/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ResetPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %s", ct)
	}
}

func TestAuthHandler_ResetPassword_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	rawToken := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	tokenHash := hashToken(rawToken)
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion",
	}).AddRow(7, "Agustin", "a@test.com", "$2a$oldhash", "ARS", time.Now(), true, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion FROM usuarios WHERE password_reset_token = ?")).
		WithArgs(tokenHash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":      {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reset_ok.html":    {Data: []byte(`reset ok`)},
		"reset_error.html": {Data: []byte(`reset error`)},
	})

	body := strings.NewReader(`{"token":"` + rawToken + `","password":"nuevaclave1","password2":"nuevaclave1"}`)
	req := httptest.NewRequest("POST", "/api/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithIsHTMX(req.Context()))
	rec := httptest.NewRecorder()

	f.authH.ResetPassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "reset ok") {
		t.Errorf("expected reset ok fragment, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_ResetPassword_FormRedirect(t *testing.T) {
	f := newHandlerFixture(t)
	rawToken := "aabbccdd11223344aabbccdd11223344aabbccdd11223344aabbccdd11223344"
	tokenHash := hashToken(rawToken)
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion",
	}).AddRow(7, "Agustin", "a@test.com", "$2a$oldhash", "ARS", time.Now(), true, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion FROM usuarios WHERE password_reset_token = ?")).
		WithArgs(tokenHash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader("token=" + rawToken + "&password=nuevaclave1&password2=nuevaclave1")
	req := httptest.NewRequest("POST", "/api/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	f.authH.ResetPassword(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login?reset=ok" {
		t.Errorf("expected redirect to /login?reset=ok, got %q", loc)
	}
}

func TestAuthHandler_ResetPassword_InvalidToken(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion FROM usuarios WHERE password_reset_token = ?")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(model.ErrNotFound)

	body := strings.NewReader(`{"token":"bad","password":"nuevaclave1","password2":"nuevaclave1"}`)
	req := httptest.NewRequest("POST", "/api/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ResetPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_ResetPassword_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("POST", "/api/auth/reset-password", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	f.authH.ResetPassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- ProfilePage ----------

func TestAuthHandler_ProfilePage_Success(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":  {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"profile.html": {Data: []byte(`{{define "content"}}profile {{.Usuario.Nombre}}{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/profile/page", nil)
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.ProfilePage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Agustin") {
		t.Errorf("expected username in body, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_ProfilePage_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(999)).
		WillReturnError(model.ErrNotFound)

	req := httptest.NewRequest("GET", "/api/profile/page", nil)
	req = req.WithContext(ctxWithUserID(999))
	rec := httptest.NewRecorder()

	f.authH.ProfilePage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------- ChangePasswordPage ----------

func TestAuthHandler_ChangePasswordPage_Success(t *testing.T) {
	f := newHandlerFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)

	old := tmpl
	defer func() { tmpl = old }()
	tmpl = newTestTemplateManager(fstest.MapFS{
		"layout.html":           {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"cambiar_password.html": {Data: []byte(`{{define "content"}}cambiar password{{end}}`)},
	})

	req := httptest.NewRequest("GET", "/api/profile/change-password", nil)
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.ChangePasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cambiar password") {
		t.Errorf("expected template content, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_ChangePasswordPage_NotFound(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(999)).
		WillReturnError(model.ErrNotFound)

	req := httptest.NewRequest("GET", "/api/profile/change-password", nil)
	req = req.WithContext(ctxWithUserID(999))
	rec := httptest.NewRecorder()

	f.authH.ChangePasswordPage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------- UpdateNombre ----------

func TestAuthHandler_UpdateNombre_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET nombre = ? WHERE id = ?")).
		WithArgs("Nuevo Nombre", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"nombre":"Nuevo Nombre"}`)
	req := httptest.NewRequest("PUT", "/api/profile/nombre", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdateNombre(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_UpdateNombre_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET nombre = ? WHERE id = ?")).
		WithArgs("Nuevo Nombre", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"nombre":"Nuevo Nombre"}`)
	req := httptest.NewRequest("PUT", "/api/profile/nombre", body)
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxWithUserID(5)
	ctx = ctxWithIsHTMX(ctx)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	f.authH.UpdateNombre(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if hx := rec.Header().Get("HX-Redirect"); hx != "/api/profile/page" {
		t.Errorf("expected HX-Redirect to /api/profile/page, got %q", hx)
	}
}

func TestAuthHandler_UpdateNombre_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("PUT", "/api/profile/nombre", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdateNombre(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- UpdatePassword ----------

func TestAuthHandler_UpdatePassword_SuccessJSON(t *testing.T) {
	f := newHandlerFixture(t)
	oldHash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(oldHash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"password_actual":"oldpass123","password_nuevo":"newpass456","password2":"newpass456"}`)
	req := httptest.NewRequest("PUT", "/api/profile/password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdatePassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_UpdatePassword_HTMXSuccess(t *testing.T) {
	f := newHandlerFixture(t)
	oldHash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(oldHash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	body := strings.NewReader(`{"password_actual":"oldpass123","password_nuevo":"newpass456","password2":"newpass456"}`)
	req := httptest.NewRequest("PUT", "/api/profile/password", body)
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxWithUserID(5)
	ctx = ctxWithIsHTMX(ctx)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	f.authH.UpdatePassword(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if hx := rec.Header().Get("HX-Redirect"); hx != "/api/profile/page" {
		t.Errorf("expected HX-Redirect to /api/profile/page, got %q", hx)
	}
}

func TestAuthHandler_UpdatePassword_WrongCurrentPassword(t *testing.T) {
	f := newHandlerFixture(t)
	oldHash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(oldHash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)

	body := strings.NewReader(`{"password_actual":"wrongpass1","password_nuevo":"newpass456","password2":"newpass456"}`)
	req := httptest.NewRequest("PUT", "/api/profile/password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdatePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAuthHandler_UpdatePassword_SameAsCurrent(t *testing.T) {
	f := newHandlerFixture(t)
	oldHash, _ := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(oldHash), "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta("SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE id = ?")).
		WithArgs(int64(5)).
		WillReturnRows(rows)

	body := strings.NewReader(`{"password_actual":"oldpass123","password_nuevo":"oldpass123","password2":"oldpass123"}`)
	req := httptest.NewRequest("PUT", "/api/profile/password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdatePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "la nueva contraseña debe ser distinta a la actual") {
		t.Errorf("expected mensaje de clave igual a la anterior, got: %s", rec.Body.String())
	}
}

func TestAuthHandler_UpdatePassword_DecodeError(t *testing.T) {
	f := newHandlerFixture(t)

	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest("PUT", "/api/profile/password", body)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxWithUserID(5))
	rec := httptest.NewRecorder()

	f.authH.UpdatePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ---------- Logout ----------

func TestAuthHandler_Logout(t *testing.T) {
	f := newHandlerFixture(t)

	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	rec := httptest.NewRecorder()

	f.authH.Logout(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "token" {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("expected MaxAge -1 (delete), got %d", c.MaxAge)
			}
			if c.Value != "" {
				t.Errorf("expected empty value, got %q", c.Value)
			}
		}
	}
	if !found {
		t.Error("expected Set-Cookie for token")
	}
}
