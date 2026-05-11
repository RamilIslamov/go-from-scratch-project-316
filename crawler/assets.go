package crawler

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strconv"
)

func checkAssets(
	ctx context.Context,
	client *http.Client,
	limiter *rateLimiter,
	opts Options,
	assetRefs []assetRef,
	cache map[string]Asset,
) []Asset {
	assets := []Asset{}

	for _, ref := range assetRefs {
		if cached, ok := cache[ref.URL]; ok {
			assets = append(assets, cached)
			continue
		}

		asset := fetchAsset(ctx, client, limiter, opts, ref)
		cache[ref.URL] = asset
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
	opts Options,
	ref assetRef,
) Asset {
	asset := Asset{
		URL:   ref.URL,
		Type:  ref.Type,
		Error: "",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ref.URL, nil)
	if err != nil {
		asset.Error = err.Error()
		return asset
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	}

	resp, err := doRequestWithRetries(ctx, client, limiter, req, opts.Retries)
	if err != nil {
		asset.Error = err.Error()
		return asset
	}

	if resp == nil {
		asset.Error = "empty response"
		return asset
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	asset.StatusCode = resp.StatusCode

	size, sizeErr := assetSize(resp)
	asset.SizeBytes = size

	if resp.StatusCode >= 400 {
		asset.Error = responseStatusText(resp)
		return asset
	}

	if sizeErr != nil {
		asset.Error = sizeErr.Error()
	}

	return asset
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
