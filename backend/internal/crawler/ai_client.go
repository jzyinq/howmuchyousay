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
	// ExtractProducts sends page markdown to AI and gets structured product data back.
	// Returns products, tokens used, and error.
	ExtractProducts(ctx context.Context, markdown string, pageURL string) ([]RawProduct, int, error)
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

// maxMarkdownChars is the maximum markdown content length sent to the AI.
// Firecrawl markdown is typically much smaller than raw HTML, but we still
// cap it to avoid exceeding token limits or incurring unnecessary cost.
const maxMarkdownChars = 50000

func (c *OpenAIClient) ExtractProducts(ctx context.Context, markdown string, pageURL string) ([]RawProduct, int, error) {
	if len([]rune(markdown)) > maxMarkdownChars {
		markdown = string([]rune(markdown)[:maxMarkdownChars])
	}

	prompt := fmt.Sprintf(`You are a product data extractor. Analyze the following markdown content from %s and extract product information.

Return a JSON array of products. Each product should have:
- "name": product name (string)
- "price": product price as a number (float, in the page's currency)
- "image_url": URL of the product image (string, empty if not found)

Return ONLY the JSON array, no other text. If no products are found, return an empty array [].

Markdown:
%s`, pageURL, markdown)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model: c.model,
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
