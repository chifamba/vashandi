package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var dbBackupCmd = &cobra.Command{
	Use:   "db:backup",
	Short: "Create a one-off database backup using current config",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[TODO] postgres pg_dump wrapper execution pending complete Go migration.")
	},
}

func init() {
	dbBackupCmd.Flags().String("dir", "", "Backup output directory (overrides config)")
	dbBackupCmd.Flags().Int("retention-days", 0, "Retention window used for pruning")
	dbBackupCmd.Flags().String("filename-prefix", "paperclip", "Backup filename prefix")
	dbBackupCmd.Flags().Bool("json", false, "Print backup metadata as JSON")
	rootCmd.AddCommand(dbBackupCmd)
}
