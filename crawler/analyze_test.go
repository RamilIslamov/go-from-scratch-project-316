package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

	opts := Options{
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

	opts := Options{
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

func TestAnalyzeBrokenLinks(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/ok">OK link</a>
								<a href="/missing">Broken link</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/ok":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
					Header:     make(http.Header),
				}, nil

			case "https://example.com/missing":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
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

	json := string(result)

	if !strings.Contains(json, `"broken_links": [`) {
		t.Fatalf("expected broken_links field")
	}

	if !strings.Contains(json, `"url": "https://example.com/missing"`) {
		t.Fatalf("expected missing link in broken_links")
	}

	if !strings.Contains(json, `"status_code": 404`) {
		t.Fatalf("expected status_code 404")
	}

	if strings.Contains(json, `"url": "https://example.com/ok"`) {
		t.Fatalf("working link should not be included in broken_links")
	}
}

func TestAnalyzeIgnoresUnsupportedLinks(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="">empty</a>
								<a href="#section">fragment only</a>
								<a href="mailto:test@example.com">email</a>
								<a href="tel:+123456789">phone</a>
								<a href="javascript:void(0)">js</a>
								<a href="/missing">missing</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/missing":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}, nil

			default:
				t.Fatalf("unsupported link should not be requested: %s", req.URL.String())
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

	json := string(result)

	if !strings.Contains(json, `"url": "https://example.com/missing"`) {
		t.Fatalf("expected missing link in broken_links")
	}

	if strings.Contains(json, "mailto:") ||
		strings.Contains(json, "tel:") ||
		strings.Contains(json, "javascript:") {
		t.Fatalf("unsupported links should not be included in report")
	}
}

func TestAnalyzeSEOWithAllTags(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<!doctype html>
						<html>
							<head>
								<title>Example Test</title>
								<meta name="description" content="Example description">
							</head>
							<body>
								<h1>Main heading</h1>
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

	json := string(result)

	if !strings.Contains(json, `"has_title": true`) {
		t.Fatalf("expected has_title true")
	}

	if !strings.Contains(json, `"title": "Example Test"`) {
		t.Fatalf("expected title text")
	}

	if !strings.Contains(json, `"has_description": true`) {
		t.Fatalf("expected has_description true")
	}

	if !strings.Contains(json, `"description": "Example description"`) {
		t.Fatalf("expected description text")
	}

	if !strings.Contains(json, `"has_h1": true`) {
		t.Fatalf("expected has_h1 true")
	}
}

func TestAnalyzeSEOWithoutTags(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<!doctype html>
						<html>
							<head></head>
							<body>
								<p>No SEO tags here</p>
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

	json := string(result)

	if !strings.Contains(json, `"has_title": false`) {
		t.Fatalf("expected has_title false")
	}

	if !strings.Contains(json, `"title": ""`) {
		t.Fatalf("expected empty title")
	}

	if !strings.Contains(json, `"has_description": false`) {
		t.Fatalf("expected has_description false")
	}

	if !strings.Contains(json, `"description": ""`) {
		t.Fatalf("expected empty description")
	}

	if !strings.Contains(json, `"has_h1": false`) {
		t.Fatalf("expected has_h1 false")
	}
}

func TestAnalyzeSEODecodesHTMLEntities(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<!doctype html>
						<html>
							<head>
								<title>Tom &amp; Jerry</title>
								<meta name="description" content="Cats &amp; mice">
							</head>
							<body>
								<h1>Cartoon</h1>
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

	json := string(result)

	if !strings.Contains(json, `"title": "Tom \u0026 Jerry"`) &&
		!strings.Contains(json, `"title": "Tom & Jerry"`) {
		t.Fatalf("expected decoded title")
	}

	if !strings.Contains(json, `"description": "Cats \u0026 mice"`) &&
		!strings.Contains(json, `"description": "Cats & mice"`) {
		t.Fatalf("expected decoded description")
	}
}

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

	opts := Options{
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

	var report Report
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

	opts := Options{
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

	var report Report
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

	opts := Options{
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

	var report Report
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

func TestAnalyzeRetriesTemporaryErrorAndSucceeds(t *testing.T) {
	requestCount := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			if requestCount == 1 {
				return nil, io.ErrUnexpectedEOF
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("<html><body>ok</body></html>")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    1,
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

	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	page := report.Pages[0]

	if page.Status != "ok" {
		t.Fatalf("expected page status ok, got %s", page.Status)
	}

	if page.HTTPStatus != http.StatusOK {
		t.Fatalf("expected http status 200, got %d", page.HTTPStatus)
	}

	if page.Error != "" {
		t.Fatalf("expected empty error, got %s", page.Error)
	}
}

func TestAnalyzeStopsAfterRetryLimit(t *testing.T) {
	requestCount := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			return nil, io.ErrUnexpectedEOF
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    2,
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

	if requestCount != 3 {
		t.Fatalf("expected 3 requests, got %d", requestCount)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	page := report.Pages[0]

	if page.Status != "error" {
		t.Fatalf("expected page status error, got %s", page.Status)
	}

	if page.Error == "" {
		t.Fatalf("expected error message")
	}
}

func TestAnalyzeDoesNotRetryNotFoundStatus(t *testing.T) {
	requestCount := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	opts := Options{
		URL:        "https://example.com/missing",
		Depth:      1,
		Retries:    2,
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

	if requestCount != 1 {
		t.Fatalf("expected 1 request for 404, got %d", requestCount)
	}

	page := report.Pages[0]

	if page.Status != "error" {
		t.Fatalf("expected page status error, got %s", page.Status)
	}

	if page.HTTPStatus != http.StatusNotFound {
		t.Fatalf("expected http status 404, got %d", page.HTTPStatus)
	}
}

func TestAnalyzeBrokenLinkUsesLastRetryResult(t *testing.T) {
	linkRequestCount := 0

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<a href="/unstable">Unstable link</a>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/unstable":
				linkRequestCount++

				if linkRequestCount == 1 {
					return &http.Response{
						StatusCode: http.StatusInternalServerError,
						Body:       io.NopCloser(strings.NewReader("temporary error")),
						Header:     make(http.Header),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("ok")),
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
		Retries:    1,
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

	if linkRequestCount != 2 {
		t.Fatalf("expected 2 link requests, got %d", linkRequestCount)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	if len(report.Pages[0].BrokenLinks) != 0 {
		t.Fatalf("expected no broken links after successful retry, got %d", len(report.Pages[0].BrokenLinks))
	}
}

func TestAnalyzeStopsRetriesWhenContextCanceled(t *testing.T) {
	requestCount := 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			if requestCount == 1 {
				cancel()
			}

			return nil, io.ErrUnexpectedEOF
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    5,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(ctx, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report Report
	if err := json.Unmarshal(result, &report); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("expected retry to stop after context cancellation, got %d requests", requestCount)
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	if report.Pages[0].Status != "error" {
		t.Fatalf("expected page status error, got %s", report.Pages[0].Status)
	}
}
