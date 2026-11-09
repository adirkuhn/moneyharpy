// Package web embeds the built web application.
package web

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the files of the built web application.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("web dist directory: %v", err)) // The embed path is fixed at compile time.
	}
	return sub
}
