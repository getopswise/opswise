package api

import (
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/getopswise/opswise/app/internal/crypto"
	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
)

type HostHandler struct {
	q         *dbq.Queries
	masterKey []byte
}

func NewHostHandler(q *dbq.Queries, masterKey []byte) *HostHandler {
	return &HostHandler{q: q, masterKey: masterKey}
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
	tags := r.FormValue("tags")

	var encKey, fingerprint string
	if sshKey != "" {
		// Compute fingerprint from key content
		fp, err := crypto.Fingerprint([]byte(sshKey))
		if err != nil {
			http.Error(w, "invalid SSH private key: "+err.Error(), http.StatusBadRequest)
			return
		}
		fingerprint = fp

		// Encrypt key content
		enc, err := crypto.Encrypt([]byte(sshKey), h.masterKey)
		if err != nil {
			http.Error(w, "failed to encrypt SSH key", http.StatusInternalServerError)
			return
		}
		encKey = enc
	}

	_, err := h.q.CreateHost(r.Context(), dbq.CreateHostParams{
		Name:           r.FormValue("name"),
		Ip:             r.FormValue("ip"),
		SshUser:        r.FormValue("ssh_user"),
		SshPort:        port,
		SshKey:         sql.NullString{String: encKey, Valid: encKey != ""},
		Tags:           sql.NullString{String: tags, Valid: tags != ""},
		KeyFingerprint: sql.NullString{String: fingerprint, Valid: fingerprint != ""},
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

	// Decrypt SSH key if present
	var decryptedKey []byte
	if host.SshKey.Valid && host.SshKey.String != "" {
		dk, err := crypto.Decrypt(host.SshKey.String, h.masterKey)
		if err != nil {
			log.Printf("failed to decrypt SSH key for host %d: %v", id, err)
		} else {
			decryptedKey = dk
		}
	}

	// Load global SSH key as fallback
	globalSSHKey, _ := h.q.GetSetting(r.Context(), "ssh_key_path")

	result := TestSSHConnection(host, globalSSHKey, decryptedKey)
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
	tags := r.FormValue("tags")

	// Get existing host for preserving values
	existing, err := h.q.GetHost(r.Context(), id)
	if err != nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}

	var encKey string
	var fingerprint string
	if sshKey != "" {
		// New key provided — validate, fingerprint, encrypt
		fp, err := crypto.Fingerprint([]byte(sshKey))
		if err != nil {
			http.Error(w, "invalid SSH private key: "+err.Error(), http.StatusBadRequest)
			return
		}
		fingerprint = fp

		enc, err := crypto.Encrypt([]byte(sshKey), h.masterKey)
		if err != nil {
			http.Error(w, "failed to encrypt SSH key", http.StatusInternalServerError)
			return
		}
		encKey = enc
	} else {
		// Preserve existing encrypted key and fingerprint
		encKey = existing.SshKey.String
		fingerprint = existing.KeyFingerprint.String
	}

	err = h.q.UpdateHost(r.Context(), dbq.UpdateHostParams{
		Name:           r.FormValue("name"),
		Ip:             r.FormValue("ip"),
		SshUser:        r.FormValue("ssh_user"),
		SshPort:        port,
		SshKey:         sql.NullString{String: encKey, Valid: encKey != ""},
		Tags:           sql.NullString{String: tags, Valid: tags != ""},
		KeyFingerprint: sql.NullString{String: fingerprint, Valid: fingerprint != ""},
		ID:             id,
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
