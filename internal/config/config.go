package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var envPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)(?::-([^}]*))?\}`)

// ExpandEnvWithDefaults replaces ${VAR} and ${VAR:-default} with environment values
func ExpandEnvWithDefaults(input []byte) []byte {
	return envPattern.ReplaceAllFunc(input, func(match []byte) []byte {
		parts := envPattern.FindSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		varName := string(parts[1])
		hasDefault := len(parts) >= 3 && parts[2] != nil
		defaultVal := ""
		if hasDefault {
			defaultVal = string(parts[2])
		}

		if val, exists := os.LookupEnv(varName); exists && val != "" {
			return []byte(val)
		}
		if hasDefault {
			return []byte(defaultVal)
		}
		return []byte("")
	})
}

// LoadDotEnv searches for and loads .env and .env.local files without overwriting existing environment variables
func LoadDotEnv(dir string) {
	candidates := []string{
		filepath.Join(dir, ".env.local"),
		filepath.Join(dir, ".env"),
	}

	for _, path := range candidates {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "export ") {
				line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			// Strip quotes if present
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				if len(val) >= 2 {
					val = val[1 : len(val)-1]
				}
			}

			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, val)
			}
		}
		_ = file.Close()
	}
}

// Load reads, interpolates environment variables, and parses a capacitylab.yaml file
func Load(filepathStr string) (*Config, error) {
	dir := filepath.Dir(filepathStr)
	LoadDotEnv(dir)
	if dir != "." {
		LoadDotEnv(".")
	}

	data, err := os.ReadFile(filepathStr)
	if err != nil {
		return nil, fmt.Errorf("could not read file: %w", err)
	}

	interpolated := ExpandEnvWithDefaults(data)

	var cfg Config
	if err := yaml.Unmarshal(interpolated, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML syntax: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

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
	CPU        string       `yaml:"cpu"`
	Memory     string       `yaml:"memory"`
	CPUSet     string       `yaml:"cpuset"`
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
	Engine         string        `yaml:"engine"`
	StartUsers     int           `yaml:"start_users"`
	MaxUsers       int           `yaml:"max_users"`
	Step           int           `yaml:"step"`
	StepDuration   time.Duration `yaml:"step_duration"`
	WarmupDuration time.Duration `yaml:"warmup_duration"`
	Duration       time.Duration `yaml:"duration"` // Used for soak / constant workloads
	Pacing         PacingConfig  `yaml:"pacing"`
}

type PacingConfig struct {
	ThinkTimeMin time.Duration `yaml:"think_time_min"`
	ThinkTimeMax time.Duration `yaml:"think_time_max"`
}

type Thresholds struct {
	MaxCPUPercent         float64 `yaml:"max_cpu_percent"`
	MaxMemoryPercent      float64 `yaml:"max_memory_percent"`
	MaxP95LatencyMs       float64 `yaml:"max_p95_latency_ms"`
	MaxErrorRatePercent   float64 `yaml:"max_error_rate_percent"`
	MaxCPUThrottlePercent float64 `yaml:"max_cpu_throttle_percent"`
}

type Scenario struct {
	Name   string         `yaml:"name"`
	Weight int            `yaml:"weight"`
	Flow   []ScenarioStep `yaml:"flow"`
}

// ScenarioStep represents an individual step definition within a scenario flow.
type ScenarioStep map[string]any

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

	// Validate Workload Type
	if c.Workload.Type == "" {
		c.Workload.Type = "step-ramp"
	} else {
		c.Workload.Type = strings.ToLower(strings.TrimSpace(c.Workload.Type))
		validTypes := map[string]bool{"step-ramp": true, "soak": true, "constant": true, "spike": true}
		if !validTypes[c.Workload.Type] {
			return fmt.Errorf("invalid workload.type '%s' (supported: 'step-ramp', 'soak', 'spike')", c.Workload.Type)
		}
	}

	// Validate Workload
	if c.Workload.StartUsers <= 0 {
		return fmt.Errorf("workload.start_users must be > 0 (got: %d)", c.Workload.StartUsers)
	}
	if (c.Workload.Type == "soak" || c.Workload.Type == "constant") && c.Workload.MaxUsers == 0 {
		c.Workload.MaxUsers = c.Workload.StartUsers
	}
	if c.Workload.MaxUsers < c.Workload.StartUsers {
		return fmt.Errorf("workload.max_users (%d) cannot be less than start_users (%d)", c.Workload.MaxUsers, c.Workload.StartUsers)
	}
	if c.Workload.Type == "step-ramp" && c.Workload.Step <= 0 {
		return fmt.Errorf("workload.step must be > 0 (got: %d)", c.Workload.Step)
	} else if c.Workload.Step <= 0 {
		c.Workload.Step = 1
	}

	if (c.Workload.Type == "soak" || c.Workload.Type == "constant") && c.Workload.Duration <= 0 {
		if c.Workload.StepDuration > 0 {
			c.Workload.Duration = c.Workload.StepDuration * 5
		} else {
			c.Workload.Duration = 60 * time.Second
		}
	}

	// Default or validate Pacing
	if c.Workload.Pacing.ThinkTimeMin < 0 || c.Workload.Pacing.ThinkTimeMax < 0 {
		return fmt.Errorf("workload.pacing think times must be non-negative")
	}
	if c.Workload.Pacing.ThinkTimeMin > c.Workload.Pacing.ThinkTimeMax && c.Workload.Pacing.ThinkTimeMax > 0 {
		return fmt.Errorf("workload.pacing.think_time_min (%v) cannot be greater than think_time_max (%v)",
			c.Workload.Pacing.ThinkTimeMin, c.Workload.Pacing.ThinkTimeMax)
	}
	if c.Workload.Pacing.ThinkTimeMin == 0 && c.Workload.Pacing.ThinkTimeMax == 0 {
		c.Workload.Pacing.ThinkTimeMin = 200 * time.Millisecond
		c.Workload.Pacing.ThinkTimeMax = 800 * time.Millisecond
	} else if c.Workload.Pacing.ThinkTimeMax == 0 {
		c.Workload.Pacing.ThinkTimeMax = c.Workload.Pacing.ThinkTimeMin
	}

	// Validate Thresholds
	if c.Thresholds.MaxCPUPercent <= 0 {
		c.Thresholds.MaxCPUPercent = 85.0 // default 85% CPU saturation threshold
	} else if c.Thresholds.MaxCPUPercent > 100 {
		return fmt.Errorf("thresholds.max_cpu_percent must be between 1 and 100")
	}

	if c.Thresholds.MaxMemoryPercent <= 0 {
		c.Thresholds.MaxMemoryPercent = 80.0 // default 80% RAM saturation threshold
	} else if c.Thresholds.MaxMemoryPercent > 100 {
		return fmt.Errorf("thresholds.max_memory_percent must be between 1 and 100")
	}

	if c.Thresholds.MaxP95LatencyMs <= 0 {
		return fmt.Errorf("thresholds.max_p95_latency_ms must be > 0")
	}
	if c.Thresholds.MaxCPUThrottlePercent <= 0 {
		c.Thresholds.MaxCPUThrottlePercent = 15.0 // default 15% throttle limit
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
