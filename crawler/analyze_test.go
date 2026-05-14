package crawler

import (
	"code/internal/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeSuccess(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	opts := models.Options{
		URL:        "https://example.com",
		Depth:      1,
		Timeout:    5 * time.Second,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	json := string(result)

	if !strings.Contains(json, `"http_status": 200`) {
		t.Fatalf("expected status 200 in result")
	}

	if !strings.Contains(json, `"status": "ok"`) {
		t.Fatalf("expected status ok")
	}
}

func TestAnalyzeNetworkError(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		}),
	}

	opts := models.Options{
		URL:        "https://example.com",
		Depth:      1,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	json := string(result)

	if !strings.Contains(json, `"http_status": 0`) {
		t.Fatalf("expected http_status 0")
	}

	if !strings.Contains(json, `"status": "error"`) {
		t.Fatalf(`expected status "error"`)
	}

	if !strings.Contains(json, `"error":`) {
		t.Fatalf("expected error field")
	}
}

func TestAnalyzeTimeout(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()

			return nil, req.Context().Err()
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	opts := models.Options{
		URL:        "https://example.com/slow-page",
		Depth:      1,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	json := string(result)

	if !strings.Contains(json, `"status": "error"`) {
		t.Fatalf(`expected status "error"`)
	}

	if !strings.Contains(json, `context deadline exceeded`) {
		t.Fatalf("expected timeout error")
	}
}

func TestAnalyzeRespectsDelayBetweenRequests(t *testing.T) {
	var requestTimes []time.Time

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestTimes = append(requestTimes, time.Now())

			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
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

	opts := models.Options{
		URL:        "https://example.com",
		Depth:      2,
		Delay:      50 * time.Millisecond,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report models.Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(report.Pages))
	}

	if len(requestTimes) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requestTimes))
	}

	interval := requestTimes[1].Sub(requestTimes[0])
	if interval < 45*time.Millisecond {
		t.Fatalf("expected delay between requests, got %v", interval)
	}
}

func TestAnalyzeRPSOverridesDelay(t *testing.T) {
	var requestTimes []time.Time

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestTimes = append(requestTimes, time.Now())

			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
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

	opts := models.Options{
		URL:        "https://example.com",
		Depth:      2,
		Delay:      time.Second,
		RPS:        20,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report models.Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(report.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(report.Pages))
	}

	if len(requestTimes) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requestTimes))
	}

	interval := requestTimes[1].Sub(requestTimes[0])

	if interval < 45*time.Millisecond {
		t.Fatalf("expected RPS delay around 50ms, got %v", interval)
	}

	if interval > 500*time.Millisecond {
		t.Fatalf("RPS should override 1s delay, got %v", interval)
	}
}

func TestAnalyzeStopsWaitingWhenContextCanceled(t *testing.T) {
	requestCount := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 {
				cancel()
			}

			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/page-1">Page 1</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	opts := models.Options{
		URL:        "https://example.com",
		Depth:      2,
		Delay:      time.Second,
		HTTPClient: client,
		IndentJSON: true,
	}

	start := time.Now()

	result, err := Analyze(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report models.Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	elapsed := time.Since(start)

	if requestCount != 1 {
		t.Fatalf("expected only root request, got %d", requestCount)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected only root page, got %d", len(report.Pages))
	}

	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected analyze to stop without waiting full delay, took %v", elapsed)
	}
}
