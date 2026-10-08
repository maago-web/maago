-- name: SessionUser :one User
-- The user of a session that has not ended, if the user may sign in.
-- param: hash string
-- param: now time.Time
SELECT u.* FROM auth_user u
JOIN auth_session s ON s.user_id = u.id
WHERE s.hash = :hash AND s.expires_at > :now
  AND u.is_active AND u.verified_at IS NOT NULL
