-- name: DeleteUserAPITokens :exec
-- param: user_id int64
DELETE FROM auth_api_token WHERE user_id = :user_id
