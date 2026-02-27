package templates

import (
	"database/sql"
	"fmt"
	"strings"
)

func itoa(n int64) string {
	return fmt.Sprintf("%d", n)
}

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func keyPlaceholder(ns sql.NullString) string {
	if ns.Valid && ns.String != "" {
		return "Key stored (paste new key to replace)"
	}
	return "Paste PEM private key content (optional)"
}

// SSHTestResult holds the result of an SSH connection test.
type SSHTestResult struct {
	Success bool
	Message string
}

func nullTime(nt sql.NullTime) string {
	if nt.Valid {
		return nt.Time.Format("2006-01-02 15:04")
	}
	return ""
}

// formatVarLabel converts a YAML variable key like "grafana_admin_password"
// into a human-readable label like "Admin Password" by stripping the product
// prefix and title-casing the remaining words.
func formatVarLabel(key, productName string) string {
	s := strings.TrimPrefix(key, productName+"_")
	s = strings.ReplaceAll(s, "_", " ")
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
