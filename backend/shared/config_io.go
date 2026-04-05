package shared

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WriteConfig writes the PaperclipConfig to a file in YAML format
func WriteConfig(config *PaperclipConfig, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ReadConfig reads the PaperclipConfig from a YAML file
func ReadConfig(path string) (*PaperclipConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config PaperclipConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}
