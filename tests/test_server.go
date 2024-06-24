package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func handler(w http.ResponseWriter, r *http.Request) {
	p := port
	if r.TLS != nil {
		p = httpsPort
	}
	log.Printf("Received request on port %v\n", p)
	fmt.Fprintf(w, "ok from port %v\n", p)
}

var port int
var httpsPort int

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatalf("WebSocket upgrade failed: %v", err)
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read failed: %v", err)
			break
		}

		log.Printf("Received message on port %v: %s", port, msg)

		waitFor := extractWaitForFromMessage(string(msg))

		if waitFor != "" {
			waitForSeconds, err := strconv.Atoi(waitFor)
			if err == nil {
				// Sleep for the specified number of seconds before responding
				time.Sleep(time.Duration(waitForSeconds) * time.Second)
			}
		}

		err = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Received message on port %v: %s", port, msg)))
		if err != nil {
			log.Printf("WebSocket write failed: %v", err)
			break
		}
	}

	err = conn.Close()
	if err != nil {
		log.Printf("WebSocket close failed: %v", err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: server <port>")
		os.Exit(1)
	}

	var err error
	port, err = strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf("Error parsing port: %v", err)
	}

	httpsPort = port + 443

	var wg sync.WaitGroup
	wg.Add(2)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	mux := http.NewServeMux()

	mux.HandleFunc("/websocket", websocketHandler)
	mux.HandleFunc("/", handler)

	httpServer := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: mux,
	}
	httpsServer := &http.Server{
		Addr:    ":" + strconv.Itoa(httpsPort),
		Handler: mux,
	}

	// Start the HTTP server in a goroutine
	go func() {
		log.Printf("Starting HTTP server on :%v\n", port)
		if err := httpServer.ListenAndServe(); err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Start the HTTPS server in a goroutine
	go func() {
		log.Printf("Starting HTTPS server on :%v\n", httpsPort)
		if err := httpsServer.ListenAndServeTLS("../server.crt", "../server.key"); err != nil {
			log.Fatalf("HTTPS server failed: %v", err)
		}
	}()

	// Wait for the shutdown signal
	<-shutdown

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	wg.Wait()
}

var waitForRegexp = regexp.MustCompile(`Hello, server! Wait for (\d+) seconds`)

func extractWaitForFromMessage(message string) string {
	matches := waitForRegexp.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
