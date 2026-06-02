// Package config loads and validates the devtree config file.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Services []Service `toml:"service"`
}

type Service struct {
	Name    string `toml:"name"`
	Repo    string `toml:"repo"`
	Command string `toml:"command"`
}

// DefaultPath returns the path to the user's config file,
// honoring $XDG_CONFIG_HOME and falling back to ~/.config.
func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "devtree", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "devtree", "config.toml"), nil
}

// Load reads a config file from disk and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config not found at %s — create it with at least one [[service]] block", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse decodes and validates raw config bytes. The path is used only for error messages.
func Parse(data []byte, path string) (*Config, error) {
	var cfg Config
	meta, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("parse %s: unknown keys: %v", path, undecoded)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Services) == 0 {
		return errors.New("no services defined (need at least one [[service]] block)")
	}

	seen := make(map[string]int, len(c.Services))
	for i, s := range c.Services {
		idx := i + 1
		if s.Name == "" {
			return fmt.Errorf("service #%d: name is required", idx)
		}
		if prev, dup := seen[s.Name]; dup {
			return fmt.Errorf("service #%d: duplicate name %q (also at service #%d)", idx, s.Name, prev)
		}
		seen[s.Name] = idx

		if s.Repo == "" {
			return fmt.Errorf("service %q: repo is required", s.Name)
		}
		if !filepath.IsAbs(s.Repo) {
			return fmt.Errorf("service %q: repo must be an absolute path, got %q", s.Name, s.Repo)
		}
		if s.Command == "" {
			return fmt.Errorf("service %q: command is required", s.Name)
		}
	}
	return nil
}
