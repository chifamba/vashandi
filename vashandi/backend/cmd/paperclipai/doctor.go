package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check your system for dependencies and potential issues",
	Long:  `Runs a suite of diagnostic checks on your environment, configuration, and dependencies.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(" paperclip doctor ")
		fmt.Println("--------------------")
		// Temporary stub. To be fully implemented once shared configuration loading logic is ported.
		fmt.Println("Config file: (stub) OK")
		fmt.Println("Deployment/auth mode: (stub) OK")
		fmt.Println("Agent JWT: (stub) OK")
		fmt.Println("Secrets adapter: (stub) OK")
		fmt.Println("Storage: (stub) OK")
		fmt.Println("Database: (stub) OK")
		fmt.Println("LLM: (stub) OK")
		fmt.Println("Log directory: (stub) OK")
		fmt.Println("Port: (stub) OK")
		fmt.Println("--------------------")
		fmt.Println("All checks passed! (Stubbed)")
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
