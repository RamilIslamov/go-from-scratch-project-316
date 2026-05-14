package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnalyzeUsesConcurrencyWorkers(t *testing.T) {
	var mu sync.Mutex
	startedRequests := 0

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
								<a href="/page-2">Page 2</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1", "https://example.com/page-2":
				if req.Method == http.MethodHead {
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}

				if req.Method != http.MethodGet {
					t.Fatalf("unexpected method: %s", req.Method)
				}

				mu.Lock()
				startedRequests++
				mu.Unlock()

				time.Sleep(100 * time.Millisecond)

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("<html><body><h1>Page</h1></body></html>")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
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

	start := time.Now()

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(report.Pages))
	}

	if startedRequests != 2 {
		t.Fatalf("expected 2 child page requests, got %d", startedRequests)
	}

	if elapsed > 180*time.Millisecond {
		t.Fatalf("expected concurrent processing to finish faster, took %v", elapsed)
	}
}

func TestAnalyzeConcurrencyOptionAffectsCrawlSpeed(t *testing.T) {
	runAnalyze := func(concurrency int) time.Duration {
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
									<a href="/page-2">Page 2</a>
								</body>
							</html>
						`)),
						Header: make(http.Header),
					}, nil

				case "https://example.com/page-1", "https://example.com/page-2":
					if req.Method == http.MethodHead {
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Body:       io.NopCloser(strings.NewReader("")),
							Header:     make(http.Header),
						}, nil
					}

					time.Sleep(100 * time.Millisecond)

					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader("<html><body><h1>Page</h1></body></html>")),
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
			Concurrency: concurrency,
			HTTPClient:  client,
			IndentJSON:  true,
		}

		start := time.Now()

		result, err := Analyze(context.Background(), opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var report Report
		if err := json.Unmarshal(result, &report); err != nil {
			t.Fatalf("failed to unmarshal report: %v", err)
		}

		if len(report.Pages) != 3 {
			t.Fatalf("expected 3 pages, got %d", len(report.Pages))
		}

		return time.Since(start)
	}

	sequentialElapsed := runAnalyze(1)
	concurrentElapsed := runAnalyze(2)

	if concurrentElapsed >= sequentialElapsed {
		t.Fatalf("expected concurrent crawl to be faster: sequential=%v concurrent=%v", sequentialElapsed, concurrentElapsed)
	}
}
