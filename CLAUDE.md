# CLAUDE.md

- Always read SPEC.md before making changes
- Backend: Go + chi + Templ + HTMX + SQLite
- No CGO, use modernc.org/sqlite
- No JavaScript frameworks, no Node.js
- Run `templ generate` after changing .templ files
- All Ansible playbooks go in deploy/products/<name>/ansible/
- Follow existing code style
