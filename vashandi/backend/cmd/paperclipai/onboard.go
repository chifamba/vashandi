package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup and onboarding for Paperclip",
	Long:  `Guides you through setting up your initial configuration and environment for a new Paperclip instance.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(" paperclipai onboard ")
		fmt.Println("--------------------")
		// Temporary stub. Actual implementation requires interactive prompts (survey/huh) and configuration scaffolding.
		fmt.Println("Welcome to Paperclip! This interactive onboarding will guide you through setup.")
		fmt.Println("(Stub: Configuration setup process is pending porting of config package)")
		fmt.Println("Seeding initial brain.md to OpenBrain namespace...")
		// Mock request to ingest brain.md
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	// Example flag that the TS version has
	onboardCmd.Flags().Bool("yes", false, "Skip prompts and use defaults")
}
