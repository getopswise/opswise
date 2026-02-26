-- name: CountHosts :one
SELECT COUNT(*) FROM hosts;

-- name: CountProducts :one
SELECT COUNT(*) FROM products;

-- name: CountDeployments :one
SELECT COUNT(*) FROM deployments;
