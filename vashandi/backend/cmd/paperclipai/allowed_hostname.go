package main

import (
"fmt"

"github.com/spf13/cobra"
)

var allowedHostnameCmd = &cobra.Command{
Use:   "allowed-hostname",
Short: "Manage allowed hostnames for private mode access",
}

var allowedHostnameListCmd = &cobra.Command{
Use:   "list",
Short: "List allowed hostnames",
RunE: func(cmd *cobra.Command, args []string) error {
cfg := loadConfig()
if len(cfg.AllowedHostnames) == 0 {
fmt.Println("No allowed hostnames configured.")
return nil
}
for _, h := range cfg.AllowedHostnames {
fmt.Println(h)
}
return nil
},
}

var allowedHostnameAddCmd = &cobra.Command{
Use:   "add <hostname>",
Short: "Add an allowed hostname",
Args:  cobra.ExactArgs(1),
RunE: func(cmd *cobra.Command, args []string) error {
host := args[0]
cfg := loadConfig()
for _, h := range cfg.AllowedHostnames {
if h == host {
fmt.Printf("Hostname already allowed: %s\n", host)
return nil
}
}
cfg.AllowedHostnames = append(cfg.AllowedHostnames, host)
if err := saveConfig(cfg); err != nil {
return err
}
fmt.Printf("Added allowed hostname: %s\n", host)
return nil
},
}

var allowedHostnameRemoveCmd = &cobra.Command{
Use:   "remove <hostname>",
Short: "Remove an allowed hostname",
Args:  cobra.ExactArgs(1),
RunE: func(cmd *cobra.Command, args []string) error {
host := args[0]
cfg := loadConfig()
newList := make([]string, 0, len(cfg.AllowedHostnames))
found := false
for _, h := range cfg.AllowedHostnames {
if h == host {
found = true
continue
}
newList = append(newList, h)
}
if !found {
fmt.Printf("Hostname not found: %s\n", host)
return nil
}
cfg.AllowedHostnames = newList
if err := saveConfig(cfg); err != nil {
return err
}
fmt.Printf("Removed allowed hostname: %s\n", host)
return nil
},
}

func init() {
allowedHostnameCmd.AddCommand(allowedHostnameListCmd, allowedHostnameAddCmd, allowedHostnameRemoveCmd)
rootCmd.AddCommand(allowedHostnameCmd)
}
