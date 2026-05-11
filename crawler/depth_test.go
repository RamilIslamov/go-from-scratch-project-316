package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnalyzeDepthOneCrawlsOnlyRootPage(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
								<a href="/page-2">Page 2</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1", "https://example.com/page-2":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("<html></html>")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
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

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected only root page, got %d", len(report.Pages))
	}

	if report.Pages[0].URL != "https://example.com" {
		t.Fatalf("expected root page, got %s", report.Pages[0].URL)
	}

	if report.Pages[0].Depth != 0 {
		t.Fatalf("expected root depth 0, got %d", report.Pages[0].Depth)
	}
}

func TestAnalyzeCrawlsInternalLinksOnly(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
								<a href="/page-2">Page 2</a>
								<a href="https://external.com/page">External</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("<html><body>Page 1</body></html>")),
					Header:     make(http.Header),
				}, nil

			case "https://example.com/page-2":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("<html><body>Page 2</body></html>")),
					Header:     make(http.Header),
				}, nil

			case "https://external.com/page":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("<html><body>External</body></html>")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 3 {
		t.Fatalf("expected 3 internal pages, got %d", len(report.Pages))
	}

	for _, page := range report.Pages {
		if strings.Contains(page.URL, "external.com") {
			t.Fatalf("external page should not be included in pages: %s", page.URL)
		}
	}
}

func TestAnalyzeDoesNotDuplicatePages(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
								<a href="/page-1">Page 1 duplicate</a>
								<a href="https://example.com/page-1">Page 1 absolute duplicate</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("<html><body>Page 1</body></html>")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
		IndentJSON: true,
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

	count := 0
	for _, page := range report.Pages {
		if page.URL == "https://example.com/page-1" {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("expected page-1 only once, got %d", count)
	}
}

func TestAnalyzeReturnsValidReportWhenContextCanceled(t *testing.T) {
	requestCount := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
								<a href="/page-2">Page 2</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1", "https://example.com/page-2":
				return nil, context.Canceled

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	opts := Options{
		URL:        "https://example.com",
		Depth:      2,
		HTTPClient: client,
		IndentJSON: true,
	}

	cancel()

	result, err := Analyze(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("report should be valid JSON: %v", err)
	}

	if report.RootURL != "https://example.com" {
		t.Fatalf("expected root url, got %s", report.RootURL)
	}

	if report.Pages == nil {
		t.Fatalf("pages should not be nil")
	}

	if requestCount != 0 {
		t.Fatalf("expected no requests after context cancellation, got %d", requestCount)
	}
}
