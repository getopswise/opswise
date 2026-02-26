-- name: ListHosts :many
SELECT * FROM hosts ORDER BY created_at DESC;

-- name: GetHost :one
SELECT * FROM hosts WHERE id = ? LIMIT 1;

-- name: CreateHost :one
INSERT INTO hosts (name, ip, ssh_user, ssh_port, ssh_key, ssh_password, tags)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteHost :exec
DELETE FROM hosts WHERE id = ?;
