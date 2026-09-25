package main

import (
	"context"
	"counter/internal/cache"
	"counter/internal/config"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/migrations"
	"counter/internal/observability"
	"counter/internal/router"
	"counter/internal/service"
	"counter/internal/store"
	"flag"
	"fmt"
	"log"
	"net"
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

	// Migrations hold a session-level advisory lock, so use a direct database
	// connection when a separate migration URL is configured. Neon pooled URLs
	// are unsuitable for this lock and require an explicit direct URL.
	databaseURL := databaseURLForCommand(cfg.DatabaseURL, cfg.MigrationDatabaseURL, *migrateFlag != "")
	if *migrateFlag != "" {
		if cfg.MigrationDatabaseURL != "" {
			log.Println("Using MIGRATION_DATABASE_URL for database migration")
		}
		if database.IsNeonPoolerURL(databaseURL) {
			log.Fatal("database migrations require a direct Neon URL; set MIGRATION_DATABASE_URL without the -pooler hostname")
		}
	}

	// Initialize database
	dbCfg := &database.DBConfig{
		DatabaseURL:            databaseURL,
		MaxOpenConns:           cfg.DBMaxOpenConns,
		MaxIdleConns:           cfg.DBMaxIdleConns,
		ConnMaxIdleTime:        time.Duration(cfg.DBMaxIdleTime) * time.Second,
		StatementTimeout:       time.Duration(cfg.DBStatementTimeoutMS) * time.Millisecond,
		LockTimeout:            time.Duration(cfg.DBLockTimeoutMS) * time.Millisecond,
		IdleTransactionTimeout: time.Duration(cfg.DBIdleTransactionTimeoutMS) * time.Millisecond,
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

	schemaVersion, err := migrations.VerifySchemaCompatibility(context.Background(), db)
	if err != nil {
		log.Fatalf("Database schema is not compatible: %v", err)
	}
	health := observability.NewHealthState(migrations.LatestVersion)
	metrics := observability.NewMetrics()

	cacheTTL := time.Duration(cfg.CounterReadCacheTTLSeconds) * time.Second
	var counterReadCache service.CounterReadCache
	var closeCounterReadCache func() error
	if cfg.CounterReadCacheRedisURL != "" {
		redisCache, err := cache.NewRedisCounterReadCache(cfg.CounterReadCacheRedisURL, cfg.CounterReadCacheMaxEntries, cacheTTL)
		if err != nil {
			log.Fatalf("Failed to initialize counter read cache: %v", err)
		}
		counterReadCache = redisCache
		closeCounterReadCache = redisCache.Close
		log.Printf("Counter read cache configured: Redis, max_entries=%d ttl=%s", cfg.CounterReadCacheMaxEntries, cacheTTL)
	} else {
		memoryCache, err := cache.NewMemoryCounterReadCache(cfg.CounterReadCacheMaxEntries, cacheTTL)
		if err != nil {
			log.Fatalf("Failed to initialize in-memory counter read cache: %v", err)
		}
		counterReadCache = memoryCache
		log.Printf("Counter read cache configured: in-memory, max_entries=%d ttl=%s", cfg.CounterReadCacheMaxEntries, cacheTTL)
	}
	if closeCounterReadCache != nil {
		defer closeCounterReadCache()
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
	if cfg.RateLimitRedisURL != "" {
		sharedLimiter, err := middleware.NewRedisRateLimitBackend(cfg.RateLimitRedisURL)
		if err != nil {
			log.Printf("Redis rate limit backend configuration invalid; using local limiter")
		} else {
			rateLimiter.SetSharedBackend(sharedLimiter)
			defer rateLimiter.Close()
			log.Println("Shared Redis rate limiting configured; local fallback remains enabled")
		}
	}

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
	r := router.NewRouterWithObservability(
		db,
		corsConfig,
		rateLimiter,
		cfg.APIKey,
		cfg.LegacyAPIKeyEnabled,
		logger,
		sentryConfig,
		middleware.RouteTimeouts{
			Read:     time.Duration(cfg.RequestReadTimeoutSeconds) * time.Second,
			Mutation: time.Duration(cfg.RequestMutationTimeoutSeconds) * time.Second,
			Database: time.Duration(cfg.DBTimeoutMS) * time.Millisecond,
		},
		cfg.ServerConcurrency,
		router.OperationalOptions{
			Health:           health,
			Metrics:          metrics,
			CounterReadCache: counterReadCache,
			Version:          Version,
			SchemaVersion:    schemaVersion,
		},
	)

	// Helper function to find last index of a byte in a string

	// Configure server
	server := &fasthttp.Server{
		Handler:            r.ServeHTTP,
		Name:               "Counter API",
		ReadTimeout:        time.Duration(cfg.ServerReadTimeoutSeconds) * time.Second,
		WriteTimeout:       time.Duration(cfg.ServerWriteTimeoutSeconds) * time.Second,
		IdleTimeout:        time.Duration(cfg.ServerIdleTimeoutSeconds) * time.Second,
		Concurrency:        cfg.ServerConcurrency,
		MaxConnsPerIP:      cfg.ServerMaxConnsPerIP,
		MaxRequestBodySize: cfg.MaxRequestBodyBytes,
	}

	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		close(stopCleanup)
		log.Fatalf("Failed to bind server on %s: %v", addr, err)
	}
	health.MarkStarted(schemaVersion)
	log.Printf("Starting server on %s", addr)
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-sigChan:
		log.Printf("Shutting down server after %s...", sig)
		health.MarkDraining()
		close(stopCleanup)
		if err := server.Shutdown(); err != nil {
			log.Printf("Error during server shutdown: %v", err)
		}
		if err := <-serverErrors; err != nil {
			log.Printf("Server stopped with error: %v", err)
		}
		health.MarkStopped()
		log.Println("Server stopped")
	case err := <-serverErrors:
		close(stopCleanup)
		health.MarkStopped()
		if err != nil {
			log.Printf("Server stopped unexpectedly: %v", err)
		}
	}
}

func databaseURLForCommand(runtimeURL, migrationURL string, isMigration bool) string {
	if isMigration && migrationURL != "" {
		return migrationURL
	}
	return runtimeURL
}
