package middleware

import (
	"net/http"
)

// Authentication middleware (basic implementation for development)
func Authentication(sessionKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// For development, allow all requests through
			// In production, implement proper session/token validation here
			
			// Check for API key in header (simple implementation)
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Allow requests without API key for development
				// In production, return 401 Unauthorized
			}

			// Set user context for request tracking
			// ctx := context.WithValue(r.Context(), "user", "development")
			// r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}