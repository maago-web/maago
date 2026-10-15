-- name: SetOrderStatus :exec
-- param: status string
-- param: now time.Time
-- param: id int64
UPDATE orders SET status = :status, updated_at = :now WHERE id = :id
