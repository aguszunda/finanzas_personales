package web

import (
	"embed"
	"io/fs"
)

//go:embed templates/*.html
var templateFiles embed.FS

//go:embed static
var staticFiles embed.FS

//go:embed images
var imageFiles embed.FS

func TemplateFS() fs.FS {
	sub, err := fs.Sub(templateFiles, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

func StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func ImagesFS() fs.FS {
	sub, err := fs.Sub(imageFiles, "images")
	if err != nil {
		panic(err)
	}
	return sub
}
