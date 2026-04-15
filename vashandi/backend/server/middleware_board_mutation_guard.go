package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

var boardMutationSafeMethods = map[string]struct{}{
	"GET":     {},
	"HEAD":    {},
	"OPTIONS": {},
}

var boardMutationDefaultDevOrigins = []string{
	"http://localhost:3100",
	"http://127.0.0.1:3100",
}

// parseBoardMutationOrigin extracts the scheme+host from a header value (Origin or Referer).
// Returns an empty string if the value is empty or cannot be parsed.
func parseBoardMutationOrigin(value string) string {
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// trustedBoardMutationOrigins builds the set of allowed origins for a request.
// It always includes the default dev origins, plus http/https variants of the
// request host (preferring X-Forwarded-Host when present).
func trustedBoardMutationOrigins(r *http.Request) map[string]struct{} {
	origins := make(map[string]struct{}, len(boardMutationDefaultDevOrigins)+2)
	for _, o := range boardMutationDefaultDevOrigins {
		origins[strings.ToLower(o)] = struct{}{}
	}

	forwardedHost := ""
	if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
		parts := strings.SplitN(fh, ",", 2)
		forwardedHost = strings.TrimSpace(parts[0])
	}
	host := forwardedHost
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if host != "" {
		origins["http://"+strings.ToLower(host)] = struct{}{}
		origins["https://"+strings.ToLower(host)] = struct{}{}
	}
	return origins
}

// isTrustedBoardMutation returns true when the request's Origin or Referer
// header resolves to one of the trusted origins for this request.
func isTrustedBoardMutation(r *http.Request) bool {
	allowed := trustedBoardMutationOrigins(r)

	if origin := parseBoardMutationOrigin(r.Header.Get("Origin")); origin != "" {
		if _, ok := allowed[origin]; ok {
			return true
		}
	}
	if refOrigin := parseBoardMutationOrigin(r.Header.Get("Referer")); refOrigin != "" {
		if _, ok := allowed[refOrigin]; ok {
			return true
		}
	}
	return false
}

// BoardMutationGuard blocks board session actors from making mutating requests
// without a trusted browser Origin or Referer header.  This guards against
// CSRF when board users are authenticated via cookie sessions.
//
// The guard is skipped for:
//   - Safe HTTP methods (GET, HEAD, OPTIONS)
//   - Non-board actors (agents, anonymous)
//   - Board actors authenticated via local_implicit or board_key sources
func BoardMutationGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, safe := boardMutationSafeMethods[strings.ToUpper(r.Method)]; safe {
			next.ServeHTTP(w, r)
			return
		}

		actor := routes.GetActorInfo(r)
		if actor.ActorType != "board" {
			next.ServeHTTP(w, r)
			return
		}

		// Board actors authenticated outside of a browser session are not subject
		// to CSRF checks — they cannot carry browser-set credentials.
		if actor.ActorSource == "local_implicit" || actor.ActorSource == "board_key" {
			next.ServeHTTP(w, r)
			return
		}

		if !isTrustedBoardMutation(r) {
			WriteError(w, http.StatusForbidden, "Board mutation requires trusted browser origin")
			return
		}

		next.ServeHTTP(w, r)
	})
}
