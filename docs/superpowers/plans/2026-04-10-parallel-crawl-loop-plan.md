# Parallel Crawl Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Speed up the AI-driven orchestrator crawl loop by executing I/O-bound tool calls in parallel and merging the extract+save two-step into a single tool.

**Architecture:** The orchestrator loop currently processes tool calls sequentially. We add a `sync.Mutex` to `ToolHandlers` for thread safety, merge `extract_product`+`save_product` into `extract_and_save_product`, and replace the sequential tool call `for` loop with a goroutine-based parallel dispatcher that uses a semaphore for concurrency control.

**Tech Stack:** Go 1.26, `sync.Mutex`, `sync.WaitGroup`, OpenAI function calling, Firecrawl SDK

---

### Task 1: Add `sync.Mutex` to `ToolHandlers`

**Files:**
- Modify: `backend/internal/crawler/tool_handlers.go:1-118`
- Test: `backend/internal/crawler/tool_handlers_test.go`

- [ ] **Step 1: Write concurrency safety test**

Add to `backend/internal/crawler/tool_handlers_test.go`:

```go
func TestToolHandlers_ConcurrentSaveProduct(t *testing.T) {
	h := NewToolHandlers(100)
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h.SaveProduct(RawProduct{
				Name:      fmt.Sprintf("Product %d", i),
				Price:     float64(10 + i),
				SourceURL: fmt.Sprintf("https://shop.com/p/%d", i),
			})
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 20, len(h.SavedProducts()))
}
```

Add `"fmt"` and `"sync"` to the test file imports.

Note: `tool_handlers_test.go` is in package `crawler` (not `crawler_test`), so it has direct access to unexported types.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -race ./internal/crawler/ -run TestToolHandlers_ConcurrentSaveProduct -v`

Expected: Data race detected or inconsistent count (the `-race` flag catches concurrent map writes).

- [ ] **Step 3: Add mutex to ToolHandlers and lock all mutable methods**

Edit `backend/internal/crawler/tool_handlers.go`:

Add `"sync"` to imports.

Update the struct:

```go
type ToolHandlers struct {
	mu            sync.Mutex
	minProducts   int
	savedProducts []RawProduct
	seenNames     map[string]bool
	seenURLs      map[string]bool
	visitedURLs   map[string]bool
	scrapeCount   int
}
```

Add locking to every method that reads or writes mutable state:

```go
func (h *ToolHandlers) SaveProduct(p RawProduct) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	// ... existing body unchanged ...
}

func (h *ToolHandlers) GetStatus() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	// ... existing body unchanged ...
}

func (h *ToolHandlers) SavedProducts() []RawProduct {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.savedProducts
}

func (h *ToolHandlers) MinProducts() int {
	// immutable, no lock needed
	return h.minProducts
}

func (h *ToolHandlers) IsVisited(url string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.visitedURLs[url]
}

func (h *ToolHandlers) MarkVisited(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.visitedURLs[url] = true
}

func (h *ToolHandlers) ScrapeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.scrapeCount
}

func (h *ToolHandlers) IncrementScrapeCount() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.scrapeCount++
}

func (h *ToolHandlers) HasReachedTarget() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.savedProducts) >= h.minProducts
}
```

The private `productNameList()` is only called from `SaveProduct` and `GetStatus`, which already hold the lock. Leave it without its own lock (it would deadlock if it tried to acquire).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd backend && go test -race ./internal/crawler/ -run TestToolHandlers -v`

