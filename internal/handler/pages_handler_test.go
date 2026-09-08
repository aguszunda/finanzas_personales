package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// newTestTemplateManager crea un TemplateManager mínimo a partir de un
// MapFS con los templates necesarios para renderTemplate/renderTemplateFragment.
func newTestTemplateManager(files fstest.MapFS) *TemplateManager {
	return NewTemplateManager(files)
}

func TestVerificacionPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}<div>verificado</div>{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "reenvio_form"}}<form></form>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/verificacion?estado=ok", nil)
	h.VerificacionPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "verificado") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestReenvioPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":  {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reenvio.html": {Data: []byte(`{{define "content"}}<div>reenvio</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/reenvio", nil)
	h.ReenvioPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestForgotPasswordPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"forgot_password.html": {Data: []byte(`{{define "content"}}<div>forgot</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/forgot-password", nil)
	h.ForgotPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "forgot") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestForgotPasswordPage_ConEstadoExito(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"forgot_password.html": {Data: []byte(`{{define "content"}}{{if .Exito}}<div>exito</div>{{else}}<div>form</div>{{end}}{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/forgot-password?estado=ok", nil)
	h.ForgotPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "exito") {
		t.Errorf("expected exito state, got: %s", rec.Body.String())
	}
}

func TestForgotPasswordPage_ConEmail(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"forgot_password.html": {Data: []byte(`{{define "content"}}<input value="{{.Email}}">{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/forgot-password?email=test%40correo.com", nil)
	h.ForgotPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "test@correo.com") {
		t.Errorf("expected pre-filled email in body, got: %s", rec.Body.String())
	}
}

func TestForgotPasswordPage_SinEmail(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":          {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"forgot_password.html": {Data: []byte(`{{define "content"}}<input value="{{.Email}}">{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/forgot-password", nil)
	h.ForgotPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "value=\"test") {
		t.Errorf("should not have email value, got: %s", rec.Body.String())
	}
}
