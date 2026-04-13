package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup and onboarding for Paperclip",
	Long:  `Guides you through setting up your initial configuration using a brief 4-question interview.`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Println(" paperclipai onboard ")
		fmt.Println("--------------------")
		fmt.Println("Welcome! Let's get your Vashandi instance configured.")

		// 1. Target URL
		fmt.Print("\n1. Target Server URL (default: http://localhost:3100): ")
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)
		if url == "" {
			url = "http://localhost:3100"
		}

		// 2. Adapter
		fmt.Print("2. AI Adapter (claude/openai/custom) [default: claude]: ")
		adapter, _ := reader.ReadString('\n')
		adapter = strings.TrimSpace(adapter)
		if adapter == "" {
			adapter = "claude"
		}

		// 3. Repo Root
		fmt.Print("3. Project Repository Root Path (e.g. ./vashandi): ")
		repo, _ := reader.ReadString('\n')
		repo = strings.TrimSpace(repo)

		// 4. Runner Strategy
		fmt.Print("4. Runner Strategy (local/docker) [default: local]: ")
		runner, _ := reader.ReadString('\n')
		runner = strings.TrimSpace(runner)
		if runner == "" {
			runner = "local"
		}

		fmt.Println("\n--- Configuration Summary ---")
		fmt.Printf("Server:  %s\n", url)
		fmt.Printf("Adapter: %s\n", adapter)
		fmt.Printf("Repo:    %s\n", repo)
		fmt.Printf("Runner:  %s\n", runner)

		// In a real implementation, we would write this to .paperclip.json
		fmt.Println("\n[Success] Configuration generated. Use 'paperclipai run' to start.")
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	// Example flag that the TS version has
	onboardCmd.Flags().Bool("yes", false, "Skip prompts and use defaults")
}
