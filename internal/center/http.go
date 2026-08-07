package center

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

func NewHTTPHandler(version string, web http.Handler) http.Handler {
	if strings.TrimSpace(version) == "" {
		panic("center version must not be empty")
	}
	if web == nil {
		panic("web handler must not be nil")
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	strict := api.NewStrictHandler(statusServer{version: version}, nil)
	api.HandlerFromMux(strict, router)
	router.Handle("/*", web)

	return router
}
