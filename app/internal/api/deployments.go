package api

import (
	"net/http"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
)

type DeploymentHandler struct {
	q *dbq.Queries
}

func NewDeploymentHandler(q *dbq.Queries) *DeploymentHandler {
	return &DeploymentHandler{q: q}
}

func (h *DeploymentHandler) List(w http.ResponseWriter, r *http.Request) {
	deployments, err := h.q.ListDeployments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.DeploymentsPage(deployments).Render(r.Context(), w)
}
