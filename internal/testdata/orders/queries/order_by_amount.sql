-- name: OrderByAmount :one Order
-- The columns in another order than the fields: Scan follows the columns.
-- param: amount int64
SELECT amount, status, id, updated_at, customer_id, created_at, priority
FROM orders
WHERE amount = :amount
