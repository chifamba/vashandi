package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print environment variables for deployment",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(" paperclip env ")
		
		vars := []struct {
			key      string
			defaultV string
			required bool
			note     string
		}{
			{"DATABASE_URL", "", true, "Required for live deployment with managed PostgreSQL"},
			{"PAPERCLIP_AGENT_JWT_SECRET", "", true, "Required for local adapter authentication"},
			{"PORT", "3100", false, "HTTP listen port"},
			{"PAPERCLIP_PUBLIC_URL", "", false, "Canonical public URL for auth/callback/invite origin wiring"},
			{"PAPERCLIP_DEBUG", "false", false, "Enable verbose logging debugging mode"},
		}

		fmt.Println("\nRequired environment variables")
		for _, v := range vars {
			if v.required {
				val := os.Getenv(v.key)
				status := "set    "
				if val == "" {
					status = "missing"
					val = "<set-this-value>"
				}
				fmt.Printf("%s %s [env] %s => '%s'\n", v.key, status, v.note, val)
			}
		}

		fmt.Println("\nOptional environment variables")
		for _, v := range vars {
			if !v.required {
				val := os.Getenv(v.key)
				status := "set    "
				if val == "" {
					status = "default"
					val = v.defaultV
				}
				fmt.Printf("%s %s [env/default] %s => '%s'\n", v.key, status, v.note, val)
			}
		}

		fmt.Println("\nDeployment export block")
		for _, v := range vars {
			val := os.Getenv(v.key)
			if val == "" {
				if v.required {
					val = "<set-this-value>"
				} else {
					val = v.defaultV
				}
			}
			fmt.Printf("export %s='%s'\n", v.key, val)
		}
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
