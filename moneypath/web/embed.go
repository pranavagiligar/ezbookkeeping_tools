package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

// GetFileSystem returns the embedded filesystem sub-rooted at static/
func GetFileSystem() (http.FileSystem, error) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}
