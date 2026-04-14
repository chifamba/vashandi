package main

import (
"fmt"
"os"
"os/exec"

"github.com/spf13/cobra"
)

var worktreeCmd = &cobra.Command{
Use:   "worktree",
Short: "Worktree utility commands",
}

var worktreeCreateCmd = &cobra.Command{
Use:   "create",
Short: "Create a new git worktree",
RunE: func(cmd *cobra.Command, args []string) error {
branch, _ := cmd.Flags().GetString("branch")
if branch == "" {
return fmt.Errorf("--branch is required")
}
path := ".worktrees/" + branch
c := exec.Command("git", "worktree", "add", path, "-b", branch)
c.Stdout = os.Stdout
c.Stderr = os.Stderr
return c.Run()
},
}

var worktreeListCmd = &cobra.Command{
Use:   "list",
Short: "List git worktrees",
RunE: func(cmd *cobra.Command, args []string) error {
c := exec.Command("git", "worktree", "list", "--porcelain")
c.Stdout = os.Stdout
c.Stderr = os.Stderr
return c.Run()
},
}

var worktreeMergeCmd = &cobra.Command{
Use:   "merge",
Short: "Merge a worktree branch into the current branch",
RunE: func(cmd *cobra.Command, args []string) error {
branch, _ := cmd.Flags().GetString("branch")
if branch == "" {
return fmt.Errorf("--branch is required")
}
c := exec.Command("git", "merge", branch)
c.Stdout = os.Stdout
c.Stderr = os.Stderr
return c.Run()
},
}

var worktreeCleanupCmd = &cobra.Command{
Use:   "cleanup",
Short: "Prune stale git worktree entries",
RunE: func(cmd *cobra.Command, args []string) error {
c := exec.Command("git", "worktree", "prune")
c.Stdout = os.Stdout
c.Stderr = os.Stderr
return c.Run()
},
}

func init() {
worktreeCreateCmd.Flags().StringP("branch", "b", "", "Branch name")
worktreeMergeCmd.Flags().StringP("branch", "b", "", "Branch name to merge")
worktreeCmd.AddCommand(worktreeCreateCmd, worktreeListCmd, worktreeMergeCmd, worktreeCleanupCmd)
rootCmd.AddCommand(worktreeCmd)
}
