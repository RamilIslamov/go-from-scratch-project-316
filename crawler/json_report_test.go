package crawler

import (
	"code/internal/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeJSONReportStructure(t *testing.T) {
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
								<title>Example title</title>
								<meta name="description" content="Example description">
							</head>
							<body>
								<h1>Main heading</h1>
								<a href="/missing">Missing page</a>
								<img src="/static/logo.png">
							</body>
						</html>
					`)),
					Header: make(http.Header),
				}, nil

			case "https://example.com/missing":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Body:       io.NopCloser(strings.NewReader("not found")),
					Header:     make(http.Header),
				}, nil

			case "https://example.com/static/logo.png":
				header := make(http.Header)
				header.Set("Content-Length", "12345")

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("image")),
					Header:     header,
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
		Retries:    0,
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

	if report.RootURL != "https://example.com" {
		t.Fatalf("expected root_url https://example.com, got %s", report.RootURL)
	}

	if report.Depth != 1 {
		t.Fatalf("expected depth 1, got %d", report.Depth)
	}

	if report.GeneratedAt.IsZero() {
		t.Fatalf("expected generated_at to be set")
	}

	if len(report.Pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(report.Pages))
	}

	page := report.Pages[0]

	if page.URL != "https://example.com" {
		t.Fatalf("expected page url https://example.com, got %s", page.URL)
	}

	if page.Depth != 0 {
		t.Fatalf("expected page depth 0, got %d", page.Depth)
	}

	if page.HTTPStatus != http.StatusOK {
		t.Fatalf("expected http_status 200, got %d", page.HTTPStatus)
	}

	if page.Status != "ok" {
		t.Fatalf("expected status ok, got %s", page.Status)
	}

	if page.Error != "" {
		t.Fatalf("expected empty page error, got %s", page.Error)
	}

	if page.DiscoveredAt.IsZero() {
		t.Fatalf("expected discovered_at to be set")
	}

	if !page.SEO.HasTitle || page.SEO.Title != "Example title" {
		t.Fatalf("unexpected seo title: %+v", page.SEO)
	}

	if !page.SEO.HasDescription || page.SEO.Description != "Example description" {
		t.Fatalf("unexpected seo description: %+v", page.SEO)
	}

	if !page.SEO.HasH1 {
		t.Fatalf("expected has_h1 true")
	}

	if len(page.BrokenLinks) != 1 {
		t.Fatalf("expected 1 broken link, got %d", len(page.BrokenLinks))
	}

	brokenLink := page.BrokenLinks[0]

	if brokenLink.URL != "https://example.com/missing" {
		t.Fatalf("unexpected broken link url: %s", brokenLink.URL)
	}

	if brokenLink.StatusCode != http.StatusNotFound {
		t.Fatalf("expected broken link status 404, got %d", brokenLink.StatusCode)
	}

	if len(page.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(page.Assets))
	}

	asset := page.Assets[0]

	if asset.URL != "https://example.com/static/logo.png" {
		t.Fatalf("unexpected asset url: %s", asset.URL)
	}

	if asset.Type != "image" {
		t.Fatalf("expected image asset, got %s", asset.Type)
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

func TestAnalyzeIndentJSONChangesOnlyFormatting(t *testing.T) {
	client := &http.Client{
		Transport: RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader("<html><body><h1>Hello</h1></body></html>")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	baseOpts := models.Options{
		URL:        "https://example.com",
		Depth:      1,
		Retries:    0,
		HTTPClient: client,
	}

	compactJSON, err := Analyze(context.Background(), baseOpts)
	if err != nil {
		t.Fatalf("unexpected error for compact JSON: %v", err)
	}

	indentedOpts := baseOpts
	indentedOpts.IndentJSON = true

	indentedJSON, err := Analyze(context.Background(), indentedOpts)
	if err != nil {
		t.Fatalf("unexpected error for indented JSON: %v", err)
	}

	var compactReport models.Report
	if err := json.Unmarshal(compactJSON, &compactReport); err != nil {
		t.Fatalf("failed to unmarshal compact JSON: %v", err)
	}

	var indentedReport models.Report
	if err := json.Unmarshal(indentedJSON, &indentedReport); err != nil {
		t.Fatalf("failed to unmarshal indented JSON: %v", err)
	}

	compactReport.GeneratedAt = indentedReport.GeneratedAt
	for i := range compactReport.Pages {
		compactReport.Pages[i].DiscoveredAt = indentedReport.Pages[i].DiscoveredAt
	}

	if !reflect.DeepEqual(compactReport, indentedReport) {
		t.Fatalf("expected same JSON content with and without indentation")
	}

	if !strings.Contains(string(indentedJSON), "\n") {
		t.Fatalf("expected indented JSON to contain new lines")
	}
}
