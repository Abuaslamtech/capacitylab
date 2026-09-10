package load

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestInterpolateDynamic(t *testing.T) {
	uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	t.Run("UUID generation", func(t *testing.T) {
		res := InterpolateDynamic("Bearer {{$uuid}}")
		parts := strings.Split(res, " ")
		if len(parts) != 2 || !uuidRegex.MatchString(parts[1]) {
			t.Errorf("Expected valid UUID token, got: %s", res)
		}
	})

	t.Run("Timestamp generation", func(t *testing.T) {
		res := InterpolateDynamic("{\"ts\":{{$timestamp}}}")
		if !strings.Contains(res, "{\"ts\":") {
			t.Errorf("Expected timestamp replacement, got: %s", res)
		}
	})

	t.Run("Random int within range", func(t *testing.T) {
		res := InterpolateDynamic("/users/{{$random_int(50,60)}}")
		valStr := strings.TrimPrefix(res, "/users/")
		val, err := strconv.Atoi(valStr)
		if err != nil || val < 50 || val > 60 {
			t.Errorf("Expected integer between 50 and 60, got: %s", res)
		}
	})

	t.Run("Static string unaltered", func(t *testing.T) {
		static := "https://api.example.com/health"
		res := InterpolateDynamic(static)
		if res != static {
			t.Errorf("Expected static string to remain unchanged, got: %s", res)
		}
	})
}

func TestParsedStep_ResolveExecution(t *testing.T) {
	dynamicStep := ParsedStep{
		Method:              "POST",
		Path:                "/items/{{$random_int(1,5)}}",
		Headers:             map[string]string{"X-ID": "{{$uuid}}"},
		Body:                []byte(`{"trace":"{{$uuid}}"}`),
		HasDynamicVariables: true,
	}

	method, path, headers, bodyReader := dynamicStep.ResolveExecution()
	if method != "POST" {
		t.Errorf("Expected POST, got %s", method)
	}
	if !strings.HasPrefix(path, "/items/") {
		t.Errorf("Expected dynamic path, got %s", path)
	}
	if !strings.Contains(headers["X-ID"], "-") {
		t.Errorf("Expected resolved UUID header, got %s", headers["X-ID"])
	}

	bodyBytes, _ := io.ReadAll(bodyReader)
	if strings.Contains(string(bodyBytes), "{{$uuid}}") {
		t.Errorf("Expected UUID in body to be interpolated, got %s", string(bodyBytes))
	}
}
