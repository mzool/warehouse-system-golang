-- name: LogAudit :exec
INSERT INTO audit_logs (user_id, username, action, entity, entity_id, details)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListMonthAuditLogs :many
SELECT id, user_id, username, action, entity, entity_id, timestamp, details
FROM audit_logs
WHERE timestamp >= date_trunc('month', $1)
  AND timestamp < date_trunc('month', $1) + INTERVAL '1 month'
ORDER BY timestamp DESC
LIMIT $2 OFFSET $3;