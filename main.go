package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"pr/middleware"
	"pr/proxy"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

func parseEnvVars() ([]*url.URL, []*url.URL, map[string]struct{}) {

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	var httpUrls []*url.URL
	var httpsUrls []*url.URL
	validTokens := make(map[string]struct{})

	envVars := os.Environ()

	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]

		if strings.HasPrefix(key, "HTTP_SERVER_URL_") {
			url, err := url.Parse(value)
			if err != nil {
				log.Printf("Error parsing URL %s: %v", value, err)
				continue
			}

			httpUrls = append(httpUrls, url)
		} else if strings.HasPrefix(key, "HTTPS_SERVER_URL_") {
			url, err := url.Parse(value)
			if err != nil {
				log.Printf("Error parsing URL %s: %v", value, err)
				continue
			}

			httpsUrls = append(httpsUrls, url)
		} else if strings.HasPrefix(key, "AUTH_TOKEN_") {
			validTokens[value] = struct{}{}
		}
	}

	return httpUrls, httpsUrls, validTokens
}

func main() {
	httpUrls, httpsUrls, validTokens := parseEnvVars()
	skipCertCheck := os.Getenv("SKIP_CERT_CHECK") == "true"
	gracefulShutdownTimeoutStr := os.Getenv("GRACEFUL_SHUTDOWN_TIMEOUT_SEC")
	if gracefulShutdownTimeoutStr == "" {
		gracefulShutdownTimeoutStr = "20"
	}
	gracefulShutdownTimeout, err := time.ParseDuration(gracefulShutdownTimeoutStr + "s")
	log.Printf("Proxy pid: %v\n", os.Getpid())
	log.Printf("gracefulShutdownTimeout: %v\n", gracefulShutdownTimeout)
	if err != nil {
		log.Fatalf("Error parsing graceful shutdown timeout: %v", err)
	}

	pool := proxy.NewServerPool(httpUrls, httpsUrls)

	httpHandler := middleware.LogRequest(middleware.Authorize((proxy.ProxyHandler(pool, false, false, skipCertCheck)), validTokens))
	httpsHandler := middleware.LogRequest(middleware.Authorize((proxy.ProxyHandler(pool, false, true, skipCertCheck)), validTokens))
	wsHandler := middleware.LogRequest(middleware.Authorize((proxy.ProxyHandler(pool, true, false, skipCertCheck)), validTokens))

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/websocket", wsHandler.ServeHTTP)
	httpMux.HandleFunc("/", httpHandler.ServeHTTP)

	httpsMux := http.NewServeMux()
	httpsMux.HandleFunc("/websocket", wsHandler.ServeHTTP)
	httpsMux.HandleFunc("/", httpsHandler.ServeHTTP)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpMux,
	}
	httpsServer := &http.Server{
		Addr:    ":8443",
		Handler: httpsHandler,
	}

	// Channel to receive the shutdown signal
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	// Start the HTTP server in a goroutine
	go func() {
		log.Println("Starting HTTP server on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error only if it's not due to graceful shutdown
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Start the HTTPS server in a goroutine
	go func() {
		log.Println("Starting HTTPS server on :8443")
		if err := httpsServer.ListenAndServeTLS("server.crt", "server.key"); err != nil && err != http.ErrServerClosed {
			// Log error only if it's not due to graceful shutdown
			log.Fatalf("HTTPS server failed: %v", err)
		}
	}()

	// Wait for the shutdown signal
	<-shutdown
	log.Printf("Shutdown signal received")

	// Context with a timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// Shut down HTTP server
	go func() {
		defer wg.Done()
		if err := httpServer.Shutdown(ctx); err != nil {
			log.Fatalf("HTTP server shutdown failed: %v", err)
		}
	}()

	// Shut down HTTPS server
	go func() {
		defer wg.Done()
		if err := httpsServer.Shutdown(ctx); err != nil {
			log.Fatalf("HTTPS server shutdown failed: %v", err)
		}
	}()

	c := make(chan struct{})
	go func() {
		defer close(c)
		proxy.ActiveConnWaiter.Wait()
		wg.Wait()
	}()

	fmt.Println("Waiting for active WS connections to close...")

	// Waiting for servers to shutdown and for all WS connections to close before exiting the process
	// Or until the context is timed out
	select {
	case <-c:
		log.Println("HTTPS server stopped. All active WS connections have been closed")
	case <-ctx.Done():
		log.Fatalf("HTTPS server stopped. Some connections might have been terminated")
	}
}
