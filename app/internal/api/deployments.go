package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getopswise/opswise/app/internal/crypto"
	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/internal/runner"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"
)

type DeploymentHandler struct {
	q         *dbq.Queries
	deploy    *runner.DeployService
	masterKey []byte
}

func NewDeploymentHandler(q *dbq.Queries, deploy *runner.DeployService, masterKey []byte) *DeploymentHandler {
	return &DeploymentHandler{q: q, deploy: deploy, masterKey: masterKey}
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

	// Subscribe to live updates (replays accumulated log)
	ch := h.deploy.Subscribe(id)

	ctx := r.Context()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			h.deploy.Unsubscribe(id, ch)
			return
		case <-keepalive.C:
			// SSE comment to keep connection alive during long tasks
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
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
			fmt.Fprintf(w, "data: <div>%s</div>\n\n", html.EscapeString(line))
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

	// Reconstruct host groups from config if available
	var hostGroups map[string][]dbq.Host
	if groupMapJSON, ok := config["_host_groups_map"]; ok {
		var groupMap map[string][]int64
		if json.Unmarshal([]byte(groupMapJSON), &groupMap) == nil {
			hostGroups = make(map[string][]dbq.Host)
			for groupName, ids := range groupMap {
				for _, id := range ids {
					host, err := h.q.GetHost(r.Context(), id)
					if err == nil {
						hostGroups[groupName] = append(hostGroups[groupName], host)
					}
				}
			}
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
		HostGroups: hostGroups,
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

// Download fetches a file from the remote host via SSH and serves it as a download.
func (h *DeploymentHandler) Download(w http.ResponseWriter, r *http.Request) {
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

	if dep.Status != "success" {
		http.Error(w, "Deployment not successful", http.StatusBadRequest)
		return
	}

	if !dep.DownloadFile.Valid || dep.DownloadFile.String == "" {
		http.Error(w, "No download file configured", http.StatusNotFound)
		return
	}

	// Resolve host from deployment
	var hostIDs []int64
	json.Unmarshal([]byte(dep.HostIds), &hostIDs)
	if len(hostIDs) == 0 {
		http.Error(w, "No hosts found for deployment", http.StatusBadRequest)
		return
	}

	// Determine which host to download from: prefer first master if groups exist
	targetHostID := hostIDs[0]
	if dep.Config.Valid {
		var config map[string]string
		if json.Unmarshal([]byte(dep.Config.String), &config) == nil {
			if groupMapJSON, ok := config["_host_groups_map"]; ok {
				var groupMap map[string][]int64
				if json.Unmarshal([]byte(groupMapJSON), &groupMap) == nil {
					if masters, ok := groupMap["masters"]; ok && len(masters) > 0 {
						targetHostID = masters[0]
					}
				}
			}
		}
	}

	host, err := h.q.GetHost(r.Context(), targetHostID)
	if err != nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	// Build SSH client config
	var authMethods []ssh.AuthMethod

	// Try per-host encrypted key
	if host.SshKey.Valid && host.SshKey.String != "" {
		keyData, err := crypto.Decrypt(host.SshKey.String, h.masterKey)
		if err != nil {
			log.Printf("download: failed to decrypt SSH key for host %s: %v", host.Name, err)
		} else {
			signer, err := ssh.ParsePrivateKey(keyData)
			if err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	// Try global key
	globalSSHKey, _ := h.q.GetSetting(r.Context(), "ssh_key_path")
	if globalSSHKey != "" {
		if result, _ := loadKeyFile(globalSSHKey); result != nil {
			authMethods = append(authMethods, result)
		}
	}

	if len(authMethods) == 0 {
		http.Error(w, "No SSH authentication method available", http.StatusInternalServerError)
		return
	}

	sshConfig := &ssh.ClientConfig{
		User:            host.SshUser,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host.Ip, host.SshPort)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		http.Error(w, "SSH connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		http.Error(w, "SSH session failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	if err := session.Run("cat " + dep.DownloadFile.String); err != nil {
		http.Error(w, "Failed to read remote file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	content := buf.String()

	// For kubeconfig files, replace 127.0.0.1 with the host's actual IP
	if strings.Contains(dep.DownloadFile.String, "kubeconfig") || strings.Contains(dep.DownloadFile.String, "rke2.yaml") {
		content = strings.ReplaceAll(content, "127.0.0.1", host.Ip)
	}

	filename := "download"
	if dep.DownloadName.Valid && dep.DownloadName.String != "" {
		filename = dep.DownloadName.String
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Write([]byte(content))
}

// loadKeyFile reads an SSH private key file and returns an AuthMethod.
func loadKeyFile(path string) (ssh.AuthMethod, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}