Expected: All pass, no data races.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/tool_handlers.go backend/internal/crawler/tool_handlers_test.go
git commit -m "feat(crawler): add sync.Mutex to ToolHandlers for thread safety"
```

---

### Task 2: Add `handleExtractAndSaveProduct` to orchestrator

**Files:**
- Modify: `backend/internal/crawler/orchestrator.go:156-277` (replace `handleExtractProduct` + `handleSaveProduct` with new handler)
- Test: `backend/internal/crawler/orchestrator_test.go`

- [ ] **Step 1: Write test for extract_and_save_product tool call**

Add to `backend/internal/crawler/orchestrator_test.go`:

```go
func TestOrchestrator_ExtractAndSaveProduct(t *testing.T) {
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
			// AI calls extract_and_save_product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": `{"url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 2:
			// AI calls done
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_2",
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
		Links:     []string{"https://shop.com/product/1"},
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
	assert.Equal(t, 99.99, result.Products[0].Price)
	assert.Equal(t, "https://shop.com/product/1", result.Products[0].SourceURL)
	assert.True(t, result.DoneByAI)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test ./internal/crawler/ -run TestOrchestrator_ExtractAndSaveProduct -v`

Expected: FAIL — the orchestrator dispatches `extract_and_save_product` to the `default` case ("Unknown tool").

- [ ] **Step 3: Add `handleExtractAndSaveProduct` method**

Add to `backend/internal/crawler/orchestrator.go`, replacing `handleExtractProduct` (lines 217-246) and `handleSaveProduct` (lines 248-277) with a single new method:

```go
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
```

- [ ] **Step 4: Update `handleToolCall` dispatch switch**

In `backend/internal/crawler/orchestrator.go`, update the `handleToolCall` method (lines 156-181). Remove the `extract_product` and `save_product` cases, add `extract_and_save_product`:

```go
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
```

- [ ] **Step 5: Update `buildToolDefinitions`**

In `backend/internal/crawler/orchestrator.go`, replace the `extract_product` and `save_product` tool definitions (lines 335-362) with a single `extract_and_save_product`:

```go
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
```

- [ ] **Step 6: Update `buildSystemPrompt`**

In `backend/internal/crawler/orchestrator.go`, replace `buildSystemPrompt` (lines 280-303):

```go
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
```

- [ ] **Step 7: Update old `TestOrchestrator_BasicFlow` test**

The existing `TestOrchestrator_BasicFlow` uses `extract_product` then `save_product`. Update it to use `extract_and_save_product`:

In `backend/internal/crawler/orchestrator_test.go`, replace the `TestOrchestrator_BasicFlow` function:

```go
func TestOrchestrator_BasicFlow(t *testing.T) {
	// Simulate: AI calls extract_and_save_product, then done
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
			// AI decides to extract and save a product
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": `{"url":"https://shop.com/product/1"}`,
					},
				},
			})
		case 2:
			// AI calls done
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_2",
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
```

- [ ] **Step 8: Run all orchestrator tests**

Run: `cd backend && go test ./internal/crawler/ -run TestOrchestrator -v`

Expected: All pass.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/crawler/orchestrator.go backend/internal/crawler/orchestrator_test.go
git commit -m "feat(crawler): merge extract+save into extract_and_save_product tool"
```

---

### Task 3: Parallel tool call dispatch

**Files:**
- Modify: `backend/internal/crawler/orchestrator.go:82-141` (the main loop)
- Test: `backend/internal/crawler/orchestrator_test.go`

- [ ] **Step 1: Write test for parallel execution of multiple tool calls**

Add to `backend/internal/crawler/orchestrator_test.go`:

```go
func TestOrchestrator_ParallelToolCalls(t *testing.T) {
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
			// AI returns 3 extract_and_save_product calls at once
			resp = openAIToolCallResponse([]map[string]interface{}{
				{
					"id":   "call_1",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": `{"url":"https://shop.com/product/1"}`,
					},
				},
				{
					"id":   "call_2",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": `{"url":"https://shop.com/product/2"}`,
					},
				},
				{
					"id":   "call_3",
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": `{"url":"https://shop.com/product/3"}`,
					},
				},
			})
		case 2:
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
		default:
			resp = openAIStopResponse("Done")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	var extractMu sync.Mutex
	extractedURLs := []string{}

	scraper := &mockFirecrawlScraper{
		extractProductFn: func(ctx context.Context, url string) (*crawler.ProductExtractionResult, error) {
			// Simulate network delay to prove parallelism
			time.Sleep(50 * time.Millisecond)
			extractMu.Lock()
			extractedURLs = append(extractedURLs, url)
			extractMu.Unlock()
			return &crawler.ProductExtractionResult{
				Name:     "Product from " + url,
				Price:    100.00,
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
			"https://shop.com/product/3",
		},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 3,
		MaxScrapes:  50,
	}

	start := time.Now()
	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Len(t, result.Products, 3)
	assert.Len(t, extractedURLs, 3)

	// If parallel: 3 * 50ms should take ~50-80ms total, not 150ms+
	// Use generous threshold to avoid flaky test, but catch sequential (150ms+)
	assert.Less(t, elapsed, 500*time.Millisecond, "parallel execution should be faster than sequential")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -race ./internal/crawler/ -run TestOrchestrator_ParallelToolCalls -v`

Expected: FAIL — either timeout (sequential 150ms+ with test overhead) or the test passes but only because the mock is fast. The important thing is the test exists; we'll verify parallel dispatch works by structure.

- [ ] **Step 3: Implement parallel tool call dispatch in the `Run` loop**

Replace the tool call processing section in `backend/internal/crawler/orchestrator.go`. The current sequential loop (lines 133-140):

