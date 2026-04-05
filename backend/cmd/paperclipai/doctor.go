package main

import (
	"fmt"
	"os"

	"github.com/chifamba/paperclip/backend/shared"
	"github.com/spf13/cobra"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and configurations",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Running paperclip doctor...")

		configPath, _ := cmd.Flags().GetString("config")
		if configPath == "" {
			configPath = "paperclip.yaml"
		}

		passed := 0
		failed := 0

		// 1. Config Check
		fmt.Print("Checking Config file... ")
		config, err := shared.ReadConfig(configPath)
		if err != nil {
			fmt.Printf("✘ FAILED (%v)\n", err)
			failed++
		} else {
			fmt.Println("✓ OK")
			passed++

			// 2. Database Check
			fmt.Print("Checking Database... ")
			if config.Database.Mode == "postgres" {
				db, err := gorm.Open(postgres.Open(config.Database.ConnectionString), &gorm.Config{})
				if err != nil {
					fmt.Printf("✘ FAILED (%v)\n", err)
					failed++
				} else {
					sqlDB, _ := db.DB()
					if err := sqlDB.Ping(); err != nil {
						fmt.Printf("✘ FAILED (%v)\n", err)
						failed++
					} else {
						fmt.Println("✓ OK")
						passed++
					}
				}
			} else {
				fmt.Println("✓ OK (Embedded)")
				passed++
			}

			// 3. LLM Check
			fmt.Print("Checking LLM... ")
			if config.Llm != nil && config.Llm.ApiKey != "" {
				fmt.Println("✓ OK")
				passed++
			} else {
				fmt.Println("! WARNING (No LLM API Key)")
			}
		}

		fmt.Printf("\nSummary: %d passed, %d failed\n", passed, failed)

		if failed > 0 {
			os.Exit(1)
		}
	},
}

func init() {
	doctorCmd.Flags().StringP("config", "c", "", "Path to the configuration file")
	doctorCmd.Flags().Bool("repair", false, "Attempt to automatically repair failed checks")
	doctorCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts when repairing")
}
