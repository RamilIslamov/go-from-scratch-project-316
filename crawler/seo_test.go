package crawler

import (
	"code/internal/models"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

	if !strings.Contains(json, `"title": "Tom \u0026 Jerry"`) &&
		!strings.Contains(json, `"title": "Tom & Jerry"`) {
		t.Fatalf("expected decoded title")
	}

	if !strings.Contains(json, `"description": "Cats \u0026 mice"`) &&
		!strings.Contains(json, `"description": "Cats & mice"`) {
		t.Fatalf("expected decoded description")
	}
}