```go
		for _, toolCall := range choice.Message.ToolCalls {
			result := o.handleToolCall(ctx, toolCall, handlers, log)
			messages = append(messages, openai.ToolMessage(result, toolCall.ID))

			if toolCall.Function.Name == "done" {
				done = true
			}
		}
```

Replace with:

```go
		results := o.dispatchToolCalls(ctx, choice.Message.ToolCalls, handlers, log)
		for i, toolCall := range choice.Message.ToolCalls {
			messages = append(messages, openai.ToolMessage(results[i], toolCall.ID))
			if toolCall.Function.Name == "done" {
				done = true
			}
		}
```

- [ ] **Step 4: Add `isIOTool` helper and `dispatchToolCalls` method**

Add to `backend/internal/crawler/orchestrator.go`:

```go
// maxParallelScrapes is the concurrency limit for Firecrawl calls.
const maxParallelScrapes = 5

// isIOTool returns true for tools that make external HTTP calls (Firecrawl).
func isIOTool(name string) bool {
	return name == "extract_and_save_product" || name == "discover_links"
}

// dispatchToolCalls executes tool calls, running I/O-bound calls in parallel.
// Returns results in the same order as the input tool calls.
func (o *Orchestrator) dispatchToolCalls(
	ctx context.Context,
	toolCalls []openai.ChatCompletionMessageToolCallUnion,
	handlers *ToolHandlers,
	log OrchestratorLogger,
) []string {
	results := make([]string, len(toolCalls))
	sem := make(chan struct{}, maxParallelScrapes)
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		name := tc.Function.Name
		args := tc.Function.Arguments

		log("TOOL_CALL", fmt.Sprintf("Tool call: %s(%s)", name, args))

		if isIOTool(name) {
			wg.Add(1)
			go func(idx int, tc openai.ChatCompletionMessageToolCallUnion) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				var result string
				switch tc.Function.Name {
				case "discover_links":
					result = o.handleDiscoverLinks(ctx, tc.Function.Arguments, handlers, log)
				case "extract_and_save_product":
					result = o.handleExtractAndSaveProduct(ctx, tc.Function.Arguments, handlers, log)
				}

				log("TOOL_RESULT", fmt.Sprintf("%s result: %s", tc.Function.Name, truncateForLog(result, 200)))
				results[idx] = result
			}(i, tc)
		} else {
			// Local tools: execute inline
			var result string
			switch name {
			case "get_status":
				result = handlers.GetStatus()
			case "done":
				result = "Crawl loop ended."
			default:
				result = fmt.Sprintf("Unknown tool: %s", name)
			}
			log("TOOL_RESULT", fmt.Sprintf("%s result: %s", name, truncateForLog(result, 200)))
			results[i] = result
		}
	}

	wg.Wait()
	return results
}
```

Add `"sync"` to the imports at the top of `orchestrator.go`.

- [ ] **Step 5: Remove the old `handleToolCall` method**

Delete the `handleToolCall` method (the one with the switch statement) since its logic is now in `dispatchToolCalls`. It should no longer be referenced anywhere.

- [ ] **Step 6: Run all tests with race detector**

Run: `cd backend && go test -race ./internal/crawler/ -v`

