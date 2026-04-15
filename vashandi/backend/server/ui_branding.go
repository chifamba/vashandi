package server

import (
	"fmt"
	"math"
	"os"
	"strings"
)

const (
	faviconBlockStart        = "<!-- PAPERCLIP_FAVICON_START -->"
	faviconBlockEnd          = "<!-- PAPERCLIP_FAVICON_END -->"
	runtimeBrandingBlockStart = "<!-- PAPERCLIP_RUNTIME_BRANDING_START -->"
	runtimeBrandingBlockEnd   = "<!-- PAPERCLIP_RUNTIME_BRANDING_END -->"

	defaultFaviconLinks = `<link rel="icon" href="/favicon.ico" sizes="48x48" />
<link rel="icon" href="/favicon.svg" type="image/svg+xml" />
<link rel="icon" type="image/png" sizes="32x32" href="/favicon-32x32.png" />
<link rel="icon" type="image/png" sizes="16x16" href="/favicon-16x16.png" />`
)

type worktreeUiBranding struct {
	Enabled    bool
	Name       string
	Color      string
	TextColor  string
	FaviconHref string
}

func isTruthyEnvValue(value string) bool {
	normalized := strings.TrimSpace(strings.ToLower(value))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

func nonEmpty(value string) string {
	normalized := strings.TrimSpace(value)
	return normalized
}

func normalizeHexColor(value string) string {
	raw := nonEmpty(value)
	if raw == "" {
		return ""
	}
	hex := raw
	if strings.HasPrefix(hex, "#") {
		hex = hex[1:]
	}
	if len(hex) == 3 {
		expanded := ""
		for _, c := range hex {
			expanded += string(c) + string(c)
		}
		return "#" + strings.ToLower(expanded)
	}
	if len(hex) == 6 {
		valid := true
		for _, c := range hex {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				valid = false
				break
			}
		}
		if valid {
			return "#" + strings.ToLower(hex)
		}
	}
	return ""
}

func hslToHex(hue, saturation, lightness float64) string {
	s := math.Max(0, math.Min(100, saturation)) / 100
	l := math.Max(0, math.Min(100, lightness)) / 100
	c := (1 - math.Abs(2*l-1)) * s
	h := math.Mod(math.Mod(hue, 360)+360, 360)
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	toHex := func(v float64) string {
		n := int(math.Round(math.Max(0, math.Min(255, (v+m)*255))))
		return fmt.Sprintf("%02x", n)
	}
	return "#" + toHex(r) + toHex(g) + toHex(b)
}

func deriveColorFromSeed(seed string) string {
	var hash uint32
	for _, c := range seed {
		hash = (hash*33 + uint32(c))
	}
	return hslToHex(float64(hash%360), 68, 56)
}

func hexToRGB(color string) (r, g, b int) {
	normalized := normalizeHexColor(color)
	if normalized == "" {
		normalized = "#000000"
	}
	hex := normalized[1:]
	fmt.Sscanf(hex[0:2], "%x", &r)
	fmt.Sscanf(hex[2:4], "%x", &g)
	fmt.Sscanf(hex[4:6], "%x", &b)
	return
}

func relativeLuminanceChannel(value int) float64 {
	normalized := float64(value) / 255
	if normalized <= 0.03928 {
		return normalized / 12.92
	}
	return math.Pow((normalized+0.055)/1.055, 2.4)
}

func relativeLuminance(color string) float64 {
	r, g, b := hexToRGB(color)
	return 0.2126*relativeLuminanceChannel(r) +
		0.7152*relativeLuminanceChannel(g) +
		0.0722*relativeLuminanceChannel(b)
}

func pickReadableTextColor(background string) string {
	lum := relativeLuminance(background)
	whiteContrast := 1.05 / (lum + 0.05)
	blackContrast := (lum + 0.05) / 0.05
	if whiteContrast >= blackContrast {
		return "#f8fafc"
	}
	return "#111827"
}

func escapeHTMLAttr(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, `"`, "&quot;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

func createFaviconDataURL(background, foreground string) string {
	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"><rect width="24" height="24" rx="6" fill="%s"/><path stroke="%s" stroke-linecap="round" stroke-linejoin="round" stroke-width="2.15" d="m16 6-8.414 8.586a2 2 0 0 0 2.829 2.829l8.414-8.586a4 4 0 1 0-5.657-5.657l-8.379 8.551a6 6 0 1 0 8.485 8.485l8.379-8.551"/></svg>`,
		background, foreground,
	)
	// Percent-encode the SVG for use as a data URL (matching Node.js encodeURIComponent behavior)
	encoded := svgEncodeURIComponent(svg)
	return "data:image/svg+xml," + encoded
}

