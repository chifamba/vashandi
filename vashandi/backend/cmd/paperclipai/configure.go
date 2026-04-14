package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Update configuration sections",
	Run: func(cmd *cobra.Command, args []string) {
		section, _ := cmd.Flags().GetString("section")
		fmt.Printf("Configuring section: %s\n", section)
		fmt.Println("[TODO] configure command logic migration pending deep UI port to Go.")
	},
}

func init() {
	configureCmd.Flags().StringP("section", "s", "", "Section to configure (llm, database, logging, server, storage, secrets)")
	rootCmd.AddCommand(configureCmd)
}
