package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ResourceLimits struct {
	TimeLimit    int `yaml:"time_limit"`    // seconds
	ProcessLimit int `yaml:"process_limit"` // 0 = unlimited
	MemoryLimitMB int `yaml:"memory_limit_mb"`
}

type ExecutionOptions struct {
	Path           string         `yaml:"path"`
	Args           []string       `yaml:"args"`
	ResourceLimits ResourceLimits `yaml:"resource_limits"`
}

type LanguageConfig struct {
	Language           string            `yaml:"language"`
	Filename           string            `yaml:"filename"`
	BinaryFilename     string            `yaml:"binary_filename"`
	CompilationOptions *ExecutionOptions `yaml:"compilation_options"`
	RuntimeOptions     ExecutionOptions  `yaml:"runtime_options"`
}

type Config struct {
	NsjailPath          string           `yaml:"nsjail_path"`
	SandboxDir          string           `yaml:"sandbox_dir"`
	DefaultNsjailArgs   []string         `yaml:"default_nsjail_args"`
	Languages           []LanguageConfig `yaml:"languages"`
}

// Load reads and parses the YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}
	return &cfg, nil
}

// Lookup returns the LanguageConfig for the given language identifier.
func (c *Config) Lookup(language string) (*LanguageConfig, error) {
	for i := range c.Languages {
		if c.Languages[i].Language == language {
			return &c.Languages[i], nil
		}
	}
	return nil, fmt.Errorf("unsupported language %q", language)
}

// ExpandArgs replaces template variables in an arg slice.
func ExpandArgs(args []string, vars map[string]string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		expanded := a
		for k, v := range vars {
			expanded = strings.ReplaceAll(expanded, "{{ "+k+" }}", v)
		}
		// Drop empty tokens (e.g. when {{ EXTRA_ARGS }} is blank)
		if strings.TrimSpace(expanded) != "" {
			out = append(out, expanded)
		}
	}
	return out
}
