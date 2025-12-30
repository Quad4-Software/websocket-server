// Package main is the entry point for the ws-relay server.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.quad4.io/Networks/ws-relay/internal/server"
)

func main() {
	host := flag.String("host", getEnv("HOST", "0.0.0.0"), "Server host")
	port := flag.Int("port", getEnvInt("PORT", 8080), "Server port")
	mode := flag.String("mode", getEnv("MODE", "echo"), "Server mode: echo, broadcast, or log")
	domain := flag.String("domain", getEnv("DOMAIN", ""), "Domain name for CORS and WSS enforcement")
	rateLimit := flag.Int("rate-limit", getEnvInt("RATE_LIMIT", 10), "Rate limit per second")
	burst := flag.Int("burst", getEnvInt("BURST", 20), "Burst size for rate limiting")
	clientLimit := flag.Int("client-limit", getEnvInt("CLIENT_LIMIT", 1000), "Maximum number of concurrent clients")

	flag.Parse()

	srv := server.NewServer(*host, *port, *mode, *domain, *rateLimit, *burst, *clientLimit)

	// Set up signal handling for graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server exited gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if result, err := strconv.Atoi(value); err == nil {
			return result
		}
	}
	return defaultValue
}
