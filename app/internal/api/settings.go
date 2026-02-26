package api

import (
	"net/http"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
)

type SettingsHandler struct {
	q *dbq.Queries
}

func NewSettingsHandler(q *dbq.Queries) *SettingsHandler {
	return &SettingsHandler{q: q}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	templates.SettingsPage().Render(r.Context(), w)
}
