package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Worktree utility commands",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[TODO] Worktree mutation parser to Go AST mapping.")
	},
}

func init() {
	rootCmd.AddCommand(worktreeCmd)
}
