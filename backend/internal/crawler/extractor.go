package crawler

import (
	"regexp"
	"strconv"
	"strings"
)

// pricePattern matches price-like numbers (e.g., "5999.99", "4999,99").
// Compiled once at package level to avoid recompilation per call.
var pricePattern = regexp.MustCompile(`\d{2,}[.,]\d{2}`)

// ExtractProductsFromPage extracts product data from a Firecrawl PageResult
// using available metadata. The Firecrawl SDK exposes ogTitle, ogImage,
// ogDescription, title, and description — but not og:type or product:price:amount.
// Therefore, this function checks if we have a title (from OG or page metadata)
// and can find a single clear price in the markdown. When both are present,
// it returns a product without needing AI. Returns nil otherwise, deferring
// to AI extraction in the crawler orchestrator.
func ExtractProductsFromPage(page PageResult) []RawProduct {
	name := page.Metadata["ogTitle"]
	if name == "" {
		name = page.Metadata["title"]
	}
	if name == "" {
		return nil
	}

	// Look for a single price in the markdown — if there's exactly one,
	// we can confidently associate it with the product title.
	prices := pricePattern.FindAllString(page.Markdown, 2)
	if len(prices) != 1 {
		// Zero or multiple prices — too ambiguous for metadata extraction.
		return nil
	}

	price := parsePriceValue(prices[0])
	if price <= 0 {
		return nil
	}

	imageURL := page.Metadata["ogImage"]

	return []RawProduct{{
		Name:      name,
		Price:     price,
		ImageURL:  imageURL,
		SourceURL: page.URL,
	}}
}

// productKeywords are e-commerce keywords used to detect product pages.
var productKeywords = []string{
	"add to cart", "buy now", "add to basket", "dodaj do koszyka",
	"kup teraz", "cena", "price", "zł", "pln",
}

// HasProductSignals checks if a page likely contains product information.
func HasProductSignals(page PageResult) bool {
	text := strings.ToLower(page.Markdown + " " + page.Metadata["title"])

	for _, kw := range productKeywords {
		if strings.Contains(text, kw) {
			return true
		}
	}

	if pricePattern.MatchString(page.Markdown) {
		return true
	}

	return false
}

// PageHasContent checks if a page has enough content to be worth processing.
func PageHasContent(page PageResult) bool {
	return len(strings.TrimSpace(page.Markdown)) > 50
}

// parsePriceValue parses a price string into a float64.
func parsePriceValue(v string) float64 {
	cleaned := strings.ReplaceAll(v, ",", ".")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	price, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0
	}
	return price
}
