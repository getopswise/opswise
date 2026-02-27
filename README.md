# Opswise Deploy

Open source web application for deploying DevOps infrastructure on bare metal VMs — through a simple GUI, without requiring a DevOps team.

Select a product (Grafana, Prometheus, GitLab, Harbor, etc.), pick your target hosts, choose a deployment mode (Ansible, Docker Compose, or Helm), and Opswise handles the rest.

## Features

- **Product catalog** — Grafana, Prometheus, Loki, GitLab CE, Harbor, Keycloak, ArgoCD
- **Predefined stacks** — Monitoring, CI/CD, Vibe Coding
- **Multiple deployment modes** — Ansible playbooks, Docker Compose, Helm charts
- **Live deployment logs** — real-time streaming via SSE
- **Host management** — register target VMs with SSH credentials
- **Git integration** — push generated deployment code to your repository
- **Single binary** — no external dependencies beyond the deployment tools themselves

## Tech Stack

- **Backend:** Go + [chi](https://github.com/go-chi/chi) + [Templ](https://templ.guide) + SQLite
- **Frontend:** [HTMX](https://htmx.org) + plain CSS (no JavaScript frameworks, no Node.js)
- **Database:** SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO)

## Quick Start

### From source

```bash
# Install dependencies
go install github.com/a-h/templ/cmd/templ@latest

# Build
cd app
templ generate
go build -o opswise ./cmd/

# Run
./opswise
```

The server starts on [http://localhost:8080](http://localhost:8080).

### With Docker

```bash
docker build -t opswise .
docker run -p 8080:8080 -v opswise-data:/app/data opswise
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `OPSWISE_DB` | `opswise.db` | Path to SQLite database file |
| `OPSWISE_DEPLOY_DIR` | `../deploy` | Path to deploy directory with playbooks/compose/helm files |

## Project Structure

```
opswise/
├── app/                    # Go application
│   ├── cmd/main.go         # Entrypoint
│   ├── internal/
│   │   ├── api/            # HTTP handlers
│   │   ├── db/             # Database, migrations, sqlc queries
│   │   ├── runner/         # Ansible, Compose, Helm executors
│   │   └── git/            # Git push integration
│   └── web/
│       ├── templates/      # Templ templates
│       └── static/         # CSS, JS (HTMX)
├── deploy/
│   ├── products/           # Per-product automation files
│   │   ├── grafana/
│   │   ├── prometheus/
│   │   ├── loki/
│   │   ├── gitlab/
│   │   ├── harbor/
│   │   ├── keycloak/
│   │   └── argocd/
│   └── stacks/             # Predefined product combinations
├── Dockerfile
└── SPEC.md
```

## SSH Key Management

Opswise uses **key-based SSH authentication only**. Private keys are encrypted with AES-256-GCM and stored in the database — they never need to exist on disk.

### Recommended workflow

```bash
# 1. Generate a dedicated key pair
ssh-keygen -t ed25519 -f /tmp/opswise_key -N ""

# 2. Install the public key on your target host(s)
ssh-copy-id -i /tmp/opswise_key.pub user@host

# 3. Paste the private key into the Opswise GUI (Hosts → Add Host → SSH Private Key)
cat /tmp/opswise_key

# 4. Delete the key files from disk
rm /tmp/opswise_key /tmp/opswise_key.pub
```

After this, the private key lives only encrypted in the SQLite database. Opswise decrypts it in memory when needed for connection tests or Ansible deployments.

### Authentication fallback chain

1. **Per-host key** — stored encrypted in the database via the host form
2. **Global key** — file path configured in Settings
3. **Default keys** — `~/.ssh/id_rsa`, `id_ed25519`, `id_ecdsa`

## Deployment Flow

1. Select a product or stack
2. Choose target hosts
3. Pick deployment mode (Ansible / Compose / Helm)
4. Configure settings (ports, passwords, etc.)
5. Deploy — logs stream live to the browser
6. Optionally push generated code to Git

## Products

| Product | Ansible | Compose | Helm |
|---|---|---|---|
| Grafana | yes | yes | yes |
| Prometheus | yes | yes | yes |
| Loki | yes | yes | — |
| GitLab CE | yes | yes | — |
| Harbor | — | yes | yes |
| Keycloak | — | yes | yes |
| ArgoCD | — | — | yes |

## Stacks

| Stack | Products |
|---|---|
| Monitoring | Prometheus, Grafana, Loki |
| CI/CD | GitLab, Harbor |
| Vibe Coding | GitLab, Harbor, Keycloak, ArgoCD |

## License

Apache 2.0
