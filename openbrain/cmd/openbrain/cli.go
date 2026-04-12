package main
import (
	"fmt"
	"io"
	"net/http"
	"os"
	"github.com/spf13/cobra"
)
var cliCmd = &cobra.Command{Use: "openbrain"}
var memoryCmd = &cobra.Command{Use: "memory"}
var memoryListCmd = &cobra.Command{
	Use: "list",
	Run: func(cmd *cobra.Command, args []string) {
		namespace, _ := cmd.Flags().GetString("namespace")
		resp, err := http.Get(fmt.Sprintf("http://localhost:3101/v1/namespaces/%s/proposals", namespace))
		if err != nil { return }
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
	},
}
var auditExportCmd = &cobra.Command{
	Use: "audit export",
	Run: func(cmd *cobra.Command, args []string) {
		format, _ := cmd.Flags().GetString("format")
		out, _ := cmd.Flags().GetString("out")
		fmt.Printf("Exporting audit log in %s format to %s\n", format, out)
		os.WriteFile(out, []byte(fmt.Sprintf(`{"exported_format": "%s"}`, format)), 0644)
	},
}
func init() {
	memoryListCmd.Flags().String("namespace", "", "Namespace ID")
	auditExportCmd.Flags().String("format", "jsonld", "Export format")
	auditExportCmd.Flags().String("out", "./audit.jsonld", "Output file")
	memoryCmd.AddCommand(memoryListCmd)
	cliCmd.AddCommand(memoryCmd)
	cliCmd.AddCommand(auditExportCmd)
}
func Execute() {
	if err := cliCmd.Execute(); err != nil { os.Exit(1) }
}
