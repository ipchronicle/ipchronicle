package center

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/ipchronicle/ipchronicle/internal/center/admin"
	"github.com/ipchronicle/ipchronicle/internal/center/database"
	"github.com/ipchronicle/ipchronicle/internal/center/nodes"
	"github.com/ipchronicle/ipchronicle/internal/center/syncws"
	"github.com/ipchronicle/ipchronicle/internal/generated/api"
)

type HTTPOptions struct {
	Version        string
	Web            http.Handler
	Administrator  *admin.Service
	Nodes          *nodes.Service
	SyncHub        *syncws.Hub
	Store          *database.Store
	ExternalOrigin *url.URL
	TrustedProxies []netip.Prefix
}

func NewHTTPHandler(options HTTPOptions) http.Handler {
	if strings.TrimSpace(options.Version) == "" {
		panic("center version must not be empty")
	}
	if options.Web == nil || options.Administrator == nil || options.Nodes == nil || options.SyncHub == nil || options.Store == nil {
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
	router.Get("/api/v1/agent/sync/{sessionID}", agentSyncWebSocket(options.Nodes, options.SyncHub))

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

func agentSyncWebSocket(nodeService *nodes.Service, hub *syncws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, api.InvalidRequest, nil)
			return
		}
		credential := bearerToken(requestSecurityFromContext(r.Context()).Authorization)
		authorization, err := nodeService.AuthorizeSync(r.Context(), credential, sessionID)
		switch {
		case errors.Is(err, nodes.ErrAgentUnauthenticated):
			writeError(w, http.StatusUnauthorized, api.AgentUnauthenticated, nil)
			return
		case errors.Is(err, nodes.ErrAgentRevoked):
			writeError(w, http.StatusForbidden, api.AgentRevoked, nil)
			return
		case errors.Is(err, nodes.ErrSyncSessionUnavailable):
			writeError(w, http.StatusForbidden, api.SyncSessionUnavailable, nil)
			return
		case err != nil:
			structuredInternalError(w, r, err)
			return
		}

		connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			return
		}
		revalidationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, revalidationErr := nodeService.AuthorizeSync(revalidationContext, credential, sessionID)
		cancel()
		if revalidationErr != nil {
			_ = connection.Close(websocket.StatusNormalClosure, "sync session unavailable")
			return
		}
		handle, attached := hub.Attach(authorization.NodeID.String(), sessionID.String(), authorization.ExpiresAt, connection)
		if !attached {
			return
		}
		revalidationContext, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		_, revalidationErr = nodeService.AuthorizeSync(revalidationContext, credential, sessionID)
		cancel()
		if revalidationErr != nil {
			handle.Close()
			return
		}
		if err := handle.Run(); err != nil && !errors.Is(err, context.Canceled) &&
			websocket.CloseStatus(err) != websocket.StatusNormalClosure {
			log.Printf("Agent sync connection for node %s closed: %v", authorization.NodeID, err)
		}
	}
}

func limitAPIRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/agent/") {
			w.Header().Set("Cache-Control", "no-store")
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
