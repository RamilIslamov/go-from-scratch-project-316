package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestAnalyzeUsesAssetCacheForDuplicateAssets(t *testing.T) {
	assetRequestCount := 0

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
								<img src="/static/logo.png">
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/page-1":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<img src="/static/logo.png">
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/static/logo.png":
				assetRequestCount++

				header := make(http.Header)
				header.Set("Content-Length", "12345")

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("image-content")),
					Header:     header,
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

	if assetRequestCount != 1 {
		t.Fatalf("expected asset to be requested once, got %d", assetRequestCount)
	}

	if len(report.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(report.Pages))
	}

	for _, page := range report.Pages {
		if len(page.Assets) != 1 {
			t.Fatalf("expected 1 asset on page %s, got %d", page.URL, len(page.Assets))
		}

		asset := page.Assets[0]

		if asset.URL != "https://example.com/static/logo.png" {
			t.Fatalf("unexpected asset url: %s", asset.URL)
		}

		if asset.Type != "image" {
			t.Fatalf("expected image asset type, got %s", asset.Type)
		}

		if asset.StatusCode != http.StatusOK {
			t.Fatalf("expected asset status 200, got %d", asset.StatusCode)
		}

		if asset.SizeBytes != 12345 {
			t.Fatalf("expected asset size 12345, got %d", asset.SizeBytes)
		}

		if asset.Error != "" {
			t.Fatalf("expected empty asset error, got %s", asset.Error)
		}
	}
}

func TestAnalyzeAssetSizeFromBodyWhenContentLengthMissing(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<head>
								<link rel="stylesheet" href="/static/app.css">
							</head>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/static/app.css":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("body { color: red; }")),
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
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	assets := report.Pages[0].Assets
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}

	asset := assets[0]

	if asset.URL != "https://example.com/static/app.css" {
		t.Fatalf("unexpected asset url: %s", asset.URL)
	}

	if asset.Type != "style" {
		t.Fatalf("expected style asset type, got %s", asset.Type)
	}

	if asset.StatusCode != http.StatusOK {
		t.Fatalf("expected asset status 200, got %d", asset.StatusCode)
	}

	expectedSize := int64(len("body { color: red; }"))
	if asset.SizeBytes != expectedSize {
		t.Fatalf("expected asset size %d, got %d", expectedSize, asset.SizeBytes)
	}

	if asset.Error != "" {
		t.Fatalf("expected empty error, got %s", asset.Error)
	}
}

func TestAnalyzeAssetErrorStatus(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<script src="/static/app.js"></script>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/static/app.js":
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(strings.NewReader("server error")),
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
		Retries:    0,
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
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	assets := report.Pages[0].Assets
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}

	asset := assets[0]

	if asset.URL != "https://example.com/static/app.js" {
		t.Fatalf("unexpected asset url: %s", asset.URL)
	}

	if asset.Type != "script" {
		t.Fatalf("expected script asset type, got %s", asset.Type)
	}

	if asset.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected asset status 500, got %d", asset.StatusCode)
	}

	if asset.Error == "" {
		t.Fatalf("expected asset error")
	}
}

func TestAnalyzeAssetNetworkError(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<body>
								<img src="/static/missing-logo.png">
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/static/missing-logo.png":
				return nil, io.ErrUnexpectedEOF

			default:
				t.Fatalf("unexpected request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	opts := Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    0,
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
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	assets := report.Pages[0].Assets
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}

	asset := assets[0]

	if asset.URL != "https://example.com/static/missing-logo.png" {
		t.Fatalf("unexpected asset url: %s", asset.URL)
	}

	if asset.Type != "image" {
		t.Fatalf("expected image asset type, got %s", asset.Type)
	}

	if asset.StatusCode != 0 {
		t.Fatalf("expected status 0 for network error, got %d", asset.StatusCode)
	}

	if asset.SizeBytes != 0 {
		t.Fatalf("expected size 0 for network error, got %d", asset.SizeBytes)
	}

	if asset.Error == "" {
		t.Fatalf("expected asset error")
	}
}

func TestAnalyzeExtractsAllSupportedAssetTypes(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body: io.NopCloser(strings.NewReader(`
						<html>
							<head>
								<link rel="stylesheet" href="/static/app.css">
							</head>
							<body>
								<img src="/static/logo.png">
								<script src="/static/app.js"></script>
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/static/app.css":
				header := make(http.Header)
				header.Set("Content-Length", "10")

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("css")),
					Header:     header,
				}, nil

			case "https://example.com/static/logo.png":
				header := make(http.Header)
				header.Set("Content-Length", "20")

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("image")),
					Header:     header,
				}, nil

			case "https://example.com/static/app.js":
				header := make(http.Header)
				header.Set("Content-Length", "30")

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("js")),
					Header:     header,
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
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	assets := report.Pages[0].Assets
	if len(assets) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(assets))
	}

	types := map[string]bool{}
	for _, asset := range assets {
		types[asset.Type] = true

		if asset.StatusCode != http.StatusOK {
			t.Fatalf("expected asset status 200, got %d", asset.StatusCode)
		}

		if asset.SizeBytes == 0 {
			t.Fatalf("expected non-zero asset size for %s", asset.URL)
		}

		if asset.Error != "" {
			t.Fatalf("expected empty asset error, got %s", asset.Error)
		}
	}

	if !types["image"] {
		t.Fatalf("expected image asset")
	}

	if !types["script"] {
		t.Fatalf("expected script asset")
	}

	if !types["style"] {
		t.Fatalf("expected style asset")
	}
}
