package crawler

import (
	"code/internal/models"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

	if !strings.Contains(json, `"url": "https://example.com/missing"`) {
		t.Fatalf("expected missing link in broken_links")
	}

	if strings.Contains(json, "mailto:") ||
		strings.Contains(json, "tel:") ||
		strings.Contains(json, "javascript:") {
		t.Fatalf("unsupported links should not be included in report")
	}
}
