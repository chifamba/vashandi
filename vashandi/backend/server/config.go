package server

import (
	"os"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

// In a real port, this would read from the file. For now, we mock the default config.
func LoadConfig() (*shared.PaperclipConfig, error) {
	return &shared.PaperclipConfig{
		Server: shared.ServerConfig{
			Port: 3100,
			Host: "127.0.0.1",
		},
		Auth: shared.AuthConfig{
			DisableSignUp: false,
		},
	}, nil
}

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
