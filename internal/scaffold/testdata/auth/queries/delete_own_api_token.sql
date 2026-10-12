-- name: DeleteOwnAPIToken :exec
-- Removes a token of the user's: another's deletes nothing.
-- param: id int64
-- param: user_id int64
DELETE FROM auth_api_token WHERE id = :id AND user_id = :user_id
