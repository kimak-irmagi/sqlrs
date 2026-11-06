package httpapi

import (
	"net/http"

	"github.com/sqlrs/engine-local/internal/auth"
	"github.com/sqlrs/engine-local/internal/prepare"
)

type cacheRoutes struct {
	opts Options
}

func (routes cacheRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/cache/status", routes.handleStatus)
	mux.HandleFunc("/v1/cache/explain/prepare", routes.handleExplainPrepare)
}

func (routes cacheRoutes) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireBearer(w, r, routes.opts.AuthToken) {
		return
	}
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if routes.opts.Prepare == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	status, err := routes.opts.Prepare.CacheStatus(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_ = writeJSON(w, status)
}

func (routes cacheRoutes) handleExplainPrepare(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireBearer(w, r, routes.opts.AuthToken) {
		return
	}
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if routes.opts.Prepare == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req, err := decodePrepareRequest(r.Body)
	if err != nil {
		_ = writeErrorResponse(w, "invalid_argument", "invalid json payload", err.Error(), http.StatusBadRequest)
		return
	}

	result, err := routes.opts.Prepare.CacheExplain(r.Context(), req)
	if err != nil {
		resp := prepare.ToErrorResponse(err)
		status := http.StatusInternalServerError
		if _, ok := err.(prepare.ValidationError); ok {
			status = http.StatusBadRequest
		}
		if _, ok := err.(*prepare.ValidationError); ok {
			status = http.StatusBadRequest
		}
		_ = writeError(w, *resp, status)
		return
	}
	_ = writeJSON(w, result)
}
