-- name: VerifyUser :exec
-- Verifies the user, if the password is still the one checked.
-- param: id int64
-- param: password_hash string
-- param: now time.Time
UPDATE auth_user SET verified_at = :now WHERE id = :id AND password_hash = :password_hash
