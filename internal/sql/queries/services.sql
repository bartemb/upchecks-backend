-- name: GetAllServices :many
SELECT * FROM services WHERE enabled = true;

-- name: GetServiceByID :one
SELECT * FROM services WHERE id = $1;
