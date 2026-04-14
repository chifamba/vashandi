package main

import (
"context"
"encoding/json"
"fmt"
"os"

"github.com/chifamba/vashandi/vashandi/backend/client"
"github.com/spf13/cobra"
)

var routinesCmd = &cobra.Command{
Use:   "routines",
Short: "Routine management commands",
}

var routinesListCmd = &cobra.Command{
Use:   "list",
Short: "List routines for a company",
RunE: func(cmd *cobra.Command, args []string) error {
companyID, _ := cmd.Flags().GetString("company")
if companyID == "" {
cfg := loadConfig()
companyID = cfg.DefaultCompany
}
if companyID == "" {
return fmt.Errorf("--company is required (or set defaultCompany in config)")
}
apiKey := os.Getenv("PAPERCLIP_AGENT_JWT_SECRET")
cfg := loadConfig()
baseURL := cfg.ServerURL
c := client.NewClient(baseURL, apiKey)
var result interface{}
if err := c.DoReq(context.Background(), "GET", "/api/v1/companies/"+companyID+"/routines", nil, &result); err != nil {
return err
}
out, _ := json.MarshalIndent(result, "", "  ")
fmt.Println(string(out))
return nil
},
}

func init() {
routinesListCmd.Flags().StringP("company", "c", "", "Company ID")
routinesCmd.AddCommand(routinesListCmd)
rootCmd.AddCommand(routinesCmd)
}
