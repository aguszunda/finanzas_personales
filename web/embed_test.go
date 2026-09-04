package web

import (
	"io/fs"
	"testing"
)

func TestTemplateFS(t *testing.T) {
	fsys := TemplateFS()
	if fsys == nil {
		t.Fatal("TemplateFS() devolvió nil")
	}
	if _, err := fs.Stat(fsys, "layout.html"); err != nil {
		t.Errorf("layout.html no accesible en TemplateFS: %v", err)
	}
}

func TestStaticFS(t *testing.T) {
	fsys := StaticFS()
	if fsys == nil {
		t.Fatal("StaticFS() devolvió nil")
	}
	if _, err := fs.Stat(fsys, "css"); err != nil {
		t.Errorf("css no accesible en StaticFS: %v", err)
	}
}

func TestImagesFS(t *testing.T) {
	fsys := ImagesFS()
	if fsys == nil {
		t.Fatal("ImagesFS() devolvió nil")
	}
	if _, err := fs.Stat(fsys, "."); err != nil {
		t.Errorf("imagen raíz no accesible: %v", err)
	}
}
