package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var staticAssets embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(staticAssets, "static")
}
