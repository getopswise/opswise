package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/internal/runner"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
)

type DeploymentHandler struct {
	q      *dbq.Queries
	deploy *runner.DeployService
}

func NewDeploymentHandler(q *dbq.Queries, deploy *runner.DeployService) *DeploymentHandler {
	return &DeploymentHandler{q: q, deploy: deploy}
}

func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	deployments, err := h.q.ListDeployments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.DeploymentsPage(deployments).Render(r.Context(), w)
}

func (h *DeploymentHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	dep, err := h.q.GetDeployment(r.Context(), id)
	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	templates.DeploymentDetailPage(dep).Render(r.Context(), w)
}

// LogStream handles SSE streaming of deployment logs.
func (h *DeploymentHandler) LogStream(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	dep, err := h.q.GetDeployment(r.Context(), id)
	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// If deployment is already finished, send existing log and close
	if dep.Status == "success" || dep.Status == "failed" {
		if dep.Log.Valid {
			fmt.Fprintf(w, "data: %s\n\n", dep.Log.String)
		}
		fmt.Fprintf(w, "event: done\ndata: %s\n\n", dep.Status)
		flusher.Flush()
		return
	}

	// Subscribe to live updates
	ch := h.deploy.Subscribe(id)

	// Send existing log first if any
	if dep.Log.Valid && dep.Log.String != "" {
		fmt.Fprintf(w, "data: %s\n\n", dep.Log.String)
		flusher.Flush()
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			h.deploy.Unsubscribe(id, ch)
			return
		case line, ok := <-ch:
			if !ok {
				// Channel closed — deployment finished
				// Re-read to get final status
				dep, _ = h.q.GetDeployment(ctx, id)
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", dep.Status)
				flusher.Flush()
				return
			}
			if line == "" {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

func (h *DeploymentHandler) Redeploy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	dep, err := h.q.GetDeployment(r.Context(), id)
	if err != nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	var hostIDs []int64
	json.Unmarshal([]byte(dep.HostIds), &hostIDs)

	var config map[string]string
	if dep.Config.Valid {
		json.Unmarshal([]byte(dep.Config.String), &config)
	}

	var hosts []dbq.Host
	for _, hid := range hostIDs {
		host, err := h.q.GetHost(r.Context(), hid)
		if err == nil {
			hosts = append(hosts, host)
		}
	}

	newID, err := h.deploy.StartDeployment(r.Context(), runner.DeployParams{
		Name:       dep.Name,
		Type:       dep.Type,
		TargetName: dep.TargetName,
		Mode:       dep.Mode,
		HostIDs:    hostIDs,
		Config:     config,
		Hosts:      hosts,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/deployments/%d", newID))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/deployments/%d", newID), http.StatusSeeOther)
}
