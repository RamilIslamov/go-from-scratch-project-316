package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"
)

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	rootURL := normalizeURL(opts.URL)
	limiter := newRateLimiter(opts)
	assetCache := make(map[string]Asset)

	report := Report{
		RootURL:     rootURL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now(),
		Pages:       []Page{},
	}

	visited := make(map[string]bool)

	queue := []crawlItem{
		{
			URL:   rootURL,
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
			DiscoveredAt: time.Now(),
		}

		body, err := fetchPage(ctx, client, limiter, opts, &page)
		if err == nil && page.Status == "ok" {
			page.SEO = extractSEO(body)

			links := extractLinks(body, page.URL)
			page.BrokenLinks = checkBrokenLinks(ctx, client, limiter, opts, links)

			assetRefs := extractAssets(body, page.URL)
			page.Assets = checkAssets(ctx, client, limiter, opts, assetRefs, assetCache)

			queue = appendInternalLinks(queue, visited, rootURL, links, item.Depth, opts.Depth)
		}

		report.Pages = append(report.Pages, page)
	}

	if opts.IndentJSON {
		return json.MarshalIndent(report, "", "  ")
	}

	return json.Marshal(report)
}

func appendInternalLinks(
	queue []crawlItem,
	visited map[string]bool,
	rootURL string,
	links []string,
	currentDepth int,
	maxDepth int,
) []crawlItem {
	nextDepth := currentDepth + 1
	if nextDepth >= maxDepth {
		return queue
	}

	seen := make(map[string]bool)
	internalLinks := []string{}

	for _, link := range links {
		link = normalizeURL(link)

		if !isInternalLink(rootURL, link) {
			continue
		}

		if visited[link] || seen[link] {
			continue
		}

		seen[link] = true
		internalLinks = append(internalLinks, link)
	}

	sort.Strings(internalLinks)

	for _, link := range internalLinks {
		queue = append(queue, crawlItem{
			URL:   link,
			Depth: nextDepth,
		})
	}

	return queue
}
