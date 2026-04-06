-- name: CreateCheck :one
INSERT INTO checks (service_id, success, status_code, latency)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUptimeForPeriod :one
SELECT
    COUNT(*)::int AS total_checks,
    COUNT(*) FILTER (WHERE success = true)::int AS successful_checks,
    COALESCE(AVG(latency), 0)::int AS avg_latency
FROM checks
WHERE service_id = $1
  AND created_at >= now() - $2::interval;

-- name: GetLastCheck :one
SELECT * FROM checks
WHERE service_id = $1
ORDER BY created_at DESC
LIMIT 1;
