-- name: DeleteOrders :exec
-- param: ids []int64
DELETE FROM orders WHERE id IN (:ids)
