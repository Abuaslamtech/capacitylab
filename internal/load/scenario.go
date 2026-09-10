package load

import (
	"bytes"
	cryptoRand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Abuaslamtech/capacitylab/internal/config"
)

var randomIntPattern = regexp.MustCompile(`\{\{\$random_int\((\d+),(\d+)\)\}\}`)

// ParsedStep represents a pre-compiled, allocation-free HTTP execution step
type ParsedStep struct {
	Method              string
	Path                string
	Headers             map[string]string
	Body                []byte
	HasDynamicVariables bool
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

func extractPathAndDetails(val any, parsed ParsedStep) (ParsedStep, error) {
	switch v := val.(type) {
	case string:
		parsed.Path = v
		return parsed, nil

	case map[string]any:
		if p, ok := v["path"].(string); ok {
			parsed.Path = p
		}
		if hdrs, ok := v["headers"].(map[string]any); ok {
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

		// Detect if step contains dynamic generator or session placeholders
		hasDynamic := strings.Contains(parsed.Path, "{{") || bytes.Contains(parsed.Body, []byte("{{"))
		for _, h := range parsed.Headers {
			if strings.Contains(h, "{{") {
				hasDynamic = true
				break
			}
		}
		parsed.HasDynamicVariables = hasDynamic
		return parsed, nil

	default:
		return parsed, fmt.Errorf("unsupported flow step format: %T", val)
	}
}

var tokenPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// ResolveExecution returns the method, path, headers, and body for a single request invocation
func (p *ParsedStep) ResolveExecution(session map[string]string) (string, string, map[string]string, io.Reader) {
	if !p.HasDynamicVariables && len(session) == 0 {
		var bodyReader io.Reader
		if len(p.Body) > 0 {
			bodyReader = bytes.NewReader(p.Body)
		}
		return p.Method, p.Path, p.Headers, bodyReader
	}

	resolvedPath := InterpolateString(p.Path, session)
	resolvedHeaders := make(map[string]string, len(p.Headers))
	for k, v := range p.Headers {
		resolvedHeaders[k] = InterpolateString(v, session)
	}

	var bodyReader io.Reader
	if len(p.Body) > 0 {
		resolvedBody := []byte(InterpolateString(string(p.Body), session))
		bodyReader = bytes.NewReader(resolvedBody)
	}

	return p.Method, resolvedPath, resolvedHeaders, bodyReader
}

// InterpolateString evaluates both generator tokens ({{$uuid}}) and session response tokens ({{response.body.id}})
func InterpolateString(input string, session map[string]string) string {
	if !strings.Contains(input, "{{") {
		return input
	}

	// 1. Evaluate generators {{$uuid}}, {{$timestamp}}, {{$random_int}}
	input = InterpolateDynamic(input)

	// 2. Handle shorthand aliases without dollar sign ({{uuid}}, {{timestamp}})
	if strings.Contains(input, "{{uuid}}") {
		input = strings.ReplaceAll(input, "{{uuid}}", generateUUID())
	}
	if strings.Contains(input, "{{timestamp}}") {
		input = strings.ReplaceAll(input, "{{timestamp}}", strconv.FormatInt(time.Now().UnixMilli(), 10))
	}

	// 3. Evaluate session/response tokens: {{response.body.sale_id}}
	if len(session) > 0 && strings.Contains(input, "{{") {
		input = tokenPattern.ReplaceAllStringFunc(input, func(m string) string {
			key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(m, "{{"), "}}"))
			if val, ok := session[key]; ok {
				return val
			}
			return m
		})
	}

	return input
}

// ExtractResponseContext parses response attributes and JSON body into dot-notation session variables
func ExtractResponseContext(status int, headers map[string][]string, bodyBytes []byte, dest map[string]string) {
	dest["response.status"] = strconv.Itoa(status)
	for k, v := range headers {
		if len(v) > 0 {
			dest["response.header."+strings.ToLower(k)] = v[0]
		}
	}
	if len(bodyBytes) > 0 {
		var parsed any
		if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
			flattenJSON("response.body", parsed, dest)
		}
	}
}

func flattenJSON(prefix string, val any, dest map[string]string) {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flattenJSON(p, child, dest)
		}
	case []any:
		for i, child := range v {
			p := fmt.Sprintf("%s.%d", prefix, i)
			flattenJSON(p, child, dest)
		}
	default:
		dest[prefix] = fmt.Sprintf("%v", val)
	}
}

// InterpolateDynamic evaluates runtime generator tokens in strings ({{$uuid}}, {{$timestamp}}, {{$random_int}})
func InterpolateDynamic(input string) string {
	if !strings.Contains(input, "{{$") {
		return input
	}

	if strings.Contains(input, "{{$uuid}}") {
		input = strings.ReplaceAll(input, "{{$uuid}}", generateUUID())
	}
	if strings.Contains(input, "{{$timestamp}}") {
		input = strings.ReplaceAll(input, "{{$timestamp}}", strconv.FormatInt(time.Now().UnixMilli(), 10))
	}
	if strings.Contains(input, "{{$random_int}}") {
		input = strings.ReplaceAll(input, "{{$random_int}}", strconv.Itoa(rand.Intn(1000000)))
	}

	input = randomIntPattern.ReplaceAllStringFunc(input, func(m string) string {
		matches := randomIntPattern.FindStringSubmatch(m)
		if len(matches) == 3 {
			min, err1 := strconv.Atoi(matches[1])
			max, err2 := strconv.Atoi(matches[2])
			if err1 == nil && err2 == nil && max >= min {
				return strconv.Itoa(min + rand.Intn(max-min+1))
			}
		}
		return m
	})

	return input
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = cryptoRand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
