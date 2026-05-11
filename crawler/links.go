package crawler

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

func checkBrokenLinks(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts Options,
	links []string,
) []BrokenLink {
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
