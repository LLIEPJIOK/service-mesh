package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LLIEPJIOK/sidecar/pkg/middleware"
	"github.com/stretchr/testify/assert"
)

func TestWrap(t *testing.T) {
	var executionOrder []string

	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "mw1")

			next.ServeHTTP(w, r)

			w.Header().Add("X-Middleware-1", "Applied")
		})
	}

	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			executionOrder = append(executionOrder, "mw2")

			next.ServeHTTP(w, r)

			w.Header().Add("X-Middleware-2", "Applied")
		})
	}

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		executionOrder = append(executionOrder, "base")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Base Handler Reached"))
	})

	wrappedHandler := middleware.Wrap(baseHandler, mw1, mw2)

	req := httptest.NewRequest("GET", "/", http.NoBody)
	rr := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rr, req)

	expectedOrder := []string{"mw1", "mw2", "base"}
	assert.Equal(
		t,
		expectedOrder,
		executionOrder,
		"Middlewares did not execute in the expected order",
	)
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")
	assert.Equal(t, "Applied", rr.Header().Get("X-Middleware-1"), "Middleware 1 header missing")
	assert.Equal(t, "Applied", rr.Header().Get("X-Middleware-2"), "Middleware 2 header missing")
	assert.Contains(t, rr.Body.String(), "Base Handler Reached", "Base handler was not reached")
}
