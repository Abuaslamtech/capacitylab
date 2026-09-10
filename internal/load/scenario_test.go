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

	method, path, headers, bodyReader := dynamicStep.ResolveExecution(nil)
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

func TestResponseExtractionAndChaining(t *testing.T) {
	// Step 1: Simulate response from POST /api/v1/sales
	respHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Trace-ID":   {"trace-xyz-789"},
	}
	respBody := []byte(`{
		"sale_id": "sale-456",
		"customer": {
			"name": "Jane Doe",
			"id": 999
		},
		"tags": ["retail", "promo"]
	}`)

	session := make(map[string]string)
	ExtractResponseContext(201, respHeaders, respBody, session)

	if session["response.status"] != "201" {
		t.Errorf("Expected status 201, got %s", session["response.status"])
	}
	if session["response.body.sale_id"] != "sale-456" {
		t.Errorf("Expected sale_id 'sale-456', got %s", session["response.body.sale_id"])
	}
	if session["response.body.customer.name"] != "Jane Doe" {
		t.Errorf("Expected customer.name 'Jane Doe', got %s", session["response.body.customer.name"])
	}
	if session["response.body.customer.id"] != "999" {
		t.Errorf("Expected customer.id '999', got %s", session["response.body.customer.id"])
	}
	if session["response.body.tags.0"] != "retail" {
		t.Errorf("Expected tag 0 'retail', got %s", session["response.body.tags.0"])
	}
	if session["response.header.x-trace-id"] != "trace-xyz-789" {
		t.Errorf("Expected trace header 'trace-xyz-789', got %s", session["response.header.x-trace-id"])
	}

	// Step 2: Next step using extracted variable in path, headers, and body
	nextStep := ParsedStep{
		Method:              "GET",
		Path:                "/api/v1/sales/{{response.body.sale_id}}/receipt",
		Headers:             map[string]string{"Authorization": "Bearer {{response.header.x-trace-id}}"},
		Body:                []byte(`{"customer_id":"{{response.body.customer.id}}"}`),
		HasDynamicVariables: true,
	}

	_, path, headers, bodyReader := nextStep.ResolveExecution(session)
	if path != "/api/v1/sales/sale-456/receipt" {
		t.Errorf("Expected interpolated path '/api/v1/sales/sale-456/receipt', got %s", path)
	}
	if headers["Authorization"] != "Bearer trace-xyz-789" {
		t.Errorf("Expected header 'Bearer trace-xyz-789', got %s", headers["Authorization"])
	}

	b, _ := io.ReadAll(bodyReader)
	if string(b) != `{"customer_id":"999"}` {
		t.Errorf("Expected body '{\"customer_id\":\"999\"}', got %s", string(b))
	}
}
