package opengraph

import (
	"context"
	"fmt"
	"io"
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

// Fetch retrieves Open Graph metadata from the given URL.
// Falls back to <title> if og:title is missing, and checks twitter:* meta tags.
// Returns nil, nil if no useful metadata is found.
func Fetch(ctx context.Context, rawURL string) (*Card, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Blackwood/1.0 (link preview)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Only read the head section — limit to 256 KB to avoid downloading huge pages.
	limited := io.LimitReader(resp.Body, 256*1024)
	return parseHead(limited, rawURL)
}

// parseHead tokenizes HTML and extracts OG/Twitter meta tags and <title>.
// Stops at </head> or <body> to avoid parsing the full document.
func parseHead(r io.Reader, pageURL string) (*Card, error) {
	tokenizer := html.NewTokenizer(r)

	var (
		ogTitle, ogDesc, ogImage, ogSiteName, ogURL string
		twTitle, twDesc, twImage                    string
		titleTag                                    string
		inTitle                                     bool
	)

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			// EOF or read error — build card from what we have.
			return buildCard(ogTitle, ogDesc, ogImage, ogSiteName, ogURL,
				twTitle, twDesc, twImage, titleTag, pageURL), nil

		case html.StartTagToken, html.SelfClosingTagToken:
			tn, hasAttr := tokenizer.TagName()
			tagName := string(tn)

			// Stop parsing at <body>.
			if tagName == "body" {
				return buildCard(ogTitle, ogDesc, ogImage, ogSiteName, ogURL,
					twTitle, twDesc, twImage, titleTag, pageURL), nil
			}

			if tagName == "title" {
				inTitle = true
				continue
			}

			if tagName == "meta" && hasAttr {
				property, name, content := extractMetaAttrs(tokenizer)

				switch property {
				case "og:title":
					ogTitle = content
				case "og:description":
					ogDesc = content
				case "og:image":
					ogImage = content
				case "og:site_name":
					ogSiteName = content
				case "og:url":
					ogURL = content
				}

				switch name {
				case "twitter:title":
					twTitle = content
				case "twitter:description":
					twDesc = content
				case "twitter:image":
					twImage = content
				}
			}

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tagName := string(tn)

			if tagName == "title" {
				inTitle = false
			}
			if tagName == "head" {
				return buildCard(ogTitle, ogDesc, ogImage, ogSiteName, ogURL,
					twTitle, twDesc, twImage, titleTag, pageURL), nil
			}

		case html.TextToken:
			if inTitle {
				titleTag += string(tokenizer.Text())
			}
		}
	}
}

// extractMetaAttrs reads attributes from a <meta> tag and returns
// the property, name, and content values.
func extractMetaAttrs(tokenizer *html.Tokenizer) (property, name, content string) {
	for {
		key, val, more := tokenizer.TagAttr()
		k := string(key)
		v := string(val)
		switch k {
		case "property":
			property = v
		case "name":
			name = v
		case "content":
			content = v
		}
		if !more {
			break
		}
	}
	return
}

// buildCard assembles a Card from the collected metadata, applying fallbacks.
// Returns nil if no useful metadata was found.
func buildCard(ogTitle, ogDesc, ogImage, ogSiteName, ogURL,
	twTitle, twDesc, twImage, titleTag, pageURL string) *Card {

	title := firstNonEmpty(ogTitle, twTitle, strings.TrimSpace(titleTag))
	desc := firstNonEmpty(ogDesc, twDesc)
	image := firstNonEmpty(ogImage, twImage)
	cardURL := firstNonEmpty(ogURL, pageURL)

	if title == "" && desc == "" && image == "" {
		return nil
	}

	// Resolve relative image URL against the page URL.
	if image != "" {
		image = resolveURL(image, pageURL)
	}

	return &Card{
		Title:       title,
		Description: desc,
		Image:       image,
		SiteName:    ogSiteName,
		URL:         cardURL,
	}
}

// resolveURL resolves a potentially relative URL against a base URL.
func resolveURL(raw, base string) string {
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return raw
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(ref).String()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
