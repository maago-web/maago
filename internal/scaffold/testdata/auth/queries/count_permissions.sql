-- name: CountPermissions :one PermissionCount
-- The rows that let the user do action to the model of the app.
-- param: user_id int64
-- param: app string
-- param: model_name string
-- param: action string
-- column: n int64
SELECT COUNT(*) AS n FROM auth_permission
WHERE user_id = :user_id AND app = :app AND model_name = :model_name AND action = :action
