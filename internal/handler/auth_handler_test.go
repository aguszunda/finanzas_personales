package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestResetPasswordPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":         {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reset_password.html": {Data: []byte(`{{define "content"}}<div>reset {{.Token}}</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &AuthHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/reset-password?token=abc123", nil)
	h.ResetPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc123") {
		t.Errorf("expected token in body, got: %s", rec.Body.String())
	}
}

func TestResetPasswordPage_SinToken(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":         {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reset_password.html": {Data: []byte(`{{define "content"}}<div>reset {{.Token}}</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &AuthHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/reset-password", nil)
	h.ResetPasswordPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "abc123") {
		t.Errorf("should not contain old token, got: %s", body)
	}
}
