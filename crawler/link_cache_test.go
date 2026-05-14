package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnalyzeUsesLinkCacheForDuplicateExternalLinks(t *testing.T) {
	externalLinkChecks := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
								<a href="https://external.com/missing">External missing</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1":
				if req.Method == http.MethodHead {
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="https://external.com/missing">External missing duplicate</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://external.com/missing":
				externalLinkChecks++

				if req.Method != http.MethodHead {
					t.Fatalf("expected HEAD for link check, got %s", req.Method)
				}

				return &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				return nil, nil
			}
		}),
	}

	opts := Options{
		URL:         "https://example.com",
		Depth:       2,
		Concurrency: 2,
		HTTPClient:  client,
		IndentJSON:  true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(report.Pages))
	}

	if externalLinkChecks != 1 {
		t.Fatalf("expected external link to be checked once, got %d", externalLinkChecks)
	}
}
