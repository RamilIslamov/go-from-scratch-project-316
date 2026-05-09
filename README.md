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
go run ./cmd/hexlet-go-crawler --depth 2 https://example.com
```

## Install dependencies

```bash
go mod tidy
```