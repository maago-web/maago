-- name: OrderCreated :one time.Time
-- param: ids []int64
SELECT created_at FROM orders WHERE id IN (:ids)
