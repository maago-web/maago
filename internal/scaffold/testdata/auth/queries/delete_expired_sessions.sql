-- name: DeleteExpiredSessions :exec
-- param: now time.Time
DELETE FROM auth_session WHERE expires_at <= :now
