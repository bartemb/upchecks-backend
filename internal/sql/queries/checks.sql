-- name: CreateCheck :one
INSERT INTO checks (service_id, success, status_code, latency)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetServiceStats :one
SELECT
    COUNT(*)::int AS total_checks,
    COUNT(*) FILTER (WHERE success = true)::int AS successful_checks,
    COALESCE(AVG(latency), 0)::int AS avg_latency,
    COALESCE((SELECT c2.latency FROM checks c2 WHERE c2.service_id = $1 ORDER BY c2.created_at DESC LIMIT 1), 0)::int AS last_latency,
    COALESCE((SELECT c3.success FROM checks c3 WHERE c3.service_id = $1 ORDER BY c3.created_at DESC LIMIT 1), false)::bool AS last_success
FROM checks
WHERE service_id = $1;
