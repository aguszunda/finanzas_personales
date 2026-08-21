package handler

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"optipay/internal/middleware"
	"optipay/internal/model"
)

// decodeBody decodes a request body into dst, accepting both JSON and
// application/x-www-form-urlencoded payloads (HTMX forms).
func decodeBody(r *http.Request, dst interface{}) error {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return json.NewDecoder(r.Body).Decode(dst)
	}
	if err := r.ParseForm(); err != nil {
		return err
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return errors.New("decodeBody requires a struct pointer")
	}
	s := v.Elem()
	t := s.Type()
	for key, values := range r.PostForm {
		if len(values) == 0 {
			continue
		}
		for i := 0; i < s.NumField(); i++ {
			name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
			if name == "" {
				name = t.Field(i).Name
			}
			if !strings.EqualFold(name, key) {
				continue
			}
			setFormField(s.Field(i), values[0])
			break
		}
	}
	return nil
}

func setFormField(field reflect.Value, raw string) {
	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		field.SetBool(raw == "true" || raw == "1" || raw == "on")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
			field.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if u, err := strconv.ParseUint(raw, 10, 64); err == nil {
			field.SetUint(u)
		}
	case reflect.Float32, reflect.Float64:
		if f, err := strconv.ParseFloat(raw, 64); err == nil {
			field.SetFloat(f)
		}
	case reflect.Ptr:
		if raw == "" {
			return
		}
		elem := field.Type().Elem()
		p := reflect.New(elem)
		switch elem.Kind() {
		case reflect.String:
			p.Elem().SetString(raw)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
				p.Elem().SetInt(i)
			}
		case reflect.Float32, reflect.Float64:
			if f, err := strconv.ParseFloat(raw, 64); err == nil {
				p.Elem().SetFloat(f)
			}
		default:
			return
		}
		field.Set(p)
	}
}

var tmpl *TemplateManager

func SetTemplateManager(t *TemplateManager) {
	tmpl = t
}

func renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if tmpl == nil {
		http.Error(w, "templates not initialized", http.StatusInternalServerError)
		return
	}
	layout, ok := tmpl.files["layout"]
	if !ok {
		http.Error(w, "layout template missing", http.StatusInternalServerError)
		return
	}
	page, ok := tmpl.files[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	t := template.New("layout").Funcs(tmpl.funcs)
	if _, err := t.Parse(layout); err != nil {
		slog.Error("layout parse error", "error", err)
		http.Error(w, "error interno del servidor", http.StatusInternalServerError)
		return
	}
	if _, err := t.Parse(page); err != nil {
		slog.Error("page parse error", "name", name, "error", err)
		http.Error(w, "error interno del servidor", http.StatusInternalServerError)
		return
	}
	for _, frag := range pageFragments[name] {
		content, ok := tmpl.files[frag]
		if !ok {
			continue
		}
		if _, err := t.Parse(content); err != nil {
			slog.Error("fragment parse error", "name", frag, "error", err)
			http.Error(w, "error interno del servidor", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("template error", "name", name, "error", err)
	}
}

// renderTemplateFragment renderiza un partial aislado (sin layout) para
// respuestas HTMX de intercambio parcial.
func renderTemplateFragment(w http.ResponseWriter, name string, data interface{}) {
	if tmpl == nil {
		http.Error(w, "templates not initialized", http.StatusInternalServerError)
		return
	}
	content, ok := tmpl.files[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	t := template.New(name).Funcs(tmpl.funcs)
	if _, err := t.Parse(content); err != nil {
		slog.Error("fragment parse error", "name", name, "error", err)
		http.Error(w, "error interno del servidor", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("template error", "name", name, "error", err)
	}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		if err := json.NewEncoder(w).Encode(data); err != nil {
			slog.Error("json encode error", "error", err)
		}
	}
}

func respondError(w http.ResponseWriter, status int, msg string) {
	respondJSON(w, status, ErrorResponse{Error: msg})
}

// respondHTMXRedirect makes HTMX navigate to url instead of swapping the
// response body. Used for mutations performed from the web UI.
func respondHTMXRedirect(w http.ResponseWriter, status int, url string) {
	w.Header().Set("HX-Redirect", url)
	w.WriteHeader(status)
}

// respondMutation answers a mutation request depending on its client:
//   - HTMX (HX-Request) -> HX-Redirect header so the page reloads.
//   - plain HTML form (urlencoded) -> standard 303 redirect.
//   - API client (JSON) -> JSON payload.
func respondMutation(w http.ResponseWriter, r *http.Request, jsonStatus int, data interface{}, redirectURL string) {
	if middleware.IsHTMXRequest(r.Context()) {
		respondHTMXRedirect(w, jsonStatus, redirectURL)
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}
	respondJSON(w, jsonStatus, data)
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, model.ErrNotFound):
		respondError(w, http.StatusNotFound, "recurso no encontrado")
	case errors.Is(err, model.ErrEmailNoExiste):
		respondError(w, http.StatusNotFound, "el email no está registrado")
	case errors.Is(err, model.ErrUnauthorized):
		respondError(w, http.StatusUnauthorized, "la contraseña es incorrecta")
	case errors.Is(err, model.ErrEmailInvalido):
		respondError(w, http.StatusBadRequest, "el email no es válido")
	case errors.Is(err, model.ErrPasswordInvalido):
		respondError(w, http.StatusBadRequest, "la contraseña debe tener entre 8 y 72 caracteres y contener solo letras y números")
	case errors.Is(err, model.ErrInvalidInput):
		respondError(w, http.StatusBadRequest, "datos inválidos")
	case errors.Is(err, model.ErrEmailExiste):
		respondError(w, http.StatusConflict, "el email ya está registrado")
	case errors.Is(err, model.ErrMesCerrado):
		respondError(w, http.StatusConflict, "el mes está cerrado, no se puede modificar")
	default:
		slog.Error("internal error", "error", err)
		respondError(w, http.StatusInternalServerError, "error interno del servidor")
	}
}
