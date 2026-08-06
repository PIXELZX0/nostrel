// Package web holds the embedded panel assets.
package web

import (
	"embed"
	"net/http"
)

// all: is required because the vendored crypto modules include files whose
// names start with an underscore, which plain go:embed skips.
//
//go:embed *.html all:static
var Files embed.FS

// Page serves one of the embedded HTML files.
func Page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := Files.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body)
	}
}
