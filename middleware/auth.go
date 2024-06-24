package middleware

import (
	"log"
	"net/http"
)

// Auth middleware
func Authorize(next http.Handler, validTokens map[string]struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Auth-Token")
		_, ok := validTokens[token]
		if !ok {
			log.Printf("Invalid token. Access forbidden")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
