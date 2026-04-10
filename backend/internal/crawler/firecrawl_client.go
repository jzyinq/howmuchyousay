package crawler

import (
	"context"
	"fmt"
	"strconv"

	firecrawl "github.com/mendableai/firecrawl-go/v2"
)

// PageResult represents a single page returned by a Firecrawl crawl.
type PageResult struct {
	// URL is the source URL of the crawled page.
	URL string
	// Markdown is the page content converted to markdown by Firecrawl.
	Markdown string
	// Metadata contains page metadata (title, description, ogImage, etc.).
	Metadata map[string]string
}

// FirecrawlCrawler defines the interface for crawling a site via Firecrawl.
type FirecrawlCrawler interface {
	CrawlSite(ctx context.Context, siteURL string, cfg CrawlConfig) ([]PageResult, error)
}

// FirecrawlClient implements FirecrawlCrawler using the firecrawl-go SDK.
type FirecrawlClient struct {
	app *firecrawl.FirecrawlApp
}

// NewFirecrawlClient creates a new FirecrawlClient with the given API key and URL.
// Returns an error if the SDK client cannot be created (e.g., empty API key).
func NewFirecrawlClient(apiKey, apiURL string) (*FirecrawlClient, error) {
	app, err := firecrawl.NewFirecrawlApp(apiKey, apiURL)
	if err != nil {
		return nil, fmt.Errorf("creating firecrawl app: %w", err)
	}
	return &FirecrawlClient{app: app}, nil
}

// CrawlSite crawls the given site URL using the Firecrawl API and returns
// the crawled pages as PageResult slices. It uses CrawlConfig to set crawl
// parameters like timeout (via context) and depth limits.
//
// Because the firecrawl-go SDK's CrawlURL does not accept a context.Context,
// we run the call in a goroutine and select on the context so that
// cancellation and deadlines are still respected.
//
// NOTE: When the context is cancelled, the background goroutine running
// CrawlURL continues until the SDK's internal HTTP polling completes or
// times out. This is a bounded leak (the goroutine will eventually finish),
// but the Firecrawl API job may continue running server-side. A future
// improvement would use AsyncCrawlURL + manual polling with context checks
// and CancelCrawl on cancellation.
func (fc *FirecrawlClient) CrawlSite(ctx context.Context, siteURL string, cfg CrawlConfig) ([]PageResult, error) {
	onlyMainContent := true
	maxDepth := cfg.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 3
	}
	limit := cfg.MinProducts * 3
	if limit < 10 {
		limit = 10
	}
	params := &firecrawl.CrawlParams{
		MaxDepth: &maxDepth,
		Limit:    &limit,
		ExcludePaths: []string{
			"blog/*",
			"about/*",
			"contact/*",
			"faq/*",
			"help/*",
			"terms/*",
			"privacy/*",
			"career*",
		},
		ScrapeOptions: firecrawl.ScrapeParams{
			Formats:         []string{"markdown"},
			OnlyMainContent: &onlyMainContent,
		},
	}

	type crawlResult struct {
		resp *firecrawl.CrawlStatusResponse
		err  error
	}
	ch := make(chan crawlResult, 1)

	go func() {
		resp, err := fc.app.CrawlURL(siteURL, params, nil)
		ch <- crawlResult{resp: resp, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("firecrawl crawl: %w", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("firecrawl crawl: %w", res.err)
		}
		if res.resp == nil {
			return nil, fmt.Errorf("firecrawl returned nil response")
		}
		return convertDocuments(res.resp.Data), nil
	}
}

// convertDocuments transforms Firecrawl SDK documents into PageResult values.
// It skips nil document pointers and safely handles nil metadata fields.
func convertDocuments(data []*firecrawl.FirecrawlDocument) []PageResult {
	if len(data) == 0 {
		return nil
	}

	results := make([]PageResult, 0, len(data))
	for _, doc := range data {
		if doc == nil {
			continue
		}

		pr := PageResult{
			Markdown: doc.Markdown,
			Metadata: make(map[string]string),
		}

		if doc.Metadata != nil {
			meta := doc.Metadata

			// Determine the page URL: prefer SourceURL, fall back to URL.
			if meta.SourceURL != nil {
				pr.URL = *meta.SourceURL
			} else if meta.URL != nil {
				pr.URL = *meta.URL
			}

			// Populate metadata map with non-nil fields.
			if meta.Title != nil {
				pr.Metadata["title"] = *meta.Title
			}
			if meta.SourceURL != nil {
				pr.Metadata["sourceURL"] = *meta.SourceURL
			}
			if meta.StatusCode != nil {
				pr.Metadata["statusCode"] = strconv.Itoa(*meta.StatusCode)
			}
			if meta.Description != nil && len(*meta.Description) > 0 {
				pr.Metadata["description"] = (*meta.Description)[0]
			}
			if meta.OGTitle != nil && len(*meta.OGTitle) > 0 {
				pr.Metadata["ogTitle"] = (*meta.OGTitle)[0]
			}
			if meta.OGImage != nil && len(*meta.OGImage) > 0 {
				pr.Metadata["ogImage"] = (*meta.OGImage)[0]
			}
			if meta.OGDescription != nil && len(*meta.OGDescription) > 0 {
				pr.Metadata["ogDescription"] = (*meta.OGDescription)[0]
			}
		}

		results = append(results, pr)
	}

	return results
}
