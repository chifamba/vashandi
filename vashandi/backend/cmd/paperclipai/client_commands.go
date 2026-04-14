package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chifamba/vashandi/vashandi/backend/client"
	"github.com/spf13/cobra"
)

func init() {
	clientNamespaces := []string{
		"company", 
		"issue", 
		"agent", 
		"approval", 
		"activity", 
		"dashboard", 
		"plugin", 
		"context",
	}

	for _, name := range clientNamespaces {
		nsName := name // capture loop variable
		cmd := &cobra.Command{
			Use:   nsName,
			Short: fmt.Sprintf("%s client commands", nsName),
			Run: func(c *cobra.Command, args []string) {
				apiKey := os.Getenv("PAPERCLIP_AGENT_JWT_SECRET")
				apiClient := client.NewClient("", apiKey)

				if nsName == "company" {
					fmt.Println("Querying /api/v1/companies...")
					res, err := apiClient.GetCompany(context.Background(), "default-company")
					if err != nil {
						fmt.Printf("API error: %v\n", err)
						return
					}
					fmt.Printf("Success! Received Company metadata: %v\n", res)
					return
				}

				if nsName == "issue" {
					fmt.Println("Patching /api/v1/issues...")
					res, err := apiClient.UpdateIssueStatus(context.Background(), "default-issue", "resolved")
					if err != nil {
						fmt.Printf("API error: %v\n", err)
						return
					}
					fmt.Printf("Success! Updated Issue: %v\n", res)
					return
				}

				fmt.Printf("[%s] client proxy logic implementation pending.\n", nsName)
			},
		}
		rootCmd.AddCommand(cmd)
	}
}
