-- name: GetNotificationRulesForService :many
SELECT * FROM notification_rules
WHERE service_id = $1 AND enabled = true;

-- name: GetNotificationChannelByID :one
SELECT * FROM notification_channels
WHERE id = $1;

-- name: GetLastNChecksByService :many
SELECT * FROM checks
WHERE service_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: CreateNotificationHistory :one
INSERT INTO notification_history (rule_id, channel_id, check_id, status)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLastNotificationForRule :one
SELECT * FROM notification_history
WHERE rule_id = $1
ORDER BY sent_at DESC
LIMIT 1;
