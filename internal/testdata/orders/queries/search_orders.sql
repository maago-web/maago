-- name: SearchOrders :many OrderSummary
-- param: status?    string
-- param: from?      time.Time
-- param: to?        time.Time
-- param: sort       latest/amount
-- param: limit      int64
SELECT id, status, amount, created_at FROM orders
WHERE {
  {status}          AND status = :status
  {from & to}       AND created_at BETWEEN :from AND :to
  {!status}         AND status <> 'deleted'
}
ORDER BY
  {sort=latest}     created_at DESC
  {sort=amount}     amount DESC
LIMIT :limit
