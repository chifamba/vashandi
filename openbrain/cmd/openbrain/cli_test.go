package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_AuditExport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/audit/export", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"exported_format":"json"}`))
	}))
	defer server.Close()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	cmd := NewRootCommand(nil)
	cmd.SetOut(w)
	cmd.SetErr(w)
	cmd.SetArgs([]string{"--base-url", server.URL, "--token", "dev_secret_token", "audit", "export", "--namespace", "ns1", "--format", "json", "--out", "./test_audit.json"})
	require.NoError(t, cmd.Execute())
	w.Close()

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	assert.Contains(t, buf.String(), "Exporting audit log in json format to ./test_audit.json")
	content, err := os.ReadFile("./test_audit.json")
	require.NoError(t, err)
	assert.Contains(t, string(content), `"exported_format":"json"`)
	_ = os.Remove("./test_audit.json")
}
