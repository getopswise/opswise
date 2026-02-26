package api

import (
	"net/http"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
)

type StackHandler struct {
	q *dbq.Queries
}

func NewStackHandler(q *dbq.Queries) *StackHandler {
	return &StackHandler{q: q}
}

func (h *StackHandler) List(w http.ResponseWriter, r *http.Request) {
	stacks, err := h.q.ListStacks(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.StacksPage(stacks).Render(r.Context(), w)
}
