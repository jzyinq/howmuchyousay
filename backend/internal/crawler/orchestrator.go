package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// CrawlConfig fields used by Orchestrator: URL, Timeout, MinProducts, MaxScrapes.
// CrawlConfig is defined in crawler.go (will be updated in Task 5).

// OrchestratorResult contains the outcome of an orchestrator crawl loop.
type OrchestratorResult struct {
	Products        []RawProduct
	ScrapeCount     int
	TotalTokensUsed int
	AIRequestCount  int
	SafetyCapHit    bool
	DoneByAI        bool
}

// OrchestratorLogger is an optional callback for logging orchestrator events.
type OrchestratorLogger func(event string, message string)

// Orchestrator manages the AI-driven crawl loop using OpenAI function calling.
type Orchestrator struct {
	client  *openai.Client
	model   string
	scraper FirecrawlScraper
}

// NewOrchestrator creates an Orchestrator with the given OpenAI credentials and Firecrawl scraper.
func NewOrchestrator(apiKey, model, baseURL string, scraper FirecrawlScraper) *Orchestrator {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)
	return &Orchestrator{
		client:  &client,
		model:   model,
		scraper: scraper,
	}
}

// Run executes the AI-driven crawl loop.
func (o *Orchestrator) Run(ctx context.Context, initialLinks *LinkDiscoveryResult, cfg CrawlConfig, logger OrchestratorLogger) (*OrchestratorResult, error) {
	log := func(event, msg string) {
		if logger != nil {
			logger(event, msg)
		}
	}

	handlers := NewToolHandlers(cfg.MinProducts)

	// Mark the initial URL as visited
	handlers.MarkVisited(cfg.URL)
	if initialLinks.PageURL != "" && initialLinks.PageURL != cfg.URL {
		handlers.MarkVisited(initialLinks.PageURL)
	}

	// Build initial messages
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(buildSystemPrompt(cfg.MinProducts, cfg.URL)),
		openai.UserMessage(buildInitialMessage(initialLinks)),
	}

	tools := buildToolDefinitions()

	var totalTokens int
	var aiRequestCount int
	var done bool

	log("ORCHESTRATOR_START", fmt.Sprintf("Starting orchestrator loop for %s, target: %d products", cfg.URL, cfg.MinProducts))

	for !done {
		if ctx.Err() != nil {
			log("TOOL_ERROR", fmt.Sprintf("Context cancelled: %v", ctx.Err()))
			break
		}

		if handlers.ScrapeCount() >= cfg.MaxScrapes {
			log("SAFETY_CAP", fmt.Sprintf("Safety cap reached (%d scrape calls). Ending loop with %d products",
				cfg.MaxScrapes, len(handlers.SavedProducts())))
			return &OrchestratorResult{
				Products:        handlers.SavedProducts(),
				ScrapeCount:     handlers.ScrapeCount(),
				TotalTokensUsed: totalTokens,
				AIRequestCount:  aiRequestCount,
				SafetyCapHit:    true,
			}, nil
		}

		// Call OpenAI
		completion, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Messages: messages,
			Tools:    tools,
			Model:    o.model,
		})
		if err != nil {
			return nil, fmt.Errorf("OpenAI API error: %w", err)
		}

		aiRequestCount++
		totalTokens += int(completion.Usage.TotalTokens)
		log("AI_TOKENS", fmt.Sprintf("OpenAI call used %d tokens (total: %d)", completion.Usage.TotalTokens, totalTokens))

		if len(completion.Choices) == 0 {
			break
		}

		choice := completion.Choices[0]

		// If the model stopped without tool calls, we're done
		if choice.FinishReason == "stop" {
			break
		}

		// Process tool calls
		if len(choice.Message.ToolCalls) == 0 {
			break
		}

		// Append the assistant message with tool calls
		messages = append(messages, choice.Message.ToParam())

		for _, toolCall := range choice.Message.ToolCalls {
			result := o.handleToolCall(ctx, toolCall, handlers, log)
			messages = append(messages, openai.ToolMessage(result, toolCall.ID))

			if toolCall.Function.Name == "done" {
				done = true
			}
		}
	}

	log("ORCHESTRATOR_DONE", fmt.Sprintf("Orchestrator finished. %d products saved in %d scrape calls",
		len(handlers.SavedProducts()), handlers.ScrapeCount()))

	return &OrchestratorResult{
		Products:        handlers.SavedProducts(),
		ScrapeCount:     handlers.ScrapeCount(),
		TotalTokensUsed: totalTokens,
		AIRequestCount:  aiRequestCount,
		DoneByAI:        done,
	}, nil
}

// handleToolCall dispatches a tool call and returns the result string.
func (o *Orchestrator) handleToolCall(ctx context.Context, tc openai.ChatCompletionMessageToolCallUnion, handlers *ToolHandlers, log OrchestratorLogger) string {
	name := tc.Function.Name
	args := tc.Function.Arguments

	log("TOOL_CALL", fmt.Sprintf("Tool call: %s(%s)", name, args))

	var result string

	switch name {
	case "discover_links":
		result = o.handleDiscoverLinks(ctx, args, handlers, log)
	case "extract_and_save_product":
		result = o.handleExtractAndSaveProduct(ctx, args, handlers, log)
	case "get_status":
		result = handlers.GetStatus()
	case "done":
		result = "Crawl loop ended."
	default:
		result = fmt.Sprintf("Unknown tool: %s", name)
	}

	log("TOOL_RESULT", fmt.Sprintf("%s result: %s", name, truncateForLog(result, 200)))
	return result
}

