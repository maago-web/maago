-- name: SetPassword :exec
-- Sets the password hash alone: what else changed meanwhile stays.
-- param: id int64
-- param: hash string
UPDATE auth_user SET password_hash = :hash WHERE id = :id
