package parser

import (
	"bytes"
	"code/internal/models"
	"golang.org/x/net/html"
	stdhtml "html"
	"net/url"
	"strings"
)

type AssetRef struct {
	URL  string
	Type string
}

func cleanText(value string) string {
	value = stdhtml.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func ExtractLinks(body []byte, baseURL string) []string {
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

func ExtractSEO(body []byte) models.SEO {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return models.SEO{}
	}

	seo := models.SEO{}

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if !seo.HasTitle {
					seo.HasTitle = true
					seo.Title = cleanText(textContent(node))
				}

			case "meta":
				if !seo.HasDescription && isMetaDescription(node) {
					seo.HasDescription = true
					seo.Description = strings.TrimSpace(getAttr(node, "content"))
				}

			case "h1":
				if !seo.HasH1 {
					seo.HasH1 = true
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(doc)

	return seo
}

func textContent(node *html.Node) string {
	var builder strings.Builder

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(node)

	return builder.String()
}

func isMetaDescription(node *html.Node) bool {
	return strings.EqualFold(getAttr(node, "name"), "description")
}

func getAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}

	return ""
}

func ExtractAssets(body []byte, baseURL string) []AssetRef {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil
	}

	var assets []AssetRef

	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "img":
				if src := normalizeLink(getAttr(node, "src"), base); src != "" {
					assets = append(assets, AssetRef{
						URL:  src,
						Type: "image",
					})
				}

			case "script":
				if src := normalizeLink(getAttr(node, "src"), base); src != "" {
					assets = append(assets, AssetRef{
						URL:  src,
						Type: "script",
					})
				}

			case "link":
				rel := strings.ToLower(getAttr(node, "rel"))
				if strings.Contains(rel, "stylesheet") {
					if href := normalizeLink(getAttr(node, "href"), base); href != "" {
						assets = append(assets, AssetRef{
							URL:  href,
							Type: "style",
						})
					}
				}
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(doc)

	return assets
}

func NormalizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}

	parsed.Fragment = ""

	if parsed.Path == "/" {
		parsed.Path = ""
	}

	return parsed.String()
}
