-- name: VerifyUser :exec
-- param: id int64
-- param: now time.Time
UPDATE auth_user SET verified_at = :now WHERE id = :id
