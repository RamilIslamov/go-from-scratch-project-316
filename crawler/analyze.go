package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	RPS         int
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
}

type rateLimiter struct {
	interval time.Duration
	last     time.Time
}

type Report struct {
	RootURL     string    `json:"root_url"`
	Depth       int       `json:"depth"`
	GeneratedAt time.Time `json:"generated_at"`
	Pages       []Page    `json:"pages"`
}

type Page struct {
	URL          string       `json:"url"`
	Depth        int          `json:"depth"`
	HTTPStatus   int          `json:"http_status"`
	Status       string       `json:"status"`
	Error        string       `json:"error,omitempty"`
	SEO          SEO          `json:"seo"`
	BrokenLinks  []BrokenLink `json:"broken_links"`
	DiscoveredAt time.Time    `json:"discovered_at"`
}

type BrokenLink struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

type SEO struct {
	HasTitle       bool   `json:"has_title"`
	Title          string `json:"title"`
	HasDescription bool   `json:"has_description"`
	Description    string `json:"description"`
	HasH1          bool   `json:"has_h1"`
}

type crawlItem struct {
	URL   string
	Depth int
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	limiter := newRateLimiter(opts)

	report := Report{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now(),
		Pages:       []Page{},
	}

	visited := make(map[string]bool)

	queue := []crawlItem{
		{
			URL:   opts.URL,
			Depth: 0,
		},
	}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			break
		}

		item := queue[0]
		queue = queue[1:]

		if visited[item.URL] {
			continue
		}

		visited[item.URL] = true

		page := Page{
			URL:          item.URL,
			Depth:        item.Depth,
			BrokenLinks:  []BrokenLink{},
			DiscoveredAt: time.Now(),
		}

		body, err := fetchPage(ctx, client, limiter, opts, &page)
		if err == nil && page.Status == "ok" {
			page.SEO = extractSEO(body)

			links := extractLinks(body, page.URL)
			page.BrokenLinks = checkBrokenLinks(ctx, client, limiter, opts, links)

			nextDepth := item.Depth + 1
			if nextDepth < opts.Depth {
				for _, link := range links {
					if isInternalLink(opts.URL, link) && !visited[link] {
						queue = append(queue, crawlItem{
							URL:   link,
							Depth: nextDepth,
						})
					}
				}
			}
		}

		report.Pages = append(report.Pages, page)
	}

	if opts.IndentJSON {
		return json.MarshalIndent(report, "", "  ")
	}

	return json.Marshal(report)
}

func isInternalLink(rootURL string, link string) bool {
	root, err := url.Parse(rootURL)
	if err != nil {
		return false
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return false
	}

	return strings.EqualFold(root.Host, parsed.Host)
}

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

	defer resp.Body.Close()

	page.HTTPStatus = resp.StatusCode

	if resp.StatusCode >= 400 {
		page.Status = "error"
		page.Error = resp.Status
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

func checkBrokenLinks(ctx context.Context, client *http.Client, limiter *rateLimiter, opts Options, links []string) []BrokenLink {
	brokenLinks := []BrokenLink{}

	for _, link := range links {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			brokenLinks = append(brokenLinks, BrokenLink{
				URL:   link,
				Error: err.Error(),
			})
			continue
		}

		if opts.UserAgent != "" {
			req.Header.Set("User-Agent", opts.UserAgent)
		}

		resp, err := doRequestWithRetries(ctx, client, limiter, req, opts.Retries)
		if err != nil {
			brokenLinks = append(brokenLinks, BrokenLink{
				URL:   link,
				Error: err.Error(),
			})
			continue
		}

		resp.Body.Close()

		if resp.StatusCode >= 400 {
			brokenLinks = append(brokenLinks, BrokenLink{
				URL:        link,
				StatusCode: resp.StatusCode,
			})
		}
	}

	return brokenLinks
}

func newRateLimiter(opts Options) *rateLimiter {
	if opts.RPS > 0 {
		return &rateLimiter{
			interval: time.Second / time.Duration(opts.RPS),
		}
	}

	if opts.Delay > 0 {
		return &rateLimiter{
			interval: opts.Delay,
		}
	}

	return &rateLimiter{}
}

func (l *rateLimiter) wait(ctx context.Context) error {
	if l.interval <= 0 {
		l.last = time.Now()
		return nil
	}

	now := time.Now()

	if l.last.IsZero() {
		l.last = now
		return nil
	}

	nextAllowed := l.last.Add(l.interval)
	if now.Before(nextAllowed) {
		timer := time.NewTimer(nextAllowed.Sub(now))
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	l.last = time.Now()
	return nil
}

func doRequestWithRetries(ctx context.Context, client *http.Client, limiter *rateLimiter, req *http.Request, retries int) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			if lastResp != nil && lastResp.Body != nil {
				lastResp.Body.Close()
			}
			return nil, ctx.Err()
		}

		if err := limiter.wait(ctx); err != nil {
			if lastResp != nil && lastResp.Body != nil {
				lastResp.Body.Close()
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
			lastResp.Body.Close()
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
