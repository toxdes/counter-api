package main

import (
	"counter/internal/cache"
	"counter/internal/config"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/migrations"
	"counter/internal/models"
	"counter/internal/router"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	// Define CLI flags
	versionFlag := flag.Bool("version", false, "Print version information")
	migrateFlag := flag.String("db-migrate", "", "Run database migrations (up or down)")
	flag.Parse()

	// Handle version flag
	if *versionFlag {
		fmt.Printf("Counter API v %s\n", Version)
		return
	}

	// Load .env file if present (for local development)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	dbCfg := &database.DBConfig{
		DatabaseURL:  cfg.DatabaseURL,
		MaxOpenConns: cfg.DBMaxOpenConns,
		MaxIdleConns: cfg.DBMaxIdleConns,
	}

	db, err := database.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Verify database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Database connection established")

	// Handle migration commands
	if *migrateFlag != "" {
		switch *migrateFlag {
		case "up":
			log.Println("Running database migrations (up)...")
			if err := migrations.RunUp(db); err != nil {
				log.Fatalf("Migration failed: %v", err)
			}
			log.Println("Migrations completed successfully")
			return
		case "down":
			log.Println("Running database migrations (down)...")
			if err := migrations.RunDown(db); err != nil {
				log.Fatalf("Migration failed: %v", err)
			}
			log.Println("Rollback completed successfully")
			return
		default:
			log.Fatalf("Invalid migration direction: %s (use 'up' or 'down')", *migrateFlag)
		}
	}

	// Initialize middleware
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		AllowCredentials: cfg.CORSAllowCredentials,
		MaxAge:           cfg.CORSMaxAge,
	}

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitGetMultiplier, cfg.RateLimitWindow)

	logger := middleware.NewDefaultLogger(cfg.LogLevel)

	// Initialize Sentry configuration
	var sentryConfig *middleware.SentryConfig
	if cfg.SentryDSN != "" {
		sentryConfig = &middleware.SentryConfig{
			DSN:         cfg.SentryDSN,
			Environment: cfg.SentryEnvironment,
			Release:     cfg.SentryRelease,
			SampleRate:  cfg.SentrySampleRate,
		}
	}

	// Start rate limiter cleanup goroutine
	stopCleanup := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.RateLimitCleanup) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				rateLimiter.Cleanup(time.Duration(cfg.RateLimitCleanup) * time.Second)
			case <-stopCleanup:
				return
			}
		}
	}()

	// Initialize cache if enabled
	var cachedCounter *cache.CachedCounter
	ctx := context.Background()

		// Helper function to find last index of a byte in a string
		indexLast := func(s string, sep byte) int {
			for i := len(s) - 1; i >= 0; i-- {
				if s[i] == sep {
					return i
				}
			}
			return -1
		}

	if cfg.CacheEnabled {
		log.Printf("Initializing cache (size=%d, workers=%d, queue=%d, ttl=%ds)",
			cfg.CacheSize, cfg.CacheWorkers, cfg.CacheQueueSize, cfg.CacheTTLSeconds)

		// Create LRU cache
		lruCache := cache.NewLRUCache(cfg.CacheSize)

		// Create fetch function for cache misses
		fetchFunc := func(key string) (*models.Counter, error) {
			// Parse key format: tenant_id:counter_id
			var tenantID, counterID string
			if idx := indexLast(key, ':'); idx != -1 {
				tenantID = key[:idx]
				counterID = key[idx+1:]
			} else {
				return nil, fmt.Errorf("invalid cache key format")
			}

			// Fetch from database
			var counter models.Counter
			err := db.Get(
				&counter,
				"SELECT id, tenant_id, label, value, max_delta, created_at, updated_at FROM counters WHERE id = $1 AND tenant_id = $2",
				counterID, tenantID,
			)
			if err != nil {
				return nil, err
			}
			return &counter, nil
		}

		// Create write function for async writes
		writeFunc := func(counterID string, delta int64) error {
			// Async write to database
			_, err := db.Exec(
				"UPDATE counters SET value = value + $1, updated_at = NOW() WHERE id = $2",
				delta, counterID,
			)
			return err
		}

		// Create cached counter instance
		cachedCounter = cache.NewCachedCounter(lruCache, fetchFunc, writeFunc, cfg.CacheWorkers, cfg.CacheQueueSize)
		cachedCounter.Start(ctx)

		log.Printf("Cache initialized and started")
	}

	// Create router
	var r *router.Router
	if cfg.CacheEnabled && cachedCounter != nil {
		r = router.NewCachedRouter(db, cachedCounter, corsConfig, rateLimiter, cfg.APIKey, logger, sentryConfig)
	} else {
		r = router.NewRouter(db, corsConfig, rateLimiter, cfg.APIKey, logger, sentryConfig)
	}

	// Helper function to find last index of a byte in a string

	// Configure server
	server := &fasthttp.Server{
		Handler:            r.ServeHTTP,
		Name:               "Counter API",
		ReadTimeout:        time.Second * 10,
		WriteTimeout:       time.Second * 10,
		MaxRequestBodySize: 1 * 1024 * 1024, // 1MB max request body
	}

	// Start server in goroutine
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
		log.Printf("Starting server on %s", addr)
		if err := server.ListenAndServe(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")

	// Shutdown cache first to drain pending writes
	if cfg.CacheEnabled && cachedCounter != nil {
		log.Printf("Shutting down cache (waiting up to %d seconds)...", cfg.CacheShutdownWait)
		cachedCounter.Shutdown()
		log.Printf("Cache shutdown complete")
	}

	// Stop cleanup goroutine
	close(stopCleanup)

	if err := server.Shutdown(); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Server stopped")
}
