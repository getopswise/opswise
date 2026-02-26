# Opswise Deploy – Technical Specification

## Overview

Opswise Deploy is an open source web application that allows users to deploy DevOps
infrastructure (Grafana, Prometheus, GitLab, Harbor, etc.) on bare metal VMs, via
Docker Compose, or on Kubernetes — through a simple GUI, without requiring a DevOps team.

The user provides VM IPs, selects a product or stack, configures settings, and Opswise
executes the appropriate Ansible playbook, Compose file, or Helm chart in the background.
All generated automation code can optionally be pushed to a Git repository.

---

## Tech Stack

### Backend
- Language: Go 1.24+
- Web Framework: net/http (stdlib) + chi router
- Templating: Templ (https://templ.guide)
- Frontend interaction: HTMX
- Database: SQLite via modernc.org/sqlite (no CGO)
- ORM/Query: sqlc for type-safe queries
- Migrations: golang-migrate
- Ansible execution: os/exec calling ansible-playbook
- Config: environment variables, optional config.yaml override

### Frontend
- HTMX 1.9 + SSE extension for live log streaming
- CSS: plain CSS with CSS variables, no framework
- No Node.js, no JavaScript frameworks, no build step

### Deployment of Opswise itself
- Single Go binary
- Systemd service
- Docker image (Dockerfile included)
- SQLite database file: opswise.db

### Build
- Makefile with targets: build, run, dev, generate, templ-generate, sqlc-generate, docker, docker-run, clean
- `templ generate` after changing .templ files
- `sqlc generate` from app/ dir after changing .sql query files

---

## Repository Structure

```
opswise/
├── app/                          # Opswise Deploy application
│   ├── cmd/
│   │   └── main.go               # entrypoint
│   ├── internal/
│   │   ├── api/                  # HTTP handlers
│   │   │   ├── hosts.go
│   │   │   ├── products.go
│   │   │   ├── stacks.go
│   │   │   ├── deployments.go
│   │   │   └── settings.go
│   │   ├── db/
│   │   │   ├── migrations/       # SQL migration files
│   │   │   ├── queries/          # sqlc query files
│   │   │   └── db.go
│   │   ├── runner/
│   │   │   ├── ansible.go        # ansible-playbook executor
│   │   │   ├── helm.go           # helm executor
│   │   │   ├── compose.go        # docker compose executor
│   │   │   ├── deploy.go         # DeployService: orchestration, log streaming
│   │   │   └── defaults.go       # defaults.yml parser for auto-form fields
│   │   ├── git/
│   │   │   └── push.go           # push generated code to git
│   │   └── models/
│   │       └── models.go
│   ├── web/
│   │   ├── templates/            # .templ files
│   │   │   ├── layout.templ
│   │   │   ├── dashboard.templ
│   │   │   ├── hosts.templ
│   │   │   ├── host_detail.templ
│   │   │   ├── products.templ
│   │   │   ├── product_detail.templ
│   │   │   ├── stacks.templ
│   │   │   ├── stack_detail.templ
│   │   │   ├── deployments.templ
│   │   │   ├── deployment_detail.templ
│   │   │   ├── settings.templ
│   │   │   └── helpers.go        # template helper functions
│   │   └── static/
│   │       ├── css/
│   │       └── js/
│   ├── go.mod
│   ├── go.sum
│   └── sqlc.yaml
│
├── observe/                      # Opswise Observe (future)
├── ai/                           # Opswise AI (future)
│
├── deploy/
│   ├── products/                 # individual tool automation
│   │   ├── grafana/
│   │   │   ├── ansible/
│   │   │   │   ├── install.yml
│   │   │   │   ├── uninstall.yml
│   │   │   │   └── defaults.yml  # default variables
│   │   │   ├── compose/
│   │   │   │   └── docker-compose.yml
│   │   │   └── helm/
│   │   │       └── values.yaml
│   │   ├── prometheus/
│   │   ├── harbor/
│   │   ├── gitlab/
│   │   ├── keycloak/
│   │   ├── loki/
│   │   ├── argocd/
│   │   └── README.md
│   └── stacks/                   # predefined product combinations
│       ├── monitoring/
│       │   ├── stack.yaml        # defines which products + config
│       │   └── README.md
│       ├── cicd/
│       └── vibe-coding/
│
├── Makefile
├── Dockerfile
├── SPEC.md
├── README.md
└── LICENSE                       # Apache 2.0
```

---

## Database Schema

```sql
-- Hosts: target VMs/servers
CREATE TABLE hosts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    ip          TEXT NOT NULL,
    ssh_user    TEXT NOT NULL DEFAULT 'root',
    ssh_port    INTEGER NOT NULL DEFAULT 22,
    ssh_key     TEXT,                          -- path to private key
    tags        TEXT,                          -- comma-separated
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Products: available tools (grafana, harbor, etc.)
CREATE TABLE products (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,          -- e.g. "grafana"
    display_name TEXT NOT NULL,                -- e.g. "Grafana"
    description TEXT,
    version     TEXT,
    icon        TEXT,                          -- emoji or svg name
    modes       TEXT NOT NULL                  -- json: ["ansible","compose","helm"]
);

-- Stacks: predefined combinations of products
CREATE TABLE stacks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    products    TEXT NOT NULL                  -- json array of product names
);

-- Deployments: deployment jobs
CREATE TABLE deployments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,                 -- "product" or "stack"
    target_name TEXT NOT NULL,                 -- product/stack name
    mode        TEXT NOT NULL,                 -- "ansible", "compose", "helm"
    host_ids    TEXT NOT NULL,                 -- json array of host ids
    config      TEXT,                          -- json: user-provided config values
    status      TEXT NOT NULL DEFAULT 'pending', -- pending/running/success/failed
    log         TEXT,                          -- full deployment log output
    git_pushed  BOOLEAN DEFAULT FALSE,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Settings: global app settings
CREATE TABLE settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL
);
-- Default settings:
-- git_enabled, git_url, git_token, git_branch
-- ssh_key_path
```

---

## HTTP Routes

```
GET  /                          → dashboard (overview, recent deployments)
GET  /hosts                     → list hosts
POST /hosts                     → add host
GET  /hosts/:id                 → host detail
DELETE /hosts/:id               → remove host

GET  /products                  → list available products
GET  /products/:name            → product detail + deploy form
POST /products/:name/deploy     → trigger deployment (HTMX)

GET  /stacks                    → list available stacks
GET  /stacks/:name              → stack detail + deploy form
POST /stacks/:name/deploy       → trigger deployment (HTMX)

GET  /deployments               → list all deployments
GET  /deployments/:id           → deployment detail + live log (SSE)
GET  /deployments/:id/log       → SSE stream of deployment log

GET  /settings                  → settings page
POST /settings                  → save settings
```

---

## Dashboard

The dashboard (`GET /`) shows:
- Hero section with terminal demo
- Stats grid: host count, product count, deployment count (via CountHosts/CountProducts/CountDeployments queries)
- Quick action cards linking to hosts, products, stacks, and deployments

---

## Deployment Flow

1. User selects a product or stack in the GUI
2. User selects target host(s) from the host list
3. User selects deployment mode: ansible / compose / helm
4. User fills in config values (auto-generated from product's `defaults.yml`)
5. On submit → POST /products/:name/deploy
6. Server creates a deployment record in DB (status: pending)
7. Server spawns goroutine via `DeployService.StartDeployment`
8. `DeployService` manages subscriber channels for live log streaming
9. Output is streamed line by line: written to DB log + broadcast to SSE subscribers
10. Frontend connects to `/deployments/:id/log` via HTMX SSE extension for live output
11. On completion → status updated to success or failed
12. If git push enabled → generated code committed and pushed to configured repo

### DeployService Architecture

`DeployService` (internal/runner/deploy.go) is the central deployment orchestrator:
- Maintains a map of `subscribers` (deployment ID → []chan string) for real-time log streaming
- `StartDeployment` creates a DB record and launches a goroutine
- `Subscribe/Unsubscribe` manage SSE client connections
- `broadcast` fans out log lines to all subscribers of a deployment
- On completion, closes all subscriber channels and cleans up

---

## Ansible Runner Integration

```go
// internal/runner/ansible.go

func RunPlaybook(playbook string, inventory []string, extraVars map[string]string) (io.Reader, error) {
    args := []string{
        playbook,
        "-i", strings.Join(inventory, ",") + ",",
    }
    for k, v := range extraVars {
        args = append(args, "--extra-vars", fmt.Sprintf("%s=%s", k, v))
    }
    cmd := exec.Command("ansible-playbook", args...)
    return cmd.StdoutPipe(), cmd.Start()
}
```

Playbook path is resolved from `deploy/products/<name>/ansible/install.yml`.

### SSH Credential Flow

SSH key resolution follows a fallback chain (highest priority first):

1. **Per-Host Key** – `hosts.ssh_key` set via the Add Host form
2. **Global Key** – `ssh_key_path` setting from the Settings page
3. **Ansible Default** – if neither is set, Ansible uses its own default key discovery

The resolved key is passed as `ansible_ssh_private_key_file` in the inventory entry for each host.

---

## Stack Definition Format

```yaml
# deploy/stacks/monitoring/stack.yaml
name: monitoring
display_name: Monitoring Stack
description: Prometheus + Grafana + Loki for full observability
products:
  - name: prometheus
    config:
      port: 9090
  - name: grafana
    config:
      port: 3000
      admin_password: "{{ .AdminPassword }}"
  - name: loki
    config:
      port: 3100
```

---

## Product Definition Format

Each product has a `defaults.yml` that defines configurable variables:

```yaml
# deploy/products/grafana/ansible/defaults.yml
grafana_version: "10.4.0"
grafana_port: 3000
grafana_admin_user: admin
grafana_admin_password: "changeme"
grafana_data_dir: /var/lib/grafana
```

These variables are surfaced as form fields in the GUI automatically:
- `LoadProductDefaults(deployDir, productName)` in `internal/runner/defaults.go` parses the YAML
- Variables are sorted alphabetically and rendered as input fields
- The product prefix (e.g. `grafana_`) is stripped from labels for readability
- Password fields (keys containing "password") use `<input type="password">`
- Form field names are prefixed with `config_` to be collected as deployment config

---

## Initial Products to Implement

Priority order:

1. **Grafana** – ansible + compose + helm
2. **Prometheus** – ansible + compose + helm
3. **Loki** – ansible + compose + helm
4. **GitLab CE** – ansible + compose + helm
5. **Harbor** – ansible + compose + helm
6. **Keycloak** – ansible + compose + helm
7. **ArgoCD** – helm only

---

## Initial Stacks to Implement

1. **monitoring** → prometheus + grafana + loki
2. **cicd** → gitlab + harbor
3. **vibe-coding** → gitlab + harbor + keycloak + argocd

---

## Git Push Feature

After a successful deployment, if git is configured in settings:

1. Copy used playbook/compose/helm files to a temp directory
2. `git clone` the configured repository
3. Copy files into `opswise-generated/<deployment-id>/`
4. `git add`, `git commit`, `git push`
5. Mark deployment as `git_pushed = true`

Supported: GitLab, GitHub, Gitea (any git remote with token auth).

---

## Non-Goals (v1)

- No multi-user / authentication (single user, local network tool)
- No cloud deployment support
- No Windows support
- No rollback functionality (v2)

---

## Future Modules

- **Opswise Observe** – monitoring dashboard, alert management
- **Opswise AI** – AI agents for incident detection and response (on-premise LLM)
