package crawler

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ExtractProducts extracts product data from HTML using structured data formats.
// Tries JSON-LD first, then Open Graph. Returns all products found.
func ExtractProducts(html string, sourceURL string) []RawProduct {
	var products []RawProduct

	// Try JSON-LD first (highest quality data)
	products = append(products, extractJSONLD(html, sourceURL)...)

	// If JSON-LD found products, return them
	if len(products) > 0 {
		return products
	}

	// Try Open Graph as fallback
	products = append(products, extractOpenGraph(html, sourceURL)...)

	return products
}

// ExtractLinks extracts all <a href="..."> links from HTML, resolving relative URLs.
func ExtractLinks(html string, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	// Simple regex for <a href="..."> - good enough for crawling
	re := regexp.MustCompile(`<a\s+[^>]*href\s*=\s*["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var links []string

	for _, match := range matches {
		href := strings.TrimSpace(match[1])

		// Skip empty, fragment-only, javascript, mailto links
		if href == "" || href == "#" || strings.HasPrefix(href, "#") ||
			strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
			continue
		}

		// Resolve relative URLs
		resolved, err := base.Parse(href)
		if err != nil {
			continue
		}

		fullURL := resolved.String()
		if !seen[fullURL] {
			seen[fullURL] = true
			links = append(links, fullURL)
		}
	}

	return links
}

// --- JSON-LD extraction ---

func extractJSONLD(html string, sourceURL string) []RawProduct {
	// Find all <script type="application/ld+json"> blocks
	re := regexp.MustCompile(`(?s)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)
	matches := re.FindAllStringSubmatch(html, -1)

	var products []RawProduct

	for _, match := range matches {
		jsonStr := strings.TrimSpace(match[1])
		products = append(products, parseJSONLDBlock(jsonStr, sourceURL)...)
	}

	return products
}

func parseJSONLDBlock(jsonStr string, sourceURL string) []RawProduct {
	// Try parsing as a single object first
	var single map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &single); err == nil {
		return extractProductsFromJSONLDObject(single, sourceURL)
	}

	// Try parsing as an array of objects
	var array []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &array); err == nil {
		var products []RawProduct
		for _, obj := range array {
			products = append(products, extractProductsFromJSONLDObject(obj, sourceURL)...)
		}
		return products
	}

	return nil
}

func extractProductsFromJSONLDObject(obj map[string]interface{}, sourceURL string) []RawProduct {
	var products []RawProduct

	// Check for @graph (nested structure)
	if graph, ok := obj["@graph"]; ok {
		if graphArray, ok := graph.([]interface{}); ok {
			for _, item := range graphArray {
				if itemMap, ok := item.(map[string]interface{}); ok {
					products = append(products, extractProductsFromJSONLDObject(itemMap, sourceURL)...)
				}
			}
			return products
		}
	}

	// Check if this object is a Product
	typeVal, _ := obj["@type"].(string)
	if !strings.EqualFold(typeVal, "Product") {
		return nil
	}

	name, _ := obj["name"].(string)
	if name == "" {
		return nil
	}

	price := extractPriceFromOffers(obj)
	imageURL := extractImageFromJSONLD(obj)

	if price > 0 {
		products = append(products, RawProduct{
			Name:      name,
			Price:     price,
			ImageURL:  imageURL,
			SourceURL: sourceURL,
		})
	}

	return products
}

func extractPriceFromOffers(obj map[string]interface{}) float64 {
	offers, ok := obj["offers"]
	if !ok {
		return 0
	}

	// Offers can be a single object or an array
	switch v := offers.(type) {
	case map[string]interface{}:
		return parsePriceValue(v["price"])
	case []interface{}:
		if len(v) > 0 {
			if firstOffer, ok := v[0].(map[string]interface{}); ok {
				return parsePriceValue(firstOffer["price"])
			}
		}
	}

	return 0
}

func parsePriceValue(v interface{}) float64 {
	switch p := v.(type) {
	case float64:
		return p
	case string:
		// Clean common price formatting
		cleaned := strings.ReplaceAll(p, ",", ".")
		cleaned = strings.ReplaceAll(cleaned, " ", "")
		price, err := strconv.ParseFloat(cleaned, 64)
		if err != nil {
			return 0
		}
		return price
	}
	return 0
}

func extractImageFromJSONLD(obj map[string]interface{}) string {
	img, ok := obj["image"]
	if !ok {
		return ""
	}

	switch v := img.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}

	return ""
}

// --- Open Graph extraction ---

func extractOpenGraph(html string, sourceURL string) []RawProduct {
	metas := extractMetaTags(html)

	ogType := metas["og:type"]
	if !strings.EqualFold(ogType, "product") {
		return nil
	}

	name := metas["og:title"]
	if name == "" {
		return nil
	}

	priceStr := metas["product:price:amount"]
	if priceStr == "" {
		return nil
	}

	price := parsePriceValue(priceStr)
	if price <= 0 {
		return nil
	}

	imageURL := metas["og:image"]

	return []RawProduct{{
		Name:      name,
		Price:     price,
		ImageURL:  imageURL,
		SourceURL: sourceURL,
	}}
}

// extractMetaTags extracts <meta property="..." content="..."> tags.
func extractMetaTags(html string) map[string]string {
	re := regexp.MustCompile(`<meta\s+[^>]*property\s*=\s*["']([^"']+)["'][^>]*content\s*=\s*["']([^"']+)["'][^>]*/?>`)
	matches := re.FindAllStringSubmatch(html, -1)

	result := make(map[string]string)
	for _, match := range matches {
		result[match[1]] = match[2]
	}

	// Also try the reverse order: content before property
	re2 := regexp.MustCompile(`<meta\s+[^>]*content\s*=\s*["']([^"']+)["'][^>]*property\s*=\s*["']([^"']+)["'][^>]*/?>`)
	matches2 := re2.FindAllStringSubmatch(html, -1)
	for _, match := range matches2 {
		if _, exists := result[match[2]]; !exists {
			result[match[2]] = match[1]
		}
	}

	return result
}
