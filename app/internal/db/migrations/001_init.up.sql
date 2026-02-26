-- Hosts: target VMs/servers
CREATE TABLE hosts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    ip          TEXT NOT NULL,
    ssh_user    TEXT NOT NULL DEFAULT 'root',
    ssh_port    INTEGER NOT NULL DEFAULT 22,
    ssh_key     TEXT,
    tags        TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Products: available tools (grafana, harbor, etc.)
CREATE TABLE products (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT,
    version      TEXT,
    icon         TEXT,
    modes        TEXT NOT NULL
);

-- Stacks: predefined combinations of products
CREATE TABLE stacks (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT,
    products     TEXT NOT NULL
);

-- Deployments: deployment jobs
CREATE TABLE deployments (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    target_name TEXT NOT NULL,
    mode        TEXT NOT NULL,
    host_ids    TEXT NOT NULL,
    config      TEXT,
    status      TEXT NOT NULL DEFAULT 'pending',
    log         TEXT,
    git_pushed  BOOLEAN DEFAULT FALSE,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Settings: global app settings
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
