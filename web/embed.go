// Package web embeds the Parcel-built reader and author/admin SPAs
// (web/dist), including locally bundled Bootstrap 5 and Bootstrap Icons
// assets, into the Go binaries. Production never reads web/dist from disk.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the embedded Parcel output rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees dist exists
	}
	return sub
}
