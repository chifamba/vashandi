// Package ui embeds the compiled admin UI static files.
package ui

import "embed"

//go:embed dist
var FS embed.FS
