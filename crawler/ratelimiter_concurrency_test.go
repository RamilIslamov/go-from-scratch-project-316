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

func TestAnalyzeRespectsGlobalDelayWithConcurrency(t *testing.T) {
	var mu sync.Mutex
	var childGetTimes []time.Time

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

				mu.Lock()
				childGetTimes = append(childGetTimes, time.Now())
				mu.Unlock()

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
		Concurrency: 2,
		Delay:       50 * time.Millisecond,
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

	if len(report.Pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(report.Pages))
	}

	if len(childGetTimes) != 2 {
		t.Fatalf("expected 2 child GET requests, got %d", len(childGetTimes))
	}

	interval := childGetTimes[1].Sub(childGetTimes[0])
	if interval < 45*time.Millisecond {
		t.Fatalf("expected global delay between concurrent requests, got %v", interval)
	}
}
