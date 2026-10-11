-- name: DeleteOldAttempts :exec
-- param: since time.Time
DELETE FROM auth_attempt WHERE created_at <= :since
