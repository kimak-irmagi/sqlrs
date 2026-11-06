package httpapi

import (
	"net/http"
	"strings"

	"github.com/sqlrs/engine-local/internal/auth"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/prepare"
)

type prepareRoutes struct {
	opts Options
}

func (routes prepareRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/prepare-jobs", routes.handleJobs)
	mux.HandleFunc("/v1/prepare-jobs/", routes.handleJob)
	mux.HandleFunc("/v1/tasks", routes.handleTasks)
}

func (routes prepareRoutes) handleJobs(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireBearer(w, r, routes.opts.AuthToken) {
		return
	}
	if routes.opts.Prepare == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = writeListResponse(w, r, routes.opts.Prepare.ListJobs(readQueryValue(r, "job")))
	case http.MethodPost:
		req, err := decodePrepareRequest(r.Body)
		if err != nil {
			_ = writeError(w, prepare.ErrorResponse{
				Code:    "invalid_argument",
				Message: "invalid json payload",
				Details: err.Error(),
			}, http.StatusBadRequest)
			return
		}
		accepted, err := routes.opts.Prepare.Submit(r.Context(), req)
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
		w.Header().Set("Location", accepted.StatusURL)
		_ = writeJSONStatus(w, accepted, http.StatusCreated)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (routes prepareRoutes) handleJob(w http.ResponseWriter, r *http.Request) {
	if !auth.RequireBearer(w, r, routes.opts.AuthToken) {
		return
	}
	if routes.opts.Prepare == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/prepare-jobs/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/cancel") {
		routes.handleCancel(w, r, strings.TrimSuffix(path, "/cancel"))
		return
	}
	if strings.HasSuffix(path, "/events") {
		routes.handleEvents(w, r, strings.TrimSuffix(path, "/events"))
		return
	}
	if strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		status, ok := routes.opts.Prepare.Get(path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = writeJSON(w, status)
	case http.MethodDelete:
		force, err := parseBoolQuery(r, "force")
		if err != nil {
			_ = writeErrorResponse(w, "invalid_argument", "invalid force", err.Error(), http.StatusBadRequest)
			return
		}
		dryRun, err := parseBoolQuery(r, "dry_run")
		if err != nil {
			_ = writeErrorResponse(w, "invalid_argument", "invalid dry_run", err.Error(), http.StatusBadRequest)
			return
		}
		result, ok := routes.opts.Prepare.Delete(path, deletion.DeleteOptions{Force: force, DryRun: dryRun})
		if !ok {
			_ = writeErrorResponse(w, "not_found", "job not found", "", http.StatusNotFound)
			return
		}
		status := http.StatusOK
		if !dryRun && result.Outcome == deletion.OutcomeBlocked {
			status = http.StatusConflict
		}
		_ = writeJSONStatus(w, result, status)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (routes prepareRoutes) handleCancel(w http.ResponseWriter, r *http.Request, jobID string) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if jobID == "" || strings.Contains(jobID, "/") {
		http.NotFound(w, r)
		return
	}
	status, ok, accepted, err := routes.opts.Prepare.Cancel(jobID)
	if err != nil {
		_ = writeErrorResponse(w, "internal_error", "cancel failed", err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		_ = writeErrorResponse(w, "not_found", "job not found", "", http.StatusNotFound)
		return
	}
	if accepted {
		_ = writeJSONStatus(w, status, http.StatusAccepted)
		return
	}
	_ = writeJSONStatus(w, status, http.StatusOK)
}

func (routes prepareRoutes) handleEvents(w http.ResponseWriter, r *http.Request, jobID string) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if jobID == "" || strings.Contains(jobID, "/") {
		http.NotFound(w, r)
		return
	}
	streamPrepareEvents(w, r, routes.opts.Prepare, jobID)
}

func (routes prepareRoutes) handleTasks(w http.ResponseWriter, r *http.Request) {
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
	_ = writeListResponse(w, r, routes.opts.Prepare.ListTasks(readQueryValue(r, "job")))
}
