-- name: DeleteUserSessions :exec
-- Signs the user out everywhere.
-- param: user_id int64
DELETE FROM auth_session WHERE user_id = :user_id
