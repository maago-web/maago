-- name: DeleteExpiredTokens :exec
-- param: now time.Time
DELETE FROM auth_token WHERE expires_at <= :now
