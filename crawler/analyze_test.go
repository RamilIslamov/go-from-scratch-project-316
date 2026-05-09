package crawler

import (
	"context"
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

func TestAnalyzeNotFoundStatus(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader("not found")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	opts := Options{
		URL:        "https://example.com/missing-page",
		Depth:      1,
		HTTPClient: client,
		IndentJSON: true,
	}

	result, err := Analyze(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	json := string(result)

	if !strings.Contains(json, `"http_status": 404`) {
		t.Fatalf("expected http_status 404")
	}

	if !strings.Contains(json, `"status": "error"`) {
		t.Fatalf(`expected status "error" for 404`)
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
