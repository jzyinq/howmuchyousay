package crawler

import "time"

// RawProduct represents a product extracted from a web page before validation.
// This is the intermediate format used by extractors and AI before persisting.
type RawProduct struct {
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	ImageURL  string  `json:"image_url"`
	SourceURL string  `json:"source_url"`
}

// CrawlConfig holds settings for a single crawl run.
type CrawlConfig struct {
	// URL is the shop URL to crawl.
	URL string
	// Timeout is the maximum duration for the entire crawl.
	Timeout time.Duration
	// MinProducts is the minimum number of products to collect.
	MinProducts int
	// Verbose enables extra stdout output (for CLI mode).
	Verbose bool
}

// DefaultCrawlConfig returns config with sensible defaults.
func DefaultCrawlConfig(url string) CrawlConfig {
	return CrawlConfig{
		URL:         url,
		Timeout:     90 * time.Second,
		MinProducts: 20,
		Verbose:     false,
	}
}

// ProgressFunc is called during crawling to report progress.
// Used by CLI for stdout output and by server for WebSocket updates.
type ProgressFunc func(event string, message string)
