package drive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	ConfigDir  = "/var/lib/mirrorvault"
	ConfigPath = "/var/lib/mirrorvault/drive_config.json"
)

type Config struct {
	Enabled      bool   `json:"enabled"`
	Provider     string `json:"provider"`
	FolderID     string `json:"folder_id"`
	FolderName   string `json:"folder_name"`
	AccountEmail string `json:"account_email"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthMethod   string `json:"auth_method"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
	ConnectedAt  string `json:"connected_at"`
	SourcePath   string `json:"-"`
	Loaded       bool   `json:"-"`
}

func LoadConfig() (*Config, error) {
	loadAt := func(path string) (*Config, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse drive config: %w", err)
		}
		if cfg.Provider == "" {
			cfg.Provider = "google_drive"
		}
		cfg.SourcePath = path
		cfg.Loaded = true
		return &cfg, nil
	}

	var sysErr error
	if _, err := os.Stat(ConfigPath); err == nil {
		if cfg, err := loadAt(ConfigPath); err == nil {
			return cfg, nil
		} else {
			sysErr = err
		}
	} else if !os.IsNotExist(err) {
		sysErr = err
	}

	if userPath, err := userConfigPath(); err == nil {
		if _, err := os.Stat(userPath); err == nil {
			if cfg, err := loadAt(userPath); err == nil {
				return cfg, nil
			} else if sysErr == nil {
				return &Config{Provider: "google_drive"}, fmt.Errorf("failed to read drive config: %w", err)
			}
		} else if !os.IsNotExist(err) && sysErr == nil {
			if os.IsPermission(err) {
				return &Config{Provider: "google_drive"}, fmt.Errorf("permission denied reading %s", userPath)
			}
			return &Config{Provider: "google_drive"}, fmt.Errorf("failed to read drive config: %w", err)
		}
	}

	if sysErr != nil {
		if os.IsPermission(sysErr) {
			return &Config{Provider: "google_drive"}, fmt.Errorf("permission denied reading %s", ConfigPath)
		}
		return &Config{Provider: "google_drive"}, fmt.Errorf("failed to read drive config: %w", sysErr)
	}
	return &Config{Provider: "google_drive"}, nil
}

func SaveConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("drive config is nil")
	}
	if cfg.Provider == "" {
		cfg.Provider = "google_drive"
	}
	if cfg.ConnectedAt == "" && cfg.RefreshToken != "" {
		cfg.ConnectedAt = time.Now().UTC().Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal drive config: %w", err)
	}

	writeConfig := func(dir, path string) error {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
		tmpPath := filepath.Join(dir, ".drive_config.json.tmp")
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			return fmt.Errorf("failed to write temp config: %w", err)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			return fmt.Errorf("failed to save drive config: %w", err)
		}
		return nil
	}

	sysErr := writeConfig(ConfigDir, ConfigPath)
	userPath, userErr := userConfigPath()
	if userErr == nil {
		_ = writeConfig(filepath.Dir(userPath), userPath)
	}
	if sysErr != nil {
		if userErr == nil {
			return nil
		}
		return sysErr
	}
	return nil
}

func (c *Config) IsConfigured() bool {
	if c == nil {
		return false
	}
	return c.RefreshToken != "" && c.TokenURI != ""
}

func userConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("failed to resolve user home")
	}
	return filepath.Join(home, ".config", "mirrorvault", "drive_config.json"), nil
}
