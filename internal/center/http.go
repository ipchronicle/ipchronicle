package center

import (
	"encoding/json"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type HTTPOptions struct {
	Version        string
	Web            http.Handler
	Administrator  *admin.Service
	Nodes          *nodes.Service
	Store          *database.Store
	ExternalOrigin *url.URL
	TrustedProxies []netip.Prefix
}

func NewHTTPHandler(options HTTPOptions) http.Handler {
	if strings.TrimSpace(options.Version) == "" {
		panic("center version must not be empty")
	}
	if options.Web == nil || options.Administrator == nil || options.Nodes == nil || options.Store == nil {
		panic("center HTTP dependencies must not be nil")
	}

	proxy := newProxyPolicy(options.ExternalOrigin, options.TrustedProxies)
	server := apiServer{
		version:                  options.Version,
		administrator:            options.Administrator,
		nodes:                    options.Nodes,
		configSchemaVersion:      options.Store.ConfigSchemaVersion,
		historySchemaVersion:     options.Store.HistorySchemaVersion,
		externalOriginConfigured: options.ExternalOrigin != nil,
		trustedProxyConfigured:   len(options.TrustedProxies) > 0,
	}
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(proxy.middleware)
	router.Use(limitAPIRequestBody)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	strict := api.NewStrictHandlerWithOptions(server, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  structuredRequestError,
		ResponseErrorHandlerFunc: structuredInternalError,
	})
	api.HandlerWithOptions(strict, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: structuredRequestError,
	})
	router.Handle("/*", options.Web)
	return router
}

func limitAPIRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
		}
		next.ServeHTTP(w, r)
	})
}

func structuredRequestError(w http.ResponseWriter, _ *http.Request, _ error) {
	writeError(w, http.StatusBadRequest, api.InvalidRequest, nil)
}

func structuredInternalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("request %s failed: %v", middleware.GetReqID(r.Context()), err)
	writeError(w, http.StatusInternalServerError, api.InternalError, nil)
}

func writeError(w http.ResponseWriter, status int, code api.ErrorCode, parameters map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	response := api.ErrorResponse{Code: code}
	if len(parameters) > 0 {
		response.Parameters = &parameters
	}
	_ = json.NewEncoder(w).Encode(response)
}
