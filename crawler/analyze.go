package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"code/internal/fetcher"
	"code/internal/models"
	"code/internal/parser"
)

type pageResult struct {
	Page  models.Page
	Links []string
}

type crawlResult struct {
	Item   models.CrawlItem
	Result pageResult
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	rootURL := parser.NormalizeURL(opts.URL)
	limiter := newRateLimiter(opts)
	assetCache := make(map[string]models.Asset)
	linkCache := make(map[string]*models.BrokenLink)

	var assetCacheMu sync.Mutex
	var linkCacheMu sync.Mutex

	report := models.Report{
		RootURL:     rootURL,
		Depth:       opts.Depth,
		GeneratedAt: time.Now(),
		Pages:       []models.Page{},
	}

	visited := make(map[string]bool)

	queue := []models.CrawlItem{
		{
			URL:   rootURL,
			Depth: 0,
		},
	}

	for len(queue) > 0 {
		if ctx.Err() != nil {
			break
		}

		batch := []models.CrawlItem{}
		seenInBatch := make(map[string]bool)

		for _, item := range queue {
			item.URL = parser.NormalizeURL(item.URL)

			if visited[item.URL] || seenInBatch[item.URL] {
				continue
			}

			visited[item.URL] = true
			seenInBatch[item.URL] = true
			batch = append(batch, item)
		}

		queue = []models.CrawlItem{}

		if len(batch) == 0 {
			continue
		}

		results := crawlWithWorkers(
			ctx,
			client,
			limiter,
			opts,
			batch,
			assetCache,
			linkCache,
			&assetCacheMu,
			&linkCacheMu,
		)

		for _, result := range results {
			if result.Result.Page.Status == "ok" {
				queue = appendInternalLinks(
					queue,
					visited,
					rootURL,
					result.Result.Links,
					result.Item.Depth,
					opts.Depth,
				)
			}

			report.Pages = append(report.Pages, result.Result.Page)
		}
	}

	sort.SliceStable(report.Pages, func(i, j int) bool {
		if report.Pages[i].Depth != report.Pages[j].Depth {
			return report.Pages[i].Depth < report.Pages[j].Depth
		}

		return report.Pages[i].URL < report.Pages[j].URL
	})

	if opts.IndentJSON {
		return json.MarshalIndent(report, "", "  ")
	}

	return json.Marshal(report)
}

func analyzePage(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts models.Options,
	item models.CrawlItem,
	assetCache map[string]models.Asset,
	linkCache map[string]*models.BrokenLink,
	assetCacheMu *sync.Mutex,
	linkCacheMu *sync.Mutex,
) pageResult {
	page := models.Page{
		URL:          item.URL,
		Depth:        item.Depth,
		DiscoveredAt: time.Now(),
	}

	result := fetcher.FetchPage(ctx, client, limiter, page.URL, fetcher.Options{
		Retries:   opts.Retries,
		UserAgent: opts.UserAgent,
	})

	page.HTTPStatus = result.StatusCode
	page.Status = result.Status
	page.Error = result.Error

	if page.Status != "ok" {
		return pageResult{
			Page: page,
		}
	}

	body := result.Body

	seo := parser.ExtractSEO(body)
	page.SEO = models.SEO{
		HasTitle:       seo.HasTitle,
		Title:          seo.Title,
		HasDescription: seo.HasDescription,
		Description:    seo.Description,
		HasH1:          seo.HasH1,
	}

	links := parser.ExtractLinks(body, page.URL)
	page.BrokenLinks = checkBrokenLinks(ctx, client, limiter, opts, links, linkCache, linkCacheMu)

	assetRefs := parser.ExtractAssets(body, page.URL)
	page.Assets = checkAssets(ctx, client, limiter, opts, assetRefs, assetCache, assetCacheMu)

	return pageResult{
		Page:  page,
		Links: links,
	}
}

func appendInternalLinks(
	queue []models.CrawlItem,
	visited map[string]bool,
	rootURL string,
	links []string,
	currentDepth int,
	maxDepth int,
) []models.CrawlItem {
	nextDepth := currentDepth + 1
	if nextDepth >= maxDepth {
		return queue
	}

	seen := make(map[string]bool)
	internalLinks := []string{}

	for _, link := range links {
		link = parser.NormalizeURL(link)

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
		queue = append(queue, models.CrawlItem{
			URL:   link,
			Depth: nextDepth,
		})
	}

	return queue
}

func crawlWithWorkers(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts models.Options,
	items []models.CrawlItem,
	assetCache map[string]models.Asset,
	linkCache map[string]*models.BrokenLink,
	assetCacheMu *sync.Mutex,
	linkCacheMu *sync.Mutex,
) []crawlResult {
	workers := workersCount(opts)
	if workers > len(items) {
		workers = len(items)
	}

	jobs := make(chan models.CrawlItem)
	results := make(chan crawlResult)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for item := range jobs {
				if ctx.Err() != nil {
					return
				}

				result := analyzePage(
					ctx,
					client,
					limiter,
					opts,
					item,
					assetCache,
					linkCache,
					assetCacheMu,
					linkCacheMu,
				)

				results <- crawlResult{
					Item:   item,
					Result: result,
				}
			}
		}()
	}

	go func() {
		defer close(jobs)

		for _, item := range items {
			select {
			case jobs <- item:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	collected := []crawlResult{}
	for result := range results {
		collected = append(collected, result)
	}

	return collected
}

func workersCount(opts models.Options) int {
	if opts.Concurrency <= 0 {
		return 1
	}

	return opts.Concurrency
}