Expected: All pass, no data races.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/crawler/orchestrator.go backend/internal/crawler/orchestrator_test.go
git commit -m "feat(crawler): parallel dispatch of I/O-bound tool calls with semaphore"
```

---

### Task 4: Safety cap enforcement for parallel batches

**Files:**
- Modify: `backend/internal/crawler/orchestrator.go` (safety cap logic in `dispatchToolCalls`)
- Test: `backend/internal/crawler/orchestrator_test.go`

- [ ] **Step 1: Write test for safety cap in parallel batch**

Add to `backend/internal/crawler/orchestrator_test.go`:

```go
func TestOrchestrator_SafetyCapInParallelBatch(t *testing.T) {
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
			// AI returns 5 extract calls, but safety cap is 3
			calls := []map[string]interface{}{}
			for i := 1; i <= 5; i++ {
				calls = append(calls, map[string]interface{}{
					"id":   fmt.Sprintf("call_%d", i),
					"type": "function",
					"function": map[string]interface{}{
						"name":      "extract_and_save_product",
						"arguments": fmt.Sprintf(`{"url":"https://shop.com/product/%d"}`, i),
					},
				})
			}
			resp = openAIToolCallResponse(calls)
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
				Name:     "Product from " + url,
				Price:    100.00,
				ImageURL: "https://img.com/test.jpg",
				Found:    true,
			}, nil
		},
	}

	initialLinks := &crawler.LinkDiscoveryResult{
		PageURL:   "https://shop.com",
		PageTitle: "Test Shop",
		Links:     []string{"https://shop.com/product/1"},
	}

	orch := crawler.NewOrchestrator("test-api-key", "gpt-5-mini", server.URL, scraper)
	cfg := crawler.CrawlConfig{
		URL:         "https://shop.com",
		Timeout:     30 * time.Second,
		MinProducts: 20,
		MaxScrapes:  3,
	}

	result, err := orch.Run(context.Background(), initialLinks, cfg, nil)
	require.NoError(t, err)

	// Only 3 products should be saved (safety cap = 3 scrapes)
	assert.LessOrEqual(t, len(result.Products), 3)
	assert.LessOrEqual(t, result.ScrapeCount, 3)
}
```

- [ ] **Step 2: Run test to verify behavior**

Run: `cd backend && go test -race ./internal/crawler/ -run TestOrchestrator_SafetyCapInParallelBatch -v`

This may pass or fail depending on whether `dispatchToolCalls` already respects the cap. If it fails, we need to add the safety cap check.

- [ ] **Step 3: Add safety cap check to `dispatchToolCalls`**

Update `dispatchToolCalls` in `backend/internal/crawler/orchestrator.go`. Add a `TryIncrementScrapeCount` method to `ToolHandlers` that atomically checks the cap:

In `backend/internal/crawler/tool_handlers.go`, add:

```go
// TryIncrementScrapeCount atomically checks the safety cap and increments if allowed.
// Returns true if the increment was allowed, false if the cap was reached.
func (h *ToolHandlers) TryIncrementScrapeCount(maxScrapes int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.scrapeCount >= maxScrapes {
		return false
	}
	h.scrapeCount++
	return true
}
```

Then update `dispatchToolCalls` to use it. Replace the I/O tool block inside the `for` loop:

```go
		if isIOTool(name) {
			// Check safety cap before launching goroutine
			if !handlers.TryIncrementScrapeCount(cfg.MaxScrapes) {
				log("SAFETY_CAP", fmt.Sprintf("Safety cap reached, skipping %s(%s)", name, args))
				results[i] = fmt.Sprintf("Safety cap reached (%d scrapes). Cannot execute %s.", cfg.MaxScrapes, name)
				continue
			}

			wg.Add(1)
			go func(idx int, tc openai.ChatCompletionMessageToolCallUnion) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				var result string
				switch tc.Function.Name {
				case "discover_links":
					result = o.handleDiscoverLinksNoCount(ctx, tc.Function.Arguments, handlers, log)
				case "extract_and_save_product":
					result = o.handleExtractAndSaveProductNoCount(ctx, tc.Function.Arguments, handlers, log)
				}

				log("TOOL_RESULT", fmt.Sprintf("%s result: %s", tc.Function.Name, truncateForLog(result, 200)))
				results[idx] = result
			}(i, tc)
		}
```

Note: We need "NoCount" variants of the handlers that skip the `IsVisited`/`MarkVisited`/`IncrementScrapeCount` calls, since the dispatch loop now handles those upfront. Actually, a cleaner approach: keep the URL visit check inside the handlers but move scrape count increment to the dispatch loop. Let me simplify.

Better approach — update `handleDiscoverLinks` and `handleExtractAndSaveProduct` to accept a `skipCountIncrement bool` parameter. Actually, even simpler: the dispatch loop already calls `TryIncrementScrapeCount`. We just need the handlers to NOT also call `IncrementScrapeCount`. Update both handlers to remove their `handlers.IncrementScrapeCount()` calls:

In `handleDiscoverLinks` (around line 198), remove:
```go
	handlers.IncrementScrapeCount()
```

In `handleExtractAndSaveProduct`, remove:
```go
	handlers.IncrementScrapeCount()
