package web

import "embed"

// Files contains the browser assets compiled into the AlphaDrive binary.
//
//go:embed templates/* static/*
var Files embed.FS