// svgEncodeURIComponent encodes an SVG string for use in a data: URL, matching
// the characters that JavaScript's encodeURIComponent encodes.
func svgEncodeURIComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isURIComponentUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isURIComponentUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '!' ||
		c == '~' || c == '*' || c == '\'' || c == '(' || c == ')'
}

func getWorktreeUiBranding() worktreeUiBranding {
	if !isTruthyEnvValue(os.Getenv("PAPERCLIP_IN_WORKTREE")) {
		return worktreeUiBranding{}
	}

	name := nonEmpty(os.Getenv("PAPERCLIP_WORKTREE_NAME"))
	if name == "" {
		name = nonEmpty(os.Getenv("PAPERCLIP_INSTANCE_ID"))
	}
	if name == "" {
		name = "worktree"
	}

	color := normalizeHexColor(os.Getenv("PAPERCLIP_WORKTREE_COLOR"))
	if color == "" {
		color = deriveColorFromSeed(name)
	}
	textColor := pickReadableTextColor(color)

	return worktreeUiBranding{
		Enabled:     true,
		Name:        name,
		Color:       color,
		TextColor:   textColor,
		FaviconHref: createFaviconDataURL(color, textColor),
	}
}

func renderFaviconLinks(branding worktreeUiBranding) string {
	if !branding.Enabled || branding.FaviconHref == "" {
		return defaultFaviconLinks
	}
	href := escapeHTMLAttr(branding.FaviconHref)
	return fmt.Sprintf(`<link rel="icon" href="%s" type="image/svg+xml" sizes="any" />
<link rel="shortcut icon" href="%s" type="image/svg+xml" />`, href, href)
}

func renderRuntimeBrandingMeta(branding worktreeUiBranding) string {
	var parts []string

	// In ui-only mode (or any mode) the operator may set PAPERCLIP_API_BASE_URL
	// to tell the frontend where the backend API is hosted.  The value is
	// injected as a <meta> tag so that the pre-built SPA can read it at runtime
	// without requiring a rebuild.
	if apiBase := nonEmpty(os.Getenv("PAPERCLIP_API_BASE_URL")); apiBase != "" {
		normalized := strings.TrimRight(apiBase, "/")
		parts = append(parts, fmt.Sprintf(
			`<meta name="paperclip-api-base-url" content="%s" />`,
			escapeHTMLAttr(normalized),
		))
	}

	if branding.Enabled && branding.Name != "" && branding.Color != "" && branding.TextColor != "" {
		parts = append(parts, fmt.Sprintf(
			`<meta name="paperclip-worktree-enabled" content="true" />
<meta name="paperclip-worktree-name" content="%s" />
<meta name="paperclip-worktree-color" content="%s" />
<meta name="paperclip-worktree-text-color" content="%s" />`,
			escapeHTMLAttr(branding.Name),
			escapeHTMLAttr(branding.Color),
			escapeHTMLAttr(branding.TextColor),
		))
	}

	return strings.Join(parts, "\n")
}

func replaceMarkedBlock(html, startMarker, endMarker, content string) string {
	start := strings.Index(html, startMarker)
	end := strings.Index(html, endMarker)
	if start == -1 || end == -1 || end < start {
		return html
	}
	before := html[:start+len(startMarker)]
	after := html[end:]

	var indented string
	if content != "" {
		lines := strings.Split(content, "\n")
		paddedLines := make([]string, len(lines))
		for i, line := range lines {
			paddedLines[i] = "    " + line
		}
		indented = "\n" + strings.Join(paddedLines, "\n") + "\n    "
	} else {
		indented = "\n    "
	}
	return before + indented + after
}

// ApplyUIBranding applies worktree-specific favicon and runtime branding meta
// tags to an index.html byte slice, matching the Node.js applyUiBranding logic.
func ApplyUIBranding(html []byte) []byte {
	branding := getWorktreeUiBranding()
	result := replaceMarkedBlock(string(html), faviconBlockStart, faviconBlockEnd, renderFaviconLinks(branding))
	result = replaceMarkedBlock(result, runtimeBrandingBlockStart, runtimeBrandingBlockEnd, renderRuntimeBrandingMeta(branding))
	return []byte(result)
}
