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

type crawlItem struct {
	URL   string
	Depth int
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

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

		body, err := fetchPage(ctx, client, opts, &page)
		if err == nil && page.Status == "ok" {
			page.SEO = extractSEO(body)

			links := extractLinks(body, page.URL)
			page.BrokenLinks = checkBrokenLinks(ctx, client, opts, links)

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
