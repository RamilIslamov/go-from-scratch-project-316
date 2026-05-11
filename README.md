### Hexlet tests and linter status:

[![Actions Status](https://github.com/RamilIslamov/go-from-scratch-project-316/actions/workflows/hexlet-check.yml/badge.svg)](https://github.com/RamilIslamov/go-from-scratch-project-316/actions)

# hexlet-go-crawler

CLI utility for analyzing a website structure.

## Requirements

- Go 1.22+
- Make

## Crawl depth

The `--depth` option controls how many internal page levels are included in the report.

Depth is counted from the root URL:

- `--depth 1` analyzes only the root page.
- `--depth 2` analyzes the root page and internal links found on it.
- `--depth 3` also analyzes internal links found on depth 1 pages.

External links are not crawled as pages, but they can still be checked and reported as broken links.

Example:

```bash
```go run ./cmd/hexlet-go-crawler --depth 2 https://example.com```
```

## Request rate limiting

The crawler can limit request speed globally for the whole crawling process.

You can use a fixed delay between HTTP requests:

```bash
```go run ./cmd/hexlet-go-crawler --delay 200ms https://example.com```
```

Supported duration examples:

- 200ms
- 1s
- 2s

If both --delay and --rps are provided, --rps has priority.

Example:
go run ./cmd/hexlet-go-crawler --delay 1s --rps 5 https://example.com

## Retries

The crawler can retry temporary request failures.

Use `--retries` to control the maximum number of retry attempts:

```bash
go run ./cmd/hexlet-go-crawler --retries 2 https://example.com
```

`--retries 2` means:

- 1 initial request
- up to 2 retry attempts
- 3 total attempts maximum

Retries are performed only for temporary failures:

- network errors
- `429 Too Many Requests`
- `5xx` server errors

Non-temporary responses like `404 Not Found` are not retried.

If a retry eventually succeeds, the final report uses the successful result. If all attempts fail,
the report contains the last error or status code.

## Assets report

The crawler collects information about static assets found on each HTML page.

Supported asset types:

- `image` from `<img src="...">`
- `script` from `<script src="...">`
- `style` from `<link rel="stylesheet" href="...">`

Each asset in the JSON report contains:

```json
{
  "url": "https://example.com/static/logo.png",
  "type": "image",
  "status_code": 200,
  "size_bytes": 12345,
  "error": ""
}
```

Asset size is calculated using the `Content-Length` response header. If `Content-Length` is missing,
the crawler reads the response body and calculates the size from it.

If the same asset is found on multiple pages, it is requested only once and reused from cache.

## JSON report format

The crawler outputs a JSON report with the analyzed website structure.

Example:

```json
{
  "root_url": "https://example.com",
  "depth": 1,
  "generated_at": "2024-06-01T12:34:56Z",
  "pages": [
    {
      "url": "https://example.com",
      "depth": 0,
      "http_status": 200,
      "status": "ok",
      "error": "",
      "seo": {
        "has_title": true,
        "title": "Example title",
        "has_description": true,
        "description": "Example description",
        "has_h1": true
      },
      "broken_links": [
        {
          "url": "https://example.com/missing",
          "status_code": 404,
          "error": "Not Found"
        }
      ],
      "assets": [
        {
          "url": "https://example.com/static/logo.png",
          "type": "image",
          "status_code": 200,
          "size_bytes": 12345,
          "error": ""
        }
      ],
      "discovered_at": "2024-06-01T12:34:56Z"
    }
  ]
}
```

### Fields

- `root_url` — starting URL passed to the crawler.
- `depth` — crawl depth option used for the run.
- `generated_at` — report generation time in ISO8601 format.
- `pages` — list of crawled internal pages.
- `pages[].url` — page URL.
- `pages[].depth` — distance from the root URL.
- `pages[].http_status` — HTTP status code of the page request.
- `pages[].status` — page analysis status, usually `ok` or `error`.
- `pages[].error` — error message, or an empty string if there is no error.
- `pages[].seo` — basic SEO metadata.
- `pages[].broken_links` — links that returned HTTP `4xx`/`5xx` or network errors.
- `pages[].assets` — static assets found on the page.
- `pages[].discovered_at` — time when the page was processed in ISO8601 format.

All JSON keys are always present. Empty values are represented as empty strings, empty arrays, zero values, or `false`.

## Install dependencies

```bash
go mod tidy
```