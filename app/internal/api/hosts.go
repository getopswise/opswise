package api

import (
	"net/http"
	"strconv"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
)

type HostHandler struct {
	q *dbq.Queries
}

func NewHostHandler(q *dbq.Queries) *HostHandler {
	return &HostHandler{q: q}
}

func (h *HostHandler) List(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.q.ListHosts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.HostsPage(hosts).Render(r.Context(), w)
}

func (h *HostHandler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	port, _ := strconv.ParseInt(r.FormValue("ssh_port"), 10, 64)
	if port == 0 {
		port = 22
	}

	_, err := h.q.CreateHost(r.Context(), dbq.CreateHostParams{
		Name:    r.FormValue("name"),
		Ip:      r.FormValue("ip"),
		SshUser: r.FormValue("ssh_user"),
		SshPort: port,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/hosts", http.StatusSeeOther)
}

func (h *HostHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := h.q.DeleteHost(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
