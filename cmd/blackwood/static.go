package main

import (
	"embed"
	"io/fs"
)

//go:embed all:static
var staticFiles embed.FS

func staticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "static")
}
