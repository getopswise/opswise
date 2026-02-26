-- name: ListStacks :many
SELECT * FROM stacks ORDER BY name;

-- name: GetStackByName :one
SELECT * FROM stacks WHERE name = ? LIMIT 1;

-- name: UpsertStack :exec
INSERT INTO stacks (name, display_name, description, products)
VALUES (?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
    display_name = excluded.display_name,
    description = excluded.description,
    products = excluded.products;
