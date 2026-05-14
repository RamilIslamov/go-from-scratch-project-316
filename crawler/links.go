package crawler

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"code/internal/fetcher"
	"code/internal/models"
	"code/internal/parser"
)

func checkBrokenLinks(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts models.Options,
	links []string,
	cache map[string]*models.BrokenLink,
	cacheMu *sync.Mutex,
) []models.BrokenLink {
	brokenLinks := []models.BrokenLink{}
	seen := make(map[string]bool)

	for _, link := range links {
		link = parser.NormalizeURL(link)

		if seen[link] {
			continue
		}
		seen[link] = true

		cacheMu.Lock()
		cached, ok := cache[link]
		cacheMu.Unlock()

		if ok {
			if cached != nil {
				brokenLinks = append(brokenLinks, *cached)
			}
			continue
		}

		result := fetcher.CheckLink(ctx, client, limiter, link, fetcher.Options{
			Retries:   opts.Retries,
			UserAgent: opts.UserAgent,
		})

		if result.Error != "" {
			brokenLink := models.BrokenLink{
				URL:   result.URL,
				Error: result.Error,
			}

			cacheMu.Lock()
			cache[link] = &brokenLink
			cacheMu.Unlock()

			brokenLinks = append(brokenLinks, brokenLink)
			continue
		}

		if result.StatusCode >= 400 {
			brokenLink := models.BrokenLink{
				URL:        result.URL,
				StatusCode: result.StatusCode,
			}

			cacheMu.Lock()
			cache[link] = &brokenLink
			cacheMu.Unlock()

			brokenLinks = append(brokenLinks, brokenLink)
			continue
		}

		cacheMu.Lock()
		cache[link] = nil
		cacheMu.Unlock()
	}

	return brokenLinks
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
