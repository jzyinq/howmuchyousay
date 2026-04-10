package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jzy/howmuchyousay/internal/config"
	"github.com/jzy/howmuchyousay/internal/crawler"
	"github.com/jzy/howmuchyousay/internal/store"
)

func main() {
	urlFlag := flag.String("url", "", "Shop URL to crawl (required)")
	timeoutFlag := flag.Duration("timeout", 90*time.Second, "Maximum crawl duration")
	minProductsFlag := flag.Int("min-products", 20, "Minimum number of products to collect")
	verboseFlag := flag.Bool("verbose", false, "Enable verbose output")

	flag.Parse()

	if *urlFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --url flag is required")
		fmt.Fprintln(os.Stderr, "Usage: crawler --url <shop_url> [--timeout 90s] [--min-products 20] [--verbose]")
		os.Exit(1)
	}

	cfg := config.Load()

	if cfg.FirecrawlAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: FIRECRAWL_API_KEY must be set")
		os.Exit(1)
	}

	if cfg.OpenAIAPIKey == "" {
		fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY must be set")
		os.Exit(1)
	}

	// Run migrations
	if err := store.RunMigrations(cfg.DatabaseURL, findMigrationsPath()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Connect to database
	pool, err := store.ConnectDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Create components
	scraper, err := crawler.NewFirecrawlClient(cfg.FirecrawlAPIKey, cfg.FirecrawlAPIURL)
	if err != nil {
		log.Fatalf("Failed to create Firecrawl client: %v", err)
	}

	orch := crawler.NewOrchestrator(cfg.OpenAIAPIKey, cfg.OpenAIModel, "", scraper)

	c := crawler.New(
		scraper,
		orch,
		store.NewShopStore(pool),
		store.NewCrawlStore(pool),
		store.NewProductStore(pool),
		cfg.LogDir,
	)

	crawlCfg := crawler.CrawlConfig{
		URL:         *urlFlag,
		Timeout:     *timeoutFlag,
		MinProducts: *minProductsFlag,
		MaxScrapes:  50,
		Verbose:     *verboseFlag,
	}

	if *verboseFlag {
		fmt.Printf("Starting crawl of %s\n", *urlFlag)
		fmt.Printf("  Timeout: %s\n", *timeoutFlag)
		fmt.Printf("  Min products: %d\n", *minProductsFlag)
		fmt.Printf("  Firecrawl API: %s\n", cfg.FirecrawlAPIURL)
		fmt.Println()
	}

	var progress crawler.ProgressFunc
	if *verboseFlag {
		progress = func(event string, message string) {
			fmt.Printf("[%s] %s\n", event, message)
		}
	}

	result, err := c.RunWithProgress(context.Background(), crawlCfg, nil, progress)
	if err != nil {
		log.Fatalf("Crawl failed: %v", err)
	}

	// Print summary
	fmt.Println("=== Crawl Complete ===")
	fmt.Printf("  Shop ID:      %s\n", result.ShopID)
	fmt.Printf("  Crawl ID:     %s\n", result.CrawlID)
	fmt.Printf("  Products:     %d\n", result.ProductsFound)
	fmt.Printf("  Scrapes:      %d\n", result.ScrapeCount)
	fmt.Printf("  AI requests:  %d\n", result.AIRequestsCount)
	fmt.Printf("  Duration:     %s\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("  Log file:     %s\n", result.LogFilePath)

	if result.ProductsFound < crawlCfg.MinProducts {
		fmt.Printf("\nWarning: Found only %d products (minimum: %d)\n", result.ProductsFound, crawlCfg.MinProducts)
		os.Exit(1)
	}
}

// findMigrationsPath tries common relative paths for the migrations directory.
func findMigrationsPath() string {
	candidates := []string{
		"../migrations",
		"../../migrations",
		"./migrations",
		"backend/migrations",
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return "../migrations" // default fallback
}
