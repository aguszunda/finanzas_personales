package handler

import (
	"html/template"
	"io/fs"
	"log/slog"
	"math"
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
	// register_exito es el pop-up post-alta (swap HTMX o render embebido);
	// login_verificar es el pop-up al loguear una cuenta sin verificar;
	// verificacion embebe el form de reenvío en sus estados de error;
	// reset_password embebe los fragments de éxito y error del reseteo.
	"register":        {"register_exito"},
	"login":           {"login_verificar"},
	"verificacion":    {"reenvio_form"},
	"forgot_password": {"forgot_ok"},
	"reset_password":  {"reset_ok", "reset_error"},
}

func NewTemplateManager(templatesFS fs.FS) *TemplateManager {
	funcMap := template.FuncMap{
		"mod": func(i, j int) int { return i % j },
		"sub": func(a, b int) int { return a - b },
		"abs": func(v float64) float64 { return math.Abs(v) },
		"deref": func(v *float64) float64 {
			if v == nil {
				return 0
			}
			return *v
		},
		"catIcon": catIcon,
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

// catIcons agrupa los trazados SVG (estilo Feather) de las categorías por
// nombre. Las categorías personalizadas que no figuren caen en el default.
var catIcons = map[string]template.HTML{
	"Sueldo":          svgIcon(`<line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>`),
	"Freelance":       svgIcon(`<rect x="2" y="7" width="20" height="14" rx="2" ry="2"/><path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16"/>`),
	"Ventas":          svgIcon(`<circle cx="9" cy="21" r="1"/><circle cx="20" cy="21" r="1"/><path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6"/>`),
	"Otros Ingresos":  svgIcon(`<polyline points="20 12 20 22 4 22 4 12"/><rect x="2" y="7" width="20" height="5"/><line x1="12" y1="22" x2="12" y2="7"/><path d="M12 7H7.5a2.5 2.5 0 0 1 0-5C11 2 12 7 12 7z"/><path d="M12 7h4.5a2.5 2.5 0 0 0 0-5C13 2 12 7 12 7z"/>`),
	"Alquiler":        svgIcon(`<path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>`),
	"Servicios":       svgIcon(`<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>`),
	"Comida":          svgIcon(`<path d="M18 8h1a4 4 0 0 1 0 8h-1"/><path d="M2 8h16v9a4 4 0 0 1-4 4H6a4 4 0 0 1-4-4V8z"/><line x1="6" y1="1" x2="6" y2="4"/><line x1="10" y1="1" x2="10" y2="4"/><line x1="14" y1="1" x2="14" y2="4"/>`),
	"Transporte":      svgIcon(`<rect x="1" y="3" width="15" height="13"/><polygon points="16 8 20 8 23 11 23 16 16 16 16 8"/><circle cx="5.5" cy="18.5" r="2.5"/><circle cx="18.5" cy="18.5" r="2.5"/>`),
	"Salud":           svgIcon(`<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>`),
	"Educación":       svgIcon(`<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>`),
	"Entretenimiento": svgIcon(`<rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"/><line x1="7" y1="2" x2="7" y2="22"/><line x1="17" y1="2" x2="17" y2="22"/><line x1="2" y1="12" x2="22" y2="12"/><line x1="2" y1="7" x2="7" y2="7"/><line x1="2" y1="17" x2="7" y2="17"/><line x1="17" y1="17" x2="22" y2="17"/><line x1="17" y1="7" x2="22" y2="7"/>`),
	"Suscripciones":   svgIcon(`<polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>`),
	"Imprevistos":     svgIcon(`<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>`),
	"":                svgIcon(`<path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.83z"/><line x1="7" y1="7" x2="7.01" y2="7"/>`),
}

func svgIcon(inner string) template.HTML {
	// #nosec G203 -- trazados SVG literales fijos (constantes de catIcons),
	// sin datos de runtime ni entrada del usuario: markup de confianza.
	return template.HTML(`<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + inner + `</svg>`)
}

// catIcon devuelve el icono SVG profesional de una categoría según su nombre.
// Las categorías desconocidas usan el icono por defecto.
func catIcon(nombre string) template.HTML {
	if icon, ok := catIcons[nombre]; ok {
		return icon
	}
	return catIcons[""]
}
