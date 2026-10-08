-- name: UseToken :one Token
-- Takes a token that has not expired, once: of two requests with one
-- token, only one gets the row.
-- param: hash string
-- param: purpose string
-- param: now time.Time
DELETE FROM auth_token
WHERE hash = :hash AND purpose = :purpose AND expires_at > :now
RETURNING *
