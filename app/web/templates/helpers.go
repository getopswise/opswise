package templates

import (
	"database/sql"
	"fmt"
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

func nullTime(nt sql.NullTime) string {
	if nt.Valid {
		return nt.Time.Format("2006-01-02 15:04")
	}
	return ""
}
