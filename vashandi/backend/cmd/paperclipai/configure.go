package main

import (
"encoding/json"
"fmt"
"os"
"path/filepath"

"github.com/spf13/cobra"
)

type Config struct {
ServerURL       string   `json:"serverUrl"`
APIKey          string   `json:"apiKey"`
DefaultCompany  string   `json:"defaultCompany"`
AllowedHostnames []string `json:"allowedHostnames"`
}

func configPath() string {
home, _ := os.UserHomeDir()
return filepath.Join(home, ".paperclip", "config.json")
}

func loadConfig() Config {
var cfg Config
data, err := os.ReadFile(configPath())
if err != nil {
return cfg
}
json.Unmarshal(data, &cfg)
return cfg
}

func saveConfig(cfg Config) error {
p := configPath()
if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
return err
}
data, err := json.MarshalIndent(cfg, "", "  ")
if err != nil {
return err
}
return os.WriteFile(p, data, 0600)
}

var configureCmd = &cobra.Command{
Use:   "configure",
Short: "Update configuration sections",
RunE: func(cmd *cobra.Command, args []string) error {
section, _ := cmd.Flags().GetString("section")
cfg := loadConfig()

prompt := func(label, current string) string {
if current != "" {
fmt.Printf("%s [%s]: ", label, current)
} else {
fmt.Printf("%s: ", label)
}
var val string
fmt.Scan(&val)
if val == "" {
return current
}
return val
}

switch section {
case "server", "":
cfg.ServerURL = prompt("Server URL", cfg.ServerURL)
cfg.APIKey = prompt("API Key", cfg.APIKey)
cfg.DefaultCompany = prompt("Default Company ID", cfg.DefaultCompany)
default:
fmt.Printf("Unknown section: %s\n", section)
return nil
}

if err := saveConfig(cfg); err != nil {
return fmt.Errorf("failed to save config: %w", err)
}
fmt.Println("Configuration saved.")
return nil
},
}

func init() {
configureCmd.Flags().StringP("section", "s", "", "Section to configure (server)")
rootCmd.AddCommand(configureCmd)
}
