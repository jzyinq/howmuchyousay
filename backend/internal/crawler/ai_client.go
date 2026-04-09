package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// AIClient is the interface for AI-powered product extraction.
type AIClient interface {
	// ExtractProducts sends HTML to AI and gets structured product data back.
	// Returns products, tokens used, and error.
	ExtractProducts(ctx context.Context, html string, pageURL string) ([]RawProduct, int, error)
	// ExtractLinks sends HTML to AI and gets promising product/category links.
	// Returns link paths, tokens used, and error.
	ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error)
}

// OpenAIClient implements AIClient using the official OpenAI Go SDK.
type OpenAIClient struct {
	client *openai.Client
	model  string
}

// NewOpenAIClient creates an OpenAI client using the official SDK.
// model is the model name (e.g. "gpt-5-mini").
// baseURL is optional - pass "" for production, or a test server URL for testing.
func NewOpenAIClient(apiKey string, model string, baseURL string) *OpenAIClient {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(opts...)
	return &OpenAIClient{
		client: &client,
		model:  model,
	}
}

func (c *OpenAIClient) ExtractProducts(ctx context.Context, html string, pageURL string) ([]RawProduct, int, error) {
	truncatedHTML := truncateHTML(html, 100000)

	prompt := fmt.Sprintf(`You are a product data extractor. Analyze the following HTML from %s and extract product information.

Return a JSON array of products. Each product should have:
- "name": product name (string)
- "price": product price as a number (float, in the page's currency)
- "image_url": URL of the product image (string, empty if not found)

Return ONLY the JSON array, no other text. If no products are found, return an empty array [].

HTML:
%s`, pageURL, truncatedHTML)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       c.model,
		Temperature: openai.Float(0.1),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("OpenAI API error: %w", err)
	}

	tokensUsed := int(completion.Usage.TotalTokens)

	if len(completion.Choices) == 0 {
		return nil, tokensUsed, fmt.Errorf("no choices in API response")
	}

	content := completion.Choices[0].Message.Content

	var products []RawProduct
	cleaned := cleanJSONResponse(content)
	if err := json.Unmarshal([]byte(cleaned), &products); err != nil {
		// AI returned invalid JSON - not an error, just no products
		return nil, tokensUsed, nil
	}

	// Set source URL for all products
	for i := range products {
		products[i].SourceURL = pageURL
	}

	return products, tokensUsed, nil
}

func (c *OpenAIClient) ExtractLinks(ctx context.Context, html string, baseURL string) ([]string, int, error) {
	truncatedHTML := truncateHTML(html, 100000)

	prompt := fmt.Sprintf(`You are a web crawler assistant. Analyze the following HTML from %s and identify links that likely lead to:
1. Product pages (individual products with prices)
2. Product category/listing pages (containing multiple products)

Return a JSON array of URL paths (relative or absolute). Focus on links that are most likely to contain product data.
Return ONLY the JSON array, no other text. Return at most 20 links. If no relevant links found, return [].

HTML:
%s`, baseURL, truncatedHTML)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       c.model,
		Temperature: openai.Float(0.1),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("OpenAI API error: %w", err)
	}

	tokensUsed := int(completion.Usage.TotalTokens)

	if len(completion.Choices) == 0 {
		return nil, tokensUsed, fmt.Errorf("no choices in API response")
	}

	content := completion.Choices[0].Message.Content

	var links []string
	cleaned := cleanJSONResponse(content)
	if err := json.Unmarshal([]byte(cleaned), &links); err != nil {
		return nil, tokensUsed, nil
	}

	return links, tokensUsed, nil
}

// truncateHTML truncates HTML content to maxChars characters.
func truncateHTML(html string, maxChars int) string {
	if len(html) <= maxChars {
		return html
	}
	return html[:maxChars]
}

// cleanJSONResponse strips markdown code fences from AI response if present.
func cleanJSONResponse(content string) string {
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	return content
}
