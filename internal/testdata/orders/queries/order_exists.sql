-- name: OrderExists :one bool
-- A result of one column: no type to name (M10 task 3).
-- param: amounts []int64
SELECT EXISTS (SELECT 1 FROM orders WHERE amount IN (:amounts))
