package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var embeddedFS embed.FS

// StaticFS returns the embedded static frontend assets rooted at static/.
func StaticFS() (fs.FS, error) {
	return fs.Sub(embeddedFS, "static")
}
