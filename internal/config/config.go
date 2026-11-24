// Package config loads ctlctl's optional TOML configuration file.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the user's configuration.
type Config struct {
	// Suffixes selects executables on PATH by the end of their name.
	Suffixes []string `toml:"suffixes"`
	// Include lists extra tools to show, by name or path.
	Include []string `toml:"include"`
	// Exclude hides tools by name.
	Exclude []string `toml:"exclude"`
	// HelpTimeout bounds how long a tool may take to print its help.
	HelpTimeout duration `toml:"help_timeout"`
	// Shell runs the final command. Defaults to $SHELL, then /bin/sh.
	Shell string `toml:"shell"`
}

type duration struct{ time.Duration }

func (d *duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	d.Duration = v
	return err
}

// Default returns the configuration used when there is no config file.
func Default() Config {
	return Config{Suffixes: []string{"ctl"}, HelpTimeout: duration{5 * time.Second}}
}

// Path returns where the config file lives: $CTLCTL_CONFIG, or
// $XDG_CONFIG_HOME/ctlctl/config.toml (~/.config on Unix, %APPDATA% on Windows).
func Path() string {
	if p := os.Getenv("CTLCTL_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		if runtime.GOOS == "windows" {
			dir = os.Getenv("APPDATA")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(dir, "ctlctl", "config.toml")
}

// Load reads the config file at path. A missing file gives the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	if cfg.HelpTimeout.Duration <= 0 {
		cfg.HelpTimeout = Default().HelpTimeout
	}
	return cfg, nil
}

// Timeout returns the help timeout.
func (c Config) Timeout() time.Duration { return c.HelpTimeout.Duration }

// Example is a commented config file for the README and `ctlctl --help`.
const Example = `# ~/.config/ctlctl/config.toml

# Executables on PATH whose names end with one of these are listed.
suffixes = ["ctl"]

# Extra tools to list, by name or path.
include = ["docker", "helm", "gh"]

# Tools to hide.
exclude = ["sysctl"]

# How long a tool may take to print its --help.
help_timeout = "5s"

# The shell that runs the final command. Defaults to $SHELL.
# shell = "/bin/zsh"
`
