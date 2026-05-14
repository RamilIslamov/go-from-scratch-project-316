package crawler

import (
	"code/internal/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

	opts := models.Options{
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

	var report models.Report
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

	opts := models.Options{
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

	var report models.Report
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

	opts := models.Options{
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

	var report models.Report
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

	opts := models.Options{
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

	var report models.Report
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

	opts := models.Options{
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

	var report models.Report
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
