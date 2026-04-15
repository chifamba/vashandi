package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/chifamba/vashandi/vashandi/backend/server"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Paperclip server",
	Long:  `Starts the Paperclip background server, connecting to the configured database and listening on the specified port.`,
	Run: func(cmd *cobra.Command, args []string) {
		configDir, _ := cmd.Flags().GetString("config")
		if configDir != "" {
			if err := os.Setenv("PAPERCLIP_HOME", configDir); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to set PAPERCLIP_HOME: %v\n", err)
				os.Exit(1)
			}
		}
		server.Run()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().String("config", "", "Path to paperclip config directory")
}
