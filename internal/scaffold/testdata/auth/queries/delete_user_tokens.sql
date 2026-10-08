-- name: DeleteUserTokens :exec
-- Removes the user's tokens of one purpose: a reset leaves a pending
-- verification link.
-- param: user_id int64
-- param: purpose string
DELETE FROM auth_token WHERE user_id = :user_id AND purpose = :purpose
