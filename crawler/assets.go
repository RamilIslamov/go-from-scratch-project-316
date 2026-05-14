package crawler

import (
	"code/internal/fetcher"
	"code/internal/models"
	"code/internal/parser"
	"context"
	"net/http"
	"sort"
	"sync"
)

func checkAssets(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts models.Options,
	assetRefs []parser.AssetRef,
	cache map[string]models.Asset,
	cacheMu *sync.Mutex,
) []models.Asset {
	assets := []models.Asset{}

	for _, ref := range assetRefs {
		cacheMu.Lock()
		cached, ok := cache[ref.URL]
		cacheMu.Unlock()

		if ok {
			assets = append(assets, cached)
			continue
		}

		asset := fetchAsset(ctx, client, limiter, opts, ref)

		cacheMu.Lock()
		cache[ref.URL] = asset
		cacheMu.Unlock()

		assets = append(assets, asset)
	}

	sort.SliceStable(assets, func(i, j int) bool {
		return assetTypeOrder(assets[i].Type) < assetTypeOrder(assets[j].Type)
	})

	return assets
}

func assetTypeOrder(assetType string) int {
	switch assetType {
	case "image":
		return 0
	case "script":
		return 1
	case "style":
		return 2
	default:
		return 3
	}
}

func fetchAsset(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts models.Options,
	ref parser.AssetRef,
) models.Asset {
	result := fetcher.FetchAsset(ctx, client, limiter, ref.URL, fetcher.Options{
		Retries:   opts.Retries,
		UserAgent: opts.UserAgent,
	})

	return models.Asset{
		URL:        ref.URL,
		Type:       ref.Type,
		StatusCode: result.StatusCode,
		SizeBytes:  result.SizeBytes,
		Error:      result.Error,
	}
}
