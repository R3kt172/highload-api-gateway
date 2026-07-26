package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/R3kt172/highload-api-gateway/internal/config"
	"github.com/R3kt172/highload-api-gateway/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

func TestRouterUsesLongestPrefix(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "api")
	}))
	defer api.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "admin")
	}))
	defer admin.Close()

	metrics := observability.NewMetrics(prometheus.NewRegistry())
	router, err := NewRouter([]config.Route{
		{Prefix: "/api", Upstream: api.URL},
		{Prefix: "/api/admin", Upstream: admin.URL},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), metrics)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Body.String() != "admin" {
		t.Fatalf("body = %q, want admin", response.Body.String())
	}
}
