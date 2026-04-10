package crawler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIClient_ExtractProducts(t *testing.T) {
	// Mock OpenAI API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		// Read the request body and verify prompt mentions "markdown"
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "markdown")
		assert.NotContains(t, bodyStr, "HTML")

		// Return a mock response with extracted products
		resp := map[string]interface{}{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  "gpt-5-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": `[{"name":"Laptop Dell XPS 15","price":5999.99,"image_url":"https://img.com/dell.jpg"},{"name":"iPhone 15 Pro","price":5499.00,"image_url":"https://img.com/iphone.jpg"}]`,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 400,
				"total_tokens":      500,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	markdownInput := strings.TrimSpace(`
# Products

## Laptop Dell XPS 15
**Price:** $5,999.99
![Laptop](https://img.com/dell.jpg)

## iPhone 15 Pro
**Price:** $5,499.00
![iPhone](https://img.com/iphone.jpg)
`)

	products, tokensUsed, err := client.ExtractProducts(ctx, markdownInput, "https://shop.com/products")
	require.NoError(t, err)
	assert.Len(t, products, 2)
	assert.Equal(t, "Laptop Dell XPS 15", products[0].Name)
	assert.Equal(t, 5999.99, products[0].Price)
	assert.Equal(t, "https://shop.com/products", products[0].SourceURL)
	assert.Equal(t, "iPhone 15 Pro", products[1].Name)
	assert.Equal(t, 500, tokensUsed)
}

func TestOpenAIClient_HandlesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	_, _, err := client.ExtractProducts(ctx, "# Empty page", "https://shop.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestOpenAIClient_HandlesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":     "chatcmpl-test",
			"object": "chat.completion",
			"model":  "gpt-5-mini",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "This is not valid JSON at all",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     50,
				"completion_tokens": 50,
				"total_tokens":      100,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := crawler.NewOpenAIClient("test-api-key", "gpt-5-mini", server.URL)
	ctx := context.Background()

	products, _, err := client.ExtractProducts(ctx, "# Some markdown content", "https://shop.com")
	require.NoError(t, err)
	// Invalid JSON from AI -> empty result, not an error
	assert.Empty(t, products)
}
