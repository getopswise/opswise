package api

import (
	"net/http"

	"github.com/getopswise/opswise/app/internal/db/dbq"
	"github.com/getopswise/opswise/app/web/templates"
)

var settingsKeys = []string{
	"git_enabled",
	"git_url",
	"git_token",
	"git_branch",
	"ssh_key_path",
}

type SettingsHandler struct {
	q *dbq.Queries
}

func NewSettingsHandler(q *dbq.Queries) *SettingsHandler {
	return &SettingsHandler{q: q}
}

func (h *SettingsHandler) Page(w http.ResponseWriter, r *http.Request) {
	vals := h.loadSettings(r)
	templates.SettingsPage(vals, false, "").Render(r.Context(), w)
}

func (h *SettingsHandler) Save(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// git_enabled is a checkbox — if absent, store "false"
	gitEnabled := "false"
	if r.FormValue("git_enabled") == "on" || r.FormValue("git_enabled") == "true" {
		gitEnabled = "true"
	}
	h.q.UpsertSetting(r.Context(), dbq.UpsertSettingParams{Key: "git_enabled", Value: gitEnabled})

	for _, key := range settingsKeys {
		if key == "git_enabled" {
			continue
		}
		val := r.FormValue(key)
		if val != "" {
			h.q.UpsertSetting(r.Context(), dbq.UpsertSettingParams{Key: key, Value: val})
		}
	}

	vals := h.loadSettings(r)
	templates.SettingsPage(vals, true, "Settings saved successfully.").Render(r.Context(), w)
}

func (h *SettingsHandler) loadSettings(r *http.Request) map[string]string {
	vals := make(map[string]string)
	for _, key := range settingsKeys {
		val, err := h.q.GetSetting(r.Context(), key)
		if err == nil {
			vals[key] = val
		}
	}
	return vals
}
