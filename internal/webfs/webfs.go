// Package webfs embeds the browser front-end so that the Go binary serves the
// pages without external files. The static assets live under web/ and are
// embedded at build time.
package webfs

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var assets embed.FS

// StaticFS returns the embedded web subtree as an http.FileSystem.
func StaticFS() http.FileSystem {
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// ContentFS returns the embedded fs.FS rooted at web for direct file reads.
func ContentFS() fs.FS {
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
