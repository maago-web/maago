-- name: CustomerOrdersStrict :many CustomerOrderStrict
-- The same, declared without ?: a customer without orders is a Scan error.
-- column: amount int64
SELECT c.id, c.name, o.amount
FROM customers c LEFT JOIN orders o ON o.customer_id = c.id
ORDER BY c.id, o.id
