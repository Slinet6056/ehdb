package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/crawler"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/internal/logger"
	"go.uber.org/zap"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	// Load default config to get log level
	cfg, _ := config.Load("config.yaml")
	logLevel := "info"
	if cfg != nil {
		logLevel = cfg.LogLevel
	}

	// Initialize logger
	log, err := logger.New(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	switch command {
	case "sync":
		runSync(log, os.Args[2:])
	case "resync":
		runResync(log, os.Args[2:])
	case "fetch":
		runFetch(log, os.Args[2:])
	case "torrent-sync":
		runTorrentSync(log, os.Args[2:])
	case "torrent-import":
		runTorrentImport(log, os.Args[2:])
	case "mark-replaced":
		runMarkReplaced(log, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: ehdb-sync <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  sync              Sync latest galleries from E-Hentai")
	fmt.Println("                    Options: -config <path> -host <host> -offset <hours>")
	fmt.Println("  resync            Resync galleries from recent hours")
	fmt.Println("                    Options: -config <path> -hours <N>")
	fmt.Println("  fetch             Manually fetch specific galleries")
	fmt.Println("                    Usage: sync fetch <gid>/<token> [<gid>/<token> ...]")
	fmt.Println("                    Or: sync fetch -file <filename>")
	fmt.Println("  torrent-sync      Sync new torrents from /torrents.php page")
	fmt.Println("                    Options: -config <path> -host <host> -pages <N> -status <s> -search <keyword>")
	fmt.Println("                    Automatically imports missing galleries")
	fmt.Println("  torrent-import    Import torrents for existing galleries")
	fmt.Println("                    Options: -config <path> -host <host>")
	fmt.Println("                    Only processes galleries with root_gid = NULL")
	fmt.Println("  mark-replaced     Mark all replaced galleries")
	fmt.Println("                    Options: -config <path>")
	fmt.Println("\nExamples:")
	fmt.Println("  ehdb-sync sync -host e-hentai.org -offset 2")
	fmt.Println("  ehdb-sync resync -hours 24")
	fmt.Println("  ehdb-sync fetch 123456/abcdef0123 234567/bcdef01234")
	fmt.Println("  ehdb-sync torrent-sync")
	fmt.Println("  ehdb-sync torrent-sync -pages 5")
	fmt.Println("  ehdb-sync torrent-import")
}

// runSync syncs latest galleries
func runSync(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	host := fs.String("host", "", "e-hentai.org or exhentai.org (overrides config)")
	offset := fs.Int("offset", 0, "time offset in hours")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if *host != "" {
		cfg.Crawler.Host = *host
	}
	if *offset != 0 {
		cfg.Crawler.Offset = *offset
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	ctx := context.Background()
	galleryCrawler, err := crawler.NewGalleryCrawler(&cfg.Crawler, logger)
	if err != nil {
		logger.Fatal("failed to create gallery crawler", zap.Error(err))
	}

	if err := galleryCrawler.Sync(ctx); err != nil {
		logger.Fatal("gallery sync failed", zap.Error(err))
	}
	logger.Info("gallery sync completed successfully")
}

// runResync resyncs galleries from recent hours
func runResync(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("resync", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	hours := fs.Int("hours", 24, "resync galleries from the last N hours")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	ctx := context.Background()
	resyncer := crawler.NewResyncer(&cfg.Crawler, logger)
	if err := resyncer.Resync(ctx, *hours); err != nil {
		logger.Fatal("resync failed", zap.Error(err))
	}
	logger.Info("resync completed successfully")
}

// runFetch manually fetches specific galleries
func runFetch(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	file := fs.String("file", "", "file containing gid/token pairs")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	var gidTokens []string
	if *file != "" {
		// Read from file
		content, err := os.ReadFile(*file)
		if err != nil {
			logger.Fatal("failed to read file", zap.Error(err))
		}
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				gidTokens = append(gidTokens, line)
			}
		}
	} else {
		// Read from command line args
		gidTokens = fs.Args()
	}

	if len(gidTokens) == 0 {
		logger.Fatal("no galleries specified")
	}

	ctx := context.Background()
	fetcher := crawler.NewFetcher(&cfg.Crawler, logger)
	if err := fetcher.Fetch(ctx, gidTokens); err != nil {
		logger.Fatal("fetch failed", zap.Error(err))
	}
	logger.Info("fetch completed successfully")
}

// runTorrentSync syncs torrents from torrent list page
func runTorrentSync(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("torrent-sync", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	host := fs.String("host", "", "e-hentai.org or exhentai.org (overrides config)")
	pages := fs.Int("pages", 0, "number of pages to fetch (0 = until reaching existing torrents)")
	status := fs.String("status", "", "torrent status filter")
	search := fs.String("search", "", "search keyword")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if *host != "" {
		cfg.Crawler.Host = *host
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	ctx := context.Background()
	torrentCrawler, err := crawler.NewTorrentCrawler(&cfg.Crawler, logger)
	if err != nil {
		logger.Fatal("failed to create torrent crawler", zap.Error(err))
	}

	// Set options
	torrentCrawler.SetOptions(crawler.TorrentCrawlerOptions{
		MaxPages:   *pages,
		StatusCode: *status,
		Search:     *search,
	})

	if err := torrentCrawler.Sync(ctx); err != nil {
		logger.Fatal("torrent sync failed", zap.Error(err))
	}
	logger.Info("torrent sync completed successfully")
}

// runTorrentImport imports torrents from all galleries
func runTorrentImport(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("torrent-import", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	host := fs.String("host", "", "e-hentai.org or exhentai.org (overrides config)")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	logger.Warn("torrent-import is a heavy operation that will scan all galleries")

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if *host != "" {
		cfg.Crawler.Host = *host
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	ctx := context.Background()
	importer, err := crawler.NewTorrentImporter(&cfg.Crawler, logger)
	if err != nil {
		logger.Fatal("failed to create torrent importer", zap.Error(err))
	}

	if err := importer.ImportAll(ctx); err != nil {
		logger.Fatal("torrent import failed", zap.Error(err))
	}
	logger.Info("torrent import completed successfully")
}

// runMarkReplaced marks all replaced galleries
func runMarkReplaced(logger *zap.Logger, args []string) {
	fs := flag.NewFlagSet("mark-replaced", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	if err := fs.Parse(args); err != nil {
		logger.Fatal("failed to parse flags", zap.Error(err))
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	if err := database.Init(&cfg.Database, logger); err != nil {
		logger.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	ctx := context.Background()
	marker := crawler.NewReplacedMarker(logger)
	if err := marker.MarkReplaced(ctx); err != nil {
		logger.Fatal("mark replaced failed", zap.Error(err))
	}
	logger.Info("mark replaced completed successfully")
}
