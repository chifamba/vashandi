package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ResolvePrivateHostnameAllowSet builds the complete set of allowed hostnames
// from the configured list and the bind host.  Loopback addresses are always
// allowed and do not need to appear here.
func ResolvePrivateHostnameAllowSet(allowedHostnames []string, bindHost string) map[string]struct{} {
	allow := make(map[string]struct{})
	for _, h := range allowedHostnames {
		trimmed := strings.TrimSpace(strings.ToLower(h))
		if trimmed != "" {
			allow[trimmed] = struct{}{}
		}
	}
	bh := strings.TrimSpace(strings.ToLower(bindHost))
	if bh != "" && bh != "0.0.0.0" {
		allow[bh] = struct{}{}
	}
	return allow
}

// isLoopbackHostname returns true for well-known loopback hostnames.
func isLoopbackHostname(hostname string) bool {
	switch strings.TrimSpace(strings.ToLower(hostname)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// extractHostname returns the effective hostname from X-Forwarded-Host or Host.
// Port numbers are stripped.  Returns an empty string when neither header is set.
func extractHostname(r *http.Request) string {
	raw := ""
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		parts := strings.SplitN(fh, ",", 2)
		raw = strings.TrimSpace(parts[0])
	}
	if raw == "" {
		raw = strings.TrimSpace(r.Host)
	}
	if raw == "" {
		return ""
	}
	// Strip port by parsing as a URL; fall back to the raw value if parsing fails.
	parsed, err := url.Parse("http://" + raw)
	if err != nil || parsed.Hostname() == "" {
		return strings.ToLower(raw)
	}
	return strings.ToLower(parsed.Hostname())
}

// blockedHostnameMessage returns the user-facing error for a blocked hostname.
func blockedHostnameMessage(hostname string) string {
	return fmt.Sprintf(
		"Hostname '%s' is not allowed for this Paperclip instance. "+
			"If you want to allow this hostname, please run pnpm paperclipai allowed-hostname %s",
		hostname, hostname,
	)
}

// PrivateHostnameGuardOptions configures PrivateHostnameGuard.
type PrivateHostnameGuardOptions struct {
	// Enabled controls whether the guard is active.  When false the middleware
	// is a no-op pass-through.
	Enabled bool

	// AllowedHostnames is the operator-configured list of permitted hostnames.
	AllowedHostnames []string

	// BindHost is the host the server is bound to; it is automatically added to
	// the allow set when it is non-empty and not "0.0.0.0".
	BindHost string
}

// PrivateHostnameGuard blocks requests whose hostname is not in the allow set.
// It is intended for "authenticated" deployments with "private" exposure where
// the operator controls which hostnames clients may use to reach the server.
//
// Loopback hostnames (localhost, 127.0.0.1, ::1) are always permitted.
// API paths respond with JSON errors; all other paths respond with plain text.
func PrivateHostnameGuard(opts PrivateHostnameGuardOptions) func(http.Handler) http.Handler {
	if !opts.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	allowSet := ResolvePrivateHostnameAllowSet(opts.AllowedHostnames, opts.BindHost)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hostname := extractHostname(r)
			wantsJSON := strings.HasPrefix(r.URL.Path, "/api") ||
				strings.Contains(r.Header.Get("Accept"), "application/json")

			if hostname == "" {
				const errMsg = "Missing Host header. If you want to allow a hostname, " +
					"run pnpm paperclipai allowed-hostname <host>."
				writeHostnameError(w, http.StatusForbidden, errMsg, wantsJSON)
				return
			}

			if isLoopbackHostname(hostname) {
				next.ServeHTTP(w, r)
				return
			}

			if _, ok := allowSet[hostname]; ok {
				next.ServeHTTP(w, r)
				return
			}

			writeHostnameError(w, http.StatusForbidden, blockedHostnameMessage(hostname), wantsJSON)
		})
	}
}

func writeHostnameError(w http.ResponseWriter, status int, message string, asJSON bool) {
	if asJSON {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, message) //nolint:errcheck
}
