package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Paperclip server",
	Long:  `Starts the Paperclip background server, connecting to the configured database and listening on the specified port.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(" paperclipai run ")
		fmt.Println("-----------------")
		// Temporary stub for server boot. When porting config logic is ready, this will initialize `backend/server.SetupRouter` and `http.ListenAndServe`.
		fmt.Println("Starting Paperclip Server... (Stubbed)")
		fmt.Println("Listening on http://127.0.0.1:3100 (Stubbed)")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().String("config", "", "Path to paperclip config directory")
}
