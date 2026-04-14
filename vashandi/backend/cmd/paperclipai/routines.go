package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var routinesCmd = &cobra.Command{
	Use:   "routines",
	Short: "Routine utilities",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[TODO] Go routine framework binding logic.")
	},
}

func init() {
	rootCmd.AddCommand(routinesCmd)
}
