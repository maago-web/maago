-- name: APITokenUser :one User
-- The user of an API token, if the user may sign in.
-- param: hash string
SELECT u.* FROM auth_user u
JOIN auth_api_token t ON t.user_id = u.id
WHERE t.hash = :hash
  AND u.is_active AND u.verified_at IS NOT NULL