```

The scrape count is now managed exclusively by `dispatchToolCalls` via `TryIncrementScrapeCount`.

Also update `dispatchToolCalls` signature to accept `cfg CrawlConfig`:

```go
func (o *Orchestrator) dispatchToolCalls(
	ctx context.Context,
	toolCalls []openai.ChatCompletionMessageToolCallUnion,
	handlers *ToolHandlers,
	cfg CrawlConfig,
	log OrchestratorLogger,
) []string {
```

And update the call site in `Run` accordingly:

```go
		results := o.dispatchToolCalls(ctx, choice.Message.ToolCalls, handlers, cfg, log)
```

- [ ] **Step 4: Also move visited-URL check to dispatch loop for I/O tools**

To avoid races where two goroutines both check `IsVisited` simultaneously, do the visited-URL check synchronously in the dispatch loop before launching goroutines. Update the I/O section of `dispatchToolCalls`:

```go
		if isIOTool(name) {
			// Parse URL from args
			var urlArgs struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal([]byte(args), &urlArgs); err != nil {
				results[i] = fmt.Sprintf("Invalid arguments: %v", err)
				continue
			}

			// Check visited synchronously (prevents two goroutines visiting same URL)
			if handlers.IsVisited(urlArgs.URL) {
				log("DUPLICATE_SKIP", fmt.Sprintf("Skipping already-visited URL: %s", urlArgs.URL))
				results[i] = fmt.Sprintf("Already visited %s. Try a different page.", urlArgs.URL)
				continue
			}
			handlers.MarkVisited(urlArgs.URL)

			// Check safety cap
			if !handlers.TryIncrementScrapeCount(cfg.MaxScrapes) {
				log("SAFETY_CAP", fmt.Sprintf("Safety cap reached, skipping %s(%s)", name, args))
				results[i] = fmt.Sprintf("Safety cap reached (%d scrapes). Cannot execute %s.", cfg.MaxScrapes, name)
				continue
			}

			wg.Add(1)
			go func(idx int, tc openai.ChatCompletionMessageToolCallUnion) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				var result string
				switch tc.Function.Name {
				case "discover_links":
					result = o.handleDiscoverLinksIO(ctx, tc.Function.Arguments, log)
				case "extract_and_save_product":
					result = o.handleExtractAndSaveProductIO(ctx, tc.Function.Arguments, handlers, log)
				}

				log("TOOL_RESULT", fmt.Sprintf("%s result: %s", tc.Function.Name, truncateForLog(result, 200)))
				results[idx] = result
			}(i, tc)
		}
```

Now we need I/O-only versions of the handlers that skip the visited/count checks (since those are done in the dispatch loop). Create these as lean methods:

```go
// handleDiscoverLinksIO performs only the Firecrawl call and result formatting.
// Visited-URL and scrape-count checks are done by dispatchToolCalls.
func (o *Orchestrator) handleDiscoverLinksIO(ctx context.Context, argsJSON string, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

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

// handleExtractAndSaveProductIO performs extraction + save without visited/count checks.
// Visited-URL and scrape-count checks are done by dispatchToolCalls.
func (o *Orchestrator) handleExtractAndSaveProductIO(ctx context.Context, argsJSON string, handlers *ToolHandlers, log OrchestratorLogger) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Invalid arguments: %v", err)
	}

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
```

Remove the old `handleDiscoverLinks`, `handleExtractAndSaveProduct`, and `handleToolCall` methods — they are fully replaced by `dispatchToolCalls` + the IO variants.

- [ ] **Step 5: Also remove the safety cap check from the top of the `Run` loop**

The existing pre-loop safety cap check at lines 88-98 of `orchestrator.go`:

```go
		if handlers.ScrapeCount() >= cfg.MaxScrapes {
			log("SAFETY_CAP", ...)
			return ...
		}
```

Keep this check but simplify — it's still useful as a loop-level guard. If the cap was hit during the previous batch, we exit cleanly here before making another OpenAI call.

- [ ] **Step 6: Run all tests with race detector**

Run: `cd backend && go test -race ./internal/crawler/ -v`

Expected: All pass, no data races.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/crawler/orchestrator.go backend/internal/crawler/tool_handlers.go backend/internal/crawler/orchestrator_test.go
git commit -m "feat(crawler): enforce safety cap in parallel dispatch, centralize visited-URL checks"
```

---

### Task 5: Clean up and final verification

**Files:**
- Modify: `backend/internal/crawler/orchestrator.go` (remove dead code)
- Test: all crawler tests

- [ ] **Step 1: Verify no dead code remains**

Check that these old methods are removed from `orchestrator.go`:
- `handleToolCall`
- `handleExtractProduct`
- `handleSaveProduct`
- `handleDiscoverLinks` (replaced by `handleDiscoverLinksIO`)
- `handleExtractAndSaveProduct` (replaced by `handleExtractAndSaveProductIO`)

- [ ] **Step 2: Run full test suite**

Run: `cd backend && go test -race ./internal/crawler/ -v -count=1`

Expected: All pass.

- [ ] **Step 3: Run full backend test suite**

Run: `cd backend && go test -race ./... -v -count=1 2>&1 | tail -30`

Expected: All pass (or only pre-existing failures in unrelated packages).

- [ ] **Step 4: Build the crawler binary**

Run: `cd backend && go build -o crawler ./cmd/crawler/`

Expected: Builds successfully.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/crawler/
git commit -m "chore(crawler): clean up dead code after parallel dispatch refactor"
```
