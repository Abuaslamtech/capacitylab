package load

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"github.com/Abuaslamtech/capacitylab/internal/config"
)

// ParsedStep represents a pre-compiled, allocation-free HTTP execution step
type ParsedStep struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    []byte
}

// CompiledScenario represents a weighted user journey ready for execution
type CompiledScenario struct {
	Name   string
	Weight int
	Steps  []ParsedStep
}

// ScenarioSelector provides O(1) weighted random selection of scenarios
type ScenarioSelector struct {
	scenarios   []CompiledScenario
	totalWeight int
}

// NewScenarioSelector compiles raw YAML scenarios into optimized execution structures
func NewScenarioSelector(rawScenarios []config.Scenario) (*ScenarioSelector, error) {
	if len(rawScenarios) == 0 {
		return nil, fmt.Errorf("no scenarios defined in configuration")
	}

	var compiled []CompiledScenario
	totalWeight := 0

	for _, raw := range rawScenarios {
		scenario := CompiledScenario{
			Name:   raw.Name,
			Weight: raw.Weight,
		}

		for _, rawStep := range raw.Flow {
			step, err := parseStep(rawStep)
			if err != nil {
				return nil, fmt.Errorf("scenario '%s': %w", raw.Name, err)
			}
			scenario.Steps = append(scenario.Steps, step)
		}

		compiled = append(compiled, scenario)
		totalWeight += raw.Weight
	}

	return &ScenarioSelector{
		scenarios:   compiled,
		totalWeight: totalWeight,
	}, nil
}

// Pick randomly selects a scenario based on weight distribution
func (s *ScenarioSelector) Pick() *CompiledScenario {
	if len(s.scenarios) == 1 {
		return &s.scenarios[0]
	}

	r := rand.Intn(s.totalWeight)
	cumulative := 0
	for i := range s.scenarios {
		cumulative += s.scenarios[i].Weight
		if r < cumulative {
			return &s.scenarios[i]
		}
	}
	return &s.scenarios[0]
}

func parseStep(stepMap config.ScenarioStep) (ParsedStep, error) {
	parsed := ParsedStep{
		Method:  "GET",
		Path:    "/",
		Headers: make(map[string]string),
	}

	// 1. Check for shorthand GET: "- get: /path" or "- get: { path: /path, ... }"
	if getVal, ok := stepMap["get"]; ok {
		parsed.Method = "GET"
		return extractPathAndDetails(getVal, parsed)
	}

	// 2. Check for POST: "- post: { path: /path, body: { ... } }"
	if postVal, ok := stepMap["post"]; ok {
		parsed.Method = "POST"
		return extractPathAndDetails(postVal, parsed)
	}

	// 3. Check for PUT: "- put: { path: /path, body: { ... } }"
	if putVal, ok := stepMap["put"]; ok {
		parsed.Method = "PUT"
		return extractPathAndDetails(putVal, parsed)
	}

	// 4. Check for DELETE: "- delete: /path"
	if delVal, ok := stepMap["delete"]; ok {
		parsed.Method = "DELETE"
		return extractPathAndDetails(delVal, parsed)
	}

	// 5. Check for explicit method/path
	if method, ok := stepMap["method"].(string); ok {
		parsed.Method = strings.ToUpper(method)
	}
	if path, ok := stepMap["path"].(string); ok {
		parsed.Path = path
	}

	return parsed, nil
}

func extractPathAndDetails(val interface{}, parsed ParsedStep) (ParsedStep, error) {
	switch v := val.(type) {
	case string:
		parsed.Path = v
		return parsed, nil

	case map[string]interface{}:
		if p, ok := v["path"].(string); ok {
			parsed.Path = p
		}
		if hdrs, ok := v["headers"].(map[string]interface{}); ok {
			for k, hVal := range hdrs {
				parsed.Headers[k] = fmt.Sprintf("%v", hVal)
			}
		}
		if body, ok := v["body"]; ok {
			bodyBytes, err := json.Marshal(body)
			if err != nil {
				return parsed, fmt.Errorf("failed to encode step body to JSON: %w", err)
			}
			parsed.Body = bytes.TrimSpace(bodyBytes)
			if _, exists := parsed.Headers["Content-Type"]; !exists {
				parsed.Headers["Content-Type"] = "application/json"
			}
		}
		return parsed, nil

	default:
		return parsed, fmt.Errorf("unsupported flow step format: %T", val)
	}
}
