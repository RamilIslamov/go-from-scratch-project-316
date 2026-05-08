package crawler

import (
	"bytes"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func extractLinks(body []byte, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	var links []string

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, attr := range node.Attr {
				if attr.Key == "href" {
					link := normalizeLink(attr.Val, base)
					if link != "" {
						links = append(links, link)
					}
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(doc)

	return links
}

func normalizeLink(rawLink string, base *url.URL) string {
	rawLink = strings.TrimSpace(rawLink)
	if rawLink == "" {
		return ""
	}

	if strings.HasPrefix(rawLink, "#") {
		return ""
	}

	parsed, err := url.Parse(rawLink)
	if err != nil {
		return ""
	}

	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}

	resolved := base.ResolveReference(parsed)

	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}

	resolved.Fragment = ""

	return resolved.String()
}
