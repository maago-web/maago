-- name: TokenUser :one User
-- The user of a token that has not expired, without using it.
-- param: hash string
-- param: purpose string
-- param: now time.Time
SELECT u.* FROM auth_user u
JOIN auth_token t ON t.user_id = u.id
WHERE t.hash = :hash AND t.purpose = :purpose AND t.expires_at > :now
