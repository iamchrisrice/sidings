package router

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type configFile struct {
	Routes map[string]struct {
		Backend string `yaml:"backend"`
		Model   string `yaml:"model"`
	} `yaml:"routes"`
}

// DefaultTable returns a copy of the hardcoded default routing table.
func DefaultTable() map[string]Decision {
	out := make(map[string]Decision, len(defaultRoutes))
	for k, v := range defaultRoutes {
		out[k] = v
	}
	return out
}

// LoadConfig loads ~/.sidings/route.yaml and merges any overrides on top of
// the default routing table. Missing config file is not an error.
func LoadConfig() map[string]Decision {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultTable()
	}
	return LoadConfigFrom(filepath.Join(home, ".sidings", "route.yaml"))
}

// LoadConfigFrom loads a routing table from the given YAML file path and merges
// it on top of the default table. Missing or malformed file returns defaults.
func LoadConfigFrom(path string) map[string]Decision {
	table := DefaultTable()

	data, err := os.ReadFile(path)
	if err != nil {
		return table // file is optional
	}

	var cfg configFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return table
	}
	for tier, route := range cfg.Routes {
		if route.Backend != "" && route.Model != "" {
			table[tier] = Decision{Backend: route.Backend, Model: route.Model}
		}
	}
	return table
}
