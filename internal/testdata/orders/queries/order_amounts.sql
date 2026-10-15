-- name: OrderAmounts :many int64
SELECT amount FROM orders ORDER BY id
