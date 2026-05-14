package crawler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPublicAnalyzeAPI(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><body><h1>Hello</h1></body></html>")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      1,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) == 0 {
		t.Fatalf("expected non-empty JSON result")
	}
}
