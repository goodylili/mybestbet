package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"mybestbet/internal/platform/config"
)

func TestRequireAdminToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		configured string
		header     string
		expected   int
	}{
		{name: "valid token", configured: "s3cret", header: "Bearer s3cret", expected: http.StatusNoContent},
		{name: "scheme is case insensitive", configured: "s3cret", header: "bearer s3cret", expected: http.StatusNoContent},
		{name: "wrong token", configured: "s3cret", header: "Bearer nope", expected: http.StatusUnauthorized},
		{name: "missing header", configured: "s3cret", header: "", expected: http.StatusUnauthorized},
		{name: "basic scheme", configured: "s3cret", header: "Basic s3cret", expected: http.StatusUnauthorized},
		{name: "unconfigured token fails closed", configured: "", header: "Bearer ", expected: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := &Server{cfg: config.Config{AdminToken: tt.configured}, log: slog.Default()}
			handler := s.requireAdminToken(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/v1/admin/review", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)
			assert.Equal(t, tt.expected, rec.Code)
		})
	}
}
