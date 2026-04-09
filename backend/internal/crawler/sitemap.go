package crawler

import (
	"context"
	"encoding/xml"
	"math/rand/v2"
	"net/url"
	"strings"
)

// maxSitemapURLs is the maximum number of URLs to collect from sitemaps.
const maxSitemapURLs = 500

// maxSubSitemaps is the maximum number of sub-sitemaps to fetch from a sitemap index.
const maxSubSitemaps = 5

// --- XML types for sitemap protocol (sitemaps.org/protocol.html) ---

// sitemapIndex represents a <sitemapindex> element containing nested sitemaps.
type sitemapIndex struct {
	XMLName  xml.Name       `xml:"sitemapindex"`
	Sitemaps []sitemapEntry `xml:"sitemap"`
}

// sitemapEntry represents a single <sitemap> within a sitemap index.
type sitemapEntry struct {
	Loc string `xml:"loc"`
}

// urlSet represents a <urlset> element containing page URLs.
type urlSet struct {
	XMLName xml.Name   `xml:"urlset"`
	URLs    []urlEntry `xml:"url"`
}

// urlEntry represents a single <url> within a urlset.
type urlEntry struct {
	Loc string `xml:"loc"`
}

// SampleURLs returns a random sample of n URLs from the given slice.
// If len(urls) <= n, returns a copy of the entire slice.
// Uses Fisher-Yates shuffle for unbiased sampling.
func SampleURLs(urls []string, n int) []string {
	if len(urls) == 0 || n <= 0 {
		return nil
	}

	if len(urls) <= n {
		result := make([]string, len(urls))
		copy(result, urls)
		return result
	}

	// Fisher-Yates shuffle on a copy, then take first n
	shuffled := make([]string, len(urls))
	copy(shuffled, urls)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.IntN(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:n]
}

// FetchSitemapURLs tries to discover URLs from the site's sitemap.xml.
// Returns collected URLs (up to maxSitemapURLs) or nil if no sitemap found.
// Errors from fetching are treated as "no sitemap" (returns nil, nil).
func FetchSitemapURLs(ctx context.Context, fetcher Fetcher, baseURL string) ([]string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil
	}

	sitemapURL := parsed.Scheme + "://" + parsed.Host + "/sitemap.xml"

	urls, err := fetchAndParseSitemap(ctx, fetcher, sitemapURL, parsed.Host)
	if err == nil && len(urls) > 0 {
		return urls, nil
	}

	// Fallback: try to find Sitemap directive in robots.txt
	robotsURL := parsed.Scheme + "://" + parsed.Host + "/robots.txt"
	robotsBody, err := fetcher.Fetch(ctx, robotsURL)
	if err != nil {
		return nil, nil
	}

	sitemapLocs := extractSitemapFromRobotsTxt(robotsBody)
	if len(sitemapLocs) == 0 {
		return nil, nil
	}

	var allURLs []string
	for _, loc := range sitemapLocs {
		if len(allURLs) >= maxSitemapURLs {
			break
		}
		found, err := fetchAndParseSitemap(ctx, fetcher, loc, parsed.Host)
		if err != nil {
			continue
		}
		allURLs = append(allURLs, found...)
	}

	if len(allURLs) == 0 {
		return nil, nil
	}
	if len(allURLs) > maxSitemapURLs {
		allURLs = allURLs[:maxSitemapURLs]
	}
	return allURLs, nil
}

// fetchAndParseSitemap fetches a single sitemap URL and parses it.
// Handles both <sitemapindex> and <urlset> formats.
// Returns same-host URLs only, capped at maxSitemapURLs.
func fetchAndParseSitemap(ctx context.Context, fetcher Fetcher, sitemapURL string, allowedHost string) ([]string, error) {
	body, err := fetcher.Fetch(ctx, sitemapURL)
	if err != nil {
		return nil, err
	}

	return parseSitemapXML(ctx, fetcher, body, allowedHost)
}

// parseSitemapXML parses XML content as either a sitemap index or a urlset.
func parseSitemapXML(ctx context.Context, fetcher Fetcher, xmlContent string, allowedHost string) ([]string, error) {
	// Try <sitemapindex> first
	var index sitemapIndex
	if err := xml.Unmarshal([]byte(xmlContent), &index); err == nil && len(index.Sitemaps) > 0 {
		var allURLs []string
		count := 0
		for _, s := range index.Sitemaps {
			if count >= maxSubSitemaps {
				break
			}
			if s.Loc == "" {
				continue
			}
			count++
			subURLs, err := fetchAndParseSitemap(ctx, fetcher, s.Loc, allowedHost)
			if err != nil {
				continue
			}
			allURLs = append(allURLs, subURLs...)
			if len(allURLs) >= maxSitemapURLs {
				allURLs = allURLs[:maxSitemapURLs]
				break
			}
		}
		return allURLs, nil
	}

	// Try <urlset>
	var us urlSet
	if err := xml.Unmarshal([]byte(xmlContent), &us); err == nil && len(us.URLs) > 0 {
		var urls []string
		seen := make(map[string]bool)
		for _, u := range us.URLs {
			if u.Loc == "" {
				continue
			}
			parsedLoc, err := url.Parse(u.Loc)
			if err != nil {
				continue
			}
			// Same-host filter
			if parsedLoc.Host != allowedHost {
				continue
			}
			if seen[u.Loc] {
				continue
			}
			seen[u.Loc] = true
			urls = append(urls, u.Loc)
			if len(urls) >= maxSitemapURLs {
				break
			}
		}
		return urls, nil
	}

	return nil, nil
}

// extractSitemapFromRobotsTxt extracts Sitemap: directive URLs from robots.txt content.
func extractSitemapFromRobotsTxt(content string) []string {
	var sitemaps []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "sitemap:") {
			loc := strings.TrimSpace(line[len("sitemap:"):])
			if loc != "" {
				sitemaps = append(sitemaps, loc)
			}
		}
	}
	return sitemaps
}
