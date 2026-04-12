package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

func TestAuthMiddleware(t *testing.T) {
	token, err := brain.SignScopedToken(brain.ScopedTokenClaims{NamespaceID: "ns-1", AgentID: "agent-1", TrustTier: 2, ActorKind: "agent"})
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(AuthMiddleware)
	r.Get("/namespaces/{namespaceId}", func(w http.ResponseWriter, r *http.Request) {
		if !maybeNamespaceAuthorized(w, r, chi.URLParam(r, "namespaceId")) {
			return
		}
		writeJSON(w, http.StatusOK, actorFromRequest(r))
	})

	req := httptest.NewRequest(http.MethodGet, "/namespaces/ns-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	assert.Equal(t, http.StatusOK, res.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/namespaces/ns-2", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	res2 := httptest.NewRecorder()
	r.ServeHTTP(res2, req2)
	assert.Equal(t, http.StatusForbidden, res2.Code)
}
