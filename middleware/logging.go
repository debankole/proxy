package middleware

import (
	"log"
	"net/http"
)

// Logging middleware
func LogRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request: %s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
		log.Printf("Completed request: %s %s %s", r.Method, r.URL, w.Header().Get("Status"))
	})
}
