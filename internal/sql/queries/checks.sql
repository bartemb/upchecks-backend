-- name: CreateCheck :one
INSERT INTO checks (service_id, success, status_code, latency)
VALUES ($1, $2, $3, $4)
RETURNING *;
