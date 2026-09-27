-- name: CustomerOrders :many CustomerOrder
-- A customer without orders comes once, with no amount.
-- column: amount? int64
SELECT c.id, c.name, o.amount
FROM customers c LEFT JOIN orders o ON o.customer_id = c.id
ORDER BY c.id, o.id
