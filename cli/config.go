package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Nick  string `json:"nick"`
	Color string `json:"color"`
}

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}

	return filepath.Join(home, ".termchat")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

func loadConfig() Config {
	path := configPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}

	var cfg Config

	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return Config{}
	}

	return cfg
}

func saveConfig(cfg Config) {
	os.MkdirAll(configDir(), 0o755)

	data, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return
	}

	os.WriteFile(configPath(), data, 0o644)
}
