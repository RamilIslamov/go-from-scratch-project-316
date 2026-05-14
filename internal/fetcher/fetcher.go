package fetcher

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

type PageResult struct {
	StatusCode int
	Status     string
	Body       []byte
	Error      string
}

type LinkResult struct {
	URL        string
	StatusCode int
	Error      string
}

type AssetResult struct {
	URL        string
	StatusCode int
	SizeBytes  int64
	Error      string
}

type RateLimiter interface {
	Wait(ctx context.Context) error
}

type Options struct {
	Retries   int
	UserAgent string
}

func Do(
	ctx context.Context,
	client *http.Client,
	limiter RateLimiter,
	req *http.Request,
	retries int,
) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			closeResponseBody(lastResp)

			return nil, ctx.Err()
		}

		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				closeResponseBody(lastResp)

				return nil, err
			}
		}

		resp, err := client.Do(req)

		if err == nil && resp != nil && !isRetriableStatus(resp.StatusCode) {
			return resp, nil
		}

		if err != nil && !isRetriableError(err) {
			return resp, err
		}

		lastResp = resp
		lastErr = err

		if attempt == retries {
			return lastResp, lastErr
		}

		closeResponseBody(lastResp)

		timer := time.NewTimer(retryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()

			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return lastResp, lastErr
}

func FetchPage(
	ctx context.Context,
	client *http.Client,
	limiter RateLimiter,
	url string,
	opts Options,
) PageResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PageResult{
			Status: "error",
			Error:  err.Error(),
		}
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := Do(ctx, client, limiter, req, opts.Retries)
	if err != nil {
		return PageResult{
			Status: "error",
			Error:  err.Error(),
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	result := PageResult{
		StatusCode: resp.StatusCode,
	}

	if resp.StatusCode >= 400 {
		result.Status = "error"
		result.Error = ResponseStatusText(resp)
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PageResult{
			StatusCode: resp.StatusCode,
			Status:     "error",
			Error:      err.Error(),
		}
	}

	result.Status = "ok"
	result.Body = body

	return result
}

func ResponseStatusText(resp *http.Response) string {
	if resp == nil {
		return ""
	}

	if resp.Status != "" {
		return resp.Status
	}

	text := http.StatusText(resp.StatusCode)
	if text == "" {
		return "HTTP error"
	}

	return text
}

func CheckLink(
	ctx context.Context,
	client *http.Client,
	limiter RateLimiter,
	url string,
	opts Options,
) LinkResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return LinkResult{
			URL:   url,
			Error: err.Error(),
		}
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := Do(ctx, client, limiter, req, opts.Retries)
	if err != nil {
		return LinkResult{
			URL:   url,
			Error: err.Error(),
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return LinkResult{
			URL:        url,
			StatusCode: resp.StatusCode,
		}
	}

	return LinkResult{
		URL: url,
	}
}

func FetchAsset(
	ctx context.Context,
	client *http.Client,
	limiter RateLimiter,
	url string,
	opts Options,
) AssetResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return AssetResult{
			URL:   url,
			Error: err.Error(),
		}
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := Do(ctx, client, limiter, req, opts.Retries)
	if err != nil {
		return AssetResult{
			URL:   url,
			Error: err.Error(),
		}
	}

	if resp == nil {
		return AssetResult{
			URL:   url,
			Error: "empty response",
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	result := AssetResult{
		URL:        url,
		StatusCode: resp.StatusCode,
	}

	size, sizeErr := assetSize(resp)
	result.SizeBytes = size

	if resp.StatusCode >= 400 {
		result.Error = ResponseStatusText(resp)
		return result
	}

	if sizeErr != nil {
		result.Error = sizeErr.Error()
	}

	return result
}

func assetSize(resp *http.Response) (int64, error) {
	contentLength := resp.Header.Get("Content-Length")
	if contentLength != "" {
		size, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return 0, err
		}

		return size, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	return int64(len(body)), nil
}

func isRetriableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func isRetriableError(err error) bool {
	return err != nil
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(attempt+1) * 200 * time.Millisecond
}

func closeResponseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}
