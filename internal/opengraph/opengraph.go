package opengraph

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Card holds Open Graph metadata extracted from a web page.
type Card struct {
	Title       string
	Description string
	Image       string // URL to og:image
	SiteName    string
	URL         string
}

// Fetch retrieves the page at rawURL and extracts Open Graph metadata.
// Returns nil, nil if no metadata is found.
func Fetch(ctx context.Context, rawURL string) (*Card, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Blackwood/1.0 (web clipper)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	card := extractMeta(doc)

	// Fallback to <title> if no og:title or twitter:title found.
	if card.Title == "" {
		card.Title = extractTitle(doc)
	}

	// Nothing useful found.
	if card.Title == "" && card.Description == "" {
		return nil, nil
	}

	// Resolve relative og:image URLs against the page URL.
	if card.Image != "" {
		if imgURL, err := url.Parse(card.Image); err == nil && !imgURL.IsAbs() {
			card.Image = parsed.ResolveReference(imgURL).String()
		}
	}

	if card.URL == "" {
		card.URL = rawURL
	}

	return card, nil
}

// extractMeta walks the HTML tree looking for og:* and twitter:* meta tags.
func extractMeta(n *html.Node) *Card {
	card := &Card{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var property, name, content string
			for _, a := range n.Attr {
				switch a.Key {
				case "property":
					property = a.Val
				case "name":
					name = a.Val
				case "content":
					content = a.Val
				}
			}

			// og:* tags (preferred)
			switch property {
			case "og:title":
				if card.Title == "" {
					card.Title = content
				}
			case "og:description":
				if card.Description == "" {
					card.Description = content
				}
			case "og:image":
				if card.Image == "" {
					card.Image = content
				}
			case "og:site_name":
				if card.SiteName == "" {
					card.SiteName = content
				}
			case "og:url":
				if card.URL == "" {
					card.URL = content
				}
			}

			// twitter:* fallback
			key := name
			if key == "" {
				key = property
			}
			switch key {
			case "twitter:title":
				if card.Title == "" {
					card.Title = content
				}
			case "twitter:description":
				if card.Description == "" {
					card.Description = content
				}
			case "twitter:image":
				if card.Image == "" {
					card.Image = content
				}
			}

			// Standard meta description fallback.
			if name == "description" && card.Description == "" {
				card.Description = content
			}
		}

		// Stop after </head> — no need to parse the body.
		if n.Type == html.ElementNode && n.Data == "body" {
			return
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return card
}

// extractTitle finds the first <title> element text.
func extractTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		var sb strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				sb.WriteString(c.Data)
			}
		}
		return strings.TrimSpace(sb.String())
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if t := extractTitle(c); t != "" {
			return t
		}
	}
	return ""
}
