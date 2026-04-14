package server

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, message string, code ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errResp := APIError{Error: message}
	if len(code) > 0 {
		errResp.Code = code[0]
	}
	json.NewEncoder(w).Encode(errResp) //nolint:errcheck
}
