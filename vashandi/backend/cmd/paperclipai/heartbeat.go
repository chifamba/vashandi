package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chifamba/vashandi/vashandi/backend/adapters/anthropic"
	"github.com/spf13/cobra"
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Heartbeat utilities",
}

var heartbeatRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run one agent heartbeat and stream live logs",
	Run: func(cmd *cobra.Command, args []string) {
		agentId, _ := cmd.Flags().GetString("agent-id")
		adapterName := "claude" // Derived from agent-id context in a full port

		if adapterName == "openai" || adapterName == "gemini" {
			panic(fmt.Sprintf("Provider %s not yet migrated to Go", adapterName))
		}

		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		runner := anthropic.NewRunner(apiKey)
		err := runner.ExecuteRun(context.Background(), agentId, "mocked_cli_context")
		if err != nil {
			fmt.Printf("Agent Run failed: %v\n", err)
		}
	},
}

func init() {
	heartbeatRunCmd.Flags().StringP("agent-id", "a", "", "Agent ID to invoke")
	heartbeatRunCmd.Flags().String("context", "", "Path to CLI context file")
	heartbeatRunCmd.Flags().String("profile", "", "CLI context profile name")
	heartbeatRunCmd.Flags().String("api-base", "", "Base URL for the Paperclip server API")
	heartbeatRunCmd.Flags().String("api-key", "", "Bearer token for agent-authenticated calls")
	heartbeatRunCmd.Flags().String("source", "on_demand", "Invocation source (timer | assignment | on_demand | automation)")
	heartbeatRunCmd.Flags().String("trigger", "manual", "Trigger detail (manual | ping | callback | system)")
	heartbeatRunCmd.Flags().String("timeout-ms", "0", "Max time to wait before giving up")
	heartbeatRunCmd.Flags().Bool("json", false, "Output raw JSON where applicable")
	heartbeatRunCmd.Flags().Bool("debug", false, "Show raw adapter stdout/stderr JSON chunks")
	
	heartbeatRunCmd.MarkFlagRequired("agent-id")
	
	heartbeatCmd.AddCommand(heartbeatRunCmd)
	rootCmd.AddCommand(heartbeatCmd)
}
