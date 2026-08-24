package handler

import (
	"html/template"
	"io/fs"
	"log/slog"
)

type TemplateManager struct {
	funcs template.FuncMap
	files map[string]string
}

// pageFragments asocia cada página con los partials de formulario que embebe
// en su contenido inicial (modo "nuevo"). El mismo partial se reutiliza en el
// modo edición a través de renderTemplateFragment cuando HTMX lo solicita.
var pageFragments = map[string][]string{
	"transacciones": {"transaccion_form"},
	"deudas":        {"deuda_form"},
	// register_exito es el pop-up post-alta (swap HTMX o render embebido);
	// login_verificar es el pop-up al loguear una cuenta sin verificar;
	// verificacion embebe el form de reenvío en sus estados de error.
	"register":     {"register_exito"},
	"login":        {"login_verificar"},
	"verificacion": {"reenvio_form"},
}

func NewTemplateManager(templatesFS fs.FS) *TemplateManager {
	funcMap := template.FuncMap{
		"mod": func(i, j int) int { return i % j },
		"sub": func(a, b int) int { return a - b },
		"deref": func(v *float64) float64 {
			if v == nil {
				return 0
			}
			return *v
		},
	}
	files := make(map[string]string)
	if entries, err := fs.Glob(templatesFS, "*.html"); err == nil {
		for _, entry := range entries {
			content, err := fs.ReadFile(templatesFS, entry)
			if err != nil {
				slog.Warn("cannot read template", "file", entry, "error", err)
				continue
			}
			name := entry[:len(entry)-5]
			files[name] = string(content)
		}
	}
	slog.Info("templates loaded", "count", len(files))
	return &TemplateManager{funcs: funcMap, files: files}
}
