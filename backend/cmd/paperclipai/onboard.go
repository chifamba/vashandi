package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chifamba/paperclip/backend/shared"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Interactive setup and onboarding for Paperclip",
	Long:  `Guides you through setting up your initial configuration and environment for a new Paperclip instance.`,
	Run: func(cmd *cobra.Command, args []string) {
		var (
			companyName string
			llmProvider string
			llmApiKey   string
			dbMode      string
		)

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Company Name").
					Value(&companyName).
					Placeholder("e.g. Acme Corp"),

				huh.NewSelect[string]().
					Title("LLM Provider").
					Options(
						huh.NewOption("Claude", "claude"),
						huh.NewOption("OpenAI", "openai"),
					).
					Value(&llmProvider),

				huh.NewInput().
					Title("LLM API Key").
					Value(&llmApiKey).
					Password(true),

				huh.NewSelect[string]().
					Title("Database Mode").
					Options(
						huh.NewOption("Embedded Postgres (Easy)", "embedded-postgres"),
						huh.NewOption("External Postgres", "postgres"),
					).
					Value(&dbMode),
			),
		)

		err := form.Run()
		if err != nil {
			log.Fatal(err)
		}

		config := &shared.PaperclipConfig{
			Meta: shared.ConfigMeta{
				Version:   1,
				UpdatedAt: time.Now(),
				Source:    "onboard",
			},
			Llm: &shared.LlmConfig{
				Provider: llmProvider,
				ApiKey:   llmApiKey,
			},
			Database: shared.DatabaseConfig{
				Mode:                 dbMode,
				EmbeddedPostgresPort: 54329,
				EmbeddedPostgresDataDir: "data/postgres",
			},
			Logging: shared.LoggingConfig{
				Mode: "file",
				LogDir: "logs",
			},
			Server: shared.ServerConfig{
				DeploymentMode: "local_trusted",
				Exposure:       "private",
				Host:           "127.0.0.1",
				Port:           3100,
				ServeUi:        true,
			},
			Auth: shared.AuthConfig{
				BaseUrlMode: "auto",
			},
			Storage: shared.StorageConfig{
				Provider: "local_disk",
				LocalDisk: shared.StorageLocalDiskConfig{
					BaseDir: "data/storage",
				},
			},
			Secrets: shared.SecretsConfig{
				Provider: "local_encrypted",
				LocalEncrypted: shared.SecretsLocalEncryptedConfig{
					KeyFilePath: "data/secrets.key",
				},
			},
		}

		err = shared.WriteConfig(config, "paperclip.yaml")
		if err != nil {
			fmt.Printf("Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("\nConfiguration saved to paperclip.yaml")
		fmt.Println("Next step: paperclipai run")
	},
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	// Example flag that the TS version has
	onboardCmd.Flags().Bool("yes", false, "Skip prompts and use defaults")
}
