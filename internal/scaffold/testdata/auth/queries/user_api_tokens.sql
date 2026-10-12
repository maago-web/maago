-- name: UserAPITokens :many APIToken
-- param: user_id int64
SELECT * FROM auth_api_token WHERE user_id = :user_id ORDER BY id
