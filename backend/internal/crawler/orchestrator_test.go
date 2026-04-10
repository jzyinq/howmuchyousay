package crawler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_BasicFlow(t *testing.T) {
	// Simulate: AI calls extract_product, then save_product, then done
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		var resp map[string]interface{}
		switch n {
		case 1:
			// AI decides to extract a product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_product",
						"arguments": `{"url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 2:
			// AI saves the product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_2",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "save_product",
						"arguments": `{"name":"Test Product","price":99.99,"image_url":"https://img.com/test.jpg","source_url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 3:
			// AI calls done
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_3",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "done",
						"arguments": `{}`,
					},
				},
			})
		default:
			resp = openAIStopResponse("Done")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scraper := &mockFirecrawlScraper{
		extractProductFn: func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
			return &crawler.ProductExtractionResult{
				Name:     "Test Product",
				Price:    99.99,
				ImageURL: "https://img.com/test.jpg",
				Found:    true,
			}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links: []string{
			"https://shop.com/product/1",
			"https://shop.com/product/2",
		},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)

	assert.Len(t, result.Products, 1)
	assert.Equal(t, "Test Product", result.Products[0].Name)
	assert.True(t, result.TotalTokensUsed > 0)
}

func TestOrchestrator_SafetyCapBreaksLoop(t *testing.T) {
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		// AI always tries to discover more links — should be stopped by safety cap
		resp := openAIToolCallResponse([]map[string]interface{}{
			{
				"id":   fmt.Sprintf("call_%d", n),
				"type": "function",
				"function": map[string]interface{}{
					"name":      "discover_links",
					"arguments": fmt.Sprintf(`{"url":"https://shop.com/page/%d"}`, n),
				},
			},
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			return &crawler.LinkDiscoveryResult{
				PageTitle: "Page",
				Links:     []string{"https://shop.com/page/next"},
			}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links:     []string{"https://shop.com/page/1"},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 20,
		MaxScrapes:  3, // Very low cap to test safety
	}

	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)
	assert.True(t, result.SafetyCapHit)
}

func TestOrchestrator_SkipVisitedURLs(t *testing.T) {
	callNum := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callNum++
		n := callNum
		mu.Unlock()

		var resp map[string]interface{}
		switch n {
		case 1:
			// AI tries to discover links on a URL that's already the initial URL
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "discover_links",
						"arguments": `{"url":"https://shop.com"}`,
					},
				},
			})
		default:
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_done",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "done",
						"arguments": `{}`,
					},
				},
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scrapeCalled := false
	scraper := &mockFirecrawlScraper{
		discoverLinksFn: func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
			scrapeCalled = true
			return &crawler.LinkDiscoveryResult{}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links:     []string{},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 1,
		MaxScrapes:  50,
	}

	_, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)

	// Scraper should NOT have been called since the URL was already visited (initial URL)
	assert.False(t, scrapeCalled, "Firecrawl should not be called for already-visited URLs")
}
