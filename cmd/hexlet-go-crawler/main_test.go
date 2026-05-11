package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunPrintsOnlyJSONWithTrailingNewline(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://example.com" {
				t.Fatalf("unexpected request: %s", req.URL.String())
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><body><h1>Hello</h1></body></html>")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	var out bytes.Buffer

	args := []string{
		"hexlet-go-crawler",
		"--depth",
		"1",
		"https://example.com",
	}

	err := run(args, &out, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("expected output to end with newline")
	}

	trimmed := strings.TrimSuffix(output, "\n")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		t.Fatalf("expected valid JSON only, got error: %v\noutput:\n%s", err, output)
	}

	if parsed["root_url"] != "https://example.com" {
		t.Fatalf("unexpected root_url: %v", parsed["root_url"])
	}

	if strings.HasPrefix(output, "Result:") ||
		strings.Contains(output, "URL is required") {
		t.Fatalf("CLI output should contain only JSON, got:\n%s", output)
	}
}
