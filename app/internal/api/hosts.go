package api

import (
	"database/sql"
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

	sshKey := r.FormValue("ssh_key")
	sshPassword := r.FormValue("ssh_password")
	tags := r.FormValue("tags")

	_, err := h.q.CreateHost(r.Context(), dbq.CreateHostParams{
		Name:        r.FormValue("name"),
		Ip:          r.FormValue("ip"),
		SshUser:     r.FormValue("ssh_user"),
		SshPort:     port,
		SshKey:      sql.NullString{String: sshKey, Valid: sshKey != ""},
		SshPassword: sql.NullString{String: sshPassword, Valid: sshPassword != ""},
		Tags:        sql.NullString{String: tags, Valid: tags != ""},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/hosts", http.StatusSeeOther)
}

func (h *HostHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	host, err := h.q.GetHost(r.Context(), id)
	if err != nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	// Find deployments that targeted this host
	hostIDStr := sql.NullString{String: strconv.FormatInt(id, 10), Valid: true}
	deployments, err := h.q.ListDeploymentsByHostID(r.Context(), hostIDStr)
	if err != nil {
		deployments = nil
	}

	templates.HostDetailPage(host, deployments).Render(r.Context(), w)
}

func (h *HostHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	host, err := h.q.GetHost(r.Context(), id)
	if err != nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	// Load global SSH key as fallback
	globalSSHKey, _ := h.q.GetSetting(r.Context(), "ssh_key_path")

	result := TestSSHConnection(host, globalSSHKey)
	templates.TestConnectionResult(result).Render(r.Context(), w)
}

func (h *HostHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	port, _ := strconv.ParseInt(r.FormValue("ssh_port"), 10, 64)
	if port == 0 {
		port = 22
	}

	sshKey := r.FormValue("ssh_key")
	sshPassword := r.FormValue("ssh_password")
	tags := r.FormValue("tags")

	// If password field is empty, preserve the existing password
	if sshPassword == "" {
		existing, err := h.q.GetHost(r.Context(), id)
		if err == nil {
			sshPassword = existing.SshPassword.String
		}
	}

	err = h.q.UpdateHost(r.Context(), dbq.UpdateHostParams{
		Name:        r.FormValue("name"),
		Ip:          r.FormValue("ip"),
		SshUser:     r.FormValue("ssh_user"),
		SshPort:     port,
		SshKey:      sql.NullString{String: sshKey, Valid: sshKey != ""},
		SshPassword: sql.NullString{String: sshPassword, Valid: sshPassword != ""},
		Tags:        sql.NullString{String: tags, Valid: tags != ""},
		ID:          id,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/hosts/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
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
