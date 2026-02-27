-- name: ListDeployments :many
SELECT * FROM deployments ORDER BY created_at DESC;

-- name: GetDeployment :one
SELECT * FROM deployments WHERE id = ? LIMIT 1;

-- name: CreateDeployment :one
INSERT INTO deployments (name, type, target_name, mode, host_ids, config, status)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateDeploymentStatus :exec
UPDATE deployments SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateDeploymentLog :exec
UPDATE deployments SET log = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: SetDeploymentGitPushed :exec
UPDATE deployments SET git_pushed = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateDeploymentGUIURL :exec
UPDATE deployments SET gui_url = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: ListDeploymentsByHostID :many
SELECT * FROM deployments
WHERE host_ids LIKE '%' || ? || '%'
ORDER BY created_at DESC;
