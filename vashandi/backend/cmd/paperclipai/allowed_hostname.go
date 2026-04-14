package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var allowedHostnameCmd = &cobra.Command{
	Use:   "allowed-hostname [host]",
	Short: "Allow a hostname for authenticated/private mode access",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		host := args[0]
		fmt.Printf("Adding allowed hostname: %s\n", host)
		fmt.Println("[TODO] Allowed hostname database mutation pending parity.")
	},
}

func init() {
	rootCmd.AddCommand(allowedHostnameCmd)
}
