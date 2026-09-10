package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete capacitylab.yaml schema
type Config struct {
	Version     string      `yaml:"version"`
	Application Application `yaml:"application"`
	Services    Services    `yaml:"services"`
	Workload    Workload    `yaml:"workload"`
	Thresholds  Thresholds  `yaml:"thresholds"`
	Scenarios   []Scenario  `yaml:"scenarios"`
}

type Application struct {
	Name    string  `yaml:"name"`
	URL     string  `yaml:"url"`
	Startup Startup `yaml:"startup"`
}

type Startup struct {
	ComposeFile   string        `yaml:"compose_file"`
	WaitForHealth bool          `yaml:"wait_for_health"`
	Timeout       time.Duration `yaml:"timeout"`
}

type Services struct {
	API      APIService      `yaml:"api"`
	Postgres PostgresService `yaml:"postgres"`
	Redis    RedisService    `yaml:"redis"`
}

type APIService struct {
	Container  string       `yaml:"container"`
	TestMatrix MatrixConfig `yaml:"test_matrix"`
}

type MatrixConfig struct {
	CPU    []string `yaml:"cpu"`
	Memory []string `yaml:"memory"`
}

type PostgresService struct {
	Container      string `yaml:"container"`
	DSN            string `yaml:"dsn"`
	MaxConnections int    `yaml:"max_connections"`
}

type RedisService struct {
	Container string `yaml:"container"`
	Addr      string `yaml:"addr"`
}

type Workload struct {
	Type           string        `yaml:"type"`
	StartUsers     int           `yaml:"start_users"`
	MaxUsers       int           `yaml:"max_users"`
	Step           int           `yaml:"step"`
	StepDuration   time.Duration `yaml:"step_duration"`
	WarmupDuration time.Duration `yaml:"warmup_duration"`
}

type Thresholds struct {
	MaxCPUPercent       float64 `yaml:"max_cpu_percent"`
	MaxMemoryPercent    float64 `yaml:"max_memory_percent"`
	MaxP95LatencyMs     float64 `yaml:"max_p95_latency_ms"`
	MaxErrorRatePercent float64 `yaml:"max_error_rate_percent"`
}

type Scenario struct {
	Name   string         `yaml:"name"`
	Weight int            `yaml:"weight"`
	Flow   []ScenarioStep `yaml:"flow"`
}

type ScenarioStep map[string]interface{}

// Load reads and parses a capacitylab.yaml file
func Load(filepath string) (*Config, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("could not read file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML syntax: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate verifies that the configuration values are semantically sound
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("missing 'version' field")
	}

	if c.Application.Name == "" {
		return fmt.Errorf("application.name is required")
	}

	if c.Application.URL == "" {
		return fmt.Errorf("application.url is required")
	}

	// Validate application URL
	parsedURL, err := url.Parse(c.Application.URL)
	if err != nil || (!strings.HasPrefix(parsedURL.Scheme, "http")) {
		return fmt.Errorf("application.url must be a valid HTTP/HTTPS URL (got: %s)", c.Application.URL)
	}

	// Validate Workload
	if c.Workload.StartUsers <= 0 {
		return fmt.Errorf("workload.start_users must be > 0 (got: %d)", c.Workload.StartUsers)
	}
	if c.Workload.MaxUsers < c.Workload.StartUsers {
		return fmt.Errorf("workload.max_users (%d) cannot be less than start_users (%d)", c.Workload.MaxUsers, c.Workload.StartUsers)
	}
	if c.Workload.Step <= 0 {
		return fmt.Errorf("workload.step must be > 0 (got: %d)", c.Workload.Step)
	}

	// Validate Thresholds
	if c.Thresholds.MaxCPUPercent <= 0 || c.Thresholds.MaxCPUPercent > 100 {
		return fmt.Errorf("thresholds.max_cpu_percent must be between 1 and 100")
	}
	if c.Thresholds.MaxMemoryPercent <= 0 || c.Thresholds.MaxMemoryPercent > 100 {
		return fmt.Errorf("thresholds.max_memory_percent must be between 1 and 100")
	}
	if c.Thresholds.MaxP95LatencyMs <= 0 {
		return fmt.Errorf("thresholds.max_p95_latency_ms must be > 0")
	}

	// Validate Scenarios
	if len(c.Scenarios) == 0 {
		return fmt.Errorf("at least one scenario must be defined")
	}

	totalWeight := 0
	for _, sc := range c.Scenarios {
		if sc.Name == "" {
			return fmt.Errorf("all scenarios must have a 'name'")
		}
		if sc.Weight <= 0 {
			return fmt.Errorf("scenario '%s' must have a positive weight", sc.Name)
		}
		totalWeight += sc.Weight
	}

	if totalWeight != 100 {
		return fmt.Errorf("scenario weights must sum to 100%% (current sum: %d%%)", totalWeight)
	}

	return nil
}
