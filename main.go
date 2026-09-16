package main

import (
	"context"
	"counter/internal/config"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/migrations"
	"counter/internal/router"
	"counter/internal/service"
	"counter/internal/store"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"

	"github.com/getsentry/sentry-go"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	// Define CLI flags
	versionFlag := flag.Bool("version", false, "Print version information")
	migrateFlag := flag.String("db-migrate", "", "Run database migrations (up or down)")
	reconcileFlag := flag.Bool("reconcile", false, "Reconcile counters against completed operation history")
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

	if *reconcileFlag {
		report, err := service.NewReconciliationService(store.NewCounterStore(db)).Reconcile(context.Background(), 100)
		if err != nil {
			log.Fatalf("Reconciliation failed: %v", err)
		}
		log.Printf("Reconciliation completed: counters=%d operations=%d mismatches=%d initial_value_violations=%d duration=%s",
			report.CountersChecked,
			report.OperationsScanned,
			report.Mismatches,
			report.InitialValueViolations,
			report.CompletedAt.Sub(report.StartedAt),
		)
		if report.Mismatches != 0 || report.InitialValueViolations != 0 {
			log.Fatalf("Reconciliation found inconsistent counter history")
		}
		return
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
		// Determine release version
		sentryRelease := cfg.SentryRelease
		if sentryRelease == "" {
			sentryRelease = Version // Use build-time version
		}

		// Initialize Sentry client
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.SentryDSN,
			Environment:      cfg.SentryEnvironment,
			Release:          sentryRelease,
			SampleRate:       cfg.SentrySampleRate,
			TracesSampleRate: cfg.SentrySampleRate,
			BeforeSend:       middleware.ScrubSentryEvent,
		})
		if err != nil {
			log.Printf("Sentry initialization failed: %v", err)
			log.Printf("Continuing without Sentry error tracking")
		} else {
			log.Printf("Sentry initialized (env=%s, release=%s, sample_rate=%.2f)",
				cfg.SentryEnvironment, sentryRelease, cfg.SentrySampleRate)
		}
		defer sentry.Flush(2 * time.Second)

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

	// Create router
	r := router.NewRouterWithOptions(db, corsConfig, rateLimiter, cfg.APIKey, cfg.LegacyAPIKeyEnabled, logger, sentryConfig)

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

	// Stop cleanup goroutine
	close(stopCleanup)

	if err := server.Shutdown(); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Server stopped")
}
