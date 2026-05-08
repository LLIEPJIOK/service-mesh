package http

import (
	"net/http"

	"golang.org/x/time/rate"
)

func applyRateLimit(next http.Handler, requestsPerSecond float64, burst int) http.Handler {
	if requestsPerSecond <= 0 {
		return next
	}

	if burst <= 0 {
		burst = 1
	}

	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burst)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
