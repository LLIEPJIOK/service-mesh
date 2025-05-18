package ratelimiter_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter"
	"github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewSlidingWindow(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		cfg                *ratelimiter.Config
		mockSetup          func(repoMock *mocks.MockRepository, cfg *ratelimiter.Config)
		expectedStatusCode int
	}{
		{
			name: "Success - Request allowed",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
			},
			mockSetup: func(repoMock *mocks.MockRepository, cfg *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(nil).
					Once()
				repoMock.On("CountRecords", mock.Anything, mock.AnythingOfType("string")).
					Return(int64(4), nil).
					Once()
				repoMock.On("AddRecord", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).
					Return(nil).
					Once()
				repoMock.On("ExpireKey", mock.Anything, mock.AnythingOfType("string"), cfg.Window).
					Return(nil).
					Once()
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Rate Limited - Request denied",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
				Name:    "test",
			},
			mockSetup: func(repoMock *mocks.MockRepository, _ *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(nil).
					Once()
				repoMock.On("CountRecords", mock.Anything, mock.AnythingOfType("string")).
					Return(int64(5), nil).
					Once()
			},
			expectedStatusCode: http.StatusTooManyRequests,
		},
		{
			name: "Error - RemoveOldRecords fails",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
				Name:    "test",
			},
			mockSetup: func(repoMock *mocks.MockRepository, _ *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(errors.New("db error")).
					Once()
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "Error - CountRecords fails",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
				Name:    "test",
			},
			mockSetup: func(repoMock *mocks.MockRepository, _ *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(nil).
					Once()
				repoMock.On("CountRecords", mock.Anything, mock.AnythingOfType("string")).
					Return(int64(0), errors.New("db error")).
					Once()
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "Error - AddRecord fails",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
				Name:    "test",
			},
			mockSetup: func(repoMock *mocks.MockRepository, _ *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(nil).
					Once()
				repoMock.On("CountRecords", mock.Anything, mock.AnythingOfType("string")).
					Return(int64(4), nil).
					Once()
				repoMock.On("AddRecord", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).
					Return(errors.New("db error")).
					Once()
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name: "Error - ExpireKey fails",
			cfg: &ratelimiter.Config{
				Window:  1 * time.Minute,
				MaxHits: 5,
				Name:    "test",
			},
			mockSetup: func(repoMock *mocks.MockRepository, cfg *ratelimiter.Config) {
				repoMock.On(
					"RemoveOldRecords",
					mock.Anything,
					mock.AnythingOfType("string"),
					mock.AnythingOfType("time.Time"),
					mock.AnythingOfType("time.Time"),
				).
					Return(nil).
					Once()
				repoMock.On("CountRecords", mock.Anything, mock.AnythingOfType("string")).
					Return(int64(4), nil).
					Once()
				repoMock.On("AddRecord", mock.Anything, mock.AnythingOfType("string"), mock.AnythingOfType("time.Time")).
					Return(nil).
					Once()
				repoMock.On("ExpireKey", mock.Anything, mock.AnythingOfType("string"), cfg.Window).
					Return(errors.New("db error")).
					Once()
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoMock := mocks.NewMockRepository(t)
			tc.mockSetup(repoMock, tc.cfg)

			middleware := ratelimiter.NewSlidingWindow(repoMock, tc.cfg)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte("OK"))
				assert.NoError(t, err, "failed to write response")
			})

			testHandler := middleware(nextHandler)

			req := httptest.NewRequest("GET", "http://example.com/test", http.NoBody)
			req = req.WithContext(context.Background())
			req.RemoteAddr = "192.0.2.1:1234"

			rr := httptest.NewRecorder()

			testHandler.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Code, "expected status code does not match")
		})
	}
}
