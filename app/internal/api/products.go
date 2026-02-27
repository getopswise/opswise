package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/internal/runner"
	"github.com/getopswise/opswise/app/web/templates"
	"github.com/go-chi/chi/v5"
)

type ProductHandler struct {
	q         *dbq.Queries
	deploy    *runner.DeployService
	deployDir string
}

func NewProductHandler(q *dbq.Queries, deploy *runner.DeployService, deployDir string) *ProductHandler {
	return &ProductHandler{q: q, deploy: deploy, deployDir: deployDir}
}

func (h *ProductHandler) List(w http.ResponseWriter, r *http.Request) {
	products, err := h.q.ListProducts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	templates.ProductsPage(products).Render(r.Context(), w)
}

func (h *ProductHandler) Detail(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	product, err := h.q.GetProductByName(r.Context(), name)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	hosts, err := h.q.ListHosts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var modes []string
	json.Unmarshal([]byte(product.Modes), &modes)

	defaults := runner.LoadProductDefaults(h.deployDir, name)
	meta := runner.LoadProductMeta(h.deployDir, name)

	templates.ProductDetailPage(product, hosts, modes, defaults, meta.HostGroups).Render(r.Context(), w)
}

func (h *ProductHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	product, err := h.q.GetProductByName(r.Context(), name)
	if err != nil {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	mode := r.FormValue("mode")

	meta := runner.LoadProductMeta(h.deployDir, name)

	var hostIDs []int64
	var hosts []dbq.Host
	var hostGroups map[string][]dbq.Host

	if len(meta.HostGroups) > 0 {
		// Parse host_role_<id> fields from form
		hostGroups = make(map[string][]dbq.Host)
		hostGroupIDs := make(map[string][]int64)
		for key, values := range r.Form {
			if !strings.HasPrefix(key, "host_role_") || len(values) == 0 || values[0] == "" {
				continue
			}
			idStr := strings.TrimPrefix(key, "host_role_")
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			groupName := values[0]
			host, err := h.q.GetHost(r.Context(), id)
			if err != nil {
				continue
			}
			hostIDs = append(hostIDs, id)
			hosts = append(hosts, host)
			hostGroups[groupName] = append(hostGroups[groupName], host)
			hostGroupIDs[groupName] = append(hostGroupIDs[groupName], id)
		}

		// Validate minimum host counts
		for _, g := range meta.HostGroups {
			if len(hostGroups[g.Name]) < g.MinHosts {
				http.Error(w, fmt.Sprintf("Group %q requires at least %d host(s)", g.DisplayName, g.MinHosts), http.StatusBadRequest)
				return
			}
		}
	} else {
		// Legacy checkbox selection
		hostIDStrs := r.Form["host_ids"]
		for _, idStr := range hostIDStrs {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			hostIDs = append(hostIDs, id)
			host, err := h.q.GetHost(r.Context(), id)
			if err == nil {
				hosts = append(hosts, host)
			}
		}
	}

	if len(hosts) == 0 && mode == "ansible" {
		http.Error(w, "At least one host is required for ansible deployments", http.StatusBadRequest)
		return
	}

	// Collect config values from form
	config := make(map[string]string)
	for key, values := range r.Form {
		if strings.HasPrefix(key, "config_") && len(values) > 0 && values[0] != "" {
			config[strings.TrimPrefix(key, "config_")] = values[0]
		}
	}

	// Serialize host group mapping into config for redeploy
	if len(hostGroups) > 0 {
		groupMap := make(map[string][]int64)
		for groupName, groupHosts := range hostGroups {
			for _, gh := range groupHosts {
				groupMap[groupName] = append(groupMap[groupName], gh.ID)
			}
		}
		groupMapJSON, _ := json.Marshal(groupMap)
		config["_host_groups_map"] = string(groupMapJSON)
	}

	deployName := fmt.Sprintf("Deploy %s via %s", product.DisplayName, mode)
	deployID, err := h.deploy.StartDeployment(r.Context(), runner.DeployParams{
		Name:       deployName,
		Type:       "product",
		TargetName: name,
		Mode:       mode,
		HostIDs:    hostIDs,
		Config:     config,
		Hosts:      hosts,
		HostGroups: hostGroups,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// HTMX redirect to deployment detail
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", fmt.Sprintf("/deployments/%d", deployID))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/deployments/%d", deployID), http.StatusSeeOther)
}
