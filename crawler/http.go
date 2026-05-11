package crawler

import (
	"context"
	"io"
	"net/http"
	"time"
)

func fetchPage(ctx context.Context, client *http.Client, limiter *rateLimiter, opts Options, page *Page) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page.URL, nil)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return nil, err
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := doRequestWithRetries(ctx, client, limiter, req, opts.Retries)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	page.HTTPStatus = resp.StatusCode

	if resp.StatusCode >= 400 {
		page.Status = "error"
		page.Error = responseStatusText(resp)
		return nil, nil
	}

	page.Status = "ok"

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return nil, err
	}

	return body, nil
}

func doRequestWithRetries(ctx context.Context, client *http.Client, limiter *rateLimiter, req *http.Request, retries int) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			if lastResp != nil && lastResp.Body != nil {
				_ = lastResp.Body.Close()
			}
			return nil, ctx.Err()
		}

		if err := limiter.wait(ctx); err != nil {
			if lastResp != nil && lastResp.Body != nil {
				_ = lastResp.Body.Close()
			}
			return nil, err
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

		if lastResp != nil && lastResp.Body != nil {
			_ = lastResp.Body.Close()
		}

		timer := time.NewTimer(retryDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return lastResp, lastErr
}

func isRetriableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func isRetriableError(err error) bool {
	return err != nil
}

func retryDelay() time.Duration {
	return 50 * time.Millisecond
}

func responseStatusText(resp *http.Response) string {
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
