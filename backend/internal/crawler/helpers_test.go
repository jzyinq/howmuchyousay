package crawler_test

import (
	"context"

	"github.com/jzy/howmuchyousay/internal/crawler"
)

// mockFirecrawlScraper implements FirecrawlScraper for testing.
type mockFirecrawlScraper struct {
	discoverLinksFn  func(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error)
	extractProductFn func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error)
}

func (m *mockFirecrawlScraper) DiscoverLinks(ctx context.Context, url string) (*crawler.LinkDiscoveryResult, error) {
	if m.discoverLinksFn != nil {
		return m.discoverLinksFn(ctx, url)
	}
	return &crawler.LinkDiscoveryResult{}, nil
}

func (m *mockFirecrawlScraper) ExtractProduct(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
	if m.extractProductFn != nil {
		return m.extractProductFn(ctx, url)
	}
	return &crawler.ProductExtractionResult{Found: false}, nil
}

// openAIToolCallResponse creates a mock OpenAI response with tool calls.
func openAIToolCallResponse(toolCalls []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": toolCalls,
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}
}

// openAIStopResponse creates a mock OpenAI response that ends the conversation.
func openAIStopResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":     "chatcmpl-test",
		"object": "chat.completion",
		"model":  "gpt-5-mini",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     50,
			"completion_tokens": 20,
			"total_tokens":      70,
		},
	}
}
