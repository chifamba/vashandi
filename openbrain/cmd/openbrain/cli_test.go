package main
import (
	"bytes"
	"io"
	"os"
	"testing"
	"github.com/stretchr/testify/assert"
)
func TestCLI_AuditExport(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Args = []string{"openbrain", "audit", "export", "--format", "json", "--out", "./test_audit.json"}
	defer func() { os.Stdout = oldStdout }()
	if err := cliCmd.Execute(); err != nil { t.Fatal(err) }
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	assert.Contains(t, buf.String(), "Exporting audit log in json format to ./test_audit.json")
	content, _ := os.ReadFile("./test_audit.json")
	assert.Contains(t, string(content), `"exported_format": "json"`)
	os.Remove("./test_audit.json")
}
