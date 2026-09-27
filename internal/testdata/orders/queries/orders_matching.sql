-- name: OrdersMatching :many OrderMatch
-- Slices before a scalar, one of them under a condition: the placeholders
-- of each slice follow the scalars and the slices before it.
-- param: ids      []int64
-- param: statuses []string
-- param: times    []time.Time
-- param: min      int64
SELECT id, amount FROM orders
WHERE {
  id IN (:ids)
  {statuses} AND status IN (:statuses)
  AND created_at IN (:times)
  AND amount >= :min
}
ORDER BY id
