-- name: UserByEmail :one User
-- param: email string
SELECT * FROM auth_user WHERE email = :email
