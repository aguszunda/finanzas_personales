package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestTemplateManager_FuncMap(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":  {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"funcmap.html": {Data: []byte(`{{define "content"}}<div>{{mod 5 2}} {{sub 7 3}} {{deref nil}}</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	rec := httptest.NewRecorder()
	renderTemplate(rec, "funcmap", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "1") || !strings.Contains(body, "4") || !strings.Contains(body, "0") {
		t.Errorf("expected '1 4 0' in body, got: %s", body)
	}
}

func TestTemplateManager_FuncMap_DerefNonNil(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"deref_nonnil.html": {Data: []byte(`{{define "content"}}<div>{{deref .Val}}</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	v := 42.0
	rec := httptest.NewRecorder()
	renderTemplate(rec, "deref_nonnil", map[string]interface{}{"Val": &v})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "42") {
		t.Errorf("expected '42' in body, got: %s", rec.Body.String())
	}
}
