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
