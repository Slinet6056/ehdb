package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/database"
	"github.com/slinet/ehdb/internal/handler"
	"github.com/slinet/ehdb/internal/logger"
	"github.com/slinet/ehdb/internal/middleware"
	"github.com/slinet/ehdb/internal/scheduler"
	"go.uber.org/zap"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "path to config file")
	enableScheduler := flag.Bool("scheduler", false, "enable task scheduler")
	flag.Parse()

	// Load configuration first
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger with configured level
	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("configuration loaded",
		zap.String("host", cfg.Database.Host),
		zap.Int("port", cfg.API.Port),
	)

	// Initialize database
	if err := database.Init(&cfg.Database, log); err != nil {
		log.Fatal("failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	// Initialize Gin
	if !cfg.API.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(middleware.GinZap(log)) // Use zap logger for Gin
	router.Use(middleware.Recovery(log))
	router.Use(middleware.CORS(cfg.API.CORS, cfg.API.CORSOrigin))

	// Initialize handlers
	galleryHandler := handler.NewGalleryHandler(log)
	listHandler := handler.NewListHandler(log)
	searchHandler := handler.NewSearchHandler(log)
	tagHandler := handler.NewTagHandler(log)
	categoryHandler := handler.NewCategoryHandler(log)
	uploaderHandler := handler.NewUploaderHandler(log)

	// Setup routes
	router.GET("/", func(c *gin.Context) {
		// Serve sadpanda.jpg if exists
		c.File("reference/api/assets/sadpanda.jpg")
	})

	api := router.Group("/api")
	{
		// Gallery routes
		api.GET("/gallery/:gid/:token", galleryHandler.GetGallery)
		api.GET("/gallery/:gid", galleryHandler.GetGallery)
		api.GET("/gallery", galleryHandler.GetGallery)
		api.GET("/g/:gid/:token", galleryHandler.GetGallery)
		api.GET("/g/:gid", galleryHandler.GetGallery)
		api.GET("/g", galleryHandler.GetGallery)

		// List route
		api.GET("/list", listHandler.GetList)

		// Search route
		api.GET("/search", searchHandler.Search)

		// Tag routes
		api.GET("/tag/:tag", tagHandler.GetByTag)
		api.GET("/tag", tagHandler.GetByTag)

		// Category routes
		api.GET("/category/:category", categoryHandler.GetByCategory)
		api.GET("/category", categoryHandler.GetByCategory)
		api.GET("/cat/:category", categoryHandler.GetByCategory)
		api.GET("/cat", categoryHandler.GetByCategory)

		// Uploader routes
		api.GET("/uploader/:uploader", uploaderHandler.GetByUploader)
		api.GET("/uploader", uploaderHandler.GetByUploader)
	}

	// Start scheduler if enabled
	var sched *scheduler.Scheduler
	if *enableScheduler {
		sched = scheduler.New(cfg, log)
		if err := sched.Start(); err != nil {
			log.Fatal("failed to start scheduler", zap.Error(err))
		}
		defer sched.Stop()
		log.Info("scheduler enabled")
	}

	// Start HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.API.Port),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		log.Info("starting HTTP server", zap.Int("port", cfg.API.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("server forced to shutdown", zap.Error(err))
	}

	log.Info("server exited")
}