// handleDiscoverLinks processes a discover_links tool call.
func (o *Orchestrator) handleDiscoverLinks(ctx context.Context, argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

	if handlers.IsVisited(args.URL) {
		log("DUPLICATE_SKIP", fmt.Sprintf("Skipping already-visited URL: %s", args.URL))
		return fmt.Sprintf("Already visited %s. Try a different page.", args.URL)
	}

	handlers.MarkVisited(args.URL)
	handlers.IncrementScrapeCount()

	result, err := o.scraper.DiscoverLinks(ctx, args.URL)
	if err != nil {
		log("TOOL_ERROR", fmt.Sprintf("discover_links failed for %s: %v", args.URL, err))
		return fmt.Sprintf("Failed to fetch %s: %v", args.URL, err)
	}

	log("TOOL_RESULT", fmt.Sprintf("discover_links returned %d links for %s", len(result.Links), args.URL))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Page: \"%s\"\n", result.PageTitle))
	sb.WriteString(fmt.Sprintf("Links (%d found):\n", len(result.Links)))
	for _, link := range result.Links {
		sb.WriteString(fmt.Sprintf("- %s\n", link))
	}
	return sb.String()
}

// handleExtractAndSaveProduct extracts a product from a URL and saves it in one step.
func (o *Orchestrator) handleExtractAndSaveProduct(ctx context.Context, argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

	if handlers.IsVisited(args.URL) {
		log("DUPLICATE_SKIP", fmt.Sprintf("Skipping already-visited URL: %s", args.URL))
		return fmt.Sprintf("Already visited %s. Try a different page.", args.URL)
	}

	handlers.MarkVisited(args.URL)
	handlers.IncrementScrapeCount()

	result, err := o.scraper.ExtractProduct(ctx, args.URL)
	if err != nil {
		log("TOOL_ERROR", fmt.Sprintf("extract_and_save_product failed for %s: %v", args.URL, err))
		return fmt.Sprintf("Failed to extract product from %s: %v", args.URL, err)
	}

	if !result.Found {
		return fmt.Sprintf("No product data found at %s. This may not be a product page — try discover_links instead.", args.URL)
	}

	product := RawProduct{
		Name:      result.Name,
		Price:     result.Price,
		ImageURL:  result.ImageURL,
		SourceURL: args.URL,
	}

	saveResult := handlers.SaveProduct(product)

	if strings.Contains(saveResult, "Saved") {
		log("PRODUCT_SAVED", fmt.Sprintf("Saved: %s ($%.2f). Progress: %d/%d",
			result.Name, result.Price, len(handlers.SavedProducts()), handlers.MinProducts()))
	} else {
		log("PRODUCT_REJECTED", saveResult)
	}

	return saveResult
}

// buildSystemPrompt creates the system prompt for the orchestrator.
func buildSystemPrompt(minProducts int, shopURL string) string {
	return fmt.Sprintf(`You are a product discovery agent for an online price guessing game. Your job is to find and save products from an online shop.

Goal: Save at least %d products from %s.

You have these tools:
- discover_links(url): Explore a page to find links. Use on listing pages, category pages, and pagination pages.
- extract_and_save_product(url): Extract product data from a product page and save it automatically. Returns the saved product info and your progress, or an error if extraction fails.
- get_status(): Check how many products you've saved and see the list.
- done(): Call when you've saved enough products or exhausted available pages.

Strategy:
1. Start by reviewing the initial page links provided below.
2. Identify which links are product pages and which are category/listing/pagination pages.
3. Use discover_links on category and pagination pages to find more products.
4. Use extract_and_save_product on product pages. You can call it on multiple URLs simultaneously — batch 3-5 product URLs per turn for efficiency.
5. Call done() when you reach %d products or run out of pages to explore.

Avoid:
- Re-scraping URLs you've already visited (the system tracks this automatically).
- Following links to non-product areas (blog, about, contact, FAQ, etc.).`, minProducts, shopURL, minProducts)
}

// buildInitialMessage creates the first user message with initial links.
func buildInitialMessage(links *LinkDiscoveryResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Shop URL: %s\n", links.PageURL))
	sb.WriteString(fmt.Sprintf("Initial page title: \"%s\"\n", links.PageTitle))
	sb.WriteString(fmt.Sprintf("Links found on initial page (%d):\n", len(links.Links)))
	for _, link := range links.Links {
		sb.WriteString(fmt.Sprintf("- %s\n", link))
	}
	sb.WriteString("\nBegin exploring and saving products.")
	return sb.String()
}

// buildToolDefinitions returns the OpenAI tool definitions for the orchestrator.
func buildToolDefinitions() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "discover_links",
			Description: openai.String("Explore a page to find links. Use on listing pages, category pages, and pagination pages."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL of the page to explore",
					},
				},
				"required": []string{"url"},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "extract_and_save_product",
			Description: openai.String("Extract product data from a product page and save it automatically. Returns saved product info and progress, or an error if extraction fails."),
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "URL of a product page",
					},
				},
				"required": []string{"url"},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "get_status",
			Description: openai.String("Check how many products have been saved and see the list."),
			Parameters: openai.FunctionParameters{
				"type":       "object",
				"properties": map[string]any{},
			},
		}),
		openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        "done",
			Description: openai.String("Signal that you are finished collecting products."),
			Parameters: openai.FunctionParameters{
				"type":       "object",
				"properties": map[string]any{},
			},
		}),
	}
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
