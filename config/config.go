package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/bazelbuild/bazelisk/ws"
)

const rcFileName = ".bazeliskrc"

// Config allows getting Bazelisk configuration values.
type Config interface {
	Get(name string) string
}

// FromEnv returns a Config which gets config values from environment variables.
func FromEnv() Config {
	return &fromEnv{}
}

type fromEnv struct{}

func (c *fromEnv) Get(name string) string {
	return os.Getenv(name)
}

// FromFile returns a Config which gets config values from a Bazelisk config file.
func FromFile(path string) (Config, error) {
	values, err := parseFileConfig(path)
	if err != nil {
		return nil, err
	}
	return &static{
		values: values,
	}, nil
}

type static struct {
	values map[string]string
}

func (c *static) Get(name string) string {
	return c.values[name]
}

// parseFileConfig parses a .bazeliskrc file as a map of key-value configuration values.
func parseFileConfig(rcFilePath string) (map[string]string, error) {
	config := make(map[string]string)

	contents, err := ioutil.ReadFile(rcFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Non-critical error.
			return config, nil
		}
		return nil, err
	}

	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "#") {
			// comments
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		config[key] = strings.TrimSpace(parts[1])
	}

	return config, nil
}

// LocateUserConfigFile locates a .bazeliskrc file in the user's home directory.
func LocateUserConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rcFileName), nil
}

// LocateWorkspaceConfigFile locates a .bazeliskrc file in the current workspace root.
func LocateWorkspaceConfigFile() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	workspaceRoot := ws.FindWorkspaceRoot(workingDirectory)
	if workspaceRoot == "" {
		return "", err
	}
	return filepath.Join(workspaceRoot, rcFileName), nil
}

// Layered returns a Config which gets config values from the first of a series of other Config values which sets the config.
func Layered(configs ...Config) Config {
	return &layered{
		configs: configs,
	}
}

type layered struct {
	configs []Config
}

func (c *layered) Get(name string) string {
	for _, config := range c.configs {
		if value := config.Get(name); value != "" {
			return value
		}
	}
	return ""
}

// Null returns a Config with no config values.
func Null() Config {
	return &static{}
}

// Static returns a Config with static values.
func Static(values map[string]string) Config {
	return &static{
		values: values,
	}
}
