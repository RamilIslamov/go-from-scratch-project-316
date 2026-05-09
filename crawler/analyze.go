package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type Options struct {
	URL         string
	Depth       int
	Retries     int
	Delay       time.Duration
	Timeout     time.Duration
	UserAgent   string
	Concurrency int
	IndentJSON  bool
	HTTPClient  *http.Client
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

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	page := Page{
		URL:          opts.URL,
		Depth:        0,
		BrokenLinks:  []BrokenLink{},
		DiscoveredAt: time.Now(),
	}

	body, err := fetchPage(ctx, client, opts, &page)
	if err == nil && page.Status == "ok" {
		page.SEO = extractSEO(body)
		links := extractLinks(body, page.URL)
		page.BrokenLinks = checkBrokenLinks(ctx, client, opts, links)
	}

	report := Report{
		RootURL:     opts.URL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now(),
		Pages:       []Page{page},
	}

	if opts.IndentJSON {
		return json.MarshalIndent(report, "", "  ")
	}

	return json.Marshal(report)
}

func fetchPage(ctx context.Context, client *http.Client, opts Options, page *Page) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, page.URL, nil)
	if err != nil {
		page.Status = "error"
		page.Error = err.Error()
		return nil, err
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := client.Do(req)
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

func checkBrokenLinks(ctx context.Context, client *http.Client, opts Options, links []string) []BrokenLink {
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

		resp, err := client.Do(req)
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
