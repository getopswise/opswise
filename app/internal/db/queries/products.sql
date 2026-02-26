-- name: ListProducts :many
SELECT * FROM products ORDER BY name;

-- name: GetProductByName :one
SELECT * FROM products WHERE name = ? LIMIT 1;

-- name: UpsertProduct :exec
INSERT INTO products (name, display_name, description, version, icon, modes)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    display_name = excluded.display_name,
    description = excluded.description,
    version = excluded.version,
    icon = excluded.icon,
    modes = excluded.modes;
