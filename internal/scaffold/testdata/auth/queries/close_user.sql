-- name: CloseUser :exec
-- Closes the account: it may not sign in, and its row stays.
-- param: id int64
UPDATE auth_user SET is_active = FALSE WHERE id = :id
