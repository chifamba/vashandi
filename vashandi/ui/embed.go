// Package ui embeds the compiled Paperclip board UI static files.
package ui

import "embed"

// FS is the embedded filesystem containing the compiled UI assets.
//
//go:embed all:dist
var FS embed.FS
