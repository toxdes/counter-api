package main

import (
	"counter/internal/config"
	"counter/internal/database"
	"counter/internal/middleware"
	"counter/internal/router"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/valyala/fasthttp"
)

func main() {
	// Load .env file if present (for local development)
	_ = godotenv.Load()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	dbCfg := &database.DBConfig{
		Host:         cfg.DBHost,
		Port:         cfg.DBPort,
		User:         cfg.DBUser,
		Password:     cfg.DBPassword,
		DBName:       cfg.DBName,
		SSLMode:      cfg.DBSSLMode,
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

	// Initialize middleware
	corsConfig := &middleware.CORSConfig{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   cfg.CORSAllowedMethods,
		AllowedHeaders:   cfg.CORSAllowedHeaders,
		AllowCredentials: cfg.CORSAllowCredentials,
		MaxAge:           cfg.CORSMaxAge,
	}

	rateLimiter := middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)

	logger := middleware.NewDefaultLogger(cfg.LogLevel)

	// Start rate limiter cleanup goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(cfg.RateLimitCleanup) * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			rateLimiter.Cleanup(time.Duration(cfg.RateLimitCleanup) * time.Second)
		}
	}()

	// Create router
	r := router.NewRouter(db, corsConfig, rateLimiter, cfg.APIKey, logger)

	// Configure server
	server := &fasthttp.Server{
		Handler:      r.ServeHTTP,
		Name:         "Counter API",
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 10,
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
	if err := server.Shutdown(); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}

	log.Println("Server stopped")
}
