package web

import "embed"

// Files contains the browser assets compiled into the AlphaDrive binary.
//
//go:embed templates/*.html static/css/*.css static/js/*.js
var Files embed.FS
