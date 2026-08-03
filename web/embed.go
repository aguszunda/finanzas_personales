package web

import (
	"embed"
	"io/fs"
)

//go:embed templates/*.html
var templateFiles embed.FS

func TemplateFS() fs.FS {
	sub, err := fs.Sub(templateFiles, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}
